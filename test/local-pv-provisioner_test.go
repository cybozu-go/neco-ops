package test

import (
	"encoding/json"
	"fmt"
	"sort"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

func testLocalPVProvisioner() {
	var ssNodes corev1.NodeList
	var ssNumber int

	It("should be deployed successfully", func() {
		By("getting SS Nodes")
		stdout, stderr, err := ExecAt(boot0, "kubectl", "get", "nodes", "--selector=cke.cybozu.com/role=ss", "-o", "json")
		Expect(err).ShouldNot(HaveOccurred(), "failed to get SS Nodes. stdout: %s, stderr: %s", stdout, stderr)

		err = json.Unmarshal(stdout, &ssNodes)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(ssNodes.Items).ShouldNot(HaveLen(0))
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
		stdout, stderr, err = ExecAt(boot0, "kubectl", "get", "pods", "--selector=app.kubernetes.io/name=local-pv-provisioner", "-n", "kube-system", "-o", "json")
		Expect(err).ShouldNot(HaveOccurred(), "failed to get a DaemonSet. stdout: %s, stderr: %s", stdout, stderr)

		var lppPods corev1.PodList
		err = json.Unmarshal(stdout, &lppPods)
		Expect(err).ShouldNot(HaveOccurred(), "failed to unmarshal JSON.")

		nodeNamesByPod := []string{}
		for _, lppPod := range lppPods.Items {
			nodeNamesByPod = append(nodeNamesByPod, lppPod.Spec.NodeName)
		}
		sort.Strings(nodeNamesByPod)

		nodeNames := []string{}
		for _, ssNode := range ssNodes.Items {
			nodeNames = append(nodeNames, ssNode.Name)
		}
		sort.Strings(nodeNames)

		Expect(nodeNamesByPod).Should(BeEquivalentTo(nodeNames))
	})

	It("should be created PV successfully", func() {
		var targetDevices = []string{
			"/dev/crypt-disk/by-path/pci-0000:00:0a.0",
			"/dev/crypt-disk/by-path/pci-0000:00:0b.0",
		}

		By("checking the number of local PVs")
		var targetPVList []corev1.PersistentVolume
		Eventually(func() error {
			stdout, stderr, err := ExecAt(boot0, "kubectl", "get", "pv", "-o", "json")
			if err != nil {
				return fmt.Errorf("failed to get PVs. stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
			}

			var pvs corev1.PersistentVolumeList
			err = json.Unmarshal(stdout, &pvs)
			if err != nil {
				return fmt.Errorf("failed to unmarshal JSON. err: %v", err)
			}

			for _, pv := range pvs.Items {
				if pv.Spec.StorageClassName == "local-storage" {
					targetPVList = append(targetPVList, pv)
				}
			}
			if len(targetPVList) != ssNumber*len(targetDevices) {
				return fmt.Errorf("the number of local PVs should be %d: %d", ssNumber*len(targetDevices), len(targetPVList))
			}

			return nil
		}).Should(Succeed())

		By("checking local PVs were created for each device on each node")
		expected := []string{}
		for _, ssNode := range ssNodes.Items {
			for _, dev := range targetDevices {
				expected = append(expected, ssNode.Name+":"+dev)
			}
		}
		sort.Strings(expected)

		actual := []string{}
		for _, pv := range targetPVList {
			ownerRefList := pv.GetOwnerReferences()
			Expect(ownerRefList).To(HaveLen(1), "local PV should have one owner reference. pv: %s, len(ownerReferences): %d", pv.Name, len(ownerRefList))
			ownerRef := ownerRefList[0]
			actual = append(actual, ownerRef.Name+":"+pv.Spec.PersistentVolumeSource.Local.Path)
		}
		sort.Strings(actual)

		Expect(actual).Should(BeEquivalentTo(expected))
	})
}
