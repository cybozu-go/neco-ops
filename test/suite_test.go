package test

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/cybozu-go/log"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	if os.Getenv("SSH_PRIVKEY") == "" {
		t.Skip("no SSH_PRIVKEY envvar")
	}

	RegisterFailHandler(Fail)
	RunSpecs(t, "Test")
}

var _ = BeforeSuite(func() {
	fmt.Println("Preparing...")

	SetDefaultEventuallyPollingInterval(time.Second)
	SetDefaultEventuallyTimeout(10 * time.Minute)

	prepare()

	log.DefaultLogger().SetOutput(GinkgoWriter)

	fmt.Println("Begin tests...")
})

// This must be the only top-level test container.
// Other tests and test containers must be listed in this.
var _ = Describe("Test applications", func() {
	Context("setup", testSetup)
	if doBootstrap {
		return
	}
	if doReboot {
		Context("reboot", testRebootAllNodes)
	}
	Context("gatekeeper", testGatekeeper)
	Context("network-policy", testNetworkPolicy)
	Context("metallb", testMetalLB)
	Context("external-dns", testExternalDNS)
	Context("cert-manager", testCertManager)
	Context("contour", testContour)
	if !withKind {
		Context("machines-endpoints", testMachinesEndpoints)
	}
	Context("kube-state-metrics", testKubeStateMetrics)
	Context("prometheus", testPrometheus)
	Context("grafana", testGrafana)
	Context("alertmanager", testAlertmanager)
	Context("metrics", testMetrics)
	if !withKind {
		Context("teleport", testTeleport)
	}
	Context("topolvm", testTopoLVM)
	Context("elastic", testElastic)
})
