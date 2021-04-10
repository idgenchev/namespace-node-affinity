// Package affinityinjector deals with AdmissionReview requests and responses
package affinityinjector

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
	ErrMissingNodeSelectorTerms      = errors.New("missing nodeSelectorTerms from config")
	ErrInvalidConfiguration          = errors.New("invalid configuration")
)

// AffinityPath is the path for the JSON patch
type AffinityPath string

// AffinityPath values
const (
	CreateAffinity              = "/spec/affinity"
	CreateNodeAffinity          = "/spec/affinity/nodeAffinity"
	AddRequiredDuringScheduling = "/spec/affinity/nodeAffinity/requiredDuringSchedulingIgnoredDuringExecution"
	AddNodeSelectorTerms        = "/spec/affinity/nodeAffinity/requiredDuringSchedulingIgnoredDuringExecution/nodeSelectorTerms/-"
)

const (
	cmKey         = "nodeSelectorTerms"
	successStatus = "Success"
	annotationKey = "namespace-node-affinity.idgenchev.github.com/applied-patch"
)

var (
	jsonMarshal   = json.Marshal
	jsonUnmarshal = json.Unmarshal
	yamlUnmarshal = yaml.Unmarshal
)

// JSONPatch is the JSON patch (http://jsonpatch.com) for patching k8s
// object
type JSONPatch struct {
	Op    string       `json:"op"`
	Path  AffinityPath `json:"path"`
	Value interface{}  `json:"value"`
}

// AffinityInjector handles AdmissionReview objects
type AffinityInjector struct {
	clientset     k8sclient.Interface
	configMapName string
}

// NewAffinityInjector returns *AffinityInjector with k8sclient and configMapName
func NewAffinityInjector(k8sclient k8sclient.Interface, configMapName string) *AffinityInjector {
	return &AffinityInjector{k8sclient, configMapName}
}

// Mutate unmarshalls the AdmissionReview (body) and creates or
// updates the nodeAffinity of the k8s object in the admission review
// request, sets the AdmissionReview response and returns the
// marshalled AdmissionReview or an error
func (m *AffinityInjector) Mutate(body []byte) ([]byte, error) {
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

	namespace := req.Namespace
	if namespace == "" {
		namespace = "default"
	}
	nodeSelectorTerms, err := m.nodeSelectorTerms(namespace)
	if err != nil {
		return nil, err
	}

	patchPath := buildPath(pod.Spec)
	patch, err := buildPatch(patchPath, nodeSelectorTerms)
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

func (m *AffinityInjector) nodeSelectorTerms(namespace string) ([]corev1.NodeSelectorTerm, error) {
	// Get(ctx context.Context, name string, opts metav1.GetOptions) (*v1.ConfigMap, error)
	configMap, err := m.clientset.CoreV1().
		ConfigMaps(namespace).
		Get(context.Background(), m.configMapName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrMissingConfiguration, err)
	}

	nodeSelectorTermsString, exists := configMap.Data[cmKey]
	if !exists {
		return nil, fmt.Errorf("%w: nodeSelectorTerms is missing from the config map", ErrMissingNodeSelectorTerms)
	}

	var nodeSelectorTerms []corev1.NodeSelectorTerm
	err = yamlUnmarshal([]byte(nodeSelectorTermsString), &nodeSelectorTerms)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrInvalidConfiguration, err)
	}

	return nodeSelectorTerms, nil
}

func buildPath(podSpec corev1.PodSpec) AffinityPath {
	var path AffinityPath

	if podSpec.Affinity == nil {
		path = CreateAffinity
	} else if podSpec.Affinity != nil && podSpec.Affinity.NodeAffinity == nil {
		path = CreateNodeAffinity
	} else if podSpec.Affinity.NodeAffinity != nil && podSpec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution == nil {
		path = AddRequiredDuringScheduling
	} else {
		path = AddNodeSelectorTerms
	}

	return path
}

func buildPatch(path AffinityPath, nodeSelectorTerms []corev1.NodeSelectorTerm) ([]byte, error) {
	patch := JSONPatch{
		Op:   "add",
		Path: path,
	}

	patchAffinity := &corev1.Affinity{
		NodeAffinity: &corev1.NodeAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
				NodeSelectorTerms: nodeSelectorTerms,
			},
		},
	}

	switch path {
	case AddNodeSelectorTerms:
		patch.Value = patchAffinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms
	case AddRequiredDuringScheduling:
		patch.Value = patchAffinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution
	case CreateNodeAffinity:
		patch.Value = patchAffinity.NodeAffinity
	case CreateAffinity:
		patch.Value = patchAffinity
	default:
		return nil, fmt.Errorf("%w: invalid patch path", ErrFailedToCreatePatch)
	}

	patchString, err := jsonMarshal([]JSONPatch{patch})
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrFailedToCreatePatch, err)
	}

	return patchString, nil
}
