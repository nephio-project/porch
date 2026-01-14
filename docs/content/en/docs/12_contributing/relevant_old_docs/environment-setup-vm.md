---
title: "Setting up a VM environment"
type: docs
weight: 2
description:
---

<div style="border: 1px solid red; background-color: #ffe6e6; color: #b30000; padding: 1em; margin-bottom: 1em;">
  <strong>⚠️ Outdated Notice:</strong> This page refers to an older version of the documentation. This content has simply been moved into its relevant new section here and must be checked, modified, rewritten, updated, or removed entirely.
</div>

This tutorial gives short instructions on how to set up a development environment for Porch on a Nephio VM. It outlines the steps to
get a [kind](https://kind.sigs.k8s.io/) cluster up and running to which a Porch instance running in Visual Studio Code
can connect to and interact with. If you are not familiar with how porch works, it is highly recommended that you go
through the [Starting with Porch tutorial]({{% relref "/docs/6_configuration_and_deployments/relevant_old_docs/install-porch.md" %}}) before going through this one.

## Setting up the environment

1. The first step is to install the Nephio sandbox environment on your VM. In short, log onto your VM and give the command
below:

```bash
wget -O - https://raw.githubusercontent.com/nephio-project/test-infra/main/e2e/provision/init.sh |  \
sudo NEPHIO_DEBUG=false   \
     NEPHIO_BRANCH=main \
     NEPHIO_USER=ubuntu   \
     bash
```

2. Set up your VM for development (optional but recommended step).

```bash
echo ''                                         >> ~/.bashrc
echo 'source <(kubectl completion bash)'        >> ~/.bashrc
echo 'source <(kpt completion bash)'            >> ~/.bashrc
echo 'source <(porchctl completion bash)'       >> ~/.bashrc
echo ''                                         >> ~/.bashrc
echo 'alias h=history'                          >> ~/.bashrc
echo 'alias k=kubectl'                          >> ~/.bashrc
echo ''                                         >> ~/.bashrc
echo 'complete -o default -F __start_kubectl k' >> ~/.bashrc

sudo usermod -a -G syslog ubuntu
sudo usermod -a -G docker ubuntu
```

3. Log out of your VM and log in again so that the group changes on the *ubuntu* user are picked up.

```bash
> exit

> ssh ubuntu@thevmhostname
> groups
ubuntu adm dialout cdrom floppy sudo audio dip video plugdev syslog netdev lxd docker
```

4. Install *go* so that you can build Porch on the VM:

```bash
wget -O - https://go.dev/dl/go1.22.5.linux-amd64.tar.gz | sudo tar -C /usr/local -zxvf -

echo ''                                   >> ~/.profile
echo '# set PATH for go'                  >> ~/.profile
echo 'if [ -d "/usr/local/go" ]'          >> ~/.profile
echo 'then'                               >> ~/.profile
echo '    PATH="/usr/local/go/bin:$PATH"' >> ~/.profile
echo 'fi'                                 >> ~/.profile 
```

5. Log out of your VM and log in again so that the *go* is added to your path. Verify that *go* is in the path:

```bash
> exit

> ssh ubuntu@thevmhostname

> go version
go version go1.22.5 linux/amd64
```

6. Install *go delve* for debugging on the VM:

```bash
go install -v github.com/go-delve/delve/cmd/dlv@latest
```

7. Clone Porch onto the VM

```bash
mkdir -p git/github/nephio-project
cd ~/git/github/nephio-project

# Clone porch
git clone https://github.com/nephio-project/porch.git
cd porch
```

8. Change the Kind cluster name in the Porch Makefile to match the Kind cluster name on the VM:

```bash
sed -i "s/^KIND_CONTEXT_NAME ?= porch-test$/KIND_CONTEXT_NAME ?= "$(kind get clusters)"/" Makefile
```

9. Expose the Porch function runner so that the Nephio server running in VS Code can access it

```bash
kubectl expose svc -n porch-system function-runner --name=xfunction-runner --type=LoadBalancer --load-balancer-ip='172.18.0.202'
```

10. Set the *KUBECONFIG* and *FUNCTION_RUNNER_IP* environment variables in the *.profile* file
    You **must** do this step before connecting with VS Code because VS Code caches the environment on the server. If you
    want to change the values of these variables subsequently, you must restart the VM server.

     ```bash
     echo ''                                              >> ~/.profile
     echo 'export KUBECONFIG="/home/ubuntu/.kube/config"' >> ~/.profile
     echo 'export FUNCTION_RUNNER_IP="172.18.0.202"'      >> ~/.profile
     ```

You have now set up the VM so that it can be used for remove debugging of Porch.

## Setting up VS Code

Use the [VS Code Remote SSH]
(https://marketplace.visualstudio.com/items?itemName=ms-vscode-remote.remote-ssh)
plugin to debug from VS Code running on your local machine towards a VM. Detailed documentation
on the plugin and its use is available on the
[Remote Development using SSH](https://code.visualstudio.com/docs/remote/ssh) in the VS Code
documentation.

1. Use the **Connect to a remote host** instructions on the
[Remote Development using SSH](https://code.visualstudio.com/docs/remote/ssh) page to connect to your VM.

2. Click **Open Folder** and browse to the Porch code on the VM, */home/ubuntu/git/github/nephio-project/porch* in this
   case:

![Browse to Porch code](/static/images/porch/contributor/01_VSCodeOpenPorchFolder.png)

3. VS Code now opens the Porch project on the VM.

![Porch code is open](/static/images/porch/contributor/02_VSCodeConnectedPorch.png)

4. We now need to install support for *go* debugging in VS Code. Trigger this by launching a debug configuration in
   VS Code.
   Here we use the **Launch Override Server** configuration.

![Launch the Override Server VS Code debug configuration](/static/images/porch/contributor/03_LaunchOverrideServer.png)

5. VS Code complains that *go* debugging is not supported, click the **Install go Extension** button. 

![VS Code go debugging not supported message](/static/images/porch/contributor/04_GoDebugNotSupportedPopup.png)

6. Go automatically presents the Go debug plugin for installation. Click the **Install** button.

![VS Code Go debugging plugin selected](/static/images/porch/contributor/05_GoExtensionAutoSelected.png)

7. VS Code installs the plugin.

![VS Code Go debugging plugin installed](/static/images/porch/contributor/06_GoExtensionInstalled.png)

You have now set up VS Code so that it can be used for remove debugging of Porch.

## Getting started with actual development

You can find a detailed description of the actual development process [here]({{% relref "/docs/12_contributing/relevant_old_docs/dev-process.md" %}}).
