# Build and deploy custom porch images on a kind cluster

This section covers the process to build and deploy the porch images to a given kind cluster.

## Prerequisites
The following software should be installed prior to running:
1. [git](https://git-scm.com/)
2. [Docker](https://www.docker.com/get-started/)
Docker image load bug on v25.0.0 Use v24.0.7
3. [kubectl](https://kubernetes.io/docs/tasks/tools/#kubectl)
4. [The go programming language](https://go.dev/doc/install)
5. [kind](https://kind.sigs.k8s.io/docs/user/quick-start#installation)
6. [kpt-cli](https://kpt.dev/installation/kpt-cli)
7. [yq](https://github.com/mikefarah/yq/#install)

## Create a default kind cluster

If one is not already available, create a kind cluster.

```
kind create cluster -n dev
```

Optionally, save the config for the new cluster and set it as the default config in the current shell:

```
kind get kubeconfig --name=dev > ~/.kube/kind-dev-config
export KUBECONFIG=~/.kube/kind-dev-config
```

## Clone Porch and deploy with custom images
Clone the [porch project](https://github.com/nephio-project/porch.git)  from Github onto your environment using whatever cloning process for development your organization recommends. 
Here, we assume Porch is cloned into a directory called `porch`.

We will now use the make target `run-in-kind-kpt` to build the images and re deploy the porch deployments to use them.

Here, we pass the following vars to the make target:

| VAR  | DEFAULT  | EG  |
|---|---|---|
|  IMAGE_TAG | $(git_tag)-dirty  | test  |
|  KUBECONFIG | $(CURDIR)/deployments/local/kubeconfig  | ~/.kube/config  |
|  KIND_CONTEXT_NAME | kind  | dev  |
 
```
cd porch

make run-in-kind-kpt IMAGE_TAG='test' KUBECONFIG='/home/ubuntu/.kube/config' KIND_CONTEXT_NAME='dev'
```
This will build the porch images locally with the given tag, load them in to the kind docker ctr, update the [porch kpt pkg](https://github.com/nephio-project/catalog/tree/main/nephio/core/porch) to use those images and deploy the pkg to the cluster.

> **_NOTE:_** The docker build can take some time to complete on first run. Future refinements to the make targets will reduce build and or deploy times. To skip the building of the porch images, we can pass an optional flag SKIP_IMG_BUILD=true to the above make target.

Check that the new images have been deployed:

```
kubectl get deployments -n porch-system -o wide
```

Sample output:
```
NAME                READY   UP-TO-DATE   AVAILABLE   AGE   CONTAINERS          IMAGES                                   SELECTOR
function-runner     2/2     2            2           81m   function-runner     porch-kind/porch-function-runner:test   app=function-runner
porch-controllers   1/1     1            1           81m   porch-controllers   porch-kind/porch-controllers:test       k8s-app=porch-controllers
porch-server        1/1     1            1           81m   porch-server        porch-kind/porch-server:test            app=porch-server
```



## Deploy a minimal, in memory gitea

To facilitate testing towards a git repo, we can deploy a minimal, in memory [gitea](https://docs.gitea.com/) installation.

Here, we pass the following vars to the make target:

| VAR  | DEFAULT  | EG  |
|---|---|---|
|  KUBECONFIG | $(CURDIR)/deployments/local/kubeconfig  | ~/.kube/config  |

```
make deploy-gitea-dev-pkg KUBECONFIG='~/.kube/config'
```

Check that gitea have been deployed:

```
kubectl get all -n gitea
```

Sample output:
```
NAME                         READY   STATUS    RESTARTS   AGE
pod/gitea-5746c4b4fc-sqgs6   1/1     Running   0          3m4s

NAME                 TYPE        CLUSTER-IP      EXTERNAL-IP   PORT(S)    AGE
service/gitea-http   ClusterIP   10.96.104.116   <none>        3000/TCP   3m4s
service/gitea-ssh    ClusterIP   10.96.168.195   <none>        22/TCP     3m4s

NAME                    READY   UP-TO-DATE   AVAILABLE   AGE
deployment.apps/gitea   1/1     1            1           3m4s

NAME                               DESIRED   CURRENT   READY   AGE
replicaset.apps/gitea-5746c4b4fc   1         1         1       3m4s

```

Now, we can expose the svc by using port forwarding.
In another terminal, run the following:

```
kubectl --namespace gitea port-forward svc/gitea-http 3000:3000
```

The gitea UI can now be reached in a local web browser over:

```
http://localhost:3000/
```

Admin login:

```
user : nephio
password : secret
``````

## Setup a test repo on gitea

To create a new test repo, we can use curl towards the forwarded backend svc port 3000:

```
curl -k -H "content-type: application/json" "http://nephio:secret@localhost:3000/api/v1/user/repos" --data '{"name":"testrepo"}'
```

To initialize the test repo with a "main" branch, we can clone it locally and push a default setup to it.

```
git clone http://localhost:3000/nephio/testrepo.git ~/Downloads/testrepo

cd ~/Downloads/testrepo


touch README.md
git init
git checkout -b main
git add README.md
git commit -m "first commit"
git remote add origin http://localhost:3000/nephio/testrepo.git
git push -u origin main

user : nephio
pass : secret

```

## Connect porch to the newly create repo

We can now connect porch to the test repo using the Porch Repository CR (Custom Resource).

Create a test namespace:

```
kubectl create namespace porch-test
```

Create a secret for the Gitea credentials in the porch-test namespace:

```
kubectl create secret generic gitea-porch-secret \
    --namespace=porch-test \
    --type=kubernetes.io/basic-auth \
    --from-literal=username=nephio \
    --from-literal=password=secret
```

Now, we define the test Repository CR in Porch:

```
cat <<EOF | kubectl create -f -
apiVersion: config.porch.kpt.dev/v1alpha1
kind: Repository

metadata:
  name: testrepo
  namespace: porch-test

spec:
  description: testrepo
  content: Package
  deployment: false
  type: git
  git:
    repo: http://gitea-http.gitea.svc.cluster.local:3000/nephio/testrepo.git
    directory: /
    branch: main
    secretRef:
      name: gitea-porch-secret
EOF
```

Check that the Repository has been correctly created:

```
kubectl get repositories -n porch-test
```

Sample output:
```
NAME       TYPE   CONTENT   DEPLOYMENT   READY   ADDRESS
testrepo   git    Package   false        True    http://gitea-http.gitea.svc.cluster.local:3000/nephio/testrepo.git
```
Check the logs of the porch-server

```
kubectl logs deployment/porch-server -n porch-system
```

Sample output:

```
...

I0124 11:23:10.400567       1 background.go:134] Repository added: porch-test:testrepo
I0124 11:23:10.533213       1 repository.go:364] repo git://http://gitea-http.gitea.svc.cluster.local:3000/nephio/testrepo.git/: poll started
I0124 11:23:10.541438       1 background.go:137] Repository modified: porch-test:testrepo
I0124 11:23:10.681949       1 git.go:1702] discovered 0 packages @ecf8bed2663745165615f9390e5e3df7c0196d93
I0124 11:23:10.681964       1 repository.go:528] repo git://http://gitea-http.gitea.svc.cluster.local:3000/nephio/testrepo.git/: addSent 0, modSent 0, delSent for 0 old and 0 new repo packages
I0124 11:23:10.681968       1 repository.go:397] repo git://http://gitea-http.gitea.svc.cluster.local:3000/nephio/testrepo.git/: refresh finished in 0.099783 secs
I0124 11:23:10.681975       1 repository.go:365] repo git://http://gitea-http.gitea.svc.cluster.local:3000/nephio/testrepo.git/: poll finished in 0.148768 secs
I0124 11:23:10.681979       1 repository.go:364] repo git://http://gitea-http.gitea.svc.cluster.local:3000/nephio/testrepo.git/: poll started
I0124 11:23:10.779443       1 repository.go:397] repo git://http://gitea-http.gitea.svc.cluster.local:3000/nephio/testrepo.git/: refresh finished in 0.047841 secs
I0124 11:23:10.779453       1 repository.go:365] repo git://http://gitea-http.gitea.svc.cluster.local:3000/nephio/testrepo.git/: poll finished in 0.097474 secs
...
```


## Teardown the custom deployments

Destroy the gitea kpt pkg resources:

```
kpt live destroy .build/deploy/kpt_pkgs/gitea-dev
```

Alternatively, we can use kubectl:

```
kubectl delete --wait --recursive --filename ./.build/deploy/kpt_pkgs/gitea-dev/
```

Destroy the porch kpt pkg resources:

```
kpt live destroy .build/deploy/kpt_pkgs/porch
```

Again, alternatively, we can use kubectl:

```
kubectl delete --wait --recursive --filename ./.build/deploy/porch/
```

Finally, we can delete the kind cluster if necessary:
```
kind delete cluster -n dev
```