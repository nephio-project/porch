#!/usr/bin/env bash
# Fallback defaults for direct script execution
# NOTE: Users should use root Makefile targets instead of calling scripts directly

# Only set defaults if variables are not already exported from Makefile
IMAGE_REPO=${IMAGE_REPO:-docker.io/nephio}
IMAGE_TAG=${IMAGE_TAG:-latest}

PORCH_SERVER_IMAGE=${PORCH_SERVER_IMAGE:-porch-server}
PORCH_CONTROLLERS_IMAGE=${PORCH_CONTROLLERS_IMAGE:-porch-controllers}
PORCH_FUNCTION_RUNNER_IMAGE=${PORCH_FUNCTION_RUNNER_IMAGE:-porch-function-runner}
PORCH_WRAPPER_SERVER_IMAGE=${PORCH_WRAPPER_SERVER_IMAGE:-porch-wrapper-server}
TEST_GIT_SERVER_IMAGE=${TEST_GIT_SERVER_IMAGE:-test-git-server}

SKIP_IMG_BUILD=${SKIP_IMG_BUILD:-false}
SKIP_PORCHSERVER_BUILD=${SKIP_PORCHSERVER_BUILD:-false}
SKIP_CONTROLLER_BUILD=${SKIP_CONTROLLER_BUILD:-false}
SKIP_LOCAL_GIT=${SKIP_LOCAL_GIT:-false}

PORCHDIR=${PORCHDIR:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)}
KIND_CONTEXT_NAME=${KIND_CONTEXT_NAME:-porch-test}
ENABLED_RECONCILERS=${ENABLED_RECONCILERS:-"packagevariants,packagevariantsets"}
PORCH_CACHE_TYPE=${PORCH_CACHE_TYPE:-CR}
FN_RUNNER_WARM_UP_POD_CACHE=${FN_RUNNER_WARM_UP_POD_CACHE:-true}
DEPLOYPORCHCONFIGDIR=${DEPLOYPORCHCONFIGDIR:-${PORCHDIR}/.build/deploy}
