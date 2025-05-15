#!/usr/bin/env bash
# Copyright 2025 The kpt and Nephio Authors
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

self_dir="$(dirname "$(readlink -f "$0")")"

git_repo_name=${1:-porch-test}
gitea_ip=${2:-172.18.255.200}  # should be from the address range in deployments/local/metallb-conf.yaml

git_root="$(readlink -f "${self_dir}/..")"
cd "${git_root}"

function h1() {
  echo
  echo "** $*"
  echo 
}

h1 Install Gitea and init test repos
mkdir -p "${git_root}/.build"
cd "${git_root}/.build"
if [ -d gitea ]; then
  kpt pkg update gitea
else
  cp -r "${git_root}/test/pkgs/gitea-dev" .
fi

cd "${git_root}/test/pkgs/gitea-dev"

kpt fn eval \
  --image gcr.io/kpt-fn/set-annotations:v0.1.4 \
  --match-kind Service \
  --match-name gitea \
  --match-namespace gitea \
  -- "metallb.universe.tf/loadBalancerIPs=${gitea_ip}"

cp -f "${git_root}/deployments/local/kind_porch_test_cluster.yaml" cluster-config.yaml
# turn kind's cluster-config into a valid KRM
cat >> cluster-config.yaml <<EOF1
metadata: 
  name: not-used 
  annotations:
    config.kubernetes.io/local-config: "true"
EOF1
kpt fn eval \
  --image gcr.io/kpt-fn/apply-replacements:v0.1.1 \
  --fn-config "${git_root}/deployments/local/replace-gitea-service-ports.yaml"

kpt fn render 
kpt live init || true
kpt live apply --inventory-policy=adopt
echo "Waiting for gitea to become ready..."
kubectl wait pod --all --for=condition=Ready --namespace=gitea --timeout=90s

###############################################################

h1 Create git repos in gitea
curl -v -k -H "content-type: application/json" "http://nephio:secret@localhost:3000/api/v1/user/repos" --data "{\"name\":\"$git_repo_name\"}"
TMP_DIR=$(mktemp -d)
cd "$TMP_DIR"
git clone "http://nephio:secret@localhost:3000/nephio/$git_repo_name.git"
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

test_git_repo_name="test-blueprints"
curl -v -k -H "content-type: application/json" "http://nephio:secret@localhost:3000/api/v1/user/repos" --data "{\"name\":\"$test_git_repo_name\"}"
TMP_DIR=$(mktemp -d)
cd "$TMP_DIR"
git clone "${git_root}/test/pkgs/test-pkgs/test-blueprints.bundle" -b main
cd "$test_git_repo_name"

git gc
git remote rename origin upstream
git remote add origin "http://nephio:secret@localhost:3000/nephio/$test_git_repo_name"
git push -u origin --all
git push -u origin --tags

cd "${git_root}"
rm -fr "$TMP_DIR"