package test

import (
	"encoding/json"
	"errors"
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/prometheus/common/model"
	corev1 "k8s.io/api/core/v1"
	policyv1beta1 "k8s.io/api/policy/v1beta1"
)

func prepareTopoLVM() {
	It("should prepare a Pod and a PVC", func() {
		manifest := `
apiVersion: v1
kind: Pod
metadata:
  name: topolvm-test
  namespace: sandbox
spec:
  priorityClassName: node-bound
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
  namespace: sandbox
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
		stdout, stderr, err := ExecAtWithInput(boot0, []byte(manifest), "kubectl", "apply", "-f", "-")
		Expect(err).ShouldNot(HaveOccurred(), "stdout=%s, stderr=%s", stdout, stderr)
	})
}

func testTopoLVM() {
	It("should work TopoLVM pod and auto-resizer", func() {
		By("checking PodDisruptionBudget for controller Deployment")
		Eventually(func() error {
			pdb := policyv1beta1.PodDisruptionBudget{}
			stdout, stderr, err := ExecAt(boot0, "kubectl", "get", "poddisruptionbudgets", "controller-pdb", "-n", "topolvm-system", "-o", "json")
			if err != nil {
				return fmt.Errorf("failed to get TopoLVM pdb: %s: %w", stderr, err)
			}

			if err := json.Unmarshal(stdout, &pdb); err != nil {
				return err
			}
			if pdb.Status.CurrentHealthy != 2 {
				return fmt.Errorf("too few healthy pods: %d", pdb.Status.CurrentHealthy)
			}
			return nil
		}).Should(Succeed())

		By("checking the test pod is running")
		Eventually(func() error {
			stdout, stderr, err := ExecAt(boot0, "kubectl", "get", "-n", "sandbox", "pods", "topolvm-test", "-o", "json")
			if err != nil {
				return fmt.Errorf("failed to get topolvm-test pod: %s: %w", stderr, err)
			}
			pod := &corev1.Pod{}
			if err := json.Unmarshal(stdout, pod); err != nil {
				return err
			}

			for _, cond := range pod.Status.Conditions {
				if cond.Type != corev1.PodReady {
					continue
				}
				if cond.Status == corev1.ConditionTrue {
					return nil
				}
			}
			return errors.New("topolvm-test pod is not ready")
		}).Should(Succeed())

		By("writing large file")
		ExecSafeAt(boot0, "kubectl", "exec", "-n", "sandbox", "topolvm-test", "--", "dd", "if=/dev/zero", "of=/test1/largefile", "bs=1M", "count=110")

		By("waiting for the PV getting resized")
		Eventually(func() error {
			stdout, stderr, err := ExecAt(boot0, "kubectl", "-n=monitoring", "exec", "vmselect-vmcluster-largeset-0", "-i", "--", "curl", "-sf", "http://localhost:8481/select/0/prometheus/api/v1/query?query=kubelet_volume_stats_capacity_bytes")
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

				if string(sample.Metric["namespace"]) != "sandbox" {
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
	})
}
