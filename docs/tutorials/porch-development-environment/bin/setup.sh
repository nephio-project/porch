#! /bin/bash

os_type=$(uname)
if [ "$os_type" = "Darwin" ]
then
  SED="gsed"
else
  SED="sed"
fi

# Create mgmt and edge1 clusters in kind
curl -s https://raw.githubusercontent.com/nephio-project/porch/main/docs/tutorials/starting-with-porch/kind_management_cluster.yaml | \
  kind create cluster --config=-

curl -s https://raw.githubusercontent.com/nephio-project/porch/main/docs/tutorials/starting-with-porch/kind_edge1_cluster.yaml | \
  kind create cluster --config=-

kind get kubeconfig --name=management > ~/.kube/kind-management-config
kind get kubeconfig --name=edge1 > ~/.kube/kind-edge1-config

export KUBECONFIG=~/.kube/kind-management-config

# Instal MetalLB
kubectl apply -f https://raw.githubusercontent.com/metallb/metallb/v0.13.12/config/manifests/metallb-native.yaml
kubectl wait --namespace metallb-system \
                --for=condition=ready pod \
                --selector=component=controller \
                --timeout=90s

kubectl apply -f https://raw.githubusercontent.com/nephio-project/porch/main/docs/tutorials/starting-with-porch/metallb-conf.yaml

TMP_DIR=$(mktemp -d)

pushd "$TMP_DIR" || exit

mkdir kpt_packages
pushd kpt_packages || exit

# Install Gitea
kpt pkg get https://github.com/nephio-project/catalog/tree/main/distros/sandbox/gitea
$SED -i 's/ metallb.universe.tf/ #metallb.universe.tf/' gitea/service-gitea.yaml
kpt fn render gitea
kpt live init gitea
kpt live apply gitea

popd || exit

# Create management and edge1 repos in gitea
curl -k -H "content-type: application/json" "http://nephio:secret@172.18.255.200:3000/api/v1/user/repos" --data '{"name":"management"}'
curl -k -H "content-type: application/json" "http://nephio:secret@172.18.255.200:3000/api/v1/user/repos" --data '{"name":"edge1"}'

mkdir repos
pushd repos || exit

# Initialize management and edge1 repos in Gitea
git clone http://172.18.255.200:3000/nephio/management
pushd management || exit

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
popd || exit

git clone http://172.18.255.200:3000/nephio/edge1
pushd edge1 || exit

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
popd || exit

popd || exit

rm -fr "$TMP_DIR"