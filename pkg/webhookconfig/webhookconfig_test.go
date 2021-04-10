package webhookconfig

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	fake "k8s.io/client-go/kubernetes/fake"
	fakeadmissionregistrationv1 "k8s.io/client-go/kubernetes/typed/admissionregistration/v1/fake"
	k8stesting "k8s.io/client-go/testing"
)

const (
	webhookConfigName = "wh"
	namespace         = "ns"
	serviceName       = "whsvc"
)

func caBundle(contents string) *bytes.Buffer {
	caBundle := &bytes.Buffer{}
	caBundle.Write([]byte(contents))
	return caBundle
}

func TestCreateMutatingWebhookConfig(t *testing.T) {
	t.Parallel()

	clientset := fake.NewSimpleClientset()

	bundle := caBundle("asdasd")
	err := CreateOrUpdateMutatingWebhookConfig(clientset, bundle, namespace, webhookConfigName, serviceName)
	assert.NoError(t, err)

	expectedConfig := &admissionregistrationv1.MutatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name: webhookConfigName,
		},
		Webhooks: []admissionregistrationv1.MutatingWebhook{
			{
				Name:                    fmt.Sprintf("%s.%s.svc", serviceName, namespace),
				SideEffects:             sideEffect(),
				AdmissionReviewVersions: []string{"v1"},
				ClientConfig: admissionregistrationv1.WebhookClientConfig{
					CABundle: bundle.Bytes(),
					Service: &admissionregistrationv1.ServiceReference{
						Name:      serviceName,
						Namespace: namespace,
						Path:      path(),
					},
				},
				Rules: []admissionregistrationv1.RuleWithOperations{
					{
						Operations: []admissionregistrationv1.OperationType{admissionregistrationv1.Create},
						Rule: admissionregistrationv1.Rule{
							APIGroups:   []string{""},
							APIVersions: []string{"v1"},
							Resources:   []string{"pods"},
						},
					},
				},
				FailurePolicy: failurePolicy(),
				NamespaceSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"namespace-node-affinity": "enabled",
					},
				},
			},
		},
	}

	actualConfig, err := clientset.AdmissionregistrationV1().MutatingWebhookConfigurations().Get(context.Background(), webhookConfigName, metav1.GetOptions{})

	assert.NoError(t, err)
	assert.Equal(t, expectedConfig, actualConfig)
}

func TestUpdateMutatingWebhookConfig(t *testing.T) {
	t.Parallel()

	initialBundle := caBundle("initialcabundle")
	existingConfig := &admissionregistrationv1.MutatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name:            webhookConfigName,
			ResourceVersion: "testv",
		},
		Webhooks: []admissionregistrationv1.MutatingWebhook{
			{
				Name:                    fmt.Sprintf("%s.%s.svc", serviceName, namespace),
				SideEffects:             sideEffect(),
				AdmissionReviewVersions: []string{"v1"},
				ClientConfig: admissionregistrationv1.WebhookClientConfig{
					CABundle: initialBundle.Bytes(),
					Service: &admissionregistrationv1.ServiceReference{
						Name:      serviceName,
						Namespace: namespace,
						Path:      path(),
					},
				},
				Rules: []admissionregistrationv1.RuleWithOperations{
					{
						Operations: []admissionregistrationv1.OperationType{admissionregistrationv1.Create},
						Rule: admissionregistrationv1.Rule{
							APIGroups:   []string{""},
							APIVersions: []string{"v1"},
							Resources:   []string{"pods"},
						},
					},
				},
				FailurePolicy: failurePolicy(),
				NamespaceSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"namespace-node-affinity": "enabled",
					},
				},
			},
		},
	}

	clientset := fake.NewSimpleClientset(existingConfig)

	newBundle := caBundle("newcabundle")
	err := CreateOrUpdateMutatingWebhookConfig(clientset, newBundle, namespace, webhookConfigName, serviceName)
	assert.NoError(t, err)

	newConfig, err := clientset.AdmissionregistrationV1().MutatingWebhookConfigurations().Get(context.Background(), webhookConfigName, metav1.GetOptions{})

	assert.NoError(t, err)
	assert.Equal(t, newBundle.Bytes(), newConfig.Webhooks[0].ClientConfig.CABundle)

	// Make sure we've set the ResourceVersion. The fake client is
	// returning whatever is passed to the Update, so the
	// ResourceVersion should be exactly the same as the existing one
	assert.Equal(t, existingConfig.ResourceVersion, newConfig.ResourceVersion)
}

func TestCreateMutatingWebhookConfigWithError(t *testing.T) {
	expectedErr := errors.New("create err")

	clientset := fake.NewSimpleClientset()
	clientset.AdmissionregistrationV1().(*fakeadmissionregistrationv1.FakeAdmissionregistrationV1).PrependReactor("create", "*", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, nil, expectedErr
	})

	bundle := caBundle("asdasd")
	err := CreateOrUpdateMutatingWebhookConfig(clientset, bundle, namespace, webhookConfigName, serviceName)
	assert.Equal(t, expectedErr, err)
}

func TestFailingToGetExistingWebhook(t *testing.T) {
	expectedErr := errors.New("get err")

	existingConfig := &admissionregistrationv1.MutatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name: webhookConfigName,
		},
	}
	clientset := fake.NewSimpleClientset(existingConfig)
	clientset.AdmissionregistrationV1().(*fakeadmissionregistrationv1.FakeAdmissionregistrationV1).PrependReactor("get", "*", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, nil, expectedErr
	})

	bundle := caBundle("asdasd")
	err := CreateOrUpdateMutatingWebhookConfig(clientset, bundle, namespace, webhookConfigName, serviceName)
	assert.Equal(t, expectedErr, err)
}
