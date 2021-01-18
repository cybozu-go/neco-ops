package test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"

	"github.com/google/go-cmp/cmp"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
)

// requiredResources is a list of namespace resources that the Neco has explicitly provided to unprivileged teams.
var requiredResources = []string{
	"elasticsearches.elasticsearch.k8s.elastic.co",
	"kibanas.kibana.k8s.elastic.co",
	"httpproxies.projectcontour.io",
	"networkpolicies.crd.projectcalico.org",
	"grafanadatasources.integreatly.org",
	"grafanadashboards.integreatly.org",
	// TODO: Remove this comment out when https://github.com/cybozu-go/moco/pull/116 is imported.
	// "mysqlclusters.moco.cybozu.com",
	"objectbucketclaims.objectbucket.io",
}

// prohibitedResources is a list of namespace resources that are not allowed to be created by unprivileged teams.
// This should be matched the `.spec.namespaceResourceBlacklist` field in the AppProject except for `networkpolicies.networking.k8s.io`.
// `networkpolicies.networking.k8s.io` is configured as bootstrappolicy so we cannot remove the definition.
// - ref: https://github.com/kubernetes/kubernetes/blob/release-1.18/plugin/pkg/auth/authorizer/rbac/bootstrappolicy/policy.go#L297
var prohibitedResources = []string{
	"limitranges",
	"resourcequotas",
}

var (
	allVerbs        = []string{"get", "list", "watch", "create", "update", "patch", "delete", "deletecollection"}
	adminVerbs      = []string{"get", "list", "watch", "create", "update", "patch", "delete", "deletecollection"}
	viewVerbs       = []string{"get", "list", "watch"}
	prohibitedVerbs = []string{}
)

// These regular expressions are used to parse the results of `kubectl auth can-i --list`.
const (
	resourceRegexp       = `[^ ]*`
	nonResourceURLRegexp = `[^ ]*`
	resourceNameRegexp   = `[^ ]*`
	verbsRegexp          = `[*a-z ]*`
	rowRegexp            = `^(` + resourceRegexp + `)\s+\[(` + nonResourceURLRegexp + `)\]\s+\[(` + resourceNameRegexp + `)\]\s+\[(` + verbsRegexp + `)\]$`
)

var authCanIRowRegexp = regexp.MustCompile(rowRegexp)

func getActualVerbs(team, ns string) map[string][]string {
	stdout, stderr, err := ExecAt(boot0, "kubectl", "-n", ns, "--as=test", "--as-group="+team, "--as-group=system:authenticated", "auth", "can-i", "--list", "--no-headers")
	Expect(err).NotTo(HaveOccurred(), "stdout: %s, stderr: %s, err: %v", stdout, stderr, err)

	ret := map[string][]string{}
	reader := bufio.NewReader(bytes.NewReader(stdout))
	for {
		line, isPrefix, err := reader.ReadLine()
		if err == io.EOF {
			break
		}
		Expect(err).NotTo(HaveOccurred())
		Expect(isPrefix).NotTo(BeTrue(), "too long line: %s", line)

		submatch := authCanIRowRegexp.FindStringSubmatch(string(line))
		Expect(submatch).NotTo(HaveLen(0))
		// The elements of the submatch slice match the following items.
		// - submatch[1] ... Resources
		// - submatch[2] ... Non-Resource URLs
		// - submatch[3] ... Resource Names
		// - submatch[4] ... Verbs

		resource := submatch[1]
		if resource == "" {
			continue
		}
		origVerbs := strings.Split(submatch[4], " ")

		// '*' means can do everything
		for _, v := range origVerbs {
			if v == "*" {
				ret[resource] = allVerbs
				continue
			}
		}

		// remove duplicate verb
		found := map[string]bool{}
		for _, v := range origVerbs {
			found[v] = true
		}
		verbs := make([]string, 0, len(allVerbs))
		for _, v := range allVerbs {
			if found[v] {
				verbs = append(verbs, v)
			}
		}
		ret[resource] = verbs
	}
	return ret
}

func appropriateAuthorityTest() {
	namespaceList := []string{}
	nsOwner := map[string]string{}
	teamList := []string{}

	By("listing namespaces and their owner team")
	stdout, stderr, err := ExecAt(boot0, "kubectl", "get", "namespaces", "-o=json")
	Expect(err).NotTo(HaveOccurred(), "stdout: %s, stderr: %s, err: %v", stdout, stderr, err)

	nsList := new(corev1.NamespaceList)
	err = json.Unmarshal(stdout, nsList)
	Expect(err).NotTo(HaveOccurred(), "stdout: %s, stderr: %s, err: %v", stdout, stderr, err)

	// make namespace list
	for _, ns := range nsList.Items {
		namespaceList = append(namespaceList, ns.Name)
		if ns.Labels["team"] != "" {
			nsOwner[ns.Name] = ns.Labels["team"]
		}
	}
	sort.Strings(namespaceList)

	// make unprivileged team list
	teamSet := make(map[string]struct{})
	for _, t := range nsOwner {
		if t != "neco" {
			teamSet[t] = struct{}{}
		}
	}
	for t := range teamSet {
		teamList = append(teamList, t)
	}
	sort.Strings(teamList)

	By("constructing expected and actual verbs for namespace resources")
	// Construct the verbs maps. The key and value are as follows.
	// - key  : "<team>:<namespace>/<resource>"
	// - value: []strings{verbs...}
	expectedVerbs := map[string][]string{}
	actualVerbs := map[string][]string{}

	keyGen := func(team, ns, resource string) string {
		return fmt.Sprintf("%s:%s/%s", team, ns, resource)
	}

	for _, team := range teamList {
		for _, ns := range namespaceList {
			actualVerbsByResource := getActualVerbs(team, ns)

			// check secrets
			key := keyGen(team, ns, "secrets")

			if ns == "sandbox" || team == nsOwner[ns] {
				expectedVerbs[key] = adminVerbs
			} else {
				expectedVerbs[key] = prohibitedVerbs
			}

			if v, ok := actualVerbsByResource["secrets"]; ok {
				actualVerbs[key] = v
			} else {
				actualVerbs[key] = prohibitedVerbs
			}

			// check required resources
			for _, resource := range requiredResources {
				key := keyGen(team, ns, resource)

				if ns == "sandbox" || team == nsOwner[ns] {
					expectedVerbs[key] = adminVerbs
				} else {
					expectedVerbs[key] = viewVerbs
				}

				if v, ok := actualVerbsByResource[resource]; ok {
					actualVerbs[key] = v
				} else {
					actualVerbs[key] = prohibitedVerbs
				}
			}

			// check prohibited resources
			for _, resource := range prohibitedResources {
				key := keyGen(team, ns, resource)
				expectedVerbs[key] = viewVerbs

				if v, ok := actualVerbsByResource[resource]; ok {
					actualVerbs[key] = v
				} else {
					actualVerbs[key] = prohibitedVerbs
				}
			}
		}
	}

	By("checking results for namespace resources")
	Expect(actualVerbs).To(Equal(expectedVerbs), cmp.Diff(actualVerbs, expectedVerbs))

	By("listing cluster resources")
	stdout, stderr, err = ExecAt(boot0, "kubectl", "api-resources", "--namespaced=false", "-o=name", "--sort-by=name")
	Expect(err).NotTo(HaveOccurred(), "stdout: %s, stderr: %s, err: %v", stdout, stderr, err)

	var clusterResources []string
	reader := bufio.NewReader(bytes.NewReader(stdout))
	for {
		line, isPrefix, err := reader.ReadLine()
		if err == io.EOF {
			break
		}
		Expect(err).NotTo(HaveOccurred())
		Expect(isPrefix).NotTo(BeTrue(), "too long line: %s", line)
		clusterResources = append(clusterResources, string(line))
	}

	By("checking RBAC of cluster resources")
	for _, team := range teamList {
		for _, ns := range namespaceList {
			actualVerbsByResource := getActualVerbs(team, ns)

			for _, resource := range clusterResources {
				actual := actualVerbsByResource[resource]
				switch resource {
				case "selfsubjectaccessreviews.authorization.k8s.io", "selfsubjectrulesreviews.authorization.k8s.io":
					Expect(actual).To(Equal([]string{"create"}))
				default:
					Expect(actual).To(BeElementOf([]string(nil), []string{}, []string{"get"}, []string{"get", "list", "watch"}))
				}
			}
		}
	}
}

func ephemeralContainersPrivilegeTest() {
	By("creating test pod")
	stdout, stderr, err := ExecAt(boot0, "kubectl", "run", "-n", "maneki", "neco-ephemeral-test", "--image=quay.io/cybozu/ubuntu-debug:18.04", "pause")
	Expect(err).NotTo(HaveOccurred(), "stdout: %s, stderr: %s, err: %v", stdout, stderr, err)

	By("waiting the pod become ready")
	Eventually(func() error {
		stdout, _, err := ExecAt(boot0, "kubectl", "get", "-n", "maneki", "pod/neco-ephemeral-test", "-o=json")
		if err != nil {
			return err
		}
		po := new(corev1.Pod)
		err = json.Unmarshal(stdout, po)
		if err != nil {
			return fmt.Errorf("failed to get pod info: %w", err)
		}

		if po.Status.ContainerStatuses == nil || len(po.Status.ContainerStatuses) == 0 || !po.Status.ContainerStatuses[0].Ready {
			return fmt.Errorf("pod is not ready")
		}

		return nil
	}).Should(Succeed())

	By("adding a ephemeral container by unprivileged team")
	stdout, stderr, err = ExecAt(boot0, "kubectl", "alpha", "debug", "-i", "-n", "maneki", "neco-ephemeral-test", "--image=quay.io/cybozu/ubuntu-debug:18.04", "--target=neco-ephemeral-test", "--as=test", "--as-group=maneki", "--as-group=system:authenticated", "--", "echo a")
	Expect(err).NotTo(HaveOccurred(), "stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
}

func cephClusterPrivilegeTest() {
	// This test confirming the configuration of RBAC so it should be at team-management_test.go but rook/ceph isn't deployed for GCP (without gcp-ceph)
	podPvcYaml := `apiVersion: objectbucket.io/v1alpha1
kind: ObjectBucketClaim
metadata:
  name: hdd-ob
  namespace: maneki
spec:
  generateBucketName: obc-poc
  storageClassName: ceph-hdd-bucket`
	stdout, stderr, err := ExecAtWithInput(boot0, []byte(podPvcYaml), "kubectl", "--as test", "--as-group sys:authenticated", "--as-group maneki", "apply", "-f", "-")
	Expect(err).NotTo(HaveOccurred(), "stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
}

func testTeamManagement() {
	It("should give appropriate authority to unprivileged team", func() {
		By("appropriateAuthorityTest", appropriateAuthorityTest)
		By("ephemeralContainersPrivilegeTest", ephemeralContainersPrivilegeTest)
		By("cephClusterPrivilegeTest", cephClusterPrivilegeTest)
	})
}
