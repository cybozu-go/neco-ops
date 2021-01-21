package test

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/prometheus/common/model"
	policyv1beta1 "k8s.io/api/policy/v1beta1"
)

var topolvmNS = "test-topolvm"

func prepareTopoLVM() {
	It("should create  create a Pod and a PVC", func() {
		By("creating test-topolvm namespace")
		ExecSafeAt(boot0, "kubectl", "delete", "namespace", topolvmNS, "--ignore-not-found=true")
		createNamespaceIfNotExists(topolvmNS)
		ExecSafeAt(boot0, "kubectl", "annotate", "namespaces", topolvmNS, "i-am-sure-to-delete="+topolvmNS)

		By("creating Pod and a PVC")
		manifest := `
apiVersion: v1
kind: Pod
metadata:
  name: ubuntu
  labels:
    app.kubernetes.io/name: ubuntu
spec:
  containers:
  - name: ubuntu
    image: quay.io/cybozu/ubuntu:20.04
    command: ["/usr/local/bin/pause"]
    volumeMounts:
    - name: my-volume
      mountPath: /test1
  volumes:
  - name: my-volume
    persistentVolumeClaim:
      claimName: topo-pvc
---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: topo-pvc
  annotations:
    resize.topolvm.io/threshold: 90%
    resize.topolvm.io/increase: 1Gi
spec:
  accessModes:
  - ReadWriteOnce
  resources:
    requests:
      storage: 1Gi
    limits:
      storage: 3Gi
  storageClassName: topolvm-provisioner
`
		stdout, stderr, err := ExecAtWithInput(boot0, []byte(manifest), "kubectl", "apply", "-n", topolvmNS, "-f", "-")
		Expect(err).ShouldNot(HaveOccurred(), "stdout=%s, stderr=%s", stdout, stderr)
	})
}

func TopoLVMPodTest() {
	By("checking PodDisruptionBudget for controller Deployment")
	pdb := policyv1beta1.PodDisruptionBudget{}
	stdout, stderr, err := ExecAt(boot0, "kubectl", "get", "poddisruptionbudgets", "controller-pdb", "-n", "topolvm-system", "-o", "json")
	Expect(err).ShouldNot(HaveOccurred(), "stdout=%s, stderr=%s", stdout, stderr)

	err = json.Unmarshal(stdout, &pdb)
	Expect(err).ShouldNot(HaveOccurred())
	Expect(pdb.Status.CurrentHealthy).Should(Equal(int32(2)))

	By("confirming that the specified volume exists in the Pod")
	Eventually(func() error {
		stdout, stderr, err := ExecAt(boot0, "kubectl", "exec", "-n", topolvmNS, "ubuntu", "--", "mountpoint", "-d", "/test1")
		if err != nil {
			return fmt.Errorf("failed to check mount point. stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
		}

		stdout, stderr, err = ExecAt(boot0, "kubectl", "exec", "-n", topolvmNS, "ubuntu", "grep", "/test1", "/proc/mounts")
		if err != nil {
			return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
		}
		fields := strings.Fields(string(stdout))
		if len(fields) < 3 {
			return errors.New("invalid mount information: " + string(stdout))
		}
		if fields[2] != "xfs" {
			return errors.New("/test1 is not xfs")
		}
		return nil
	}).Should(Succeed())

	By("writing file under /test1")
	writePath := "/test1/bootstrap.log"
	stdout, stderr, err = ExecAt(boot0, "kubectl", "exec", "-n", topolvmNS, "ubuntu", "--", "cp", "/etc/passwd", writePath)
	Expect(err).ShouldNot(HaveOccurred(), "stdout=%s, stderr=%s", stdout, stderr)
	stdout, stderr, err = ExecAt(boot0, "kubectl", "exec", "-n", topolvmNS, "ubuntu", "--", "sync")
	Expect(err).ShouldNot(HaveOccurred(), "stdout=%s, stderr=%s", stdout, stderr)
	stdout, stderr, err = ExecAt(boot0, "kubectl", "exec", "-n", topolvmNS, "ubuntu", "--", "cat", writePath)
	Expect(err).ShouldNot(HaveOccurred(), "stdout=%s, stderr=%s", stdout, stderr)
	Expect(strings.TrimSpace(string(stdout))).ShouldNot(BeEmpty())

	// skip reboot of node temporarily due to ckecli or Kubernetes issue

	// By("getting node name where pod is placed")
	// stdout, stderr, err = ExecAt(boot0, "kubectl", "-n", topolvmNS, "get", "pods/ubuntu", "-o", "json")
	// Expect(err).ShouldNot(HaveOccurred(), "stdout=%s, stderr=%s", stdout, stderr)
	// var pod corev1.Pod
	// err = json.Unmarshal(stdout, &pod)
	// Expect(err).ShouldNot(HaveOccurred(), "stdout=%s", stdout)
	// nodeName := pod.Spec.NodeName

	// By("rebooting the node")
	// ExecSafeAt(boot0, "ckecli", "sabakan", "disable")
	// stdout, stderr, err = ExecAt(boot0, "neco", "ipmipower", "restart", nodeName)
	// Expect(err).ShouldNot(HaveOccurred(), "stdout=%s, stderr=%s", stdout, stderr)
	// time.Sleep(5 * time.Second)

	// By("confirming that the file survives")
	// Eventually(func() error {
	// 	stdout, stderr, err = ExecAt(boot0, "kubectl", "exec", "-n", topolvmNS, "ubuntu", "--", "cat", writePath)
	// 	if err != nil {
	// 		return fmt.Errorf("failed to cat. stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
	// 	}
	// 	if len(strings.TrimSpace(string(stdout))) == 0 {
	// 		return errors.New(writePath + " is empty")
	// 	}
	// 	return nil
	// }).Should(Succeed())
}

func pvcAutoresizerTest() {
	By("writing large file")
	ExecSafeAt(boot0, "kubectl", "exec", "-n", topolvmNS, "ubuntu", "--", "dd", "if=/dev/zero", "of=/test1/largefile", "bs=1M", "count=110")

	By("waiting for the PV getting resized")
	Eventually(func() error {
		stdout, stderr, err := ExecAt(boot0, "kubectl", "-n=monitoring", "exec", "prometheus-0", "-i", "--", "curl", "-sf", "http://localhost:9090/api/v1/query?query=kubelet_volume_stats_capacity_bytes")
		if err != nil {
			return fmt.Errorf("stderr=%s: %w", string(stderr), err)
		}

		result := struct {
			Data struct {
				Result model.Vector `json:"result"`
			} `json:"data"`
		}{}
		err = json.Unmarshal(stdout, &result)
		if err != nil {
			return err
		}

		for _, sample := range result.Data.Result {
			if sample.Metric == nil {
				continue
			}

			if string(sample.Metric["namespace"]) != topolvmNS {
				continue
			}
			if string(sample.Metric["persistentvolumeclaim"]) != "topo-pvc" {
				continue
			}
			if sample.Value > (1 << 30) {
				return nil
			}

			return fmt.Errorf("filesystem capacity is under < 1 GiB: %f", float64(sample.Value))
		}

		return fmt.Errorf("no metric for PVC")
	}).Should(Succeed())
}

func testTopoLVM() {
	It("should work TopoLVM pod and auto-resizer", func() {
		By("TopoLVMPodTest", TopoLVMPodTest)
		By("pvcAutoresizerTest", pvcAutoresizerTest)
	})
}
