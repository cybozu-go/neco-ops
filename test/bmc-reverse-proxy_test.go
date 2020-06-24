package test

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/cybozu-go/sabakan/v2"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
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

	var machines []sabakan.Machine

	It("should create ConfigMap", func() {
		// check consistency between "sabactl machines get" and bmc-reverse-proxy ConfigMap.
		stdout, _, err := ExecAt(boot0, "sabactl", "machines", "get")
		Expect(err).ShouldNot(HaveOccurred())
		err = json.Unmarshal(stdout, &machines)
		Expect(err).ShouldNot(HaveOccurred())

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
			for _, m := range machines {
				bmcIP := m.Spec.BMC.IPv4
				var hostname string
				if m.Spec.Role == "boot" {
					hostname = fmt.Sprintf("boot-%d", m.Spec.Rack)
				} else {
					hostname = fmt.Sprintf("rack%d-%s%d", m.Spec.Rack, m.Spec.Role, m.Spec.IndexInRack)
				}
				if data[hostname] != bmcIP {
					return fmt.Errorf("bmc-reverse-proxy %s IP Address is not %s: %s", hostname, bmcIP, data[hostname])
				}

				// IPv4[0] is the virtual IP, and IPv4[1] and IPv4[2] are the real node IP belong to ToR subnet.
				// See sabakan integration
				nodeIP := strings.Replace(m.Spec.IPv4[0], ".", "-", -1)
				if data[nodeIP] != bmcIP {
					return fmt.Errorf("bmc-reverse-proxy %s IP Address is not %s: %s", nodeIP, bmcIP, data[nodeIP])
				}

				nodeSerial := m.Spec.Serial
				if data[nodeSerial] != bmcIP {
					return fmt.Errorf("bmc-reverse-proxy %s IP Address is not %s: %s", nodeSerial, bmcIP, data[nodeSerial])
				}
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

			someNodeSerial := machines[0].Spec.Serial
			cmd := exec.Command("curl", "--fail", "--insecure", "-H", fmt.Sprintf("Host: %s.bmc.gcp0.dev-ne.co", someNodeSerial), fmt.Sprintf("https://%s", addr))
			output, err := cmd.CombinedOutput()
			if err != nil {
				return fmt.Errorf("output: %s, err: %v", output, err)
			}

			return nil
		}).Should(Succeed())
	})
}
