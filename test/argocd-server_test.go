package test

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strconv"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
)

func testArgoCDServer() {
	It("should create the LoadBalancer service for argocd-server", func() {
		var lbIP string
		var lbPort string
		var password string

		By("confirming that argocd-server service has external IP")
		Eventually(func() error {
			stdout, _, err := ExecAt(boot0, "kubectl", "--namespace=argocd",
				"get", "service/argocd-server", "-o=json")
			if err != nil {
				return err
			}

			svc := new(corev1.Service)
			err = json.Unmarshal(stdout, svc)
			if err != nil {
				return err
			}

			if len(svc.Status.LoadBalancer.Ingress) != 1 {
				return errors.New("argocd-server service should have external ip")
			}
			lbIP = svc.Status.LoadBalancer.Ingress[0].IP
			if ip := net.ParseIP(lbIP); ip == nil {
				return fmt.Errorf("invalid ip: %s", lbIP)
			}

			for _, port := range svc.Spec.Ports {
				if port.Name != "http" {
					continue
				}
				lbPort = strconv.Itoa(int(port.Port))
			}
			if lbPort == "" {
				return errors.New("invalid port")
			}
			return nil
		}).Should(Succeed())

		By("fetching the password")
		Eventually(func() error {
			stdout, stderr, err := ExecAt(boot0, "kubectl", "get", "pods", "-n", "argocd",
				"-l", "app.kubernetes.io/name=argocd-server", "-o", "json")
			if err != nil {
				return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
			}
			var podList corev1.PodList
			err = json.Unmarshal(stdout, &podList)
			if err != nil {
				return err
			}
			if podList.Items == nil {
				return errors.New("podList.Items is nil")
			}
			if len(podList.Items) != 1 {
				return fmt.Errorf("podList.Items is not 1: %d", len(podList.Items))
			}

			password = podList.Items[0].Name
			return nil
		}).Should(Succeed())

		By("logging in to Argo CD via external IP")
		Eventually(func() error {
			stdout, stderr, err := ExecAt(boot0, "argocd", "login", lbIP+":"+lbPort,
				"--insecure", "--username", "admin", "--password", password)
			if err != nil {
				return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
			}
			return nil
		}).Should(Succeed())
	})
}
