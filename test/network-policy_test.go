package test

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os/exec"
	"strings"

	"github.com/cybozu-go/log"
	"github.com/cybozu-go/sabakan/v2"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"golang.org/x/crypto/ssh"
	"golang.org/x/sync/errgroup"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

func testNetworkPolicy() {
	It("should create test-netpol namespace", func() {
		ExecSafeAt(boot0, "kubectl", "delete", "namespace", "test-netpol", "--ignore-not-found=true")
		ExecSafeAt(boot0, "kubectl", "create", "namespace", "test-netpol")
	})

	It("should create test pods", func() {
		By("deploying testhttpd pods")
		deployYAML := `
apiVersion: apps/v1
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
`
		_, stderr, err := ExecAtWithInput(boot0, []byte(deployYAML), "kubectl", "apply", "-f", "-")
		Expect(err).NotTo(HaveOccurred(), "stderr: %s", stderr)

		By("waiting pods are ready")
		Eventually(func() error {
			stdout, _, err := ExecAt(boot0, "kubectl", "-n", "test-netpol", "get", "deployments/testhttpd", "-o", "json")
			if err != nil {
				return err
			}

			deployment := new(appsv1.Deployment)
			err = json.Unmarshal(stdout, deployment)
			if err != nil {
				return err
			}

			if deployment.Status.ReadyReplicas != 2 {
				return errors.New("ReadyReplicas is not 2")
			}
			return nil
		}).Should(Succeed())

		// connections to 8080 and 8443 of contour are rejected unless we register HTTPProxy
		By("creating HTTPProxy")
		fqdnHTTP := testID + "-http.test-netpol.gcp0.dev-ne.co"
		fqdnHTTPS := testID + "-https.test-netpol.gcp0.dev-ne.co"
		ingressRoute := fmt.Sprintf(`
apiVersion: projectcontour.io/v1
kind: HTTPProxy
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
    - conditions:
        - prefix: /
      services:
        - name: testhttpd
          port: 80
    - conditions:
        - prefix: /insecure
      permitInsecure: true
      services:
        - name: testhttpd
          port: 80
---
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: root
  namespace: test-netpol
spec:
  virtualhost:
    fqdn: %s
  routes:
    - conditions:
        - prefix: /testhttpd
      services:
        - name: testhttpd
          port: 80
`, fqdnHTTPS, fqdnHTTP)
		_, stderr, err = ExecAtWithInput(boot0, []byte(ingressRoute), "kubectl", "apply", "-f", "-")
		Expect(err).NotTo(HaveOccurred(), "stderr: %s", stderr)
		By("deploying ubuntu for network commands")
		createUbuntuDebugPod("default")
	})

	testhttpdPodList := new(corev1.PodList)
	nodeList := new(corev1.NodeList)
	var nodeIP string
	var apiServerIP string

	patchUbuntu := `-p='[{"op": "add", "path": "/spec/template/spec/containers/-", "value": { "image": "quay.io/cybozu/ubuntu-debug:18.04", "imagePullPolicy": "IfNotPresent", "name": "ubuntu", "command": ["pause"], "securityContext": { "readOnlyRootFilesystem": true, "runAsGroup": 10000, "runAsUser": 10000 }}}]'`

	It("should get pod/node list", func() {

		By("getting httpd pod list")
		stdout, stderr, err := ExecAt(boot0, "kubectl", "get", "pods", "-n", "test-netpol", "-o=json")
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

	It("should filter packets from squid/unbound to private network", func() {
		By("accessing to local IP")
		stdout, stderr, err := ExecAt(boot0, "kubectl", "-n", "internet-egress", "get", "pods", "-o=json")
		Expect(err).NotTo(HaveOccurred(), "stdout: %s, stderr: %s", stdout, stderr)
		podList := new(corev1.PodList)
		err = json.Unmarshal(stdout, podList)
		Expect(err).NotTo(HaveOccurred())
		testhttpdIP := testhttpdPodList.Items[0].Status.PodIP

		for _, pod := range podList.Items {
			stdout, stderr, err := ExecAt(boot0, "kubectl", "exec", "-n", pod.Namespace, pod.Name, "--", "curl", testhttpdIP, "-m", "5")
			Expect(err).To(HaveOccurred(), "stdout: %s, stderr: %s", stdout, stderr)
		}

		if withKind {
			Skip("does not make sense with kindtest")
		}

		By("patching squid pods to add ubuntu-debug sidecar container")
		stdout, stderr, err = ExecAt(boot0, "kubectl", "patch", "-n=internet-egress", "deploy", "squid", "--type=json", patchUbuntu)
		Expect(err).NotTo(HaveOccurred(), "stdout: %s, stderr: %s", stdout, stderr)

		By("waiting for pods to be ready")
		Eventually(func() error {
			stdout, stderr, err := ExecAt(boot0, "kubectl", "--namespace=internet-egress", "get", "deployment/squid", "-o=json")
			if err != nil {
				return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
			}

			deployment := new(appsv1.Deployment)
			err = json.Unmarshal(stdout, deployment)
			if err != nil {
				return err
			}

			if deployment.Status.ReadyReplicas != 2 {
				return fmt.Errorf("squid deployment's ReadyReplicas is not 2: %d", int(deployment.Status.ReadyReplicas))
			}
			if deployment.Status.UpdatedReplicas != 2 {
				return fmt.Errorf("squid deployment's UpdatedReplicas is not 2: %d", int(deployment.Status.UpdatedReplicas))
			}

			return nil
		}).Should(Succeed())

		By("accessing DNS port of some node as squid")
		stdout, _, err = ExecAt(boot0, "kubectl", "get", "pods", "-n=internet-egress", "-l=app.kubernetes.io/name=squid", "-o", "go-template='{{ (index .items 0).metadata.name }}'")
		Expect(err).NotTo(HaveOccurred())
		podName := string(stdout)
		Eventually(func() error {
			stdout, stderr, err := ExecAtWithInput(boot0, []byte("Xclose"), "kubectl", "-n", "internet-egress", "exec", "-i", podName, "-c", "ubuntu", "--", "timeout", "3s", "telnet", nodeIP, "53", "-e", "X")
			switch t := err.(type) {
			case *ssh.ExitError:
				// telnet command returns 124 when it times out
				if t.ExitStatus() != 124 {
					return fmt.Errorf("exit status should be 124: %d, stdout: %s, stderr: %s, err: %v", t.ExitStatus(), stdout, stderr, err)
				}
			case *exec.ExitError:
				if t.ExitCode() != 124 {
					return fmt.Errorf("exit status should be 124: %d, stdout: %s, stderr: %s, err: %v", t.ExitCode(), stdout, stderr, err)
				}
			default:
				return fmt.Errorf("telnet should fail with timeout; stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
			}
			return nil
		}).Should(Succeed())

		By("patching unbound pods to add ubuntu-debug sidecar container")
		stdout, stderr, err = ExecAt(boot0,
			"kubectl", "patch", "-n=internet-egress", "deploy", "unbound", "--type=json", patchUbuntu)
		Expect(err).NotTo(HaveOccurred(), "stdout: %s, stderr: %s", stdout, stderr)

		By("waiting for pods to be ready")
		Eventually(func() error {
			stdout, stderr, err := ExecAt(boot0, "kubectl", "get", "pods", "-n=internet-egress", "-l=app.kubernetes.io/name=unbound", "-o", "go-template='{{ (index .items 0).metadata.name }}'")
			if err != nil {
				return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
			}
			podName = string(stdout)

			stdout, stderr, err = ExecAt(boot0, "kubectl", "exec", "-n=internet-egress", podName, "-c", "ubuntu", "true")
			if err != nil {
				return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
			}
			return nil
		}).Should(Succeed())

		By("accessing DNS port of some node as unbound")
		Eventually(func() error {
			stdout, stderr, err := ExecAtWithInput(boot0, []byte("Xclose"), "kubectl", "-n", "internet-egress", "exec", "-i", podName, "-c", "ubuntu", "--", "timeout", "3s", "telnet", nodeIP, "53", "-e", "X")
			switch t := err.(type) {
			case *ssh.ExitError:
				// telnet command returns 124 when it times out
				if t.ExitStatus() != 124 {
					return fmt.Errorf("exit status should be 124: %d, stdout: %s, stderr: %s, err: %v", t.ExitStatus(), stdout, stderr, err)
				}
			case *exec.ExitError:
				if t.ExitCode() != 124 {
					return fmt.Errorf("exit status should be 124: %d, stdout: %s, stderr: %s, err: %v", t.ExitCode(), stdout, stderr, err)
				}
			default:
				return fmt.Errorf("telnet should fail with timeout; stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
			}
			return nil
		}).Should(Succeed())
	})

	It("should pass packets to node network for system services", func() {
		if withKind {
			Skip("does not make sense with kindtest")
		}

		By("accessing DNS port of some node")
		stdout, stderr, err := ExecAtWithInput(boot0, []byte("Xclose"), "kubectl", "exec", "-i", "ubuntu", "--", "timeout", "3s", "telnet", nodeIP, "53", "-e", "X")
		Expect(err).NotTo(HaveOccurred(), "stdout: %s, stderr: %s", stdout, stderr)

		By("accessing API server port of control plane node")
		stdout, stderr, err = ExecAtWithInput(boot0, []byte("Xclose"), "kubectl", "exec", "-i", "ubuntu", "--", "timeout", "3s", "telnet", apiServerIP, "6443", "-e", "X")
		Expect(err).NotTo(HaveOccurred(), "stdout: %s, stderr: %s", stdout, stderr)

		By("patching prometheus pods to add ubuntu-debug sidecar container")
		stdout, stderr, err = ExecAt(boot0, "kubectl", "patch", "-n=monitoring", "statefulset", "prometheus", "--type=json", patchUbuntu)
		Expect(err).NotTo(HaveOccurred(), "stdout: %s, stderr: %s", stdout, stderr)

		var podName string
		By("waiting for pods to be ready")
		Eventually(func() error {
			stdout, stderr, err := ExecAt(boot0, "kubectl", "get", "pods", "-n=monitoring", "-l=app.kubernetes.io/name=prometheus", "-o", "go-template='{{ (index .items 0).metadata.name }}'")
			if err != nil {
				return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
			}
			podName = string(stdout)

			stdout, stderr, err = ExecAt(boot0, "kubectl", "exec", "-n=monitoring", podName, "-c", "ubuntu", "true")
			if err != nil {
				return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
			}
			return nil
		}).Should(Succeed())

		By("accessing node-expoter port of some node as prometheus")
		Eventually(func() error {
			stdout, stderr, err := ExecAtWithInput(boot0, []byte("Xclose"), "kubectl", "-n", "monitoring", "exec", "-i", podName, "-c", "ubuntu", "--", "timeout", "3s", "telnet", nodeIP, "9100", "-e", "X")
			if err != nil {
				return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
			}
			return nil
		}).Should(Succeed())
	})

	It("should filter icmp packets to BMC/Node/Bastion/switch networks", func() {
		if withKind {
			Skip("does not make sense with kindtest")
		}

		stdout, stderr, err := ExecAt(boot0, "sabactl", "machines", "get")
		Expect(err).NotTo(HaveOccurred(), "stdout: %s, stderr: %s", stdout, stderr)

		var machines []sabakan.Machine
		err = json.Unmarshal(stdout, &machines)
		Expect(err).ShouldNot(HaveOccurred())

		eg := errgroup.Group{}
		ping := func(addr string) error {
			_, _, err := ExecAt(boot0, "kubectl", "exec", "ubuntu", "--", "ping", "-c", "1", "-W", "3", addr)
			if err != nil {
				return err
			}
			log.Error("ping should be failed, but it was succeeded", map[string]interface{}{
				"target": addr,
			})
			return nil
		}
		for _, m := range machines {
			bmcAddr := m.Spec.BMC.IPv4
			node0Addr := m.Spec.IPv4[0]
			eg.Go(func() error {
				return ping(bmcAddr)
			})
			eg.Go(func() error {
				return ping(node0Addr)
			})
		}
		// Bastion
		eg.Go(func() error {
			return ping(boot0)
		})
		Expect(eg.Wait()).Should(HaveOccurred())
		// switch -- not tested for now because address range for switches is 10.0.1.0/24 in placemat env, not 10.72.0.0/20.
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
