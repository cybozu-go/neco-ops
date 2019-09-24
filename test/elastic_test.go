package test

import (
	"encoding/json"
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/yaml"
)

func testElastic() {
	It("should create test-ingress namespace", func() {
		ExecSafeAt(boot0, "kubectl", "delete", "namespace", "test-es", "--ignore-not-found=true")
		ExecSafeAt(boot0, "kubectl", "create", "namespace", "test-es")
	})

	It("should be deployed successfully", func() {
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
	})
	It("should deploy Elasticsearch cluster", func() {
		elasticYAML := `apiVersion: elasticsearch.k8s.elastic.co/v1alpha1
kind: Elasticsearch
metadata:
  name: sample
  namespace: test-es
spec:
  version: 7.1.0
  # it avoids sysctl command by initContainers under PSP
  setVmMaxMapCount: false
  nodes:
  - nodeCount: 1
    config:
      node.master: true
      node.data: true
      node.ingest: true
    volumeClaimTemplates:
    - metadata:
        name: elasticsearch-data
      spec:
        accessModes:
        - ReadWriteOnce
        resources:
          requests:
            storage: 1Gi
        storageClassName: topolvm-provisioner
    podTemplate:
      spec:
        containers:
          - name: elasticsearch
            env:
              - name: ES_JAVA_OPTS
                value: "-Xms1g -Xmx1g"
            resources:
              limits:
                memory: 2Gi
              requests:
                memory: 2Gi
---
apiVersion: crd.projectcalico.org/v1
kind: NetworkPolicy
metadata:
  name: ingress-sample
  namespace: test-es
spec:
  order: 2000.0
  selector: elasticsearch.k8s.elastic.co/cluster-name == "sample"
  types:
    - Ingress
  ingress:
    - action: Allow
      protocol: TCP
      destination:
        ports:
          - 9200:9400
`
		_, stderr, err := ExecAtWithInput(boot0, []byte(elasticYAML), "kubectl", "apply", "-f", "-")
		Expect(err).NotTo(HaveOccurred(), "stderr: %s", stderr)

		By("waiting Elasticsearch resource health becomes green")
		Eventually(func() error {
			stdout, stderr, err := ExecAt(
				boot0,
				"kubectl", "-n", "test-es", "get", "elasticsearch/sample",
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
			"kubectl", "get", "secret", "sample-es-elastic-user", "-n", "test-es", "-o=jsonpath='{.data.elastic}'",
			"|", "base64", "--decode")
		Expect(err).NotTo(HaveOccurred(), "stderr: %s", stderr)
		password := string(stdout)

		if withKind {
			stdout, stderr, err = ExecAt(boot0, "kubectl", "-n", "test-es", "get", "svc", "sample-es-http", "-o", "json")
			Expect(err).NotTo(HaveOccurred(), "stdout: %s, stderr: %s", stdout, stderr)
			svc := new(corev1.Service)
			err = json.Unmarshal(stdout, svc)
			Expect(err).NotTo(HaveOccurred(), "stdout: %s", stdout)
			stdout, stderr, err = ExecAt(boot0,
				"docker", "exec", "-i", "kindtest-worker", "curl", "-u", "elastic:"+password, "-k", "https://"+svc.Spec.ClusterIP+":9200")
			Expect(err).NotTo(HaveOccurred(), "stdout: %s, stderr: %s", stdout, stderr)
		} else {
			stdout, stderr, err = ExecAt(boot0, "ckecli", "cluster", "get")
			Expect(err).ShouldNot(HaveOccurred(), "stderr=%s", stderr)
			cluster := new(ckeCluster)
			err = yaml.Unmarshal(stdout, cluster)
			Expect(err).ShouldNot(HaveOccurred())
			workerAddr := cluster.Nodes[0].Address
			stdout, stderr, err = ExecAt(boot0,
				"ckecli", "ssh", "cybozu@"+workerAddr, "--",
				"curl", "-u", "elastic:"+password, "-k", "https://sample-es-http.test-es.svc.cluster.local:9200")
			Expect(err).NotTo(HaveOccurred(), "stdout: %s, stderr: %s", stdout, stderr)
		}
	})
}
