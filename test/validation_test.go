package test

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	k8sYaml "k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/yaml"
)

const (
	manifestDir = "../"
)

var (
	excludeDirs = []string{
		filepath.Join(manifestDir, "bin"),
		filepath.Join(manifestDir, "docs"),
		filepath.Join(manifestDir, "test"),
	}
)

func isKustomizationFile(name string) bool {
	if name == "kustomization.yaml" || name == "kustomization.yml" || name == "Kustomization" {
		return true
	}
	return false
}

func kustomizeBuild(dir string) ([]byte, []byte, error) {
	outBuf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	workdir, err := os.Getwd()
	if err != nil {
		return nil, nil, err
	}
	cmd := exec.Command(filepath.Join(workdir, "bin", "kustomize"), "build", dir)
	cmd.Stdout = outBuf
	cmd.Stderr = errBuf
	err = cmd.Run()
	return outBuf.Bytes(), errBuf.Bytes(), err
}

func testNamespaceResources(t *testing.T) {
	t.Parallel()

	// All namespaces defined in neco-apps should have the `team` label.
	// Exceptionally, `sandbox` ns should not have the `team` label.
	doCheckKustomizedYaml(t, func(t *testing.T, data []byte) {
		var meta struct {
			metav1.TypeMeta   `json:",inline"`
			metav1.ObjectMeta `json:"metadata,omitempty"`
		}
		err := yaml.Unmarshal(data, &meta)
		if err != nil {
			t.Fatal(err)
		}
		if meta.Kind != "Namespace" {
			return
		}

		// `sandbox` namespace should not have a team label.
		if meta.Name == "sandbox" {
			if _, ok := meta.Labels["team"]; ok {
				t.Errorf("sandbox ns has team label: value=%s", meta.Labels["team"])
			}
			return
		}

		// other namespace should have a team label.
		if meta.Labels["team"] == "" {
			t.Errorf("%s ns doesn't have team label", meta.Name)
		}
	})
}

func testAppProjectResources(t *testing.T) {
	// Verify the destination namespaces in the AppPorject for unprivileged team are listed correctly.
	targetDir := filepath.Join(manifestDir, "team-management", "base")

	namespacesByTeam := map[string][]string{}
	namespacesInAppProject := map[string][]string{}

	stdout, stderr, err := kustomizeBuild(targetDir)
	if err != nil {
		t.Fatalf("kustomize build failed. path: %s, stderr: %s, err: %v", targetDir, stderr, err)
	}
	y := k8sYaml.NewYAMLReader(bufio.NewReader(bytes.NewReader(stdout)))
	for {
		data, err := y.Read()
		if err == io.EOF {
			break
		} else if err != nil {
			t.Fatal(err)
		}

		var meta struct {
			metav1.TypeMeta   `json:",inline"`
			metav1.ObjectMeta `json:"metadata,omitempty"`
		}
		err = yaml.Unmarshal(data, &meta)
		if err != nil {
			t.Fatal(err)
		}

		// Make lists from each resources.
		switch meta.Kind {
		case "Namespace":
			if meta.Name == "sandbox" {
				// Skip. sandbox ns does not have team label.
				continue
			}

			team := meta.Labels["team"]
			namespacesByTeam[team] = append(namespacesByTeam[team], meta.Name)

		case "AppProject":
			if meta.Name == "default" || meta.Name == "tenant-app-of-apps" || meta.Name == "tenant-apps" {
				// Skip. default app and tenant-app-of-apps app are privileged.
				continue
			}

			var proj AppProject
			err = yaml.Unmarshal(data, &proj)
			if err != nil {
				t.Fatal(err)
			}

			var namespaces []string
			for _, dest := range proj.Spec.Destinations {
				// The namespaces with dev- prefix is maneged by tenant teams via neco-tenant-apps.
				// As dev-* is to enable Argo CD to deploy resources to such namespaces, ignore this.
				if dest.Namespace == "dev-*" {
					continue
				}
				namespaces = append(namespaces, dest.Namespace)
			}

			// Some dev teams do not have their namespaces and are only accessible for sandbox and dev-* namespaces.
			if len(namespaces) == 1 && namespaces[0] == "sandbox" {
				continue
			}

			sort.Strings(namespaces)
			namespacesInAppProject[proj.Name] = namespaces
		}
	}

	for team, namespaces := range namespacesByTeam {
		namespaces = append(namespaces, "sandbox")
		sort.Strings(namespaces)
		namespacesByTeam[team] = namespaces
	}

	// List the all tenant namespaces.
	allNamespacesForTenantTeam := []string{}
	nsSet := map[string]bool{}
	for _, namespaces := range namespacesByTeam {
		for _, ns := range namespaces {
			nsSet[ns] = true
		}
	}
	for ns := range nsSet {
		allNamespacesForTenantTeam = append(allNamespacesForTenantTeam, ns)
	}
	sort.Strings(allNamespacesForTenantTeam)

	// The Maneki team can deploy to all tenant namespaces.
	namespacesByTeam["maneki"] = allNamespacesForTenantTeam

	if !cmp.Equal(namespacesByTeam, namespacesInAppProject) {
		t.Errorf("namespaces in AppProjects are not listed correctly: %s", cmp.Diff(namespacesByTeam, namespacesInAppProject))
	}
}

func testApplicationResources(t *testing.T) {
	syncWaves := map[string]string{
		"namespaces":             "1",
		"argocd":                 "2",
		"coil":                   "3",
		"local-pv-provisioner":   "3",
		"sealed-secrets":         "3",
		"secrets":                "3",
		"cert-manager":           "3",
		"external-dns":           "3",
		"metallb":                "3",
		"ingress":                "4",
		"neco-admission":         "4",
		"pod-security-admission": "4",
		"topolvm":                "4",
		"unbound":                "5",
		"argocd-ingress":         "5",
		"bmc-reverse-proxy":      "5",
		"customer-egress":        "5",
		"elastic":                "5",
		"moco":                   "5",
		"rook":                   "5",
		"sandbox":                "5",
		"team-management":        "5",
		"logging":                "6",
		"monitoring":             "6",
		"session-log":            "6",
		"teleport":               "6",
		"kube-metrics-adapter":   "7",
		"prometheus-adapter":     "7",
		"pvc-autoresizer":        "7",
		"network-policy":         "8",
	}

	targetRevisions := map[string]string{
		"gcp":      "release",
		"gcp-rook": "release",
		"neco-dev": "release",
		"osaka0":   "release",
		"stage0":   "stage",
		"tokyo0":   "release",
	}

	// List overlays
	var overlays []string
	entries, err := os.ReadDir(filepath.Join(manifestDir, "argocd-config/overlays"))
	if err != nil {
		t.Fatal(err)
	}
	for _, ent := range entries {
		info, err := ent.Info()
		if err != nil || !info.IsDir() {
			t.Fatal(err)
		}
		overlays = append(overlays, info.Name())
	}

	t.Parallel()
	for _, overlay := range overlays {
		targetDir := filepath.Join(manifestDir, "argocd-config/overlays", overlay)
		t.Run(overlay, func(t *testing.T) {
			stdout, stderr, err := kustomizeBuild(targetDir)
			if err != nil {
				t.Errorf("kustomize build failed. path: %s, stderr: %s, err: %v", targetDir, stderr, err)
			}

			y := k8sYaml.NewYAMLReader(bufio.NewReader(bytes.NewReader(stdout)))
			for {
				data, err := y.Read()
				if err == io.EOF {
					break
				} else if err != nil {
					t.Error(err)
				}

				var app Application
				err = yaml.Unmarshal(data, &app)
				if err != nil {
					t.Error(err)
				}

				if app.Name == "argocd-config" || app.Name == "prometheus-adapter" || app.GetLabels()["is-tenant"] == "true" {
					// Target revision for tenant apps is maintained in team-management/template/settings.json.
					continue
				}

				// Check the sync wave
				if syncWaves[app.Name] == "" {
					t.Errorf("expected sync-wave should be defined. application: %s", app.Name)
				}
				if app.GetAnnotations()["argocd.argoproj.io/sync-wave"] != syncWaves[app.Name] {
					t.Errorf("invalid sync-wave. application: %s, sync-wave: %s (should be %s)", app.Name, app.GetAnnotations()["argocd.argoproj.io/sync-wave"], syncWaves[app.Name])
				}

				// Check the targetRevision
				expectedTargetRevision := targetRevisions[overlay]

				if expectedTargetRevision == "" {
					t.Errorf("expected targetRevision should be defined. application: %s, overlay: %s", app.Name, overlay)
				}
				if app.Spec.Source.TargetRevision != expectedTargetRevision {
					t.Errorf("invalid targetRevision. application: %s, targetRevision: %s (should be %s)", app.Name, app.Spec.Source.TargetRevision, expectedTargetRevision)
				}
			}
		})
	}
}

// Use to check the existence of the status field in manifest files for CRDs.
// `apiextensionsv1beta1.CustomResourceDefinition` cannot be used because the status field always exists in the struct.
type crdValidation struct {
	Kind     string `json:"kind"`
	Metadata struct {
		Name string `json:"name"`
	} `json:"metadata"`
	Status *struct{} `json:"status"`
}

func testCRDStatus(t *testing.T) {
	t.Parallel()

	doCheckKustomizedYaml(t, func(t *testing.T, data []byte) {
		var crd crdValidation
		err := yaml.Unmarshal(data, &crd)
		if err != nil {
			// Skip because this YAML might not be custom resource definition
			return
		}

		if crd.Kind != "CustomResourceDefinition" {
			// Skip because this YAML is not custom resource definition
			return
		}
		if crd.Status != nil {
			t.Errorf(".status(Status) exists in %s, remove it to prevent occurring OutOfSync by Argo CD", crd.Metadata.Name)
		}
	})
}

func doCheckKustomizedYaml(t *testing.T, checkFunc func(*testing.T, []byte)) {
	targets := []string{}
	err := filepath.Walk(manifestDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		for _, exDir := range excludeDirs {
			if strings.HasPrefix(path, exDir) {
				// Skip files in the directory
				return filepath.SkipDir
			}
		}
		if !isKustomizationFile(info.Name()) {
			return nil
		}
		targets = append(targets, filepath.Dir(path))
		// Skip other files in the directory
		return filepath.SkipDir
	})
	if err != nil {
		t.Error(err)
	}

	for _, path := range targets {
		t.Run(path, func(t *testing.T) {
			stdout, stderr, err := kustomizeBuild(path)
			if err != nil {
				t.Errorf("kustomize build failed. path: %s, stderr: %s, err: %v", path, stderr, err)
			}

			y := k8sYaml.NewYAMLReader(bufio.NewReader(bytes.NewReader(stdout)))
			for {
				data, err := y.Read()
				if err == io.EOF {
					break
				} else if err != nil {
					t.Error(err)
				}

				checkFunc(t, data)
			}
		})
	}
}

type RelabelConfig struct {
	Action       string   `json:"action"`
	SourceLabels []string `json:"sourceLabels"`
	Regex        string   `json:"regex"`
	TargetLabel  string   `json:"targetLabel"`
	Replacement  string   `json:"replacement"`
}

type resourceMeta struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
}

// shrinked and merged version of VMServiceScrape, VMPodScrape and VMNodeScrape
type VMScrapeOrRule struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              struct {
		Selector               metav1.LabelSelector `json:"selector"`
		ServiceScrapeEndpoints []struct {
			RelabelConfigs []RelabelConfig `json:"relabelConfigs"`
		} `json:"endpoints"`
		PodScrapeEndpoints []struct {
			RelabelConfigs []RelabelConfig `json:"relabelConfigs"`
		} `json:"podMetricsEndpoints"`
		NodeScrapeRelabelConfigs []RelabelConfig `json:"relabelConfigs"`
		// VMProbe is not used yet.
	} `json:"spec"`
}

// shrinked version of github.com/VictoriaMetrics/operator/api/v1beta1.VMAgent
type VMAgent struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              struct {
		ServiceScrapeSelector          *metav1.LabelSelector `json:"serviceScrapeSelector,omitempty"`
		ServiceScrapeNamespaceSelector *metav1.LabelSelector `json:"serviceScrapeNamespaceSelector,omitempty"`
		PodScrapeSelector              *metav1.LabelSelector `json:"podScrapeSelector,omitempty"`
		PodScrapeNamespaceSelector     *metav1.LabelSelector `json:"podScrapeNamespaceSelector,omitempty"`
		NodeScrapeSelector             *metav1.LabelSelector `json:"nodeScrapeSelector,omitempty"`
		NodeScrapeNamespaceSelector    *metav1.LabelSelector `json:"nodeScrapeNamespaceSelector,omitempty"`
		ProbeSelector                  *metav1.LabelSelector `json:"probeSelector,omitempty"`
		ProbeNamespaceSelector         *metav1.LabelSelector `json:"probeNamespaceSelector,omitempty"`
	} `json:"spec"`
}

// shrinked version of github.com/VictoriaMetrics/operator/api/v1beta1.VMAlert
type VMAlert struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              struct {
		RuleSelector          *metav1.LabelSelector `json:"ruleSelector,omitempty"`
		RuleNamespaceSelector *metav1.LabelSelector `json:"ruleNamespaceSelector,omitempty"`
	} `json:"spec"`
}

func testVMCustomResources(t *testing.T) {
	vmBaseDir := filepath.Join(manifestDir, "monitoring/base/victoriametrics")

	// expected resource names of each CRs which are handled by smallset cluster (must be sorted)
	expectedSmallsetServiceScrapes := []string{
		"kube-state-metrics",
		"kubernetes",
		"rook",
		"vmagent-largeset",
		"vmagent-smallset",
		"vmalert-largeset",
		"vmalert-smallset",
		"vmalertmanager-largeset",
		"vmalertmanager-smallset",
		"vminsert-largeset",
		"vmselect-largeset",
		"vmsingle-smallset",
		"vmstorage-largeset",
	}
	expectedSmallsetPodScrapes := []string{
		"kube-state-metrics-telemetry",
		"topolvm",
		"victoriametrics-operator",
	}
	expectedSmallsetNodeScrapes := []string{
		"kubernetes-cadvisor",
		"kubernetes-nodes",
	}
	expectedSmallsetProbes := []string{}
	expectedSmallsetRules := []string{
		"monitoring",
		"rook",
	}

	// gather CRs in files

	crsInFiles := []string{}
	err := filepath.Walk("../monitoring/base/victoriametrics/rules", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		file, err := os.Open(path)
		if err != nil {
			return err
		}
		reader := k8sYaml.NewYAMLReader(bufio.NewReader(file))
		for {
			data, err := reader.Read()
			if err == io.EOF {
				break
			} else if err != nil {
				return fmt.Errorf("failed to read yaml: %v", err)
			}
			var metaType string
			var r VMScrapeOrRule
			yaml.Unmarshal(data, &r)
			var relabelConfigs [][]RelabelConfig
			switch r.Kind {
			case "VMServiceScrape":
				metaType = "service"
				for _, ep := range r.Spec.ServiceScrapeEndpoints {
					relabelConfigs = append(relabelConfigs, ep.RelabelConfigs)
				}
			case "VMPodScrape":
				metaType = "pod"
				for _, ep := range r.Spec.PodScrapeEndpoints {
					relabelConfigs = append(relabelConfigs, ep.RelabelConfigs)
				}
			case "VMNodeScrape":
				metaType = "node"
				relabelConfigs = append(relabelConfigs, r.Spec.NodeScrapeRelabelConfigs)
			case "VMProbe":
			case "VMRule":
			default:
				continue
			}
			crsInFiles = append(crsInFiles, r.Kind+"/"+r.Name)

			metaLabelPrefix := "__meta_kubernetes_" + metaType + "_label_"
			for i, rcs := range relabelConfigs {
				found := false
				for _, rc := range rcs {
					if rc.Action == "" && rc.TargetLabel == "job" && rc.Replacement != "" && !strings.Contains(rc.Replacement, "/") {
						if !strings.HasSuffix(rc.Replacement, "$1") {
							found = true
							continue
						}

						if rc.Regex != "" || len(rc.SourceLabels) != 1 || !strings.HasPrefix(rc.SourceLabels[0], metaLabelPrefix) || len(r.Spec.Selector.MatchLabels) != 0 {
							continue
						}
						labelName := strings.TrimPrefix(rc.SourceLabels[0], metaLabelPrefix)
						for _, me := range r.Spec.Selector.MatchExpressions {
							if me.Key == labelName && me.Operator == metav1.LabelSelectorOpIn {
								found = true
								continue
							}
						}
					}
				}
				if !found {
					t.Errorf("%s %s endpoint %d should have a relabelConfig that set job label correctly", r.Kind, r.Name, i)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("failed to read CRs files: %v", err)
	}

	// gather CRs actually applied

	kustomizeResult, err := exec.Command("bin/kustomize", "build", vmBaseDir).Output()
	if err != nil {
		t.Fatalf("failed to kustomize build: %v", err)
	}

	reader := k8sYaml.NewYAMLReader(bufio.NewReader(bytes.NewReader(kustomizeResult)))

	var serviceScrapes []resourceMeta
	var podScrapes []resourceMeta
	var nodeScrapes []resourceMeta
	var probes []resourceMeta
	var rules []resourceMeta
	crsInKBuild := []string{}

	for {
		data, err := reader.Read()
		if err == io.EOF {
			break
		} else if err != nil {
			t.Fatalf("failed to read yaml: %v", err)
		}
		var r resourceMeta
		yaml.Unmarshal(data, &r)
		switch r.Kind {
		case "VMServiceScrape":
			serviceScrapes = append(serviceScrapes, r)
		case "VMPodScrape":
			podScrapes = append(podScrapes, r)
		case "VMNodeScrape":
			nodeScrapes = append(nodeScrapes, r)
		case "VMProbe":
			probes = append(probes, r)
		case "VMRule":
			rules = append(rules, r)
		default:
			continue
		}
		crsInKBuild = append(crsInKBuild, r.Kind+"/"+r.Name)
	}

	sort.Strings(crsInFiles)
	sort.Strings(crsInKBuild)
	if !reflect.DeepEqual(crsInFiles, crsInKBuild) {
		t.Errorf("some CRs mismatch: actual=%v, expected=%v", crsInFiles, crsInKBuild)
	}

	// read VMAgent/VMAlert CRs (their label selectors)

	file, err := os.Open(filepath.Join(vmBaseDir, "vmagent-smallset.yaml"))
	if err != nil {
		t.Fatalf("failed open vmagent-smallset.yaml: %v", err)
	}
	defer file.Close()
	reader = k8sYaml.NewYAMLReader(bufio.NewReader(file))
	var smallsetVMAgent *VMAgent
	for {
		data, err := reader.Read()
		if err == io.EOF {
			break
		} else if err != nil {
			t.Fatalf("failed to read yaml: %v", err)
		}
		var r VMAgent
		err = yaml.Unmarshal(data, &r)
		if err != nil {
			continue
		}
		if r.Kind == "VMAgent" && r.Name == "vmagent-smallset" {
			smallsetVMAgent = &r
			break
		}
	}
	if smallsetVMAgent == nil {
		t.Fatalf("failed to get vmagent-smallset")
	}

	file, err = os.Open(filepath.Join(vmBaseDir, "vmalert-smallset.yaml"))
	if err != nil {
		t.Fatalf("failed open vmalert-smallset.yaml: %v", err)
	}
	defer file.Close()
	reader = k8sYaml.NewYAMLReader(bufio.NewReader(file))
	var smallsetVMAlert *VMAlert
	for {
		data, err := reader.Read()
		if err == io.EOF {
			break
		} else if err != nil {
			t.Fatalf("failed to read yaml: %v", err)
		}
		var r VMAlert
		err = yaml.Unmarshal(data, &r)
		if err != nil {
			continue
		}
		if r.Kind == "VMAlert" && r.Name == "vmalert-smallset" {
			smallsetVMAlert = &r
			break
		}
	}
	if smallsetVMAlert == nil {
		t.Fatalf("failed to get vmalert-smallset")
	}

	// check namespace label selectors
	if smallsetVMAgent.Spec.ServiceScrapeNamespaceSelector != nil ||
		smallsetVMAgent.Spec.PodScrapeNamespaceSelector != nil ||
		smallsetVMAgent.Spec.NodeScrapeNamespaceSelector != nil ||
		smallsetVMAgent.Spec.ProbeNamespaceSelector != nil ||
		smallsetVMAlert.Spec.RuleNamespaceSelector != nil {
		t.Errorf("bad namespace selector")
	}

	// filter CRs by label selectors and check the results

	selections := []struct {
		Name     string
		Selector *metav1.LabelSelector
		Objects  []resourceMeta
		Expected []string
	}{
		{
			Name:     "VMServiceScrape",
			Selector: smallsetVMAgent.Spec.ServiceScrapeSelector,
			Objects:  serviceScrapes,
			Expected: expectedSmallsetServiceScrapes,
		},
		{
			Name:     "VMPodScrape",
			Selector: smallsetVMAgent.Spec.PodScrapeSelector,
			Objects:  podScrapes,
			Expected: expectedSmallsetPodScrapes,
		},
		{
			Name:     "VMNodeScrape",
			Selector: smallsetVMAgent.Spec.NodeScrapeSelector,
			Objects:  nodeScrapes,
			Expected: expectedSmallsetNodeScrapes,
		},
		{
			Name:     "VMProbe",
			Selector: smallsetVMAgent.Spec.ProbeSelector,
			Objects:  probes,
			Expected: expectedSmallsetProbes,
		},
		{
			Name:     "VMRule",
			Selector: smallsetVMAlert.Spec.RuleSelector,
			Objects:  rules,
			Expected: expectedSmallsetRules,
		},
	}

	for _, selection := range selections {
		actual := []string{}
		selector, err := metav1.LabelSelectorAsSelector(selection.Selector)
		if err != nil {
			t.Errorf("cannot convert label selector: %v", err)
			continue
		}
		for _, r := range selection.Objects {
			if selector.Matches(labels.Set(r.Labels)) {
				actual = append(actual, r.Name)
			}
		}
		sort.Strings(actual)
		if !reflect.DeepEqual(actual, selection.Expected) {
			t.Errorf("smallset %s mismatch: actual=%v, expected=%v", selection.Name, actual, selection.Expected)
			continue
		}
	}
}

func TestValidation(t *testing.T) {
	if os.Getenv("SSH_PRIVKEY") != "" {
		t.Skip("SSH_PRIVKEY envvar is defined as running e2e test")
	}

	t.Run("AppProjectNamespaces", testAppProjectResources)
	t.Run("ApplicationTargetRevision", testApplicationResources)
	t.Run("CRDStatus", testCRDStatus)
	t.Run("NamespaceLabels", testNamespaceResources)
	t.Run("VictoriaMetricsCustomResources", testVMCustomResources)
}
