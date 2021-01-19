package test

import (
	"encoding/json"
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func testAdmission() {
	It("should mutate pod to append emptyDir for /tmp", func() {
		podYAML := `apiVersion: v1
kind: Pod
metadata:
  name: pod-mutator-test
  namespace: default
spec:
  containers:
  - name: ubuntu
    image: quay.io/cybozu/ubuntu:20.04
    command: ["pause"]
`
		stdout, stderr, err := ExecAtWithInput(boot0, []byte(podYAML), "kubectl", "apply", "-f", "-")
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
		networkPolicyYAML := `
apiVersion: crd.projectcalico.org/v1
kind: NetworkPolicy
metadata:
  name: admission-test
  namespace: default
spec:
  order: 100.0
  selector: app.kubernetes.io/name == 'hoge'
  types:
  - Ingress
  ingress:
  - action: Allow
    protocol: TCP
    destination:
      ports:
      - 8000
`
		_, stderr, err := ExecAtWithInput(boot0, []byte(networkPolicyYAML), "kubectl", "apply", "-f", "-")
		Expect(err).To(HaveOccurred())
		Expect(string(stderr)).Should(ContainSubstring(`admission webhook "vnetworkpolicy.kb.io" denied the request`))
	})

	It("should default/validate Contour HTTPProxy", func() {
		httpProxyYAML := `
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: bad
  namespace: default
spec:
  virtualhost:
    fqdn: bad.test-admission.gcp0.dev-ne.co
  routes:
    - conditions:
        - prefix: /
      services:
        - name: dummy
          port: 80
`
		By("creating HTTPProxy without annotations")
		stdout, stderr, err := ExecAtWithInput(boot0, []byte(httpProxyYAML), "kubectl", "apply", "-f", "-")
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

		stdout, stderr, err = ExecAtWithInput(boot0, []byte(httpProxyYAML), "kubectl", "delete", "-f", "-")
		Expect(err).NotTo(HaveOccurred(), "stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
	})

	It("should validate Application", func() {
		applicationTmplYAML := `
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: %s
  namespace: default
spec:
  project: %s
  source:
    repoURL: %s
    targetRevision: master
    path: dummy/
  destination:
    server: https://kubernetes.default.svc
    namespace: default
`

		By("creating Application which points to neco-apps repo and belongs to default project")
		name := "valid"
		project := "default"
		repoURL := "https://github.com/cybozu-go/neco-apps.git"
		stdout, stderr, err := ExecAtWithInput(boot0, []byte(fmt.Sprintf(applicationTmplYAML, name, project, repoURL)),
			"kubectl", "apply", "-f", "-")
		Expect(err).NotTo(HaveOccurred(), "stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
		ExecSafeAt(boot0, "kubectl", "delete", "application", name)

		By("denying to create Application which points to maneki-apps repo and belongs to default project")
		name = "invalid"
		repoURL = "https://github.com/cybozu-private/maneki-apps.git"
		stdout, stderr, err = ExecAtWithInput(boot0, []byte(fmt.Sprintf(applicationTmplYAML, name, project, repoURL)),
			"kubectl", "apply", "-f", "-")
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
