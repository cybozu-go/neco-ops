// +build !kind

package test

import (
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"strings"
	"time"

	. "github.com/onsi/gomega"
	"golang.org/x/crypto/ssh"
)

const (
	sshTimeout = 3 * time.Minute

	defaultDialTimeout = 30 * time.Second
	defaultKeepAlive   = 5 * time.Second

	// DefaultRunTimeout is the timeout value for Agent.Run().
	DefaultRunTimeout = 10 * time.Minute
)

var (
	sshClients = make(map[string]*sshAgent)

	// https://github.com/cybozu-go/cke/pull/81/files
	agentDialer = &net.Dialer{
		Timeout:   defaultDialTimeout,
		KeepAlive: defaultKeepAlive,
	}
)

type sshAgent struct {
	client *ssh.Client
	conn   net.Conn
}

func prepare() {
	err := prepareSSHClients(boot0, boot1, boot2)
	Expect(err).NotTo(HaveOccurred())

	// sync VM root filesystem to store newly generated SSH host keys.
	for h := range sshClients {
		ExecSafeAt(h, "sync")
	}
}

func sshTo(address string, sshKey ssh.Signer, userName string) (*sshAgent, error) {
	conn, err := agentDialer.Dial("tcp", address+":22")
	if err != nil {
		fmt.Printf("failed to dial: %s\n", address)
		return nil, err
	}
	config := &ssh.ClientConfig{
		User: userName,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(sshKey),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         5 * time.Second,
	}
	err = conn.SetDeadline(time.Now().Add(defaultDialTimeout))
	if err != nil {
		conn.Close()
		return nil, err
	}
	clientConn, channelCh, reqCh, err := ssh.NewClientConn(conn, "tcp", config)
	if err != nil {
		// conn was already closed in ssh.NewClientConn
		return nil, err
	}
	err = conn.SetDeadline(time.Time{})
	if err != nil {
		clientConn.Close()
		return nil, err
	}
	a := sshAgent{
		client: ssh.NewClient(clientConn, channelCh, reqCh),
		conn:   conn,
	}
	return &a, nil
}

func parsePrivateKey(keyPath string) (ssh.Signer, error) {
	f, err := os.Open(keyPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	data, err := ioutil.ReadAll(f)
	if err != nil {
		return nil, err
	}

	return ssh.ParsePrivateKey(data)
}

func prepareSSHClients(addresses ...string) error {
	sshKey, err := parsePrivateKey(sshKeyFile)
	if err != nil {
		return err
	}

	ch := time.After(sshTimeout)
	for _, a := range addresses {
	RETRY:
		select {
		case <-ch:
			return errors.New("timed out")
		default:
		}
		agent, err := sshTo(a, sshKey, "cybozu")
		if err != nil {
			time.Sleep(time.Second)
			goto RETRY
		}
		sshClients[a] = agent
	}

	return nil
}

// ExecAt executes command at given host
func ExecAt(host string, args ...string) (stdout, stderr []byte, e error) {
	return ExecAtWithInput(host, nil, args...)
}

// ExecAtWithInput executes command at given host with input
// WARNING: `input` can contain secret data.  Never output `input` to console.
func ExecAtWithInput(host string, input []byte, args ...string) (stdout, stderr []byte, e error) {
	agent := sshClients[host]
	stdout, stderr, err := doExec(agent, input, args...)
	if err != nil {
		e = fmt.Errorf("Exec failed: args: %v, err: %w", args, err)
	}
	return
}

func doExec(agent *sshAgent, input []byte, args ...string) ([]byte, []byte, error) {
	err := agent.conn.SetDeadline(time.Now().Add(DefaultRunTimeout))
	if err != nil {
		return nil, nil, err
	}
	defer agent.conn.SetDeadline(time.Time{})

	sess, err := agent.client.NewSession()
	if err != nil {
		return nil, nil, err
	}
	defer sess.Close()

	if input != nil {
		sess.Stdin = bytes.NewReader(input)
	}
	outBuf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	sess.Stdout = outBuf
	sess.Stderr = errBuf
	err = sess.Run(strings.Join(args, " "))
	return outBuf.Bytes(), errBuf.Bytes(), err
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
