package generator

import (
	"encoding/base64"
	"github.com/go-errors/errors"
	"sigs.k8s.io/kustomize/api/ifc"
	"sigs.k8s.io/kustomize/api/types"
	"sigs.k8s.io/kustomize/kyaml/yaml"
	"strings"
	"unicode/utf8"
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
				"configmap %s illegally repeats the key `%s`", name, p.Key)
		}
		knownKeys[p.Key] = p.Value
	}
	return knownKeys, nil
}

func loadMapIntoConfigMapData(m map[string]string, rn *yaml.RNode) error {
	for _, k := range yaml.SortedMapKeys(m) {
		fldName, vrN := makeConfigMapValueRNode(m[k])
		if _, err := rn.Pipe(
			yaml.LookupCreate(yaml.MappingNode, append(resourcePath, fldName)...),
			yaml.SetField(k, vrN)); err != nil {
			return err
		}
	}
	return nil
}

func makeConfigMapValueRNode(s string) (field string, rN *yaml.RNode) {
	yN := &yaml.Node{Kind: yaml.ScalarNode}
	yN.Tag = yaml.NodeTagString
	if utf8.ValidString(s) {
		field = yaml.DataField
		yN.Value = s
	} else {
		field = yaml.BinaryDataField
		yN.Value = encodeBase64(s)
	}
	if strings.Contains(yN.Value, "\n") {
		yN.Style = yaml.LiteralStyle
	}
	return field, yaml.NewRNode(yN)
}

func encodeBase64(s string) string {
	const lineLen = 70
	encLen := base64.StdEncoding.EncodedLen(len(s))
	lines := encLen/lineLen + 1
	buf := make([]byte, encLen*2+lines)
	in := buf[0:encLen]
	out := buf[encLen:]
	base64.StdEncoding.Encode(in, []byte(s))
	k := 0
	for i := 0; i < len(in); i += lineLen {
		j := i + lineLen
		if j > len(in) {
			j = len(in)
		}
		k += copy(out[k:], in[i:j])
		if lines > 1 {
			out[k] = '\n'
			k++
		}
	}
	return string(out[:k])
}

func loadMapIntoSecretData(m map[string]string, rn *yaml.RNode) error {
	mapNode, err := rn.Pipe(yaml.LookupCreate(yaml.MappingNode, append(resourcePath, yaml.DataField)...))
	if err != nil {
		return err
	}
	for _, k := range yaml.SortedMapKeys(m) {
		vrN := makeSecretValueRNode(m[k])
		if _, err := mapNode.Pipe(yaml.SetField(k, vrN)); err != nil {
			return err
		}
	}
	return nil
}

func makeSecretValueRNode(s string) *yaml.RNode {
	yN := &yaml.Node{Kind: yaml.ScalarNode}
	// Purposely don't use YAML tags to identify the data as being plain text or
	// binary.  It kubernetes Secrets the values in the `data` map are expected
	// to be base64 encoded, and in ConfigMaps that same can be said for the
	// values in the `binaryData` field.
	yN.Tag = yaml.NodeTagString
	yN.Value = encodeBase64(s)
	if strings.Contains(yN.Value, "\n") {
		yN.Style = yaml.LiteralStyle
	}
	return yaml.NewRNode(yN)
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
