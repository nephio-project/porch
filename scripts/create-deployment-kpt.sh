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
  --enabled-reconcilers RECONCILDERS  ... comma-separated list of reconcilers that should be enabled in porch controller
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

    --enabled-reconcilers)
      ENABLED_RECONCILERS="${2}"
      shift 2
      ;;

    *)
      error "Invalid argument: ${key}"
    ;;
  esac
done


function validate() {
  [ -n "${DESTINATION}"       ] || error "--destination is required"
  [ -n "${SERVER_IMAGE}"      ] || error "--server-image is required"
  [ -n "${CONTROLLERS_IMAGE}" ] || error "--controllers-image is required"
  [ -n "${FUNCTION_IMAGE}"    ] || error "--function-image is required"
  [ -n "${WRAPPER_SERVER_IMAGE}"    ] || error "--wrapper-server-image is required"
}


function customize_kpt {
	kpt fn eval "${DESTINATION}" --image gcr.io/kpt-fn/search-replace:v0.2.0 -- by-value-regex="${1}" put-value="${2}"
}

function customize-container-env {
  local ENV_KEY="${1}"
  local ENV_VAL="${2}"

  # TODO: This is terrible. Do we have a good way to handle this with kpt?
  sed "/env:/a\            - name: ${ENV_KEY}\n              value: ${ENV_VAL}\n" -i "${DESTINATION}/porch/9-controllers.yaml"
}

function main() {
  
  kpt pkg get https://github.com/nephio-project/catalog/tree/main/nephio/core/porch ${DESTINATION}

  IFS=',' read -ra RECONCILERS <<< "$ENABLED_RECONCILERS"
  for i in "${RECONCILERS[@]}"; do
    # Update the porch-controllers Deployment env variables to enable the reconciler.
    customize-container-env \
      "ENABLE_${i^^}" \
      "\"true\""
  done

  customize_kpt \
  "porch-server:latest" \
  "${SERVER_IMAGE}"

  customize_kpt \
  "porch-controllers:latest" \
  "${CONTROLLERS_IMAGE}"

  customize_kpt \
  "porch-function-runner:latest" \
  "${FUNCTION_IMAGE}"

  customize_kpt \
  "porch-wrapper-server:latest" \
  "${WRAPPER_SERVER_IMAGE}"
}

validate
main
