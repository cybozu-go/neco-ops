package main

import (
	"bufio"
	"fmt"
	"io"
	"os"

	k8syaml "k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/yaml"
)

type VMRule struct {
	Spec struct {
		Groups []interface{} `json:"groups"`
	} `json:"spec"`
}

type Rules struct {
	Groups []interface{} `json:"groups"`
}

func main() {
	reader := k8syaml.NewYAMLReader(bufio.NewReader(os.Stdin))

	var outRules Rules

	for {
		data, err := reader.Read()
		if err == io.EOF {
			break
		} else if err != nil {
			fmt.Fprintf(os.Stderr, "Read failed: %v\n", err)
			os.Exit(1)
		}

		var rule VMRule
		err = yaml.Unmarshal(data, &rule)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Unmarshal failed: %v\n", err)
			os.Exit(1)
		}

		outRules.Groups = append(outRules.Groups, rule.Spec.Groups...)
	}

	b, err := yaml.Marshal(outRules)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Marshal failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("%s", b)
}
