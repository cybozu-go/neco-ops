package test

import (
	"encoding/json"
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
)

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
		podName := "customer-egress-test"

		By("running ubuntu pod on sandbox ns")
		stdout, stderr, err := ExecAt(boot0, "kubectl", "-nsandbox", "run", podName, "--image=quay.io/cybozu/ubuntu-debug:18.04", "pause")
		Expect(err).NotTo(HaveOccurred(), "stdout: %s, stderr: %s, err: %v", stdout, stderr, err)

		By("executing curl to web page on the Internet")
		Eventually(func() error {
			stdout, stderr, err := ExecAt(boot0, "kubectl", "-nsandbox", "exec", podName, "--", "curl", "-sf", "--proxy", "http://squid.customer-egress.svc:3128", "cybozu.com")
			if err != nil {
				return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
			}
			return nil
		}).Should(Succeed())

		By("deleting ubuntu pod on sandbox ns")
		stdout, stderr, err = ExecAt(boot0, "kubectl", "-nsandbox", "delete", "pod", podName)
		Expect(err).NotTo(HaveOccurred(), "stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
	})
}
