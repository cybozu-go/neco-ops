package test

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/cybozu-go/log"
	. "github.com/onsi/ginkgo"
	"github.com/onsi/ginkgo/reporters"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	if os.Getenv("SSH_PRIVKEY") == "" {
		t.Skip("no SSH_PRIVKEY envvar")
	}

	RegisterFailHandler(Fail)
	junitReporter := reporters.NewJUnitReporter("/tmp/junit.xml")
	RunSpecsWithDefaultAndCustomReporters(t, "Test", []Reporter{junitReporter})
}

var _ = BeforeSuite(func() {
	fmt.Println("Preparing...")

	SetDefaultEventuallyPollingInterval(time.Second)
	SetDefaultEventuallyTimeout(40 * time.Minute)

	prepare()

	log.DefaultLogger().SetOutput(GinkgoWriter)

	fmt.Println("Begin tests...")
})

// This must be the only top-level test container.
// Other tests and test containers must be listed in this.
var _ = Describe("Test applications", func() {
	BeforeEach(func() {
		fmt.Printf("START: %s\n", time.Now().Format(time.RFC3339))
	})
	AfterEach(func() {
		fmt.Printf("END: %s\n", time.Now().Format(time.RFC3339))
	})

	switch testSuite {
	case "bootstrap":
		bootstrapTest()
	case "prepare":
		bootstrapTest()
		prepareTest()
	case "run":
		runTest()
	}
})

func bootstrapTest() {
	Context("prepareNodes", prepareNodes)
	Context("prepareLoadPods", prepareLoadPods)
	Context("setup", testSetup)
}

func prepareTest() {
	if doReboot {
		Context("prepare reboot rook-ceph", prepareRebootRookCeph)
		Context("reboot", testRebootAllNodes)
		Context("reboot rook-ceph", testRebootRookCeph)
	}

	// preparing resources before test to make things faster
	Context("preparing moco", prepareMoco)
	Context("preparing rook-ceph", prepareRookCeph)
	Context("preparing argocd-ingress", prepareArgoCDIngress)
	Context("preparing contour", prepareContour)
	Context("preparing elastic", prepareElastic)
	Context("preparing local-pv-provisioner", prepareLocalPVProvisioner)
	Context("preparing metallb", prepareMetalLB)
	Context("preparing pushgateway", preparePushgateway)
	Context("preparing hpa", prepareHPA)
	Context("preparing grafana-operator", prepareGrafanaOperator)
	Context("preparing sandbox grafana", prepareSandboxGrafanaIngress)
	Context("preparing topolvm", prepareTopoLVM)
	Context("preparing teleport", prepareTeleport)
	Context("preparing customer-egress", prepareCustomerEgress)
	Context("preparing sealed-secret", prepareSealedSecret)
	Context("preparing network-policy", prepareNetworkPolicy) // this must be the last preparation.
}

func runTest() {
	// running tests
	Context("rook-ceph", testRookCeph)
	Context("network-policy", testNetworkPolicy)
	Context("metallb", testMetalLB)
	Context("contour", testContour)
	Context("machines-endpoints", testMachinesEndpoints)
	Context("kube-state-metrics", testKubeStateMetrics)
	Context("logging", testLogging)
	Context("grafana-operator", testGrafanaOperator)
	Context("sandbox-grafana", testSandboxGrafana)
	Context("pushgateway", testPushgateway)
	Context("hpa", testHPA)
	Context("victoriametrics-operator", testVictoriaMetricsOperator)
	Context("vmsmallset-components", testVMSmallsetClusterComponents)
	Context("vmlargeset-components", testVMLargesetClusterComponents)
	Context("topolvm", testTopoLVM)
	Context("elastic", testElastic)
	Context("argocd-ingress", testArgoCDIngress)
	Context("admission", testAdmission)
	Context("bmc-reverse-proxy", testBMCReverseProxy)
	Context("local-pv-provisioner", testLocalPVProvisioner)
	Context("teleport", testTeleport)
	Context("team-management", testTeamManagement)
	Context("moco", testMoco)
	Context("sealed-secret", testSealedSecret)
	Context("customer-egress", testCustomerEgress)
	Context("pod-security-admission", testPodSecurityAdmission)
}
