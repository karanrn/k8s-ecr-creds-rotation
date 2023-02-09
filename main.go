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
	"os/signal"
	"time"

	//"k8s.io/apimachinery/pkg/runtime/schema"
	//"k8s.io/client-go/dynamic"
	//"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	"os"
	//"os/signal"
	//"time"
	"github.com/robfig/cron/v3"
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
	DockerConfigJson = ".dockerconfigjson"

	SecretName = "regcred"
	Empty = ""
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
		fmt.Printf("failed to build kubeconfig: %v\n", err)
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
		fmt.Printf("failed to create client from config: %v\n", err)
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

	// Run cron to rotate and update secret
	c := cron.New()
	updateJob := UpdateJob{client: clientset}
	//c.AddFunc("*/2 * * * *", func() { fmt.Println("Every hour on the half hour") })
	c.AddJob("*/2 * * * *", updateJob)
	c.Start()


	clusterClient, err := dynamic.NewForConfig(config)
	if err != nil {
		fmt.Printf("failed to create client: %v\n", err)
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

type UpdateJob struct {
	client *kubernetes.Clientset
}

func (job UpdateJob) Run() {
	namespaces, err := job.client.CoreV1().Namespaces().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		fmt.Printf("failed to list namespaces: %v\n", err)
	}

	dockerConfig, err := RotateECRCreds()
	if err != nil {
		fmt.Printf("failed to get docker config: %v\n", err)
	}
	for _, ns := range namespaces.Items {
		err := UpdateSecret(job.client, ns.Name, dockerConfig)
		if err != nil {
			fmt.Printf("failed to update secret %s in namespace %s: %v\n", SecretName, ns.Name, err)
		}
		fmt.Printf("updated secret in namespace: %s\n", ns.Name)
	}
}

func startWatching(stopCh <-chan struct{}, s cache.SharedIndexInformer, client *kubernetes.Clientset) {
	handlers := cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			u := obj.(*unstructured.Unstructured)
			ns := u.GetName()
			fmt.Printf("received add event! for %v\n", ns)
			dockerConfig, err := RotateECRCreds()
			if err != nil {
				fmt.Printf("failed to get docker config: %v\n", err)
			}
			err = CreateSecret(client, ns, dockerConfig)
			if err != nil {
				// Update secret
				fmt.Printf("updating secret\n")
				err = UpdateSecret(client, ns, dockerConfig)
				if err != nil {
					fmt.Printf("failed to update secret: %v\n", err)
				}
			}
		},
		//UpdateFunc: func(oldObj, obj interface{}) {
		//	u := obj.(*unstructured.Unstructured)
		//	fmt.Printf("received update event! for %v\n", u.GetName())
		//},
		//DeleteFunc: func(obj interface{}) {
		//	u := obj.(*unstructured.Unstructured)
		//	fmt.Printf("received delete event! for %v\n", u.GetName())
		//},
	}

	s.AddEventHandler(handlers)
	s.Run(stopCh)
}

func CreateSecret(client *kubernetes.Clientset, namespace string, value string) error {
	secretClient := client.CoreV1().Secrets(namespace)
	data := make(map[string]string)
	data[DockerConfigJson] = value
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:  SecretName,
		},
		StringData: data,
		Type:       corev1.SecretTypeDockerConfigJson,
	}
	result, err := secretClient.Create(context.Background(), secret, metav1.CreateOptions{})
	if err != nil {
		fmt.Printf("failed creating secret: %v\n", err)
		return err
	}
	fmt.Printf("Created secret %q.\n", result.GetObjectMeta().GetName())
	return nil
}

func UpdateSecret(client *kubernetes.Clientset, namespace string, value string) error {
	secretClient := client.CoreV1().Secrets(namespace)
	retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		// Retrieve the latest version of secret before attempting update
		// RetryOnConflict uses exponential backoff to avoid exhausting the apiserver
		result, getErr := secretClient.Get(context.TODO(), SecretName, metav1.GetOptions{})
		if getErr != nil {
			fmt.Printf("Failed to get latest version of secret: %v\n", getErr)
			return getErr
		}

		data := make(map[string]string)
		data[DockerConfigJson] = value
		result.StringData = data
		_, updateErr := secretClient.Update(context.TODO(), result, metav1.UpdateOptions{})
		return updateErr
	})
	if retryErr != nil {
		fmt.Printf("Update failed: %v\n", retryErr)
		return retryErr
	}
	return nil
}

func RotateECRCreds() (string, error) {
	// Read from ECR registry and region
	registry := os.Getenv(ECRRegistry)
	region := os.Getenv(Region)
	accessKey := os.Getenv(AccessKey)
	secretKey := os.Getenv(SecretAccessKey)

	cfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")),
		config.WithRegion(region),
	)
	if err != nil {
		fmt.Printf("failed to create aws config: %v\n", err)
		return Empty, err
	}

	// Get Authorization token for the ECR registry
	ecrSvc := ecr.NewFromConfig(cfg)
	resp, err := ecrSvc.GetAuthorizationToken(context.Background(), &ecr.GetAuthorizationTokenInput{})
	if err != nil {
		fmt.Println(err.Error())
		return Empty, err
	}

	// Filter based on ECR registry
	token := *resp.AuthorizationData[0].AuthorizationToken
	//fmt.Printf("Token: %v\n\n", token)

	// Create docker config
	// Encode token to base64
	encodedToken := base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%s", DockerUser, token)))
	var dockerConfig = map[string]map[string]map[string]string{}
	dockerConfig[AuthList] = map[string]map[string]string{}
	dockerConfig[AuthList][registry] = map[string]string{}
	dockerConfig[AuthList][registry][AuthKey] = encodedToken
	jsonStr, err := json.Marshal(dockerConfig)
	if err != nil {
		fmt.Printf("failed to marshal map to json: %v\n", err)
		return Empty, err
	}

	return string(jsonStr), nil
}
