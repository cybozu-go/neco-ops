package test

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
)

//go:embed testdata/moco.yaml
var mocoYAML []byte

func prepareMoco() {
	It("should deploy mysqlcluster", func() {
		By("creating mysqlcluster")
		createNamespaceIfNotExists("test-moco", false)
		_, stderr, err := ExecAtWithInput(boot0, mocoYAML, "kubectl", "apply", "-f", "-")
		Expect(err).NotTo(HaveOccurred(), "stderr: %s", stderr)
	})
}

func testMoco() {
	It("should be deployed successfully", func() {
		Eventually(func() error {
			stdout, _, err := ExecAt(boot0, "kubectl", "--namespace=moco-system",
				"get", "deployment/moco-controller-manager", "-o=json")
			if err != nil {
				return err
			}
			deployment := new(appsv1.Deployment)
			err = json.Unmarshal(stdout, deployment)
			if err != nil {
				return err
			}

			if int(deployment.Status.AvailableReplicas) != 1 {
				return fmt.Errorf("AvailableReplicas is not 1: %d", int(deployment.Status.AvailableReplicas))
			}
			return nil
		}).Should(Succeed())
	})

	It("should work", func() {
		By("waiting mysqlcluster is ready")
		Eventually(func() error {
			stdout, _, err := ExecAt(boot0, "kubectl", "--namespace=test-moco", "get", "mysqlcluster/my-cluster", "-o", "jsonpath='{.status.ready}'")
			if err != nil {
				return err
			}

			if string(stdout) != "True" {
				return errors.New("MySQLCluster is not ready")
			}
			return nil
		}).Should(Succeed())

		By("running kubectl moco mysql")
		stdout, stderr, err := ExecAt(boot0, "kubectl", "moco", "-n", "test-moco", "mysql", "-u", "root", "my-cluster", "--", "--version")
		Expect(err).ShouldNot(HaveOccurred(), "stdout=%s, stderr=%s", stdout, stderr)
		Expect(string(stdout)).Should(ContainSubstring("mysql  Ver 8"))
	})
}
