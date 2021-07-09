package test

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"strings"
	"text/template"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

//go:embed testdata/admission-pod.yaml
var admissionPodYAML []byte

//go:embed testdata/admission-networkpolicy.yaml
var admissionNetworkPolicyYAML []byte

//go:embed testdata/admission-httpproxy.yaml
var admissionHTTPProxyYAML []byte

//go:embed testdata/admission-application.yaml
var admissionApplicationYAML string

func testAdmission() {
	It("should mutate pod to append emptyDir for /tmp", func() {
		stdout, stderr, err := ExecAtWithInput(boot0, admissionPodYAML, "kubectl", "apply", "-f", "-")
		Expect(err).NotTo(HaveOccurred(), "stdout: %s, stderr: %s, err: %v", stdout, stderr, err)

		By("confirming that a emptyDir is added")
		stdout, stderr, err = ExecAt(boot0, "kubectl", "get", "pod", "pod-mutator-test", "-o", "json")
		Expect(err).NotTo(HaveOccurred(), "stdout: %s, stderr: %s, err: %v", stdout, stderr, err)

		po := new(corev1.Pod)
		err = json.Unmarshal(stdout, po)
		Expect(err).NotTo(HaveOccurred())

		found := false
		for _, vol := range po.Spec.Volumes {
			if !strings.HasPrefix(vol.Name, "tmp-") {
				continue
			}
			found = true
			Expect(vol.VolumeSource).Should(Equal(corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}))
		}
		Expect(found).Should(BeTrue())
	})

	It("should validate Calico NetworkPolicy", func() {
		_, stderr, err := ExecAtWithInput(boot0, admissionNetworkPolicyYAML, "kubectl", "apply", "-f", "-")
		Expect(err).To(HaveOccurred())
		Expect(string(stderr)).Should(ContainSubstring(`admission webhook "vnetworkpolicy.kb.io" denied the request`))
	})

	It("should default/validate Contour HTTPProxy", func() {
		By("creating HTTPProxy without annotations")
		stdout, stderr, err := ExecAtWithInput(boot0, admissionHTTPProxyYAML, "kubectl", "apply", "-f", "-")
		Expect(err).NotTo(HaveOccurred(), "stdout: %s, stderr: %s, err: %v", stdout, stderr, err)

		stdout, stderr, err = ExecAt(boot0, "kubectl", "get", "-n", "default", "httpproxy/bad", "-o", "json")
		Expect(err).NotTo(HaveOccurred(), "stdout: %s, stderr: %s, err: %v", stdout, stderr, err)

		hp := &unstructured.Unstructured{}
		err = json.Unmarshal(stdout, hp)
		Expect(err).NotTo(HaveOccurred(), "stdout: %s, err: %v", stdout, err)
		Expect(hp.GetAnnotations()).To(HaveKeyWithValue("kubernetes.io/ingress.class", "forest"))

		By("updating HTTPProxy to remove annotations")
		stdout, stderr, err = ExecAt(boot0, "kubectl", "annotate", "-n", "default", "httpproxy/bad", "kubernetes.io/ingress.class-")
		Expect(err).To(HaveOccurred(), "stdout: %s, stderr: %s", stdout, stderr)

		stdout, stderr, err = ExecAtWithInput(boot0, admissionHTTPProxyYAML, "kubectl", "delete", "-f", "-")
		Expect(err).NotTo(HaveOccurred(), "stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
	})

	It("should validate Application", func() {
		By("creating Application which points to neco-apps repo and belongs to default project")
		tmpl := template.Must(template.New("").Parse(admissionApplicationYAML))
		type tmplParams struct {
			Name    string
			Project string
			URL     string
		}
		buf := new(bytes.Buffer)
		err := tmpl.Execute(buf, tmplParams{"valid", "default", "https://github.com/cybozu-go/neco-apps.git"})
		Expect(err).NotTo(HaveOccurred())
		stdout, stderr, err := ExecAtWithInput(boot0, buf.Bytes(), "kubectl", "apply", "-f", "-")
		Expect(err).NotTo(HaveOccurred(), "stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
		ExecSafeAt(boot0, "kubectl", "delete", "application", "valid")

		By("denying to create Application which points to invalid repo and belongs to default project")
		buf.Reset()
		err = tmpl.Execute(buf, tmplParams{"invalid", "default", "https://github.com/cybozu-go/invalid-apps.git"})
		Expect(err).NotTo(HaveOccurred())
		stdout, stderr, err = ExecAtWithInput(boot0, buf.Bytes(), "kubectl", "apply", "-f", "-")
		Expect(err).To(HaveOccurred(), "stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
	})

	It("should validate deletion", func() {
		By("trying to delete a namespace")
		_, _, err := ExecAt(boot0, "kubectl", "delete", "namespace", "internet-egress")
		Expect(err).Should(HaveOccurred())

		By("trying to delete a CRD")
		_, _, err = ExecAt(boot0, "kubectl", "delete", "crd", "applications.argoproj.io")
		Expect(err).Should(HaveOccurred())

		By("trying to delete a CephCluster")
		_, _, err = ExecAt(boot0, "kubectl", "delete", "-n", "ceph-hdd", "cephclusters.ceph.rook.io", "ceph-hdd")
		Expect(err).Should(HaveOccurred())
	})
}
