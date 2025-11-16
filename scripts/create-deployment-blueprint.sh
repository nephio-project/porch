#!/usr/bin/env bash
# Copyright 2022-2025 The kpt and Nephio Authors
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

function error() {
  cat <<EOF
Error: ${1}
Usage: ${0} [flags]
Supported Flags:
  --destination DIRECTORY             ... directory in which to create the Porch deploymetn blueprint
  --server-image IMAGE                ... address of the Porch server image
  --controllers-image IMAGE           ... address of the Porch controllers image
  --function-image IMAGE              ... address of the Porch function runtime image
  --wrapper-server-image IMAGE        ... address of the Porch function wrapper server image
  --enabled-reconcilers RECONCILDERS  ... comma-separated list of reconcilers that should be enabled in
                                          porch controller
  --ghcr-image-prefix PREFIX          ... ghcr image url prefix for running porch behind a proxy
EOF
  exit 1
}

# Flag variables
DESTINATION=""
SERVER_IMAGE=""
CONTROLLERS_IMAGE=""
FUNCTION_IMAGE=""
WRAPPER_SERVER_IMAGE=""
ENABLED_RECONCILERS=""
GHCR_IMAGE_PREFIX=""

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
    --ghcr-image-prefix)
          GHCR_IMAGE_PREFIX="${2}"
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
}

function customize-image {
  local OLD="${1}"
  local NEW="${2}"
  local TAG="${NEW##*:}"
  local IMG="${NEW%:*}"

  kpt fn eval "${DESTINATION}" --image set-image:v0.1.1 -- \
    "name=${OLD}" \
    "newName=${IMG}" \
    "newTag=${TAG}"
}

function customize-image-in-env {
  local OLD="${1}"
  local NEW="${2}"
  local TAG="${NEW##*:}"
  local IMG="${NEW%:*}"
  local FN_CONFIG="${PORCH_DIR}/.build/set-image-config.yaml"

  cat > "${FN_CONFIG}" << EOF
apiVersion: fn.kpt.dev/v1alpha1
kind: SetImage
metadata:
  name: my-func-config
image:
  name: ${OLD}
  newName: ${IMG}
  newTag: ${TAG}
additionalImageFields:
- group: apps
  version: v1
  kind: Deployment
  path: spec/template/spec/containers[]/env[]/value
EOF

  trap "rm -f ${FN_CONFIG}" EXIT

  kpt fn eval "${DESTINATION}" --image set-image:v0.1.1 --fn-config "${FN_CONFIG}" || echo "kpt fn eval failed"
}


function customize-container-env {
  kpt fn eval ${DESTINATION} \
    --image ghcr.io/kptdev/krm-functions-catalog/starlark:v0.5.0 \
    --match-kind Deployment \
    --match-name porch-controllers \
    --match-namespace porch-system \
    -- "reconcilers=$ENABLED_RECONCILERS" 'source=
reconcilers = ctx.resource_list["functionConfig"]["data"]["reconcilers"].split(",")
for resource in ctx.resource_list["items"]:
  c = resource["spec"]["template"]["spec"]["containers"][0]
  if c["env"] == None:
    c["env"] = []
  for r in reconcilers:
    found = False
    for env in c["env"]:
      if env["name"] == "ENABLE_" + r.upper():
        env["value"] = "true"
        found = True
        break
    if not found:
      c["env"].append({"name": "ENABLE_" + r.upper(), "value": "true"})
'
}

function add_image_args_porch_server() {
    kpt fn eval ${DESTINATION} \
      --image ghcr.io/kptdev/krm-functions-catalog/starlark:v0.5.0 \
      --match-kind Deployment \
      --match-name porch-server \
      --match-namespace porch-system \
      -- "source=
for resource in ctx.resource_list['items']:
  containers = resource['spec']['template']['spec']['containers']
  for container in containers:
    container['args'].append('--default-image-prefix=${GHCR_IMAGE_PREFIX}')
"
}

function main() {
  # Repository CRD
  cp "./api/porchconfig/v1alpha1/config.porch.kpt.dev_repositories.yaml" \
     "${DESTINATION}/0-repositories.yaml"
  cp "./internal/api/porchinternal/v1alpha1/config.porch.kpt.dev_packagerevs.yaml" \
     "${DESTINATION}/0-packagerevs.yaml"

  # Porch Deployment Config
  cp ${PORCH_DIR}/deployments/porch/*.yaml "${PORCH_DIR}/deployments/porch/Kptfile" "${DESTINATION}"
  # Copy Porch controller manager rbac
  cp ${PORCH_DIR}/controllers/config/rbac/role.yaml "${DESTINATION}/9-porch-controller-clusterrole.yaml"

  IFS=',' read -ra RECONCILERS <<< "$ENABLED_RECONCILERS"
  for i in "${RECONCILERS[@]}"; do
    if [[ -f "${PORCH_DIR}/controllers/config/crd/bases/config.porch.kpt.dev_${i}.yaml" ]]; then
      # Copy over the CRD (if it exists)
      cp "${PORCH_DIR}/controllers/config/crd/bases/config.porch.kpt.dev_${i}.yaml" \
         "${DESTINATION}/0-${i}.yaml"
    fi

    # Copy over the rbac rules for the reconciler
    cp "${PORCH_DIR}/controllers/${i}/config/rbac/role.yaml" \
    "${DESTINATION}/9-porch-controller-${i}-clusterrole.yaml"
    # Copy over the rbac rules for the reconciler
    cp "${PORCH_DIR}/controllers/${i}/config/rbac/rolebinding.yaml" \
    "${DESTINATION}/9-porch-controller-${i}-clusterrolebinding.yaml"
  done

  if [[ -n "${GHCR_IMAGE_PREFIX}" ]]; then
          add_image_args_porch_server
  fi

  customize-container-env
  
  customize-image \
    "docker.io/nephio/porch-function-runner:latest" \
    "${FUNCTION_IMAGE}"
  customize-image \
    "docker.io/nephio/porch-server:latest" \
    "${SERVER_IMAGE}"
  customize-image \
    "docker.io/nephio/porch-controllers:latest" \
    "${CONTROLLERS_IMAGE}"
  customize-image-in-env \
    "docker.io/nephio/porch-wrapper-server:latest" \
    "${WRAPPER_SERVER_IMAGE}"
}

validate
main
