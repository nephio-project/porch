---
title: "Installing Porch"
type: docs
weight: 1
description: "A tutorial to install Porch"
---

<div style="border: 1px solid red; background-color: #ffe6e6; color: #b30000; padding: 1em; margin-bottom: 1em;">
  <strong>⚠️ Outdated Notice:</strong> This page refers to an older version of the documentation. This content has simply been moved into its relevant new section here and must be checked, modified, rewritten, updated, or removed entirely.
</div>

This tutorial is a guide to installing Porch. It is based on the
[Porch demo produced by Tal Liron of Google](https://github.com/tliron/klab/tree/main/environments/porch-demo). Users
should be comfortable using *git*, *docker*, and *kubernetes*.

See also [the Nephio Learning Resource](https://github.com/nephio-project/docs/blob/main/learning.md) page for
background help and information.

## Prerequisites

The tutorial can be executed on a Linux VM or directly on a laptop. It has been verified to execute on a MacBook Pro M1
machine and an Ubuntu 20.04 VM.

The following software should be installed prior to running through the tutorial:

1. [git](https://git-scm.com/)
2. [Docker](https://www.docker.com/get-started/)
3. [kubectl](https://kubernetes.io/docs/reference/kubectl/) - make sure that [kubectl context](https://kubernetes.io/docs/concepts/configuration/organize-cluster-access-kubeconfig/) configured with your cluster
4. [kind](https://kind.sigs.k8s.io/)
5. [kpt](https://github.com/kptdev/kpt)
6. [The go programming language](https://go.dev/)
7. [Visual Studio Code](https://code.visualstudio.com/download)
8. [VS Code extensions for go](https://code.visualstudio.com/docs/languages/go)

## Clone the repository and cd into the tutorial

```bash
git clone https://github.com/nephio-project/porch.git

cd porch/examples/tutorials/starting-with-porch/
```

## Create the Kind clusters for management and edge1

Create the clusters:

```bash
kind create cluster --config=kind_management_cluster.yaml
kind create cluster --config=kind_edge1_cluster.yaml
```

Output the *kubectl* configuration for the clusters:

```bash
kind get kubeconfig --name=management > ~/.kube/kind-management-config
kind get kubeconfig --name=edge1 > ~/.kube/kind-edge1-config
```

Toggling *kubectl* between the clusters:

```bash
export KUBECONFIG=~/.kube/kind-management-config

export KUBECONFIG=~/.kube/kind-edge1-config
```

## Install MetalLB on the management cluster

Install the MetalLB load balancer on the management cluster to expose services:

```bash
export KUBECONFIG=~/.kube/kind-management-config
kubectl apply -f https://raw.githubusercontent.com/metallb/metallb/v0.13.12/config/manifests/metallb-native.yaml
kubectl wait --namespace metallb-system \
                --for=condition=ready pod \
                --selector=component=controller \
                --timeout=90s
```

Check the subnet that is being used by the kind network in docker

```bash
docker network inspect kind | grep Subnet
```

Sample output:

```yaml
"Subnet": "172.18.0.0/16",
"Subnet": "fc00:f853:ccd:e793::/64"
```

Edit the *metallb-conf.yaml* file and ensure the spec.addresses range is in the IPv4 subnet being used by the kind network in docker.

```yaml
...
spec:
  addresses:
  - 172.18.255.200-172.18.255.250
...
```

Apply the MetalLB configuration:

```bash
kubectl apply -f metallb-conf.yaml
```

## Deploy and set up Gitea on the management cluster using kpt

Get the *gitea kpt* package:

```bash
export KUBECONFIG=~/.kube/kind-management-config

cd kpt_packages

kpt pkg get https://github.com/nephio-project/catalog/tree/main/distros/sandbox/gitea
```

Comment out the preconfigured IP address from the *gitea/service-gitea.yaml* file in the *gitea kpt* package:

```bash
11c11
<     metallb.universe.tf/loadBalancerIPs: 172.18.0.200
---
>     #    metallb.universe.tf/loadBalancerIPs: 172.18.0.200
```

Now render, init and apply the *gitea kpt* package:

```bash
kpt fn render gitea
kpt live init gitea # You only need to do this command once
kpt live apply gitea
```

Once the package is applied, all the Gitea pods should come up and you should be able to reach the Gitea UI on the
exposed IP Address/port of the Gitea service.

```bash
kubectl get svc -n gitea gitea

NAME    TYPE           CLUSTER-IP      EXTERNAL-IP      PORT(S)                       AGE
gitea   LoadBalancer   10.96.243.120   172.18.255.200   22:31305/TCP,3000:31102/TCP   10m
```

The UI is available at http://172.18.255.200:3000 in the example above.

To login to Gitea, use the credentials nephio:secret.

## Create repositories on Gitea for management and edge1

On the Gitea UI, click the **+** opposite **Repositories** and fill in the form for both the *management* and *edge1*
repositories. Use default values except for the following fields:

- Repository Name: "Management" or "edge1"
- Description: Something appropriate
 
Alternatively, we can create the repositories via curl:

```bash
curl -k -H "content-type: application/json" "http://nephio:secret@172.18.255.200:3000/api/v1/user/repos" --data '{"name":"management"}'

curl -k -H "content-type: application/json" "http://nephio:secret@172.18.255.200:3000/api/v1/user/repos" --data '{"name":"edge1"}'
```

Check the repositories:

```bash
 curl -k -H "content-type: application/json" "http://nephio:secret@172.18.255.200:3000/api/v1/user/repos" | grep -Po '"name": *\K"[^"]*"'
```

Now initialize both repositories with an initial commit.

Initialize the *management* repository:

```bash
cd ../repos
git clone http://172.18.255.200:3000/nephio/management
cd management

touch README.md
git init
git checkout -b main
git config user.name nephio
git add README.md

git commit -m "first commit"
git remote remove origin
git remote add origin http://nephio:secret@172.18.255.200:3000/nephio/management.git
git remote -v
git push -u origin main
cd ..
 ```

Initialize the *edge1* repository:

```bash
git clone http://172.18.255.200:3000/nephio/edge1
cd edge1

touch README.md
git init
git checkout -b main
git config user.name nephio
git add README.md

git commit -m "first commit"
git remote remove origin
git remote add origin http://nephio:secret@172.18.255.200:3000/nephio/edge1.git
git remote -v
git push -u origin main
cd ../../
```

## Install Porch

We will use the *Porch Kpt* package from Nephio catalog repository.

```bash
cd kpt_packages

kpt pkg get https://github.com/nephio-project/catalog/tree/main/nephio/core/porch
```

Now we can install porch. We render the *kpt* package and then init and apply it.

```bash
kpt fn render porch
kpt live init porch # You only need to do this command once
kpt live apply porch
```

Check that the Porch PODs are running on the management cluster:

```bash
kubectl get pod -n porch-system
NAME                                 READY   STATUS    RESTARTS   AGE
function-runner-7994f65554-nrzdh     1/1     Running   0          81s
function-runner-7994f65554-txh9l     1/1     Running   0          81s
porch-controllers-7fb4497b77-2r2r6   1/1     Running   0          81s
porch-server-68bfdddbbf-pfqsm        1/1     Running   0          81s
```

Check that the Porch CRDs and other resources have been created:

```bash
kubectl api-resources | grep porch   
packagerevs                                    config.porch.kpt.dev/v1alpha1          true         PackageRev
packagevariants                                config.porch.kpt.dev/v1alpha1          true         PackageVariant
packagevariantsets                             config.porch.kpt.dev/v1alpha2          true         PackageVariantSet
repositories                                   config.porch.kpt.dev/v1alpha1          true         Repository
packagerevisionresources                       porch.kpt.dev/v1alpha1                 true         PackageRevisionResources
packagerevisions                               porch.kpt.dev/v1alpha1                 true         PackageRevision
packages                                       porch.kpt.dev/v1alpha1                 true         Package
```

## Connect the Gitea repositories to Porch

Create a demo namespace:

```bash
kubectl create namespace porch-demo
```

Create a secret for the Gitea credentials in the demo namespace:

```bash
kubectl create secret generic gitea \
    --namespace=porch-demo \
    --type=kubernetes.io/basic-auth \
    --from-literal=username=nephio \
    --from-literal=password=secret
```

Now, define the Gitea repositories in Porch:

```bash
kubectl apply -f porch-repositories.yaml
```

Check that the repositories have been correctly created:

```bash
kubectl get repositories -n porch-demo
NAME                  TYPE   CONTENT   DEPLOYMENT   READY   ADDRESS
edge1                 git    Package   true         True    http://172.18.255.200:3000/nephio/edge1.git
external-blueprints   git    Package   false        True    https://github.com/nephio-project/free5gc-packages.git
management            git    Package   false        True    http://172.18.255.200:3000/nephio/management.git
```

## Configure configsync on the workload cluster

configsync is installed on the edge1 cluster so that it syncs the contents of the *edge1* repository onto the edge1
workload cluster. We will use the configsync package from Nephio.

```bash
export KUBECONFIG=~/.kube/kind-edge1-config

cd kpt_packages

kpt pkg get https://github.com/nephio-project/catalog/tree/main/nephio/core/configsync
kpt fn render configsync
kpt live init configsync
kpt live apply configsync
```

Check that the configsync PODs are up and running:

```bash
kubectl get pod -n config-management-system
NAME                                          READY   STATUS    RESTARTS   AGE
config-management-operator-6946b77565-f45pc   1/1     Running   0          118m
reconciler-manager-5b5d8557-gnhb2             2/2     Running   0          118m
```

Now, we need to set up a RootSync CR to synchronize the *edge1* repository:

```bash
kpt pkg get https://github.com/nephio-project/catalog/tree/main/nephio/optional/rootsync
```

Edit the *rootsync/package-context.yaml* file to set the name of the cluster/repo we are syncing from/to:

```bash
9c9
<   name: example-rootsync
---
>   name: edge1
```

Render the package. This configures the *rootsync/rootsync.yaml* file in the Kpt package:

```bash
kpt fn render rootsync
```

Edit the *rootsync/rootsync.yaml* file to set the IP address of Gitea and to turn off authentication for accessing
Gitea:

```bash
11c11
<     repo: http://172.18.0.200:3000/nephio/example-cluster-name.git
---
>     repo: http://172.18.255.200:3000/nephio/edge1.git
13,15c13,16
<     auth: token
<     secretRef:
<       name: example-cluster-name-access-token-configsync
---
>     auth: none
> #    auth: token
> #    secretRef:
> #      name: edge1-access-token-configsync
```

Initialize and apply RootSync:

```bash
export KUBECONFIG=~/.kube/kind-edge1-config

kpt live init rootsync # This command is only needed once
kpt live apply rootsync
```

Check that the RootSync CR is created:

```bash
kubectl get rootsync -n config-management-system
NAME    RENDERINGCOMMIT                            RENDERINGERRORCOUNT   SOURCECOMMIT                               SOURCEERRORCOUNT   SYNCCOMMIT                                 SYNCERRORCOUNT
edge1   613eb1ad5632d95c4336894f8a128cc871fb3266                         613eb1ad5632d95c4336894f8a128cc871fb3266                      613eb1ad5632d95c4336894f8a128cc871fb3266   
```

Check that configsync is synchronized with the repository on the management cluster:

```bash
kubectl get pod -n config-management-system -l app=reconciler
NAME                                     READY   STATUS    RESTARTS   AGE
root-reconciler-edge1-68576f878c-92k54   4/4     Running   0          2d17h

kubectl logs -n config-management-system root-reconciler-edge1-68576f878c-92k54 -c git-sync -f

```

The result should be similar to:

```bash
INFO: detected pid 1, running init handler
I0105 17:50:11.472934      15 cmd.go:48] "level"=5 "msg"="running command" "cwd"="" "cmd"="git config --global gc.autoDetach false"
I0105 17:50:11.493046      15 cmd.go:48] "level"=5 "msg"="running command" "cwd"="" "cmd"="git config --global gc.pruneExpire now"
I0105 17:50:11.513487      15 main.go:473] "level"=0 "msg"="starting up" "pid"=15 "args"=["/git-sync","--root=/repo/source","--dest=rev","--max-sync-failures=30","--error-file=error.json","--v=5"]
I0105 17:50:11.514044      15 main.go:923] "level"=0 "msg"="cloning repo" "origin"="http://172.18.255.200:3000/nephio/edge1.git" "path"="/repo/source"
I0105 17:50:11.514061      15 cmd.go:48] "level"=5 "msg"="running command" "cwd"="" "cmd"="git clone -v --no-checkout -b main --depth 1 http://172.18.255.200:3000/nephio/edge1.git /repo/source"
I0105 17:50:11.706506      15 cmd.go:48] "level"=5 "msg"="running command" "cwd"="/repo/source" "cmd"="git rev-parse HEAD"
I0105 17:50:11.729292      15 main.go:737] "level"=0 "msg"="syncing git" "rev"="HEAD" "hash"="385295a2143f10a6cda0cf4609c45d7499185e01"
I0105 17:50:11.729332      15 cmd.go:48] "level"=5 "msg"="running command" "cwd"="/repo/source" "cmd"="git fetch -f --tags --depth 1 http://172.18.255.200:3000/nephio/edge1.git main"
I0105 17:50:11.920110      15 cmd.go:48] "level"=5 "msg"="running command" "cwd"="/repo/source" "cmd"="git cat-file -t 385295a2143f10a6cda0cf4609c45d7499185e01"
I0105 17:50:11.945545      15 cmd.go:48] "level"=5 "msg"="running command" "cwd"="/repo/source" "cmd"="git rev-parse 385295a2143f10a6cda0cf4609c45d7499185e01"
I0105 17:50:11.967150      15 main.go:726] "level"=1 "msg"="removing worktree" "path"="/repo/source/385295a2143f10a6cda0cf4609c45d7499185e01"
I0105 17:50:11.967359      15 cmd.go:48] "level"=5 "msg"="running command" "cwd"="/repo/source" "cmd"="git worktree prune"
I0105 17:50:11.987522      15 cmd.go:48] "level"=5 "msg"="running command" "cwd"="/repo/source" "cmd"="git worktree add --detach /repo/source/385295a2143f10a6cda0cf4609c45d7499185e01 385295a2143f10a6cda0cf4609c45d7499185e01 --no-checkout"
I0105 17:50:12.057698      15 main.go:772] "level"=0 "msg"="adding worktree" "path"="/repo/source/385295a2143f10a6cda0cf4609c45d7499185e01" "branch"="origin/main"
I0105 17:50:12.057988      15 cmd.go:48] "level"=5 "msg"="running command" "cwd"="/repo/source/385295a2143f10a6cda0cf4609c45d7499185e01" "cmd"="git reset --hard 385295a2143f10a6cda0cf4609c45d7499185e01"
I0105 17:50:12.099783      15 main.go:833] "level"=0 "msg"="reset worktree to hash" "path"="/repo/source/385295a2143f10a6cda0cf4609c45d7499185e01" "hash"="385295a2143f10a6cda0cf4609c45d7499185e01"
I0105 17:50:12.099805      15 main.go:838] "level"=0 "msg"="updating submodules"
I0105 17:50:12.099976      15 cmd.go:48] "level"=5 "msg"="running command" "cwd"="/repo/source/385295a2143f10a6cda0cf4609c45d7499185e01" "cmd"="git submodule update --init --recursive --depth 1"
I0105 17:50:12.442466      15 main.go:694] "level"=1 "msg"="creating tmp symlink" "root"="/repo/source/" "dst"="385295a2143f10a6cda0cf4609c45d7499185e01" "src"="tmp-link"
I0105 17:50:12.442494      15 cmd.go:48] "level"=5 "msg"="running command" "cwd"="/repo/source/" "cmd"="ln -snf 385295a2143f10a6cda0cf4609c45d7499185e01 tmp-link"
I0105 17:50:12.453694      15 main.go:699] "level"=1 "msg"="renaming symlink" "root"="/repo/source/" "old_name"="tmp-link" "new_name"="rev"
I0105 17:50:12.453718      15 cmd.go:48] "level"=5 "msg"="running command" "cwd"="/repo/source/" "cmd"="mv -T tmp-link rev"
I0105 17:50:12.467904      15 cmd.go:48] "level"=5 "msg"="running command" "cwd"="/repo/source" "cmd"="git gc --auto"
I0105 17:50:12.492329      15 cmd.go:48] "level"=5 "msg"="running command" "cwd"="/repo/source" "cmd"="git cat-file -t HEAD"
I0105 17:50:12.518878      15 cmd.go:48] "level"=5 "msg"="running command" "cwd"="/repo/source" "cmd"="git rev-parse HEAD"
I0105 17:50:12.540979      15 main.go:585] "level"=1 "msg"="next sync" "wait_time"=15000000000
I0105 17:50:27.553609      15 cmd.go:48] "level"=5 "msg"="running command" "cwd"="/repo/source/rev" "cmd"="git rev-parse HEAD"
I0105 17:50:27.600401      15 cmd.go:48] "level"=5 "msg"="running command" "cwd"="/repo/source/rev" "cmd"="git ls-remote -q http://172.18.255.200:3000/nephio/edge1.git refs/heads/main"
I0105 17:50:27.694035      15 main.go:1065] "level"=1 "msg"="no update required" "rev"="HEAD" "local"="385295a2143f10a6cda0cf4609c45d7499185e01" "remote"="385295a2143f10a6cda0cf4609c45d7499185e01"
I0105 17:50:27.694159      15 main.go:585] "level"=1 "msg"="next sync" "wait_time"=15000000000
I0105 17:50:42.695482      15 cmd.go:48] "level"=5 "msg"="running command" "cwd"="/repo/source/rev" "cmd"="git rev-parse HEAD"
I0105 17:50:42.733276      15 cmd.go:48] "level"=5 "msg"="running command" "cwd"="/repo/source/rev" "cmd"="git ls-remote -q http://172.18.255.200:3000/nephio/edge1.git refs/heads/main"
I0105 17:50:42.826422      15 main.go:1065] "level"=1 "msg"="no update required" "rev"="HEAD" "local"="385295a2143f10a6cda0cf4609c45d7499185e01" "remote"="385295a2143f10a6cda0cf4609c45d7499185e01"
I0105 17:50:42.826611      15 main.go:585] "level"=1 "msg"="next sync" "wait_time"=15000000000

.......

I0108 11:04:05.935586      15 cmd.go:48] "level"=5 "msg"="running command" "cwd"="/repo/source/rev" "cmd"="git rev-parse HEAD"
I0108 11:04:05.981750      15 cmd.go:48] "level"=5 "msg"="running command" "cwd"="/repo/source/rev" "cmd"="git ls-remote -q http://172.18.255.200:3000/nephio/edge1.git refs/heads/main"
I0108 11:04:06.079536      15 main.go:1065] "level"=1 "msg"="no update required" "rev"="HEAD" "local"="385295a2143f10a6cda0cf4609c45d7499185e01" "remote"="385295a2143f10a6cda0cf4609c45d7499185e01"
I0108 11:04:06.079599      15 main.go:585] "level"=1 "msg"="next sync" "wait_time"=15000000000
```
