package test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/creack/pty"
	"github.com/google/go-cmp/cmp"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	k8sYaml "k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/yaml"
)

type Node struct {
	Kind     string
	Metadata struct {
		Name string
	}
}

func testTeleport() {
	It("should deploy teleport node services", func() {
		By("retrieving LoadBalancer IP address of teleport auth service")
		var addr string
		Eventually(func() error {
			stdout, stderr, err := ExecAt(boot0, "kubectl", "-n", "teleport", "get", "service", "teleport-auth",
				"--output=jsonpath={.status.loadBalancer.ingress[0].ip}")
			if err != nil {
				return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
			}
			ret := strings.TrimSpace(string(stdout))
			if len(ret) == 0 {
				return errors.New("teleport auth IP address is empty")
			}
			addr = ret
			return nil
		}).Should(Succeed())

		By("storing LoadBalancer IP address to etcd")
		ExecSafeAt(boot0, "env", "ETCDCTL_API=3", "etcdctl", "--cert=/etc/etcd/backup.crt", "--key=/etc/etcd/backup.key",
			"put", "/neco/teleport/auth-servers", `[\"`+addr+`:3025\"]`)

		By("starting teleport node services on boot servers")
		for _, h := range []string{boot0, boot1, boot2} {
			ExecSafeAt(h, "sudo", "neco", "teleport", "config")
			ExecSafeAt(h, "sudo", "systemctl", "start", "teleport-node.service")
		}
	})

	It("should access to boot servers via teleport ssh", func() {
		By("adding proxy addr entry to /etc/hosts")
		stdout, stderr, err := ExecAt(boot0, "kubectl", "-n", "teleport", "get", "service", "teleport-proxy",
			"--output=jsonpath={.status.loadBalancer.ingress[0].ip}")
		Expect(err).ShouldNot(HaveOccurred(), "stderr=%s", stderr)
		addr := string(stdout)
		f, err := os.OpenFile("/etc/hosts", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		Expect(err).ShouldNot(HaveOccurred())
		_, err = f.Write([]byte(addr + " teleport.gcp0.dev-ne.co\n"))
		Expect(err).ShouldNot(HaveOccurred())
		f.Close()

		By("creating user")
		stdout, stderr, err = ExecAt(boot0, "kubectl", "-n", "teleport", "exec", "teleport-auth-0", "tctl", "users", "add", "cybozu", "cybozu,root")
		Expect(err).ShouldNot(HaveOccurred(), "stderr=%s", stderr)
		slashSplited := strings.Split(strings.Split(string(stdout), "\n")[1], "/")
		token := slashSplited[len(slashSplited)-1]
		payload, err := json.Marshal(map[string]string{
			"invite_token":        token,
			"pass":                "dummypass",
			"second_factor_token": "",
			"user":                "cybozu",
		})
		Expect(err).ShouldNot(HaveOccurred())
		cmd := exec.Command("curl", "--fail", "--insecure", "-H", "Content-Type: application/json; charset=UTF-8", "-d", string(payload), "https://teleport.gcp0.dev-ne.co/v1/webapi/users")
		output, err := cmd.CombinedOutput()
		Expect(err).ShouldNot(HaveOccurred(), "output=%s", output)

		By("logging in using tsh command")
		Eventually(func() error {
			cmd = exec.Command("tsh", "--insecure", "--proxy=teleport.gcp0.dev-ne.co:443", "--user=cybozu", "login")
			ptmx, err := pty.Start(cmd)
			if err != nil {
				return fmt.Errorf("pts.Start failed: %w", err)
			}
			defer ptmx.Close()
			_, err = ptmx.Write([]byte("dummypass\n"))
			if err != nil {
				return fmt.Errorf("ptmx.Write failed: %w", err)
			}
			go func() { io.Copy(os.Stdout, ptmx) }()
			return cmd.Wait()
		}).Should(Succeed())
		cmd = exec.Command("tsh", "--insecure", "--proxy=teleport.gcp0.dev-ne.co:443", "--user=cybozu", "login")
		ptmx, err := pty.Start(cmd)
		Expect(err).ShouldNot(HaveOccurred())
		defer ptmx.Close()
		_, err = ptmx.Write([]byte("dummypass\n"))
		Expect(err).ShouldNot(HaveOccurred())
		go func() { io.Copy(os.Stdout, ptmx) }()
		err = cmd.Wait()
		Expect(err).ShouldNot(HaveOccurred())

		By("accessing boot servers using tsh command")
		for _, n := range []string{"boot-0", "boot-1", "boot-2"} {
			Eventually(func() error {
				cmd := exec.Command("tsh", "--insecure", "--proxy=teleport.gcp0.dev-ne.co:443", "--user=cybozu", "ssh", "cybozu@"+n, "date")
				output, err := cmd.CombinedOutput()
				if err != nil {
					return fmt.Errorf("tsh ssh failed for %s: %s", n, string(output))
				}
				return nil
			}).Should(Succeed())
		}

		By("clearing /etc/hosts")
		output, err = exec.Command("sed", "-i", "-e", "/teleport.gcp0.dev-ne.co/d", "/etc/hosts").CombinedOutput()
		Expect(err).ShouldNot(HaveOccurred(), "output=%s", output)
	})

	It("should retain the state even after recreating the teleport-auth pod", func() {
		By("getting the node list before recreating the teleport-auth pod")
		stdout, stderr, err := ExecAt(boot0, "kubectl", "-n", "teleport", "exec", "teleport-auth-0", "tctl", "get", "nodes")
		Expect(err).ShouldNot(HaveOccurred(), "stderr=%s", stderr)
		beforeNodes := decodeNodes(stdout)

		By("recreating the teleport-auth pod")
		ExecSafeAt(boot0, "kubectl", "-n", "teleport", "delete", "pod", "teleport-auth-0")
		Eventually(func() error {
			stdout, stderr, err := ExecAt(boot0, "kubectl", "-n", "teleport", "exec", "teleport-auth-0", "tctl", "status")
			if err != nil {
				return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
			}
			return nil
		}).Should(Succeed())

		By("comparing the current node list with the obtained before")
		Eventually(func() error {
			stdout, stderr, err = ExecAt(boot0, "kubectl", "-n", "teleport", "exec", "teleport-auth-0", "tctl", "get", "nodes")
			if err != nil {
				return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
			}
			afterNodes := decodeNodes(stdout)
			if !cmp.Equal(afterNodes, beforeNodes) {
				return fmt.Errorf("before: %v, after: %v", beforeNodes, afterNodes)
			}
			return nil
		}).Should(Succeed())
	})
}

func decodeNodes(input []byte) []Node {
	r := bytes.NewReader(input)
	y := k8sYaml.NewYAMLReader(bufio.NewReader(r))

	var nodes []Node
	for {
		data, err := y.Read()
		if err == io.EOF {
			break
		} else if err != nil {
			return nil
		}

		var node Node
		err = yaml.Unmarshal(data, &node)
		if err != nil {
			return nil
		}
		nodes = append(nodes, node)
	}

	return nodes
}
