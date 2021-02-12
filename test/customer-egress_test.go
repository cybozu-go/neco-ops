package test

import (
	"encoding/json"
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

func prepareCustomerEgress() {
	It("should create ubuntu pod on sandbox ns", func() {
		podYAML := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: ubuntu-without-nat-annotation
  namespace: sandbox
spec:
  replicas: 1
  selector:
    matchLabels:
      custom-egress-test: non-nat
  template:
    metadata:
      labels:
        custom-egress-test: non-nat
    spec:
      securityContext:
        runAsUser: 1000
        runAsGroup: 1000
      containers:
      - args:
        - pause
        image: quay.io/cybozu/ubuntu-debug:20.04
        name: ubuntu
`
		stdout, stderr, err := ExecAtWithInput(boot0, []byte(podYAML), "kubectl", "apply", "-f", "-")
		Expect(err).NotTo(HaveOccurred(), "stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
	})

	It("should create ubuntu pod with annotation on sandbox ns", func() {
		podYAMLWIthAnnotation := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: ubuntu-with-nat-annotation
  namespace: sandbox
spec:
  replicas: 1
  selector:
    matchLabels:
      custom-egress-test: nat
  template:
    metadata:
      annotations:
        egress.coil.cybozu.com/customer-egress: nat
      labels:
        custom-egress-test: nat
    spec:
      securityContext:
        runAsUser: 1000
        runAsGroup: 1000
      containers:
      - args:
        - pause
        image: quay.io/cybozu/ubuntu-debug:20.04
        name: ubuntu
`
		stdout, stderr, err := ExecAtWithInput(boot0, []byte(podYAMLWIthAnnotation), "kubectl", "apply", "-f", "-")
		Expect(err).NotTo(HaveOccurred(), "stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
	})
}

func testCustomerEgress() {
	It("should deploy squid successfully", func() {
		Eventually(func() error {
			stdout, _, err := ExecAt(boot0, "kubectl", "--namespace=customer-egress",
				"get", "deployment/squid", "-o=json")
			if err != nil {
				return err
			}
			deployment := new(appsv1.Deployment)
			err = json.Unmarshal(stdout, deployment)
			if err != nil {
				return err
			}

			if int(deployment.Status.ReadyReplicas) != 2 {
				return fmt.Errorf("ReadyReplicas is not 2: %d", int(deployment.Status.ReadyReplicas))
			}
			return nil
		}).Should(Succeed())
	})

	It("should serve proxy to the Internet", func() {
		By("executing curl to web page on the Internet with squid")
		Eventually(func() error {
			stdout, stderr, err := ExecAt(boot0, "kubectl", "-nsandbox", "get", "pods", "-l", "custom-egress-test=non-nat", "-o", "json")
			if err != nil {
				return fmt.Errorf("stderr: %s: %w", string(stderr), err)
			}
			podList := &corev1.PodList{}
			if err := json.Unmarshal(stdout, podList); err != nil {
				return err
			}
			if len(podList.Items) != 1 {
				return fmt.Errorf("podList length is not 1: %d", len(podList.Items))
			}
			podName := podList.Items[0].Name
			stdout, stderr, err = ExecAt(boot0, "kubectl", "-nsandbox", "exec", podName, "--", "curl", "-sf", "--proxy", "http://squid.customer-egress.svc:3128", "cybozu.com")
			if err != nil {
				return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
			}
			return nil
		}).Should(Succeed())
	})

	It("should deploy coil egress successfully", func() {
		Eventually(func() error {
			stdout, _, err := ExecAt(boot0, "kubectl", "--namespace=customer-egress",
				"get", "deployment/nat", "-o=json")
			if err != nil {
				return err
			}
			deployment := new(appsv1.Deployment)
			err = json.Unmarshal(stdout, deployment)
			if err != nil {
				return err
			}

			if int(deployment.Status.ReadyReplicas) != 2 {
				return fmt.Errorf("ReadyReplicas is not 2: %d", int(deployment.Status.ReadyReplicas))
			}
			return nil
		}).Should(Succeed())

		By("executing curl to web page on the Internet without squid")
		Eventually(func() error {
			stdout, stderr, err := ExecAt(boot0, "kubectl", "-nsandbox", "get", "pods", "-l", "custom-egress-test=nat", "-o", "json")
			if err != nil {
				return fmt.Errorf("stderr: %s: %w", string(stderr), err)
			}
			podList := &corev1.PodList{}
			if err := json.Unmarshal(stdout, podList); err != nil {
				return err
			}
			if len(podList.Items) != 1 {
				return fmt.Errorf("podList length is not 1: %d", len(podList.Items))
			}
			podName := podList.Items[0].Name
			stdout, stderr, err = ExecAt(boot0, "kubectl", "-nsandbox", "exec", podName, "--", "curl", "-sf", "cybozu.com")
			if err != nil {
				return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
			}
			return nil
		}).Should(Succeed())

		By("deleting ubuntu pods on sandbox ns")
		for _, name := range []string{"ubuntu-without-nat-annotation", "ubuntu-with-nat-annotation"} {
			stdout, stderr, err := ExecAt(boot0, "kubectl", "-nsandbox", "delete", "deployments", name)
			Expect(err).NotTo(HaveOccurred(), "stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
		}
	})
}
