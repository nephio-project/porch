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

git_root="$(readlink -f "${self_dir}/..")"
cd "${git_root}"

jaeger_all_in_one_path="${git_root}/deployment/tracing/deployment.yaml"
prometheus_path="${git_root}/deployment/metrics/deployment.yaml"

kubectl create ns monitoring

kubectl apply -f "${jaeger_all_in_one_path}"
kubectl apply -f "${prometheus_path}"
kubectl wait pod --all --for=condition=Ready --namespace=monitoring --timeout=90s

