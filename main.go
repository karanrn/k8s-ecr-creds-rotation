package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	"os"
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

	// Read from ECR registry and region
	registry := os.Getenv(ECRRegistry)
	region := os.Getenv(Region)
	accessKey := os.Getenv(AccessKey)
	secretKey := os.Getenv(SecretAccessKey)

	fmt.Printf("Registry: %s\nRegion: %s\n", registry, region)
	fmt.Printf("Access Key: %s\nSecret Key: %s\n", accessKey, secretKey)

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
	fmt.Printf("Proxy Endpoint: %v\n", *resp.AuthorizationData[0].ProxyEndpoint)
	token := *resp.AuthorizationData[0].AuthorizationToken
	fmt.Printf("Token: %v\n\n", token)

	// Create docker config
	// Encode token to base64
	encodedToken := base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%s", DockerUser, token)))
	var dockerConfig = map[string]map[string]map[string]string{}
	dockerConfig[AuthList] = map[string]map[string]string{}
	dockerConfig[AuthList][registry] = map[string]string{}
	dockerConfig[AuthList][registry][AuthKey] = encodedToken
	jsonStr, _ := json.Marshal(dockerConfig)
	fmt.Println(string(jsonStr))
}

