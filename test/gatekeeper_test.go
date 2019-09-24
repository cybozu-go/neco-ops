package test

import (
	"encoding/json"
	"errors"
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func testGatekeeper() {
	It("should validate Service resource according to policy", func() {
		By("registering constraint template")
		constraintTemplete := `
apiVersion: templates.gatekeeper.sh/v1beta1
kind: ConstraintTemplate
metadata:
  name: k8srequiredlabels
spec:
  crd:
    spec:
      names:
        kind: K8sRequiredLabels
      validation:
        openAPIV3Schema:
          properties:
            labels:
              type: array
              items:
                type: string
  targets:
    - target: admission.k8s.gatekeeper.sh
      rego: |
        package k8srequiredlabels

        violation[{"msg": msg, "details": {"missing_labels": missing}}] {
          provided := {label | input.review.object.metadata.labels[label]}
          required := {label | label := input.parameters.labels[_]}
          missing := required - provided
          count(missing) > 0
          msg := sprintf("you must provide labels: %v", [missing])
        }
`
		Eventually(func() error {
			stdout, stderr, err := ExecAtWithInput(boot0, []byte(constraintTemplete), "kubectl", "apply", "-f", "-")
			if err != nil {
				return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
			}
			return nil
		}).Should(Succeed())

		By("registering constraint")
		constraint := `
apiVersion: constraints.gatekeeper.sh/v1beta1
kind: K8sRequiredLabels
metadata:
  name: ns-must-have-gk
spec:
  match:
    kinds:
      - apiGroups: [""]
        kinds: ["Namespace"]
  parameters:
    labels: ["gatekeeper"]
`
		Eventually(func() error {
			stdout, stderr, err := ExecAtWithInput(boot0, []byte(constraint), "kubectl", "apply", "-f", "-")
			if err != nil {
				return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
			}
			return nil
		}).Should(Succeed())

		By("waiting for constrains enforced")
		Eventually(func() error {
			stdout, stderr, err := ExecAtWithInput(boot0, []byte(constraint), "kubectl", "get", "k8srequiredlabels", "ns-must-have-gk", "-o", "json")
			if err != nil {
				return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
			}
			var constraint struct {
				Status struct {
					ByPod []struct {
						Enforced bool `json:"enforced"`
					} `json:"byPod"`
				} `json:"status"`
			}

			err = json.Unmarshal(stdout, &constraint)
			if err != nil {
				return fmt.Errorf("data: %s, err: %v", string(stdout), err)
			}
			if len(constraint.Status.ByPod) < 1 {
				return errors.New("byPod is empty")
			}
			if constraint.Status.ByPod[0].Enforced != true {
				return errors.New("not enforced")
			}
			return nil
		}).Should(Succeed())

		By("creating bad Namespace")
		badNSYAML := `
apiVersion: v1
kind: Namespace
metadata:
  name: bad
`
		stdout, stderr, err := ExecAtWithInput(boot0, []byte(badNSYAML), "kubectl", "apply", "-f", "-")
		Expect(err).To(HaveOccurred(), "stdout: %s, stderr: %s", stdout, stderr)

		By("creating good Namespace")
		goodNSYAML := `
apiVersion: v1
kind: Namespace
metadata:
  name: good
  labels:
    gatekeeper: defined
`
		stdout, stderr, err = ExecAtWithInput(boot0, []byte(goodNSYAML), "kubectl", "apply", "-f", "-")
		Expect(err).NotTo(HaveOccurred(), "stdout: %s, stderr: %s", stdout, stderr)

		By("unregistering policy")
		stdout, stderr, err = ExecAt(boot0, "kubectl", "delete", "K8sRequiredLabels", "ns-must-have-gk")
		Expect(err).NotTo(HaveOccurred(), "stdout: %s, stderr: %s", stdout, stderr)

		By("creating formerly-bad Namespace")
		stdout, stderr, err = ExecAtWithInput(boot0, []byte(badNSYAML), "kubectl", "apply", "-f", "-")
		Expect(err).NotTo(HaveOccurred(), "stdout: %s, stderr: %s", stdout, stderr)

		By("deleting Namespaces")
		stdout, stderr, err = ExecAt(boot0, "kubectl", "delete", "namespace", "bad")
		Expect(err).NotTo(HaveOccurred(), "stdout: %s, stderr: %s", stdout, stderr)
		stdout, stderr, err = ExecAt(boot0, "kubectl", "delete", "namespace", "good")
		Expect(err).NotTo(HaveOccurred(), "stdout: %s, stderr: %s", stdout, stderr)
	})
}
