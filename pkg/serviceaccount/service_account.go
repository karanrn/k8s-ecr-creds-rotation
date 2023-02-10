package serviceaccount

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"
)

// includeImagePullSecret checks if the secret is already part of imagePullSecrets
func includeImagePullSecret(sa *corev1.ServiceAccount, secretName string) bool {
	for _, imagePullSecret := range sa.ImagePullSecrets {
		if imagePullSecret.Name == secretName {
			return true
		}
	}
	return false
}

// PatchImagePullSecrets patches imagePullSecret by adding secret mentioned
func PatchImagePullSecrets(client *kubernetes.Clientset, saName string, namespace string, secretName string) error {
	saClient := client.CoreV1().ServiceAccounts(namespace)
	retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		// RetryOnConflict uses exponential backoff to avoid exhausting the apiserver
		result, getErr := saClient.Get(context.TODO(), saName, metav1.GetOptions{})
		if getErr != nil {
			return fmt.Errorf("failed to get latest version of service account: %v\n", getErr)
		}

		existingImagePullSecrets := append([]corev1.LocalObjectReference(nil), result.ImagePullSecrets...)

		if !includeImagePullSecret(result, secretName) {
			result.ImagePullSecrets = append(existingImagePullSecrets, corev1.LocalObjectReference{Name: secretName})
			_, updateErr := saClient.Update(context.TODO(), result, metav1.UpdateOptions{})
			return updateErr
		}

		return nil
	})
	if retryErr != nil {
		return fmt.Errorf("update failed: %v\n", retryErr)
	}
	return nil
}
