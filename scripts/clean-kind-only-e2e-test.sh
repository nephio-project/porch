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

kind delete cluster --name dev || true
kind create cluster --name dev 
make run-in-kind-kpt IMAGE_TAG='test' KIND_CONTEXT_NAME='dev' KUBECONFIG=$KUBECONFIG
kubectl rollout status deployment porch-controllers --namespace porch-system
kubectl rollout status deployment porch-server --namespace porch-system
kubectl rollout status deployment function-runner --namespace porch-system
sleep 2
E2E=1 go test -v ./test/e2e
