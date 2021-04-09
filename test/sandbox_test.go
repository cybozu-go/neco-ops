package test

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"text/template"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
)

var sandboxGrafanaFQDN = testID + "-sandbox-grafana.gcp0.dev-ne.co"

//go:embed testdata/sandbox.yaml
var sandboxYAML string

func prepareSandboxGrafanaIngress() {
	It("should create HTTPProxy for Sandbox Grafana", func() {
		tmpl := template.Must(template.New("").Parse(sandboxYAML))
		buf := new(bytes.Buffer)
		err := tmpl.Execute(buf, testID)
		Expect(err).NotTo(HaveOccurred())
		_, stderr, err := ExecAtWithInput(boot0, buf.Bytes(), "kubectl", "apply", "-f", "-")
		Expect(err).NotTo(HaveOccurred(), "stderr: %s", stderr)
	})
}

func testSandboxGrafana() {
	It("should have data sources and dashboards", func() {
		By("confirming grafana is deployed successfully")
		Eventually(func() error {
			stdout, _, err := ExecAt(boot0, "kubectl", "--namespace=sandbox",
				"get", "statefulset/grafana", "-o=json")
			if err != nil {
				return err
			}
			statefulSet := new(appsv1.StatefulSet)
			err = json.Unmarshal(stdout, statefulSet)
			if err != nil {
				return err
			}

			if int(statefulSet.Status.ReadyReplicas) != 1 {
				return fmt.Errorf("ReadyReplicas is not 1: %d", int(statefulSet.Status.ReadyReplicas))
			}
			return nil
		}).Should(Succeed())

		By("confirming created Certificate")
		Eventually(func() error {
			return checkCertificate("grafana-test", "sandbox")
		}).Should(Succeed())

		By("getting admin stats from grafana")
		Eventually(func() error {
			ip, err := getLoadBalancerIP("ingress-bastion", "envoy")
			if err != nil {
				return err
			}
			stdout, stderr, err := ExecInNetns("external", "curl", "--resolve", sandboxGrafanaFQDN+":443:"+ip, "-kL", "-u", "admin:AUJUl1K2xgeqwMdZ3XlEFc1QhgEQItODMNzJwQme", "https://"+sandboxGrafanaFQDN+"/api/admin/stats")
			if err != nil {
				return fmt.Errorf("unable to get admin stats, stderr: %s, err: %v", stderr, err)
			}
			var adminStats struct {
				Dashboards  int `json:"dashboards"`
				Datasources int `json:"datasources"`
			}
			err = json.Unmarshal(stdout, &adminStats)
			if err != nil {
				return err
			}
			if adminStats.Datasources == 0 {
				return fmt.Errorf("no data sources")
			}
			if adminStats.Dashboards != 0 {
				return fmt.Errorf("%d dashboards exist", adminStats.Dashboards)
			}
			return nil
		}).Should(Succeed())
	})
}
