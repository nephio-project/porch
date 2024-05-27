# Table of contents

- [Table of contents](#table-of-contents)
- [Setting up the development environment for Porch](#setting-up-the-development-environment-for-porch)
   * [Setup the environment everything automatically](#setup-the-environment-everything-automatically)
   * [Configure VSCode to run the Porch (api)server](#configure-vscode-to-run-the-porch-apiserver)
   * [Build the CLI](#build-the-cli)
   * [Test that everything works as expected](#test-that-everything-works-as-expected)
      + [Run the porch unit tests](#run-the-porch-unit-tests)
      + [Run the end-to-end tests](#run-the-end-to-end-tests)
- [Create Repositories using your local Porch server](#create-repositories-using-your-local-porch-server)
- [Restart from scratch](#restart-from-scratch)

# Setting up the development environment for Porch

This tutorial gives short instructions on how to set up a development environment for Porch. It outlines the steps to get 
a [kind](https://kind.sigs.k8s.io/) cluster up and running to which a Porch instance running in Visual Studio Code can connect to and interact with.
It is highly recommended to go through the [Starting with Porch tutorial](https://github.com/nephio-project/porch/tree/main/docs/tutorials/starting-with-porch) before this one, if you are not familiar with how porch works.

> **_NOTE:_**  The code itself can be run on a remote VM and we can use the [VSCode Remote SSH](https://marketplace.visualstudio.com/items?itemName=ms-vscode-remote.remote-ssh) plugin to connect to it as our Dev environment.


## Setup the environment everything automatically

This [setup script](bin/setup.sh) automatically bulids a porch development environment. 
Please note that this is not the only possible way to build a working porch development environment, and feel free to customize your own.
The setup script will perform the following steps:
1. Install a kind cluster.   
   The name of the cluster is read from PORCH_TEST_CLUSTER environment variable, otherwise it defaults to `porch-test`.
   The configuration of the cluster is taken from [here](bin/kind_porch_test_cluster.yaml).

1. Install the MetalLB load balancer into the cluster, in order to `LoadBalancer` typed Services to work properly.

1. Install the Gitea git server into the cluster.  
   This can be used to test porch during development, but it is not used in automated end-to-end tests.
   Gitea is exposed to the host via port 3000. The GUI is accessible via http://localhost:3000/nephio, or http://172.18.255.200:3000/nephio (username: nephio, password: secret).

   > **_NOTE:_** If you are using WSL2 (Windows Subsystem for Linux), then Gitea is also accessible from the Windows host via the http://localhost:3000/nephio URL.

1. Generate the PKI resources (key pairs and certificates) required for end-to-end tests.

1. Install porch CRDs into the cluster.

1. Build the porch containers and loads them into the nodes of the kind cluster.

1. Deploy all porch components in the kind cluster, except the porch-server (porch's aggregated API server).  
   The function-runner service will be exposed to the host via 172.18.255.201:9445.

1. Build the porch CLI binary.  
   The result will be generated as `.build/porchctl`.

That's it. If you want to run the steps manually, please use the code of the script as a detailed description.

The setup script is idempotent in the sense that you can rerun it without cleaning up first. This also means that if the script is interrupted for any reason, and you run it again it should continue the process where it left off.


## Configure VSCode to run the Porch (api)server

After the environemnt is set you can start the porch API server localy on your machine. There are multiple ways to do that, the simplest is to run it in a VSCode IDE:

1. Edit your local `.vscode.launch.json` file as follows: Change the `--kubeconfig` argument of the `Launch Server` configuration to point to a KUBECONFIG file that is set to the kind cluster as the current context.

1. You can now launch the Porch server locally in VSCode by selecting the "Launch Server" configuration on the VSCode "Run and Debug" window. for
more information please refer to the [VSCode debugging documentation](https://code.visualstudio.com/docs/editor/debugging).

1. Check that the apiservice is now Ready:
```
kubectl get apiservice v1alpha1.porch.kpt.dev
```
Sample output:
```
NAME                     SERVICE            AVAILABLE   AGE
v1alpha1.porch.kpt.dev   porch-system/api   True        18m
```
Check the porch api-resources:
```
kubectl api-resources | grep porch
```
Sample output:
```
...

functions                                      porch.kpt.dev/v1alpha1            true         Function
packagerevisionresources                       porch.kpt.dev/v1alpha1            true         PackageRevisionResources
packagerevisions                               porch.kpt.dev/v1alpha1            true         PackageRevision
packages                                       porch.kpt.dev/v1alpha1            true         Package

```

Check to ensure that the apiserver is serving requests:
```
curl https://localhost:4443/apis/porch.kpt.dev/v1alpha1 -k
```

<details closed>
<summary>Sample output</summary>

```json
{
  "kind": "APIResourceList",
  "apiVersion": "v1",
  "groupVersion": "porch.kpt.dev/v1alpha1",
  "resources": [
    {
      "name": "functions",
      "singularName": "",
      "namespaced": true,
      "kind": "Function",
      "verbs": [
        "get",
        "list"
      ]
    },
    {
      "name": "packagerevisionresources",
      "singularName": "",
      "namespaced": true,
      "kind": "PackageRevisionResources",
      "verbs": [
        "get",
        "list",
        "patch",
        "update"
      ]
    },
    {
      "name": "packagerevisions",
      "singularName": "",
      "namespaced": true,
      "kind": "PackageRevision",
      "verbs": [
        "create",
        "delete",
        "get",
        "list",
        "patch",
        "update",
        "watch"
      ]
    },
    {
      "name": "packagerevisions/approval",
      "singularName": "",
      "namespaced": true,
      "kind": "PackageRevision",
      "verbs": [
        "get",
        "patch",
        "update"
      ]
    },
    {
      "name": "packages",
      "singularName": "",
      "namespaced": true,
      "kind": "Package",
      "verbs": [
        "create",
        "delete",
        "get",
        "list",
        "patch",
        "update"
      ]
    }
  ]
}
```
</details>


## Build the CLI

Build the porchctl CLI in the git root folder by

```
make porchctl
```

and then copy the result from `.build/porchctl` to somewhere in your $PATH.


## Test that everything works as expected

Make sure that the porch server is still running in VS Code and than run the following tests from the git root folder.

### Run the porch unit tests

```
make test
```

### Run the end-to-end tests
In order for the end-to-end tests to run properly for a locally running API server, you have to add the following line to your `/etc/hosts` file:
```
127.0.0.1         api.porch-system.svc
```
TODO: remove this requirement

To test porch directly via its API:
```
E2E=1 go test -v ./test/e2e
```

To test porch via its CLI:
```
E2E=1 go test -v ./test/e2e/cli
```

# Create Repositories using your local Porch server

To connect Porch to Gitea, follow [step 7 in the Starting with Porch](https://github.com/nephio-project/porch/tree/main/docs/tutorials/starting-with-porch#Connect-the-Gitea-repositories-to-Porch) tutorial to create the repositories in Porch.

You will notice logging messages in VSCode when you run the command `kubectl apply -f porch-repositories.yaml` command.

You can check that your locally running Porch server has created the repositories by running the `porchctl` command:

```
porchctl repo get -A
```
Sample output:
```
NAME                  TYPE   CONTENT   DEPLOYMENT   READY   ADDRESS
external-blueprints   git    Package   false        True    https://github.com/nephio-project/free5gc-packages.git
management            git    Package   false        True    http://172.18.255.200:3000/nephio/management.git
```

You can also check the repositories using kubectl.

```
kubectl get  repositories -n porch-demo
```

Sample output:
```
NAME                  TYPE   CONTENT   DEPLOYMENT   READY   ADDRESS
external-blueprints   git    Package   false        True    https://github.com/nephio-project/free5gc-packages.git
management            git    Package   false        True    http://172.18.255.200:3000/nephio/management.git
```

You now have a locally running Porch (api)server. Happy developing!


# Restart from scratch

Sometimes the development cluster gets cluttered and you may experience weird behaviour from porch. 
In this case you might want to restart with a clean slate, by deleting the dvelopemnt cluster with the follwoing command:
```
kind delete cluster --name porch-test
```

and running the [setup script](bin/setup.sh) again:
```
docs/tutorials/porch-development-environment/bin/setup.sh
```