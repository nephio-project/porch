#!/bin/bash
# Reload a specific component in kind cluster

set -e

COMPONENT=$1
KIND_CONTEXT_NAME=${KIND_CONTEXT_NAME:-porch-test}

if [ -z "$COMPONENT" ]; then
    echo "Usage: $0 <component>"
    echo "Components: function-runner, server, controllers"
    exit 1
fi

case $COMPONENT in
    "function-runner")
        echo "Building ${IMAGE_REPO}/${PORCH_FUNCTION_RUNNER_IMAGE}:${IMAGE_TAG}"
        IMAGE_NAME="${PORCH_FUNCTION_RUNNER_IMAGE}" WRAPPER_SERVER_IMAGE_NAME="${PORCH_WRAPPER_SERVER_IMAGE}" make -C func/ build-image
        kind load docker-image ${IMAGE_REPO}/${PORCH_FUNCTION_RUNNER_IMAGE}:${IMAGE_TAG} ${IMAGE_REPO}/${PORCH_WRAPPER_SERVER_IMAGE}:${IMAGE_TAG} -n ${KIND_CONTEXT_NAME}
        kubectl rollout restart -n porch-system deployment/function-runner
        kubectl rollout status deployment function-runner -n porch-system 2>/dev/null || true
        ;;
    "server")
        echo "Building ${IMAGE_REPO}/${PORCH_SERVER_IMAGE}:${IMAGE_TAG}"
        docker build --load --tag ${IMAGE_REPO}/${PORCH_SERVER_IMAGE}:${IMAGE_TAG} -f ./build/Dockerfile "${PORCHDIR}"
        kind load docker-image ${IMAGE_REPO}/${PORCH_SERVER_IMAGE}:${IMAGE_TAG} -n ${KIND_CONTEXT_NAME}
        kubectl rollout restart -n porch-system deployment/porch-server
        kubectl rollout status deployment porch-server -n porch-system 2>/dev/null || true
        ;;
    "controllers")
        echo "Building ${IMAGE_REPO}/${PORCH_CONTROLLERS_IMAGE}:${IMAGE_TAG}"
        IMAGE_NAME="${PORCH_CONTROLLERS_IMAGE}" make -C controllers/ build-image
        kind load docker-image ${IMAGE_REPO}/${PORCH_CONTROLLERS_IMAGE}:${IMAGE_TAG} -n ${KIND_CONTEXT_NAME}
        kubectl rollout restart -n porch-system deployment/porch-controllers
        kubectl rollout status deployment porch-controllers -n porch-system 2>/dev/null || true
        ;;
    *)
        echo "Unknown component: $COMPONENT"
        exit 1
        ;;
esac