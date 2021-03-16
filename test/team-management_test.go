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
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
)

// requiredResources is a list of namespace resources that the Neco has explicitly provided to unprivileged teams.
var requiredResources = []string{
	"elasticsearches.elasticsearch.k8s.elastic.co",
	"kibanas.kibana.k8s.elastic.co",
	"httpproxies.projectcontour.io",
	"networkpolicies.crd.projectcalico.org",
	"grafanadatasources.integreatly.org",
	"grafanadashboards.integreatly.org",
	"mysqlclusters.moco.cybozu.com",
	"objectbucketclaims.objectbucket.io",
	"vmagents.operator.victoriametrics.com",
	"vmalertmanagers.operator.victoriametrics.com",
	"vmalerts.operator.victoriametrics.com",
	"vmpodscrapes.operator.victoriametrics.com",
	"vmprobes.operator.victoriametrics.com",
	"vmrules.operator.victoriametrics.com",
	"vmservicescrapes.operator.victoriametrics.com",
	"vmstaticscrapes.operator.victoriametrics.com",
	"apmservers.apm.k8s.elastic.co",
	"beats.beat.k8s.elastic.co",
	"certificaterequests.cert-manager.io",
	"certificates.cert-manager.io",
	"dnsendpoints.externaldns.k8s.io",
	"enterprisesearches.enterprisesearch.k8s.elastic.co",
	"grafanas.integreatly.org",
	"issuers.cert-manager.io",
	"networksets.crd.projectcalico.org",
	"sealedsecrets.bitnami.com",
}

// viewableResources is a list of resources that Neco allows for tenant users
// to view or list.
var viewableResources = []string{
	"applications.argoproj.io",
	"appprojects.argoproj.io",
	"blockrequests.coil.cybozu.com",
	"certificatesigningrequests.certificates.k8s.io",
	"challenges.acme.cert-manager.io",
	"clusterinformations.crd.projectcalico.org",
	"clusterissuers.cert-manager.io",
	"felixconfigurations.crd.projectcalico.org",
	"globalnetworkpolicies.crd.projectcalico.org",
	"globalnetworksets.crd.projectcalico.org",
	"hostendpoints.crd.projectcalico.org",
	"logicalvolumes.topolvm.cybozu.com",
	"orders.acme.cert-manager.io",
	"tlscertificatedelegations.projectcontour.io",
}

// prohibitedResources is a list of namespace resources that are not allowed to be created by unprivileged teams.
// This should be matched the `.spec.namespaceResourceBlacklist` field in the AppProject except for `networkpolicies.networking.k8s.io`.
// `networkpolicies.networking.k8s.io` is configured as bootstrappolicy so we cannot remove the definition.
// - ref: https://github.com/kubernetes/kubernetes/blob/release-1.18/plugin/pkg/auth/authorizer/rbac/bootstrappolicy/policy.go#L297
var prohibitedResources = []string{
	"limitranges",
	"resourcequotas",
	"egresses.coil.cybozu.com",
}

// viewableClusterResources is a list of cluster resources that Neco allows for tenant users
// to view or list.
var viewableClusterResources = []string{
	"addressblocks.coil.cybozu.com",
	"addresspools.coil.cybozu.com",
	"objectbuckets.objectbucket.io",
}

// prohibitedClusterResources is a list of cluster resources that are not allowed to be created by unprivileged teams
var prohibitedClusterResources = []string{
	"bgpconfigurations.crd.projectcalico.org",
	"bgppeers.crd.projectcalico.org",
	"blockaffinities.crd.projectcalico.org",
	"cephblockpools.ceph.rook.io",
	"cephclients.ceph.rook.io",
	"cephclusters.ceph.rook.io",
	"cephfilesystems.ceph.rook.io",
	"cephnfses.ceph.rook.io",
	"cephobjectrealms.ceph.rook.io",
	"cephobjectstores.ceph.rook.io",
	"cephobjectstoreusers.ceph.rook.io",
	"cephobjectzonegroups.ceph.rook.io",
	"cephobjectzones.ceph.rook.io",
	"cephrbdmirrors.ceph.rook.io",
	"ipamblocks.crd.projectcalico.org",
	"ipamconfigs.crd.projectcalico.org",
	"ipamhandles.crd.projectcalico.org",
	"ippools.crd.projectcalico.org",
	"kubecontrollersconfigurations.crd.projectcalico.org",
	"vmclusters.operator.victoriametrics.com",
	"vmnodescrapes.operator.victoriametrics.com",
	"vmsingles.operator.victoriametrics.com",
	"volumes.rook.io",
}

var (
	allVerbs        = []string{"get", "list", "watch", "create", "update", "patch", "delete"}
	adminVerbs      = []string{"get", "list", "watch", "create", "update", "patch", "delete"}
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

func testTeamManagement() {
	It("should give appropriate authority to unprivileged team", func() {
		namespaceList := []string{}
		nsOwner := map[string]string{}
		tenantTeamList := []string{}

		By("checking CRD list")
		stdout, stderr, err := ExecAt(boot0, "kubectl", "get", "crd", "-o=json")
		Expect(err).NotTo(HaveOccurred(), "stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
		crds := &apiextensionsv1.CustomResourceDefinitionList{}
		crdSet := make(map[string]bool)
		err = json.Unmarshal(stdout, crds)
		Expect(err).NotTo(HaveOccurred())
		for _, c := range crds.Items {
			crdSet[c.Name] = false
		}

		for _, r := range requiredResources {
			if _, ok := crdSet[r]; ok {
				crdSet[r] = true
			}
		}
		for _, r := range viewableResources {
			if _, ok := crdSet[r]; ok {
				crdSet[r] = true
			}
		}
		for _, r := range prohibitedResources {
			if _, ok := crdSet[r]; ok {
				crdSet[r] = true
			}
		}
		for _, r := range viewableClusterResources {
			if _, ok := crdSet[r]; ok {
				crdSet[r] = true
			}
		}
		for _, r := range prohibitedClusterResources {
			if _, ok := crdSet[r]; ok {
				crdSet[r] = true
			}
		}

		uncheckedCRDList := []string{}
		for key, val := range crdSet {
			if !val {
				uncheckedCRDList = append(uncheckedCRDList, key)
			}
		}
		Expect(uncheckedCRDList).Should(HaveLen(0), "tenants' permissions to all the CRDs should be checked., but %v are not checked", uncheckedCRDList)

		By("listing namespaces and their owner team")
		stdout, stderr, err = ExecAt(boot0, "kubectl", "get", "namespaces", "-o=json")
		Expect(err).NotTo(HaveOccurred(), "stdout: %s, stderr: %s, err: %v", stdout, stderr, err)

		nsList := new(corev1.NamespaceList)
		err = json.Unmarshal(stdout, nsList)
		Expect(err).NotTo(HaveOccurred(), "stdout: %s, stderr: %s, err: %v", stdout, stderr, err)

		// make namespace list
		for _, ns := range nsList.Items {
			namespaceList = append(namespaceList, ns.Name)
			// Some namespaces (default, kube-public, kube-node-lease) don't have a team label.
			// In this test, they are considered as managed by the Neco team.
			if ns.Labels["team"] == "" {
				nsOwner[ns.Name] = "neco"
			} else {
				nsOwner[ns.Name] = ns.Labels["team"]
			}
		}
		sort.Strings(namespaceList)

		// make unprivileged team list
		tenantTeamSet := make(map[string]struct{})
		for _, t := range nsOwner {
			if t != "neco" {
				tenantTeamSet[t] = struct{}{}
			}
		}
		for t := range tenantTeamSet {
			tenantTeamList = append(tenantTeamList, t)
		}
		sort.Strings(tenantTeamList)

		By("constructing expected and actual verbs for namespace resources")
		// Construct the verbs maps. The key and value are as follows.
		// - key  : "<team>:<namespace>/<resource>"
		// - value: []strings{verbs...}
		expectedVerbs := map[string][]string{}
		actualVerbs := map[string][]string{}

		keyGen := func(team, ns, resource string) string {
			return fmt.Sprintf("%s:%s/%s", team, ns, resource)
		}

		for _, team := range tenantTeamList {
			for _, ns := range namespaceList {
				actualVerbsByResource := getActualVerbs(team, ns)

				// check secrets
				for _, resource := range []string{"secrets", "sealedsecrets.bitnami.com"} {
					key := keyGen(team, ns, resource)

					if ns == "sandbox" || nsOwner[ns] == team || (team == "maneki" && nsOwner[ns] != "neco") {
						expectedVerbs[key] = adminVerbs
					} else {
						expectedVerbs[key] = prohibitedVerbs
					}

					if v, ok := actualVerbsByResource[resource]; ok {
						actualVerbs[key] = v
					} else {
						actualVerbs[key] = prohibitedVerbs
					}
				}

				// check required resources
				for _, resource := range requiredResources {
					key := keyGen(team, ns, resource)

					if ns == "sandbox" || nsOwner[ns] == team || (team == "maneki" && nsOwner[ns] != "neco") {
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

				// check viewable resources
				for _, resource := range viewableResources {
					key := keyGen(team, ns, resource)
					expectedVerbs[key] = viewVerbs

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

				// check viewable cluster resources
				for _, resource := range viewableClusterResources {
					key := keyGen(team, ns, resource)
					expectedVerbs[key] = viewVerbs

					if v, ok := actualVerbsByResource[resource]; ok {
						actualVerbs[key] = v
					} else {
						actualVerbs[key] = prohibitedVerbs
					}
				}

				// check prohibited cluster resources
				for _, resource := range prohibitedClusterResources {
					v, ok := actualVerbsByResource[resource]
					Expect(ok).To(BeFalse(), "unexpected permission was attached. name: %v. permission: %v", resource, v)
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
		for _, team := range tenantTeamList {
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
	})

	It("should give authority of ephemeral containers to unprivileged team", func() {
		By("creating test pod")
		stdout, stderr, err := ExecAt(boot0, "kubectl", "run", "-n", "maneki", "neco-ephemeral-test", "--image=quay.io/cybozu/ubuntu-debug:20.04", "pause")
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
		stdout, stderr, err = ExecAt(boot0, "kubectl", "alpha", "debug", "-i", "-n", "maneki", "neco-ephemeral-test", "--image=quay.io/cybozu/ubuntu-debug:20.04", "--target=neco-ephemeral-test", "--as=test", "--as-group=maneki", "--as-group=system:authenticated", "--", "echo a")
		Expect(err).NotTo(HaveOccurred(), "stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
	})
}
