package test

import (
	"encoding/json"
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
)

func testArgoCDIngress() {
	It("should login via IngressRoute", func() {
		By("getting the ip address of the contour LoadBalancer")
		stdout, _, err := ExecAt(boot0, "kubectl", "--namespace=ingress", "get", "service/contour-bastion", "-o=json")
		Expect(err).ShouldNot(HaveOccurred())

		svc := new(corev1.Service)
		err = json.Unmarshal(stdout, svc)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(len(svc.Status.LoadBalancer.Ingress)).To(Equal(1))
		lbIP := svc.Status.LoadBalancer.Ingress[0].IP

		By("adding loadbalancer address entry to /etc/hosts")
		_, stderr, err := ExecAt(boot0, "sudo", "bash", "-c", "'echo "+lbIP+" argocd.gcp0.dev-ne.co >> /etc/hosts'")
		Expect(err).ShouldNot(HaveOccurred(), "stderr: %s", stderr)

		By("logging in to Argo CD")
		Eventually(func() error {
			stdout, stderr, err := ExecAt(boot0, "argocd", "login", "argocd.gcp0.dev-ne.co",
				"--insecure", "--username", "admin", "--password", loadArgoCDPassword())
			if err != nil {
				return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
			}
			return nil
		}).Should(Succeed())
	})
}
