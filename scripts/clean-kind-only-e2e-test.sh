#!/usr/bin/env bash
# Copyright 2022 The kpt and Nephio Authors
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

PORCH_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )/.." && pwd )"
cd $PORCH_DIR

kind_cluster="porch-e2e"

kind delete cluster --name "$kind_cluster" || true
kind create cluster --name "$kind_cluster" 
make run-in-kind-kpt IMAGE_TAG='test' KIND_CONTEXT_NAME="$kind_cluster" KUBECONFIG="$(kind get kubeconfig --name "$kind_cluster")"
for deployment in porch-controllers porch-server function-runner; do
    kubectl rollout status deployment $deployment --namespace porch-system
    kubectl wait --namespace porch-system deploy $deployment \
                --for=condition=available \
                --timeout=90s
done
kubectl wait --namespace porch-system pod --selector=app=function-runner \
            --for=condition=ready \
            --timeout=90s

sleep 20

echo "--- test/e2e ---"
E2E=1 go test -failfast -v ./test/e2e
echo "--- test/e2e/cli ---"
E2E=1 go test -failfast -v ./test/e2e/cli
echo "--- leaving e2e kind cluster $kind_cluster up for debugging ---"
