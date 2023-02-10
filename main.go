package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	"github.com/robfig/cron/v3"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"

	"ecrcredsrotate/secret"
	"ecrcredsrotate/serviceaccount"
	"ecrcredsrotate/utils"
)

const (
	ECRRegistry     = "ECR_REGISTRY"
	Region          = "REGION"
	AccessKey       = "ACCESS_KEY_ID"
	SecretAccessKey = "SECRET_ACCESS_KEY"
	DockerUser      = "AWS"

	// Docker config constants
	AuthList         = "auths"
	AuthKey          = "auth"

	DefaultSA  = "default"
	Empty      = ""

	DefaultRegion = "us-east-1"
	KubeConfig = "KUBECONFIG"
)

var (
	log = utils.GetLogger()
)

type UpdateJob struct {
	client *kubernetes.Clientset
}

func main() {
	var config *rest.Config
	var err error
	
	kubeconfig := utils.GetEnv(KubeConfig, Empty)
	if kubeconfig != Empty {
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			log.Errorf("failed to build from kubeconfig: %v", err)
			os.Exit(1)
		}
	} else {
		config, err = rest.InClusterConfig()
		if err != nil {
			panic(err.Error())
		}
	}

	// create the clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Errorf("failed to create client from config: %v", err)
	}

	// Run cron to rotate and update secret
	c := cron.New()
	updateJob := UpdateJob{client: clientset}
	// Run every 11 hours
	c.AddJob("*/11 * * * *", updateJob)
	c.Start()

	// Dynamic client for informer
	clusterClient, err := dynamic.NewForConfig(config)
	if err != nil {
		log.Errorf("failed to create client: %v", err)
	}

	// Listen/Watch for namespaces
	resource := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "namespaces"}
	// Make use of shared informer factory
	factory := dynamicinformer.NewFilteredDynamicSharedInformerFactory(clusterClient, time.Minute, corev1.NamespaceAll, nil)
	informer := factory.ForResource(resource).Informer()

	// Run informer
	stopCh := make(chan struct{})
	go startWatching(stopCh, informer, clientset)

	sigCh := make(chan os.Signal, 0)
	signal.Notify(sigCh, os.Kill, os.Interrupt)

	<-sigCh
	close(stopCh)
}

// Run rotates ecr creds and updates secret in all namespaces
// This is will be run as Cron Job
func (job UpdateJob) Run() {
	namespaces, err := job.client.CoreV1().Namespaces().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		log.Errorf("failed to list namespaces: %v", err)
	}

	dockerConfig, err := RotateECRCreds()
	if err != nil {
		log.Errorf("failed to get docker config: %v", err)
	}
	for _, ns := range namespaces.Items {
		err := secret.UpdateDockerConfig(job.client, secret.CredSecretName, ns.Name, dockerConfig)
		if err != nil {
			log.Errorf("failed to update secret %s in namespace %s: %v", secret.CredSecretName, ns.Name, err)
		}
		log.Infof("Updated secret %s in namespace %s", secret.CredSecretName, ns.Name)
	}
}

// startWatching listens/watches for new namespace creation.
// When new namespace is created, ecr creds are created and stored in secret along with patching of default service account
func startWatching(stopCh <-chan struct{}, s cache.SharedIndexInformer, client *kubernetes.Clientset) {
	handlers := cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			u := obj.(*unstructured.Unstructured)
			ns := u.GetName()
			log.Infof("Received add event! for %v", ns)
			dockerConfig, err := RotateECRCreds()
			if err != nil {
				log.Errorf("failed to get docker config: %v", err)
			}

			log.Infof("Create secret %s in namespace %s", secret.CredSecretName, ns)
			_, err = secret.CreateDockerConfig(client, secret.CredSecretName, ns, dockerConfig)
			if err != nil {
				log.Error(err)
				// Update secret
				log.Infof("Updating secret %s in namespace %s", secret.CredSecretName, ns)
				err = secret.UpdateDockerConfig(client, secret.CredSecretName, ns, dockerConfig)
				if err != nil {
					log.Errorf("failed to update secret: %v", err)
				}
			}

			// Patch imagePullSecrets for default service account
			log.Infof("Patch service account %s with secret %s as ImagePullSecret", DefaultSA, secret.CredSecretName)
			err = serviceaccount.PatchImagePullSecrets(client, DefaultSA, ns, secret.CredSecretName)
			if err != nil {
				log.Errorf("failed to patch service account %s: %v", DefaultSA, err)
			}
		},
	}

	s.AddEventHandler(handlers)
	s.Run(stopCh)
}

// RotateECRCreds rotates ecr creds and returns docker config json
func RotateECRCreds() (string, error) {
	// Read from ECR registry and region
	registry := os.Getenv(ECRRegistry)
	region := utils.GetEnv(Region, DefaultRegion)
	accessKey := os.Getenv(AccessKey)
	secretKey := os.Getenv(SecretAccessKey)

	cfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")),
		config.WithRegion(region),
	)
	if err != nil {
		log.Errorf("failed to create aws config: %v", err)
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

	// Create docker config
	// Encode token to base64
	encodedToken := base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%s", DockerUser, token)))
	var dockerConfig = map[string]map[string]map[string]string{}
	dockerConfig[AuthList] = map[string]map[string]string{}
	dockerConfig[AuthList][registry] = map[string]string{}
	dockerConfig[AuthList][registry][AuthKey] = encodedToken
	jsonStr, err := json.Marshal(dockerConfig)
	if err != nil {
		log.Errorf("failed to marshal map to json: %v", err)
		return Empty, err
	}

	return string(jsonStr), nil
}
