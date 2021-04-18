package affinityinjector

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
	fake "k8s.io/client-go/kubernetes/fake"
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

func nodeSelectorTermsJSON() string {
	terms, _ := json.Marshal(nodeSelectorTerms())
	return string(terms)
}

func TestBuildPath(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name         string
		podSpec      corev1.PodSpec
		expectedPath AffinityPath
	}{
		{
			name:         "WithNoAffinity",
			podSpec:      corev1.PodSpec{},
			expectedPath: CreateAffinity,
		},
		{
			name: "WithNoNodeAffinity",
			podSpec: corev1.PodSpec{
				Affinity: &corev1.Affinity{PodAffinity: &corev1.PodAffinity{}},
			},
			expectedPath: CreateNodeAffinity,
		},
		{
			name: "WithPreferredDuringSchedulingIgnoredDuringExecution",
			podSpec: corev1.PodSpec{
				Affinity: &corev1.Affinity{
					NodeAffinity: &corev1.NodeAffinity{
						PreferredDuringSchedulingIgnoredDuringExecution: []corev1.PreferredSchedulingTerm{},
					},
				},
			},
			expectedPath: AddRequiredDuringScheduling,
		},
		{
			name: "WithExistingAffinity",
			podSpec: corev1.PodSpec{
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
			},
			expectedPath: AddNodeSelectorTerms,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			path := buildPath(tc.podSpec)
			assert.Equal(t, tc.expectedPath, path)
		})
	}
}

func TestBuildPatch(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		path  AffinityPath
		value string
	}{
		{
			name: "ForCreateAffinityPath",
			path: CreateAffinity,
			value: fmt.Sprintf(
				"{\"nodeAffinity\":{\"requiredDuringSchedulingIgnoredDuringExecution\":{\"nodeSelectorTerms\":%s}}}",
				nodeSelectorTermsJSON(),
			),
		},
		{
			name: "ForCreateNodeAffinityPath",
			path: CreateNodeAffinity,
			value: fmt.Sprintf(
				"{\"requiredDuringSchedulingIgnoredDuringExecution\":{\"nodeSelectorTerms\":%s}}",
				nodeSelectorTermsJSON(),
			),
		},
		{
			name: "ForAddRequiredDuringSchedulingPath",
			path: AddRequiredDuringScheduling,
			value: fmt.Sprintf(
				"{\"nodeSelectorTerms\":%s}",
				nodeSelectorTermsJSON(),
			),
		},
		{
			name:  "ForAddNodeSelectorTermsPath",
			path:  AddNodeSelectorTerms,
			value: nodeSelectorTermsJSON(),
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			patch, err := buildPatch(tc.path, nodeSelectorTerms())

			expectedPatch := []byte(
				fmt.Sprintf("[{\"op\":\"add\",\"path\":\"%s\",\"value\":%s}]",
					tc.path, tc.value,
				),
			)

			assert.NoError(t, err)
			assert.Equal(t, expectedPatch, patch)
		})
	}
}

func TestBuildPatchWithInvalidPath(t *testing.T) {
	t.Parallel()

	patch, err := buildPatch("invalid", nodeSelectorTerms())
	assert.Nil(t, patch)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrFailedToCreatePatch))
}

func TestMutateWithInvalidBody(t *testing.T) {
	t.Parallel()

	clientset := fake.NewSimpleClientset()
	m := AffinityInjector{clientset, "cm"}

	body, err := m.Mutate([]byte("invalid"))

	assert.Nil(t, body)
	assert.True(t, errors.Is(err, ErrInvalidAdmissionReview))
}

func TestMutateWithNoRequest(t *testing.T) {
	t.Parallel()

	clientset := fake.NewSimpleClientset()
	m := AffinityInjector{clientset, "cm"}

	admissionReview := []byte("{}")

	body, err := m.Mutate(admissionReview)

	assert.Nil(t, body)
	assert.NoError(t, err)
}

func TestMutateWithMissingConfigMap(t *testing.T) {
	t.Parallel()

	clientset := fake.NewSimpleClientset()
	m := AffinityInjector{clientset, "test-cm"}

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

func TestMutateWithMissingNodeSelectorTerms(t *testing.T) {
	t.Parallel()

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cm",
			Namespace: "test-ns",
		},
	}
	clientset := fake.NewSimpleClientset(cm)
	m := AffinityInjector{clientset, "test-cm"}

	admissionReview := v1beta1.AdmissionReview{
		Request: &v1beta1.AdmissionRequest{
			Namespace: "test-ns",
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
	assert.True(t, errors.Is(err, ErrMissingNodeSelectorTerms))
}

func TestMutateWithInvalidNodeSelectorTerms(t *testing.T) {
	t.Parallel()

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cm",
			Namespace: "default",
		},
		Data: map[string]string{cmKey: "invalid"},
	}
	clientset := fake.NewSimpleClientset(cm)
	m := AffinityInjector{clientset, "test-cm"}

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
	assert.True(t, errors.Is(err, ErrInvalidConfiguration))
}

func TestMutateWithBuildPatchError(t *testing.T) {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cm",
			Namespace: "default",
		},
		Data: map[string]string{cmKey: nodeSelectorTermsJSON()},
	}
	clientset := fake.NewSimpleClientset(cm)
	m := AffinityInjector{clientset, "test-cm"}

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
		fmt.Printf("\n\nthe obj: %#v\n\n", v)
		return nil, errors.New("some error")
	}
	defer func() { jsonMarshal = origMarshal }()

	body, err := m.Mutate(j)
	assert.Nil(t, body)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrFailedToCreatePatch))
}

func TestMutate(t *testing.T) {
	t.Parallel()

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cm",
			Namespace: "test-ns",
		},
		Data: map[string]string{cmKey: nodeSelectorTermsJSON()},
	}
	clientset := fake.NewSimpleClientset(cm)
	m := AffinityInjector{clientset, "test-cm"}

	admissionReview := v1beta1.AdmissionReview{
		Request: &v1beta1.AdmissionRequest{
			Namespace: "test-ns",
			Object: runtime.RawExtension{
				Object: &corev1.Pod{},
			},
		},
	}
	j, err := json.Marshal(admissionReview)
	assert.NoError(t, err)

	body, err := m.Mutate(j)
	assert.NoError(t, err)

	affinity := fmt.Sprintf(
		"{\"nodeAffinity\":{\"requiredDuringSchedulingIgnoredDuringExecution\":{\"nodeSelectorTerms\":%s}}}",
		nodeSelectorTermsJSON(),
	)
	expectedPatch := fmt.Sprintf("[{\"op\":\"add\",\"path\":\"%s\",\"value\":%s}]", CreateAffinity, affinity)

	jsonPatch := v1beta1.PatchTypeJSONPatch
	expectedResp := v1beta1.AdmissionResponse{
		PatchType:        &jsonPatch,
		Allowed:          true,
		Patch:            []byte(expectedPatch),
		AuditAnnotations: map[string]string{annotationKey: expectedPatch},
		Result:           &metav1.Status{Status: successStatus},
	}

	expectedAdmissionReview := admissionReview
	expectedAdmissionReview.Response = &expectedResp

	expectedBody, err := json.Marshal(expectedAdmissionReview)
	assert.NoError(t, err)
	assert.Equal(t, expectedBody, body)
}
