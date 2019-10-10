package test

import (
	"bufio"
	"errors"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
	Kind   string                                               `json:"kind"`
	Status *apiextensionsv1beta1.CustomResourceDefinitionStatus `json:"status"`
}

type secret struct {
	Metadata struct {
		Name string `json:"name"`
	} `json:"metadata"`
}

func testCRDStatus(t *testing.T) {
	t.Parallel()

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

		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()

		y := k8sYaml.NewYAMLReader(bufio.NewReader(f))
		for {
			data, err := y.Read()
			if err == io.EOF {
				break
			} else if err != nil {
				return err
			}

			var crd crdValidation
			err = yaml.Unmarshal(data, &crd)
			if err != nil {
				// return nil
				// Skip because this YAML might not be custom resource definition
				return nil
			}

			if crd.Kind != "CustomResourceDefinition" {
				// Skip because this YAML is not custom resource definition
				return nil
			}

			if crd.Status != nil {
				return errors.New(".status(Status) exists in " + path + ", remove it to prevent occurring OutOfSync by Argo CD")
			}
		}

		return nil
	})
	if err != nil {
		t.Error(err)
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

func TestValidation(t *testing.T) {
	if os.Getenv("SSH_PRIVKEY") != "" {
		t.Skip("SSH_PRIVKEY envvar is defined as running e2e test")
	}

	t.Run("CRDStatus", testCRDStatus)
	t.Run("GeneratedSecretName", testGeneratedSecretName)
}
