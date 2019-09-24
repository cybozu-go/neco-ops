package test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/cybozu-go/sabakan/v2"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/yaml"
)

// ckeCluster is part of cke.Cluster in github.com/cybozu-go/cke
type ckeCluster struct {
	Nodes []*ckeNode `yaml:"nodes"`
}

// ckeNode is part of cke.Node in github.com/cybozu-go/cke
type ckeNode struct {
	Address      string `yaml:"address"`
	ControlPlane bool   `yaml:"control_plane"`
}

// serfMember is copied from type Member https://godoc.org/github.com/hashicorp/serf/cmd/serf/command#Member
// to prevent much vendoring
type serfMember struct {
	Name   string            `json:"name"`
	Addr   string            `json:"addr"`
	Port   uint16            `json:"port"`
	Tags   map[string]string `json:"tags"`
	Status string            `json:"status"`
	Proto  map[string]uint8  `json:"protocol"`
	// contains filtered or unexported fields
}

// serfMemberContainer is copied from type MemberContainer https://godoc.org/github.com/hashicorp/serf/cmd/serf/command#MemberContainer
// to prevent much vendoring
type serfMemberContainer struct {
	Members []serfMember `json:"members"`
}

func fetchClusterNodes() (map[string]bool, error) {
	stdout, stderr, err := ExecAt(boot0, "ckecli", "cluster", "get")
	if err != nil {
		return nil, fmt.Errorf("stdout=%s, stderr=%s err=%v", stdout, stderr, err)
	}

	cluster := new(ckeCluster)
	err = yaml.Unmarshal(stdout, cluster)
	if err != nil {
		return nil, err
	}

	m := make(map[string]bool)
	for _, n := range cluster.Nodes {
		m[n.Address] = n.ControlPlane
	}
	return m, nil
}

func getSerfMembers() (*serfMemberContainer, error) {
	stdout, stderr, err := ExecAt(boot0, "serf", "members", "-format", "json")
	if err != nil {
		return nil, fmt.Errorf("stdout=%s, stderr=%s err=%v", stdout, stderr, err)
	}
	var result serfMemberContainer
	err = json.Unmarshal(stdout, &result)
	if err != nil {
		return nil, fmt.Errorf("stdout=%s, stderr=%s err=%v", stdout, stderr, err)
	}
	return &result, nil
}

// testRebootAllNodes tests all nodes stop scenario
func testRebootAllNodes() {
	var beforeNodes map[string]bool
	It("fetch cluster nodes", func() {
		var err error
		beforeNodes, err = fetchClusterNodes()
		Expect(err).ShouldNot(HaveOccurred())
	})

	It("stop CKE sabakan integration", func() {
		ExecSafeAt(boot0, "ckecli", "sabakan", "disable")
	})

	It("reboots all nodes", func() {
		By("reboot all nodes")
		stdout, _, err := ExecAt(boot0, "sabactl", "machines", "get")
		Expect(err).ShouldNot(HaveOccurred())
		var machines []sabakan.Machine
		err = json.Unmarshal(stdout, &machines)
		Expect(err).ShouldNot(HaveOccurred())
		// Skip reboot vm on rack-3 because IPMI is not initialized
		for _, m := range machines {
			if m.Spec.Role == "boot" || m.Spec.Rack == 3 {
				continue
			}
			stdout, stderr, err := ExecAt(boot0, "neco", "ipmipower", "stop", m.Spec.IPv4[0])
			Expect(err).ShouldNot(HaveOccurred(), "stdout: %s, stderr: %s", stdout, stderr)
		}
		for _, m := range machines {
			if m.Spec.Rack == 3 {
				continue
			}
			stdout, stderr, err := ExecAt(boot0, "neco", "ipmipower", "start", m.Spec.IPv4[0])
			Expect(err).ShouldNot(HaveOccurred(), "stdout: %s, stderr: %s", stdout, stderr)
		}

		By("wait for start of rebooting")
		preReboot := make(map[string]bool)
		for _, m := range machines {
			preReboot[m.Spec.IPv4[0]] = true
		}
		Eventually(func() error {
			result, err := getSerfMembers()
			if err != nil {
				return err
			}
			for _, member := range result.Members {
				addrs := strings.Split(member.Addr, ":")
				if len(addrs) != 2 {
					return fmt.Errorf("unexpected addr: %s", member.Addr)
				}
				addr := addrs[0]
				if preReboot[addr] && member.Status != "alive" {
					delete(preReboot, addr)
				}
			}
			if len(preReboot) > 0 {
				return fmt.Errorf("some nodes are still starting reboot: %v", preReboot)
			}
			return nil
		})

		By("wait for recovery of all nodes")
		Eventually(func() error {
			nodes, err := fetchClusterNodes()
			if err != nil {
				return err
			}
			result, err := getSerfMembers()
			if err != nil {
				return err
			}
		OUTER:
			for k := range nodes {
				for _, m := range result.Members {
					addrs := strings.Split(m.Addr, ":")
					if len(addrs) != 2 {
						return fmt.Errorf("unexpected addr: %s", m.Addr)
					}
					addr := addrs[0]
					if addr == k {
						if m.Status != "alive" {
							return fmt.Errorf("reboot failed: %s, %v", k, m)
						}
						continue OUTER
					}
				}
				return fmt.Errorf("cannot find in serf members: %s", k)
			}
			return nil
		}).Should(Succeed())
	})

	It("fetch cluster nodes", func() {
		Eventually(func() error {
			afterNodes, err := fetchClusterNodes()
			if err != nil {
				return err
			}

			if !reflect.DeepEqual(beforeNodes, afterNodes) {
				return fmt.Errorf("cluster nodes mismatch after reboot: before=%v after=%v", beforeNodes, afterNodes)
			}

			return nil
		}).Should(Succeed())
	})

	It("sets all nodes' machine state to healthy", func() {
		Eventually(func() error {
			stdout, stderr, err := ExecAt(boot0, "sabactl", "machines", "get")
			if err != nil {
				return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
			}

			var machines []sabakan.Machine
			err = json.Unmarshal(stdout, &machines)
			if err != nil {
				return err
			}

			for _, m := range machines {
				if m.Spec.Role == "boot" {
					continue
				}
				stdout := ExecSafeAt(boot0, "sabactl", "machines", "get-state", m.Spec.Serial)
				state := string(bytes.TrimSpace(stdout))
				if state != "healthy" {
					return fmt.Errorf("sabakan machine state of %s is not healthy: %s", m.Spec.Serial, state)
				}
			}

			return nil
		}).Should(Succeed())
	})

	It("wait for Kubernetes cluster to become ready", func() {
		By("waiting nodes")
		Eventually(func() error {
			stdout, stderr, err := ExecAt(boot0, "kubectl", "get", "nodes", "-o", "json")
			if err != nil {
				return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
			}

			var nl corev1.NodeList
			err = json.Unmarshal(stdout, &nl)
			if err != nil {
				return err
			}

			// control-plane-count + minimum-workers = 5
			// https://github.com/cybozu-go/cke/blob/master/docs/sabakan-integration.md#initialization
			if len(nl.Items) != 5 {
				return fmt.Errorf("too few nodes: %d", len(nl.Items))
			}
		OUTER:
			for _, n := range nl.Items {
				for _, cond := range n.Status.Conditions {
					if cond.Type != corev1.NodeReady {
						continue
					}
					if cond.Status != corev1.ConditionTrue {
						return fmt.Errorf("node %s is not ready", n.Name)
					}
					continue OUTER
				}
				return fmt.Errorf("node %s has no readiness status", n.Name)
			}
			return nil
		}).Should(Succeed())
	})

	It("re-enable CKE sabakan integration", func() {
		ExecSafeAt(boot0, "ckecli", "sabakan", "enable")
	})
}
