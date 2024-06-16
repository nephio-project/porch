# Overview of a possible development process

After you ran the setup script as explained in the [first part of the tutorial](README.md) you are ready to start the actual development of porch. That process involves (among others) a combination of the tasks explained below.

  * [Build and deploy all of porch](#build-and-deploy-all-of-porch)
  * [Troubleshoot the porch API server](#troubleshoot-the-porch-api-server)
    + [Configure VSCode to run the Porch (api)server](#configure-vscode-to-run-the-porch--api-server)
    + [Check to ensure that the API server is serving requests:](#check-to-ensure-that-the-api-server-is-serving-requests-)
  * [Troubleshoot the porch controllers](#troubleshoot-the-porch-controllers)
  * [Run the unit tests](#run-the-unit-tests)
  * [Run the end-to-end tests](#run-the-end-to-end-tests)
  * [Switching between tasks](#switching-between-tasks)
  * [Getting to know the make targets](#getting-to-know-the-make-targets)
  * [Restart with a clean-slate](#restart-with-a-clean-slate)

## Build and deploy all of porch

The following command will rebuild all of porch and deploy all of its components into your porch-test kind cluster (created in the [first part of the tutorial](README.md)):

```bash
make run-in-kind
```

## Troubleshoot the porch API server

There are several ways to develop, test and troubleshoot the porch API server. In this chapter we describe an option where every other parts of porch is running in the porch-test kind cluster, but the porch API server is running locally on your machine, typically in an IDE.

The following command will rebuild and deploy porch, except the porch API server component, and also prepares your environment for connecting the local API server with the in-cluster components.

```bash
make run-in-kind-no-server
```

After issuing this command you are expected to start the porch API server locally on your machine (outside of the kind cluster); probably in your IDE, potentially in a debugger.

### Configure VSCode to run the Porch (api)server

The simplest way to run the porch API server is to launch it in a VSCode IDE, as described by the following process:

1. Open the `porch.code-workspace` file in the root of the porch git repo.

1. Edit your local `.vscode/launch.json` file as follows: Change the `--kubeconfig` argument of the `Launch Server` configuration to point to a KUBECONFIG file that is set to the kind cluster as the current context. 
  > **_NOTE:_**  If your current KUBECONFIG environment variable already points to the porch-test kind cluster, then you don't have to touch anything.

1. Launch the Porch server locally in VSCode by selecting the "Launch Server" configuration on the VSCode "Run and Debug" window. For more information please refer to the [VSCode debugging documentation](https://code.visualstudio.com/docs/editor/debugging).

### Check to ensure that the API server is serving requests:

```bash
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


## Troubleshoot the porch controllers

There are several ways to develop, test and troubleshoot the porch controllers (i.e. PackageVariant, PackageVariantSet). In this chapter we describe an option where every other parts of porch is running in the porch-test kind cluster, but the process hosting all porch controllers is running locally on your machine.

The following command will rebuild and deploy porch, except the porch-controllers component:

```bash
make run-in-kind-no-controllers
```

After issuing this command you are expected to start the porch controllers process locally on your machine (outside of the kind cluster); probably in your IDE, potentially in a debugger.

## Run the unit tests

```bash
make test
```

## Run the end-to-end tests

To run the end-to-end tests against the Kubernetes API server where KUBECONFIG points to, simply issue:

```bash
make test-e2e
```

To run the end-to-end tests against a clean deployment, simply issue:

```bash
make test-e2e-clean
```
This will 
- create a brand new kind cluster, 
- rebuild porch
- deploy the newly built porch into the new cluster
- run the end-to-end tests against that
- deletes the kind cluster if all tests passed

This process closely mimics the end-to-end tests that are run against your PR on Github.

In order to run just one particular test case you can execute something similar to this:

```bash
E2E=1 go test -v ./test/e2e -run TestE2E/PorchSuite/TestPackageRevisionInMultipleNamespaces
```
or this: 
```bash
E2E=1 go test -v ./test/e2e/cli -run TestPorch/rpkg-lifecycle

```

## Switching between tasks

The `make run-in-kind`, `make run-in-kind-no-server` and `make run-in-kind-no-controller` commands can be executed right after each other. No clean-up or restart is required between them. The make scripts will intelligently do the necessary changes in your current porch deployment in kind (e.g. removing or re-adding the porch API server).

You can always find the configuration of your current deployment in `.build/deploy`.

You can always use `make test` and `make test-e2e` to test your current setup, no matter which of the above detailed configurations it is.

## Getting to know the make targets

Try: `make help`

## Restart with a clean-slate

Sometimes the development kind cluster gets cluttered and you may experience weird behavior from porch.
In this case you might want to restart with a clean slate:
First, delete the development kind cluster with the following command:

```bash
kind delete cluster --name porch-test
```

then re-run the [setup script](../../../scripts/setup-dev-env.sh):

```bash
./scripts/setup-dev-env.sh
```

finally deploy porch into the kind cluster by any of the methods explained above.

