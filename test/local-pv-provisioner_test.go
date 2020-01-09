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

	It("should be deployed successfully", func() {
		By("getting SS Nodes")
		stdout, stderr, err := ExecAt(boot0, "kubectl", "get", "nodes", "--selector=cke.cybozu.com/role=cs", "-o", "json")
		Expect(err).ShouldNot(HaveOccurred(), "failed to get SS Nodes. stdout: %s, stderr: %s", stdout, stderr)

		err = json.Unmarshal(stdout, &ssNodes)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(ssNodes.Items).ShouldNot(HaveLen(0))
		ssNumber := len(ssNodes.Items)

		By("checking the number of available Pod by the state of DaemonSet")
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
			Expect(ds.Status.NumberAvailable).Should(Equal(int32(ssNumber)))
			return nil
		}).Should(Succeed())

		By("checking the Pods was assigned for Nodes")
		stdout, stderr, err = ExecAt(boot0, "kubectl", "get", "pods", "--selector=app.kubernetes.io/name=local-pv-provisioner", "-o", "json")
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
		// PVをすべて取得する
		// すべてのssにおいてPVが必要なだけ作られている
		// PVに過不足が無い
	})
}
