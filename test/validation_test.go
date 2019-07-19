package test

import (
	"bufio"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	//	. "github.com/onsi/ginkgo"
	//	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v2"
	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	k8sYaml "k8s.io/apimachinery/pkg/util/yaml"
)

const (
	manifestDir = "../"
)

type crdValidation struct {
	Kind   string                                               `json:"kind"`
	Status *apiextensionsv1beta1.CustomResourceDefinitionStatus `json:"status"`
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
