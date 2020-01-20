package test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"strconv"
	"strings"
	"text/template"

	argocd "github.com/argoproj/argo-cd/pkg/apis/application/v1alpha1"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	k8sYaml "k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/yaml"
)

const (
	argoCDPasswordFile = "./argocd-password.txt"

	grafanaSecret = `apiVersion: v1
kind: Secret
metadata:
  labels:
    app.kubernetes.io/name: grafana
  name: grafana
  namespace: monitoring
type: Opaque
data:
  admin-password: QVVKVWwxSzJ4Z2Vxd01kWjNYbEVGYzFRaGdFUUl0T0RNTnpKd1FtZQ==
  admin-user: YWRtaW4=
  ldap-toml: ""
`

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
    teleport:
      data_dir: /var/lib/teleport
      auth_token: {{ .Token }}
      log:
        output: stderr
        severity: DEBUG
      storage:
        type: etcd
        peers: ["https://cke-etcd.kube-system.svc:2379"]
        tls_cert_file: /var/lib/etcd-certs/tls.crt
        tls_key_file: /var/lib/etcd-certs/tls.key
        tls_ca_file: /var/lib/etcd-certs/ca.crt
        prefix: /teleport
        insecure: false
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

// testSetup tests setup of Argo CD
func testSetup() {
	if !withKind {
		It("should disable CKE-sabakan integration feature", func() {
			ExecSafeAt(boot0, "ckecli", "sabakan", "disable")
		})
	}

	if !doUpgrade {
		It("should create secrets of account.json", func() {
			By("loading account.json")
			var data []byte
			var err error
			if withKind {
				data = []byte("{}")
			} else {
				data, err = ioutil.ReadFile("account.json")
				Expect(err).ShouldNot(HaveOccurred())
			}

			By("creating namespace and secrets for external-dns")
			_, _, err = ExecAt(boot0, "kubectl", "get", "namespace", "external-dns")
			if err != nil {
				ExecSafeAt(boot0, "kubectl", "create", "namespace", "external-dns")
			}
			_, _, err = ExecAt(boot0, "kubectl", "--namespace=external-dns", "get", "secret", "clouddns")
			if err != nil {
				_, stderr, err := ExecAtWithInput(boot0, data, "kubectl", "--namespace=external-dns",
					"create", "secret", "generic", "clouddns", "--from-file=account.json=/dev/stdin")
				Expect(err).ShouldNot(HaveOccurred(), "stderr=%s", stderr)
			}

			By("creating namespace and secrets for cert-manager")
			_, _, err = ExecAt(boot0, "kubectl", "get", "namespace", "cert-manager")
			if err != nil {
				ExecSafeAt(boot0, "kubectl", "create", "namespace", "cert-manager")
			}
			_, _, err = ExecAt(boot0, "kubectl", "--namespace=cert-manager", "get", "secret", "clouddns")
			if err != nil {
				_, stderr, err := ExecAtWithInput(boot0, data, "kubectl", "--namespace=cert-manager",
					"create", "secret", "generic", "clouddns", "--from-file=account.json=/dev/stdin")
				Expect(err).ShouldNot(HaveOccurred(), "stderr=%s", stderr)
			}
		})

		It("should prepare secrets", func() {
			By("creating namespace and secrets for grafana")
			_, _, err := ExecAt(boot0, "kubectl", "get", "namespace", "monitoring")
			if err != nil {
				ExecSafeAt(boot0, "kubectl", "create", "namespace", "monitoring")
			}
			stdout, stderr, err := ExecAtWithInput(boot0, []byte(grafanaSecret), "dd", "of=grafana.yaml")
			Expect(err).NotTo(HaveOccurred(), "stdout: %s, stderr: %s", stdout, stderr)
			ExecSafeAt(boot0, "kubectl", "apply", "-f", "grafana.yaml")

			if !withKind {
				By("creating namespace and secrets for teleport")
				stdout, stderr, err = ExecAt(boot0, "env", "ETCDCTL_API=3", "etcdctl", "--cert=/etc/etcd/backup.crt", "--key=/etc/etcd/backup.key",
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
				_, _, err := ExecAt(boot0, "kubectl", "get", "namespace", "teleport")
				if err != nil {
					ExecSafeAt(boot0, "kubectl", "create", "namespace", "teleport")
				}
				stdout, stderr, err = ExecAtWithInput(boot0, buf.Bytes(), "kubectl", "apply", "-n", "teleport", "-f", "-")
				Expect(err).NotTo(HaveOccurred(), "stdout: %s, stderr: %s", stdout, stderr)

				controlPlaceAddress := ExecSafeAt(boot0, "kubectl", "get", "node", "-l=cke.cybozu.com/master=true", `-o=jsonpath="{.items[0].metadata.name}"`)
				ExecSafeAt(boot0, "ckecli", "etcd", "root-issue", "--output=file")
				_, _, err = ExecAt(boot0, "env", "ETCDCTL_API=3", "etcdctl", "user", "get", "teleport",
					fmt.Sprintf("--endpoints=https://%s:2379", controlPlaceAddress),
					"--key=etcd-root.key", "--cert=etcd-root.crt", "--cacert=etcd-ca.crt")
				if err != nil {
					ExecSafeAt(boot0, "ckecli", "etcd", "user-add", "teleport", "/teleport")
				}

				_, _, err = ExecAt(boot0, "kubectl", "get", "secret", "teleport-etcd-certs", "-n=teleport")
				if err != nil {
					ExecSafeAt(boot0, "ckecli", "etcd", "issue", "teleport", "--output", "file")
					ExecSafeAt(boot0, "kubectl", "-n", "teleport", "create", "secret", "generic",
						"teleport-etcd-certs", "--from-file=ca.crt=etcd-ca.crt",
						"--from-file=tls.crt=etcd-teleport.crt", "--from-file=tls.key=etcd-teleport.key")
				}
			}
		})
	}

	It("should checkout neco-apps repository@"+commitID, func() {
		ExecSafeAt(boot0, "rm", "-rf", "neco-apps")
		if !withKind {
			ExecSafeAt(boot0, "env", "https_proxy=http://10.0.49.3:3128",
				"git", "clone", "https://github.com/cybozu-go/neco-apps")
		} else {
			ExecSafeAt(boot0, "git", "clone", "https://github.com/cybozu-go/neco-apps")
		}
		ExecSafeAt(boot0, "cd neco-apps; git checkout "+commitID)
	})

	It("should setup applications", func() {
		if !doUpgrade {
			setupArgoCD()
		}
		ExecSafeAt(boot0, "sed", "-i", "s/release/"+commitID+"/", "./neco-apps/argocd-config/base/*.yaml")
		if withKind {
			applyAndWaitForApplications("kind")
		} else {
			applyAndWaitForApplications("gcp")
		}
	})

	if !withKind {
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
}

func applyAndWaitForApplications(overlay string) {
	By("creating Argo CD app")
	Eventually(func() error {
		stdout, stderr, err := ExecAt(boot0, "argocd", "app", "create", "argocd-config",
			"--repo", "https://github.com/cybozu-go/neco-apps.git",
			"--path", "argocd-config/overlays/"+overlay,
			"--dest-namespace", "argocd",
			"--dest-server", "https://kubernetes.default.svc",
			"--sync-policy", "none",
			"--revision", "release")
		if err != nil {
			return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
		}
		return nil
	}).Should(Succeed())
	ExecSafeAt(boot0, "cd", "./neco-apps", "&&", "argocd", "app", "sync", "argocd-config", "--local", "argocd-config/overlays/"+overlay)

	By("getting application list")
	stdout, _, err := kustomizeBuild("../argocd-config/overlays/" + overlay)
	Expect(err).ShouldNot(HaveOccurred())

	var appList []string
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
		appList = append(appList, app.Name)
	}
	fmt.Printf("application list: %v\n", appList)
	Expect(appList).ShouldNot(HaveLen(0))

	By("pruning resource quota manually")
	//TODO: Remove this `By` block after merged this PR: https://github.com/cybozu-go/neco-apps/pull/344
	if doUpgrade {
		Eventually(func() error {
			stdout, stderr, err := ExecAt(boot0, "argocd", "app", "sync", "team-management", "--prune")
			if err != nil {
				return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
			}
			return nil
		}).Should(Succeed())
	}

	By("waiting initialization")
	Eventually(func() error {
	OUTER:
		for _, appName := range appList {
			appStdout, stderr, err := ExecAt(boot0, "argocd", "app", "get", "-o", "json", appName)
			if err != nil {
				return fmt.Errorf("stdout: %s, stderr: %s, err: %v", appStdout, stderr, err)
			}
			var app argocd.Application
			err = json.Unmarshal(appStdout, &app)
			if err != nil {
				return fmt.Errorf("stdout: %s, err: %v", appStdout, err)
			}
			if app.Status.Sync.ComparedTo.Source.TargetRevision != commitID {
				return errors.New(appName + " does not have correct target yet")
			}
			st := app.Status
			if st.Sync.Status == argocd.SyncStatusCodeSynced &&
				st.Health.Status == argocd.HealthStatusHealthy &&
				app.Operation == nil {
				continue
			}

			// In upgrade test, sync without --force may cause temporal network disruption.
			// It leads to sync-error of other applications,
			// so sync manually sync-error apps in upgrade test.
			if doUpgrade {
				for _, cond := range st.Conditions {
					if cond.Type == argocd.ApplicationConditionSyncError {
						stdout, stderr, err := ExecAt(boot0, "argocd", "app", "sync", appName)
						if err != nil {
							return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
						}
						continue OUTER
					}
				}
			}
			return fmt.Errorf("%s is not initialized. argocd app get %s -o json: %s", appName, appName, appStdout)
		}
		return nil
	}).Should(Succeed())
}

func setupArgoCD() {
	By("installing Argo CD")
	_, _, err := ExecAt(boot0, "kubectl", "get", "namespace", "argocd")
	if err != nil {
		ExecSafeAt(boot0, "kubectl", "create", "namespace", "argocd")
	}
	data, err := ioutil.ReadFile("install.yaml")
	Expect(err).ShouldNot(HaveOccurred())
	_, stderr, err := ExecAtWithInput(boot0, data, "kubectl", "apply", "-n", "argocd", "-f", "-")
	Expect(err).ShouldNot(HaveOccurred(), "stderr=%s", stderr)

	By("waiting Argo CD comes up")
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
			return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
		}
		return nil
	}).Should(Succeed())
}
