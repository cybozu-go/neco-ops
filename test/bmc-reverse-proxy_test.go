package test

import (
	"encoding/json"
	"fmt"
	corev1 "k8s.io/api/core/v1"
	"os/exec"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
)

func testBMCReverseProxy() {
	It("should be deployed successfully", func() {
		Eventually(func() error {
			stdout, stderr, err := ExecAt(boot0, "kubectl", "--namespace=bmc-reverse-proxy",
				"get", "deployment", "bmc-reverse-proxy", "-o=json")
			if err != nil {
				return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
			}

			deployment := new(appsv1.Deployment)
			err = json.Unmarshal(stdout, deployment)
			if err != nil {
				return fmt.Errorf("stdout: %s, err: %v", stdout, err)
			}

			if deployment.Status.AvailableReplicas != 2 {
				return fmt.Errorf("bmc-reverse-proxy deployment's AvailableReplica is not 2: %d", int(deployment.Status.AvailableReplicas))
			}

			return nil
		}).Should(Succeed())
	})

	It("should create ConfigMap", func() {
		Eventually(func() error {
			stdout, stderr, err := ExecAt(boot0, "kubectl", "--namespace=bmc-reverse-proxy",
				"get", "configmap", "bmc-reverse-proxy", "-o=json")
			if err != nil {
				return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
			}

			cm := new(corev1.ConfigMap)
			err = json.Unmarshal(stdout, cm)
			if err != nil {
				return fmt.Errorf("stdout: %s, err: %v", stdout, err)
			}

			data := cm.Data
			if data["rack3-cs4"] != "10.72.17.100" {
				return fmt.Errorf("bmc-reverse-proxy rack3-cs4 IP Adress is not 10.72.17.100: %s", data["rack3-cs4"])
			}

			if data["10-69-2-68"] != "10.72.17.100" {
				return fmt.Errorf("bmc-reverse-proxy 10-69-2-68 IP Adress is not 10.72.17.100: %s", data["10-69-2-68"])
			}

			if data["168799a36a60da24f934e2adf9e455e25a3ad4ef"] != "10.72.17.100" {
				return fmt.Errorf("bmc-reverse-proxy 168799a36a60da24f934e2adf9e455e25a3ad4ef IP Adress is not 10.72.17.100: %s", data["168799a36a60da24f934e2adf9e455e25a3ad4ef"])
			}

			return nil
		}).Should(Succeed())
	})

	It("should be accessed via https", func() {
		Eventually(func() error {
			stdout, stderr, err := ExecAt(boot0, "kubectl", "-n", "bmc-reverse-proxy", "get", "service", "bmc-reverse-proxy",
				"--output=jsonpath={.status.loadBalancer.ingress[0].ip}")
			if err != nil {
				return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
			}
			addr := string(stdout)

			cmd := exec.Command("curl", "--fail", "--insecure", "-H", "Host: rack3-cs4.bmc.gcp0.dev-ne.co", fmt.Sprintf("https://%s", addr))
			output, err := cmd.CombinedOutput()
			if err != nil {
				return fmt.Errorf("output: %s, err: %v", output, err)
			}

			return nil
		}).Should(Succeed())
	})
}
