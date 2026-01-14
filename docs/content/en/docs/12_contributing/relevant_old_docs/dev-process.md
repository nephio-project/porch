---
title: "Development process"
type: docs
weight: 3
description:
---

<div style="border: 1px solid red; background-color: #ffe6e6; color: #b30000; padding: 1em; margin-bottom: 1em;">
  <strong>⚠️ Outdated Notice:</strong> This page refers to an older version of the documentation. This content has simply been moved into its relevant new section here and must be checked, modified, rewritten, updated, or removed entirely.
</div>

After you ran the setup script as explained in the [environment setup]({{% relref "/docs/6_configuration_and_deployments/relevant_old_docs/environment-setup.md" %}}) you are ready to start the actual development of porch. That process involves (among others) a combination of the tasks explained below.

## Build and deploy all of porch

The following command will rebuild all of porch and deploy all of its components into your porch-test kind cluster (created in the [environment setup]({{% relref "/docs/6_configuration_and_deployments/relevant_old_docs/environment-setup.md" %}})):

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

### Configure VS Code to run the Porch (API)server

The simplest way to run the porch API server is to launch it in a VS Code IDE, as described by the following process:

1. Open the *porch.code-workspace* file in the root of the porch git repository.

1. Edit your local *.vscode/launch.json* file as follows: Change the `--kubeconfig` argument of the Launch Server
   configuration to point to a *KUBECONFIG* file that is set to the kind cluster as the current context. 

{{% alert title="Note" color="primary" %}}

  If your current *KUBECONFIG* environment variable already points to the porch-test kind cluster, then you don't have to touch anything.

  {{% /alert %}}

1. Launch the Porch server locally in VS Code by selecting the *Launch Server* configuration on the VS Code
   *Run and Debug* window. For more information please refer to the
   [VS Code debugging documentation](https://code.visualstudio.com/docs/editor/debugging).

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

There are several ways to develop, test and troubleshoot the porch controllers (i.e. *PackageVariant*, *PackageVariantSet*). In this chapter we describe an option where every other parts of porch is running in the porch-test kind cluster, but the process hosting all porch controllers is running locally on your machine.

The following command will rebuild and deploy porch, except the porch-controllers component:

```bash
make run-in-kind-no-controllers
```

After issuing this command you are expected to start the porch controllers process locally on your machine (outside of
the kind cluster); probably in your IDE, potentially in a debugger. If you are using VS Code you can use the
**Launch Controllers** configuration that is defined in the
[launch.json](https://github.com/nephio-project/porch/blob/main/.vscode/launch.json) file of the porch git repository.

## Run the unit tests

```bash
make test
```

## Run the end-to-end tests

To run the end-to-end tests against the Kubernetes API server where *KUBECONFIG* points to, simply issue:

```bash
make test-e2e
```

To run the end-to-end tests against a clean deployment, issue:

```bash
make test-e2e-clean
```
This will 
- create a brand new kind cluster, 
- rebuild porch
- deploy the newly built porch into the new cluster
- run the end-to-end tests against that
- deletes the kind cluster if all tests passed

This process closely mimics the end-to-end tests that are run against your PR on GitHub.

In order to run just one particular test case you can execute something similar to this:

```bash
E2E=1 go test -v ./test/e2e -run TestE2E/PorchSuite/TestPackageRevisionInMultipleNamespaces
```
or this: 
```bash
E2E=1 go test -v ./test/e2e/cli -run TestPorch/rpkg-lifecycle

```

To run the end to end tests on your local machine towards a Porch server running in VS Code, be aware of the following if the tests are not running:
- Set the actual load balancer IP address for the function runner in your *launch.json*, for example
  "--function-runner=172.18.255.201:9445"
- Clear the git cache of your Porch workspace before every test run, for example
  `rm -fr <workspace_root_dir>/.cache/git/*`

## Run the load test

A script is provided to run a Porch load test against the Kubernetes API server where *KUBECONFIG* points to.

```bash
porch % scripts/run-load-test.sh -h

run-load-test.sh - runs a load test on porch

       usage:  run-load-test.sh [-options]

       options
         -h                        - this help message
         -s hostname               - the host name of the git server for porch git repositories
         -r repo-count             - the number of repositories to create during the test, a positive integer
         -p package-count          - the number of packages to create in each repo during the test, a positive integer
         -e package-revision-count - the number of packagerevisions to create on each package during the test, a positive integer
         -f result-file            - the file where the raw results will be stored, defaults to load_test_results.txt
         -o repo-result-file       - the file where the results by reop will be stored, defaults to load_test_repo_results.csv
         -l log-file               - the file where the test log will be stored, defaults to load_test.log
         -y                        - dirty mode, do not clean up after tests
```

The load test creates, copies, proposes and approves `repo-count` repositories, each with `package-count` packages
with `package-revision-count` package revisions created for each package. The script initializes or copies each
package revision in turn. It adds a pipeline with two "apply-replacements" kpt functions to the Kptfile of each
package revision. It updates the package revision, and then proposes and approves it.

The load test script creates repositories on the git server at `hostname`, so it's URL will be `http://nephio:secret@hostname:3000/nephio/`.
The script expects a git server to be running at that URL.

The `result-file` is a text file containing the time it takes for a package to move from being initialized or
copied to being approved. It also records the time it takes to proppose-delete and delete each package revision.

The `repo-result-file` is a CSV file that tabulates the results from `result-file` into columns for each repository created.

For example:

```bash
porch % scripts/run-load-test.sh -s 172.18.255.200 -r 4 -p 2 -e 3
running load test towards git server http://nephio:secret@172.18.255.200:3000/nephio/
  4 repositories will be created
  2 packages in each repo
  3 pacakge revisions in each package
  results will be stored in "load_test_results.txt"
  repo results will be stored in "load_test_repo_results.csv"
  the log will be stored in "load_test.log"
load test towards git server http://nephio:secret@172.18.255.200:3000/nephio/ completed
```

In the load test above, a total of 24 package revisions were created and deleted.

|REPO-1-TEST|REPO-1-TIME|REPO-2-TEST|REPO-2-TIME|REPO-3-TEST|REPO-3-TIME|REPO-4-TEST|REPO-4-TIME|
|-----------|-----------|-----------|-----------|-----------|-----------|-----------|-----------|
1:1|1.951846|1:1|1.922723|1:1|2.019615|1:1|1.992746
1:2|1.762657|1:2|1.864306|1:2|1.873962|1:2|1.846436
1:3|1.807281|1:3|1.930068|1:3|1.860375|1:3|1.881649
2:1|1.829227|2:1|1.904997|2:1|1.956160|2:1|1.988209
2:2|1.803494|2:2|1.912169|2:2|1.915905|2:2|1.902103
2:3|1.816716|2:3|1.948171|2:3|1.931904|2:3|1.952902
del-6a0b3…|.918442|del-e757b…|.904881|del-d39cd…|.944850|del-6222f…|.911060
del-378a4…|.831815|del-9211c…|.866386|del-316a5…|.898638|del-31d9f…|.895919
del-89073…|.874867|del-97d45…|.876450|del-830e0…|.905896|del-7d411…|.866947
del-4756f…|.850528|del-c95db…|.903599|del-4c450…|.884997|del-587f8…|.842529
del-9860a…|.887118|del-9c1b9…|1.018930|del-66ae…|.929470|del-6ae3d…|.905359
del-a11e5…|.845834|del-71540…|.899935|del-8d1e8…|.891296|del-9e2bb…|.864382
del-1d789…|.851242|del-ffdc3…|.897862|del-75e45…|.852323|del-82eef…|.916630
del-8ae7e…|.872696|del-58097…|.894618|del-d164f…|.852093|del-9da24…|.849919

## Switching between tasks

The `make run-in-kind`, `make run-in-kind-no-server` and `make run-in-kind-no-controller` commands can be executed right after each other. No clean-up or restart is required between them. The make scripts will intelligently do the necessary changes in your current porch deployment in kind (e.g. removing or re-adding the porch API server).

You can always find the configuration of your current deployment in *.build/deploy*.

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

then re-run the [setup script](https://github.com/nephio-project/porch/blob/main/scripts/setup-dev-env.sh):

```bash
./scripts/setup-dev-env.sh
```

finally deploy porch into the kind cluster by any of the methods explained above.

