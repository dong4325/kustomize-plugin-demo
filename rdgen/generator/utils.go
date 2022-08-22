package generator

import (
	"github.com/go-errors/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/kustomize/api/kv"
	"sigs.k8s.io/kustomize/api/loader"
	"sigs.k8s.io/kustomize/api/provider"
	"sigs.k8s.io/kustomize/api/types"
	"sigs.k8s.io/kustomize/kyaml/filesys"
	"sigs.k8s.io/kustomize/kyaml/yaml"
	"sort"
	"strconv"
)

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

func setResource(rn *yaml.RNode, args *ResourceArgs) error {
	if err := setResourceName(rn, args.ResourceName); err != nil {
		return err
	}

	if err := setResourceKind(rn, args.ResourceKind); err != nil {
		return err
	}

	if err := setLabelsOrAnnotations(rn, args.ResourceOptions.Labels, resourceLabelsPath); err != nil {
		return err
	}
	if err := setLabelsOrAnnotations(rn, args.ResourceOptions.Annotations, resourceAnnotationsPath); err != nil {
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

func setResourceName(rn *yaml.RNode, name string) error {
	if name == "" {
		return errors.Errorf("a ResourceDistribution must have a resource name ")
	}
	err := rn.PipeE(yaml.PathGetter{Path: metadataPath, Create: yaml.MappingNode},
		yaml.FieldSetter{Name: nameField, Value: yaml.NewStringRNode(name)})
	if err != nil {
		return err
	}
	return nil
}

func setResourceKind(
	rn *yaml.RNode, kind string) error {
	if kind == "" || kind != "Secret" && kind != "ConfigMap" {
		return errors.Errorf("resourceKind must be ConfigMap or Secret ")
	}
	err := rn.PipeE(yaml.PathGetter{Path: resourcePath, Create: yaml.MappingNode},
		yaml.FieldSetter{Name: kindField, Value: yaml.NewStringRNode(kind)})
	if err != nil {
		return err
	}

	return nil
}

func setLabelsOrAnnotations(
	rn *yaml.RNode, labelsOrAnnotations map[string]string, labelsOrAnnotationsPath []string) error {
	if labelsOrAnnotations == nil {
		return nil
	}

	for _, k := range yaml.SortedMapKeys(labelsOrAnnotations) {
		v := labelsOrAnnotations[k]
		if _, err := rn.Pipe(
			yaml.PathGetter{Path: labelsOrAnnotationsPath, Create: yaml.MappingNode},
			yaml.FieldSetter{Name: k, Value: yaml.NewStringRNode(v)}); err != nil {
			return err
		}
	}
	return nil
}

// TODO 看懂代码
func setData(rn *yaml.RNode, args *ResourceArgs) error {
	// 尝试使用 BASE 后也可以使用

	ldr, err := loader.NewLoader(loader.RestrictionRootOnly,
		"./", filesys.MakeFsOnDisk())
	kvLdr := kv.NewLoader(ldr, provider.NewDefaultDepProvider().GetFieldValidator())

	m, err := makeValidatedDataMap(kvLdr, args.ResourceName, args.KvPairSources)
	if err != nil {
		return err
	}

	if args.ResourceKind == "ConfigMap" {
		if err = rn.LoadMapIntoConfigMapData(m); err != nil {
			return err
		}
	} else {
		t := "Opaque"
		if args.Type != "" {
			t = args.Type
		}
		_, err := rn.Pipe(yaml.PathGetter{Path: resourcePath, Create: yaml.MappingNode},
			yaml.FieldSetter{Name: typeField, Value: yaml.NewStringRNode(t)})
		if err != nil {
			return err
		}

		if err = rn.LoadMapIntoSecretData(m); err != nil {
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
			yaml.FieldSetter{Name: immutableField, Value: yaml.NewRNode(n)})
		if err != nil {
			return err
		}
	}

	return nil
}

func setTargets(rn *yaml.RNode, args *TargetsArgs) error {
	if args.AllNamespaces == false && args.IncludedNamespaces == nil && args.ExcludedNamespaces == nil &&
		(args.NamespaceLabelSelector == nil ||
			args.NamespaceLabelSelector.MatchLabels == nil && args.NamespaceLabelSelector.MatchExpressions == nil) {
		return errors.Errorf("The targets field of the ResourceDistribution cannot be empty")
	}

	err := setIncludedExcludedNs(rn, args.IncludedNamespaces, includedNamespacesPath)
	if err != nil {
		return err
	}

	err = setIncludedExcludedNs(rn, args.ExcludedNamespaces, excludedNamespacesPath)
	if err != nil {
		return err
	}

	err = setAllNs(rn, args.AllNamespaces)
	if err != nil {
		return err
	}

	err = setNsLabelSelector(rn, args.NamespaceLabelSelector)
	if err != nil {
		return err
	}

	return nil
}

func setIncludedExcludedNs(rn *yaml.RNode, v []string, inExNsPath []string) error {
	if v == nil {
		return nil
	}
	if _, err := rn.Pipe(
		yaml.PathGetter{Path: inExNsPath, Create: yaml.MappingNode},
		yaml.FieldSetter{Name: listField, Value: newNameListRNode(v...)}); err != nil {
		return err
	}
	return nil
}

// resourcedistribution 默认 allNamespaces为false
func setAllNs(rn *yaml.RNode, allNs bool) error {
	if !allNs {
		return nil
	}
	allNamespaces := strconv.FormatBool(allNs)
	n := &yaml.Node{
		Kind:  yaml.ScalarNode,
		Value: allNamespaces,
		Tag:   yaml.NodeTagBool,
	}
	if _, err := rn.Pipe(
		yaml.PathGetter{Path: targetsPath, Create: yaml.MappingNode},
		yaml.FieldSetter{Name: allNamespacesField, Value: yaml.NewRNode(n)}); err != nil {
		return err
	}
	return nil
}

func setNsLabelSelector(rn *yaml.RNode, sel *metav1.LabelSelector) error {
	if sel == nil {
		return nil
	}

	err := setMatchLabels(rn, sel.MatchLabels)
	if err != nil {
		return err
	}

	err = setMatchExpressions(rn, sel.MatchExpressions)
	if err != nil {
		return err
	}

	return nil
}

func setMatchLabels(rn *yaml.RNode, matchLabels map[string]string) error {
	if matchLabels == nil {
		return nil
	}
	for _, k := range yaml.SortedMapKeys(matchLabels) {
		v := matchLabels[k]
		if _, err := rn.Pipe(
			yaml.PathGetter{Path: MatchLabelsPath, Create: yaml.MappingNode},
			yaml.FieldSetter{Name: k, Value: yaml.NewStringRNode(v)}); err != nil {
			return err
		}
	}
	return nil
}

// TODO 处理异常
func setMatchExpressions(rn *yaml.RNode, args []metav1.LabelSelectorRequirement) error {
	if args == nil {
		return nil
	}

	matchExpList := &yaml.Node{Kind: yaml.SequenceNode}
	for _, matchExpArgs := range args {
		matchExpElement := &yaml.Node{
			Kind: yaml.MappingNode,
		}

		// append key for matchExpression
		if matchExpArgs.Key == "" {
			return errors.Errorf("the field " +
				"ResourceDistribution.targets.namespaceLabelSelector.matchExpressions.key cannot be empty")
		}
		matchExpElement.Content = append(matchExpElement.Content, &yaml.Node{
			Kind: yaml.ScalarNode, Value: keyField,
		}, &yaml.Node{
			Kind: yaml.ScalarNode, Value: matchExpArgs.Key,
		})

		// append operator for matchExpression
		if matchExpArgs.Operator != metav1.LabelSelectorOpIn &&
			matchExpArgs.Operator != metav1.LabelSelectorOpNotIn &&
			matchExpArgs.Operator != metav1.LabelSelectorOpExists &&
			matchExpArgs.Operator != metav1.LabelSelectorOpDoesNotExist {
			return errors.Errorf("the field " +
				"ResourceDistribution.targets.namespaceLabelSelector.matchExpressions.operator is invalid")
		}
		matchExpElement.Content = append(matchExpElement.Content, &yaml.Node{
			Kind: yaml.ScalarNode, Value: operatorField,
		}, &yaml.Node{
			Kind: yaml.ScalarNode, Value: string(matchExpArgs.Operator),
		})

		// append values for matchExpression
		if matchExpArgs.Values == nil {
			return errors.Errorf("the field " +
				"ResourceDistribution.targets.namespaceLabelSelector.matchExpressions.values cannot be empty")
		}
		valSeq := &yaml.Node{Kind: yaml.SequenceNode}
		sort.Strings(matchExpArgs.Values)
		for _, val := range matchExpArgs.Values {
			//
			if val == "" {
				return errors.Errorf("the element of field " +
					"ResourceDistribution.targets.namespaceLabelSelector.matchExpressions.values cannot be empty")
			}
			valSeq.Content = append(valSeq.Content, &yaml.Node{
				Kind: yaml.ScalarNode, Value: val,
			})
		}
		matchExpElement.Content = append(matchExpElement.Content, &yaml.Node{
			Kind: yaml.ScalarNode, Value: valuesField,
		}, valSeq)

		// element is added to the list
		matchExpList.Content = append(matchExpList.Content, matchExpElement)
	}

	_, err := rn.Pipe(
		yaml.PathGetter{Path: NamespaceLabelSelectorPath, Create: yaml.MappingNode},
		yaml.FieldSetter{Name: matchExpressionsField, Value: yaml.NewRNode(matchExpList)})
	if err != nil {
		return err
	}

	return nil
}
