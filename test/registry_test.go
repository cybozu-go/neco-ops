package test

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

const podsYaml = `
apiVersion: v1
kind: Pod
metadata:
  name: elastic-elasticsearch
  namespace: test-registry
spec:
  containers:
  - command:
    - /usr/local/bin/pause
    image: docker.elastic.co/elasticsearch/elasticsearch-oss:7.9.3
    imagePullPolicy: Always
    name: elasticsearch
  securityContext:
    runAsGroup: 1000
    runAsUser: 1000
---
apiVersion: v1
kind: Pod
metadata:
  name: ghcr-moco
  namespace: test-registry
spec:
  containers:
  - command:
    - /usr/local/bin/pause
    image: ghcr.io/cybozu-go/moco:0.3.1
    imagePullPolicy: Always
    name: moco
---
apiVersion: v1
kind: Pod
metadata:
  name: quay-ubuntu
  namespace: test-registry
spec:
  containers:
  - command:
    - /usr/local/bin/pause
    image: quay.io/cybozu/ubuntu-debug:20.04
    imagePullPolicy: Always
    name: ubuntu
  securityContext:
    runAsGroup: 1000
    runAsUser: 1000
---
apiVersion: v1
kind: Pod
metadata:
  name: quay-private-testhttpd
  namespace: test-registry
spec:
  containers:
  - command:
    image: quay.io/neco_test/testhttpd:0.1.2
    imagePullPolicy: Always
    name: testhttpd
`

func prepareRegistry() {
	It("should prepare resources", func() {
		By("creating pods")
		createNamespaceIfNotExists("test-registry")
		_, stderr, err := ExecAtWithInput(boot0, []byte(podsYaml), "kubectl", "apply", "-f", "-")
		Expect(err).NotTo(HaveOccurred(), "stderr: %s", stderr)
	})
}

func testRegistry() {
	It("should cache containers on mirror registries", func() {
		type catalog struct {
			Repositories []string `json:"repositories"`
		}

		Eventually(func() error {
			By("checking docker.elastic.co")
			stdout, stderr, err := ExecAt(boot0, "kubectl", "-ntest-registry", "exec", "quay-ubuntu", "--", "curl", "-sf", "http://registry-elastic.registry:5000/v2/_catalog")
			if err != nil {
				return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
			}
			elasticCatalog := catalog{}
			err = json.Unmarshal(stdout, &elasticCatalog)
			if err != nil {
				return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
			}
			cached := false
			for _, repo := range elasticCatalog.Repositories {
				if repo == "elasticsearch/elasticsearch-oss" {
					cached = true
					break
				}
			}
			if !cached {
				stdout, stderr, err = ExecAt(boot0, "kubectl", "delete", "-ntest-registry", "pod", "elastic-elasticsearch")
				if err != nil {
					return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
				}
				stdout, stderr, err := ExecAtWithInput(boot0, []byte(podsYaml), "kubectl", "apply", "-f", "-")
				if err != nil {
					return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
				}
				return errors.New("elasticsearch-oss is not found in elastic registry")
			}
			return nil
		}, 10*time.Minute).Should(Succeed())

		By("checking ghcr.io")
		Eventually(func() error {
			stdout, stderr, err := ExecAt(boot0, "kubectl", "-ntest-registry", "exec", "quay-ubuntu", "--", "curl", "-sf", "http://registry-ghcr.registry:5000/v2/_catalog")
			if err != nil {
				return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
			}
			ghcrCatalog := catalog{}
			err = json.Unmarshal(stdout, &ghcrCatalog)
			if err != nil {
				return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
			}
			cached := false
			for _, repo := range ghcrCatalog.Repositories {
				if repo == "cybozu-go/moco" {
					cached = true
					break
				}
			}
			if !cached {
				stdout, stderr, err = ExecAt(boot0, "kubectl", "delete", "-ntest-registry", "pod", "ghcr-moco")
				if err != nil {
					return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
				}
				stdout, stderr, err := ExecAtWithInput(boot0, []byte(podsYaml), "kubectl", "apply", "-f", "-")
				if err != nil {
					return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
				}
				return errors.New("cybozu-go/moco is not found in ghcr registry")
			}

			return nil
		}, 10*time.Minute).Should(Succeed())

		By("checking quay.io")
		Eventually(func() error {
			stdout, stderr, err := ExecAt(boot0, "kubectl", "-ntest-registry", "exec", "quay-ubuntu", "--", "curl", "-sf", "http://registry-quay.registry:5000/v2/_catalog")
			if err != nil {
				return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
			}
			quayCatalog := catalog{}
			err = json.Unmarshal(stdout, &quayCatalog)
			if err != nil {
				return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
			}

			cached := false
			for _, repo := range quayCatalog.Repositories {
				if repo == "cybozu/ubuntu-debug" {
					cached = true
					break
				}
			}
			if !cached {
				stdout, stderr, err = ExecAt(boot0, "kubectl", "delete", "-ntest-registry", "pod", "ghcr-moco")
				if err != nil {
					return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
				}
				stdout, stderr, err := ExecAtWithInput(boot0, []byte(podsYaml), "kubectl", "apply", "-f", "-")
				if err != nil {
					return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
				}
				return errors.New("cybozu/ubuntu is not found in quay registry")
			}

			cached = false
			for _, repo := range quayCatalog.Repositories {
				if repo == "neco_test/testhttpd" {
					cached = true
					break
				}
			}
			if !cached {
				stdout, stderr, err = ExecAt(boot0, "kubectl", "delete", "-ntest-registry", "pod", "quay-private-testhttpd")
				if err != nil {
					return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
				}
				stdout, stderr, err := ExecAtWithInput(boot0, []byte(podsYaml), "kubectl", "apply", "-f", "-")
				if err != nil {
					return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
				}
				return errors.New("neco_test/testhttpd is not found in quay registry")
			}
			return nil
		}, 10*time.Minute).Should(Succeed())
	})
}
