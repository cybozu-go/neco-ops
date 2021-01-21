package test

import (
	"encoding/json"
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
)

func testMetricsServer() {
	It("should get metrics", func() {
		By("confirming metrics-server is deployed successfully")
		Eventually(func() error {
			stdout, stderr, err := ExecAt(boot0, "kubectl", "--namespace=kube-system",
				"get", "deployment", "metrics-server", "-o=json")
			if err != nil {
				return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
			}

			deployment := new(appsv1.Deployment)
			err = json.Unmarshal(stdout, deployment)
			if err != nil {
				return fmt.Errorf("stdout: %s, err: %v", stdout, err)
			}

			if deployment.Status.AvailableReplicas != 2 {
				return fmt.Errorf("metrics-server deployment's AvailableReplica is not 2: %d", int(deployment.Status.AvailableReplicas))
			}

			return nil
		}).Should(Succeed())

		By("confirming top command is available")
		Eventually(func() error {
			stdout, stderr, err := ExecAt(boot0, "kubectl", "top", "node")
			if err != nil {
				return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
			}

			stdout, stderr, err = ExecAt(boot0, "kubectl", "top", "pod")
			if err != nil {
				return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
			}

			return nil
		}).Should(Succeed())
	})
}
