#!/usr/bin/env bash
# Copyright 2022-2024 The kpt and Nephio Authors
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

function_runner_ip="${1:-172.18.255.201}"
self_dir="$(dirname "$(readlink -f "$0")")"
deployment_config_dir="${DEPLOYPORCHCONFIGDIR:-$(readlink -f "${self_dir}/../.build/deploy")}"

cd "${deployment_config_dir}"

# remove porch-controllers Deployment from package
kpt fn eval \
  --image gcr.io/kpt-fn/starlark:v0.5.0 \
  --match-kind Deployment \
  --match-name porch-controllers \
  --match-namespace porch-system \
  -- 'source=ctx.resource_list["items"] = []'
