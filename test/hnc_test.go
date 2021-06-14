package test

import (
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var (
	HNCTestNamespaces    = []string{"hnc-test-1", "hnc-test-2", "hnc-test-3"}
	HNCTestSubnamespaces = []string{"hnc-test-1-sub1", "hnc-test-1-sub2"}
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
		for _, namespace := range HNCTestNamespaces {
			stdout, stderr, err := ExecAt(boot0, "kubectl", "create", "namespace", namespace)
			Expect(err).ShouldNot(HaveOccurred(), "stdout=%s, stderr=%s", stdout, stderr)
		}
	})

	It("should deploy service account and rolebinding", func() {
		for _, namespace := range HNCTestNamespaces {
			stdout, stderr, err := ExecAtWithInput(boot0,
				[]byte(fmt.Sprintf(roleBindingYAMLTemplate, namespace)),
				"kubectl", "apply", "-f", "-")
			Expect(err).ShouldNot(HaveOccurred(), "stdout=%s, stderr=%s", stdout, stderr)
		}
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
			HNCTestSubnamespaces[0], "hnc-test-1")),
		"kubectl", "apply", "--as-group=tenant", "--as=tenant", "-f", "-")
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
		stdout, stderr, err = ExecAt(boot0, "kubectl", "get", "subnamespaceanchors.hnc.x-k8s.io", "-n", "hnc-test-1", HNCTestSubnamespaces[0], "-o", "jsonpath='{.status.status}'")
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
		"kubectl", "hns", "--as-group=tenant", "--as=tenant",
		"create", HNCTestSubnamespaces[1], "-n", "hnc-test-1")
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
		stdout, stderr, err = ExecAt(boot0, "kubectl", "get", "subnamespaceanchors.hnc.x-k8s.io", "-n", "hnc-test-1", HNCTestSubnamespaces[1], "-o", "jsonpath='{.status.status}'")
		if err != nil {
			return fmt.Errorf("Failed to get subnamespaceanchor of %s, stdout: %s, stderr: %s", HNCTestSubnamespaces[1], stdout, stderr)
		}
		if string(stdout) != "Ok" {
			return fmt.Errorf("subnamespaceanchor of %s status is not Ok, stdout: %s, stderr: %s", HNCTestSubnamespaces[1], stdout, stderr)
		}
		return nil
	}).Should(Succeed())
}

func createChildNamespaces() {
	hierarchyConfigurationYAMLTemplate := `
apiVersion: hnc.x-k8s.io/v1alpha2
kind: HierarchyConfiguration
metadata:
  name: hierarchy
  namespace: %s
spec:
  parent: %s
`

	By("moving namespace with HierarchyConfiguration")
	stdout, stderr, err := ExecAtWithInput(boot0,
		[]byte(fmt.Sprintf(hierarchyConfigurationYAMLTemplate,
			HNCTestNamespaces[1], HNCTestNamespaces[0])),
		"kubectl", "apply", "--as-group=tenant", "--as=tenant", "-f", "-")
	Expect(err).ShouldNot(HaveOccurred(), "stdout=%s, stderr=%s", stdout, stderr)

	By("moving namespace with kubectl-hns")
	stdout, stderr, err = ExecAt(boot0,
		"kubectl", "hns", "--as-group=tenant", "--as=tenant",
		"set", HNCTestNamespaces[2], "--parent", HNCTestNamespaces[0])
	Expect(err).ShouldNot(HaveOccurred(), "stdout=%s, stderr=%s", stdout, stderr)

	By("checking status of these namespaces")
	stdout, stderr, err = ExecAt(boot0,
		"kubectl", "get", "hierarchyconfigurations.hnc.x-k8s.io", "hierarchy",
		"-n", HNCTestNamespaces[0], "-o", "jsonpath='{.status.children}'")
	Expect(err).ShouldNot(HaveOccurred(), "stdout=%s, stderr=%s", stdout, stderr)
	Eventually(func() error {
		for _, namespace := range HNCTestNamespaces[1:] {
			if strings.Contains(string(stdout), namespace) {
				return fmt.Errorf("Failed to move %s, stdout: %s, stderr: %s", namespace, stdout, stderr)
			}
		}
		return nil
	}).Should(Succeed())
}

func checkPropagation() {
	By("checking the existance of role binding in subnamespaces")
	for _, namespace := range HNCTestSubnamespaces {
		stdout, stderr, err := ExecAt(boot0,
			"kubectl", "get", "--as-group=tenant", "--as=tenant",
			"rolebindings", "tenant-role-binding", "-n", namespace)
		Expect(err).ShouldNot(HaveOccurred(), "stdout=%s, stderr=%s", stdout, stderr)
	}

	By("checking the existance of role binding in child namespaces")
	for _, namespace := range HNCTestNamespaces[1:] {
		stdout, stderr, err := ExecAt(boot0,
			"kubectl", "get", "--as-group=tenant", "--as=tenant",
			"rolebindings", "tenant-role-binding", "-n", namespace)
		Expect(err).ShouldNot(HaveOccurred(), "stdout=%s, stderr=%s", stdout, stderr)
	}
}

func deleteSubnamespaceAnchor() {
	// TODO
}

func testHNC() {
	It("should test HNC", func() {
		By("creating subnamespaces", createSubnamespaces)
		By("creating child namespace", createChildNamespaces)
		By("checking propagation of role binding", checkPropagation)
		By("deleting subnamespaceanchor", deleteSubnamespaceAnchor)
	})
}