package test

import (
	"errors"
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func testOpenPolicyAgent() {
	It("should create test-opa namespace", func() {
		ExecSafeAt(boot0, "kubectl", "delete", "namespace", "test-opa", "--ignore-not-found=true")
		ExecSafeAt(boot0, "kubectl", "create", "namespace", "test-opa")
	})

	It("should validate Service resource according to policy", func() {
		By("registering policy")
		policyRego := `
package kubernetes.admission

operations = {"CREATE", "UPDATE"}

deny[msg] {
    input.request.kind.kind == "Service"
    operations[input.request.operation]
    input.request.namespace == "test-opa"
    input.request.object.metadata.name == "bad"
    msg := "Service test-opa/bad is prohibited"
}
`
		stdout, stderr, err := ExecAtWithInput(boot0, []byte(policyRego), "dd", "of=service-name.rego")
		Expect(err).NotTo(HaveOccurred(), "stdout: %s, stderr: %s", stdout, stderr)
		stdout, stderr, err = ExecAt(boot0, "kubectl", "-n", "opa", "create", "configmap", "service-name", "--from-file=service-name.rego")
		Expect(err).NotTo(HaveOccurred(), "stdout: %s, stderr: %s", stdout, stderr)
		stdout, stderr, err = ExecAt(boot0, "kubectl", "-n", "opa", "label", "configmap", "service-name", "openpolicyagent.org/policy=rego")
		Expect(err).NotTo(HaveOccurred(), "stdout: %s, stderr: %s", stdout, stderr)

		By("creating/updating bad Service")
		badServiceYAML := `
apiVersion: v1
kind: Service
metadata:
  name: bad
  namespace: test-opa
  labels:
    counter: "%d"
spec:
  selector:
    app.kubernetes.io/name: opa
  ports:
    - name: https
      protocol: TCP
      port: 443
      targetPort: 8443
`
		counter := 0
		Eventually(func() error {
			counter++
			bad := fmt.Sprintf(badServiceYAML, counter)
			stdout, stderr, err := ExecAtWithInput(boot0, []byte(bad), "kubectl", "apply", "-f", "-")
			if err == nil {
				return errors.New("creating/updating bad Service succeeded unexpectedly")
			}
			if !strings.Contains(string(stderr), "Service test-opa/bad is prohibited") {
				return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
			}
			return nil
		}, "10s", "1s").Should(Succeed())

		By("creating good Service")
		goodServiceYAML := `
apiVersion: v1
kind: Service
metadata:
  name: good
  namespace: test-opa
spec:
  selector:
    app.kubernetes.io/name: opa
  ports:
    - name: https
      protocol: TCP
      port: 443
      targetPort: 8443
`
		stdout, stderr, err = ExecAtWithInput(boot0, []byte(goodServiceYAML), "kubectl", "apply", "-f", "-")
		Expect(err).NotTo(HaveOccurred(), "stdout: %s, stderr: %s", stdout, stderr)

		By("unregistering policy")
		stdout, stderr, err = ExecAt(boot0, "kubectl", "-n", "opa", "delete", "configmap", "service-name")
		Expect(err).NotTo(HaveOccurred(), "stdout: %s, stderr: %s", stdout, stderr)

		By("creating/updating formerly-bad Service")
		Eventually(func() error {
			counter++
			bad := fmt.Sprintf(badServiceYAML, counter)
			stdout, stderr, err := ExecAtWithInput(boot0, []byte(bad), "kubectl", "apply", "-f", "-")
			if err != nil {
				return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
			}
			return nil
		}, "10s", "1s").Should(Succeed())

		By("trying to register policy from outside opa namespace")
		stdout, stderr, err = ExecAt(boot0, "kubectl", "-n", "test-opa", "create", "configmap", "service-name", "--from-file=service-name.rego")
		Expect(err).NotTo(HaveOccurred(), "stdout: %s, stderr: %s", stdout, stderr)
		stdout, stderr, err = ExecAt(boot0, "kubectl", "-n", "test-opa", "label", "configmap", "service-name", "openpolicyagent.org/policy=rego")
		Expect(err).NotTo(HaveOccurred(), "stdout: %s, stderr: %s", stdout, stderr)

		By("creating/updating formerly-bad Service")
		Eventually(func() error {
			counter++
			bad := fmt.Sprintf(badServiceYAML, counter)
			stdout, stderr, err := ExecAtWithInput(boot0, []byte(bad), "kubectl", "apply", "-f", "-")
			if err != nil {
				return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
			}
			return nil
		}, "10s", "1s").Should(Succeed())
	})
}
