package test

import (
	_ "embed"
	"encoding/json"
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	"sigs.k8s.io/yaml"
)

//go:embed testdata/elastic.yaml
var elasticYAML []byte

func prepareElastic() {
	It("should create Elasticsearch cluster", func() {
		_, stderr, err := ExecAtWithInput(boot0, elasticYAML, "kubectl", "apply", "-f", "-")
		Expect(err).NotTo(HaveOccurred(), "stderr: %s", stderr)
	})
}

func testElastic() {
	It("should deploy Elasticsearch cluster", func() {
		By("confirming elastic-operator is deployed")
		Eventually(func() error {
			stdout, _, err := ExecAt(boot0, "kubectl", "--namespace=elastic-system",
				"get", "statefulset/elastic-operator", "-o=json")
			if err != nil {
				return err
			}

			ss := new(appsv1.StatefulSet)
			err = json.Unmarshal(stdout, ss)
			if err != nil {
				return err
			}

			if ss.Status.ReadyReplicas != 1 {
				return fmt.Errorf("elastic-operator statefulset's ReadyReplica is not 1: %d", int(ss.Status.ReadyReplicas))
			}
			return nil
		}).Should(Succeed())

		By("waiting Elasticsearch resource health becomes green")
		Eventually(func() error {
			stdout, stderr, err := ExecAt(
				boot0,
				"kubectl", "-n", "dctest", "get", "elasticsearch/sample",
				"--template", "'{{ .status.health }}'",
			)
			if err != nil {
				return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
			}
			if string(stdout) != "green" {
				return fmt.Errorf("elastic resource health should be green: %s", stdout)
			}
			return nil
		}).Should(Succeed())

		By("accessing to elasticsearch")
		stdout, stderr, err := ExecAt(boot0,
			"kubectl", "get", "secret", "sample-es-elastic-user", "-n", "dctest", "-o=jsonpath='{.data.elastic}'",
			"|", "base64", "--decode")
		Expect(err).NotTo(HaveOccurred(), "stderr: %s", stderr)
		password := string(stdout)

		stdout, stderr, err = ExecAt(boot0, "ckecli", "cluster", "get")
		Expect(err).ShouldNot(HaveOccurred(), "stderr=%s", stderr)
		cluster := new(ckeCluster)
		err = yaml.Unmarshal(stdout, cluster)
		Expect(err).ShouldNot(HaveOccurred())
		workerAddr := cluster.Nodes[0].Address
		stdout, stderr, err = ExecAt(boot0,
			"ckecli", "ssh", "cybozu@"+workerAddr, "--",
			"curl", "-u", "elastic:"+password, "-k", "https://sample-es-http.dctest.svc:9200")
		Expect(err).NotTo(HaveOccurred(), "stdout: %s, stderr: %s", stdout, stderr)
	})
}
