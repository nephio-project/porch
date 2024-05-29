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

porch_cluster_name=${PORCH_TEST_CLUSTER:-porch-test}
git_repo_name="$porch_cluster_name"
gitea_ip=172.18.255.200  # should be from the address range specified here: docs/tutorials/starting-with-porch/metallb-conf.yaml
function_runner_ip=172.18.255.201

self_dir="$(dirname "$(readlink -f "$0")")"
git_root="$(readlink -f "${self_dir}/../../../..")"
cd "${git_root}"

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
  kind create cluster \
    --config="${git_root}/docs/tutorials/porch-development-environment/bin/kind_porch_test_cluster.yaml" \
    --name "$porch_cluster_name" || true

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
kubectl apply -f "${git_root}/docs/tutorials/starting-with-porch/metallb-conf.yaml"

############################################
h1 Install Gitea
mkdir -p "${git_root}/.build"
cd "${git_root}/.build"
if [ -d gitea ]; then
  kpt pkg update gitea
else
  kpt pkg get https://github.com/nephio-project/catalog/tree/main/distros/sandbox/gitea
fi

kpt fn eval gitea \
  --image gcr.io/kpt-fn/set-annotations:v0.1.4 \
  --match-kind Service \
  --match-name gitea \
  --match-namespace gitea \
  -- "metallb.universe.tf/loadBalancerIPs=${gitea_ip}"

cp -f "${git_root}/docs/tutorials/porch-development-environment/bin/kind_porch_test_cluster.yaml" gitea/cluster-config.yaml
# turn kind's cluster-config into a valid KRM
cat >> gitea/cluster-config.yaml <<EOF1
metadata: 
  name: not-used 
  annotations:
    config.kubernetes.io/local-config: "true"
EOF1
kpt fn eval gitea \
  --image gcr.io/kpt-fn/apply-replacements:v0.1.1 \
  --fn-config "${self_dir}/replace-gitea-service-ports.yaml"

kpt fn render gitea
kpt live init gitea || true
kpt live apply gitea --inventory-policy=adopt
echo "Waiting for gitea to become ready..."
kubectl wait --namespace gitea statefulset gitea \
                --for='jsonpath={.status.readyReplicas}=1' \
                --timeout=90s

############################################
h1 Create git repos in gitea
curl -k -H "content-type: application/json" "http://nephio:secret@${gitea_ip}:3000/api/v1/user/repos" --data "{\"name\":\"$git_repo_name\"}"
TMP_DIR=$(mktemp -d)
cd "$TMP_DIR"
git clone "http://nephio:secret@${gitea_ip}:3000/nephio/$git_repo_name"
cd "$git_repo_name"
if ! git rev-parse -q --verify refs/remotes/origin/main >/dev/null; then
  echo "Add main branch to git repo:"
  git switch -c  main
  touch README.md
  git add README.md
  git config user.name nephio
  git commit -m "first commit"
  git push -u origin main
else
  echo "main branch already exists in git repo."
fi
cd "${git_root}"
rm -fr "$TMP_DIR"

############################################
h1 Generate certs and keys
cd "${git_root}"
deployments/local/makekeys.sh

############################################
h1 Load container images into kind cluster
cd "${git_root}"
export IMAGE_TAG=v2.0.0
export KIND_CONTEXT_NAME="$porch_cluster_name"
if ! docker exec -it "$porch_cluster_name-control-plane" crictl images | grep -q docker.io/nephio/test-git-server ; then
  make build-images 
  kind load docker-image docker.io/nephio/porch-controllers:${IMAGE_TAG} -n ${KIND_CONTEXT_NAME}
  kind load docker-image docker.io/nephio/porch-function-runner:${IMAGE_TAG} -n ${KIND_CONTEXT_NAME}
  kind load docker-image docker.io/nephio/porch-wrapper-server:${IMAGE_TAG} -n ${KIND_CONTEXT_NAME}
  kind load docker-image docker.io/nephio/test-git-server:${IMAGE_TAG} -n ${KIND_CONTEXT_NAME}
else
  echo "Images already loaded into kind cluster."
fi

############################################
h1 Install all porch components, except porch-server
cd "${git_root}"
make deployment-config-no-sa
cd .build/deploy-no-sa
# expose function-runner to local processes
kpt fn eval \
  --image gcr.io/kpt-fn/starlark:v0.5.0 \
  --match-kind Service \
  --match-name function-runner \
  --match-namespace porch-system \
  -- "ip=${function_runner_ip}"  'source=
ip = ctx.resource_list["functionConfig"]["data"]["ip"]
for resource in ctx.resource_list["items"]:
  resource["metadata"].setdefault("annotations", {})["metallb.universe.tf/loadBalancerIPs"] = ip
  resource["spec"]["type"] = "LoadBalancer"
  resource["spec"]["ports"][0]["nodePort"] = 30001'
# "remove" porch-server from package
kpt fn eval \
  --image gcr.io/kpt-fn/starlark:v0.5.0 \
  --match-kind Deployment \
  --match-name porch-server \
  --match-namespace porch-system \
  -- 'source=ctx.resource_list["items"] = []'
# make the api service point to the local porch-server
if [ "$(uname)" = "Darwin" ]
then
  # MAC
  kpt fn eval \
    --image gcr.io/kpt-fn/starlark:v0.5.0 \
    --match-kind Service \
    --match-name api \
    --match-namespace porch-system \
    -- 'source=
for resource in ctx.resource_list["items"]:
  resource["spec"] = {
    "type": "ExternalName",
    "externalName": "host.docker.internal"
  }
'

else
  # Linux
  docker_bridge_ip="$(docker network inspect bridge --format='{{(index .IPAM.Config 0).Gateway}}')"
  kpt fn eval \
    --image upsert-resource:v0.2.0 \
    --fn-config "${git_root}/deployments/local/porch-api-endpoints.yaml"
  kpt fn eval \
    --image gcr.io/kpt-fn/search-replace:v0.2.0 \
    --match-kind Endpoints \
    --match-name api \
    --match-namespace porch-system \
    -- 'by-path=subsets[0].addresses[0].ip' "put-value=$docker_bridge_ip"
  kpt fn eval \
    --image gcr.io/kpt-fn/starlark:v0.5.0 \
    --match-kind Service \
    --match-name api \
    --match-namespace porch-system \
    -- 'source=
for resource in ctx.resource_list["items"]:
  resource["spec"].pop("selector")'
fi
kpt fn render 
kpt live init || true
kpt live apply --inventory-policy=adopt

############################################
h1 "Build the porch CLI (.build/porchctl)"
cd "${git_root}"
make porchctl


############################################
echo
echo Done.
