package test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"sort"
	"strconv"
	"strings"
	"text/template"
	"time"

	argocd "github.com/argoproj/argo-cd/pkg/apis/application/v1alpha1"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	extv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	k8sYaml "k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/yaml"
)

const (
	argoCDPasswordFile = "./argocd-password.txt"

	teleportSecret = `
apiVersion: v1
kind: Secret
metadata:
  name: teleport-auth-secret
  namespace: teleport
  labels:
    app.kubernetes.io/name: teleport
stringData:
  teleport.yaml: |
    auth_service:
      authentication:
        second_factor: "off"
        type: local
      cluster_name: gcp0
      public_addr: teleport-auth:3025
      tokens:
        - "proxy,node:{{ .Token }}"
        - "app:teleport-app-token"
    teleport:
      data_dir: /var/lib/teleport
      auth_token: {{ .Token }}
      log:
        output: stderr
        severity: DEBUG
      storage:
        type: dir
---
apiVersion: v1
kind: Secret
metadata:
  name: teleport-proxy-secret
  namespace: teleport
  labels:
    app.kubernetes.io/name: teleport
stringData:
  teleport.yaml: |
    proxy_service:
      https_cert_file: /var/lib/certs/tls.crt
      https_key_file: /var/lib/certs/tls.key
      kubernetes:
        enabled: true
        listen_addr: 0.0.0.0:3026
        public_addr: [ "teleport.gcp0.dev-ne.co:3026" ]
      listen_addr: 0.0.0.0:3023
      public_addr: [ "teleport.gcp0.dev-ne.co:443" ]
      web_listen_addr: 0.0.0.0:3080
    teleport:
      data_dir: /var/lib/teleport
      auth_token: {{ .Token }}
      auth_servers:
        - teleport-auth:3025
      log:
        output: stderr
        severity: DEBUG
`
)

func prepareNodes() {
	It("should increase worker nodes", func() {
		ExecSafeAt(boot0, "ckecli", "constraints", "set", "minimum-workers", "4")
		Eventually(func() error {
			stdout, stderr, err := ExecAt(boot0, "kubectl", "get", "nodes", "-o", "json")
			if err != nil {
				return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
			}

			var nl corev1.NodeList
			err = json.Unmarshal(stdout, &nl)
			if err != nil {
				return err
			}

			// control-plane: 3, minimum-workers: 4
			if len(nl.Items) != 7 {
				return fmt.Errorf("too few nodes: %d", len(nl.Items))
			}

			readyNodeSet := make(map[string]struct{})
			for _, n := range nl.Items {
				for _, c := range n.Status.Conditions {
					if c.Type == corev1.NodeReady && c.Status == corev1.ConditionTrue {
						readyNodeSet[n.Name] = struct{}{}
					}
				}
			}
			if len(readyNodeSet) != 7 {
				return fmt.Errorf("some nodes are not ready")
			}

			return nil
		}).Should(Succeed())
	})
}

func createNamespaceIfNotExists(ns string) {
	_, _, err := ExecAt(boot0, "kubectl", "get", "namespace", ns)
	if err == nil {
		return
	}

	ExecSafeAt(boot0, "kubectl", "create", "namespace", ns)
	Eventually(func() error {
		stdout, stderr, err := ExecAt(boot0, "kubectl", "get", "sa", "default", "-n", ns)
		if err != nil {
			return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
		}
		return nil
	}).Should(Succeed())
}

// testSetup tests setup of Argo CD
func testSetup() {
	if !doUpgrade {
		It("should create secrets of account.json", func() {
			By("loading account.json")
			data, err := ioutil.ReadFile("account.json")
			Expect(err).ShouldNot(HaveOccurred())

			By("creating namespace and secrets for external-dns")
			createNamespaceIfNotExists("external-dns")
			_, _, err = ExecAt(boot0, "kubectl", "--namespace=external-dns", "get", "secret", "clouddns")
			if err != nil {
				_, stderr, err := ExecAtWithInput(boot0, data, "kubectl", "--namespace=external-dns",
					"create", "secret", "generic", "clouddns", "--from-file=account.json=/dev/stdin")
				Expect(err).ShouldNot(HaveOccurred(), "stderr=%s", stderr)
			}

			By("creating namespace and secrets for cert-manager")
			createNamespaceIfNotExists("cert-manager")
			_, _, err = ExecAt(boot0, "kubectl", "--namespace=cert-manager", "get", "secret", "clouddns")
			if err != nil {
				_, stderr, err := ExecAtWithInput(boot0, data, "kubectl", "--namespace=cert-manager",
					"create", "secret", "generic", "clouddns", "--from-file=account.json=/dev/stdin")
				Expect(err).ShouldNot(HaveOccurred(), "stderr=%s", stderr)
			}
		})

		It("should prepare secrets", func() {
			By("creating namespace and secrets for grafana")
			createNamespaceIfNotExists("sandbox")

			By("creating namespace and secrets for teleport")
			stdout, stderr, err := ExecAt(boot0, "env", "ETCDCTL_API=3", "etcdctl", "--cert=/etc/etcd/backup.crt", "--key=/etc/etcd/backup.key",
				"get", "--print-value-only", "/neco/teleport/auth-token")
			Expect(err).NotTo(HaveOccurred(), "stdout: %s, stderr: %s", stdout, stderr)
			teleportToken := strings.TrimSpace(string(stdout))
			teleportTmpl := template.Must(template.New("").Parse(teleportSecret))
			buf := bytes.NewBuffer(nil)
			err = teleportTmpl.Execute(buf, struct {
				Token string
			}{
				Token: teleportToken,
			})
			Expect(err).NotTo(HaveOccurred())
			createNamespaceIfNotExists("teleport")
			stdout, stderr, err = ExecAtWithInput(boot0, buf.Bytes(), "kubectl", "apply", "-n", "teleport", "-f", "-")
			Expect(err).NotTo(HaveOccurred(), "stdout: %s, stderr: %s", stdout, stderr)
		})
	}

	It("should checkout neco-apps repository@"+commitID, func() {
		ExecSafeAt(boot0, "rm", "-rf", "neco-apps")

		ExecSafeAt(boot0, "env", "https_proxy=http://10.0.49.3:3128",
			"git", "clone", "https://github.com/cybozu-go/neco-apps")
		ExecSafeAt(boot0, "cd neco-apps; git checkout "+commitID)
	})

	It("should setup applications", func() {
		if !doUpgrade {
			applyNetworkPolicy()
			setupArgoCD()
		}
		ExecSafeAt(boot0, "sed", "-i", "s/release/"+commitID+"/", "./neco-apps/argocd-config/base/*.yaml")
		ExecSafeAt(boot0, "sed", "-i", "s/release/"+commitID+"/", "./neco-apps/argocd-config/overlays/"+overlayName+"/*.yaml")
		applyAndWaitForApplications(commitID)
	})

	It("should set DNS", func() {
		var ip string
		By("confirming that unbound is exported")
		Eventually(func() error {
			stdout, stderr, err := ExecAt(boot0, "kubectl", "--namespace=internet-egress",
				"get", "service/unbound-bastion", "-o=json")
			if err != nil {
				return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
			}
			service := new(corev1.Service)
			err = json.Unmarshal(stdout, service)
			if err != nil {
				return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
			}

			if len(service.Status.LoadBalancer.Ingress) != 1 {
				return fmt.Errorf("unable to get LoadBalancer's IP address. stdout: %s, stderr: %s, err: %w", stdout, stderr, err)
			}

			ip = service.Status.LoadBalancer.Ingress[0].IP

			return nil
		}).Should(Succeed())

		By("setting dns address to neco config")
		stdout, stderr, err := ExecAt(boot0, "neco", "config", "set", "dns", ip)
		Expect(err).NotTo(HaveOccurred(), "stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
	})

	It("should set HTTP proxy", func() {
		var proxyIP string
		Eventually(func() error {
			stdout, stderr, err := ExecAt(boot0, "kubectl", "-n", "internet-egress", "get", "svc", "squid", "-o", "json")
			if err != nil {
				return fmt.Errorf("stdout: %v, stderr: %v, err: %v", stdout, stderr, err)
			}

			var svc corev1.Service
			err = json.Unmarshal(stdout, &svc)
			if err != nil {
				return fmt.Errorf("stdout: %v, err: %v", stdout, err)
			}

			if len(svc.Status.LoadBalancer.Ingress) == 0 {
				return errors.New("len(svc.Status.LoadBalancer.Ingress) == 0")
			}
			proxyIP = svc.Status.LoadBalancer.Ingress[0].IP
			return nil
		}).Should(Succeed())

		proxyURL := fmt.Sprintf("http://%s:3128", proxyIP)
		ExecSafeAt(boot0, "neco", "config", "set", "proxy", proxyURL)
		ExecSafeAt(boot0, "neco", "config", "set", "node-proxy", proxyURL)

		necoVersion := string(ExecSafeAt(boot0, "dpkg-query", "-W", "-f", "'${Version}'", "neco"))
		rolePaths := strings.Fields(string(ExecSafeAt(boot0, "ls", "/usr/share/neco/ignitions/roles/*/site.yml")))
		for _, rolePath := range rolePaths {
			role := strings.Split(rolePath, "/")[6]
			ExecSafeAt(boot0, "sabactl", "ignitions", "delete", role, necoVersion)
		}
		ExecSafeAt(boot0, "neco", "init-data", "--ignitions-only")
	})
}

func applyAndWaitForApplications(commitID string) {
	By("creating Argo CD app")
	Eventually(func() error {
		stdout, stderr, err := ExecAt(boot0, "argocd", "app", "create", "argocd-config",
			"--upsert",
			"--repo", "https://github.com/cybozu-go/neco-apps.git",
			"--path", "argocd-config/overlays/"+overlayName,
			"--dest-namespace", "argocd",
			"--dest-server", "https://kubernetes.default.svc",
			"--sync-policy", "none",
			"--revision", commitID)
		if err != nil {
			return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
		}
		return nil
	}).Should(Succeed())

	Eventually(func() error {
		stdout, stderr, err := ExecAt(boot0, "cd", "./neco-apps", "&&", "argocd", "app", "sync", "argocd-config", "--local", "argocd-config/overlays/"+overlayName, "--async")
		if err != nil {
			return fmt.Errorf("stdout=%s, stderr=%s: %w", string(stdout), string(stderr), err)
		}
		return nil
	}).Should(Succeed())

	By("getting application list and sort the applications with sync wave in ascending order")
	stdout, _, err := kustomizeBuild("../argocd-config/overlays/" + overlayName)
	Expect(err).ShouldNot(HaveOccurred())

	type nameAndWave struct {
		name     string
		syncWave float64
	}
	var appList []nameAndWave
	y := k8sYaml.NewYAMLReader(bufio.NewReader(bytes.NewReader(stdout)))
	for {
		data, err := y.Read()
		if err == io.EOF {
			break
		}
		Expect(err).ShouldNot(HaveOccurred())

		var app argocd.Application
		err = yaml.Unmarshal(data, &app)
		if err != nil {
			continue
		}

		// Skip if the app is for tenants
		if app.Labels["is-tenant"] == "true" {
			continue
		}

		wave, err := strconv.ParseFloat(app.GetAnnotations()["argocd.argoproj.io/sync-wave"], 32)
		Expect(err).ShouldNot(HaveOccurred())
		appList = append(appList, nameAndWave{name: app.Name, syncWave: wave})
	}

	sort.Slice(appList, func(i, j int) bool {
		if appList[i].syncWave != appList[j].syncWave {
			return appList[i].syncWave < appList[j].syncWave
		} else {
			return strings.Compare(appList[i].name, appList[j].name) <= 0
		}
	})

	fmt.Printf("application list:\n")
	for _, app := range appList {
		fmt.Printf("  %4.1f: %s\n", app.syncWave, app.name)
	}
	Expect(appList).ShouldNot(HaveLen(0))

	By("waiting initialization")
	checkAllAppsSynced := func() error {
		for _, target := range appList {
			appStdout, stderr, err := ExecAt(boot0, "argocd", "app", "get", "-o", "json", target.name)
			if err != nil {
				return fmt.Errorf("stdout: %s, stderr: %s, err: %v", appStdout, stderr, err)
			}
			var app argocd.Application
			err = json.Unmarshal(appStdout, &app)
			if err != nil {
				return fmt.Errorf("stdout: %s, err: %v", appStdout, err)
			}
			if app.Status.Sync.ComparedTo.Source.TargetRevision != commitID {
				return errors.New(target.name + " does not have correct target yet")
			}
			if app.Status.Sync.Status == argocd.SyncStatusCodeSynced &&
				app.Status.Health.Status == argocd.HealthStatusHealthy &&
				app.Operation == nil {
				continue
			}

			// TODO: remove this block when the following PR is released.
			// https://github.com/cybozu-go/neco-apps/pull/966
			if doUpgrade && app.Name == "sandbox" {
				stdout, stderr, err := ExecAt(boot0, "kubectl", "-n", "sandbox", "get", "svc", "grafana", "-o", "json")
				if err != nil {
					return fmt.Errorf("stdout: %s, stderr: %s, err: %v", appStdout, stderr, err)
				}

				var svc corev1.Service
				err = json.Unmarshal(stdout, &svc)
				if err != nil {
					return fmt.Errorf("stdout: %v, err: %v", stdout, err)
				}
				if svc.Spec.Type != corev1.ServiceTypeClusterIP {
					stdout, stderr, err = ExecAt(boot0, "kubectl", "delete", "-n", "sandbox", "svc", "grafana")
					if err != nil {
						return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
					}
				}
			}

			// In upgrade test, syncing network-policy app may cause temporal network disruption.
			// It leads to ArgoCD's improper behavior. In spite of the network-policy app becomes Synced/Healthy, the operation does not end.
			// So terminate the unexpected operation manually in upgrade test.
			// TODO: This is workaround for ArgoCD's improper behavior. When this issue (T.B.D.) is closed, delete this block.
			if app.Status.Sync.Status == argocd.SyncStatusCodeSynced &&
				app.Status.Health.Status == argocd.HealthStatusHealthy &&
				app.Operation != nil &&
				app.Status.OperationState.Phase == "Running" {
				fmt.Printf("%s terminate unexpected operation: app=%s\n", time.Now().Format(time.RFC3339), target.name)
				stdout, stderr, err := ExecAt(boot0, "argocd", "app", "terminate-op", target.name)
				if err != nil {
					return fmt.Errorf("failed to terminate operation. app: %s, stdout: %s, stderr: %s, err: %v", target.name, stdout, stderr, err)
				}
				stdout, stderr, err = ExecAt(boot0, "argocd", "app", "sync", target.name)
				if err != nil {
					return fmt.Errorf("failed to sync application. app: %s, stdout: %s, stderr: %s, err: %v", target.name, stdout, stderr, err)
				}
			}

			return fmt.Errorf("%s is not initialized. argocd app get %s -o json: %s", target.name, target.name, appStdout)
		}
		return nil
	}
	// want to do "Eventually( Consistently(checkAllAppsSynced, 15sec, 1sec) )"
	Eventually(func() error {
		for i := 0; i < 15; i++ {
			if i%5 == 1 {
				fmt.Printf("Checking all app synced: count=%d\n", i)
			}
			err := checkAllAppsSynced()
			if err != nil {
				return err
			}
			time.Sleep(1 * time.Second)
		}
		return nil
	}, 40*time.Minute).Should(Succeed())
}

// Sometimes synchronization fails when argocd applies network policies.
// So, apply the network policies before argocd synchronization.
// TODO: This is a workaround. When this issue is solved, delete this func.
func applyNetworkPolicy() {
	By("apply namespaces")
	namespaceManifest, stderr, err := kustomizeBuild("../namespaces/base/")
	Expect(err).ShouldNot(HaveOccurred(), "failed to kustomize build: stderr=%s", stderr)

	stdout, stderr, err := ExecAtWithInput(boot0, namespaceManifest, "kubectl", "apply", "-f", "-")
	Expect(err).ShouldNot(HaveOccurred(), "failed to apply namespaces: stdout=%s, stderr=%s", stdout, stderr)

	stdout, stderr, err = ExecAt(boot0, "kubectl", "apply", "-f", "./neco-apps/customer-egress/base/namespace.yaml")
	Expect(err).ShouldNot(HaveOccurred(), "failed to apply customer-egress namespace: stdout=%s, stderr=%s", stdout, stderr)

	By("apply network-policies")
	netpolManifest, stderr, err := kustomizeBuild("../network-policy/base/")
	Expect(err).ShouldNot(HaveOccurred(), "failed to kustomize build: stderr=%s", stderr)

	y := k8sYaml.NewYAMLReader(bufio.NewReader(bytes.NewReader(netpolManifest)))
	for {
		data, err := y.Read()
		if err == io.EOF {
			break
		}
		Expect(err).ShouldNot(HaveOccurred())

		var crd extv1beta1.CustomResourceDefinition
		err = yaml.Unmarshal(data, &crd)
		if err != nil {
			continue
		}
		if crd.Kind != "CustomResourceDefinition" {
			continue
		}

		stdout, stderr, err = ExecAtWithInput(boot0, data, "kubectl", "apply", "-f", "-")
		Expect(err).ShouldNot(HaveOccurred(), "failed to apply crd: stdout=%s, stderr=%s", stdout, stderr)

		Eventually(func() error {
			stdout, stderr, err := ExecAt(boot0, "kubectl", "get", "crd/"+crd.Name, "-o=json")
			if err != nil {
				return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
			}
			gotcrd := new(extv1beta1.CustomResourceDefinition)
			err = json.Unmarshal(stdout, gotcrd)
			if err != nil {
				return err
			}
			for _, cond := range gotcrd.Status.Conditions {
				if cond.Type == extv1beta1.Established && cond.Status == extv1beta1.ConditionTrue {
					return nil
				}
			}
			return fmt.Errorf("CRD is not established: %s", crd.Name)
		}, 1*time.Minute).Should(Succeed())
	}

	stdout, stderr, err = ExecAtWithInput(boot0, netpolManifest, "kubectl", "apply", "-f", "-")
	Expect(err).ShouldNot(HaveOccurred(), "failed to apply network-policy: stdout=%s, stderr=%s", stdout, stderr)

	Eventually(func() error {
		stdout, stderr, err := ExecAt(boot0, "kubectl", "--namespace=kube-system", "get", "deployment/calico-typha", "-o=json")
		if err != nil {
			return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
		}
		deployment := new(appsv1.Deployment)
		err = json.Unmarshal(stdout, deployment)
		if err != nil {
			return err
		}
		if deployment.Status.Replicas != deployment.Status.ReadyReplicas {
			return fmt.Errorf("calico-typha deployment's ReadyReplicas is not %d: %d", int(deployment.Status.Replicas), int(deployment.Status.ReadyReplicas))
		}
		return nil
	}, 3*time.Minute).Should(Succeed())

	Eventually(func() error {
		stdout, stderr, err := ExecAt(boot0, "kubectl", "--namespace=kube-system", "get", "daemonset/calico-node", "-o=json")
		if err != nil {
			return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
		}
		daemonset := new(appsv1.DaemonSet)
		err = json.Unmarshal(stdout, daemonset)
		if err != nil {
			return err
		}
		if daemonset.Status.DesiredNumberScheduled != daemonset.Status.NumberReady {
			return fmt.Errorf("calico-node daemonset's NumberReady is not %d: %d", int(daemonset.Status.DesiredNumberScheduled), int(daemonset.Status.NumberReady))
		}
		return nil
	}, 3*time.Minute).Should(Succeed())
}

func setupArgoCD() {
	By("installing Argo CD")
	createNamespaceIfNotExists("argocd")
	data, err := ioutil.ReadFile("install.yaml")
	Expect(err).ShouldNot(HaveOccurred())
	_, stderr, err := ExecAtWithInput(boot0, data, "kubectl", "apply", "-n", "argocd", "-f", "-")
	Expect(err).ShouldNot(HaveOccurred(), "faied to apply install.yaml. stderr=%s", stderr)

	By("waiting Argo CD comes up")
	// admin password is same as pod name
	var podList corev1.PodList
	Eventually(func() error {
		stdout, stderr, err := ExecAt(boot0, "kubectl", "get", "pods", "-n", "argocd",
			"-l", "app.kubernetes.io/name=argocd-server", "-o", "json")
		if err != nil {
			return fmt.Errorf("unable to get argocd-server pods. stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
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

	saveArgoCDPassword(podList.Items[0].Name)

	By("getting node address")
	var nodeList corev1.NodeList
	data = ExecSafeAt(boot0, "kubectl", "get", "nodes", "-o", "json")
	err = json.Unmarshal(data, &nodeList)
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
			"--insecure", "--username", "admin", "--password", loadArgoCDPassword())
		if err != nil {
			return fmt.Errorf("failed to login to argocd. stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
		}
		return nil
	}).Should(Succeed())
}
