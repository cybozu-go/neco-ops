package test

import (
	"encoding/json"
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

func existTargetLocalPV(localPVs []corev1.PersistentVolume, nodename, path string) bool {
	for _, pv := range localPVs {
		if len(pv.OwnerReferences) != 1 {
			continue
		}
		owner := pv.OwnerReferences[0]
		if owner.Kind != "Node" || owner.Name != nodename {
			continue
		}
		if pv.Spec.Local.Path != path {
			continue
		}
		return true
	}
	return false
}

func getNodeIPFromPV(pv *corev1.PersistentVolume) (string, error) {
	Expect(pv.Spec.NodeAffinity.Required.NodeSelectorTerms).To(HaveLen(1))
	Expect(pv.Spec.NodeAffinity.Required.NodeSelectorTerms[0].MatchExpressions).To(HaveLen(1))
	Expect(pv.Spec.NodeAffinity.Required.NodeSelectorTerms[0].MatchExpressions[0].Key).Should(Equal("kubernetes.io/hostname"))
	return pv.Spec.NodeAffinity.Required.NodeSelectorTerms[0].MatchExpressions[0].Values[0], nil
}

func prepareLocalPVProvisioner() {
	ns := "test-local-pv-provisioner"
	It("should create test-local-pv-provisioner namespace", func() {
		ExecSafeAt(boot0, "kubectl", "delete", "namespace", ns, "--ignore-not-found=true")
		createNamespaceIfNotExists(ns)
		ExecSafeAt(boot0, "kubectl", "annotate", "namespaces", ns, "i-am-sure-to-delete="+ns)
	})

	It("should be used as block device", func() {
		By("deploying Pod with PVC")
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
    image: quay.io/cybozu/ubuntu:18.04
    command: ["/usr/local/bin/pause"]
    volumeDevices:
    - name: local-volume
      devicePath: /dev/local-dev
  volumes:
  - name: local-volume
    persistentVolumeClaim:
      claimName: local-pvc
  tolerations:
  - key: cke.cybozu.com/role
    operator: Equal
    value: storage
---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: local-pvc
spec:
  storageClassName: local-storage
  accessModes:
  - ReadWriteOnce
  volumeMode: Block
  resources:
    requests:
      storage: 1Gi
`
		stdout, stderr, err := ExecAtWithInput(boot0, []byte(manifest), "kubectl", "apply", "-n", ns, "-f", "-")
		Expect(err).ShouldNot(HaveOccurred(), "stdout=%s, stderr=%s", stdout, stderr)
	})
}

func testLocalPVProvisioner() {
	const cryptPartDir = "/dev/crypt-disk/by-path/"

	var ssNodes corev1.NodeList
	var ssNumber int
	var targetDeviceNum int
	var targetPVList []corev1.PersistentVolume

	ns := "test-local-pv-provisioner"

	It("should have created PV successfully", func() {
		By("confirming it has be successfully deployed")
		By("getting SS Nodes")
		stdout, stderr, err := ExecAt(boot0, "kubectl", "get", "nodes", "--selector=cke.cybozu.com/role=ss", "-o", "json")
		Expect(err).NotTo(HaveOccurred(), "failed to get SS Nodes. stdout: %s, stderr: %s", stdout, stderr)

		err = json.Unmarshal(stdout, &ssNodes)
		Expect(err).NotTo(HaveOccurred())
		Expect(ssNodes.Items).NotTo(HaveLen(0))
		ssNumber = len(ssNodes.Items)

		By("checking the number of available Pods by the state of DaemonSet")
		Eventually(func() error {
			stdout, stderr, err := ExecAt(boot0, "kubectl", "get", "ds", "local-pv-provisioner", "-n", "kube-system", "-o", "json")
			if err != nil {
				return fmt.Errorf("failed to get a DaemonSet. stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
			}

			var ds appsv1.DaemonSet
			err = json.Unmarshal(stdout, &ds)
			if err != nil {
				return fmt.Errorf("failed to unmarshal JSON. err: %v", err)
			}

			if ds.Status.NumberAvailable != int32(ssNumber) {
				return fmt.Errorf("available pods is not %d: %d", int32(ssNumber), ds.Status.NumberAvailable)
			}
			return nil
		}).Should(Succeed())

		By("checking the Pods were assigned for Nodes")
		for _, ssNode := range ssNodes.Items {
			By("checking the pod on " + ssNode.GetName())
			stdout, stderr, err := ExecAt(boot0, "kubectl", "get", "pods", "--selector=app.kubernetes.io/name=local-pv-provisioner", "--field-selector=spec.nodeName=="+ssNode.GetName(), "-n", "kube-system", "-o", "json")
			Expect(err).NotTo(HaveOccurred(), "failed to get a DaemonSet. stdout: %s, stderr: %s", stdout, stderr)

			var lppPods corev1.PodList
			err = json.Unmarshal(stdout, &lppPods)
			Expect(err).NotTo(HaveOccurred(), "failed to unmarshal JSON")
			Expect(lppPods.Items).To(HaveLen(1))
		}

		By("getting local PVs")
		stdout, stderr, err = ExecAt(boot0, "kubectl", "get", "pv", "-o", "json")
		Expect(err).NotTo(HaveOccurred(), "failed to get PVs. stdout: %s, stderr: %s", stdout, stderr)

		var pvs corev1.PersistentVolumeList
		err = json.Unmarshal(stdout, &pvs)
		Expect(err).NotTo(HaveOccurred(), "failed to unmarshal JSON")

		for _, pv := range pvs.Items {
			if pv.Spec.StorageClassName == "local-storage" {
				targetPVList = append(targetPVList, pv)
			}
		}

		By("checking local PVs were created for each device on each node")
		for _, ssNode := range ssNodes.Items {
			By("checking target device files on " + ssNode.GetName())
			ssNodeIP := ssNode.GetName()
			stdout, stderr, err := ExecAt(boot0, "ckecli", "ssh", "cybozu@"+ssNodeIP, "ls", cryptPartDir)
			Expect(err).NotTo(HaveOccurred(), "stdout=%s, stderr=%s", stdout, stderr)
			devices := strings.Fields(strings.TrimSpace(string(stdout)))

			for _, dev := range devices {
				path := cryptPartDir + dev
				By("checking the existence of local PV for " + path)
				Expect(existTargetLocalPV(targetPVList, ssNodeIP, path)).To(BeTrue())
			}

			targetDeviceNum += len(devices)
		}

		By("checking the number of local PVs")
		Expect(targetPVList).To(HaveLen(targetDeviceNum))

		By("waiting to be able to execute a command")
		Eventually(func() error {
			stdout, stderr, err := ExecAt(boot0, "kubectl", "exec", "-n", ns, "ubuntu", "--", "date")
			if err != nil {
				return fmt.Errorf("failed to execute a command. stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
			}

			return nil
		}).Should(Succeed())

		By("confirming that can make filesystem for the block device")
		stdout, stderr, err = ExecAt(boot0, "kubectl", "exec", "-n", ns, "ubuntu", "--", "mkfs.ext4", "-F", "/dev/local-dev")
		Expect(err).ShouldNot(HaveOccurred(), "stdout=%s, stderr=%s", stdout, stderr)

		By("cleaning up")
		By("getting used local PV")
		stdout = ExecSafeAt(boot0, "kubectl", "get", "pvc", "local-pvc", "-n", ns, "-o", "json")

		pvc := new(corev1.PersistentVolumeClaim)
		err = json.Unmarshal(stdout, pvc)
		Expect(err).ShouldNot(HaveOccurred())
		usedPVName := pvc.Spec.VolumeName

		By("deleting test resources")
		ExecSafeAt(boot0, "kubectl", "delete", "namespace", ns)

		var pv corev1.PersistentVolume
		By("waiting used local PV will be recreated")
		Eventually(func() error {
			stdout, stderr, err := ExecAt(boot0, "kubectl", "get", "pv", usedPVName, "-o", "json")
			if err != nil {
				return fmt.Errorf("failed to get PVs. stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
			}

			err = json.Unmarshal(stdout, &pv)
			if err != nil {
				return fmt.Errorf("failed to unmarshal JSON. err: %v", err)
			}

			if pv.Status.Phase != corev1.VolumeAvailable {
				return fmt.Errorf("local PVs status should be %s: %s", corev1.VolumeAvailable, pv.Status.Phase)
			}

			return nil
		}).Should(Succeed())

		By("confirming that the recreated volume was wiped out")
		ssNodeIP, err := getNodeIPFromPV(&pv)
		Expect(err).ShouldNot(HaveOccurred())
		// read ext4 super block. ref: https://ext4.wiki.kernel.org/index.php/Ext4_Disk_Layout#Layout
		stdout, stderr, err = ExecAt(boot0, "ckecli", "ssh", "cybozu@"+ssNodeIP, "sudo", "dd", "if="+pv.Spec.Local.Path, "bs=1024", "skip=1", "count=4")
		Expect(err).NotTo(HaveOccurred(), "stderr=%s", stderr)
		Expect(stdout).Should(Equal(make([]byte, 4096)))
	})
}
