package test

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

//go:embed testdata/hpa.yaml
var hpaYAML []byte

func prepareHPA() {
	It("should prepare resources for HPA tests", func() {
		_, stderr, err := ExecAtWithInput(boot0, hpaYAML, "kubectl", "apply", "-f", "-")
		Expect(err).ShouldNot(HaveOccurred(), "stderr=%s", stderr)
	})
}

func testHPA() {
	It("should work for standard resources (CPU)", func() {
		Eventually(func() error {
			stdout, stderr, err := ExecAt(boot0, "kubectl", "-n", "dctest", "get", "deployments", "hpa-resource", "-o", "json")
			if err != nil {
				return fmt.Errorf("failed to get hpa-resource deployment: %s: %w", stderr, err)
			}
			dpl := &appsv1.Deployment{}
			if err := json.Unmarshal(stdout, dpl); err != nil {
				return err
			}
			if dpl.Spec.Replicas == nil || *dpl.Spec.Replicas != 2 {
				return errors.New("replicas of hpa-resource deployment is not 2")
			}
			return nil
		}).Should(Succeed())

		ExecSafeAt(boot0, "kubectl", "-n", "dctest", "delete", "deployments", "hpa-resource")
	})

	It("should work for custom resources provided by prometheus-adapter", func() {
		By("waiting for the test Pod to be created")
		var pod *corev1.Pod
		Eventually(func() error {
			pods := &corev1.PodList{}
			stdout, stderr, err := ExecAt(boot0, "kubectl", "-n", "dctest", "get", "pods", "-l", "run=hpa-custom", "-o", "json")
			if err != nil {
				return fmt.Errorf("failed to get pod list: %s: %w", stderr, err)
			}
			if err := json.Unmarshal(stdout, pods); err != nil {
				return err
			}
			if len(pods.Items) != 1 {
				return errors.New("no hpa-custom pods")
			}
			pod = &pods.Items[0]
			return nil
		}).Should(Succeed())

		metric := fmt.Sprintf(`test_hpa_requests_per_second{namespace="dctest",pod="%s"} 20`, pod.Name) + "\n"
		url := fmt.Sprintf("http://%s/metrics/job/some_job", bastionPushgatewayFQDN)

		By("checking the number of replicas increases")
		Eventually(func() error {
			ip, err := getLoadBalancerIP("ingress-bastion", "envoy")
			if err != nil {
				return err
			}
			_, stderr, err := ExecInNetnsWithInput("external", []byte(metric), "curl", "--resolve", bastionPushgatewayFQDN+":80:"+ip, "-sf", "--data-binary", "@-", url)
			if err != nil {
				return fmt.Errorf("failed to push a metrics to pushgateway: %s: %w", stderr, err)
			}
			stdout, stderr, err := ExecAt(boot0, "kubectl", "-n", "dctest", "get", "deployments", "hpa-custom", "-o", "json")
			if err != nil {
				return fmt.Errorf("failed to get hpa-custom deployment: %s: %w", stderr, err)
			}
			dpl := &appsv1.Deployment{}
			if err := json.Unmarshal(stdout, dpl); err != nil {
				return err
			}
			if dpl.Spec.Replicas == nil || *dpl.Spec.Replicas != 2 {
				return errors.New("replicas of hpa-custom is not 2")
			}
			return nil
		}).Should(Succeed())

		ExecSafeAt(boot0, "kubectl", "-n", "dctest", "delete", "deployments", "hpa-custom")
	})

	It("should work for external resources provided by kube-metrics-adapter", func() {
		metric := "test_hpa_external 23\n"
		url := fmt.Sprintf("http://%s/metrics/job/some_job", bastionPushgatewayFQDN)
		Eventually(func() error {
			ip, err := getLoadBalancerIP("ingress-bastion", "envoy")
			if err != nil {
				return err
			}
			_, stderr, err := ExecInNetnsWithInput("external", []byte(metric), "curl", "--resolve", bastionPushgatewayFQDN+":80:"+ip, "-sf", "--data-binary", "@-", url, "-m", "5")
			if err != nil {
				return fmt.Errorf("failed to push a metrics to pushgateway: %s: %w", stderr, err)
			}
			stdout, stderr, err := ExecAt(boot0, "kubectl", "-n", "dctest", "get", "deployments", "hpa-external", "-o", "json")
			if err != nil {
				return fmt.Errorf("failed to get hpa-external deployment: %s: %w", stderr, err)
			}
			dpl := &appsv1.Deployment{}
			if err := json.Unmarshal(stdout, dpl); err != nil {
				return err
			}
			if dpl.Spec.Replicas == nil || *dpl.Spec.Replicas != 3 {
				return errors.New("replicas of hpa-external is not 3")
			}
			return nil
		}).Should(Succeed())

		ExecSafeAt(boot0, "kubectl", "-n", "dctest", "delete", "deployments", "hpa-external")
	})
}
