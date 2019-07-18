package test

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	argocd "github.com/argoproj/argo-cd/pkg/apis/application/v1alpha1"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
)

const (
	appSyncOrderFile   = "../app-sync-order.txt"
	alertmanagerSecret = `
route:
  receiver: slack
  group_wait: 5s # Send a notification after 5 seconds
  routes:
  - receiver: slack
    continue: true # Continue notification to next receiver

# Receiver configurations
receivers:
- name: slack
  slack_configs:
  - channel: '#test'
    api_url: https://hooks.slack.com/services/XXX/XXX
    icon_url: https://avatars3.githubusercontent.com/u/3380462 # Prometheus icon
    http_config:
      proxy_url: http://squid.internet-egress.svc.cluster.local:3128
`
)

// testSetup tests setup of Argo CD
func testSetup() {
	It("should list all apps in app-sync-order.txt", func() {
		appList := loadSyncOrder()
		kustomFile, err := filepath.Abs("../argocd-config/base/kustomization.yaml")
		Expect(err).ShouldNot(HaveOccurred())
		stdout, err := ioutil.ReadFile(kustomFile)
		Expect(err).ShouldNot(HaveOccurred())
		k := struct {
			Resources []string `yaml:"resources"`
		}{}
		Expect(yaml.Unmarshal(stdout, &k)).ShouldNot(HaveOccurred())
		var resources []string
		for _, r := range k.Resources {
			r = r[:len(r)-len(filepath.Ext(r))]
			resources = append(resources, r)
		}
		sort.Strings(appList)
		sort.Strings(resources)
		Expect(appList).Should(Equal(resources))
	})

	It("should be ready K8s cluster after loading snapshot", func() {
		By("re-issuing kubeconfig")
		Eventually(func() error {
			_, _, err := ExecAt(boot0, "ckecli", "kubernetes", "issue", ">", ".kube/config")
			if err != nil {
				return err
			}
			return nil
		}).Should(Succeed())

		By("waiting nodes")
		Eventually(func() error {
			stdout, _, err := ExecAt(boot0, "kubectl", "get", "nodes", "-o", "json")
			if err != nil {
				return err
			}

			var nl corev1.NodeList
			err = json.Unmarshal(stdout, &nl)
			if err != nil {
				return err
			}

			if len(nl.Items) != 5 {
				return fmt.Errorf("too few nodes: %d", len(nl.Items))
			}

		OUTER:
			for _, n := range nl.Items {
				for _, cond := range n.Status.Conditions {
					if cond.Type != corev1.NodeReady {
						continue
					}
					if cond.Status != corev1.ConditionTrue {
						return fmt.Errorf("node %s is not ready", n.Name)
					}
					continue OUTER
				}

				return fmt.Errorf("node %s has no readiness status", n.Name)
			}

			return nil
		}).Should(Succeed())
	})

	It("should prepare secrets", func() {
		By("creating namespace and secrets for external-dns")
		_, stderr, err := ExecAt(boot0, "kubectl", "create", "namespace", "external-dns")
		Expect(err).ShouldNot(HaveOccurred(), "stderr=%s", stderr)
		_, stderr, err = ExecAt(boot0, "kubectl", "--namespace=external-dns", "create", "secret",
			"generic", "external-dns", "--from-file=account.json")
		Expect(err).ShouldNot(HaveOccurred(), "stderr=%s", stderr)

		By("creating namespace and secrets for alertmanager")
		stdout, stderr, err := ExecAtWithInput(boot0, []byte(alertmanagerSecret), "dd", "of=alertmanager.yaml")
		Expect(err).NotTo(HaveOccurred(), "stdout: %s, stderr: %s", stdout, stderr)
		stdout, stderr, err = ExecAt(boot0, "kubectl", "create", "namespace", "monitoring")
		Expect(err).NotTo(HaveOccurred(), "stdout: %s, stderr: %s", stdout, stderr)
		stdout, stderr, err = ExecAt(boot0, "kubectl", "--namespace=monitoring", "create", "secret",
			"generic", "alertmanager", "--from-file", "alertmanager.yaml")
		Expect(err).NotTo(HaveOccurred(), "stdout: %s, stderr: %s", stdout, stderr)
	})

	It("should install Argo CD", func() {
		data, err := ioutil.ReadFile("install.yaml")
		Expect(err).ShouldNot(HaveOccurred())
		Eventually(func() error {
			stdout, stderr, err := ExecAtWithInput(boot0, data, "kubectl", "apply", "-n", "argocd", "-f", "-")
			if err != nil {
				return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
			}
			return nil
		}).Should(Succeed())
	})

	It("should login to Argo CD", func() {
		By("getting password")
		// admin password is same as pod name
		var podList corev1.PodList
		Eventually(func() error {
			stdout, stderr, err := ExecAt(boot0, "kubectl", "get", "pods", "-n", "argocd",
				"-l", "app.kubernetes.io/name=argocd-server", "-o", "json")
			if err != nil {
				return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
			}
			err = json.Unmarshal(stdout, &podList)
			if err != nil {
				return err
			}
			if podList.Items == nil {
				return errors.New("podList.Items is nil")
			}
			if len(podList.Items) != 1 {
				return fmt.Errorf("podList.Items is not 1: %d", len(podList.Items))
			}
			return nil
		}).Should(Succeed())

		password := podList.Items[0].Name

		By("getting node address")
		var nodeList corev1.NodeList
		data := ExecSafeAt(boot0, "kubectl", "get", "nodes", "-o", "json")
		err := json.Unmarshal(data, &nodeList)
		Expect(err).ShouldNot(HaveOccurred(), "data=%s", string(data))
		Expect(nodeList.Items).ShouldNot(BeEmpty())
		node := nodeList.Items[0]

		var nodeAddress string
		for _, addr := range node.Status.Addresses {
			if addr.Type != corev1.NodeInternalIP {
				continue
			}
			nodeAddress = addr.Address
		}
		Expect(nodeAddress).ShouldNot(BeNil())

		By("getting node port")
		var svc corev1.Service
		data = ExecSafeAt(boot0, "kubectl", "get", "svc/argocd-server", "-n", "argocd", "-o", "json")
		err = json.Unmarshal(data, &svc)
		Expect(err).ShouldNot(HaveOccurred(), "data=%s", string(data))
		Expect(svc.Spec.Ports).ShouldNot(BeEmpty())

		var nodePort string
		for _, port := range svc.Spec.Ports {
			if port.Name != "http" {
				continue
			}
			nodePort = strconv.Itoa(int(port.NodePort))
		}
		Expect(nodePort).ShouldNot(BeNil())

		By("logging in to Argo CD")
		Eventually(func() error {
			stdout, stderr, err := ExecAt(boot0, "argocd", "login", nodeAddress+":"+nodePort,
				"--insecure", "--username", "admin", "--password", password)
			if err != nil {
				return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
			}
			return nil
		}).Should(Succeed())
	})

	It("should checkout neco-apps repository", func() {
		ExecSafeAt(boot0, "env", "https_proxy=http://10.0.49.3:3128", "git", "clone", "https://github.com/cybozu-go/neco-apps")
		ExecSafeAt(boot0, "cd", "neco-apps", ";", "git", "checkout", commitID)
		ExecSafeAt(boot0, "sed", "-i", "s/release/"+commitID+"/", "./neco-apps/argocd-config/base/*.yaml")
	})

	It("should setup Argo CD application as Argo CD app", func() {
		By("creating Argo CD app")
		ExecSafeAt(boot0, "kubectl", "apply", "-k", "./neco-apps/argocd-config/overlays/gcp")

		syncOrder := loadSyncOrder()

		By("waiting initialization")
		Eventually(func() error {
		OUTER:
			for _, appName := range syncOrder {
				out := ExecSafeAt(boot0, "argocd", "app", "get", "-o", "json", appName)
				var app argocd.Application
				err := json.Unmarshal(out, &app)
				if err != nil {
					return err
				}
				st := app.Status
				if st.Sync.Status == argocd.SyncStatusCodeSynced && st.Health.Status == argocd.HealthStatusHealthy {
					continue
				}
				for _, cond := range st.Conditions {
					if cond.Type == argocd.ApplicationConditionSyncError {
						continue OUTER
					}
				}
				return errors.New(appName + " is not initialized")
			}
			return nil
		}).Should(Succeed())

		for _, appName := range syncOrder {
			By("syncing " + appName + " manually")
			ExecSafeAt(boot0, "argocd", "app", "sync", appName)
		}
	})
}

func loadSyncOrder() []string {
	orderFileAbs, err := filepath.Abs(appSyncOrderFile)
	Expect(err).ShouldNot(HaveOccurred())
	stdout, err := ioutil.ReadFile(orderFileAbs)
	Expect(err).ShouldNot(HaveOccurred())
	lines := strings.Split(string(stdout), "\n")
	var results []string
	for _, line := range lines {
		if line == "" {
			continue
		}
		results = append(results, line)
	}

	return results
}
