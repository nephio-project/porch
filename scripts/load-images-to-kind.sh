#!/bin/bash
# Load images to kind cluster

set -e

KIND_CONTEXT_NAME=${KIND_CONTEXT_NAME:-porch-test}

echo "Loading images to kind cluster..."

if [ "${SKIP_IMG_BUILD}" = "false" ]; then
    # Build and load test-git-server if needed
    if [ "${SKIP_LOCAL_GIT}" = "false" ] && ! docker exec "${KIND_CONTEXT_NAME}-control-plane" crictl images | grep -q "${IMAGE_REPO}/${TEST_GIT_SERVER_IMAGE}  *${IMAGE_TAG}"; then
        echo "Building ${IMAGE_REPO}/${TEST_GIT_SERVER_IMAGE}:${IMAGE_TAG}"
        IMAGE_NAME="${TEST_GIT_SERVER_IMAGE}" make -C test/ build-image
        kind load docker-image ${IMAGE_REPO}/${TEST_GIT_SERVER_IMAGE}:${IMAGE_TAG} -n ${KIND_CONTEXT_NAME}
    else
        echo "Skipping building and loading ${IMAGE_REPO}/${TEST_GIT_SERVER_IMAGE}:${IMAGE_TAG}"
    fi

    # Build and load function runner if needed
    if ! docker exec "${KIND_CONTEXT_NAME}-control-plane" crictl images | grep -q "${IMAGE_REPO}/${PORCH_FUNCTION_RUNNER_IMAGE}  *${IMAGE_TAG} "; then
        echo "Building ${IMAGE_REPO}/${PORCH_FUNCTION_RUNNER_IMAGE}:${IMAGE_TAG}"
        IMAGE_NAME="${PORCH_FUNCTION_RUNNER_IMAGE}" WRAPPER_SERVER_IMAGE_NAME="${PORCH_WRAPPER_SERVER_IMAGE}" make -C func/ build-image
        kind load docker-image ${IMAGE_REPO}/${PORCH_FUNCTION_RUNNER_IMAGE}:${IMAGE_TAG} -n ${KIND_CONTEXT_NAME}
        kind load docker-image ${IMAGE_REPO}/${PORCH_WRAPPER_SERVER_IMAGE}:${IMAGE_TAG} -n ${KIND_CONTEXT_NAME}
    else
        echo "Skipping building ${IMAGE_REPO}/${PORCH_FUNCTION_RUNNER_IMAGE}:${IMAGE_TAG} as it is already loaded into kind"
    fi

    # Build and load porch server if needed
    if [ "${SKIP_PORCHSERVER_BUILD}" = "false" ]; then
        echo "Building ${IMAGE_REPO}/${PORCH_SERVER_IMAGE}:${IMAGE_TAG}"
        docker buildx build --load --tag ${IMAGE_REPO}/${PORCH_SERVER_IMAGE}:${IMAGE_TAG} -f ./build/Dockerfile "${PORCHDIR}"
        kind load docker-image ${IMAGE_REPO}/${PORCH_SERVER_IMAGE}:${IMAGE_TAG} -n ${KIND_CONTEXT_NAME}
        kubectl delete deployment -n porch-system --ignore-not-found=true porch-server
    fi

    # Build and load controllers if needed
    if [ "${SKIP_CONTROLLER_BUILD}" = "false" ]; then
        echo "Building ${IMAGE_REPO}/${PORCH_CONTROLLERS_IMAGE}:${IMAGE_TAG}"
        IMAGE_NAME="${PORCH_CONTROLLERS_IMAGE}" make -C controllers/ build-image
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

echo "Images loaded to kind cluster."