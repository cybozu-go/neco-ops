package test

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

//go:embed testdata/metallb.yaml
var metallbYAML []byte

func prepareMetalLB() {
	It("should deploy load balancer type service", func() {
		By("creating deployments and service")
		_, stderr, err := ExecAtWithInput(boot0, metallbYAML, "kubectl", "create", "-f", "-")
		Expect(err).NotTo(HaveOccurred(), "stderr: %s", stderr)
	})
}

func testMetalLB() {
	It("should be deployed successfully", func() {
		Eventually(func() error {
			stdout, _, err := ExecAt(boot0, "kubectl", "--namespace=metallb-system",
				"get", "daemonsets/speaker", "-o=json")
			if err != nil {
				return err
			}
			ds := new(appsv1.DaemonSet)
			err = json.Unmarshal(stdout, ds)
			if err != nil {
				return err
			}

			if ds.Status.DesiredNumberScheduled <= 0 {
				return errors.New("speaker daemonset's desiredNumberScheduled is not updated")
			}

			if ds.Status.DesiredNumberScheduled != ds.Status.NumberAvailable {
				return fmt.Errorf("not all nodes running speaker daemonset: %d", ds.Status.NumberAvailable)
			}
			return nil
		}).Should(Succeed())

		Eventually(func() error {
			stdout, _, err := ExecAt(boot0, "kubectl", "--namespace=metallb-system",
				"get", "deployments/controller", "-o=json")
			if err != nil {
				return err
			}
			deployment := new(appsv1.Deployment)
			err = json.Unmarshal(stdout, deployment)
			if err != nil {
				return err
			}

			if int(deployment.Status.AvailableReplicas) != 1 {
				return fmt.Errorf("AvailableReplicas is not 1: %d", int(deployment.Status.AvailableReplicas))
			}
			return nil
		}).Should(Succeed())

		By("waiting pods are ready")
		Eventually(func() error {
			stdout, _, err := ExecAt(boot0, "kubectl", "get", "deployments/testhttpd", "-o", "json")
			if err != nil {
				return err
			}

			deployment := new(appsv1.Deployment)
			err = json.Unmarshal(stdout, deployment)
			if err != nil {
				return err
			}

			if deployment.Status.ReadyReplicas != 2 {
				return errors.New("ReadyReplicas is not 2")
			}
			return nil
		}).Should(Succeed())
	})

	It("should work", func() {
		By("waiting service are ready")
		var targetIP string
		Eventually(func() error {
			stdout, _, err := ExecAt(boot0, "kubectl", "get", "service/testhttpd", "-o", "json")
			if err != nil {
				return err
			}

			service := new(corev1.Service)
			err = json.Unmarshal(stdout, service)
			if err != nil {
				return err
			}

			if len(service.Status.LoadBalancer.Ingress) == 0 {
				return errors.New("LoadBalancer status is not updated")
			}

			targetIP = service.Status.LoadBalancer.Ingress[0].IP
			return nil
		}).Should(Succeed())

		By("access service from boot-0")
		Eventually(func() error {
			_, _, err := ExecAt(boot0, "curl", targetIP, "-m", "5")
			return err
		}).Should(Succeed())

		By("access service from external")
		Eventually(func() error {
			return exec.Command("ip", "netns", "exec", "external", "curl", targetIP, "-m", "5").Run()
		}).Should(Succeed())
	})
}
