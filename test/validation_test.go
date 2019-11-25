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
	"strings"
	"testing"
	"text/template"

	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	k8sYaml "k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/yaml"
)

const (
	manifestDir        = "../"
	expectedSecretFile = "./expected-secret.yaml"
	currentSecretFile  = "./current-secret.yaml"
)

var (
	excludeDirs = []string{
		filepath.Join(manifestDir, "bin"),
		filepath.Join(manifestDir, "docs"),
		filepath.Join(manifestDir, "test"),
		filepath.Join(manifestDir, "vendor"),
	}
)

type crdValidation struct {
	Kind     string `json:"kind"`
	Metadata struct {
		Name string `json:"name"`
	} `json:"metadata"`
	Status *apiextensionsv1beta1.CustomResourceDefinitionStatus `json:"status"`
}

type secret struct {
	Metadata struct {
		Name string `json:"name"`
	} `json:"metadata"`
}

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

func testCRDStatus(t *testing.T) {
	t.Parallel()

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

				var crd crdValidation
				err = yaml.Unmarshal(data, &crd)
				if err != nil {
					// Skip because this YAML might not be custom resource definition
					continue
				}

				if crd.Kind != "CustomResourceDefinition" {
					// Skip because this YAML is not custom resource definition
					continue
				}
				if crd.Status != nil {
					t.Error(errors.New(".status(Status) exists in " + crd.Metadata.Name + ", remove it to prevent occurring OutOfSync by Argo CD"))
				}
			}
		})
	}
}

func readSecret(path string) ([]secret, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var secrets []secret
	y := k8sYaml.NewYAMLReader(bufio.NewReader(f))
	for {
		data, err := y.Read()
		if err == io.EOF {
			break
		} else if err != nil {
			return nil, err
		}

		var s secret
		err = yaml.Unmarshal(data, &s)
		if err != nil {
			return nil, err
		}
		secrets = append(secrets, s)
	}
	return secrets, nil
}

func testGeneratedSecretName(t *testing.T) {
	t.Parallel()

	defer func() {
		os.Remove(expectedSecretFile)
		os.Remove(currentSecretFile)
	}()

	expected, err := readSecret(expectedSecretFile)
	if err != nil {
		t.Fatal(err)
	}
	dummySecrets, err := readSecret(currentSecretFile)
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

			if strings.Contains(string(str), "secretName: "+es.Metadata.Name) {
				appeared = true
			}
			return nil
		})
		if err != nil {
			t.Fatal("failed to walk manifest directories")
		}
		if !appeared {
			t.Error("secret:", es.Metadata.Name, "was not found in any manifests")
		}

		for _, cs := range dummySecrets {
			if cs.Metadata.Name == es.Metadata.Name {
				continue OUTER
			}
		}
		t.Error("secret:", es.Metadata.Name, "was not found in dummy secrets", dummySecrets)
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

	t.Run("CRDStatus", testCRDStatus)
	t.Run("GeneratedSecretName", testGeneratedSecretName)
	t.Run("AlertRules", testAlertRules)
}
