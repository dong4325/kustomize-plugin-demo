package generator

import (
	"encoding/base64"
	"github.com/go-errors/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/kustomize/api/ifc"
	"sigs.k8s.io/kustomize/api/kv"
	"sigs.k8s.io/kustomize/api/loader"
	"sigs.k8s.io/kustomize/api/provider"
	"sigs.k8s.io/kustomize/api/types"
	"sigs.k8s.io/kustomize/kyaml/filesys"
	"sigs.k8s.io/kustomize/kyaml/yaml"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

func setLabelsAndAnnotations(
	rn *yaml.RNode, opts *types.GeneratorOptions) error {
	if opts == nil {
		return nil
	}
	for _, k := range yaml.SortedMapKeys(opts.Labels) {
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
			yaml.PathGetter{Path: metadataPath, Create: yaml.MappingNode},
			yaml.FieldSetter{Name: LabelsField, Value: yaml.NewStringRNode(v)}); err != nil {
			return err
		}
	}
	for _, k := range yaml.SortedMapKeys(opts.Annotations) {
		v := opts.Annotations[k]
		if _, err := rn.Pipe(yaml.PathGetter{Path: metadataPath, Create: yaml.MappingNode},
			yaml.FieldSetter{Name: AnnotationsField, Value: yaml.NewStringRNode(v)}); err != nil {
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
			yaml.PathGetter{
				Path:   includedNamespacesPath,
				Create: yaml.MappingNode},
			yaml.FieldSetter{
				Name:  ListField,
				Value: yaml.NewListRNode(v...)}); err != nil {
			return err
		}
	}

	v = args.ExcludedNamespaces
	if v != nil {
		_, err := rn.Pipe(
			yaml.PathGetter{
				Path:   excludedNamespacesPath,
				Create: yaml.MappingNode},
			yaml.FieldSetter{
				Name:  ListField,
				Value: yaml.NewListRNode(v...)})
		if err != nil {
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
		if _, err := rn.Pipe(
			yaml.PathGetter{
				Path:   targetsPath,
				Create: yaml.MappingNode},
			yaml.FieldSetter{
				Name:  AllNamespacesField,
				Value: yaml.NewRNode(n)}); err != nil {
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
			yaml.PathGetter{Path: MatchLabelsPath,
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

		node.Content = append(node.Content, &yaml.Node{
			Kind:  yaml.ScalarNode,
			Value: KeyField,
		}, &yaml.Node{
			Kind:  yaml.ScalarNode,
			Value: req.Key,
		})
		node.Content = append(node.Content, &yaml.Node{
			Kind:  yaml.ScalarNode,
			Value: OperatorField,
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
			Value: ValuesField,
		}, seq)

		matchSeq.Content = append(matchSeq.Content, node)
	}
	if sel.MatchExpressions != nil {
		_, err := rn.Pipe(yaml.PathGetter{Path: NamespaceLabelSelectorPath, Create: yaml.MappingNode},
			yaml.FieldSetter{Name: MatchExpressionsField, Value: yaml.NewRNode(matchSeq)})
		if err != nil {
			return err
		}
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
		_, err := rn.Pipe(
			yaml.PathGetter{Path: resourcePath, Create: yaml.MappingNode},
			yaml.FieldSetter{Name: ImmutableField, Value: yaml.NewRNode(n)})
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
			yaml.LookupCreate(yaml.MappingNode, append(resourcePath, fldName)...),
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

func setResourceKind(
	rn *yaml.RNode, kind string) error {
	if kind == "" || kind != "Secret" && kind != "ConfigMap" {
		return errors.Errorf("resourceKind must be ConfigMap or Secret ")
	}
	err := rn.PipeE(yaml.PathGetter{Path: resourcePath, Create: yaml.MappingNode},
		yaml.FieldSetter{Name: KindField, Value: yaml.NewStringRNode(kind)})
	if err != nil {
		return err
	}

	return nil
}

func setResourceName(rn *yaml.RNode, name string) error {
	if name == "" {
		return errors.Errorf("a ResourceDistribution must have a resource name ")
	}
	err := rn.PipeE(yaml.PathGetter{Path: metadataPath, Create: yaml.MappingNode},
		yaml.FieldSetter{Name: NameField, Value: yaml.NewStringRNode(name)})
	if err != nil {
		return err
	}
	return nil
}

func setData(rn *yaml.RNode, args *ResourceArgs) error {
	//setData
	ldr, err := loader.NewLoader(loader.RestrictionRootOnly,
		"./", filesys.MakeFsOnDisk())
	kvLdr := kv.NewLoader(ldr, provider.NewDefaultDepProvider().GetFieldValidator())

	m, err := makeValidatedDataMap(kvLdr, args.ResourceName, args.KvPairSources)
	if err != nil {
		return err
	}

	if args.ResourceKind == "ConfigMap" {
		if err = loadMapIntoConfigMapData2(m, rn); err != nil {
			return err
		}
	} else {
		t := "Opaque"
		if args.Type != "" {
			t = args.Type
		}
		if _, err := rn.Pipe(yaml.PathGetter{Path: resourcePath, Create: yaml.MappingNode},
			yaml.FieldSetter{Name: TypeField, Value: yaml.NewStringRNode(t)}); err != nil {
			return err
		}

		if err = loadMapIntoSecretData(m, rn); err != nil {
			return err
		}
	}
	return nil
}

func setResource(rn *yaml.RNode, args *ResourceArgs) error {
	if err := setResourceName(rn, args.ResourceName); err != nil {
		return err
	}

	if err := setResourceLabelsAndAnnotations(rn, args.ResourceOptions); err != nil {
		return err
	}

	if err := setResourceKind(rn, args.ResourceKind); err != nil {
		return err
	}

	if err := setData(rn, args); err != nil {
		return err
	}

	if err := setImmutable(rn, args.ResourceOptions); err != nil {
		return err
	}

	return nil
}

func makeBaseNode(meta *types.ObjectMeta) (*yaml.RNode, error) {
	rn, err := yaml.Parse(tmpl)
	if err != nil {
		return nil, err
	}
	if meta.Name == "" {
		return nil, errors.Errorf("a ResourceDistribution must have a name ")
	}
	err = rn.PipeE(yaml.SetK8sName(meta.Name))
	if err != nil {
		return nil, err
	}

	return rn, nil
}