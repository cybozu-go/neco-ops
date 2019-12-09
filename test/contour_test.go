package test

import (
	"encoding/json"
	"errors"
	"fmt"

	certmanagerv1alpha2 "github.com/jetstack/cert-manager/pkg/apis/certmanager/v1alpha2"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1beta1 "k8s.io/api/policy/v1beta1"
)

func testContour() {
	It("should create test-ingress namespace", func() {
		ExecSafeAt(boot0, "kubectl", "delete", "namespace", "test-ingress", "--ignore-not-found=true")
		ExecSafeAt(boot0, "kubectl", "create", "namespace", "test-ingress")
	})

	It("should be deployed successfully", func() {
		Eventually(func() error {
			stdout, stderr, err := ExecAt(boot0, "kubectl", "--namespace=ingress",
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
				return fmt.Errorf("contour deployment's AvailableReplica is not 2: %d", int(deployment.Status.AvailableReplicas))
			}
			return nil
		}).Should(Succeed())
	})

	It("should deploy HTTPProxy", func() {
		By("deployment Pods")
		deployYAML := `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: testhttpd
  namespace: test-ingress
spec:
  replicas: 2
  selector:
    matchLabels:
      app.kubernetes.io/name: testhttpd
  template:
    metadata:
      labels:
        app.kubernetes.io/name: testhttpd
    spec:
      containers:
      - image: quay.io/cybozu/testhttpd:0
        name: testhttpd
      restartPolicy: Always
---
apiVersion: v1
kind: Service
metadata:
  name: testhttpd
  namespace: test-ingress
spec:
  ports:
  - port: 80
    protocol: TCP
    targetPort: 8000
  selector:
    app.kubernetes.io/name: testhttpd
---
apiVersion: crd.projectcalico.org/v1
kind: NetworkPolicy
metadata:
  name: ingress-httpdtest
  namespace: test-ingress
spec:
  order: 2000.0
  selector: app.kubernetes.io/name == 'testhttpd'
  types:
    - Ingress
  ingress:
    - action: Allow
      protocol: TCP
      destination:
        ports:
          - 8000
`
		_, stderr, err := ExecAtWithInput(boot0, []byte(deployYAML), "kubectl", "apply", "-f", "-")
		Expect(err).NotTo(HaveOccurred(), "stderr: %s", stderr)

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
		pdb := policyv1beta1.PodDisruptionBudget{}
		stdout, stderr, err := ExecAt(boot0, "kubectl", "get", "poddisruptionbudgets", "contour-pdb", "-n", "ingress", "-o", "json")
		if err != nil {
			Expect(err).ShouldNot(HaveOccurred(), "stdout=%s, stderr=%s", stdout, stderr)
		}
		err = json.Unmarshal(stdout, &pdb)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(pdb.Status.CurrentHealthy).Should(Equal(int32(2)))

		By("checking PodDisruptionBudget for envoy Deployment")
		stdout, stderr, err = ExecAt(boot0, "kubectl", "get", "poddisruptionbudgets", "envoy-pdb", "-n", "ingress", "-o", "json")
		if err != nil {
			Expect(err).ShouldNot(HaveOccurred(), "stdout=%s, stderr=%s", stdout, stderr)
		}
		err = json.Unmarshal(stdout, &pdb)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(pdb.Status.CurrentHealthy).Should(Equal(int32(3)))

		By("creating HTTPProxy")
		fqdnHTTP := testID + "-http.test-ingress.gcp0.dev-ne.co"
		fqdnHTTPS := testID + "-https.test-ingress.gcp0.dev-ne.co"
		ingressRoute := fmt.Sprintf(`
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: tls
  namespace: test-ingress
  annotations:
    kubernetes.io/tls-acme: "true"
spec:
  virtualhost:
    fqdn: %s
    tls:
      secretName: testsecret
  routes:
    - conditions:
        - prefix: /
      services:
        - name: testhttpd
          port: 80
    - conditions:
        - prefix: /insecure
      permitInsecure: true
      services:
        - name: testhttpd
          port: 80
---
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: root
  namespace: test-ingress
spec:
  virtualhost:
    fqdn: %s
  routes:
    - conditions:
        - prefix: /testhttpd
      services:
        - name: testhttpd
          port: 80
`, fqdnHTTPS, fqdnHTTP)
		_, stderr, err = ExecAtWithInput(boot0, []byte(ingressRoute), "kubectl", "apply", "-f", "-")
		Expect(err).NotTo(HaveOccurred(), "stderr: %s", stderr)

		By("getting contour service")
		var targetIP string
		Eventually(func() error {
			stdout, _, err := ExecAt(boot0, "kubectl", "get", "-n", "ingress", "service/contour-global", "-o", "json")
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

		if !withKind {
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
				stdout, stderr, err := ExecAt(boot0, "kubectl", "get", "-n", "test-ingress", "certificate", "tls", "-o", "json")
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
					if st.Reason != "Ready" {
						failed, err := isCertificateRequestFailed(cert)
						if err != nil {
							return err
						}
						if failed {
							ExecAt(boot0, "kubectl", "delete", "-n", "test-ingress", "certificate", "tls", "-o", "json")
							if err != nil {
								return err
							}
							return fmt.Errorf("recreate Certificate")
						}
						return fmt.Errorf("Certificate is not ready")
					}
					return nil
				}
				return errors.New("certificate is not ready")
			})
		}

		By("accessing with curl: http")
		Eventually(func() error {
			_, _, err := ExecAt(boot0, "curl", "--resolve", fqdnHTTP+":80:"+targetIP,
				"http://"+fqdnHTTP+"/testhttpd", "-m", "5", "--fail")
			return err
		}).Should(Succeed())

		if !withKind {
			By("accessing with curl: https")
			ExecSafeAt(boot0, "HTTPS_PROXY=http://10.0.49.3:3128",
				"curl", "-sfL", "-o", "lets.crt", "https://letsencrypt.org/certs/fakelerootx1.pem")
			Eventually(func() error {
				_, _, err := ExecAt(boot0, "curl", "--resolve", fqdnHTTPS+":443:"+targetIP,
					"https://"+fqdnHTTPS+"/",
					"-m", "5",
					"--fail",
					"--cacert", "lets.crt",
				)
				return err
			}).Should(Succeed())
		}

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
	})
}
