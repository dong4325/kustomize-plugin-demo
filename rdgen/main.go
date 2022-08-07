package main

import (
	"encoding/base64"
	"github.com/go-errors/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"os"
	"sigs.k8s.io/kustomize/api/ifc"
	"sigs.k8s.io/kustomize/api/kv"
	"sigs.k8s.io/kustomize/api/loader"
	"sigs.k8s.io/kustomize/api/provider"
	"sigs.k8s.io/kustomize/api/types"
	"sigs.k8s.io/kustomize/kyaml/filesys"
	"sigs.k8s.io/kustomize/kyaml/fn/framework"
	"sigs.k8s.io/kustomize/kyaml/fn/framework/command"
	"sigs.k8s.io/kustomize/kyaml/kio"
	"sigs.k8s.io/kustomize/kyaml/yaml"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

type resourceDistributionPlugin struct {
	types.ObjectMeta `json:"metadata,omitempty" yaml:"metadata,omitempty"`
	ResourceArgs     `json:"resource,omitempty" yaml:"resource,omitempty"`
	TargetsArgs      `json:"targets,omitempty" yaml:"targets,omitempty"`

	// Options for the resourcedistribution.
	// GeneratorOptions same as configmap and secret generator Options
	Options *types.GeneratorOptions `json:"options,omitempty" yaml:"options,omitempty"`

	// Behavior of generated resource, must be one of:
	//   'create': create a new one
	//   'replace': replace the existing one
	//   'merge': merge with the existing one
	Behavior string `json:"behavior,omitempty" yaml:"behavior,omitempty"`
}

// ResourceArgs contain arguments for the resource to be distributed.
type ResourceArgs struct {
	// Name of the resource to be distributed.
	ResourceName string `json:"resourceName,omitempty" yaml:"resourceName,omitempty"`
	// Only configmap and secret are available
	ResourceKind string `json:"resourceKind,omitempty" yaml:"resourceKind,omitempty"`

	// KvPairSources defines places to obtain key value pairs.
	// same as configmap and secret generator KvPairSources
	types.KvPairSources `json:",inline,omitempty" yaml:",inline,omitempty"`

	// Options for the resource to be distributed.
	// GeneratorOptions same as configmap and secret generator Options
	ResourceOptions *types.GeneratorOptions `json:"resourceOptions,omitempty" yaml:"resourceOptions,omitempty"`

	// Type of the secret. It can be "Opaque" (default), or "kubernetes.io/tls".
	//
	// If type is "kubernetes.io/tls", then "literals" or "files" must have exactly two
	// keys: "tls.key" and "tls.crt"
	Type string `json:"type,omitempty" yaml:"type,omitempty"`
}

// TargetsArgs defines places to obtain target namespace args.
type TargetsArgs struct {
	// AllNamespaces if true distribute all namespaces
	AllNamespaces bool `json:"allNamespaces,omitempty" yaml:"allNamespaces,omitempty"`

	// ExcludedNamespaces is a list of excluded namespaces name.
	ExcludedNamespaces []string `json:"excludedNamespaces,omitempty" yaml:"excludedNamespaces,omitempty"`

	// IncludedNamespaces is a list of included namespaces name.
	IncludedNamespaces []string `json:"includedNamespaces,omitempty" yaml:"includedNamespaces,omitempty"`

	// NamespaceLabelSelector for the generator.
	NamespaceLabelSelector *metav1.LabelSelector `json:"namespaceLabelSelector,omitempty" yaml:"namespaceLabelSelector,omitempty"`
}

const tmpl = `
apiVersion: apps.kruise.io/v1alpha1
kind: ResourceDistributionGenerator
metadata:
  name: rdname
spec:
  resource:
    apiVersion: v1
    kind: ConfigMap
    metadata:
      name: cmname
`

// 设置了空以后就没法添加内容了。。taegets:

func setLabelsAndAnnotations(
	rn *yaml.RNode, opts *types.GeneratorOptions) error {
	if opts == nil {
		return nil
	}
	for _, k := range yaml.SortedMapKeys(opts.Labels) { //  重复？
		v := opts.Labels[k]
		if _, err := rn.Pipe(yaml.SetLabel(k, v)); err != nil {
			return err
		}
	}
	for _, k := range yaml.SortedMapKeys(opts.Annotations) {
		v := opts.Annotations[k]
		if _, err := rn.Pipe(yaml.SetAnnotation(k, v)); err != nil {
			return err
		}
	}
	return nil
}

func setResourceLabelsAndAnnotations(
	rn *yaml.RNode, opts *types.GeneratorOptions) error {
	if opts == nil {
		return nil
	}
	for _, k := range yaml.SortedMapKeys(opts.Labels) {
		v := opts.Labels[k]
		if _, err := rn.Pipe(
			yaml.PathGetter{Path: []string{"spec", "resource", "metadata"}, Create: yaml.MappingNode},
			yaml.FieldSetter{Name: "labels", Value: yaml.NewStringRNode(v)}); err != nil {
			return err
		}
	}
	for _, k := range yaml.SortedMapKeys(opts.Annotations) {
		v := opts.Annotations[k]
		if _, err := rn.Pipe(yaml.PathGetter{Path: []string{"spec", "resource", "metadata"}, Create: yaml.MappingNode},
			yaml.FieldSetter{Name: "annotations", Value: yaml.NewStringRNode(v)}); err != nil {
			return err
		}
	}
	return nil
}

func setTargets(rn *yaml.RNode, args *TargetsArgs) error {
	if args == nil {
		return nil
	}
	v := args.IncludedNamespaces
	if v != nil {
		if _, err := rn.Pipe(
			yaml.PathGetter{Path: []string{"spec", "targets", "includedNamespaces"},
				Create: yaml.MappingNode},
			yaml.FieldSetter{Name: "list", Value: yaml.NewListRNode(v...)}); err != nil {
			return err
		}
	}

	v = args.ExcludedNamespaces
	if v != nil {
		if _, err := rn.Pipe(
			yaml.PathGetter{Path: []string{"spec", "targets", "excludedNamespaces"},
				Create: yaml.MappingNode},
			yaml.FieldSetter{Name: "list", Value: yaml.NewListRNode(v...)}); err != nil {
			return err
		}
	}

	// resourcedistribution 默认 allNamespaces为false
	if args.AllNamespaces {
		allNamespaces := strconv.FormatBool(args.AllNamespaces)
		n := &yaml.Node{
			Kind:  yaml.ScalarNode,
			Value: allNamespaces,
			Tag:   yaml.NodeTagBool,
		}
		if _, err := rn.Pipe(yaml.PathGetter{Path: []string{"spec", "targets"},
			Create: yaml.MappingNode},
			yaml.FieldSetter{Name: "allNamespaces", Value: yaml.NewRNode(n)}); err != nil {
			return err
		}
	}

	err := setNamespaceLabelSelector(rn, args.NamespaceLabelSelector)
	if err != nil {
		return err
	}

	return nil
}

func setNamespaceLabelSelector(
	rn *yaml.RNode, sel *metav1.LabelSelector) error {
	if sel == nil {
		return nil
	}
	for _, k := range yaml.SortedMapKeys(sel.MatchLabels) {
		v := sel.MatchLabels[k]
		if _, err := rn.Pipe(
			yaml.PathGetter{Path: []string{"spec", "targets", "namespaceLabelSelector", "matchLabels"},
				Create: yaml.MappingNode},
			yaml.FieldSetter{Name: k, Value: yaml.NewStringRNode(v)}); err != nil {
			return err
		}
	}

	matchSeq := &yaml.Node{Kind: yaml.SequenceNode}

	// set matchExpressions
	for _, req := range sel.MatchExpressions {

		node := &yaml.Node{
			Kind: yaml.MappingNode,
		}
		//if node == nil {
		//	return node
		//}
		node.Content = append(node.Content, &yaml.Node{
			Kind:  yaml.ScalarNode,
			Value: "key",
		}, &yaml.Node{
			Kind:  yaml.ScalarNode,
			Value: req.Key,
		})
		node.Content = append(node.Content, &yaml.Node{
			Kind:  yaml.ScalarNode,
			Value: "operator",
		}, &yaml.Node{
			Kind:  yaml.ScalarNode,
			Value: string(req.Operator),
		})

		seq := &yaml.Node{Kind: yaml.SequenceNode}
		sort.Strings(req.Values)
		for _, val := range req.Values {
			seq.Content = append(seq.Content, &yaml.Node{
				Kind:  yaml.ScalarNode,
				Value: val,
			})
		}

		node.Content = append(node.Content, &yaml.Node{
			Kind:  yaml.ScalarNode,
			Value: "values",
		}, seq)

		matchSeq.Content = append(matchSeq.Content, node)

	}
	if _, err := rn.Pipe(yaml.PathGetter{Path: []string{"spec", "targets", "namespaceLabelSelector"},
		Create: yaml.MappingNode},
		yaml.FieldSetter{Name: "matchExpressions", Value: yaml.NewRNode(matchSeq)}); err != nil {
		return err
	}

	return nil
}

func setImmutable(
	rn *yaml.RNode, opts *types.GeneratorOptions) error {
	if opts == nil {
		return nil
	}
	if opts.Immutable {
		n := &yaml.Node{
			Kind:  yaml.ScalarNode,
			Value: "true",
			Tag:   yaml.NodeTagBool,
		}
		_, err := rn.Pipe(yaml.PathGetter{Path: []string{"spec", "resource"},
			Create: yaml.MappingNode},
			yaml.FieldSetter{Name: "immutable", Value: yaml.NewRNode(n)})
		if err != nil {
			return err
		}
	}

	return nil
}

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

func loadMapIntoConfigMapData2(m map[string]string, rn *yaml.RNode) error {
	for _, k := range yaml.SortedMapKeys(m) {
		fldName, vrN := makeConfigMapValueRNode(m[k])
		if _, err := rn.Pipe(
			yaml.LookupCreate(yaml.MappingNode, "spec", "resource", fldName),
			yaml.SetField(k, vrN)); err != nil {
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

func loadMapIntoSecretData(m map[string]string, rn *yaml.RNode) error {
	mapNode, err := rn.Pipe(yaml.LookupCreate(yaml.MappingNode, "spec", "resource", yaml.DataField))
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

func main() {
	config := new(resourceDistributionPlugin)
	fn := func(items []*yaml.RNode) ([]*yaml.RNode, error) {
		rn, err := yaml.Parse(tmpl)
		if err != nil {
			return nil, err
		}

		// setName
		err = rn.PipeE(yaml.SetK8sName(config.Name))
		if err != nil {
			return nil, err
		}
		err = setLabelsAndAnnotations(rn, config.Options)
		if err != nil {
			return nil, err
		}

		// setResourceName
		err = rn.PipeE(
			yaml.PathGetter{Path: []string{"spec", "resource", "metadata"}, Create: yaml.MappingNode},
			yaml.FieldSetter{Name: "name", Value: yaml.NewStringRNode(config.ResourceName)})
		if err != nil {
			return nil, err
		}

		// setResourceLabelsAndAnnotations
		err = setResourceLabelsAndAnnotations(rn, config.ResourceOptions)
		if err != nil {
			return nil, err
		}

		// setResource()

		ldr, err := loader.NewLoader(
			loader.RestrictionRootOnly,
			"./", filesys.MakeFsOnDisk())
		kvLdr := kv.NewLoader(ldr, provider.NewDefaultDepProvider().GetFieldValidator())

		m, err := makeValidatedDataMap(kvLdr, config.Name, config.KvPairSources)
		if err != nil {
			return nil, err
		}

		if config.ResourceKind == "ConfigMap" {
			if err = loadMapIntoConfigMapData2(m, rn); err != nil {
				return nil, err
			}
		} else if config.ResourceKind == "Secret" {
			t := "Opaque"
			if config.Type != "" {
				t = config.Type
			}
			if _, err := rn.Pipe(
				yaml.PathGetter{Path: []string{"spec", "resource"},
					Create: yaml.MappingNode},
				yaml.FieldSetter{
					Name:  "type",
					Value: yaml.NewStringRNode(t)}); err != nil {
				return nil, err
			}

			if err = loadMapIntoSecretData(m, rn); err != nil {
				return nil, err
			}
		} else {
			//return nil, err
			// 返回错误
		}

		err = setTargets(rn, &config.TargetsArgs)
		if err != nil {
			return nil, err
		}

		// setImmutable
		err = setImmutable(rn, config.ResourceOptions)
		if err != nil {
			return nil, err
		}

		var ptr []*yaml.RNode
		ptr = append(ptr, rn)
		return ptr, nil
	}

	p := framework.SimpleProcessor{Config: config, Filter: kio.FilterFunc(fn)}
	cmd := command.Build(p, command.StandaloneDisabled, false)
	//command.AddGenerateDockerfile(cmd)
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
