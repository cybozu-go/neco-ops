package test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
)

func testLogging() {
	It("should be successful", func() {
		checkLog("should be get pod logs", `'{namespace="logging", pod="logging-loki-0"}'`)

		ssNodeName := getNodeName("ss")
		checkLog("should be get journal logs by ss", fmt.Sprintf(`'{job="systemd-journal", instance="%s"}'`, ssNodeName))

		csNodeName := getNodeName("cs")
		checkLog("should be get journal logs by cs", fmt.Sprintf(`'{job="systemd-journal", instance="%s"}'`, csNodeName))
	})
}

func checkLog(title, query string) {
	By(title, func() {
		stdout, stderr, err := ExecAt(boot0,
			"kubectl", "exec", "-n", "logging", "statefulset/logging-loki", "--", "logcli", "query", query, "-ojsonl")
		Expect(err).ShouldNot(HaveOccurred(), "stdout=%s, stderr=%s", stdout, stderr)

		scanner := bufio.NewScanner(bytes.NewBuffer(stdout))
		hasLog := false
		for scanner.Scan() {
			hasLog = true
			log := make(map[string]interface{})
			line := scanner.Bytes()
			err = json.Unmarshal(line, &log)
			Expect(err).ShouldNot(HaveOccurred(), "log=%s", string(line))
			Expect(log).Should(HaveKey("labels"))
			Expect(log).Should(HaveKey("line"))
		}
		Expect(hasLog).Should(BeTrue())
	})
}

func getNodeName(role string) string {
	stdout, stderr, err := ExecAt(boot0, "kubectl", "get", "node", "-l", fmt.Sprintf("node-role.kubernetes.io/%s=true", role), "-o=json")
	Expect(err).ShouldNot(HaveOccurred(), "stdout=%s, stderr=%s", stdout, stderr)

	nodes := new(corev1.NodeList)
	err = json.Unmarshal(stdout, nodes)
	Expect(err).ShouldNot(HaveOccurred())
	Expect(nodes.Items).ShouldNot(BeEmpty())

	return nodes.Items[0].Name
}
