package test

import (
	"bufio"
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"text/template"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	k8sYaml "k8s.io/apimachinery/pkg/util/yaml"
)

const (
	argoCDPasswordFile = "./argocd-password.txt"
)

var decUnstructured = yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)

//go:embed testdata/setup-teleport.yaml
var setupTeleportYAML string

func prepareNodes() {
	It("should increase worker nodes", func() {
		Eventually(func() error {
			_, _, err := ExecAt(boot0, "ckecli", "cluster", "get")
			return err
		}).Should(Succeed())
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

func createNamespaceIfNotExists(ns string, privileged bool) {
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
	if privileged {
		stdout, stderr, err := ExecAt(boot0, "kubectl", "label", "namespace/"+ns, "pod-security.cybozu.com/policy=privileged")
		Expect(err).ShouldNot(HaveOccurred(), "stdout=%s, stderr=%s", stdout, stderr)
	}
}

// testSetup tests setup of Argo CD
func testSetup() {
	if !doUpgrade {
		It("should create secrets of account.json", func() {
			By("loading account.json")
			data, err := os.ReadFile("account.json")
			Expect(err).ShouldNot(HaveOccurred())

			By("creating namespace and secrets for external-dns")
			createNamespaceIfNotExists("external-dns", false)
			_, _, err = ExecAt(boot0, "kubectl", "--namespace=external-dns", "get", "secret", "clouddns")
			if err != nil {
				_, stderr, err := ExecAtWithInput(boot0, data, "kubectl", "--namespace=external-dns",
					"create", "secret", "generic", "clouddns", "--from-file=account.json=/dev/stdin")
				Expect(err).ShouldNot(HaveOccurred(), "stderr=%s", stderr)
			}

			By("creating namespace and secrets for cert-manager")
			createNamespaceIfNotExists("cert-manager", true)
			_, _, err = ExecAt(boot0, "kubectl", "--namespace=cert-manager", "get", "secret", "clouddns")
			if err != nil {
				_, stderr, err := ExecAtWithInput(boot0, data, "kubectl", "--namespace=cert-manager",
					"create", "secret", "generic", "clouddns", "--from-file=account.json=/dev/stdin")
				Expect(err).ShouldNot(HaveOccurred(), "stderr=%s", stderr)
			}
		})

		It("should prepare secrets", func() {
			By("creating namespace and secrets for grafana")
			createNamespaceIfNotExists("sandbox", true)

			By("creating namespace and secrets for teleport")
			stdout, stderr, err := ExecAt(boot0, "env", "ETCDCTL_API=3", "etcdctl", "--cert=/etc/etcd/backup.crt", "--key=/etc/etcd/backup.key",
				"get", "--print-value-only", "/neco/teleport/auth-token")
			Expect(err).NotTo(HaveOccurred(), "stdout: %s, stderr: %s", stdout, stderr)
			teleportToken := strings.TrimSpace(string(stdout))
			teleportTmpl := template.Must(template.New("").Parse(setupTeleportYAML))
			buf := bytes.NewBuffer(nil)
			err = teleportTmpl.Execute(buf, struct {
				Token string
			}{
				Token: teleportToken,
			})
			Expect(err).NotTo(HaveOccurred())
			createNamespaceIfNotExists("teleport", false)
			stdout, stderr, err = ExecAtWithInput(boot0, buf.Bytes(), "kubectl", "apply", "-n", "teleport", "-f", "-")
			Expect(err).NotTo(HaveOccurred(), "stdout: %s, stderr: %s", stdout, stderr)
		})
	} else {
		It("should set privileged label to sandbox namespace", func() {
			stdout, stderr, err := ExecAt(boot0, "kubectl", "label", "namespace/sandbox", "pod-security.cybozu.com/policy=privileged")
			Expect(err).ShouldNot(HaveOccurred(), "stdout=%s, stderr=%s", stdout, stderr)
		})
	}

	It("should apply zerossl secrets", func() {
		By("loading zerossl-secret-resource.json")
		data, err := os.ReadFile("zerossl-secret-resource.json")
		Expect(err).ShouldNot(HaveOccurred())

		By("creating namespace and secrets for zerossl")
		createNamespaceIfNotExists("cert-manager", true)
		_, stderr, err := ExecAtWithInput(boot0, data, "kubectl", "apply", "-f", "-")
		Expect(err).ShouldNot(HaveOccurred(), "stderr=%s", stderr)
	})

	It("should checkout neco-apps repository@"+commitID, func() {
		ExecSafeAt(boot0, "rm", "-rf", "neco-apps")

		ExecSafeAt(boot0, "env", "https_proxy=http://10.0.49.3:3128",
			"git", "clone", "https://github.com/cybozu-go/neco-apps")
		ExecSafeAt(boot0, "cd neco-apps; git checkout "+commitID)
	})

	It("should setup applications", func() {
		if !doUpgrade {
			applyNetworkPolicy()
			applyCertManager()
			setupArgoCD()
			applyMutatingWebhooks()
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
		By("getting proxy address")
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
		ExecSafeAt(boot0, "neco", "config", "set", "node-proxy", proxyURL)
		ExecSafeAt(boot0, "neco", "config", "set", "proxy", proxyURL)

		By("waiting for docker to be restarted")
		Eventually(func() error {
			stdout, stderr, err := ExecAt(boot0, "docker", "info", "-f", "{{.HTTPProxy}}")
			if err != nil {
				return fmt.Errorf("docker info failed: %s: %w", stderr, err)
			}
			if strings.TrimSpace(string(stdout)) != proxyURL {
				return errors.New("docker has not been restarted")
			}
			return nil
		}).Should(Succeed())

		// want to do "Eventually( Consistently(<check logic>, 15sec, 1sec) )"
		By("waiting for the system to become stable")
		Eventually(func() error {
			st := time.Now()
			for {
				if time.Since(st) > 15*time.Second {
					return nil
				}
				_, _, err := ExecAt(boot0, "sabactl", "ipam", "get")
				if err != nil {
					return err
				}
				_, _, err = ExecAt(boot0, "ckecli", "cluster", "get")
				if err != nil {
					return err
				}
				time.Sleep(1 * time.Second)
			}
		}).Should(Succeed())
	})

	It("should reconfigure ignitions", func() {
		necoVersion := string(ExecSafeAt(boot0, "dpkg-query", "-W", "-f", "'${Version}'", "neco"))
		rolePaths := strings.Fields(string(ExecSafeAt(boot0, "ls", "/usr/share/neco/ignitions/roles/*/site.yml")))
		for _, rolePath := range rolePaths {
			role := strings.Split(rolePath, "/")[6]
			ExecSafeAt(boot0, "sabactl", "ignitions", "delete", role, necoVersion)
		}
		Eventually(func() error {
			_, stderr, err := ExecAt(boot0, "neco", "init-data", "--ignitions-only")
			if err != nil {
				fmt.Fprintf(os.Stderr, "neco init-data failed: %s: %v\n", stderr, err)
				return fmt.Errorf("neco init-data failed: %s: %w", stderr, err)
			}
			return nil
		}).Should(Succeed())
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

	By("getting application list")
	stdout, _, err := kustomizeBuild("../argocd-config/overlays/" + overlayName)
	Expect(err).ShouldNot(HaveOccurred())

	var appList []string
	y := k8sYaml.NewYAMLReader(bufio.NewReader(bytes.NewReader(stdout)))
	for {
		data, err := y.Read()
		if err == io.EOF {
			break
		}
		Expect(err).ShouldNot(HaveOccurred())

		app := &unstructured.Unstructured{}
		_, _, err = decUnstructured.Decode(data, nil, app)
		Expect(err).ShouldNot(HaveOccurred())

		// Skip if the app is for tenants
		if app.GetLabels()["is-tenant"] == "true" {
			continue
		}
		appList = append(appList, app.GetName())
	}
	Expect(appList).ShouldNot(HaveLen(0))

	sort.Strings(appList)
	fmt.Printf("application list:\n")
	for _, app := range appList {
		fmt.Println("  " + app)
	}

	By("waiting initialization")
	checkAllAppsSynced := func() error {
		for _, target := range appList {
			appStdout, stderr, err := ExecAt(boot0, "argocd", "app", "get", "-o", "json", target)
			if err != nil {
				return fmt.Errorf("stdout: %s, stderr: %s, err: %v", appStdout, stderr, err)
			}
			var app Application
			err = json.Unmarshal(appStdout, &app)
			if err != nil {
				return fmt.Errorf("stdout: %s, err: %v", appStdout, err)
			}
			switch app.Name {
			case "prometheus-adapter":
			// These reference upstream Helm chart versions, so no need to check commitID.
			default:
				if app.Status.Sync.ComparedTo.Source.TargetRevision != commitID {
					return errors.New(target + " does not have correct target yet")
				}
			}
			if app.Status.Sync.Status == SyncStatusCodeSynced &&
				app.Status.Health.Status == HealthStatusHealthy &&
				app.Operation == nil {
				continue
			}

			// In upgrade test, syncing network-policy app may cause temporal network disruption.
			// It leads to ArgoCD's improper behavior. In spite of the network-policy app becomes Synced/Healthy, the operation does not end.
			// So terminate the unexpected operation manually in upgrade test.
			// TODO: This is workaround for ArgoCD's improper behavior. When this issue (T.B.D.) is closed, delete this block.
			if app.Status.Sync.Status == SyncStatusCodeSynced &&
				app.Status.Health.Status == HealthStatusHealthy &&
				app.Operation != nil &&
				app.Status.OperationState != nil &&
				app.Status.OperationState.Phase == "Running" {
				fmt.Printf("%s terminate unexpected operation: app=%s\n", time.Now().Format(time.RFC3339), target)
				stdout, stderr, err := ExecAt(boot0, "argocd", "app", "terminate-op", target)
				if err != nil {
					return fmt.Errorf("failed to terminate operation. app: %s, stdout: %s, stderr: %s, err: %v", target, stdout, stderr, err)
				}
				stdout, stderr, err = ExecAt(boot0, "argocd", "app", "sync", target)
				if err != nil {
					return fmt.Errorf("failed to sync application. app: %s, stdout: %s, stderr: %s, err: %v", target, stdout, stderr, err)
				}
			}

			return fmt.Errorf("%s is not initialized. argocd app get %s -o json: %s", target, target, appStdout)
		}
		return nil
	}

	// want to do "Eventually( Consistently(checkAllAppsSynced, 15sec, 1sec) )"
	Eventually(func() error {
		st := time.Now()
		for {
			if time.Since(st) > 15*time.Second {
				return nil
			}
			err := checkAllAppsSynced()
			if err != nil {
				return err
			}
			time.Sleep(1 * time.Second)
		}
	}, 60*time.Minute).Should(Succeed())
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

	var nonCRDResources []*unstructured.Unstructured
	y := k8sYaml.NewYAMLReader(bufio.NewReader(bytes.NewReader(netpolManifest)))
	for {
		data, err := y.Read()
		if err == io.EOF {
			break
		}
		Expect(err).ShouldNot(HaveOccurred())

		resources := &unstructured.Unstructured{}
		_, gvk, err := decUnstructured.Decode(data, nil, resources)
		if err != nil {
			continue
		}
		if gvk.Kind != "CustomResourceDefinition" {
			nonCRDResources = append(nonCRDResources, resources)
			continue
		}

		stdout, stderr, err = ExecAtWithInput(boot0, data, "kubectl", "apply", "-f", "-")
		Expect(err).ShouldNot(HaveOccurred(), "failed to apply crd: stdout=%s, stderr=%s", stdout, stderr)
	}

	for _, r := range nonCRDResources {
		labels := r.GetLabels()
		if labels == nil {
			labels = make(map[string]string)
		}
		// ArgoCD will add this label, so adding this label here beforehand to speed up CI
		labels["app.kubernetes.io/instance"] = "network-policy"
		r.SetLabels(labels)
		data, err := r.MarshalJSON()
		Expect(err).ShouldNot(HaveOccurred(), "failed to marshal json. err=%s", err)
		stdout, stderr, err = ExecAtWithInput(boot0, data, "kubectl", "apply", "-f", "-")
		Expect(err).ShouldNot(HaveOccurred(), "failed to apply non-crd resource: stdout=%s, stderr=%s", stdout, stderr)
	}

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

// This step is purely optional.  This is only for making Argo CD synchronization faster.
func applyCertManager() {
	// Egress in internet-egress is a dependency of cert-manager
	By("apply coil")
	ExecSafeAt(boot0, "kustomize build neco-apps/coil/base | kubectl apply -f -")

	By("apply cert-manager")
	manifest, stderr, err := kustomizeBuild("../cert-manager/overlays/" + overlayName)
	Expect(err).ShouldNot(HaveOccurred(), "failed to kustomize build: stderr=%s", stderr)

	var nonCRDResources []*unstructured.Unstructured
	y := k8sYaml.NewYAMLReader(bufio.NewReader(bytes.NewReader(manifest)))
	for {
		data, err := y.Read()
		if err == io.EOF {
			break
		}
		Expect(err).ShouldNot(HaveOccurred())

		resources := &unstructured.Unstructured{}
		_, gvk, err := decUnstructured.Decode(data, nil, resources)
		if err != nil {
			continue
		}
		if gvk.Kind == "ClusterIssuer" {
			continue
		}
		if gvk.Kind != "CustomResourceDefinition" {
			nonCRDResources = append(nonCRDResources, resources)
			continue
		}

		stdout, stderr, err := ExecAtWithInput(boot0, data, "kubectl", "apply", "-f", "-")
		Expect(err).ShouldNot(HaveOccurred(), "failed to apply crd: stdout=%s, stderr=%s", stdout, stderr)
	}

	for _, r := range nonCRDResources {
		labels := r.GetLabels()
		if labels == nil {
			labels = make(map[string]string)
		}
		// ArgoCD will add this label, so adding this label here beforehand to speed up CI
		labels["app.kubernetes.io/instance"] = "cert-manager"
		r.SetLabels(labels)
		data, err := r.MarshalJSON()
		Expect(err).ShouldNot(HaveOccurred(), "failed to marshal json. err=%s", err)
		stdout, stderr, err := ExecAtWithInput(boot0, data, "kubectl", "apply", "-f", "-")
		Expect(err).ShouldNot(HaveOccurred(), "failed to apply non-crd resource: stdout=%s, stderr=%s", stdout, stderr)
	}

	Eventually(func() error {
		for _, name := range []string{"cert-manager", "cert-manager-cainjector", "cert-manager-webhook"} {
			stdout, stderr, err := ExecAt(boot0, "kubectl", "-n", "cert-manager", "get", "deployments", name, "-o=json")
			if err != nil {
				return fmt.Errorf("%s, err: %w", stderr, err)
			}
			deployment := new(appsv1.Deployment)
			err = json.Unmarshal(stdout, deployment)
			if err != nil {
				return err
			}
			if deployment.Status.ReadyReplicas != 1 {
				return fmt.Errorf("cert-manager/%s is not ready: %d", name, deployment.Status.ReadyReplicas)
			}
		}
		return nil
	}, 3*time.Minute).Should(Succeed())
}

func applyWebhooksFrom(manifests []byte) {
	var webhooks []*unstructured.Unstructured
	y := k8sYaml.NewYAMLReader(bufio.NewReader(bytes.NewReader(manifests)))
	for {
		data, err := y.Read()
		if err == io.EOF {
			break
		}
		ExpectWithOffset(1, err).ShouldNot(HaveOccurred())

		res := &unstructured.Unstructured{}
		_, gvk, err := decUnstructured.Decode(data, nil, res)
		ExpectWithOffset(1, err).ShouldNot(HaveOccurred())

		if gvk.Kind == "MutatingWebhookConfiguration" {
			webhooks = append(webhooks, res)
		}
	}

	for _, r := range webhooks {
		data, err := r.MarshalJSON()
		ExpectWithOffset(1, err).ShouldNot(HaveOccurred())

		stdout, stderr, err := ExecAtWithInput(boot0, data, "kubectl", "apply", "-f", "-")
		ExpectWithOffset(1, err).ShouldNot(HaveOccurred(), "failed to apply webhook %s: stdout=%s, stderr=%s", r.GetName(), stdout, stderr)
	}
}

// Since Argo CD 1.8, it does not check and wait for Applications to become ready.
// This is normally okay except for some critical mutating webhooks.
// To workaround this bootstrap problem, we need to apply webhooks manually first.
func applyMutatingWebhooks() {
	By("applying neco-admission webhooks")
	manifests, stderr, err := kustomizeBuild("../neco-admission/base/")
	Expect(err).ShouldNot(HaveOccurred(), "failed to kustomize build: stderr=%s", stderr)
	applyWebhooksFrom(manifests)

	By("applying TopoLVM webhooks")
	manifests, stderr, err = kustomizeBuild("../topolvm/base/")
	Expect(err).ShouldNot(HaveOccurred(), "failed to kustomize build: stderr=%s", stderr)
	applyWebhooksFrom(manifests)
}

func setupArgoCD() {
	By("installing Argo CD")
	createNamespaceIfNotExists("argocd", true)
	data, err := os.ReadFile("install.yaml")
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
