package secret

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"
)

const (
	DockerConfigJson = ".dockerconfigjson"
	CredSecretName = "regcred"
)

// CreateDockerConfig creates secret of type DockerConfigJson in the given namespace
func CreateDockerConfig(client *kubernetes.Clientset, secretName string, namespace string, value string) (*corev1.Secret, error) {
	secretClient := client.CoreV1().Secrets(namespace)
	data := make(map[string]string)
	data[DockerConfigJson] = value
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: secretName,
		},
		StringData: data,
		Type:       corev1.SecretTypeDockerConfigJson,
	}
	result, err := secretClient.Create(context.Background(), secret, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed creating secret: %v\n", err)
	}
	return result, nil
}

// UpdateDockerConfig creates secret of type DockerConfigJson in the given namespace
func UpdateDockerConfig(client *kubernetes.Clientset, secretName string, namespace string, value string) error {
	secretClient := client.CoreV1().Secrets(namespace)
	retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		// Retrieve the latest version of secret before attempting update
		// RetryOnConflict uses exponential backoff to avoid exhausting the apiserver
		result, getErr := secretClient.Get(context.TODO(), secretName, metav1.GetOptions{})
		if getErr != nil {
			return fmt.Errorf("failed to get latest version of secret: %v\n", getErr)
		}

		data := make(map[string]string)
		data[DockerConfigJson] = value
		result.StringData = data
		_, updateErr := secretClient.Update(context.TODO(), result, metav1.UpdateOptions{})
		return updateErr
	})
	if retryErr != nil {
		return fmt.Errorf("secret update failed: %v\n", retryErr)
	}
	return nil
}
