#! /bin/bash -e

# Copyright 2024 The kpt and Nephio Authors
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

porch_cluster_name=porch-test
git_repo_name="$porch_cluster_name"
gitea_ip=172.18.255.200  # should be from the address range specified here: https://github.com/nephio-project/porch/blob/main/docs/tutorials/starting-with-porch/metallb-conf.yaml
self_dir="$(dirname "$(readlink -f "$0")")"

function h1() {
  echo
  echo "** $*"
  echo 
}

# check if git user.name is set in your environment
if ! git config --global user.name > /dev/null ; then
  cat << ENDOFEMSG
Please set the user.name and user.email global configuration values in your git environment.
This is needed for porch tests to run correctly. 
E.g:
  git config --global user.name "My Name"
  git config --global user.email "myemail@example.com"
ENDOFEMSG
  exit 128
fi

##############################################
h1 "Install kind cluster: $porch_cluster_name"
if ! kind get clusters | grep -q "^$porch_cluster_name\$" ; then
  curl -s https://raw.githubusercontent.com/nephio-project/porch/main/docs/tutorials/starting-with-porch/kind_management_cluster.yaml | \
    kind create cluster --config=- --name "$porch_cluster_name" || true

  mkdir -p ~/.kube
  kind get kubeconfig --name="$porch_cluster_name" > ~/.kube/"kind-$porch_cluster_name"
else
  echo "Cluster already exists."
fi
kind export kubeconfig --name="$porch_cluster_name"

##############################################
h1 Instal MetalLB
kubectl apply -f https://raw.githubusercontent.com/metallb/metallb/v0.13.12/config/manifests/metallb-native.yaml
sleep 1
echo "Waiting for controller to become ready..."
kubectl wait --namespace metallb-system deploy controller \
                --for=condition=available \
                --timeout=90s
kubectl apply -f https://raw.githubusercontent.com/nephio-project/porch/main/docs/tutorials/starting-with-porch/metallb-conf.yaml

############################################
h1 Prepare tmp dir
TMP_DIR=$(mktemp -d)
echo "$TMP_DIR"

############################################
h1 Install Gitea
mkdir "$TMP_DIR/kpt_packages"
cd "$TMP_DIR/kpt_packages"
kpt pkg get https://github.com/nephio-project/catalog/tree/main/distros/sandbox/gitea
kpt fn eval gitea \
  --image gcr.io/kpt-fn/set-annotations:v0.1.4 \
  --match-kind Service \
  --match-name gitea \
  --match-namespace gitea \
  -- "metallb.universe.tf/loadBalancerIPs=${gitea_ip}"
curl -o gitea/cluster-config.yaml https://raw.githubusercontent.com/nephio-project/porch/main/docs/tutorials/starting-with-porch/kind_management_cluster.yaml
echo "metadata: { name: "porch-test" }" >> gitea/cluster-config.yaml
kpt fn eval gitea \
  --image gcr.io/kpt-fn/set-annotations:v0.1.4 \
  --match-kind Cluster \
  --match-api-version kind.x-k8s.io/v1alpha4 \
  -- "config.kubernetes.io/local-config=true"
kpt fn eval gitea \
  --image gcr.io/kpt-fn/apply-replacements:v0.1.1 \
  --fn-config "${self_dir}/replace-gitea-service-ports.yaml"

kpt fn render gitea
kpt live init gitea
kpt live apply gitea

############################################
h1 Create git repos in gitea
curl -k -H "content-type: application/json" "http://nephio:secret@${gitea_ip}:3000/api/v1/user/repos" --data "{\"name\":\"$git_repo_name\"}"

mkdir "$TMP_DIR/repos"
cd "$TMP_DIR/repos"
git clone "http://nephio:secret@${gitea_ip}:3000/nephio/$git_repo_name"
cd "$git_repo_name"

if ! git rev-parse -q --verify refs/remotes/origin/main >/dev/null; then
  git switch -c  main
  touch README.md
  git add README.md
  git config user.name nephio
  git commit -m "first commit"
  git push -u origin main
else
  echo "main branch already exists in git repo."
fi

############################################
h1 "Clean up"
cd "$self_dir"
rm -fr "$TMP_DIR"

echo
echo Done.
