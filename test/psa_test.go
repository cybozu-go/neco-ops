package test

import (
	_ "embed"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

//go:embed testdata/invalid-pod.yaml
var invalidPodYAML []byte

func testPodSecurityAdmission() {
	It("should prohibit pod that uses hostNetwork", func() {
		_, stderr, err := ExecAtWithInput(boot0, invalidPodYAML, "kubectl", "apply", "-f", "-")
		Expect(err).To(HaveOccurred())
		Expect(string(stderr)).Should(ContainSubstring(`admission webhook "baseline.vpod.kb.io" denied the request`))
	})
}
