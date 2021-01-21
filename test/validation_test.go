package test

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"text/template"

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
				namespaces = append(namespaces, dest.Namespace)
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
		"namespaces":           "1",
		"argocd":               "2",
		"coil":                 "3",
		"local-pv-provisioner": "3",
		"secrets":              "3",
		"cert-manager":         "4",
		"external-dns":         "4",
		"metallb":              "4",
		"ingress":              "5",
		"topolvm":              "5",
		"unbound":              "5",
		"elastic":              "6",
		"moco":                 "6",
		"rook":                 "6",
		"monitoring":           "7",
		"registry-elastic":     "7",
		"registry-ghcr":        "7",
		"registry-quay":        "7",
		"sandbox":              "7",
		"teleport":             "7",
		"pvc-autoresizer":      "8",
		"argocd-ingress":       "8",
		"bmc-reverse-proxy":    "8",
		"metrics-server":       "8",
		"team-management":      "8",
		"customer-egress":      "8",
		"neco-admission":       "8",
		"network-policy":       "9",
		"tenant-apps":          "11",
		"ept-apps":             "12",
		"maneki-apps":          "12",
	}

	necoAppsTargetRevisions := map[string]string{
		"gcp":      "release",
		"gcp-rook": "release",
		"neco-dev": "release",
		"osaka0":   "release",
		"stage0":   "stage",
		"tokyo0":   "release",
	}
	tenantAppsTargetRevisions := map[string]map[string]string{
		"ept-apps": {
			"stage0": "main",
		},
		"maneki-apps": {
			"osaka0": "release",
			"stage0": "stage",
			"tokyo0": "release",
		},
		"tenant-apps": {
			"stage0": "stage",
		},
	}

	// Getting overlays list
	overlayDirs := map[string]string{}
	err := filepath.Walk(filepath.Join(manifestDir, "argocd-config", "overlays"), func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() && info.Name() != "overlays" {
			overlayDirs[info.Name()] = path
		}
		return nil
	})
	if err != nil {
		t.Error(err)
	}

	t.Parallel()
	for overlay, targetDir := range overlayDirs {
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

				// Check the sync wave
				if syncWaves[app.Name] == "" {
					t.Errorf("expected sync-wave should be defined. application: %s", app.Name)
				}
				if app.GetAnnotations()["argocd.argoproj.io/sync-wave"] != syncWaves[app.Name] {
					t.Errorf("invalid sync-wave. application: %s, sync-wave: %s (should be %s)", app.Name, app.GetAnnotations()["argocd.argoproj.io/sync-wave"], syncWaves[app.Name])
				}

				// Check the tergetRevision
				var expectedTargetRevision string
				if app.GetLabels()["is-tenant"] == "true" {
					expectedTargetRevision = tenantAppsTargetRevisions[app.Name][overlay]
				} else {
					expectedTargetRevision = necoAppsTargetRevisions[overlay]
				}

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

type certificateValidation struct {
	Kind     string `json:"kind"`
	Metadata struct {
		Name string `json:"name"`
	} `json:"metadata"`
	Spec struct {
		IsCA   bool     `json:"isCA"`
		Usages []string `json:"usages"`
	} `json:"spec"`
}

func testCertificateUsages(t *testing.T) {
	t.Parallel()

	doCheckKustomizedYaml(t, func(t *testing.T, data []byte) {
		var cert certificateValidation
		err := yaml.Unmarshal(data, &cert)
		if err != nil {
			// Skip because this YAML might not be certificate
			return
		}

		if cert.Kind != "Certificate" {
			// Skip because this YAML is not certificate
			return
		}

		var expected []string
		if cert.Spec.IsCA {
			expected = []string{"digital signature", "key encipherment", "cert sign"}
		} else {
			expected = []string{"digital signature", "key encipherment", "server auth", "client auth"}
		}
		if !cmp.Equal(cert.Spec.Usages, expected) {
			t.Errorf(".spec.usages has incorrect list in %s: %s", cert.Metadata.Name, cmp.Diff(cert.Spec.Usages, expected))
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

// These struct types are copied from the following link:
// https://github.com/prometheus/prometheus/blob/master/pkg/rulefmt/rulefmt.go

type alertRuleGroups struct {
	Groups []alertRuleGroup `json:"groups"`
}

type alertRuleGroup struct {
	Name   string      `json:"name"`
	Alerts []alertRule `json:"rules"`
}

type alertRule struct {
	Record      string            `json:"record,omitempty"`
	Alert       string            `json:"alert,omitempty"`
	Expr        string            `json:"expr"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations"`
}

type recordRuleGroups struct {
	Groups []recordRuleGroup `json:"groups"`
}

type recordRuleGroup struct {
	Name    string       `json:"name"`
	Records []recordRule `json:"rules"`
}

type recordRule struct {
	Record string `json:"record,omitempty"`
}

func testAlertRules(t *testing.T) {
	var groups alertRuleGroups

	str, err := ioutil.ReadFile("../monitoring/base/alertmanager/neco.template")
	if err != nil {
		t.Fatal(err)
	}
	tmpl := template.Must(template.New("alert").Parse(string(str))).Option("missingkey=error")

	err = filepath.Walk("../monitoring/base/prometheus/alert_rules", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		str, err := ioutil.ReadFile(path)
		if err != nil {
			return err
		}

		err = yaml.Unmarshal(str, &groups)
		if err != nil {
			return fmt.Errorf("failed to unmarshal %s, err: %v", path, err)
		}

		for _, g := range groups.Groups {
			t.Run(g.Name, func(t *testing.T) {
				t.Parallel()
				var buf bytes.Buffer
				err := tmpl.ExecuteTemplate(&buf, "slack.neco.text", g)
				if err != nil {
					t.Error(err)
				}
			})
		}

		return nil
	})
	if err != nil {
		t.Error(err)
	}
}

type resourceMeta struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
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
		"registry",
		"rook",
		"vmagent-largeset",
		"vmagent-smallset",
		"vmalert-largeset",
		"vmalert-smallset",
		"vmalertmanager",
		"vminsert-largeset",
		"vmselect-largeset",
		"vmsingle-smallset",
		"vmstorage-largeset",
	}
	expectedSmallsetPodScrapes := []string{
		"topolvm",
		"victoriametrics-operator",
	}
	expectedSmallsetNodeScrapes := []string{
		"kubernetes-cadvisor",
		"kubernetes-nodes",
	}
	expectedSmallsetProbes := []string{}
	expectedSmallsetRules := []string{
		"kube-state-metrics",
		"kubernetes",
		"monitoring",
		"topolvm",
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
		}
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
	t.Run("CertificateUsages", testCertificateUsages)
	t.Run("AlertRules", testAlertRules)
	t.Run("NamespaceLabels", testNamespaceResources)
	t.Run("VictoriaMetricsCustomResources", testVMCustomResources)
}
