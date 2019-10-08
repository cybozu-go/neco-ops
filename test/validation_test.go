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

type crdValidation struct {
	Kind   string                                               `json:"kind"`
	Status *apiextensionsv1beta1.CustomResourceDefinitionStatus `json:"status"`
}

type secret struct {
	Metadata struct {
		Name string `json:"name"`
	} `json:"metadata"`
}

func TestValidation(t *testing.T) {
	rootDir, err := filepath.Abs(manifestDir)
	if err != nil {
		t.Fatal(err)
	}

	excludeDirs := []string{
		filepath.Join(rootDir, "bin"),
		filepath.Join(rootDir, "docs"),
		filepath.Join(rootDir, "test"),
		filepath.Join(rootDir, "vendor"),
	}

	err = filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
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

func TestSecret(t *testing.T) {
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
		name := es.Metadata.Name

		rootDir := "../"
		excludeDirs := []string{
			filepath.Join(rootDir, "bin"),
			filepath.Join(rootDir, "docs"),
			filepath.Join(rootDir, "test"),
			filepath.Join(rootDir, "vendor"),
		}

		var appeared bool
		err = filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
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

			if strings.Contains(string(str), "secretName: "+name) {
				appeared = true
			}
			return nil
		})
		if err != nil {
			t.Fatal("failed to walk manifest directories")
		}
		if !appeared {
			t.Error("secret:", name, "was not found in any manifests")
		}

		for _, cs := range dummySecrets {
			if cs.Metadata.Name == name {
				continue OUTER
			}
		}
		t.Error("secret:", name, "was not found in dummy secrets", dummySecrets)
	}
}
