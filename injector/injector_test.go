package injector

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	v1beta1 "k8s.io/api/admission/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
)

func nodeSelectorTerms() []corev1.NodeSelectorTerm {
	return []corev1.NodeSelectorTerm{
		{
			MatchExpressions: []corev1.NodeSelectorRequirement{
				{
					Key:      "key",
					Operator: corev1.NodeSelectorOpIn,
					Values:   []string{"val"},
				},
			},
		},
	}
}

func tolerations() []corev1.Toleration {
	return []corev1.Toleration{
		{
			Key:      "example-key",
			Operator: corev1.TolerationOpExists,
			Value:    "example-value",
			Effect:   corev1.TaintEffectNoSchedule,
		},
		{
			Key:      "example-key-b",
			Operator: corev1.TolerationOpExists,
			Value:    "example-value-b",
			Effect:   corev1.TaintEffectPreferNoSchedule,
		},
	}
}

// PodSpecs with various levels of completion for Node Affinity
var (
	podSpecWithNoAffinity     = corev1.PodSpec{}
	podSpecWithNoNodeAffinity = corev1.PodSpec{
		Affinity: &corev1.Affinity{PodAffinity: &corev1.PodAffinity{}},
	}
	podSpecWithNoRequiredDuringSchedulingIgnoreDuringExecution = corev1.PodSpec{
		Affinity: &corev1.Affinity{
			NodeAffinity: &corev1.NodeAffinity{},
		},
	}
	podSpecWithNoNodeSelectorTerms = corev1.PodSpec{
		Affinity: &corev1.Affinity{
			NodeAffinity: &corev1.NodeAffinity{
				RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{},
			},
		},
	}
	podSpecWithEmptyNodeSelectorTerms = corev1.PodSpec{
		Affinity: &corev1.Affinity{
			NodeAffinity: &corev1.NodeAffinity{
				RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
					NodeSelectorTerms: []corev1.NodeSelectorTerm{
						{
							MatchExpressions: []corev1.NodeSelectorRequirement{},
						},
					},
				},
			},
		},
	}
	podSpecWithExistingNodeSelectorTerms = corev1.PodSpec{
		Affinity: &corev1.Affinity{
			NodeAffinity: &corev1.NodeAffinity{
				RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
					NodeSelectorTerms: []corev1.NodeSelectorTerm{
						{
							MatchExpressions: []corev1.NodeSelectorRequirement{
								{
									Key:      "key",
									Operator: corev1.NodeSelectorOpIn,
									Values:   []string{"val"},
								},
							},
						},
					},
				},
			},
		},
	}
)

func TestBuildNodeSelectorTermPath(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name         string
		podSpec      corev1.PodSpec
		expectedPath PatchPath
	}{
		{
			name:         "WithNoAffinity",
			podSpec:      podSpecWithNoAffinity,
			expectedPath: CreateAffinity,
		},
		{
			name:         "WithNoNodeAffinity",
			podSpec:      podSpecWithNoNodeAffinity,
			expectedPath: CreateNodeAffinity,
		},
		{
			name:         "WithNoRequiredDuringSchedulingIgnoredDuringExecution",
			podSpec:      podSpecWithNoRequiredDuringSchedulingIgnoreDuringExecution,
			expectedPath: AddRequiredDuringScheduling,
		},
		{
			name:         "WithNoNodeSelectorTerms",
			podSpec:      podSpecWithNoNodeSelectorTerms,
			expectedPath: AddNodeSelectorTerms,
		},
		{
			name:         "WithEmptyNodeSelectorTerms",
			podSpec:      podSpecWithEmptyNodeSelectorTerms,
			expectedPath: AddToNodeSelectorTerms,
		},
		{
			name:         "WithExistingAffinity",
			podSpec:      podSpecWithExistingNodeSelectorTerms,
			expectedPath: AddToNodeSelectorTerms,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			path := buildNodeSelectorTermsPath(tc.podSpec)
			assert.Equal(t, tc.expectedPath, path)
		})
	}
}

func TestBuildTolerationsPath(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name         string
		podSpec      corev1.PodSpec
		expectedPath PatchPath
	}{
		{
			name: "WithTolerations",
			podSpec: corev1.PodSpec{
				Tolerations: tolerations(),
			},
			expectedPath: AddTolerations,
		},
		{
			name:         "WithoutTolerations",
			podSpec:      corev1.PodSpec{},
			expectedPath: CreateTolerations,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			path := buildTolerationsPath(tc.podSpec)
			assert.Equal(t, tc.expectedPath, path)
		})
	}
}

func TestBuildNodeSelectorTermPatch(t *testing.T) {
	t.Parallel()

	path := PatchPath("")
	nodeSelectorTerm := corev1.NodeSelectorTerm{
		MatchExpressions: []corev1.NodeSelectorRequirement{
			corev1.NodeSelectorRequirement{
				Key:      "test key",
				Operator: corev1.NodeSelectorOpIn,
				Values:   []string{"val"},
			},
		},
	}
	expectedPatch := JSONPatch{
		Op:    "add",
		Path:  path,
		Value: nodeSelectorTerm,
	}

	patch := buildNodeSelectorTermPatch(path, nodeSelectorTerm)
	assert.Equal(t, patch, expectedPatch)
}

func TestBuildNodeSelctorTermsInitPatch(t *testing.T) {
	t.Parallel()

	//podSpecWithEmptyNodeSelectorTerms := corev1.PodSpec{
	//	Affinity: &corev1.Affinity{
	//		NodeAffinity: &corev1.NodeAffinity{
	//			RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
	//				NodeSelectorTerms: []corev1.NodeSelectorTerm{},
	//			},
	//		},
	//	},
	//}

	testCases := []struct {
		name          string
		podSpec       corev1.PodSpec
		expectedPatch JSONPatch
	}{
		{
			name:    "WithNoAffinity",
			podSpec: podSpecWithNoAffinity,
			expectedPatch: JSONPatch{
				Op:   "add",
				Path: CreateAffinity,
				Value: &corev1.Affinity{
					NodeAffinity: &corev1.NodeAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
							NodeSelectorTerms: []corev1.NodeSelectorTerm{},
						},
					},
				},
			},
		},
		{
			name:    "WithNoNodeAffinity",
			podSpec: podSpecWithNoNodeAffinity,
			expectedPatch: JSONPatch{
				Op:   "add",
				Path: CreateNodeAffinity,
				Value: &corev1.NodeAffinity{
					RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
						NodeSelectorTerms: []corev1.NodeSelectorTerm{},
					},
				},
			},
		},
		{
			name:    "WithNoRequiredDuringSchedulingIgnoredDuringExecution",
			podSpec: podSpecWithNoRequiredDuringSchedulingIgnoreDuringExecution,
			expectedPatch: JSONPatch{
				Op:   "add",
				Path: AddRequiredDuringScheduling,
				Value: &corev1.NodeSelector{
					NodeSelectorTerms: []corev1.NodeSelectorTerm{},
				},
			},
		},
		{
			name:    "WithNoNodeSelectorTerms",
			podSpec: podSpecWithNoNodeSelectorTerms,
			expectedPatch: JSONPatch{
				Op:    "add",
				Path:  AddNodeSelectorTerms,
				Value: []corev1.NodeSelectorTerm{},
			},
		},
		{
			name:          "WithEmptyNodeSelectorTerms",
			podSpec:       podSpecWithEmptyNodeSelectorTerms,
			expectedPatch: JSONPatch{},
		},
		{
			name:          "WithExistingNodeSelectorTerms",
			podSpec:       podSpecWithExistingNodeSelectorTerms,
			expectedPatch: JSONPatch{},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			patch, err := buildNodeSelectorTermsInitPatch(tc.podSpec)

			assert.Nil(t, err)
			assert.Equal(t, tc.expectedPatch, patch)

		})
	}
}

// todo: TestBuildNodeSelectorTermsInitPatch where the patch path is invalid?

func TestMutateWithInvalidBody(t *testing.T) {
	t.Parallel()

	clientset := fake.NewSimpleClientset()
	m := Injector{clientset, "default", "cm"}

	body, err := m.Mutate([]byte("invalid"))

	assert.Nil(t, body)
	assert.True(t, errors.Is(err, ErrInvalidAdmissionReview))
}

func TestMutateWithNoRequest(t *testing.T) {
	t.Parallel()

	clientset := fake.NewSimpleClientset()
	m := Injector{clientset, "default", "cm"}

	admissionReview := []byte("{}")

	body, err := m.Mutate(admissionReview)

	assert.Nil(t, body)
	assert.NoError(t, err)
}

func TestMutateWithMissingConfigMap(t *testing.T) {
	t.Parallel()

	clientset := fake.NewSimpleClientset()
	m := Injector{clientset, "default", "test-cm"}

	admissionReview := v1beta1.AdmissionReview{
		Request: &v1beta1.AdmissionRequest{
			Object: runtime.RawExtension{
				Object: &corev1.Pod{},
			},
		},
	}
	j, err := json.Marshal(admissionReview)
	assert.NoError(t, err)

	body, err := m.Mutate(j)
	assert.Nil(t, body)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrMissingConfiguration))
}

func TestMutateWithMissingConfigurationForTheNamespace(t *testing.T) {
	t.Parallel()

	deploymentNamespace := "ns-node-affinity"
	podNamespace := "test-ns"
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cm",
			Namespace: deploymentNamespace,
		},
		Data: map[string]string{"someconfig": "somevalue"},
	}
	clientset := fake.NewSimpleClientset(cm)
	m := Injector{clientset, deploymentNamespace, "test-cm"}

	admissionReview := v1beta1.AdmissionReview{
		Request: &v1beta1.AdmissionRequest{
			Namespace: podNamespace,
			Object: runtime.RawExtension{
				Object: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pod",
						Namespace: podNamespace,
					},
				},
			},
		},
	}
	j, err := json.Marshal(admissionReview)
	assert.NoError(t, err)

	body, err := m.Mutate(j)
	assert.Nil(t, body)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrMissingConfiguration))
}

func TestMutateWithInvalidConfigForNamespace(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
	}{
		{
			name: "nodeSelectorTerms",
		},
		{
			name: "tolerations",
		},
		{
			name: "noneoftheexpected",
		},
	}

	for _, tc := range testCases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			deploymentNamespace := "ns-node-affinity"
			podNamespace := "test-ns"

			namespaceConfig := fmt.Sprintf("%s: \"invalid\"", tc.name)

			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cm",
					Namespace: deploymentNamespace,
				},
				Data: map[string]string{podNamespace: namespaceConfig},
			}
			clientset := fake.NewSimpleClientset(cm)
			m := Injector{clientset, deploymentNamespace, "test-cm"}

			admissionReview := v1beta1.AdmissionReview{
				Request: &v1beta1.AdmissionRequest{
					Namespace: podNamespace,
					Object: runtime.RawExtension{
						Object: &corev1.Pod{},
					},
				},
			}
			j, err := json.Marshal(admissionReview)
			assert.NoError(t, err)

			body, err := m.Mutate(j)
			assert.Nil(t, body)
			assert.Error(t, err)
			assert.True(t, errors.Is(err, ErrInvalidConfiguration))
		})
	}
}

func TestMutateWithBuildPatchError(t *testing.T) {
	deploymentNamespace := "ns-node-affinity"
	podNamespace := "default"
	nodeSelectorTermsJSON, _ := json.Marshal(nodeSelectorTerms())
	namespaceConfig := fmt.Sprintf("%s: %s", nodeSelectorKey, nodeSelectorTermsJSON)

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cm",
			Namespace: deploymentNamespace,
		},
		Data: map[string]string{podNamespace: namespaceConfig},
	}
	clientset := fake.NewSimpleClientset(cm)
	m := Injector{clientset, deploymentNamespace, "test-cm"}

	admissionReview := v1beta1.AdmissionReview{
		Request: &v1beta1.AdmissionRequest{
			Object: runtime.RawExtension{
				Object: &corev1.Pod{},
			},
		},
	}
	j, err := json.Marshal(admissionReview)
	assert.NoError(t, err)

	origMarshal := jsonMarshal
	jsonMarshal = func(v interface{}) ([]byte, error) {
		return nil, errors.New("some error")
	}
	defer func() { jsonMarshal = origMarshal }()

	body, err := m.Mutate(j)
	assert.Nil(t, body)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrFailedToCreatePatch))
}

// todo: refactor test and re-enable it
//func TestMutate(t *testing.T) {
//	t.Parallel()
//
//	deploymentNamespace := "ns-node-affinity"
//	podNamespace := "testing-ns"
//
//	nsConfig := NamespaceConfig{
//		NodeSelectorTerms: nodeSelectorTerms(),
//		Tolerations:       tolerations(),
//	}
//	nsConfigJSON, _ := json.Marshal(nsConfig)
//
//	cm := &corev1.ConfigMap{
//		ObjectMeta: metav1.ObjectMeta{
//			Name:      "test-cm",
//			Namespace: deploymentNamespace,
//		},
//		Data: map[string]string{podNamespace: string(nsConfigJSON)},
//	}
//	clientset := fake.NewSimpleClientset(cm)
//	m := Injector{clientset, deploymentNamespace, "test-cm"}
//
//	admissionReview := v1beta1.AdmissionReview{
//		Request: &v1beta1.AdmissionRequest{
//			Namespace: podNamespace,
//			Object: runtime.RawExtension{
//				Object: &corev1.Pod{},
//			},
//		},
//	}
//	j, err := json.Marshal(admissionReview)
//	assert.NoError(t, err)
//
//	body, err := m.Mutate(j)
//	assert.NoError(t, err)
//
//	nodeSelectorPatch, _ := buildNodeSelectorTermsPatch(CreateAffinity, nodeSelectorTerms())
//	patches := []JSONPatch{
//		nodeSelectorPatch,
//		{
//			Op:    "add",
//			Path:  CreateTolerations,
//			Value: tolerations()[0],
//		},
//		{
//			Op:    "add",
//			Path:  CreateTolerations,
//			Value: tolerations()[1],
//		},
//	}
//	expectedPatch, _ := json.Marshal(patches)
//
//	jsonPatch := v1beta1.PatchTypeJSONPatch
//	expectedResp := v1beta1.AdmissionResponse{
//		PatchType:        &jsonPatch,
//		Allowed:          true,
//		Patch:            expectedPatch,
//		AuditAnnotations: map[string]string{annotationKey: string(expectedPatch)},
//		Result:           &metav1.Status{Status: successStatus},
//	}
//
//	expectedAdmissionReview := admissionReview
//	expectedAdmissionReview.Response = &expectedResp
//
//	expectedBody, err := json.Marshal(expectedAdmissionReview)
//	assert.NoError(t, err)
//	assert.Equal(t, expectedBody, body)
//}

func TestMutateIgnoresPodsWithExcludedLabels(t *testing.T) {
	t.Parallel()

	deploymentNamespace := "ns-node-affinity"
	podNamespace := "testing-ns"

	nsConfig := NamespaceConfig{
		NodeSelectorTerms: nodeSelectorTerms(),
		Tolerations:       tolerations(),
		ExcludedLabels:    map[string]string{"ignore-me": "ignored"},
	}
	nsConfigJSON, _ := json.Marshal(nsConfig)

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cm",
			Namespace: deploymentNamespace,
		},
		Data: map[string]string{podNamespace: string(nsConfigJSON)},
	}
	clientset := fake.NewSimpleClientset(cm)
	m := Injector{clientset, deploymentNamespace, "test-cm"}

	admissionReview := v1beta1.AdmissionReview{
		Request: &v1beta1.AdmissionRequest{
			Namespace: podNamespace,
			Object: runtime.RawExtension{
				Object: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"ignore-me":     "ignored",
							"another-label": "label",
						},
					},
				},
			},
		},
	}
	j, err := json.Marshal(admissionReview)
	assert.NoError(t, err)

	body, err := m.Mutate(j)
	assert.NoError(t, err)

	assert.NoError(t, err)
	assert.Equal(t, j, body)
}
