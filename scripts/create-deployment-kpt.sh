#!/usr/bin/env bash
# Copyright 2024 The Nephio Authors
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

function error() {
  cat <<EOF
Error: ${1}
Usage: ${0} [flags]
Supported Flags:
  --destination DIRECTORY             ... directory in which the Porch kpt pkg will be downloaded to
  --server-image IMAGE                ... address of the Porch server image
  --controllers-image IMAGE           ... address of the Porch controllers image
  --function-image IMAGE              ... address of the Porch function runtime image
  --wrapper-server-image IMAGE        ... address of the Porch function wrapper server image
  --test-git-server-image             ... address of the test git server image
  --enabled-reconcilers RECONCILERS   ... comma-separated list of reconcilers that should be enabled in porch controller
EOF
  exit 1
}

# Flag variables
DESTINATION=""
IMAGE_REPO=""
IMAGE_TAG=""
ENABLED_RECONCILERS=""

while [[ $# -gt 0 ]]; do
  key="${1}"
  case "${key}" in
    --destination)
      DESTINATION="${2}"
      shift 2
    ;;

   --server-image)
      SERVER_IMAGE="${2}"
      shift 2
    ;;

    --controllers-image)
      CONTROLLERS_IMAGE="${2}"
      shift 2
    ;;

    --function-image)
      FUNCTION_IMAGE="${2}"
      shift 2
    ;;

    --wrapper-server-image)
      WRAPPER_SERVER_IMAGE="${2}"
      shift 2
    ;;

    --test-git-server-image)
      TEST_GIT_SERVER_IMAGE="${2}"
      shift 2
    ;;

    --enabled-reconcilers)
      ENABLED_RECONCILERS="${2}"
      shift 2
    ;;
    
    --kind-context)
      KIND_CONTEXT_NAME="${2}"
      shift 2
    ;;

    *)
      error "Invalid argument: ${key}"
    ;;
  esac
done


function validate() {
  yq -v &> /dev/null            || error "'yq' command must be installed"
  [ -n "${DESTINATION}"       ] || error "--destination is required"
  [ -n "${SERVER_IMAGE}"      ] || error "--server-image is required"
  [ -n "${CONTROLLERS_IMAGE}" ] || error "--controllers-image is required"
  [ -n "${FUNCTION_IMAGE}"    ] || error "--function-image is required"
  [ -n "${WRAPPER_SERVER_IMAGE}"    ] || error "--wrapper-server-image is required"
}


function customize-pkg-images {
	kpt fn eval "${DESTINATION}" --image gcr.io/kpt-fn/search-replace:v0.2.0 -- by-value-regex="${1}" put-value="${2}"
}

function deploy-gitea-dev-pkg {
	cp -R ./test/pkgs/gitea-dev "${DESTINATION}"
  kpt fn render ${DESTINATION}/gitea-dev
  kpt live init ${DESTINATION}/gitea-dev
  kpt live apply ${DESTINATION}/gitea-dev
}

function deploy-porch-dev-pkg {
  kpt fn render ${DESTINATION}/porch
  kpt live init ${DESTINATION}/porch
  kpt live apply ${DESTINATION}/porch
}

function load-custom-images {
  kind load docker-image ${SERVER_IMAGE} -n ${KIND_CONTEXT_NAME}
	kind load docker-image ${CONTROLLERS_IMAGE} -n ${KIND_CONTEXT_NAME}
	kind load docker-image ${FUNCTION_IMAGE} -n ${KIND_CONTEXT_NAME}
	kind load docker-image ${WRAPPER_SERVER_IMAGE} -n ${KIND_CONTEXT_NAME}
  kind load docker-image ${TEST_GIT_SERVER_IMAGE} -n ${KIND_CONTEXT_NAME}
}

function main() {
  
  load-custom-images

  kpt pkg get https://github.com/nephio-project/catalog/tree/main/nephio/core/porch ${DESTINATION}

  yq -i eval 'del(.spec.template.spec.containers[0].env)' "${DESTINATION}/porch/9-controllers.yaml"
  yq -i eval '.spec.template.spec.containers[0].env = []' "${DESTINATION}/porch/9-controllers.yaml"

  IFS=',' read -ra RECONCILERS <<< "$ENABLED_RECONCILERS"
  for i in "${RECONCILERS[@]}"; do
    # Update the porch-controllers Deployment env variables to enable the reconciler.
    RECONCILER_ENV_VAR="ENABLE_$(echo "$i" | tr '[:lower:]' '[:upper:]')"
    reconciler="$RECONCILER_ENV_VAR" \
      yq -i eval '.spec.template.spec.containers[0].env += {"name": env(reconciler), "value": "true"}' \
      "${DESTINATION}/porch/9-controllers.yaml"
  done

  customize-pkg-images \
  "porch-server:v2.0.0" \
  "${SERVER_IMAGE}"

  customize-pkg-images \
  "porch-controllers:v2.0.0" \
  "${CONTROLLERS_IMAGE}"

  customize-pkg-images \
  "porch-function-runner:v2.0.0" \
  "${FUNCTION_IMAGE}"

  customize-pkg-images \
  "porch-wrapper-server:v2.0.0" \
  "${WRAPPER_SERVER_IMAGE}"

  deploy-porch-dev-pkg
}

validate
main
