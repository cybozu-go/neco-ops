package test

import (
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

var (
	HNCTestNamespace     = "hnc-test-1"
	HNCTestSubnamespaces = []string{"dev-foo1", "dev-foo2"}
)

func prepareHNC() {
	roleBindingYAMLTemplate := `
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: tenant-role-binding
  namespace: %s
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: admin
subjects:
  - apiGroup: rbac.authorization.k8s.io
    kind: Group
    name: tenant
`

	It("should create namespace", func() {
		stdout, stderr, err := ExecAt(boot0, "kubectl", "create", "namespace", HNCTestNamespace)
		Expect(err).ShouldNot(HaveOccurred(), "stdout=%s, stderr=%s", stdout, stderr)
	})

	It("should deploy rolebinding", func() {
		stdout, stderr, err := ExecAtWithInput(boot0,
			[]byte(fmt.Sprintf(roleBindingYAMLTemplate, HNCTestNamespace)),
			"kubectl", "apply", "-f", "-")
		Expect(err).ShouldNot(HaveOccurred(), "stdout=%s, stderr=%s", stdout, stderr)
	})
}

func createSubnamespaces() {
	subnamespaceAnchorYAMLtemplate := `
apiVersion: hnc.x-k8s.io/v1alpha2
kind: SubnamespaceAnchor
metadata:
  name: %s
  namespace: %s
`

	By("creating subnamespace with subnamespaceanchor")
	stdout, stderr, err := ExecAtWithInput(boot0,
		[]byte(fmt.Sprintf(subnamespaceAnchorYAMLtemplate,
			HNCTestSubnamespaces[0], HNCTestNamespace)),
		"kubectl", "apply", "--as-group=tenant", "--as-group=system:authenticated", "--as=tenant", "-f", "-")
	Expect(err).ShouldNot(HaveOccurred(), "stdout=%s, stderr=%s", stdout, stderr)

	By("checking subnamespace is OK")
	Eventually(func() error {
		stdout, stderr, err = ExecAt(boot0, "kubectl", "get", "ns", HNCTestSubnamespaces[0])
		if err != nil {
			return fmt.Errorf("Failed to find subnamespace %s, stdout: %s, stderr: %s", HNCTestSubnamespaces[0], stdout, stderr)
		}
		return nil
	}).Should(Succeed())
	Eventually(func() error {
		stdout, stderr, err = ExecAt(boot0, "kubectl", "get", "subnamespaceanchors.hnc.x-k8s.io", "-n", HNCTestNamespace, HNCTestSubnamespaces[0], "-o", "jsonpath='{.status.status}'")
		if err != nil {
			return fmt.Errorf("Failed to get subnamespaceanchor of %s, stdout: %s, stderr: %s", HNCTestSubnamespaces[0], stdout, stderr)
		}
		if string(stdout) != "Ok" {
			return fmt.Errorf("subnamespaceanchor of %s status is not Ok, stdout: %s, stderr: %s", HNCTestSubnamespaces[0], stdout, stderr)
		}
		return nil
	}).Should(Succeed())

	By("creating subnamespace with kubectl-hns")
	stdout, stderr, err = ExecAt(boot0,
		"kubectl", "hns", "--as-group=tenant", "--as-group=system:authenticated",
		"--as=tenant", "create", HNCTestSubnamespaces[1], "-n", HNCTestNamespace)
	Expect(err).ShouldNot(HaveOccurred(), "stdout=%s, stderr=%s", stdout, stderr)

	By("checking subnamespace is OK")
	Eventually(func() error {
		stdout, stderr, err = ExecAt(boot0, "kubectl", "get", "ns", HNCTestSubnamespaces[1])
		if err != nil {
			return fmt.Errorf("Failed to find subnamespace %s, stdout: %s, stderr: %s", HNCTestSubnamespaces[1], stdout, stderr)
		}
		return nil
	}).Should(Succeed())
	Eventually(func() error {
		stdout, stderr, err = ExecAt(boot0, "kubectl", "get", "subnamespaceanchors.hnc.x-k8s.io", "-n", HNCTestNamespace, HNCTestSubnamespaces[1], "-o", "jsonpath='{.status.status}'")
		if err != nil {
			return fmt.Errorf("Failed to get subnamespaceanchor of %s, stdout: %s, stderr: %s", HNCTestSubnamespaces[1], stdout, stderr)
		}
		if string(stdout) != "Ok" {
			return fmt.Errorf("subnamespaceanchor of %s status is not Ok, stdout: %s, stderr: %s", HNCTestSubnamespaces[1], stdout, stderr)
		}
		return nil
	}).Should(Succeed())
}

func checkPropagation() {
	By("checking the existance of role binding in subnamespaces")
	Eventually(func() error {
		for _, namespace := range HNCTestSubnamespaces {
			stdout, stderr, err := ExecAt(boot0,
				"kubectl", "get", "--as-group=tenant", "--as-group=system:authenticated",
				"--as=tenant", "rolebindings", "tenant-role-binding", "-n", namespace)
			if err != nil {
				return fmt.Errorf("failed to get the propageted role binding. stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
			}
		}
		return nil
	}).Should(Succeed())
}

func deleteSubnamespace() {
	By("deleting subnamespaceanchor")
	stdout, stderr, err := ExecAt(boot0,
		"kubectl", "delete", "--as-group=tenant", "--as-group=system:authenticated",
		"--as=tenant", "subnamespaceanchors.hnc.x-k8s.io", HNCTestSubnamespaces[1], "-n", HNCTestNamespace)
	Expect(err).ShouldNot(HaveOccurred(), "stdout=%s, stderr=%s", stdout, stderr)
	stdout, stderr, err = ExecAt(boot0,
		"kubectl", "delete", "--as-group=tenant", "--as-group=system:authenticated", "--as=tenant", "subnamespaceanchors.hnc.x-k8s.io", HNCTestSubnamespaces[1], "-n", HNCTestNamespace)
	Expect(err).Should(HaveOccurred(), "stdout=%s, stderr=%s", stdout, stderr)
	Expect(apierrors.IsNotFound(err)).Should(BeTrue())

	By("checking subnamespace is deleted")
	stdout, stderr, err = ExecAt(boot0, "kubectl", "get", HNCTestSubnamespaces[1])
	Expect(err).Should(HaveOccurred(), "stdout=%s, stderr=%s", stdout, stderr)
	Expect(apierrors.IsNotFound(err)).Should(BeTrue())
}

func testHNC() {
	It("should test HNC", func() {
		By("creating subnamespaces", createSubnamespaces)
		By("checking propagation of role binding", checkPropagation)
		By("deleting subnamespace", deleteSubnamespace)
	})
}
