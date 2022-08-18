package main

import (
	"os"

	"kustomize-plugin-demo/rdgen/generator"
	"sigs.k8s.io/kustomize/kyaml/fn/framework"
	"sigs.k8s.io/kustomize/kyaml/fn/framework/command"
	"sigs.k8s.io/kustomize/kyaml/kio"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

func main() {
	config := new(generator.ResourceDistributionPlugin)
	fn := func(items []*yaml.RNode) ([]*yaml.RNode, error) {
		rn, err := generator.MakeResourceDistribution(config)
		if err != nil {
			return nil, err
		}

		var itemsOutput []*yaml.RNode
		itemsOutput = append(itemsOutput, rn)
		return itemsOutput, nil
	}

	// TODO 看懂
	p := framework.SimpleProcessor{Config: config, Filter: kio.FilterFunc(fn)}
	cmd := command.Build(p, command.StandaloneDisabled, false)
	//command.AddGenerateDockerfile(cmd)
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
