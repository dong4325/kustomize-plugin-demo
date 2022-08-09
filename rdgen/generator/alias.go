package generator

const tmpl = `
apiVersion: apps.kruise.io/v1alpha1
kind: ResourceDistributionGenerator
spec:
  resource:
    apiVersion: v1
`

// Field names
const (
	AnnotationsField      = "annotations"
	APIVersionField       = "apiVersion"
	KindField             = "kind"
	MetadataField         = "metadata"
	DataField             = "data"
	BinaryDataField       = "binaryData"
	NameField             = "name"
	NamespaceField        = "namespace"
	LabelsField           = "labels"
	ListField             = "list"
	AllNamespacesField    = "allNamespaces"
	ImmutableField        = "immutable"
	TypeField             = "type"
	MatchExpressionsField = "matchExpressions"
	KeyField              = "key"
	OperatorField         = "operator"
	ValuesField           = "values"
)

var resourcePath = []string{"spec", "resource"}
var metadataPath = []string{"spec", "resource", "metadata"}
var targetsPath = []string{"spec", "targets"}
var includedNamespacesPath = []string{"spec", "targets", "includedNamespaces"}
var excludedNamespacesPath = []string{"spec", "targets", "excludedNamespaces"}
var NamespaceLabelSelectorPath = []string{"spec", "targets", "namespaceLabelSelector"}
var MatchLabelsPath = []string{"spec", "targets", "namespaceLabelSelector", "matchLabels"}
