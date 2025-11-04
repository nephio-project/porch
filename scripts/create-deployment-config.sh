#!/bin/bash
# Generate deployment configuration

set -e

DEPLOYPORCHCONFIGDIR=${DEPLOYPORCHCONFIGDIR:-$(pwd)/.build/deploy}

echo "Creating deployment configuration..."

rm -rf ${DEPLOYPORCHCONFIGDIR} || true
mkdir -p ${DEPLOYPORCHCONFIGDIR}

./scripts/create-deployment-blueprint.sh \
  --destination "${DEPLOYPORCHCONFIGDIR}" \
  --server-image "${IMAGE_REPO}/${PORCH_SERVER_IMAGE}:${IMAGE_TAG}" \
  --controllers-image "${IMAGE_REPO}/${PORCH_CONTROLLERS_IMAGE}:${IMAGE_TAG}" \
  --function-image "${IMAGE_REPO}/${PORCH_FUNCTION_RUNNER_IMAGE}:${IMAGE_TAG}" \
  --wrapper-server-image "${IMAGE_REPO}/${PORCH_WRAPPER_SERVER_IMAGE}:${IMAGE_TAG}" \
  --enabled-reconcilers "${ENABLED_RECONCILERS}" \
  $(if [ -n "${PORCH_GCR_PREFIX_URL}" ]; then echo "--gcr-image-prefix \"${PORCH_GCR_PREFIX_URL}\""; fi)

echo "Deployment configuration created."