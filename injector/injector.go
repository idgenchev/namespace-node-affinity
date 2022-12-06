// Package injector deals with AdmissionReview requests and responses
package injector

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	log "github.com/sirupsen/logrus"
	v1beta1 "k8s.io/api/admission/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sclient "k8s.io/client-go/kubernetes"
	"sigs.k8s.io/yaml"
)

// Errors returned by this package
var (
	ErrInvalidAdmissionReview        = errors.New("invalid admission review")
	ErrInvalidAdmissionReviewObj     = errors.New("invalid admission review object")
	ErrFailedToCreatePatch           = errors.New("failed to create patch")
	ErrFailedToReadNodeSelectorTerms = errors.New("failed to load node selector terms")
	ErrMissingConfiguration          = errors.New("missing configuration")
	ErrInvalidConfiguration          = errors.New("invalid configuration")
)

// PatchPath is the path for the JSON patch
type PatchPath string

// PatchPath values
const (
	// affinity
	CreateAffinity              = "/spec/affinity"
	CreateNodeAffinity          = "/spec/affinity/nodeAffinity"
	AddRequiredDuringScheduling = "/spec/affinity/nodeAffinity/requiredDuringSchedulingIgnoredDuringExecution"
	AddNodeSelectorTerms        = "/spec/affinity/nodeAffinity/requiredDuringSchedulingIgnoredDuringExecution/nodeSelectorTerms"
	AddToNodeSelectorTerms      = "/spec/affinity/nodeAffinity/requiredDuringSchedulingIgnoredDuringExecution/nodeSelectorTerms/-"
	// tolerations
	CreateTolerations = "/spec/tolerations"
	AddTolerations    = "/spec/tolerations/-"
)

const (
	nodeSelectorKey = "nodeSelectorTerms"
	tolerationsKey  = "tolerations"
	successStatus   = "Success"
	annotationKey   = "namespace-node-affinity.idgenchev.github.com/applied-patch"
)

var (
	jsonMarshal   = json.Marshal
	jsonUnmarshal = json.Unmarshal
	yamlUnmarshal = yaml.Unmarshal
)

// JSONPatch is the JSON patch (http://jsonpatch.com) for patching k8s
// object
type JSONPatch struct {
	Op    string      `json:"op"`
	Path  PatchPath   `json:"path"`
	Value interface{} `json:"value"`
}

// NamespaceConfig is the per-namespace configuration
type NamespaceConfig struct {
	NodeSelectorTerms []corev1.NodeSelectorTerm `json:"nodeSelectorTerms"`
	Tolerations       []corev1.Toleration       `json:"tolerations"`
	ExcludedLabels    map[string]string         `json:"excludedLabels"`
}

// Injector handles AdmissionReview objects
type Injector struct {
	clientset     k8sclient.Interface
	namespace     string
	configMapName string
}

// NewInjector returns *Injector with k8sclient and configMapName
func NewInjector(k8sclient k8sclient.Interface, namespace string, configMapName string) *Injector {
	return &Injector{k8sclient, namespace, configMapName}
}

// Mutate unmarshalls the AdmissionReview (body) and creates or updates the
// nodeAffinity and/or the tolerations of the k8s object in the admission
// review request, sets the AdmissionReview response and returns the marshalled
// AdmissionReview or an error
func (m *Injector) Mutate(body []byte) ([]byte, error) {
	log.Infof("Received AdmissionReview: %s\n", string(body))

	// unmarshal request into AdmissionReview struct
	admissionReview := v1beta1.AdmissionReview{}
	if err := jsonUnmarshal(body, &admissionReview); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrInvalidAdmissionReview, err)
	}

	var pod *corev1.Pod

	req := admissionReview.Request
	if req == nil {
		log.Warning("admissionReview with empty request")
		return nil, nil
	}

	resp := v1beta1.AdmissionResponse{}

	if err := jsonUnmarshal(req.Object.Raw, &pod); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidAdmissionReviewObj, err)
	}

	// set response options
	resp.Allowed = true
	resp.UID = req.UID
	jsonPatch := v1beta1.PatchTypeJSONPatch
	resp.PatchType = &jsonPatch

	podNamespace := req.Namespace
	if podNamespace == "" {
		podNamespace = "default"
	}

	config, err := m.configForNamespace(podNamespace)
	if err != nil {
		return nil, err
	}

	if ignorePodWithLabels(pod.Labels, config) {
		log.Infof("Ignoring pod with labels: %#v in namespace: %s", pod.Labels, podNamespace)
		// return the unmodified AdmissionReview
		return body, nil
	}

	patch, err := buildPatch(config, pod.Spec)
	if err != nil {
		return nil, err
	}

	resp.Patch = patch

	resp.AuditAnnotations = map[string]string{
		annotationKey: string(patch),
	}

	resp.Result = &metav1.Status{
		Status: successStatus,
	}

	admissionReview.Response = &resp

	responseBody, err := jsonMarshal(admissionReview)
	if err != nil {
		return nil, err
	}

	log.Infof("AdmissionReview response: %s\n", string(responseBody))

	return responseBody, nil
}

func (m *Injector) configForNamespace(namespace string) (*NamespaceConfig, error) {
	configMap, err := m.clientset.CoreV1().
		ConfigMaps(m.namespace).
		Get(context.Background(), m.configMapName, metav1.GetOptions{})

	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrMissingConfiguration, err)
	}

	namespaceConfigString, exists := configMap.Data[namespace]
	if !exists {
		return nil, fmt.Errorf("%w: for %s", ErrMissingConfiguration, namespace)
	}

	config := &NamespaceConfig{}
	err = yamlUnmarshal([]byte(namespaceConfigString), config)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrInvalidConfiguration, err)
	} else if config.NodeSelectorTerms == nil && config.Tolerations == nil {
		return nil, fmt.Errorf("%w: at least one of nodeSelectorTerms or tolerations needs to be specified for %s", ErrInvalidConfiguration, namespace)
	}

	return config, nil
}

func buildNodeSelectorTermsPath(podSpec corev1.PodSpec) PatchPath {
	var path PatchPath

	if podSpec.Affinity == nil {
		path = CreateAffinity
	} else if podSpec.Affinity != nil && podSpec.Affinity.NodeAffinity == nil {
		path = CreateNodeAffinity
	} else if podSpec.Affinity.NodeAffinity != nil && podSpec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution == nil {
		path = AddRequiredDuringScheduling
	} else if podSpec.Affinity.NodeAffinity != nil && podSpec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution != nil && podSpec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms == nil {
		path = AddNodeSelectorTerms
	} else {
		// We have a nodeSelectorTerms that != nil (eg: it is a 0 or more length array, or it is malformed)
		path = AddToNodeSelectorTerms
	}

	return path
}

func buildTolerationsPath(podSpec corev1.PodSpec) PatchPath {
	if podSpec.Tolerations == nil {
		return CreateTolerations
	}
	return AddTolerations
}

func buildNodeSelectorTermPatch(path PatchPath, nodeSelectorTerm corev1.NodeSelectorTerm) JSONPatch {
	patch := JSONPatch{
		Op:    "add",
		Path:  path,
		Value: nodeSelectorTerm,
	}

	return patch
}

// Returns a patch that initialises the PodSpec's NodeSelectorTerms array as an empty array, if it does not exist
func buildNodeSelectorTermsInitPatch(podSpec corev1.PodSpec) (JSONPatch, error) {
	path := buildNodeSelectorTermsPath(podSpec)

	patch := JSONPatch{
		Op:   "add",
		Path: path,
	}

	patchAffinity := &corev1.Affinity{
		NodeAffinity: &corev1.NodeAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
				NodeSelectorTerms: []corev1.NodeSelectorTerm{},
			},
		},
	}

	switch path {
	case AddToNodeSelectorTerms:
		// Array for NodeSelectorTerms already exists.  Do nothing
		return JSONPatch{}, nil
	case AddNodeSelectorTerms:
		// NodeSelectorTerms array missing, add it
		fmt.Print("buildNodeSelectorTermsInitPatch case AddNodeSelectorTerms\n")
		patch.Value = patchAffinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms
	case AddRequiredDuringScheduling:
		// Adds RequiredDuringScheduling with NodeSelectorTerms
		patch.Value = patchAffinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution
	case CreateNodeAffinity:
		// Adds NodeAffinity with RequiredDuringScheduling and NodeSelectorTerms
		patch.Value = patchAffinity.NodeAffinity
	case CreateAffinity:
		// Adds Affinity with NodeAffinity, RequiredDuringScheduling, and NodeSelectorTerms
		patch.Value = patchAffinity
	default:
		return JSONPatch{}, fmt.Errorf("%w: invalid patch path", ErrFailedToCreatePatch)
	}

	return patch, nil
}

func buildPatch(config *NamespaceConfig, podSpec corev1.PodSpec) ([]byte, error) {
	var patches []JSONPatch

	if config.NodeSelectorTerms != nil {
		initPatch, err := buildNodeSelectorTermsInitPatch(podSpec)
		if err != nil {
			return nil, err
		}
		if (initPatch != JSONPatch{}) {
			patches = append(patches, initPatch)
		}

		// todo: extract this so it is easier to test
		for _, NodeSelectorTerm := range config.NodeSelectorTerms {
			nodeSelectorTermsPatch := buildNodeSelectorTermPatch(AddToNodeSelectorTerms, NodeSelectorTerm)

			patches = append(patches, nodeSelectorTermsPatch)
		}
	}

	if config.Tolerations != nil {
		// todo: handle adding tolerations to an existing array
		tolerationsPatchPath := buildTolerationsPath(podSpec)
		for _, toleration := range config.Tolerations {
			tolerationsPatch := JSONPatch{
				Op:    "add",
				Path:  tolerationsPatchPath,
				Value: toleration,
			}

			patches = append(patches, tolerationsPatch)
		}
	}

	patch, err := jsonMarshal(patches)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrFailedToCreatePatch, err)
	}

	return patch, nil
}

func ignorePodWithLabels(podLabels map[string]string, config *NamespaceConfig) bool {
	if len(config.ExcludedLabels) == 0 {
		return false
	}

	numMatchedLabels := 0
	for k, v := range config.ExcludedLabels {
		if podVal, ok := podLabels[k]; ok && podVal == v {
			numMatchedLabels++
		}
	}

	return numMatchedLabels == len(config.ExcludedLabels)
}
