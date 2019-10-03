package test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

func testExternalDNS() {
	It("should be deployed successfully", func() {
		Eventually(func() error {
			stdout, _, err := ExecAt(boot0, "kubectl", "--namespace=external-dns",
				"get", "deployment/external-dns", "-o=json")
			if err != nil {
				return err
			}

			deployment := new(appsv1.Deployment)
			err = json.Unmarshal(stdout, deployment)
			if err != nil {
				return err
			}

			if deployment.Status.AvailableReplicas != 1 {
				return fmt.Errorf("external-dns deployment's AvailableReplica is not 1: %d", int(deployment.Status.AvailableReplicas))
			}
			return nil
		}).Should(Succeed())
	})

	It("should create DNS record", func() {
		domainName := testID + "-external-dns.gcp0.dev-ne.co"
		By("deploying DNSEndpoint")
		dnsEndpoint := fmt.Sprintf(`
apiVersion: externaldns.k8s.io/v1alpha1
kind: DNSEndpoint
metadata:
  name: test-endpoint
  namespace: ingress
spec:
  endpoints:
  - dnsName: %s
    recordTTL: 180
    recordType: A
    targets:
    - 10.0.5.9
`, domainName)
		_, stderr, err := ExecAtWithInput(boot0, []byte(dnsEndpoint), "kubectl", "apply", "-f", "-")
		Expect(err).NotTo(HaveOccurred(), "stderr: %s", stderr)

		By("resolving xxx.gcp0.dev-ne.co")
		Eventually(func() error {
			stdout, stderr, err := ExecAt(boot0, "kubectl", "get", "nodes", "-o", "json")
			if err != nil {
				return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
			}

			var nl corev1.NodeList
			err = json.Unmarshal(stdout, &nl)
			if err != nil {
				return err
			}
			if len(nl.Items) == 0 {
				return errors.New("nodes not found")
			}
			node := nl.Items[0]

			var nodeAddress string
			for _, addr := range node.Status.Addresses {
				if addr.Type != corev1.NodeInternalIP {
					continue
				}
				nodeAddress = addr.Address
			}
			if len(nodeAddress) == 0 {
				return errors.New("can not get NodeIP")
			}

			stdout, stderr, err = ExecAt(boot0, "ckecli", "ssh", nodeAddress, "dig", "+noall", "+answer", domainName)
			if err != nil {
				return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
			}
			// expected: xxx.gcp0.dev-ne.co. 300 IN A 10.0.5.9
			fields := strings.Fields(string(bytes.TrimSpace(stdout)))
			if len(fields) < 5 || fields[4] != "10.0.5.9" {
				// ignore errors while gathering additional information
				dumpEndpoint, _, _ := ExecAt(boot0, "kubectl", "get", "-n", "ingress", "dnsendpoint", "test-endpoint", "-o", "yaml")
				podLog, _, _ := ExecAt(boot0, "kubectl", "-n", "external-dns", "logs", "-l", "app.kubernetes.io/name=external-dns", "--tail", "10000")
				return fmt.Errorf("expected IP address is 10.0.5.9, but actual response is %s\ndump of DNSEndpoint: %s\nexternal-dns logs: %s", string(stdout), string(dumpEndpoint), string(podLog))
			}
			return nil
		}).Should(Succeed())

		By("cleaning up")
		_, stderr, err = ExecAt(boot0, "kubectl", "delete", "-n=ingress", "dnsendpoints/test-endpoint")
		Expect(err).NotTo(HaveOccurred(), "stderr: %s", stderr)
	})
}
