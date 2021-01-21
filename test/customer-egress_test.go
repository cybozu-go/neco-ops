package test

import (
	"encoding/json"
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
)

var (
	ubuntuPodName         = "ubuntu"
	podWithAnnotationName = "ubuntu-with-nat-annotation"
)

func prepareCustomerEgress() {
	It("should create ubuntu pod on sandbox ns", func() {
		podYAML := fmt.Sprintf(`apiVersion: v1
kind: Pod
metadata:
  name: %s
  namespace: sandbox
spec:
  containers:
  - args:
    - pause
    image: quay.io/cybozu/ubuntu-debug:20.04
    name: ubuntu`, ubuntuPodName)
		stdout, stderr, err := ExecAtWithInput(boot0, []byte(podYAML), "kubectl", "apply", "-f", "-")
		Expect(err).NotTo(HaveOccurred(), "stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
	})

	It("should create ubuntu pod with annotation on sandbox ns", func() {
		podYAMLWIthAnnotation := fmt.Sprintf(`apiVersion: v1
kind: Pod
metadata:
  name: %s
  namespace: sandbox
  annotations:
    egress.coil.cybozu.com/customer-egress: nat
spec:
  containers:
  - args:
    - pause
    image: quay.io/cybozu/ubuntu-debug:20.04
    name: ubuntu`, podWithAnnotationName)
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
			stdout, stderr, err := ExecAt(boot0, "kubectl", "-nsandbox", "exec", ubuntuPodName, "--", "curl", "-sf", "--proxy", "http://squid.customer-egress.svc:3128", "cybozu.com")
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
			stdout, stderr, err := ExecAt(boot0, "kubectl", "-nsandbox", "exec", podWithAnnotationName, "--", "curl", "-sf", "cybozu.com")
			if err != nil {
				return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
			}
			return nil
		}).Should(Succeed())

		By("deleting ubuntu pod on sandbox ns")
		for _, name := range []string{ubuntuPodName, podWithAnnotationName} {
			stdout, stderr, err := ExecAt(boot0, "kubectl", "-nsandbox", "delete", "pod", name)
			Expect(err).NotTo(HaveOccurred(), "stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
		}
	})
}
