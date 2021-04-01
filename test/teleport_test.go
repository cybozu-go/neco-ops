package test

import (
	"bufio"
	"bytes"
	"encoding/base64"
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
	appsv1 "k8s.io/api/apps/v1"
	k8sYaml "k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/yaml"
)

type Node struct {
	Kind     string
	Metadata struct {
		Name string
	}
}

func prepareTeleport() {
	It("should add proxy addr entry to /etc/hosts", func() {
		stdout, stderr, err := ExecAt(boot0, "kubectl", "-n", "teleport", "get", "service", "teleport-proxy",
			"--output=jsonpath={.status.loadBalancer.ingress[0].ip}")
		Expect(err).ShouldNot(HaveOccurred(), "stderr=%s", stderr)
		addr := string(stdout)
		// Save a backup before editing /etc/hosts
		b, err := os.ReadFile("/etc/hosts")
		Expect(err).NotTo(HaveOccurred())
		Expect(os.WriteFile("./hosts", b, 0644)).NotTo(HaveOccurred())
		f, err := os.OpenFile("/etc/hosts", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		Expect(err).ShouldNot(HaveOccurred())
		_, err = f.Write([]byte(addr + " teleport.gcp0.dev-ne.co\n"))
		Expect(err).ShouldNot(HaveOccurred())
		f.Close()
	})
}

// This code block should be deleted after cke-localproxy is installed on boot servers
func teleportNodeServiceTest() {
	By("storing LoadBalancer IP address to etcd")
	ExecSafeAt(boot0, "env", "ETCDCTL_API=3", "etcdctl", "--cert=/etc/etcd/backup.crt", "--key=/etc/etcd/backup.key",
		"put", "/neco/teleport/auth-servers", `[\"teleport-auth.teleport.svc.cluster.local:3025\"]`)

	By("starting teleport node services on boot servers")
	for _, h := range []string{boot0, boot1, boot2} {
		ExecSafeAt(h, "sudo", "neco", "teleport", "config")
		ExecSafeAt(h, "sudo", "systemctl", "start", "teleport-node.service")
	}
}

func teleportSSHConnectionTest() {
	By("creating user")
	ExecAt(boot0, "kubectl", "-n", "teleport", "exec", "teleport-auth-0", "tctl", "users", "rm", "cybozu")
	stdout, stderr, err := ExecAt(boot0, "kubectl", "-n", "teleport", "exec", "teleport-auth-0", "tctl", "users", "add", "cybozu", "cybozu,root")
	Expect(err).ShouldNot(HaveOccurred(), "stderr=%s", stderr)
	tctlOutput := string(stdout)
	fmt.Println("output:")
	fmt.Println(tctlOutput)

	By("extracting invite token")
	/* target is b86d5b576174f7bbcb87d4905366aa9a in this example:
	User cybozu has been created but requires a password. Share this URL with the user to complete user setup, link is valid for 1h0m0s:
	https://teleport.gcp0.dev-ne.co:443/web/invite/b86d5b576174f7bbcb87d4905366aa9a

	NOTE: Make sure teleport.gcp0.dev-ne.co:443 points at a Teleport proxy which users can access.
	*/
	inviteURL, err := grepLine(tctlOutput, "https://")
	Expect(err).ShouldNot(HaveOccurred())
	slashSplit := strings.Split(inviteURL, "/")
	inviteToken := slashSplit[len(slashSplit)-1]
	Expect(inviteToken).NotTo(BeEmpty())
	fmt.Println("invite token: " + inviteToken)

	By("constructing payload")
	payload, err := json.Marshal(map[string]string{
		"token":               inviteToken,
		"password":            base64.StdEncoding.EncodeToString([]byte("dummypass")),
		"second_factor_token": "",
	})
	Expect(err).ShouldNot(HaveOccurred())
	fmt.Println("payload: " + string(payload))

	By("accessing invite URL")
	filename := "teleport_cookie.txt"
	cmd := exec.Command("curl", "--fail", "--insecure", "-c", filename, inviteURL)
	output, err := cmd.CombinedOutput()
	Expect(err).ShouldNot(HaveOccurred(), "output=%s", output)
	buf, err := os.ReadFile(filename)
	Expect(err).ShouldNot(HaveOccurred(), "cookie=%s", buf)
	cookieFileContents := string(buf)
	fmt.Println("cookie file:")
	fmt.Println(cookieFileContents)

	By("extracting CSRF token")
	/* target is c7c59fea8ec95e81c81b285e0070cb4791e04733fa4f22dffff5a25bb5b1c4f7 in this example:
	# Netscape HTTP Cookie File
	# https://curl.haxx.se/docs/http-cookies.html
	# This file was generated by libcurl! Edit at your own risk.

	#HttpOnly_teleport.gcp0.dev-ne.co	FALSE	/	TRUE	0	grv_csrf	c7c59fea8ec95e81c81b285e0070cb4791e04733fa4f22dffff5a25bb5b1c4f7

	*/
	csrfLine, err := grepLine(cookieFileContents, "#HttpOnly_")
	Expect(err).ShouldNot(HaveOccurred(), "output=%s", stdout)
	csrfLineFields := strings.Fields(csrfLine)
	csrfToken := csrfLineFields[len(csrfLineFields)-1]
	Expect(csrfToken).NotTo(BeEmpty())
	fmt.Printf("CSRF token: %s\n", csrfToken)

	By("updating password")
	cmd = exec.Command(
		"curl",
		"--fail", "--insecure",
		"-X", "PUT",
		"-b", filename,
		"-H", "X-CSRF-Token: "+csrfToken,
		"-H", "Content-Type: application/json; charset=UTF-8",
		"-d", string(payload),
		"https://teleport.gcp0.dev-ne.co/v1/webapi/users/password/token",
	)
	output, err = cmd.CombinedOutput()
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

	By("getting node resources with kubectl via teleport proxy")
	output, err = exec.Command("kubectl", "get", "nodes").CombinedOutput()
	Expect(err).ShouldNot(HaveOccurred(), "output=%s", output)

	By("accessing boot servers using tsh command")
	for _, n := range []string{"boot-0", "boot-1", "boot-2"} {
		Eventually(func() error {
			cmd := exec.Command("tsh", "--insecure", "--proxy=teleport.gcp0.dev-ne.co:443", "--user=cybozu", "ssh", "cybozu@gcp0-"+n, "date")
			output, err := cmd.CombinedOutput()
			if err != nil {
				return fmt.Errorf("tsh ssh failed for %s: %s", n, string(output))
			}
			return nil
		}).Should(Succeed())
	}

	By("confirming kubectl works in node Pod using tsh command")
	for _, n := range []string{"node-maneki-0"} {
		Eventually(func() error {
			cmd := exec.Command("tsh", "--insecure", "--proxy=teleport.gcp0.dev-ne.co:443", "--user=cybozu", "ssh", "cybozu@"+n, ". /etc/profile.d/update-necocli.sh && kubectl -v5 -n maneki get pod")
			output, err := cmd.CombinedOutput()
			if err != nil {
				return fmt.Errorf("tsh ssh failed for %s: %s", n, string(output))
			}
			return nil
		}).Should(Succeed())
	}

	By("clearing /etc/hosts")
	// /etc/hosts cannot be modified via sed directly inside a container because that is a mount point
	b, err = os.ReadFile("./hosts")
	Expect(err).NotTo(HaveOccurred())
	Expect(os.WriteFile("/etc/hosts", b, 0644)).NotTo(HaveOccurred())
	Expect(os.Remove("./hosts")).NotTo(HaveOccurred())
}

func teleportAuthTest() {
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
}

func teleportApplicationTest() {
	// This test requires CNAME record "teleport.gcp0.dev-ne.co : teleport-proxy.teleport.svc".
	By("getting the application names")
	stdout, _, err := kustomizeBuild("../teleport/base/apps")
	Expect(err).ShouldNot(HaveOccurred())
	var appNames []string
	y := k8sYaml.NewYAMLReader(bufio.NewReader(bytes.NewReader(stdout)))
	for {
		data, err := y.Read()
		if err == io.EOF {
			break
		}
		Expect(err).ShouldNot(HaveOccurred())

		var deploy appsv1.Deployment
		err = yaml.Unmarshal(data, &deploy)
		if err != nil {
			continue
		}

		var name string
		for _, a := range deploy.Spec.Template.Spec.Containers[0].Args {
			if !strings.HasPrefix(a, "--app-name=") {
				continue
			}
			name = strings.Split(a, "=")[1]
		}
		Expect(name).ShouldNot(BeEmpty())
		appNames = append(appNames, name)
	}
	fmt.Printf("Found applications in manifests: %+v\n", appNames)

	By("checking applications are correctly deployed")
	Eventually(func() error {
		for _, n := range appNames {
			query := fmt.Sprintf("'.[].spec.apps[].name | select(. == \"%s\")'", n)
			stdout, stderr, err := ExecAt(boot0, "kubectl", "-n", "teleport", "exec", "-it", "teleport-auth-0", "--", "tctl", "apps", "ls", "--format=json", "--", "|", "jq", "-r", query)
			if err != nil {
				return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
			}
			if string(stdout) != n+"\n" {
				return fmt.Errorf("app %s mismatch: actual = %s", n, stdout)
			}
		}
		return nil
	}).Should(Succeed())
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

func grepLine(input string, prefix string) (string, error) {
	reader := bufio.NewReader(strings.NewReader(input))

	for {
		line, isPrefix, err := reader.ReadLine()
		if err == io.EOF {
			return "", errors.New("no match line")
		}
		if isPrefix {
			return "", errors.New("too long line")
		}

		if !strings.HasPrefix(string(line), prefix) {
			continue
		}

		return string(line), nil
	}
}

func testTeleport() {
	It("should deploy teleport services", func() {
		By("teleportNodeServiceTest", teleportNodeServiceTest)
		By("teleportSSHConnectionTest", teleportSSHConnectionTest)
		By("teleportAuthTest", teleportAuthTest)
		By("teleportApplicationTest", teleportApplicationTest)
	})
}
