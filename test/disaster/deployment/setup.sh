#!/usr/bin/env bash
# Copyright 2026 The kpt and Nephio Authors
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

# Stricter error handling
set -e # Exit on error
set -u # Must predefine variables
set -o pipefail # Check errors in piped commands

PORCH_DIR="$(git rev-parse --show-toplevel)"
self_dir="$(dirname "$(readlink -f "$0")")"

source "$self_dir/../util/functions.sh"

CATALOG_REPO='https://github.com/nephio-project/catalog.git'
KPT_REPO='https://github.com/kptdev/kpt.git'
cd "$PORCH_DIR"

cleandown_before=${CLEANDOWN_BEFORE:-true}

data_kind_cluster="porch-disaster-test-data"
crcache_kind_cluster="porch-disaster-test-crcache"
dbcache_kind_cluster="porch-disaster-test-dbcache"

data_cluster_kubeconfig_file="$self_dir/kubeconfigs/data_cluster.conf"
crcache_kubeconfig_file="$self_dir/kubeconfigs/porch_crcache.conf"
dbcache_kubeconfig_file="$self_dir/kubeconfigs/porch_dbcache.conf"

function kubectl_data() {
    kubectl --kubeconfig "$data_cluster_kubeconfig_file" "$@"
}
function kubectl_crcache() {
    kubectl --kubeconfig "$crcache_kubeconfig_file" "$@"
}
function kubectl_dbcache() {
    kubectl --kubeconfig "$dbcache_kubeconfig_file" "$@"
}
function porchctl_crcache() {
    porchctl --kubeconfig "$crcache_kubeconfig_file" "$@"
}
function porchctl_dbcache() {
    porchctl --kubeconfig "$dbcache_kubeconfig_file" "$@"
}

h1 "Cleandown before install"

if [[ $cleandown_before = "true" ]]; then
    rm -rf "$self_dir/kubeconfigs"
    mkdir "$self_dir/kubeconfigs"
    kind delete cluster --name "$data_kind_cluster" || true
    kind delete cluster --name "$crcache_kind_cluster" || true
    kind delete cluster --name "$dbcache_kind_cluster" || true
fi

h1 "Create data cluster"
kind create cluster --name "$data_kind_cluster" --kubeconfig "$data_cluster_kubeconfig_file" || kind export kubeconfig --name "$data_kind_cluster" --kubeconfig "$data_cluster_kubeconfig_file"
# h1 "Create CR cache cluster"
# kind create cluster --name "$crcache_kind_cluster" --kubeconfig "$crcache_kubeconfig_file" || kind export kubeconfig --name "$crcache_kind_cluster" --kubeconfig "$crcache_kubeconfig_file"
h1 "Create DB cache cluster"
kind create cluster --name "$dbcache_kind_cluster" --kubeconfig "$dbcache_kubeconfig_file" || kind export kubeconfig --name "$dbcache_kind_cluster" --kubeconfig "$dbcache_kubeconfig_file"



deploy_config_dir="${PORCH_DIR}/.build/disaster-test"
rm -rf "$deploy_config_dir" || true
mkdir -p "$deploy_config_dir"


h1 "Install MetalLB on data cluster"
kubectl_data apply -f https://raw.githubusercontent.com/metallb/metallb/v0.14.9/config/manifests/metallb-native.yaml
sleep 1
echo "Waiting for controller to become ready..."
kubectl_data wait --namespace metallb-system deploy controller \
                --for=condition=available \
                --timeout=90s
kubectl_data apply -f "${PORCH_DIR}/deployments/local/metallb-conf.yaml"


h1 "Install Gitea on data cluster"
# Extract first IP from MetalLB address range
gitea_ip=$(\
    grep -A1 "addresses:" "${PORCH_DIR}/deployments/local/metallb-conf.yaml" \
    | grep -o "[0-9]\+\.[0-9]\+\.[0-9]\+\.[0-9]\+" \
    | head -1 \
)
KUBECONFIG="$data_cluster_kubeconfig_file" "${PORCH_DIR}"/scripts/install-dev-gitea-setup.sh "disaster" "$gitea_ip"


# h1 "Install Porch with CR cache"
# make run-in-kind IMAGE_TAG='test' KIND_CONTEXT_NAME="$crcache_kind_cluster" DEPLOYPORCHCONFIGDIR="$deploy_config_dir/crcache" KUBECONFIG="$crcache_kubeconfig_file"


h1 "Install Postgres on data cluster"
h2 "Preparing deployment config for Porch with DB cache"
make load-images-to-kind deployment-config PORCH_CACHE_TYPE='DB' IMAGE_REPO='porch-kind' IMAGE_TAG='test' KIND_CONTEXT_NAME="$dbcache_kind_cluster" DEPLOYPORCHCONFIGDIR="$deploy_config_dir/dbcache" KUBECONFIG="$dbcache_kubeconfig_file"

h2 "Extracting Postgres deployment config for standalone DB"
postgres_dir="$deploy_config_dir/external-postgres"
mkdir "$postgres_dir"
kpt pkg init "$postgres_dir"

ls -al "$deploy_config_dir"/dbcache
mv "$deploy_config_dir"/dbcache/*porch-postgres*.yaml "$postgres_dir"
cp "$deploy_config_dir"/dbcache/*namespace*.yaml "$postgres_dir"
kpt fn eval "$postgres_dir" \
  --image ghcr.io/kptdev/krm-functions-catalog/starlark:v0.5.0 \
  --match-kind Namespace --match-name porch-fn-system \
  -- source='ctx.resource_list["items"] = []'

h2 "Deploying Postgres"
make KUBECONFIG="$data_cluster_kubeconfig_file" DEPLOYPORCHCONFIGDIR="$postgres_dir" deploy-current-config
db_host_ip=$(kubectl_data --namespace porch-system get service porch-postgresql-lb -o custom-columns='IP:.status.loadBalancer.ingress[0].ip' --no-headers)

h1 "Install Porch with DB cache (with no in-cluster DB)"
h2 "Setting DB cache to use Postgres on data cluster"
cp "$self_dir/postgres-config.yaml" "$deploy_config_dir/dbcache"
sed -i -e 's/\(  DB_HOST: \).*/\1"'"$db_host_ip"'"/' "$deploy_config_dir/dbcache/postgres-config.yaml" && echo "Set"

PORCH_SERVER_MEMORY="${PORCH_SERVER_MEMORY:-2048Mi}"
h2 "Increasing porch-server memory to $PORCH_SERVER_MEMORY"
kpt fn eval "$deploy_config_dir/dbcache/" \
  --image ghcr.io/kptdev/krm-functions-catalog/starlark:v0.5.0 \
  --match-kind Deployment --match-name porch-server --match-namespace porch-system \
  -- source='for resource in ctx.resource_list["items"]:
      containers = resource.get("spec", {}).get("template", {}).get("spec", {}).get("containers", [])
      for container in containers:
        if container["name"] == "porch-server":
          container["resources"]["limits"]["memory"] = "'"$PORCH_SERVER_MEMORY"'"' \
&& echo "Done"

h2 "Deploying..."
make deploy-current-config IMAGE_TAG='test' KIND_CONTEXT_NAME="$dbcache_kind_cluster" DEPLOYPORCHCONFIGDIR="$deploy_config_dir/dbcache" KUBECONFIG="$dbcache_kubeconfig_file"


h1 "Load Git with test packages from Nephio catalog and Kpt examples"
[[ -d "$self_dir/catalog" ]] || git clone "$CATALOG_REPO" "$self_dir/catalog"
[[ -d "$self_dir/kpt" ]] || git clone "$KPT_REPO" "$self_dir/kpt"

for repo in "catalog" "kpt"; do
    cd "$self_dir/$repo"
    for copy in {1..5}; do
        git remote set-url origin "http://nephio:secret@$gitea_ip:3000/nephio/$repo-load-$copy"
        git push origin main
    done
done

mkdir "$self_dir"/edge1 && cd "$self_dir"/edge1 && {
    touch README.md
    git init; git checkout -b main; git config user.name nephio
    git add README.md; git commit -m "first commit"; git remote add origin http://nephio:secret@"$gitea_ip":3000/nephio/edge1.git; git push -u origin main
    cd "$self_dir"
} && rm -rf edge1/


h1 "Create Porch repositories for workload"
sed -i -e 's/\[GITEA_PLACEHOLDER\]/nephio:secret@'"$gitea_ip"':3000/' "$self_dir"/load-repositories/*.yaml
for file in "$self_dir"/load-repositories/*batch*.yaml; do
    h2 "Creating repositories in batch $file"
    kubectl_dbcache apply -f "$file"
    h2 "Waiting for all repositories to have condition Ready==True"
    kubectl_dbcache wait repositories --all-namespaces --all --for condition=Ready --timeout 5m
done
kubectl_dbcache apply -f "$self_dir/load-repositories/edge1-repository.yaml"


h1 "Create some new package revisions in a selection of lifecycle states"

package_revisions="$(kubectl_dbcache get packagerevisions -n porch -o name | awk -F '/' '{print $2}' | grep -E '(catalog|kpt).*-[12]')"
to_clone="$(grep -E '^.*-1\.' <<< "$package_revisions" | sed -n '1,20p')"
to_edit="$(grep -E '^.*-2\.' <<< "$package_revisions" | sed -n '1,20p')"

h2 "Cloning $(wc -l <<< "$to_clone") package revisions"

cloned=""
cloned_count=0
for rev in $to_clone; do
    echo
    cloned_count=$(( cloned_count + 1 ))
    echo "Cloning $rev"
    output="$(porchctl_dbcache -n porch rpkg clone "$rev" "clone-$cloned_count" --repository edge1)"
    cloned+="$(awk '{print $1}' <<< "$output")"$'\n'
    echo "$output"
done

# h2 "Editing $(wc -l <<< "$to_edit") package revisions"

edited=""
# edited_count=0
# for rev in $to_edit; do
#     echo
#     edited_count=$(( edited_count + 1 ))
#     echo "Editing $rev"
#     output="$(porchctl_dbcache -n porch rpkg edit "$rev" --workspace "edit-$edited_count")"
#     edited+="$(awk '{print $1}' <<< "$output")"$'\n'
#     echo "$output"
# done

to_propose="$(sed -n '6,20p' <<< "$cloned")"$'\n'"$(sed -n '6,20p' <<< "$edited")"
h2 "Proposing $(wc -l <<< "$to_propose") package revisions"

proposed=""
proposed_count=0
for rev in $to_propose; do
    echo
    proposed_count=$(( proposed_count + 1 ))
    echo "Proposing $rev"
    output="$(porchctl_dbcache -n porch rpkg propose "$rev")"
    proposed+="$(awk '{print $1}' <<< "$output")"$'\n'
    echo "$output"
done
echo "$proposed"


to_approve="$(sed -n '11,20p' <<< "$cloned")"$'\n'"$(sed -n '11,20p' <<< "$edited")"
h2 "Approving $(wc -l <<< "$to_approve") package revisions"

approved=""
approved_count=0
for rev in $to_approve; do
    echo
    approved_count=$(( approved_count + 1 ))
    echo "Approving $rev"
    output="$(porchctl_dbcache -n porch rpkg approve "$rev")"
    approved+="$(awk '{print $1}' <<< "$output")"$'\n'
    echo "$output"
done
echo "$approved"


to_proposedelete="$(sed -n '16,20p' <<< "$cloned")"$'\n'"$(sed -n '16,20p' <<< "$edited")"
h2 "Proposing $(wc -l <<< "$to_proposedelete") package revisions for deletion"

proposedeleted=""
proposedeleted_count=0
for rev in $to_proposedelete; do
    echo
    proposedeleted_count=$(( proposedeleted_count + 1 ))
    echo "Proposing $rev for deletion"
    output="$(porchctl_dbcache -n porch rpkg propose-delete "$rev")"
    proposedeleted+="$(awk '{print $1}' <<< "$output")"$'\n'
    echo "$output"
done
echo "$proposedeleted"

h1 "--- test/disaster ---"

h1 "Done"
