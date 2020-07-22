package test

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"text/template"

	argocd "github.com/argoproj/argo-cd/pkg/apis/application/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
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
		filepath.Join(manifestDir, "vendor"),
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
	cmd := exec.Command("kustomize", "build", dir)
	cmd.Stdout = outBuf
	cmd.Stderr = errBuf
	err := cmd.Run()
	return outBuf.Bytes(), errBuf.Bytes(), err
}

func testApplicationResources(t *testing.T) {
	targetRevisions := map[string]string{
		"gcp":    "release",
		"osaka0": "release",
		"stage0": "stage",
		"tokyo0": "release",
	}

	syncWaves := map[string]string{
		"namespaces":           "1",
		"argocd":               "2",
		"local-pv-provisioner": "3",
		"secrets":              "3",
		"cert-manager":         "4",
		"external-dns":         "4",
		"metallb":              "4",
		"ingress":              "5",
		"topolvm":              "5",
		"unbound":              "5",
		"elastic":              "6",
		"rook":                 "6",
		"monitoring":           "7",
		"sandbox":              "7",
		"teleport":             "7",
		"network-policy":       "8",
		"argocd-ingress":       "9",
		"bmc-reverse-proxy":    "9",
		"metrics-server":       "9",
		"neco-admission":       "9",
		"team-management":      "9",
		"maneki-apps":          "10",
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
				t.Error(fmt.Errorf("kustomize build faled. path: %s, stderr: %s, err: %v", targetDir, stderr, err))
			}

			y := k8sYaml.NewYAMLReader(bufio.NewReader(bytes.NewReader(stdout)))
			for {
				data, err := y.Read()
				if err == io.EOF {
					break
				} else if err != nil {
					t.Error(err)
				}

				var app argocd.Application
				err = yaml.Unmarshal(data, &app)
				if err != nil {
					t.Error(err)
				}

				// Check the tergetRevision
				if targetRevisions[overlay] == "" {
					t.Error(fmt.Errorf("targetRevision should exist. overlay: %s", overlay))
				}
				if app.Spec.Source.TargetRevision != targetRevisions[overlay] {
					t.Error(fmt.Errorf("invalid targetRevision. application: %s, targetRevision: %s (should be %s)", app.Name, app.Spec.Source.TargetRevision, targetRevisions[overlay]))
				}

				// Check the sync wave
				if syncWaves[app.Name] == "" {
					t.Error(fmt.Errorf("sync-wave should exist. application: %s", app.Name))
				}
				if app.GetAnnotations()["argocd.argoproj.io/sync-wave"] != syncWaves[app.Name] {
					t.Error(fmt.Errorf("invalid sync-wave. application: %s, sync-wave: %s (should be %s)", app.Name, app.GetAnnotations()["argocd.argoproj.io/sync-wave"], syncWaves[app.Name]))
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
	Status *apiextensionsv1beta1.CustomResourceDefinitionStatus `json:"status"`
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
			t.Error(errors.New(".status(Status) exists in " + crd.Metadata.Name + ", remove it to prevent occurring OutOfSync by Argo CD"))
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
		if !reflect.DeepEqual(cert.Spec.Usages, expected) {
			t.Error(errors.New(".spec.usages has incorrect list in " + cert.Metadata.Name))
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
				t.Error(fmt.Errorf("kustomize build faled. path: %s, stderr: %s, err: %v", path, stderr, err))
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

func readSecret(path string) ([]corev1.Secret, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var secrets []corev1.Secret
	y := k8sYaml.NewYAMLReader(bufio.NewReader(f))
	for {
		data, err := y.Read()
		if err == io.EOF {
			break
		} else if err != nil {
			return nil, err
		}

		var s corev1.Secret
		err = yaml.Unmarshal(data, &s)
		if err != nil {
			return nil, err
		}
		secrets = append(secrets, s)
	}
	return secrets, nil
}

func testGeneratedSecretName(t *testing.T) {
	const currentSecretFile = "./current-secret.yaml"
	expectedSecretFiles := []string{
		"./expected-secret-osaka0.yaml",
		"./expected-secret-stage0.yaml",
		"./expected-secret-tokyo0.yaml",
	}

	t.Parallel()

	defer func() {
		for _, f := range expectedSecretFiles {
			os.Remove(f)
		}
		os.Remove(currentSecretFile)
	}()

	dummySecrets, err := readSecret(currentSecretFile)
	if err != nil {
		t.Fatal(err)
	}

	for _, f := range expectedSecretFiles {
		expected, err := readSecret(f)
		if err != nil {
			t.Fatal(err)
		}

	OUTER:
		for _, es := range expected {
			var appeared bool
			err = filepath.Walk(manifestDir, func(path string, info os.FileInfo, err error) error {
				if err != nil {
					return err
				}
				for _, exDir := range excludeDirs {
					if strings.HasPrefix(path, exDir) {
						// Skip files in the directory
						return filepath.SkipDir
					}
				}
				if info.IsDir() || !strings.HasSuffix(path, ".yaml") {
					return nil
				}
				str, err := ioutil.ReadFile(path)
				if err != nil {
					return err
				}

				// These lines test all secrets to be used.
				// grafana-admin-credentials is skipped because it is used internally in Grafana Operator.
				if es.Name == "grafana-admin-credentials" {
					appeared = true
				}
				if strings.Contains(string(str), "secretName: "+es.Name) {
					appeared = true
				}
				// These lines test secrets to be used as references, such like:
				// - secretRef:
				//     name: <key>
				strCondensed := strings.Join(strings.Fields(string(str)), "")
				if strings.Contains(strCondensed, "secretRef:name:"+es.Name) {
					appeared = true
				}
				return nil
			})
			if err != nil {
				t.Fatal("failed to walk manifest directories")
			}
			if !appeared {
				t.Error("secret:", es.Name, "was not found in any manifests")
			}

			for _, cs := range dummySecrets {
				if cs.Name == es.Name && cs.Namespace == es.Namespace {
					continue OUTER
				}
			}
			t.Error("secret:", es.Namespace+"/"+es.Name, "was not found in dummy secrets")
		}
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

func TestValidation(t *testing.T) {
	if os.Getenv("SSH_PRIVKEY") != "" {
		t.Skip("SSH_PRIVKEY envvar is defined as running e2e test")
	}

	t.Run("ApplicationTargetRevision", testApplicationResources)
	t.Run("CRDStatus", testCRDStatus)
	t.Run("CertificateUsages", testCertificateUsages)
	t.Run("GeneratedSecretName", testGeneratedSecretName)
	t.Run("AlertRules", testAlertRules)
}
