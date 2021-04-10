// Package webhookconfig deals with creating or updating
// MutatingWebhookConfiguration for the namespace-node-affinity webhook
package webhookconfig

import (
	"bytes"
	"context"
	"fmt"

	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sclient "k8s.io/client-go/kubernetes"
)

func failurePolicy() *admissionregistrationv1.FailurePolicyType {
	policy := admissionregistrationv1.Ignore
	return &policy
}

func sideEffect() *admissionregistrationv1.SideEffectClass {
	sideEffectClass := admissionregistrationv1.SideEffectClassNone
	return &sideEffectClass
}

func path() *string {
	p := "/mutate"
	return &p
}

// CreateOrUpdateMutatingWebhookConfig creates "namespace-node-affinity"
// mutating webhook configuration with "Ignore" failure policy for pods
// or returns an error
// NOTE: If the MutatingWebhookConfiguration already exists, the only
// thing that will be updated is the CABundle
func CreateOrUpdateMutatingWebhookConfig(k8sClient k8sclient.Interface, caBundle *bytes.Buffer, namespace, name, serviceName string) error {
	webhookName := fmt.Sprintf("%s.%s.svc", serviceName, namespace)

	mutateconfig := &admissionregistrationv1.MutatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Webhooks: []admissionregistrationv1.MutatingWebhook{
			{
				Name:                    webhookName,
				SideEffects:             sideEffect(),
				AdmissionReviewVersions: []string{"v1"},
				ClientConfig: admissionregistrationv1.WebhookClientConfig{
					CABundle: caBundle.Bytes(),
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

	if _, err := k8sClient.AdmissionregistrationV1().MutatingWebhookConfigurations().Create(context.Background(), mutateconfig, metav1.CreateOptions{}); err != nil {
		if k8serrors.IsAlreadyExists(err) {
			existingConf, err := k8sClient.AdmissionregistrationV1().MutatingWebhookConfigurations().Get(context.Background(), name, metav1.GetOptions{})
			if err != nil {
				return err
			}

			// The metadata.resourceVersion needs to be specified for an update
			mutateconfig.ResourceVersion = existingConf.ResourceVersion
			_, err = k8sClient.AdmissionregistrationV1().MutatingWebhookConfigurations().Update(context.Background(), mutateconfig, metav1.UpdateOptions{})
			return err
		}
		return err
	}

	return nil
}
