package test

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func prepareArgoCDIngress() {
	argocdFQDN := testID + "-argocd.gcp0.dev-ne.co"
	It("should create HTTPProxy for ArgoCD", func() {
		manifest := fmt.Sprintf(`
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: argocd-server-test
  namespace: argocd
  annotations:
    kubernetes.io/tls-acme: "true"
    kubernetes.io/ingress.class: bastion
spec:
  virtualhost:
    fqdn: %s
    tls:
      secretName: argocd-server-cert-test
  routes:
    # For static files and Dex APIs
    - conditions:
        - prefix: /
      services:
        - name: argocd-server-https
          port: 443
      timeoutPolicy:
        response: 2m
        idle: 5m
    # For gRPC APIs
    - conditions:
        - prefix: /
        - header:
            name: content-type
            contains: application/grpc
      services:
        - name: argocd-server
          port: 443
      timeoutPolicy:
        response: 2m
        idle: 5m
`, argocdFQDN)

		_, stderr, err := ExecAtWithInput(boot0, []byte(manifest), "kubectl", "apply", "-f", "-")
		Expect(err).NotTo(HaveOccurred(), "stderr: %s", stderr)
	})
}

func testArgoCDIngress() {
	argocdFQDN := testID + "-argocd.gcp0.dev-ne.co"
	It("should confirm Argo CD functionalities", func() {
		By("confirming created Certificate")
		Eventually(func() error {
			return checkCertificate("argocd-server-test", "argocd")
		}).Should(Succeed())

		By("logging in to Argo CD")
		Eventually(func() error {
			stdout, stderr, err := ExecAt(boot0, "argocd", "login", argocdFQDN,
				"--insecure", "--username", "admin", "--password", loadArgoCDPassword())
			if err != nil {
				return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
			}
			return nil
		}).Should(Succeed())

		By("requesting to web UI with https")
		stdout, stderr, err := ExecAt(boot0,
			"curl", "-skL", "https://"+argocdFQDN,
			"-o", "/dev/null",
			"-w", `'%{http_code}\n%{content_type}'`,
		)
		Expect(err).ShouldNot(HaveOccurred(), "stdout: %s, stderr: %s", stdout, stderr)
		s := strings.Split(string(stdout), "\n")
		Expect(s[0]).To(Equal(strconv.Itoa(http.StatusOK)))
		Expect(s[1]).To(Equal("text/html; charset=utf-8"))

		By("requesting to argocd-dex-server via argocd-server with https")
		stdout, stderr, err = ExecAt(boot0,
			"curl", "-skL", "https://"+argocdFQDN+"/api/dex/.well-known/openid-configuration",
			"-o", "/dev/null",
			"-w", `'%{http_code}\n%{content_type}'`,
		)
		Expect(err).ShouldNot(HaveOccurred(), "stdout: %s, stderr: %s", stdout, stderr)
		s = strings.Split(string(stdout), "\n")
		Expect(s[0]).To(Equal(strconv.Itoa(http.StatusOK)))
		Expect(s[1]).To(Equal("application/json"))

		By("requesting to argocd-server with gRPC")
		stdout, stderr, err = ExecAt(boot0,
			"curl", "-skL", "https://"+argocdFQDN+"/account.AccountService/Read",
			"-H", "'Content-Type: application/grpc'",
			"-o", "/dev/null",
			"-w", `'%{http_code}\n%{content_type}'`,
		)
		Expect(err).ShouldNot(HaveOccurred(), "stdout: %s, stderr: %s", stdout, stderr)
		s = strings.Split(string(stdout), "\n")
		Expect(s[0]).To(Equal(strconv.Itoa(http.StatusOK)))
		Expect(s[1]).To(Equal("application/grpc"))

		By("requesting to argocd-server with gRPC-Web")
		stdout, stderr, err = ExecAt(boot0,
			"curl", "-skL", "https://"+argocdFQDN+"/application.ApplicationService/Read",
			"-H", "'Content-Type: application/grpc-web+proto'",
			"-o", "/dev/null",
			"-w", `'%{http_code}\n%{content_type}'`,
		)
		Expect(err).ShouldNot(HaveOccurred(), "stdout: %s, stderr: %s", stdout, stderr)
		s = strings.Split(string(stdout), "\n")
		Expect(s[0]).To(Equal(strconv.Itoa(http.StatusOK)))
		Expect(s[1]).To(Equal("application/grpc-web+proto"))
	})
}
