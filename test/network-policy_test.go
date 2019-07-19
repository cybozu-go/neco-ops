package test

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/cybozu-go/sabakan/v2"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"golang.org/x/crypto/ssh"
	corev1 "k8s.io/api/core/v1"
)

func testNetworkPolicy() {
	It("should create test-netpol namespace", func() {
		ExecSafeAt(boot0, "kubectl", "delete", "namespace", "test-netpol", "--ignore-not-found=true")
		ExecSafeAt(boot0, "kubectl", "create", "namespace", "test-netpol")
	})

	It("should create test pods with network policies", func() {
		By("deploying testhttpd pods")
		deployYAML := `
apiVersion: extensions/v1beta1
kind: Deployment
metadata:
  name: testhttpd
  namespace: test-netpol
spec:
  replicas: 2
  selector:
    matchLabels:
      app.kubernetes.io/name: testhttpd
  template:
    metadata:
      labels:
        app.kubernetes.io/name: testhttpd
    spec:
      containers:
      - image: quay.io/cybozu/testhttpd:0
        name: testhttpd
      restartPolicy: Always
---
apiVersion: v1
kind: Service
metadata:
  name: testhttpd
  namespace: test-netpol
spec:
  ports:
  - port: 80
    protocol: TCP
    targetPort: 8000
  selector:
    app.kubernetes.io/name: testhttpd
---
apiVersion: crd.projectcalico.org/v1
kind: NetworkPolicy
metadata:
  name: ingress-httpdtest
  namespace: test-netpol
spec:
  order: 2000.0
  selector: app.kubernetes.io/name== 'testhttpd'
  types:
    - Ingress
  ingress:
    - action: Allow
      protocol: TCP
      destination:
        ports:
          - 8000
`
		_, stderr, err := ExecAtWithInput(boot0, []byte(deployYAML), "kubectl", "apply", "-f", "-")
		Expect(err).NotTo(HaveOccurred(), "stderr: %s", stderr)

		// connections to 8080 and 8443 of contour are rejected unless we register IngressRoute
		By("creating IngressRoute")
		fqdnHTTP := "http.test-netpol.gcp0.dev-ne.co"
		fqdnHTTPS := "https.test-netpol.gcp0.dev-ne.co"
		ingressRoute := fmt.Sprintf(`
apiVersion: contour.heptio.com/v1beta1
kind: IngressRoute
metadata:
  name: tls
  namespace: test-netpol
  annotations:
    kubernetes.io/tls-acme: "true"
spec:
  virtualhost:
    fqdn: %s
    tls:
      secretName: testsecret
  routes:
    - match: /
      services:
        - name: testhttpd
          port: 80
    - match: /insecure
      permitInsecure: true
      services:
        - name: testhttpd
          port: 80
---
apiVersion: contour.heptio.com/v1beta1
kind: IngressRoute
metadata:
  name: root
  namespace: test-netpol
spec:
  virtualhost:
    fqdn: %s
  routes:
    - match: /testhttpd
      services:
        - name: testhttpd
          port: 80
`, fqdnHTTPS, fqdnHTTP)
		_, stderr, err = ExecAtWithInput(boot0, []byte(ingressRoute), "kubectl", "apply", "-f", "-")
		Expect(err).NotTo(HaveOccurred(), "stderr: %s", stderr)

		By("deploying ubuntu for network commands")
		createUbuntuDebugPod("default")
	})

	podList := new(corev1.PodList)
	testhttpdPodList := new(corev1.PodList)
	nodeList := new(corev1.NodeList)
	var nodeIP string
	var apiServerIP string

	It("should get pod/node list", func() {
		By("getting all pod list")
		stdout, stderr, err := ExecAt(boot0, "kubectl", "get", "pods", "-A", "-o=json")
		Expect(err).NotTo(HaveOccurred(), "stdout: %s, stderr: %s", stdout, stderr)
		err = json.Unmarshal(stdout, podList)
		Expect(err).NotTo(HaveOccurred())

		By("getting httpd pod list")
		stdout, stderr, err = ExecAt(boot0, "kubectl", "get", "pods", "-n", "test-netpol", "-o=json")
		Expect(err).NotTo(HaveOccurred(), "stdout: %s, stderr: %s", stdout, stderr)
		err = json.Unmarshal(stdout, testhttpdPodList)
		Expect(err).NotTo(HaveOccurred())

		By("getting all node list")
		stdout, stderr, err = ExecAt(boot0, "kubectl", "get", "node", "-o=json")
		Expect(err).NotTo(HaveOccurred(), "stdout: %s, stderr: %s", stdout, stderr)
		err = json.Unmarshal(stdout, nodeList)
		Expect(err).NotTo(HaveOccurred())

		By("getting a certain node IP address")
	OUTER:
		for _, node := range nodeList.Items {
			for _, addr := range node.Status.Addresses {
				if addr.Type == "InternalIP" {
					nodeIP = addr.Address
					break OUTER
				}
			}
		}
		Expect(nodeIP).NotTo(BeEmpty())

		stdout, stderr, err = ExecAt(boot0, "kubectl", "config", "view", "--output=jsonpath={.clusters[0].cluster.server}")
		Expect(err).NotTo(HaveOccurred(), "stdout: %s, stderr: %s", stdout, stderr)
		u, err := url.Parse(string(stdout))
		Expect(err).NotTo(HaveOccurred(), "server: %s", stdout)
		apiServerIP = strings.Split(u.Host, ":")[0]
		Expect(apiServerIP).NotTo(BeEmpty(), "server: %s", stdout)
	})

	It("should resolve hostname with DNS", func() {
		By("resolving hostname inside of cluster (by cluster-dns)")
		Eventually(func() error {
			stdout, stderr, err := ExecAt(boot0, "kubectl", "exec", "ubuntu", "--", "nslookup", "-timeout=10", "testhttpd.test-netpol")
			if err != nil {
				return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
			}
			return nil
		}).Should(Succeed())

		By("resolving hostname outside of cluster (by unbound)")
		Eventually(func() error {
			stdout, stderr, err := ExecAt(boot0, "kubectl", "exec", "ubuntu", "--", "nslookup", "-timeout=10", "cybozu.com")
			if err != nil {
				return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
			}
			return nil
		}).Should(Succeed())
	})

	It("should filter icmp packets to pods", func() {
		for _, pod := range podList.Items {
			if pod.Spec.HostNetwork {
				continue
			}
			By("ping to " + pod.GetName())
			stdout, stderr, err := ExecAt(boot0, "ping", "-c", "1", "-W", "3", pod.Status.PodIP)
			Expect(err).To(HaveOccurred(), "stdout: %s, stderr: %s", stdout, stderr)
		}
	})

	It("should accept and deny TCP packets according to the registered network policies", func() {
		const portShouldBeDenied = 65535

		testcase := []struct {
			namespace string
			selector  string
			ports     []int
		}{
			{"argocd", "app.kubernetes.io/name=argocd-application-controller", []int{8082}},
			{"argocd", "app.kubernetes.io/name=argocd-redis", []int{6379}},
			{"argocd", "app.kubernetes.io/name=argocd-repo-server", []int{8081, 8084}},
			{"argocd", "app.kubernetes.io/name=argocd-server", []int{8080, 8083}},
			{"external-dns", "app.kubernetes.io/name=external-dns", []int{7979}},
			{"external-dns", "app.kubernetes.io/name=cert-manager", []int{9402}},
			{"external-dns", "app.kubernetes.io/name=webhook", []int{6443}},
			{"ingress", "app.kubernetes.io/name=contour", []int{8002, 8080, 8443}},
			{"internet-egress", "app.kubernetes.io/name=squid", []int{3128}},
			{"internet-egress", "app.kubernetes.io/name=unbound", []int{53}},
			{"kube-system", "cke.cybozu.com/appname=cluster-dns", []int{1053, 8080}},
			{"kube-system", "app.kubernetes.io/name=kube-state-metrics", []int{8080, 8081}},
			{"metallb-system", "app.kubernetes.io/component=controller", []int{7472}},
			{"monitoring", "app.kubernetes.io/name=alertmanager", []int{9093}},
			{"monitoring", "app.kubernetes.io/name=prometheus", []int{9090}},
			{"opa", "app.kubernetes.io/name=opa", []int{8443}},
		}

		for _, tc := range testcase {
			By("getting target pod list: ns=" + tc.namespace + ", selector=" + tc.selector)
			stdout, stderr, err := ExecAt(boot0, "kubectl", "-n", tc.namespace, "-l", tc.selector, "get", "pods", "-o=json")
			Expect(err).NotTo(HaveOccurred(), "stdout: %s, stderr: %s", stdout, stderr)

			podList := new(corev1.PodList)
			err = json.Unmarshal(stdout, podList)
			Expect(err).NotTo(HaveOccurred())

			for _, pod := range podList.Items {
				By("connecting to pod: " + pod.GetName())
				for _, port := range tc.ports {
					By("  -> port: " + strconv.Itoa(port) + " (allowed)")
					stdout, stderr, err = ExecAtWithInput(boot0, []byte("Xclose"), "timeout", "3s", "telnet", pod.Status.PodIP, strconv.Itoa(port), "-e", "X")
					Expect(err).NotTo(HaveOccurred(), "stdout: %s, stderr: %s", stdout, stderr)
				}

				By("  -> port: " + strconv.Itoa(portShouldBeDenied) + " (denied)")
				stdout, stderr, err = ExecAtWithInput(boot0, []byte("Xclose"), "timeout", "3s", "telnet", pod.Status.PodIP, strconv.Itoa(portShouldBeDenied), "-e", "X")
				switch t := err.(type) {
				case *ssh.ExitError:
					// telnet command returns 124 when it times out
					Expect(t.ExitStatus()).To(Equal(124), "stdout: %s, stderr: %s", stdout, stderr)
				default:
					Fail("telnet should fail with timeout")
				}

				if tc.namespace == "internet-egress" {
					By("accessing to local IP")
					testhttpdIP := testhttpdPodList.Items[0].Status.PodIP
					stdout, stderr, err = ExecAt(boot0, "kubectl", "exec", "-n", pod.Namespace, pod.Name, "--", "curl", testhttpdIP, "-m", "5")
					Expect(err).To(HaveOccurred(), "stdout: %s, stderr: %s", stdout, stderr)
				}
			}
		}
	})

	It("should filter packets from squid/unbound to private network", func() {
		By("deploying ubuntu for network commands in internet-egress NS")
		createUbuntuDebugPod("internet-egress")

		By("labelling pod as squid")
		_, stderr, err := ExecAt(boot0, "kubectl", "-n", "internet-egress", "label", "pod", "ubuntu", "app.kubernetes.io/name=squid")
		Expect(err).NotTo(HaveOccurred(), "stderr: %s", stderr)

		By("accessing DNS port of some node as squid")
		_, _, err = ExecAtWithInput(boot0, []byte("Xclose"), "kubectl", "-n", "internet-egress", "exec", "-i", "ubuntu", "--", "timeout", "3s", "telnet", nodeIP, "53", "-e", "X")
		switch t := err.(type) {
		case *ssh.ExitError:
			// telnet command returns 124 when it times out
			Expect(t.ExitStatus()).To(Equal(124))
		default:
			Fail("telnet should fail with timeout")
		}

		By("removing label")
		_, stderr, err = ExecAt(boot0, "kubectl", "-n", "internet-egress", "label", "pod", "ubuntu", "app.kubernetes.io/name-")
		Expect(err).NotTo(HaveOccurred(), "stderr: %s", stderr)

		By("labelling pod as unbound")
		_, stderr, err = ExecAt(boot0, "kubectl", "-n", "internet-egress", "label", "--overwrite", "pod", "ubuntu", "app.kubernetes.io/name=unbound")
		Expect(err).NotTo(HaveOccurred(), "stderr: %s", stderr)

		By("accessing DNS port of some node as unbound")
		_, _, err = ExecAtWithInput(boot0, []byte("Xclose"), "kubectl", "-n", "internet-egress", "exec", "-i", "ubuntu", "--", "timeout", "3s", "telnet", nodeIP, "53", "-e", "X")
		switch t := err.(type) {
		case *ssh.ExitError:
			// telnet command returns 124 when it times out
			Expect(t.ExitStatus()).To(Equal(124))
		default:
			Fail("telnet should fail with timeout")
		}

		By("removing label")
		_, stderr, err = ExecAt(boot0, "kubectl", "-n", "internet-egress", "label", "pod", "ubuntu", "app.kubernetes.io/name-")
		Expect(err).NotTo(HaveOccurred(), "stderr: %s", stderr)
	})

	It("should pass packets to node network for system services", func() {
		By("accessing DNS port of some node")
		stdout, stderr, err := ExecAtWithInput(boot0, []byte("Xclose"), "kubectl", "exec", "-i", "ubuntu", "--", "timeout", "3s", "telnet", nodeIP, "53", "-e", "X")
		Expect(err).NotTo(HaveOccurred(), "stdout: %s, stderr: %s", stdout, stderr)

		By("accessing API server port of control plane node")
		stdout, stderr, err = ExecAtWithInput(boot0, []byte("Xclose"), "kubectl", "exec", "-i", "ubuntu", "--", "timeout", "3s", "telnet", apiServerIP, "6443", "-e", "X")
		Expect(err).NotTo(HaveOccurred(), "stdout: %s, stderr: %s", stdout, stderr)

		By("deploying ubuntu for network commands in monitoring NS")
		createUbuntuDebugPod("monitoring")

		By("labelling pod as prometheus")
		_, stderr, err = ExecAt(boot0, "kubectl", "-n", "monitoring", "label", "pod", "ubuntu", "app.kubernetes.io/name=prometheus")
		Expect(err).NotTo(HaveOccurred(), "stderr: %s", stderr)

		By("accessing node exporter port of some node as prometheus")
		stdout, stderr, err = ExecAtWithInput(boot0, []byte("Xclose"), "kubectl", "-n", "monitoring", "exec", "-i", "ubuntu", "--", "timeout", "3s", "telnet", nodeIP, "9100", "-e", "X")
		Expect(err).NotTo(HaveOccurred(), "stdout: %s, stderr: %s", stdout, stderr)

		By("removing label")
		_, stderr, err = ExecAt(boot0, "kubectl", "-n", "monitoring", "label", "pod", "ubuntu", "app.kubernetes.io/name-")
		Expect(err).NotTo(HaveOccurred(), "stderr: %s", stderr)
	})

	It("should filter icmp packets to BMC/Node/Bastion/switch networks", func() {
		stdout, stderr, err := ExecAt(boot0, "sabactl", "machines", "get")
		Expect(err).NotTo(HaveOccurred(), "stdout: %s, stderr: %s", stdout, stderr)

		var machines []sabakan.Machine
		err = json.Unmarshal(stdout, &machines)
		Expect(err).ShouldNot(HaveOccurred())
		for _, m := range machines {
			// BMC
			By("ping to " + m.Spec.BMC.IPv4)
			stdout, stderr, err := ExecAt(boot0, "kubectl", "exec", "ubuntu", "--", "ping", "-c", "1", "-W", "3", m.Spec.BMC.IPv4)
			Expect(err).To(HaveOccurred(), "stdout: %s, stderr: %s", stdout, stderr)

			// Node
			By("ping to " + m.Spec.IPv4[0])
			stdout, stderr, err = ExecAt(boot0, "kubectl", "exec", "ubuntu", "--", "ping", "-c", "1", "-W", "3", m.Spec.IPv4[0])
			Expect(err).To(HaveOccurred(), "stdout: %s, stderr: %s", stdout, stderr)
		}

		// Bastion
		By("ping to " + boot0)
		stdout, stderr, err = ExecAt(boot0, "kubectl", "exec", "ubuntu", "--", "ping", "-c", "1", "-W", "3", boot0)
		Expect(err).To(HaveOccurred(), "stdout: %s, stderr: %s", stdout, stderr)

		// switch -- not tested for now because address range for switches is 10.0.1.0/24 in placemat env, not 10.72.0.0/20.
	})

	It("should deny network policy in non-system namespace with order <= 1000", func() {
		By("creating invalid network policy")
		policyYAML := `
apiVersion: crd.projectcalico.org/v1
kind: NetworkPolicy
metadata:
  name: ingress-httpdtest-high-prio
  namespace: test-netpol
spec:
  order: 1000.0
  selector: app.kubernetes.io/name == 'testhttpd'
  types:
    - Ingress
  ingress:
    - action: Allow
      protocol: TCP
      destination:
        ports:
          - 8000
`
		_, stderr, err := ExecAtWithInput(boot0, []byte(policyYAML), "kubectl", "apply", "-f", "-")
		Expect(err).To(HaveOccurred())
		Expect(stderr).To(ContainSubstring("validating-webhook.openpolicyagent.org"))
	})
}

func createUbuntuDebugPod(namespace string) {
	debugYAML := `
apiVersion: v1
kind: Pod
metadata:
  name: ubuntu
spec:
  securityContext:
    runAsUser: 10000
    runAsGroup: 10000
  containers:
  - name: ubuntu
    image: quay.io/cybozu/ubuntu-debug:18.04
    command: ["sleep", "infinity"]`
	_, stderr, err := ExecAtWithInput(boot0, []byte(debugYAML), "kubectl", "apply", "-n", namespace, "-f", "-")
	Expect(err).NotTo(HaveOccurred(), "stderr: %s", stderr)

	By("waiting for ubuntu pod to start")
	Eventually(func() error {
		stdout, stderr, err := ExecAt(boot0, "kubectl", "-n", namespace, "exec", "ubuntu", "--", "date")
		if err != nil {
			return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
		}
		return nil
	}).Should(Succeed())
}
