#!/bin/bash
# Apply development configuration
# These scripts are likely defunct as we do not plan to use GCP registries

set -e

echo "Applying development configuration..."

# TODO: Replace with KCC (or self-host a registry?)
gcloud services enable artifactregistry.googleapis.com
gcloud artifacts repositories describe  --location=us-west1 packages --format="value(name)" || gcloud artifacts repositories create  --location=us-west1 --repository-format=docker packages

# TODO: Replace with kpt function
cat config/samples/oci-repository.yaml | sed -e s/example-google-project-id/${GCP_PROJECT_ID}/g | kubectl apply -f -

# TODO: Replace with KCC (or self-host a registry?)
gcloud services enable artifactregistry.googleapis.com
gcloud artifacts repositories describe  --location=us-west1 deployment --format="value(name)" || gcloud artifacts repositories create  --location=us-west1 --repository-format=docker deployment

# TODO: Replace with kpt function
cat config/samples/deployment-repository.yaml | sed -e s/example-google-project-id/${GCP_PROJECT_ID}/g | kubectl apply -f -

echo "Development configuration applied."