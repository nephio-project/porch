# Porch Performance Testing Setup Guide

## Prerequisites
- Docker
- Kubernetes CLI (kubectl)
- Go development environment
- Access to GitHub repositories

## 1. Set Up Development Environment
Run the following script to set up the development environment:
```bash
https://github.com/nephio-project/porch/blob/main/scripts/setup-dev-env.sh
```

## 2. Build and Deploy Porch
Execute the command below to build and deploy Porch in a Kubernetes Kind cluster:
```bash
make run-in-kind
```
Once deployment is complete, you should see the related pods running.

## 3. Create a Demo Namespace
Create a namespace for the demo setup:
```bash
kubectl create namespace porch-demo
```

## 4. Set Up Gitea Credentials
Create a secret to store Gitea credentials within the demo namespace:
```bash
kubectl create secret generic gitea \
    --namespace=porch-demo \
    --type=kubernetes.io/basic-auth \
    --from-literal=username=nephio \
    --from-literal=password=secret
```

## 5. Define Gitea Repositories in Porch
Apply the repository configuration file:
```bash
kubectl apply -f porch-repositories.yaml
```

## 6. Verify Repository Setup
Check the status of repositories in the porch-demo namespace:
```bash
kubectl get repositories -n porch-demo
```


## 7. Run Performance Tests
Navigate to the performance test directory:
```bash
cd porch/test/performance/
```

Execute the performance test command:
```bash
E2E=1 go test -v ./... -repos=4 -packages=4 -timeout 60m
```

Parameters:
- `repos`: Number of repositories to test
- `packages`: Number of package revisions in each repository

Once the tests begin, the metrics can be observed.

## 8. Verify Prometheus Targets
Check the Prometheus targets:
```
http://localhost:9090/targets
```
You should be able to run PromQL queries here.

## 9. Check Metrics Output
Retrieve the captured metrics using:
```bash
curl -k http://localhost:2113/metrics
```
This should return all the collected performance metrics from the test execution.