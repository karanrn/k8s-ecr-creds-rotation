package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	"os"
	"os/signal"
	"time"
)

const (
	ECRRegistry = "ECR_REGISTRY"
	Region = "REGION"
	AccessKey = "ACCESS_KEY_ID"
	SecretAccessKey = "SECRET_ACCESS_KEY"
	DockerUser = "AWS"

	// Docker config constants
	AuthList = "auths"
	AuthKey = "auth"
)


func main() {
	/*
	0. Read ECR registry and region from environment
	1. Read access keys and secret access key
	2. Generate docker creds from the above keys through AWS ECR
	3. Update Secret in every namespace (List all namespaces)
	4. Give option to exclude namespaces - next release
	 */
	// Local setup
	var kubeconfig = "/Users/karan.nadagoudarnutanix.com/Downloads/kubeconfigs/kubeconfig-demo.yaml"
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		fmt.Printf("failed to build kubeconfig: %v", err)
	}

	// In-cluster setup
	// creates the in-cluster config
	/*
	config, err := rest.InClusterConfig()
	if err != nil {
		panic(err.Error())
	}
	*/

	// create the clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		fmt.Printf("failed to create client from config: %v", err)
	}

	/*
	pods, err := clientset.CoreV1().Pods("cpaas-demo-pulsar").List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		fmt.Printf("failed list pods from namespace: %v", err)
	}
	for _, v := range pods.Items {
		fmt.Printf("pod: %s\nstatus: %s\n\n", v.Name, v.Status.Phase)
	}
	//fmt.Println(pods.Items)
	*/

	clusterClient, err := dynamic.NewForConfig(config)
	if err != nil {
		fmt.Printf("failed to create client: %v", err)
	}

	// Listen/Watch for namespaces
	resource := schema.GroupVersionResource{Group:"", Version:"v1", Resource: "namespaces"}
	// Make use of shared informer factory
	factory := dynamicinformer.NewFilteredDynamicSharedInformerFactory(clusterClient, time.Minute, corev1.NamespaceAll, nil)
	informer := factory.ForResource(resource).Informer()

	stopCh := make(chan struct{})
	go startWatching(stopCh, informer, clientset)

	sigCh := make(chan os.Signal, 0)
	signal.Notify(sigCh, os.Kill, os.Interrupt)

	<-sigCh
	close(stopCh)


}

func startWatching(stopCh <-chan struct{}, s cache.SharedIndexInformer, client *kubernetes.Clientset) {
	handlers := cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			u := obj.(*unstructured.Unstructured)
			ns := u.GetName()
			fmt.Printf("received add event! for %v\n", ns)
			dockerConfig := RotateECRCreds()
			CreateSecret(client, ns, dockerConfig)
		},
		UpdateFunc: func(oldObj, obj interface{}) {
			u := obj.(*unstructured.Unstructured)
			fmt.Printf("received update event! for %v\n", u.GetName())
		},
		DeleteFunc: func(obj interface{}) {
			u := obj.(*unstructured.Unstructured)
			fmt.Printf("received delete event! for %v\n", u.GetName())
		},
	}

	s.AddEventHandler(handlers)
	s.Run(stopCh)
}

func CreateSecret(client *kubernetes.Clientset, namespace string, value string) {
	secretClient := client.CoreV1().Secrets(namespace)
	data := make(map[string]string)
	data[".dockerconfigjson"] = value
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:  "regcred",
		},
		StringData: data,
		Type:       corev1.SecretTypeDockerConfigJson,
	}
	result, err := secretClient.Create(context.Background(), secret, metav1.CreateOptions{})
	if err != nil {
		fmt.Printf("failed creating secret: %v", err)
	}
	fmt.Printf("Created secret %q.\n", result.GetObjectMeta().GetName())
}

func UpdateSecret(client *kubernetes.Clientset, namespace string, value string) {
	secretClient := client.CoreV1().Secrets(namespace)
	retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		// Retrieve the latest version of secret before attempting update
		// RetryOnConflict uses exponential backoff to avoid exhausting the apiserver
		result, getErr := secretClient.Get(context.TODO(), "regcred", metav1.GetOptions{})
		if getErr != nil {
			fmt.Printf("Failed to get latest version of secret: %v", getErr)
		}

		data := make(map[string]string)
		data[".dockerconfigjson"] = value
		result.StringData = data
		_, updateErr := secretClient.Update(context.TODO(), result, metav1.UpdateOptions{})
		return updateErr
	})
	if retryErr != nil {
		fmt.Printf("Update failed: %v", retryErr)
	}
}

func RotateECRCreds() string {
	// Read from ECR registry and region
	registry := os.Getenv(ECRRegistry)
	region := os.Getenv(Region)
	accessKey := os.Getenv(AccessKey)
	secretKey := os.Getenv(SecretAccessKey)

	//fmt.Printf("Registry: %s\nRegion: %s\n", registry, region)
	//fmt.Printf("Access Key: %s\nSecret Key: %s\n", accessKey, secretKey)

	cfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")),
		config.WithRegion(region),
	)
	if err != nil {
		fmt.Printf("failed to create aws config: %v", err)
	}

	// Get Authorization token for the ECR registry
	ecrSvc := ecr.NewFromConfig(cfg)
	resp, err := ecrSvc.GetAuthorizationToken(context.Background(), &ecr.GetAuthorizationTokenInput{})
	if err != nil {
		fmt.Println(err.Error())
	}
	//fmt.Printf("Proxy Endpoint: %v\n", *resp.AuthorizationData[0].ProxyEndpoint)
	token := *resp.AuthorizationData[0].AuthorizationToken
	//fmt.Printf("Token: %v\n\n", token)

	// Create docker config
	// Encode token to base64
	encodedToken := base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%s", DockerUser, token)))
	var dockerConfig = map[string]map[string]map[string]string{}
	dockerConfig[AuthList] = map[string]map[string]string{}
	dockerConfig[AuthList][registry] = map[string]string{}
	dockerConfig[AuthList][registry][AuthKey] = encodedToken
	jsonStr, _ := json.Marshal(dockerConfig)
	//fmt.Println(string(jsonStr))

	return string(jsonStr)
}
