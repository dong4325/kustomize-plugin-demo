package generator

const tmpl = `
apiVersion: apps.kruise.io/v1alpha1
kind: ResourceDistribution
spec:
  resource:
    apiVersion: v1
`

// Field names
const (
	annotationsField      = "annotations"
	apiVersionField       = "apiVersion"
	kindField             = "kind"
	metadataField         = "metadata"
	dataField             = "data"
	binaryDataField       = "binaryData"
	nameField             = "name"
	namespaceField        = "namespace"
	labelsField           = "labels"
	listField             = "list"
	allNamespacesField    = "allNamespaces"
	immutableField        = "immutable"
	typeField             = "type"
	matchExpressionsField = "matchExpressions"
	keyField              = "key"
	operatorField         = "operator"
	valuesField           = "values"
)

var metadataLabelsPath = []string{"metadata", "labels"}
var metadataAnnotationsPath = []string{"metadata", "annotations"}
var resourcePath = []string{"spec", "resource"}
var metadataPath = []string{"spec", "resource", "metadata"}
var resourceLabelsPath = []string{"spec", "resource", "metadata", "labels"}
var resourceAnnotationsPath = []string{"spec", "resource", "metadata", "annotations"}
var targetsPath = []string{"spec", "targets"}
var includedNamespacesPath = []string{"spec", "targets", "includedNamespaces"}
var excludedNamespacesPath = []string{"spec", "targets", "excludedNamespaces"}
var NamespaceLabelSelectorPath = []string{"spec", "targets", "namespaceLabelSelector"}
var MatchLabelsPath = []string{"spec", "targets", "namespaceLabelSelector", "matchLabels"}
