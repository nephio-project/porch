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

logs_dir='/tmp/disaster-setup-logs_'$$
mkdir -p "$logs_dir"
trap 'cleanUpLogs' EXIT
function cleanUpLogs(){
    h1 "Clean up log pipes..."
    rm -frv "$logs_dir"
}

source "$self_dir/../util/functions.sh"

CATALOG_REPO='https://github.com/nephio-project/catalog.git'
KPT_REPO='https://github.com/kptdev/kpt.git'
cd "$PORCH_DIR"

export cleandown_before=${CLEANDOWN_BEFORE:-true}
export skip_variant_sets=${SKIP_VARIANT_SETS:-true}
export skip_variants=${SKIP_VARIANTS:-true}
export SKIP_IMG_BUILD="false"

data_kind_cluster="porch-disaster-test-data"
crcache_kind_cluster="porch-disaster-test-crcache"
dbcache_kind_cluster="porch-disaster-test-dbcache"

function install() {
    h1 "Clean down before install"

    if [[ $cleandown_before != "false" ]]; then
        rm -rf "$self_dir/kubeconfigs"
        mkdir "$self_dir/kubeconfigs"
        kind delete cluster --name "$data_kind_cluster" || true
        kind delete cluster --name "$crcache_kind_cluster" || true
        kind delete cluster --name "$dbcache_kind_cluster" || true
    fi

    {
        tmp_logfile="$logs_dir"/create_data_cluster_$$.tmp
        mknod $tmp_logfile p
        sed -e 's/^\(.\)/'"$(tput setaf 6)"'creating data cluster|>'"$(tput sgr0)"' \1/' --unbuffered <$tmp_logfile &
        exec &> $tmp_logfile

        h1 "Create data cluster"
        kind create cluster --name "$data_kind_cluster" --kubeconfig "$data_cluster_kubeconfig_file" || kind export kubeconfig --name "$data_kind_cluster" --kubeconfig "$data_cluster_kubeconfig_file"
    } &

    {
        tmp_logfile="$logs_dir"/create_dbcache_cluster_$$.tmp
        mknod $tmp_logfile p
        sed -e 's/^\(.\)/'"$(tput setaf 5)"'creating DB cache cluster|>'"$(tput sgr0)"' \1/' --unbuffered <$tmp_logfile &
        exec &> $tmp_logfile

        h1 "Create DB cache cluster"
        kind create cluster --name "$dbcache_kind_cluster" --kubeconfig "$dbcache_kubeconfig_file" || kind export kubeconfig --name "$dbcache_kind_cluster" --kubeconfig "$dbcache_kubeconfig_file"
    } &

    {
        tmp_logfile="$logs_dir"/create_crcache_cluster_$$.tmp
        mknod $tmp_logfile p
        sed -e 's/^\(.\)/'"$(tput setaf 3)"'creating CR cache cluster|>'"$(tput sgr0)"' \1/' --unbuffered <$tmp_logfile &
        exec &> $tmp_logfile

        h1 "Create CR cache cluster"
        kind create cluster --name "$crcache_kind_cluster" --kubeconfig "$crcache_kubeconfig_file" || kind export kubeconfig --name "$crcache_kind_cluster" --kubeconfig "$crcache_kubeconfig_file"
    } &

    {
        if [[ $SKIP_IMG_BUILD == "false" ]]; then
            tmp_logfile="$logs_dir"/docker_build$$.tmp
            mknod $tmp_logfile p
            sed -e 's/^\(.\)/'"$(tput setaf 7)"'building images|>'"$(tput sgr0)"' \1/' --unbuffered <$tmp_logfile &
            exec &> $tmp_logfile

            make build-images IMAGE_REPO='porch-kind' IMAGE_TAG='test'
        fi
    } &
    wait

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

    {
        tmp_logfile="$logs_dir"/install_gitea_$$.tmp
        mknod $tmp_logfile p
        sed -e 's/^\(.\)/'"$(tput setaf 6)"'Install Gitea|>'"$(tput sgr0)"' \1/' --unbuffered <$tmp_logfile &
        exec &> $tmp_logfile

        h1 "Install Gitea on data cluster"
        # Extract first IP from MetalLB address range
        gitea_ip=$(\
            grep -A1 "addresses:" "${PORCH_DIR}/deployments/local/metallb-conf.yaml" \
            | grep -o "[0-9]\+\.[0-9]\+\.[0-9]\+\.[0-9]\+" \
            | head -1 \
        )
        KUBECONFIG="$data_cluster_kubeconfig_file" "${PORCH_DIR}"/scripts/install-dev-gitea-setup.sh "disaster" "$gitea_ip"
    } &
    wait

    PORCH_SERVER_MEMORY="${PORCH_SERVER_MEMORY:-4Gi}"
    {
        tmp_logfile="$logs_dir"/install_crcache$$.tmp
        mknod $tmp_logfile p
        sed -e 's/^\(.\)/'"$(tput setaf 3)"'Install Porch with CR cache|>'"$(tput sgr0)"' \1/' --unbuffered <$tmp_logfile &
        exec &> $tmp_logfile

        h1 "Install Porch with CR cache"
        h2 "Preparing deployment config for Porch with DB cache"
        make load-images-to-kind deployment-config PORCH_CACHE_TYPE='CR' SKIP_IMG_BUILD='true' IMAGE_REPO='porch-kind' IMAGE_TAG='test' KIND_CONTEXT_NAME="$crcache_kind_cluster" DEPLOYPORCHCONFIGDIR="$deploy_config_dir/crcache" KUBECONFIG="$crcache_kubeconfig_file"
        h2 "Increasing porch-server memory to $PORCH_SERVER_MEMORY"
        kpt fn eval "$deploy_config_dir/crcache/" \
        --image ghcr.io/kptdev/krm-functions-catalog/starlark:v0.5.0 \
        --match-kind Deployment --match-name porch-server --match-namespace porch-system \
        -- source='for resource in ctx.resource_list["items"]:
            containers = resource.get("spec", {}).get("template", {}).get("spec", {}).get("containers", [])
            for container in containers:
                if container["name"] == "porch-server":
                    container["resources"]["limits"]["memory"] = "'"$PORCH_SERVER_MEMORY"'"' \
        && echo "Done"

        h2 "Deploying..."
        make deploy-current-config IMAGE_TAG='test' KIND_CONTEXT_NAME="$crcache_kind_cluster" DEPLOYPORCHCONFIGDIR="$deploy_config_dir/crcache" KUBECONFIG="$crcache_kubeconfig_file"
    } &

    {
        tmp_logfile="$logs_dir"/install_dbcache_$$.tmp
        mknod $tmp_logfile p
        sed -e 's/^\(.\)/'"$(tput setaf 5)"'Install Porch with DB cache|>'"$(tput sgr0)"' \1/' --unbuffered <$tmp_logfile &
        exec &> $tmp_logfile

        h1 "Install Postgres on data cluster"
        h2 "Preparing deployment config for Porch with DB cache"
        make load-images-to-kind deployment-config PORCH_CACHE_TYPE='DB' SKIP_IMG_BUILD='true' IMAGE_REPO='porch-kind' IMAGE_TAG='test' KIND_CONTEXT_NAME="$dbcache_kind_cluster" DEPLOYPORCHCONFIGDIR="$deploy_config_dir/dbcache" KUBECONFIG="$dbcache_kubeconfig_file"

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
        db_host_ip=$(\
            kubectl_data --namespace porch-system get service porch-postgresql-lb -o custom-columns='IP:.status.loadBalancer.ingress[0].ip' --no-headers \
        ) || { echo "error getting DB host IP from data cluster"; exit 1; }

        h1 "Install Porch with DB cache (with no in-cluster DB)"
        h2 "Setting DB cache to use Postgres on data cluster (at IP $db_host_ip)"
        cp "$self_dir/postgres-config.yaml" "$deploy_config_dir/dbcache"
        sed -i -e 's/\(  DB_HOST: \).*/\1"'"$db_host_ip"'"/' "$deploy_config_dir/dbcache/postgres-config.yaml" && echo "Set"

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
    } &
    wait
}

function resetTestWorkload() {
    if [[ $# -gt 0 && $1 == "cleandown" ]]; then
        h1 "Cleaning down..."
        for cluster in dbcache crcache; do
            {
                color="$( if [[ $cluster == "dbcache" ]]; then echo 5; elif [[ $cluster == "crcache" ]]; then echo 3; fi ; )"
                tmp_logfile="$logs_dir"'/cleandown_'"$cluster"'_'$$'.tmp'
                mknod $tmp_logfile p
                sed -e 's/^\(.\)/'"$(tput setaf "$color")"'cleaning down '"$cluster"'|>'"$(tput sgr0)"' \1/' --unbuffered <$tmp_logfile &
                exec &> $tmp_logfile

                function clusterKubectl(){ "kubectl_$cluster" "$@" ; }

                if [[ $skip_variant_sets != "true" ]]; then
                    h2 "Deleting package variant sets"
                    clusterKubectl delete packagevariantsets --all
                fi
                if [[ $skip_variants != "true" ]]; then
                    h2 "Deleting package variants"
                    clusterKubectl delete packagevariants --all
                fi

                h2 "Deleting Porch repositories"
                clusterKubectl delete repositories --all
            } &
            wait
        done

        h2 "Deleting Git repositories"
        kubectl_data --namespace gitea delete pod --selector app=gitea --force --grace-period 0

        h2 "Wiping and resetting Postgres"
        { kubectl_data -n porch-system delete pvc data-porch-postgresql-0 --force --grace-period 0 & }
        kubectl_data -n porch-system patch pvc data-porch-postgresql-0 -p '{"metadata":{"finalizers":null}}' --type merge
        kubectl_data -n porch-system delete pod porch-postgresql-0 --force --grace-period 0

        h2 "Waiting for Gitea and Postgres to become ready again"
        kubectl_data --namespace gitea wait --for condition=available --timeout 5m deployment/gitea
        kubectl_data --namespace porch-system wait --for condition=Ready --timeout 5m pod porch-postgresql-0
    fi

    h1 "Load Git with test packages from Nephio catalog and Kpt examples"
    [[ -d "$self_dir/catalog" ]] || {
        git clone "$CATALOG_REPO" "$self_dir/catalog"
    } &
    [[ -d "$self_dir/kpt" ]] || {
        git clone "$KPT_REPO" "$self_dir/kpt"
        cd "$self_dir/kpt" && {
            find "$self_dir/kpt" -mindepth 1 -maxdepth 1 -type d ! \( -name ".git" -o -name "e2e" -o -name "internal" -o -name "package-examples" -o -name "rollouts" \) -print0 | xargs -0 git rm -r
            git repack -a -d -f --depth=250 --window=250
            git commit  -m "trim down repo file size for Porch disaster tests"
        }
    } &
    wait

    gitea_ip="$(\
        kubectl_data --namespace gitea get service gitea-lb --output jsonpath='{.status.loadBalancer.ingress[0].ip}' \
    )" || { echo "Error getting Gitea IP from data cluster"; exit 1; }

    {
        for repo in "catalog" "kpt"; do
            {
                tmp_logfile="$logs_dir"/push_"$repo"_upstream_repos_$$.tmp
                mknod $tmp_logfile p
                sed -e 's/^\(.\)/'"$(tput setaf 6)"'pushing "'"$repo"'" copies|>'"$(tput sgr0)"' \1/' --unbuffered <$tmp_logfile &
                exec &> $tmp_logfile

                cd "$self_dir/$repo"
                git config http.postBuffer 524288000
                git config protocol.version 2
                for copy in {1..6}; do
                    h2 "Pushing http://nephio:secret@$gitea_ip:3000/nephio/$repo-load-$copy"
                    git remote set-url origin "http://nephio:secret@$gitea_ip:3000/nephio/$repo-load-$copy"
                    git push origin main
                done
            } &
        done
        wait
    } &

    {
        tmp_logfile="$logs_dir"/push_downstream_repos_$$.tmp
        mknod $tmp_logfile p
        sed -e 's/^\(.\)/'"$(tput setaf 6)"'pushing edge1 repo copies|>'"$(tput sgr0)"' \1/' --unbuffered <$tmp_logfile &
        exec &> $tmp_logfile
        mkdir "$self_dir"/edge1 && cd "$self_dir"/edge1 && {
            touch README.md
            git init; git checkout -b main; git config user.name nephio
            git add README.md; git commit -m "first commit";
            
            git remote add origin http://nephio:secret@"$gitea_ip":3000/nephio/edge1-dbcache.git; git push -u origin main
            git remote set-url origin http://nephio:secret@"$gitea_ip":3000/nephio/edge1-crcache.git; git push -u origin main

            if [[ $skip_variant_sets != "true" ]]; then
                for copy in {1..10}; do
                    h2 "Pushing http://nephio:secret@$gitea_ip:3000/nephio/variantset-edge-$copy"
                    git remote set-url origin "http://nephio:secret@$gitea_ip:3000/nephio/variantset-edge-$copy"
                    git push origin main
                done
            fi

            cd "$self_dir"
        } && rm -rf edge1/
    } &
    wait

    sed -i -e 's/\[GITEA_PLACEHOLDER\]/nephio:secret@'"$gitea_ip"':3000/' "$self_dir"/load-repositories/*.yaml

    for cluster in crcache dbcache; do
        {
            color="$( if [[ $cluster == "dbcache" ]]; then echo 5; elif [[ $cluster == "crcache" ]]; then echo 3; fi ; )"
            tmp_logfile="$logs_dir"'/load_on_'"$cluster"'_'$$'.tmp'
            mknod $tmp_logfile p
            sed -e 's/^\(.\)/'"$(tput setaf "$color")"'loading Porch with '"$cluster"'|>'"$(tput sgr0)"' \1/' --unbuffered <$tmp_logfile &
            exec &> $tmp_logfile

            function clusterKubectl(){ "kubectl_$cluster" "$@" ; }
            function clusterPorchctl(){ "porchctl_$cluster" "$@" ; }

            sed -e 's/edge1\.git/edge1-'"$cluster"'.git/' "$self_dir/load-repositories/edge1-repository.yaml" | clusterKubectl apply -f -

            if [[ $skip_variant_sets != "true" ]]; then
                h2 "Creating variant-set repositories from $self_dir/load-repositories/variantset-repositories.yaml"
                clusterKubectl apply -f "$self_dir/load-repositories/variantset-repositories.yaml"
                h2 "Waiting for all repositories to have condition Ready==True"
                clusterKubectl wait repositories --all-namespaces --all --for 'condition=Ready=True' --timeout 10m

                h2 "Creating package variant set from $self_dir/load-variantset.yaml"
                clusterKubectl apply -f "$self_dir/load-variantset.yaml"
                h2 "Waiting for package variant set to have condition Ready==True"
                clusterKubectl wait packagevariantsets --all --all-namespaces --for 'condition=Ready=True' --timeout 5m
            fi

            if [[ $skip_variants != "true" ]]; then
                h2 "Creating package variants from $self_dir/load-variants.yaml"
                clusterKubectl apply -f "$self_dir/load-variants.yaml"
            fi

            h1 "Create Porch repositories for workload"
            for file in "$self_dir"/load-repositories/*batch*.yaml; do
                h2 "Creating repositories in batch $file"
                clusterKubectl apply -f "$file"
                h2 "Waiting for all repositories to have condition Ready==True"
                clusterKubectl wait repositories --all-namespaces --all --for 'condition=Ready=True' --timeout 20m
            done

            if [[ $skip_variant_sets != "true" || $skip_variants != "true" ]]; then

                h2 "Waiting for all package variants to have condition Ready==True"
                clusterKubectl wait packagevariants --all --all-namespaces --for 'condition=Ready=True' --timeout 10m
            fi


            h1 "Create some new package revisions in a selection of lifecycle states"

            package_revisions="$(\
                clusterKubectl get packagerevisions -o name --no-headers | awk -F '/' '{print $2}' \
            )" || { echo "Error getting package revisions to clone"; exit 1; }

            to_clone="$(grep -E '(catalog|kpt).*-[12]' <<< "$package_revisions" | grep -E '^.*-1\.' | sed -n '1,20p')"
            to_edit="$(grep -E '(catalog|kpt).*-[12]' <<< "$package_revisions" | grep -E '^.*-2\.' | sed -n '1,20p')"

            {
                h2 "Cloning $(wc -l <<< "$to_clone") package revisions"
                cloned_count=0
                for rev in $to_clone; do
                    echo
                    cloned_count=$(( cloned_count + 1 ))
                    echo "Cloning $rev"
                    clusterPorchctl rpkg clone "$rev" "clone-$cloned_count" --repository edge1
                done
            } #&

            {
                h2 "Editing $(wc -l <<< "$to_edit") package revisions"
                edited_count=0
                for rev in $to_edit; do
                    echo
                    edited_count=$(( edited_count + 1 ))
                    clusterPorchctl rpkg clone "$rev" "clone-to-edit-$edited_count" --repository edge1
                done
                to_edit="$(\
                    clusterKubectl get packagerevisions -o name --no-headers | awk -F '/' '{print $2}' | grep -E 'clone-to-edit' \
                )" || { echo "Error getting package revisions to edit"; exit 1; }
                for rev in $to_edit; do
                    echo
                    clusterPorchctl rpkg propose "$rev"
                    clusterPorchctl rpkg approve "$rev"
                    echo "Editing $rev"
                    clusterPorchctl rpkg edit "$rev" --workspace "edit-$edited_count"
                done
            } #&
            #wait

            package_revisions="$(\
                clusterKubectl get packagerevisions -o name --no-headers | awk -F '/' '{print $2}' \
            )" || { echo "Error getting package revisions to perform lifecycle operations"; exit 1; }
            cloned="$(grep -E "clone-" <<< "$package_revisions")" || { echo "Error picking out cloned package revisions"; exit 1; }
            edited="$(grep -E "\.edit-" <<< "$package_revisions")" || { echo "Error picking out edited package revisions"; exit 1; }
            
            if [[ $skip_variant_sets != "true" ]]; then
                variantset_created="$(grep -E 'variantset' <<< "$package_revisions")" || { echo "Error picking out package revisions created from variant-set"; exit 1; }
            else
                variantset_created=""
            fi
            if [[ $skip_variants != "true" ]]; then
                variant_created="$(grep -E '\-variant\.' <<< "$package_revisions")" || { echo "Error picking out package revisions created from variants"; exit 1; }
            else
                variant_created=""
            fi

            to_propose="$(sed -n '6,20p' <<< "$cloned")"$'\n'"$(sed -n '6,20p' <<< "$edited")"$'\n'"$(sed -n '6,20p' <<< "$variantset_created")"$'\n'"$(sed -n '6,20p' <<< "$variant_created")"
            h2 "Proposing $(wc -l <<< "$(echo -n "$to_propose")") package revisions"

            for rev in $to_propose; do
                echo
                echo "Proposing $rev"
                clusterPorchctl rpkg propose "$rev"
            done


            to_approve="$(sed -n '11,20p' <<< "$cloned")"$'\n'"$(sed -n '11,20p' <<< "$edited")"$'\n'"$(sed -n '11,20p' <<< "$variantset_created")"$'\n'"$(sed -n '11,20p' <<< "$variant_created")"
            h2 "Approving $(wc -l <<< "$(echo -n "$to_approve")") package revisions"

            for rev in $to_approve; do
                echo
                echo "Approving $rev"
                clusterPorchctl rpkg approve "$rev"
            done


            to_proposedelete="$(sed -n '16,20p' <<< "$cloned")"$'\n'"$(sed -n '16,20p' <<< "$edited")"$'\n'"$(sed -n '16,20p' <<< "$variantset_created")"$'\n'"$(sed -n '16,20p' <<< "$variant_created")"
            h2 "Proposing $(wc -l <<< "$(echo -n "$to_proposedelete")") package revisions for deletion"

            for rev in $to_proposedelete; do
                echo
                echo "Proposing $rev for deletion"
                clusterPorchctl rpkg propose-delete "$rev"
            done
        } &
    done
    wait
}

if [[ $# -gt 0 && $1 == "reset" ]]; then
    resetTestWorkload "cleandown"
else
    install
    resetTestWorkload
fi