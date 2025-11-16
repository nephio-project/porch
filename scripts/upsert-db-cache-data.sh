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

git_root="$(readlink -f "${self_dir}/..")"
deployment_config_dir="${DEPLOYPORCHCONFIGDIR:-${git_root}/.build/deploy)}"
cd "${deployment_config_dir}"

# add required db cache config to the porch server deployment
kpt fn eval \
  --image ghcr.io/kptdev/krm-functions-catalog/starlark:v0.5.0 \
  --match-kind Deployment \
  --match-name porch-server \
  --match-namespace porch-system \
  --fn-config "${git_root}/deployments/local/postgres/insert-db-cache-config.yaml"

# copy the porch postgres KRM to the deployment pkg
cp "${git_root}/deployments/local/postgres/postgres-statefulset.yaml" 3-porch-postgres-statefulset.yaml

# create the configmap to hold the db schema
kubectl create configmap -n porch-system porch-schema --from-file="${git_root}/api/sql/porch-db.sql" --dry-run=client -o yaml | cat > 3-porch-postgres-schema-configmap.yaml
