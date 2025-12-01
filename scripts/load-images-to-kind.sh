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

# Load images to kind cluster

set -e

# Source common configuration
source "$(dirname "$0")/common.sh"

# Override defaults for kind usage
IMAGE_REPO=${IMAGE_REPO:-porch-kind}
IMAGE_TAG=${IMAGE_TAG:-test}

echo "Loading images to kind cluster..."
START_TIME=$(date +%s)

if [ "${SKIP_IMG_BUILD}" = "false" ]; then
    build_pids=()
    
    # Build test-git-server if needed
    if [ "${SKIP_LOCAL_GIT}" = "false" ] && ! docker exec "${KIND_CONTEXT_NAME}-control-plane" crictl images | grep -q "${IMAGE_REPO}/${TEST_GIT_SERVER_IMAGE}  *${IMAGE_TAG}"; then
        echo "Building ${IMAGE_REPO}/${TEST_GIT_SERVER_IMAGE}:${IMAGE_TAG}"
        if [ "${PARALLEL_BUILDS:-true}" = "true" ]; then
            (
                IMAGE_NAME="${TEST_GIT_SERVER_IMAGE}" IMAGE_REPO="${IMAGE_REPO}" IMAGE_TAG="${IMAGE_TAG}" make -C test/ build-image
            ) &
            build_pids+=("$!")
        else
            IMAGE_NAME="${TEST_GIT_SERVER_IMAGE}" IMAGE_REPO="${IMAGE_REPO}" IMAGE_TAG="${IMAGE_TAG}" make -C test/ build-image
        fi
    else
        echo "Skipping building ${IMAGE_REPO}/${TEST_GIT_SERVER_IMAGE}:${IMAGE_TAG}"
    fi

    # Build function runner if needed
    if ! docker exec "${KIND_CONTEXT_NAME}-control-plane" crictl images | grep -q "${IMAGE_REPO}/${PORCH_FUNCTION_RUNNER_IMAGE}  *${IMAGE_TAG} "; then
        echo "Building ${IMAGE_REPO}/${PORCH_FUNCTION_RUNNER_IMAGE}:${IMAGE_TAG}"
        if [ "${PARALLEL_BUILDS:-true}" = "true" ]; then
            (
                IMAGE_NAME="${PORCH_FUNCTION_RUNNER_IMAGE}" WRAPPER_SERVER_IMAGE_NAME="${PORCH_WRAPPER_SERVER_IMAGE}" IMAGE_REPO="${IMAGE_REPO}" IMAGE_TAG="${IMAGE_TAG}" make -C func/ build-image
            ) &
            build_pids+=("$!")
        else
            IMAGE_NAME="${PORCH_FUNCTION_RUNNER_IMAGE}" WRAPPER_SERVER_IMAGE_NAME="${PORCH_WRAPPER_SERVER_IMAGE}" IMAGE_REPO="${IMAGE_REPO}" IMAGE_TAG="${IMAGE_TAG}" make -C func/ build-image
        fi
    else
        echo "Skipping building ${IMAGE_REPO}/${PORCH_FUNCTION_RUNNER_IMAGE}:${IMAGE_TAG} as it is already loaded into kind"
    fi

    # Build porch server if needed
    if [ "${SKIP_PORCHSERVER_BUILD}" = "false" ]; then
        echo "Building ${IMAGE_REPO}/${PORCH_SERVER_IMAGE}:${IMAGE_TAG}"
        if [ "${PARALLEL_BUILDS:-true}" = "true" ]; then
            (
                docker buildx build --load --tag ${IMAGE_REPO}/${PORCH_SERVER_IMAGE}:${IMAGE_TAG} -f ./build/Dockerfile "${PORCHDIR}"
            ) &
            build_pids+=("$!")
        else
            docker buildx build --load --tag ${IMAGE_REPO}/${PORCH_SERVER_IMAGE}:${IMAGE_TAG} -f ./build/Dockerfile "${PORCHDIR}"
        fi
    fi

    # Build controllers if needed
    if [ "${SKIP_CONTROLLER_BUILD}" = "false" ]; then
        echo "Building ${IMAGE_REPO}/${PORCH_CONTROLLERS_IMAGE}:${IMAGE_TAG}"
        if [ "${PARALLEL_BUILDS:-true}" = "true" ]; then
            (
                IMAGE_NAME="${PORCH_CONTROLLERS_IMAGE}" IMAGE_REPO="${IMAGE_REPO}" IMAGE_TAG="${IMAGE_TAG}" make -C controllers/ build-image
            ) &
            build_pids+=("$!")
        else
            IMAGE_NAME="${PORCH_CONTROLLERS_IMAGE}" IMAGE_REPO="${IMAGE_REPO}" IMAGE_TAG="${IMAGE_TAG}" make -C controllers/ build-image
        fi
    fi

    # Wait for parallel builds to complete
    if [ "${PARALLEL_BUILDS:-true}" = "true" ] && [ ${#build_pids[@]} -gt 0 ]; then
        echo "Waiting for ${#build_pids[@]} parallel builds to complete..."
        for pid in "${build_pids[@]}"; do
            wait "$pid"
        done
        echo "All builds completed."
    fi

    # Load images to kind (sequential to avoid conflicts)
    if [ "${SKIP_LOCAL_GIT}" = "false" ] && ! docker exec "${KIND_CONTEXT_NAME}-control-plane" crictl images | grep -q "${IMAGE_REPO}/${TEST_GIT_SERVER_IMAGE}  *${IMAGE_TAG}"; then
        kind load docker-image ${IMAGE_REPO}/${TEST_GIT_SERVER_IMAGE}:${IMAGE_TAG} -n ${KIND_CONTEXT_NAME}
    fi

    if ! docker exec "${KIND_CONTEXT_NAME}-control-plane" crictl images | grep -q "${IMAGE_REPO}/${PORCH_FUNCTION_RUNNER_IMAGE}  *${IMAGE_TAG} "; then
        kind load docker-image ${IMAGE_REPO}/${PORCH_FUNCTION_RUNNER_IMAGE}:${IMAGE_TAG} -n ${KIND_CONTEXT_NAME}
        kind load docker-image ${IMAGE_REPO}/${PORCH_WRAPPER_SERVER_IMAGE}:${IMAGE_TAG} -n ${KIND_CONTEXT_NAME}
    fi

    if [ "${SKIP_PORCHSERVER_BUILD}" = "false" ]; then
        kind load docker-image ${IMAGE_REPO}/${PORCH_SERVER_IMAGE}:${IMAGE_TAG} -n ${KIND_CONTEXT_NAME}
        kubectl delete deployment -n porch-system --ignore-not-found=true porch-server
    fi

    if [ "${SKIP_CONTROLLER_BUILD}" = "false" ]; then
        kind load docker-image ${IMAGE_REPO}/${PORCH_CONTROLLERS_IMAGE}:${IMAGE_TAG} -n ${KIND_CONTEXT_NAME}
        kubectl delete deployment -n porch-system --ignore-not-found=true porch-controllers
    fi
else
    # Load pre-built images
    if [ "${SKIP_LOCAL_GIT}" = "false" ]; then
        kind load docker-image ${IMAGE_REPO}/${TEST_GIT_SERVER_IMAGE}:${IMAGE_TAG} -n ${KIND_CONTEXT_NAME}
    fi
    kind load docker-image ${IMAGE_REPO}/${PORCH_FUNCTION_RUNNER_IMAGE}:${IMAGE_TAG} -n ${KIND_CONTEXT_NAME}
    kind load docker-image ${IMAGE_REPO}/${PORCH_WRAPPER_SERVER_IMAGE}:${IMAGE_TAG} -n ${KIND_CONTEXT_NAME}
    kind load docker-image ${IMAGE_REPO}/${PORCH_SERVER_IMAGE}:${IMAGE_TAG} -n ${KIND_CONTEXT_NAME}
    kind load docker-image ${IMAGE_REPO}/${PORCH_CONTROLLERS_IMAGE}:${IMAGE_TAG} -n ${KIND_CONTEXT_NAME}
fi

END_TIME=$(date +%s)
TOTAL_TIME=$((END_TIME - START_TIME))
echo "Images loaded to kind cluster in ${TOTAL_TIME} seconds."