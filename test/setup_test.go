package test

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"strconv"

	argoappv1 "github.com/argoproj/argo-cd/pkg/apis/application/v1alpha1"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
)

const alertmanagerSecret = `
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

// testSetup tests setup of Argo CD
func testSetup() {
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

		By("disabling resource validation of the cert-manager on external-dns namespace")
		_, stderr, err := ExecAt(boot0, "kubectl", "label", "namespace", "external-dns", "certmanager.k8s.io/disable-validation=true")
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

	It("should checkout neco-ops repository", func() {
		ExecSafeAt(boot0, "env", "https_proxy=http://10.0.49.3:3128", "git", "clone", "https://github.com/cybozu-go/neco-ops")
		ExecSafeAt(boot0, "cd", "neco-ops", ";", "git", "checkout", commitID)
		ExecSafeAt(boot0, "sed", "-i", "s/release/"+commitID+"/", "./neco-ops/argocd-config/base/*.yaml")
	})

	It("should setup Argo CD application as Argo CD app", func() {
		By("creating Argo CD app")
		ExecSafeAt(boot0, "kubectl", "apply", "-k", "./neco-ops/argocd-config/overlays/gcp")

		By("checking app status")
		Eventually(func() error {
			apps := []string{"argocd", "external-dns", "ingress", "metallb", "monitoring"}
			for _, a := range apps {
				stdout, stderr, err := ExecAt(boot0, "kubectl", "get", "app", a, "-n", "argocd", "-o", "json")
				if err != nil {
					return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
				}
				var app argoappv1.Application
				err = json.Unmarshal(stdout, &app)
				if err != nil {
					return err
				}

				for _, r := range app.Status.Resources {
					if r.Status != argoappv1.SyncStatusCodeSynced {
						return fmt.Errorf("%s is not yet Synced: %s", a, r.Status)
					}
					if r.Health.Status != argoappv1.HealthStatusHealthy {
						return fmt.Errorf("%s is not yet Healthy: %s", a, r.Health.Status)
					}
				}
			}
			return nil
		}).Should(Succeed())
	})
}
