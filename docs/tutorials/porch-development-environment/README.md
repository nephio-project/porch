# Setting up a development environment for Porch

This tutorial gives short instructions on how to set up a development environment for Porch. It outlines the steps to get a [kind](https://kind.sigs.k8s.io/) cluster up
and running to which a Porch instance running in Visual Studio Code can connect to and interact with.

# Setup kind with MetalLB and Gitea

Follow steps 1-5 inclusive of the [Starting with Porch](https://github.com/nephio-project/porch/tree/main/docs/tutorials/starting-with-porch) tutorial. You now have 2 Kind clusters
running with Gitea installed. Gitea has the repositories `management` and `edge1` defined.

> **_NOTE:_** The [setup script](bin/setup.sh) automates steps 1-5 of the Starting with Porch tutorial. You may need to adaot this script to your local environment.

> **_NOTE:_** The [cleardown script script](bin/cleardown.sh) clears everything down by deleting the `management` and `edge1` Kind clusters. USE WITH CARE.


You can reach the Gitea web interface on the address reported byt he following command:
```
kubectl get svc -n gitea gitea        
NAME    TYPE           CLUSTER-IP      EXTERNAL-IP      PORT(S)                       AGE
gitea   LoadBalancer   10.197.10.118   172.18.255.200   22:31260/TCP,3000:31012/TCP   8m35s
```

# Install the Porch function runner

The Porch server requires that the Porch function runner is executing. To install the Porch function runner on the Kind management cluster, execute the following commands.

```
kubectl apply -f https://raw.githubusercontent.com/nephio-project/catalog/main/nephio/core/porch/0-packagerevs.yaml
kubectl apply -f https://raw.githubusercontent.com/nephio-project/catalog/main/nephio/core/porch/0-packagevariants.yaml
kubectl apply -f https://raw.githubusercontent.com/nephio-project/catalog/main/nephio/core/porch/0-packagevariantsets.yaml
kubectl apply -f https://raw.githubusercontent.com/nephio-project/catalog/main/nephio/core/porch/0-repositories.yaml
kubectl apply -f https://raw.githubusercontent.com/nephio-project/catalog/main/nephio/core/porch/1-namespace.yaml
kubectl apply -f https://raw.githubusercontent.com/nephio-project/catalog/main/nephio/core/porch/2-function-runner.yaml

kubectl wait --namespace porch-system \
                --for=condition=ready pod \
                --selector=app=function-runner \
                --timeout=300s
```

The Porch function runner should now be executing

```
kubectl get pod -n porch-system --selector=app=function-runner
NAME                              READY   STATUS    RESTARTS   AGE
function-runner-67d4c7c7b-7wm97   1/1     Running   0          16m
function-runner-67d4c7c7b-czvvq   1/1     Running   0          16m
```

Expose the `function-runner` service so that the Porch server running in Visual Studio Code can reach it, change the service type from `ClusterIP` to `LoadBalancer`:

```
kubectl edit svc -n porch-system function-runner

31c31
<   type: ClusterIP
---
>   type: LoadBalancer
```

Now check that the `function-runner` service has been assigned an IP address external to the cluster:
```
kubectl get svc -n porch-system function-runner
NAME              TYPE           CLUSTER-IP       EXTERNAL-IP      PORT(S)          AGE
function-runner   LoadBalancer   10.197.168.148   172.18.255.201   9445:31794/TCP   22m
```

# Install Porch resources for standalone execution

The Porch server requires that the following resources are defined int he K8S cluster where it is executed:

- The `porch-system` namespace, an API Service called `apiservice.apiregistration.k8s.io/v1alpha1.porch.kpt.dev` and the `service.api` service to expose the API Service. These resources are defined in the the file `deployments/local/localconfig.yaml`
- the `repositories.config.porch.kpt.dev` and `functions.config.porch.kpt.dev` CRDs. These CRDs are defined in the `api/porchconfig/v1alpha1/` directory.
- the `packagerevs.config.porch.kpt.dev` CRD. This CRD is defined in the `internal/api/porchinternal/v1alpha1/` directory.
- 

```
kubectl apply -f https://raw.githubusercontent.com/nephio-project/porch/main/deployments/local/localconfig.yaml
kubectl apply -f https://raw.githubusercontent.com/nephio-project/porch/main/api/porchconfig/v1alpha1/config.porch.kpt.dev_repositories.yaml
kubectl apply -f https://raw.githubusercontent.com/nephio-project/porch/main/api/porchconfig/v1alpha1/config.porch.kpt.dev_functions.yaml
kubectl apply -f https://raw.githubusercontent.com/nephio-project/porch/main/internal/api/porchinternal/v1alpha1/config.porch.kpt.dev_packagerevs.yaml
```
Verify that the resources have been created
```
kubectl api-resources | grep -i porch
functions                                      config.porch.kpt.dev/v1alpha1     true         Function
packagerevs                                    config.porch.kpt.dev/v1alpha1     true         PackageRev
packagevariants                                config.porch.kpt.dev/v1alpha1     true         PackageVariant
packagevariantsets                             config.porch.kpt.dev/v1alpha2     true         PackageVariantSet
repositories                                   config.porch.kpt.dev/v1alpha1     true         Repository
```

# Configure VSCode to run the Porch server

Check out porch and start vscode in the root of your checked out Porch repo.

Edit your local `.vscode.launch.json` file as follows:
1. Change the `--kubeconfig` value to point at your local Kind cluster configuration file
2. Change the `--function-runner` IP address to that of the function runner service running in the Kind `management` cluster

```
        {
            "name": "Launch Server",
            "type": "go",
            "request": "launch",
            "mode": "auto",
            "program": "${workspaceFolder}/cmd/porch/main.go",
            "args": [
                "--secure-port=9443",
                "--v=7",
                "--standalone-debug-mode",
                "--kubeconfig=/home/sean-citizen/.kube/kind-management-config",
                "--cache-directory=${workspaceFolder}/.cache",
                "--function-runner=172.18.255.201:9445"
            ],
            "env": {
				"KUBECONFIG": "/Users/liam/.kube/kind-management-config"
			},
            "cwd": "${workspaceFolder}"
        },
```

You can now launch the Porch server locally in VSCode by selecting the "Launch Server" configuration on the VSCode "Run and Debug" window.

# Create Repositories using your local Porch server

Follow step 6 to connect Gitea to Porch in the [Starting with Porch](https://github.com/nephio-project/porch/tree/main/docs/tutorials/starting-with-porch#connect-the-gitea-repositories-to-porch) tutorial to create the Gitea and external repositories in Porch.

You will notice logging messages in VSCode when you run the command `kubectl apply -f porch-repositories.yaml` command.

You can check that your locally running Porch server has created the repositories by running the `porchctl` command.

```
porchctl repo get -A
NAME                  TYPE   CONTENT   DEPLOYMENT   READY   ADDRESS
edge1                 git    Package   true         True    http://172.18.255.200:3000/nephio/edge1.git
external-blueprints   git    Package   false        True    https://github.com/nephio-project/free5gc-packages.git
management            git    Package   false        True    http://172.18.255.200:3000/nephio/management.git
```

You can also check the repositories using kubectl.

```
kubectl get  repositories -n porch-demo  
NAME                  TYPE   CONTENT   DEPLOYMENT   READY   ADDRESS
edge1                 git    Package   true         True    http://172.18.255.200:3000/nephio/edge1.git
external-blueprints   git    Package   false        True    https://github.com/nephio-project/free5gc-packages.git
management            git    Package   false        True    http://172.18.255.200:3000/nephio/management.git
```

You now have a locally running Porch server. Happy developing!