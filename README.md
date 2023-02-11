# ECR Credentials Rotation
### Motivation
As ECR credentials(creds) expire in 12 hours, it becomes necessary to rotate them without impacting any deployments to the cluster.

### Introduction
When we use ECR as container image registry for your CICD builds it becomes essential to configure Kubernetes(K8s) to pull from the ECR both for security and ease for users/developers of K8s as a platform. 

In case of K8s cluster hosted on AWS, it can be easily configured with IAM roles and permissions to pull from ECR but does not apply when K8s cluster is hosted on non-AWS world.
Other way is to use Access keys with appropriate permission to generate token/creds to pull from the ECR - 
```
docker login -U AWS -p $(aws ecr get-login-password --region us-east-1) <private-ecr-registry>
```
But these creds expire in 12 hours after generating which forces us to re-generate them

So for workloads to pull from private registry, we need to create [ImagePullSecret](https://kubernetes.io/docs/tasks/configure-pod-container/pull-image-private-registry/) which needs to be used as part of workloads.

Workloads can be deployed in any existing or new namespace so secret with ECR creds has to be present so workloads to use as part of `ImagePullSecret`. 

So we have 2 conditions:
* Secret has to be updated with rotated ECR creds
* Secret has to be created in every new namespace that gets created

### Solution
We generate ECR creds using AWS Access and Secret Access keys and create secret `regcred` as type `kubernetes.io/dockerconfigjson` in every namespace.

ECR creds are generated every 11 hours and secret `regcred` is updated in all namespaces. And we run controller watching for namespace creation which creates `regcred` with ECR creds once new namespace is created.

We also patch `default` service account so workloads not mentioning `imagePullSecrets` and service account should still be deployed.

---
### Further Reading
1. [K8s Informer](https://aly.arriqaaq.com/kubernetes-informers/)
2. [Pull image from private registry](https://kubernetes.io/docs/tasks/configure-pod-container/pull-image-private-registry/)
3. [Private ECR registry authentication](https://docs.aws.amazon.com/AmazonECR/latest/userguide/registry_auth.html)
4. [ImagePullSecrets to service account](https://kubernetes.io/docs/tasks/configure-pod-container/configure-service-account/#add-imagepullsecrets-to-a-service-account)
