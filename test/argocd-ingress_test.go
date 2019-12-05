package test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
)

func testArgoCDIngress() {
	It("should login via HTTPProxy as admin", func() {
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

	It("should make SSO enabled", func() {
		By("requesting to web UI with https")
		stdout, stderr, err := ExecAt(boot0,
			"curl", "-skL", "https://argocd.gcp0.dev-ne.co",
			"-o", "/dev/null",
			"-w", `'%{http_code}\n%{content_type}'`,
		)
		Expect(err).ShouldNot(HaveOccurred(), "stdout: %s, stderr: %s", stdout, stderr)
		s := strings.Split(string(stdout), "\n")
		Expect(s[0]).To(Equal(strconv.Itoa(http.StatusOK)))
		Expect(s[1]).To(Equal("text/html; charset=utf-8"))

		By("requesting to argocd-dex-server via argocd-server with https")
		stdout, stderr, err = ExecAt(boot0,
			"curl", "-skL", "https://argocd.gcp0.dev-ne.co/api/dex/.well-known/openid-configuration",
			"-o", "/dev/null",
			"-w", `'%{http_code}\n%{content_type}'`,
		)
		Expect(err).ShouldNot(HaveOccurred(), "stdout: %s, stderr: %s", stdout, stderr)
		s = strings.Split(string(stdout), "\n")
		Expect(s[0]).To(Equal(strconv.Itoa(http.StatusOK)))
		Expect(s[1]).To(Equal("application/json"))

		By("requesting to argocd-server with grpc")
		// They are configured as routes in HTTPProxy individually to communicate with grpc and should be tested.
		endpoints := []string{
			"/account.AccountService",
			"/application.ApplicationService",
			"/certificate.CertificateService",
			"/cluster.ClusterService",
			"/cluster.SettingsService",
			"/project.ProjectService",
			"/repocreds.RepoCredsService",
			"/repository.RepoServerService",
			"/repository.RepositoryService",
			"/session.SessionService",
			"/version.VersionService",
		}
		for _, e := range endpoints {
			stdout, stderr, err = ExecAt(boot0,
				"curl", "-skL", "https://argocd.gcp0.dev-ne.co"+e+"/Read",
				"-o", "/dev/null",
				"-w", `'%{http_code}\n%{content_type}'`,
			)
			Expect(err).ShouldNot(HaveOccurred(), "stdout: %s, stderr: %s", stdout, stderr)
			s = strings.Split(string(stdout), "\n")
			Expect(s[0]).To(Equal(strconv.Itoa(http.StatusOK)))
			Expect(s[1]).To(Equal("application/grpc"))
		}
	})
}
