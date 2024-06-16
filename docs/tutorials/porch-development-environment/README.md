# Table of contents

- [Setting up the development environment for Porch](#setting-up-the-development-environment-for-porch)
  * [Extra steps for MacOS users](#extra-steps-for-macos-users)
  * [Setup the environment automatically](#setup-the-environment-automatically)
  * [Extra manual steps](#extra-manual-steps)
  * [Build and deploy porch](#build-and-deploy-porch)
  * [Check that everything works as expected](#check-that-everything-works-as-expected)
    + [Check that the apiservice is ready](#check-that-the-apiservice-is-ready)
    + [Check the porch api-resources](#check-the-porch-api-resources)
- [Create Repositories using your local Porch server](#create-repositories-using-your-local-porch-server)
- [Restart from scratch](#restart-from-scratch)
- [Getting started with actual development](dev-process.md)

# Setting up the development environment for Porch

This tutorial gives short instructions on how to set up a development environment for Porch. It outlines the steps to get
a [kind](https://kind.sigs.k8s.io/) cluster up and running to which a Porch instance running in Visual Studio Code can connect to and interact with.
if you are not familiar with how porch works, it is highly recommended that you go through the [Starting with Porch tutorial](https://github.com/nephio-project/porch/tree/main/docs/tutorials/starting-with-porch) before going through this one.

> **_NOTE:_**  As your Dev environment, you can run the code on a remote VM and use the [VSCode Remote SSH](https://marketplace.visualstudio.com/items?itemName=ms-vscode-remote.remote-ssh) plugin to connect to it. 
TODO: re-check WSL2 + Docker Desktop setup

## Extra steps for MacOS users

The script the `make deployment-config` target to generate the deployment files for porch. The scripts called by this make target use recent `bash` additions and expect the gnu version of the `sed` command. MacOS comes with `bash` 3.x.x by default and with a version of `sed` that is incompatible with gnu `sed`. TODO: check if sed is indeed still used.

1. Install `bash` 4.x.x or better of `bash` using homebrew, see [this this post for details](https://apple.stackexchange.com/questions/193411/update-bash-to-version-4-0-on-osx)
2. Install `gsed` using homebrew, see [this post for details](https://stackoverflow.com/questions/4247068/sed-command-with-i-option-failing-on-mac-but-works-on-linux)
3. `sudo ln -s /opt/homebrew/bin/gsed /opt/homebrew/bin/sed` create a soft link for `sed` from `gsed`
4. Ensure that `/opt/homebrew/bin` is earlier in your path than `/bin` and `/usr/bin`

> **_NOTE:_**  The changes above **permanently** change the `bash` version and `sed` for **all** applications and may cause side effects. You may wish to revert the changes (particularly the `sed-->gsed` soft link) when you complete your work on Porch.

## Setup the environment automatically

The [`./scripts/setup-dev-env.sh` setup script](../../../scripts/setup-dev-env.sh) automatically bulids a porch development environment.
Please note that this is only one of many possible ways of building a working porch development environment so feel free to customize it to suit your needs.
The setup script will perform the following steps:

1. Install a kind cluster.
   The name of the cluster is read from the PORCH_TEST_CLUSTER environment variable, otherwise it defaults to `porch-test`.
   The configuration of the cluster is taken from [here](../../../deployments/local/kind_porch_test_cluster.yaml).

1. Install the MetalLB load balancer into the cluster, in order to allow `LoadBalancer` typed Services to work properly.

1. Install the Gitea git server into the cluster.  
   This can be used to test porch during development, but it is not used in automated end-to-end tests.
   Gitea is exposed to the host via port 3000. The GUI is accessible via <http://localhost:3000/nephio>, or <http://172.18.255.200:3000/nephio> (username: nephio, password: secret).

   > **_NOTE:_** If you are using WSL2 (Windows Subsystem for Linux), then Gitea is also accessible from the Windows host via the <http://localhost:3000/nephio> URL.

1. Generate the PKI resources (key pairs and certificates) required for end-to-end tests.

1. <a id="CLI">Build the porch CLI binary.</a>  
   The result will be generated as `.build/porchctl`.

That's it! If you want to run the steps manually, please use the code of the script as a detailed description.

The setup script is idempotent in the sense that you can rerun it without cleaning up first. This also means that if the script is interrupted for any reason, and you run it again it should effectively continue the process where it left off.

## Extra manual steps

Copy the `.build/porchctl` binary (that was built by the setup script) to somewhere in your $PATH, or add the `.build` directory to your PATH.

## Build and deploy porch

You can build all of porch, and also deploy it into your newly created kind cluster with this command.

```bash
make run-in-kpt
```

See more advanced variants of this command in the [detailed description of the development process](dev-process.md).

## Check that everything works as expected

At this point you are basically ready to start developing porch, but before you start it is worth checking that everything works as expected.

### Check that the apiservice is ready

```bash
kubectl get apiservice v1alpha1.porch.kpt.dev
```

Sample output:

```bash
NAME                     SERVICE            AVAILABLE   AGE
v1alpha1.porch.kpt.dev   porch-system/api   True        18m
```

### Check the porch api-resources

```bash
kubectl api-resources | grep porch
```

Sample output:

```bash
packagerevs                                      config.porch.kpt.dev/v1alpha1     true         PackageRev
packagevariants                                  config.porch.kpt.dev/v1alpha1     true         PackageVariant
packagevariantsets                               config.porch.kpt.dev/v1alpha2     true         PackageVariantSet
repositories                                     config.porch.kpt.dev/v1alpha1     true         Repository
functions                                        porch.kpt.dev/v1alpha1            true         Function
packagerevisionresources                         porch.kpt.dev/v1alpha1            true         PackageRevisionResources
packagerevisions                                 porch.kpt.dev/v1alpha1            true         PackageRevision
packages                                         porch.kpt.dev/v1alpha1            true         PorchPackage
```

# Create Repositories using your local Porch server

To connect Porch to Gitea, follow [step 7 in the Starting with Porch](https://github.com/nephio-project/porch/tree/main/docs/tutorials/starting-with-porch#Connect-the-Gitea-repositories-to-Porch) tutorial to create the repositories in Porch.

You will notice logging messages in VSCode when you run the command `kubectl apply -f porch-repositories.yaml` command.

You can check that your locally running Porch server has created the repositories by running the `porchctl` command:

```bash
porchctl repo get -A
```

Sample output:

```bash
NAME                  TYPE   CONTENT   DEPLOYMENT   READY   ADDRESS
external-blueprints   git    Package   false        True    https://github.com/nephio-project/free5gc-packages.git
management            git    Package   false        True    http://172.18.255.200:3000/nephio/management.git
```

You can also check the repositories using kubectl.

```bash
kubectl get  repositories -n porch-demo
```

Sample output:

```bash
NAME                  TYPE   CONTENT   DEPLOYMENT   READY   ADDRESS
external-blueprints   git    Package   false        True    https://github.com/nephio-project/free5gc-packages.git
management            git    Package   false        True    http://172.18.255.200:3000/nephio/management.git
```

You now have a locally running Porch (api)server. Happy developing!

# Restart from scratch

Sometimes the development cluster gets cluttered and you may experience weird behaviour from porch.
In this case you might want to restart with a clean slate, by deleting the development cluster with the following command:

```bash
kind delete cluster --name porch-test
```

and running the [setup script](bin/setup.sh) again:

```bash
./scripts/setup-dev-env.sh
```

# Getting started with actual development

You can find a detailed description of the actual development process [here](dev-process.md).

