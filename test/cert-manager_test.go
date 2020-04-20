package test

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/cybozu-go/log"
	certmanagerv1alpha2 "github.com/jetstack/cert-manager/pkg/apis/certmanager/v1alpha2"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
)

func testCertManager() {
	It("should be deployed successfully", func() {
		Eventually(func() error {
			stdout, _, err := ExecAt(boot0, "kubectl", "--namespace=cert-manager",
				"get", "deployment", "--selector=app.kubernetes.io/instance=cert-manager", "-o=json")
			if err != nil {
				return err
			}

			deploymentList := new(appsv1.DeploymentList)
			err = json.Unmarshal(stdout, deploymentList)
			if err != nil {
				return err
			}

			for _, deploy := range deploymentList.Items {
				if deploy.Status.AvailableReplicas != 1 {
					return fmt.Errorf("%s deployment's AvailableReplica is not 1: %d", deploy.Name, int(deploy.Status.AvailableReplicas))
				}
			}

			return nil
		}).Should(Succeed())
	})

	It("should issue certificate", func() {
		domainName := testID + "-cert-manager.gcp0.dev-ne.co"
		By("deploying Certificate")
		issuerName := "clouddns"
		if withKind {
			issuerName = "self-signed-issuer"
		}
		certificate := fmt.Sprintf(`
apiVersion: cert-manager.io/v1alpha2
kind: Certificate
metadata:
  name: test-certificate
  namespace: cert-manager
spec:
  secretName: example-com-tls
  issuerRef:
    kind: ClusterIssuer
    name: %s
  commonName: %s
  dnsNames:
    - %s
`, issuerName, domainName, domainName)
		_, stderr, err := ExecAtWithInput(boot0, []byte(certificate), "kubectl", "apply", "-f", "-")
		Expect(err).NotTo(HaveOccurred(), "stderr: %s", stderr)

		By("checking ClusterIssuer has registered")
		Eventually(func() error {
			stdout, stderr, err := ExecAt(boot0, "kubectl", "get", "clusterissuers", issuerName, "-o", "json")
			if err != nil {
				return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
			}

			var ci certmanagerv1alpha2.ClusterIssuer
			err = json.Unmarshal(stdout, &ci)
			if err != nil {
				return err
			}

			if len(ci.Status.Conditions) == 0 {
				return errors.New("status not found")
			}

			status := ci.Status.Conditions[0]
			if status.Status != "True" {
				return fmt.Errorf("Certificate status is not True: %s", status.Status)
			}
			desiredReason := "ACMEAccountRegistered"
			if withKind {
				desiredReason = "IsReady"
			}
			if status.Reason != desiredReason {
				return fmt.Errorf("ClusterIssuer reason not %s: %s", desiredReason, status.Reason)
			}

			return nil
		}).Should(Succeed())

		By("checking certificate is issued for xxx.gcp0.dev-ne.co")
		Eventually(func() error {
			stdout, stderr, err := ExecAt(boot0, "kubectl", "get", "-n=cert-manager", "certificate", "test-certificate", "-o", "json")
			if err != nil {
				return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
			}

			var cert certmanagerv1alpha2.Certificate
			err = json.Unmarshal(stdout, &cert)
			if err != nil {
				return err
			}

			for _, st := range cert.Status.Conditions {
				if st.Type != certmanagerv1alpha2.CertificateConditionReady {
					continue
				}

				if st.Status == "True" {
					return nil
				}
			}

			// Check the CertificateRequest status (the result of ACME challenge).
			// If the status is failed, recreate the Certificate and force to retry the ACME challenge.
			certReq, err := getCertificateRequest(cert)
			if err != nil {
				return err
			}
			for _, st := range certReq.Status.Conditions {
				if st.Type != certmanagerv1alpha2.CertificateRequestConditionReady {
					continue
				}
				// debug log
				log.Info("certificate request status", map[string]interface{}{"time": st.LastTransitionTime, "status": st.Status, "reason": st.Reason, "message": st.Message})

				if st.Reason == certmanagerv1alpha2.CertificateRequestReasonFailed {
					log.Error("CertificateRequest failed", map[string]interface{}{
						"certificate":        cert.Name,
						"certificaterequest": certReq.Name,
						"status":             st.Status,
						"reason":             st.Reason,
						"message":            st.Message,
					})
					err = recreateCertificate("test-certificate", "cert-manager", certificate)
					if err != nil {
						return err
					}
				}
			}
			return errors.New("certificate is not ready")
		}).Should(Succeed())

		By("checking certificate is issued for xxx.gcp0.dev-ne.co")
		_, _, err = ExecAt(boot0, "kubectl", "get", "-n=cert-manager", "secrets/example-com-tls")
		Expect(err).NotTo(HaveOccurred())
	})
}

func getCertificateRequest(cert certmanagerv1alpha2.Certificate) (*certmanagerv1alpha2.CertificateRequest, error) {
	var certReqList certmanagerv1alpha2.CertificateRequestList
	var targetCertReq *certmanagerv1alpha2.CertificateRequest

	stdout, stderr, err := ExecAt(boot0, "kubectl", "get", "-n=cert-manager", "certificaterequest", "-o", "json")
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

func recreateCertificate(name, namespace, certificate string) error {
	stdout, stderr, err := ExecAt(boot0, "kubectl", "delete", "-n", namespace, "certificate", name)
	if err != nil {
		return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
	}

	stdout, stderr, err = ExecAtWithInput(boot0, []byte(certificate), "kubectl", "apply", "-f", "-")
	if err != nil {
		return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
	}

	return nil
}
