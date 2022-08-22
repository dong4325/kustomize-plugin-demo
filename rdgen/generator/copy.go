package generator

import (
	"github.com/go-errors/errors"
	"sigs.k8s.io/kustomize/api/ifc"
	"sigs.k8s.io/kustomize/api/types"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

func makeValidatedDataMap(
	ldr ifc.KvLoader, name string, sources types.KvPairSources) (map[string]string, error) {
	pairs, err := ldr.Load(sources)
	if err != nil {
		return nil, errors.WrapPrefix(err, "loading KV pairs", 0)
	}
	knownKeys := make(map[string]string)
	for _, p := range pairs {
		// legal key: alphanumeric characters, '-', '_' or '.'
		if err := ldr.Validator().ErrIfInvalidKey(p.Key); err != nil {
			return nil, err
		}
		if _, ok := knownKeys[p.Key]; ok {
			return nil, errors.Errorf(
				"configmap  %s illegally repeats the key `%s`", name, p.Key)
		}
		knownKeys[p.Key] = p.Value
	}
	return knownKeys, nil
}

// NewListRNode returns a new List *RNode containing the provided scalar values.
func newNameListRNode(values ...string) *yaml.RNode {

	matchSeq := &yaml.Node{Kind: yaml.SequenceNode}
	for _, v := range values {
		node := &yaml.Node{
			Kind: yaml.MappingNode,
		}
		node.Content = append(node.Content, &yaml.Node{
			Kind:  yaml.ScalarNode,
			Value: nameField,
		}, &yaml.Node{
			Kind:  yaml.ScalarNode,
			Value: v,
		})
		matchSeq.Content = append(matchSeq.Content, node)

	}
	return yaml.NewRNode(matchSeq)
}
