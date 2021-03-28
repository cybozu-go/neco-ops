package test

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"text/template"

	"github.com/cybozu-go/log"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1beta1 "k8s.io/api/policy/v1beta1"
)

//go:embed testdata/contour-deploy.yaml
var contourDeployYAML []byte

//go:embed testdata/contour-httpproxy.yaml
var contourHTTPProxyYAML string

var ingressNamespaces = []string{"ingress-global", "ingress-forest", "ingress-bastion"}

func prepareContour() {
	It("should prepare resources in test-ingress namespace", func() {
		By("preparing namespace")
		createNamespaceIfNotExists("test-ingress")

		By("creating pod and service")
		_, stderr, err := ExecAtWithInput(boot0, contourDeployYAML, "kubectl", "apply", "-f", "-")
		Expect(err).NotTo(HaveOccurred(), "stderr: %s", stderr)

		By("creating HTTPProxy")
		tmpl := template.Must(template.New("").Parse(contourHTTPProxyYAML))
		buf := new(bytes.Buffer)
		err = tmpl.Execute(buf, testID)
		Expect(err).NotTo(HaveOccurred())
		_, stderr, err = ExecAtWithInput(boot0, buf.Bytes(), "kubectl", "apply", "-f", "-")
		Expect(err).NotTo(HaveOccurred(), "stderr: %s", stderr)
	})
}

func testContour() {
	It("should deploy contour successfully", func() {
		Eventually(func() error {
			for _, ns := range ingressNamespaces {
				stdout, stderr, err := ExecAt(boot0, "kubectl", "--namespace="+ns,
					"get", "deployment/contour", "-o=json")
				if err != nil {
					return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
				}

				deployment := new(appsv1.Deployment)
				err = json.Unmarshal(stdout, deployment)
				if err != nil {
					return err
				}

				if deployment.Status.AvailableReplicas != 2 {
					return fmt.Errorf("contour deployment's AvailableReplica is not 2 in %s: %d", ns, int(deployment.Status.AvailableReplicas))
				}
			}
			return nil
		}).Should(Succeed())
	})

	It("should deploy envoy successfully", func() {
		Eventually(func() error {
			for _, ns := range ingressNamespaces {
				stdout, stderr, err := ExecAt(boot0, "kubectl", "--namespace="+ns,
					"get", "deployment/envoy", "-o=json")
				if err != nil {
					return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
				}

				deployment := new(appsv1.Deployment)
				err = json.Unmarshal(stdout, deployment)
				if err != nil {
					return err
				}

				if deployment.Status.AvailableReplicas != 3 {
					return fmt.Errorf("envoy deployment's AvailableReplica is not 3 in %s: %d", ns, int(deployment.Status.AvailableReplicas))
				}
			}
			return nil
		}).Should(Succeed())
	})

	It("should deploy HTTPProxy", func() {
		By("waiting pods are ready")
		Eventually(func() error {
			stdout, _, err := ExecAt(boot0, "kubectl", "-n", "test-ingress", "get", "deployments/testhttpd", "-o", "json")
			if err != nil {
				return err
			}

			deployment := new(appsv1.Deployment)
			err = json.Unmarshal(stdout, deployment)
			if err != nil {
				return err
			}

			if deployment.Status.ReadyReplicas != 2 {
				return errors.New("ReadyReplicas is not 2")
			}
			return nil
		}).Should(Succeed())

		By("checking PodDisruptionBudget for contour Deployment")
		Eventually(func() error {
			for _, ns := range ingressNamespaces {
				pdb := policyv1beta1.PodDisruptionBudget{}
				stdout, stderr, err := ExecAt(boot0, "kubectl", "get", "poddisruptionbudgets", "contour-pdb", "-n", ns, "-o", "json")
				if err != nil {
					return fmt.Errorf("failed to get %s/contour-pdb: %s: %w", ns, stderr, err)
				}
				if err := json.Unmarshal(stdout, &pdb); err != nil {
					return err
				}
				if pdb.Status.CurrentHealthy != 2 {
					return fmt.Errorf("unhalthy contour-pdb in %s: %d", ns, pdb.Status.CurrentHealthy)
				}
			}
			return nil
		}).Should(Succeed())

		By("checking PodDisruptionBudget for envoy Deployment")
		for _, ns := range ingressNamespaces {
			pdb := policyv1beta1.PodDisruptionBudget{}
			stdout, stderr, err := ExecAt(boot0, "kubectl", "get", "poddisruptionbudgets", "envoy-pdb", "-n", ns, "-o", "json")
			if err != nil {
				Expect(err).ShouldNot(HaveOccurred(), "stdout=%s, stderr=%s", stdout, stderr)
			}
			err = json.Unmarshal(stdout, &pdb)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(pdb.Status.CurrentHealthy).Should(Equal(int32(3)), "namespace=%s", ns)
		}

		fqdnHTTP := testID + "-http.test-ingress.gcp0.dev-ne.co"
		fqdnHTTPS := testID + "-https.test-ingress.gcp0.dev-ne.co"
		fqdnBastion := testID + "-bastion.test-ingress.gcp0.dev-ne.co"

		By("getting contour service")
		var targetIP string
		Eventually(func() error {
			stdout, _, err := ExecAt(boot0, "kubectl", "get", "-n", "ingress-global", "service/envoy", "-o", "json")
			if err != nil {
				return err
			}

			service := new(corev1.Service)
			err = json.Unmarshal(stdout, service)
			if err != nil {
				return err
			}

			if len(service.Status.LoadBalancer.Ingress) < 1 {
				return errors.New("LoadBalancerIP is not assigned")
			}
			targetIP = service.Status.LoadBalancer.Ingress[0].IP
			if len(targetIP) == 0 {
				return errors.New("LoadBalancerIP is empty")
			}
			return nil
		}).Should(Succeed())

		By("confirming generated DNSEndpoint")
		Eventually(func() error {
			stdout, _, err := ExecAt(boot0, "kubectl", "get", "-n", "test-ingress", "dnsendpoint/root", "-o", "json")
			if err != nil {
				return err
			}

			var de struct {
				Spec struct {
					Endpoints []*struct {
						Targets []string `json:"targets,omitempty"`
					} `json:"endpoints,omitempty"`
				} `json:"spec,omitempty"`
			}
			err = json.Unmarshal(stdout, &de)
			if err != nil {
				return err
			}
			if len(de.Spec.Endpoints) == 0 {
				return errors.New("len(de.Spec.Endpoints) == 0")
			}
			actualIP := de.Spec.Endpoints[0].Targets[0]

			if targetIP != actualIP {
				return fmt.Errorf("expected IP is (%s), but actual is (%s)", targetIP, actualIP)
			}
			return nil
		}).Should(Succeed())

		By("confirming created Certificate")
		Eventually(func() error {
			return checkCertificate("tls", "test-ingress")
		}).Should(Succeed())

		By("accessing with curl: http")
		Eventually(func() error {
			_, _, err := ExecInNetns("external", "curl", "--resolve", fqdnHTTP+":80:"+targetIP,
				"http://"+fqdnHTTP+"/testhttpd", "-m", "5", "--fail")
			return err
		}).Should(Succeed())

		By("accessing with curl: https")
		exec.Command("curl", "-sfL", "-o", "lets.crt", "https://letsencrypt.org/certs/fakelerootx1.pem")
		Eventually(func() error {
			stdout, stderr, err := ExecInNetns(
				"external",
				"curl",
				"-v",
				"--resolve",
				fqdnHTTPS+":443:"+targetIP,
				"https://"+fqdnHTTPS+"/",
				"-m",
				"5",
				"--cacert",
				"lets.crt",
			)
			if err != nil {
				return fmt.Errorf("failed to curl; stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
			}
			return nil
		}).Should(Succeed())

		By("redirecting to https")
		Eventually(func() error {
			stdout, _, err := ExecAt(boot0, "curl", "-I", "--resolve", fqdnHTTPS+":80:"+targetIP,
				"http://"+fqdnHTTPS+"/",
				"-m", "5",
				"--fail",
				"-o", "/dev/null",
				"-w", "'%{http_code}'",
				"-s",
				"--cacert", "lets.crt",
			)
			if err != nil {
				return err
			}
			if string(stdout) != "301" {
				return errors.New("unexpected status: " + string(stdout))
			}
			return nil
		}).Should(Succeed())

		By("permitting insecure access")
		Eventually(func() error {
			stdout, _, err := ExecAt(boot0, "curl", "-I", "--resolve", fqdnHTTPS+":80:"+targetIP,
				"http://"+fqdnHTTPS+"/insecure",
				"-m", "5",
				"--fail",
				"-o", "/dev/null",
				"-w", "'%{http_code}'",
				"-s",
			)
			if err != nil {
				return err
			}
			if string(stdout) != "200" {
				return errors.New("unexpected status: " + string(stdout))
			}
			return nil
		}).Should(Succeed())

		By("trying to access from the Internet with a bastion URL")
		// Though we expect 404 errors for invalid accesses, such errors can occur even when HTTPProxy has not been processed.
		// So at first, access through the valid IP address and expect 200.
		var bastionIP string
		Eventually(func() error {
			stdout, _, err := ExecAt(boot0, "kubectl", "get", "-n", "ingress-bastion", "service/envoy", "-o", "json")
			if err != nil {
				return err
			}

			service := new(corev1.Service)
			err = json.Unmarshal(stdout, service)
			if err != nil {
				return err
			}

			if len(service.Status.LoadBalancer.Ingress) < 1 {
				return errors.New("LoadBalancerIP is not assigned")
			}
			bastionIP = service.Status.LoadBalancer.Ingress[0].IP
			if len(bastionIP) == 0 {
				return errors.New("LoadBalancerIP is empty")
			}
			return nil
		}).Should(Succeed())

		Eventually(func() error {
			stdout, _, err := ExecAt(boot0, "curl", "-I", "--resolve", fqdnBastion+":80:"+bastionIP,
				"http://"+fqdnBastion+"/testhttpd",
				"-m", "5",
				"--fail",
				"-o", "/dev/null",
				"-w", "'%{http_code}'",
				"-s",
			)
			if err != nil {
				return err
			}
			if string(stdout) != "200" {
				return errors.New("unexpected status: " + string(stdout))
			}
			return nil
		}).Should(Succeed())

		stdout, _, err := ExecAt(boot0, "curl", "-I", "--resolve", fqdnBastion+":80:"+targetIP,
			"http://"+fqdnBastion+"/testhttpd",
			"-m", "5",
			"--fail",
			"-o", "/dev/null",
			"-w", "'%{http_code}'",
			"-s",
		)
		Expect(err).To(HaveOccurred())
		Expect(string(stdout)).To(Equal("404"))
	})
}

func getCertificateRequest(cert Certificate, namespace string) (*CertificateRequest, error) {
	var certReqList CertificateRequestList
	var targetCertReq *CertificateRequest

	stdout, stderr, err := ExecAt(boot0, "kubectl", "get", "-n", namespace, "certificaterequest", "-o", "json")
	if err != nil {
		return nil, fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
	}
	err = json.Unmarshal(stdout, &certReqList)
	if err != nil {
		return nil, err
	}

OUTER:
	for _, cr := range certReqList.Items {
		for _, or := range cr.OwnerReferences {
			if or.Name == cert.Name {
				targetCertReq = &cr
				break OUTER
			}
		}
	}

	if targetCertReq == nil {
		return nil, fmt.Errorf("CertificateRequest is not found")
	}
	return targetCertReq, nil
}

func checkCertificate(name, namespace string) error {
	stdout, stderr, err := ExecAt(boot0, "kubectl", "get", "-n", namespace, "certificate", name, "-o", "json")
	if err != nil {
		return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
	}

	var cert Certificate
	err = json.Unmarshal(stdout, &cert)
	if err != nil {
		return err
	}

	for _, st := range cert.Status.Conditions {
		if st.Type != CertificateConditionReady {
			continue
		}
		// debug output
		fmt.Printf("certificate status. time: %s, status: %s, reason: %s, message: %s\n", st.LastTransitionTime.String(), st.Status, st.Reason, st.Message)

		if st.Status == "True" {
			return nil
		}
	}

	// Check the CertificateRequest status (the result of ACME challenge).
	// If the status is failed, delete the Certificate and force to retry the ACME challenge.
	// The Certificate will be recreated by contour-plus.
	certReq, err := getCertificateRequest(cert, namespace)
	if err != nil {
		return err
	}
	for _, st := range certReq.Status.Conditions {
		if st.Type != CertificateRequestConditionReady {
			continue
		}

		if st.Reason == CertificateRequestReasonFailed {
			log.Error("CertificateRequest failed", map[string]interface{}{
				"certificate":        cert.Name,
				"certificaterequest": certReq.Name,
				"status":             st.Status,
				"reason":             st.Reason,
				"message":            st.Message,
			})
			stdout, stderr, err := ExecAt(boot0, "kubectl", "delete", "-n", namespace, "certificates", name)
			if err != nil {
				return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
			}
			return errors.New("recreate certificate")
		}
	}
	return errors.New("certificate is not ready")
}
