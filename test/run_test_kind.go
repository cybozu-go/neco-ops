// +build kind

package test

import (
	"bytes"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"strings"

	. "github.com/onsi/gomega"
	"golang.org/x/crypto/ssh"
)

type sshAgent struct {
	client *ssh.Client
	conn   net.Conn
}

func prepare() {
	resources := []string{
		"https://raw.githubusercontent.com/cybozu-go/cke/master/static/pod-security-policy.yml",
		"https://raw.githubusercontent.com/cybozu-go/cke/master/static/rbac.yml",
		"https://raw.githubusercontent.com/cybozu-go/neco/master/etc/namespaces.yml",
		"https://raw.githubusercontent.com/cybozu-go/neco/master/etc/pod-security-policy.yml",
	}

	for _, r := range resources {
		ExecSafeAt("", "kubectl", "apply", "-f", r)
	}

	ExecSafeAt("", "curl", "-sSLf", "-O", "https://raw.githubusercontent.com/cybozu-go/neco/master/artifacts.go")
	bytes := ExecSafeAt("", "cat", "artifacts.go", "|", "grep", "squid", "|", "awk", "'match($0, /Repository: \"(.*)\", Tag: \"(.*)\"/, a) { print a[1] \":\" a[2]}'")
	squidImg := strings.TrimSpace(string(bytes))
	ExecSafeAt("", "rm", "-f", "artifacts.go")

	ExecSafeAt("", "curl", "-sSLf", "-O", "https://raw.githubusercontent.com/cybozu-go/cke/master/images.go")
	bytes = ExecSafeAt("", "cat", "images.go", "|", "grep", "quay.io/cybozu/unbound", "|", "awk", "'match($0, /Image\\(\"(.*)\"\\)/, a) { print a[1] }'")
	unboundImg := strings.TrimSpace(string(bytes))
	ExecSafeAt("", "rm", "-f", "images.go")

	ExecSafeAt("", "curl", "-sSLf", "-O", "https://raw.githubusercontent.com/cybozu-go/neco/master/etc/squid.yml")
	ExecSafeAt("", "sed", "-i", "'s@{{ .squid }}@"+squidImg+"@g'", "squid.yml")
	ExecSafeAt("", "sed", "-i", "'s@{{ index . \"cke-unbound\" }}@"+unboundImg+"@g'", "squid.yml")
	ExecSafeAt("", "kubectl", "apply", "-f", "squid.yml")

	ExecSafeAt("", "kubectl", "label", "node", "kindtest-control-plane", "cke.cybozu.com/index-in-rack=4", "cke.cybozu.com/master=true", "cke.cybozu.com/rack=0", "cke.cybozu.com/role=cs")
	ExecSafeAt("", "kubectl", "label", "node", "kindtest-worker", "cke.cybozu.com/index-in-rack=4", "cke.cybozu.com/rack=1", "cke.cybozu.com/role=cs")
	ExecSafeAt("", "kubectl", "label", "node", "kindtest-worker2", "cke.cybozu.com/index-in-rack=4", "cke.cybozu.com/rack=2", "cke.cybozu.com/role=cs")
	ExecSafeAt("", "kubectl", "label", "node", "kindtest-worker3", "cke.cybozu.com/index-in-rack=5", "cke.cybozu.com/rack=0", "cke.cybozu.com/role=cs")
}

// ExecAt executes command at given host
func ExecAt(host string, args ...string) (stdout, stderr []byte, e error) {
	return ExecAtWithInput(host, nil, args...)
}

// ExecAtWithInput executes command at given host with input
// WARNING: `input` can contain secret data.  Never output `input` to console.
func ExecAtWithInput(host string, input []byte, args ...string) (stdout, stderr []byte, e error) {
	return doExec(nil, input, args...)
}

func doExec(agent *sshAgent, input []byte, args ...string) ([]byte, []byte, error) {
	var stdout, stderr bytes.Buffer
	cmd := exec.Command("sh", "-c", strings.Join(args, " "))
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if input != nil {
		cmd.Stdin = bytes.NewReader(input)
	}

	err := cmd.Run()
	return stdout.Bytes(), stderr.Bytes(), err
}

// ExecSafeAt executes command at given host and returns only stdout
func ExecSafeAt(host string, args ...string) []byte {
	stdout, stderr, err := ExecAt(host, args...)
	ExpectWithOffset(1, err).To(Succeed(), "[%s] %v, stdout: %s, stderr: %s", host, args, stdout, stderr)
	return stdout
}

func loadArgoCDPassword() string {
	password, err := ioutil.ReadFile(argoCDPasswordFile)
	Expect(err).NotTo(HaveOccurred())
	return string(password)
}

func saveArgoCDPassword(password string) {
	err := ioutil.WriteFile(argoCDPasswordFile, []byte(password), os.FileMode(0644))
	Expect(err).NotTo(HaveOccurred())
}
