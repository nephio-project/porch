# Copyright 2022-2024 The kpt and Nephio Authors
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

MYGOBIN := $(shell go env GOPATH)/bin
KUBECONFIG=$(CURDIR)/deployments/local/kubeconfig
BUILDDIR=$(CURDIR)/.build
CACHEDIR=$(CURDIR)/.cache
DEPLOYCONFIGDIR=$(BUILDDIR)/deploy
DEPLOYKPTCONFIGDIR=$(DEPLOYCONFIGDIR)/kpt_pkgs
DEPLOYCONFIG_NO_SA_DIR=$(BUILDDIR)/deploy-no-sa
KPTDIR=$(abspath $(CURDIR)/..)

# This includes the following targets:
#   test, unit, unit-clean,
#   gosec, lint,
#   fmt, vet
include default-go.mk

# This includes the 'help' target that prints out all targets with their descriptions organized by categories
include default-help.mk

# GCP project to use for development

ifeq ($(GCP_PROJECT_ID),)
ifeq ($(shell command -v gcloud > /dev/null 2>&1; echo $$?), 0)
export GCP_PROJECT_ID=$(shell gcloud config get-value project)
else
export GCP_PROJECT_ID=pure-faculty-367518
endif
endif

export IMAGE_REPO ?= docker.io/nephio
export USER ?= nephio

export IMAGE_TAG
ifndef IMAGE_TAG
  git_tag := $(shell git rev-parse --short HEAD || "latest" )
  $(shell git diff --quiet)
  ifneq ($(.SHELLSTATUS), 0)
    git_tag := $(git_tag)-dirty
  endif

  IMAGE_TAG=$(USER)-$(git_tag)
endif

PORCH_SERVER_IMAGE ?= porch-server
PORCH_FUNCTION_RUNNER_IMAGE ?= porch-function-runner
PORCH_CONTROLLERS_IMAGE ?= porch-controllers
PORCH_WRAPPER_SERVER_IMAGE ?= porch-wrapper-server
TEST_GIT_SERVER_IMAGE ?= test-git-server
SKIP_IMG_BUILD ?= false

# Only enable a subset of reconcilers in porch controllers by default. Use the RECONCILERS
# env variable to specify a specific list of reconcilers or use
# RECONCILERS=* to enable all known reconcilers.
ALL_RECONCILERS="packagevariants,packagevariantsets,fleetsyncs"
ifndef RECONCILERS
  ENABLED_RECONCILERS="packagevariants,packagevariantsets"
else
  ifeq ($(RECONCILERS),*)
    ENABLED_RECONCILERS=${ALL_RECONCILERS}
  else
    ENABLED_RECONCILERS=$(RECONCILERS)
  endif
endif

# Modules are ordered in dependency order. A module precedes modules that depend on it.
MODULES = \
 examples/apps/hello-server \
 api \
 . \
 controllers \

.DEFAULT_GOAL := all

.PHONY: all
all: stop network start-etcd start-kube-apiserver start-function-runner run-local

.PHONY: network
network:
	docker network create --subnet 192.168.8.0/24 porch

.PHONY: stop
stop:
	docker stop kube-apiserver || true
	docker rm kube-apiserver || true
	docker stop etcd || true
	docker rm etcd || true
	docker stop function-runner || true
	docker rm function-runner || true
	docker network rm porch || true

.PHONY: start-etcd
start-etcd:
	docker buildx build -t etcd --output=type=docker -f ./build/Dockerfile.etcd ./build
	mkdir -p $(BUILDDIR)/data/etcd
	docker stop etcd || true
	docker rm etcd || true
	docker run --detach --user `id -u`:`id -g` \
	  --network=porch \
	  --ip 192.168.8.200 \
	  --name etcd -v $(BUILDDIR)/data/etcd:/data \
	  etcd --listen-client-urls http://0.0.0.0:2379 --advertise-client-urls http://127.0.0.1:2379

.PHONY: start-kube-apiserver
start-kube-apiserver:
	docker buildx build -t kube-apiserver --output=type=docker -f ./build/Dockerfile.apiserver ./build
	docker stop kube-apiserver || true
	docker rm kube-apiserver || true
	deployments/local/makekeys.sh
	docker run --detach --user `id -u`:`id -g` \
	  --network=porch \
	  --ip 192.168.8.201 \
	  --name kube-apiserver -v $(BUILDDIR)/pki:/pki \
	  --add-host host.docker.internal:host-gateway \
	  kube-apiserver \
	  --etcd-servers http://etcd:2379 \
	  --secure-port 9444 \
	  --service-account-issuer=https://kubernetes.default.svc.cluster.local \
	  --service-account-key-file=/pki/service-account.pub \
	  --service-account-signing-key-file=/pki/service-account.key \
	  --cert-dir=/pki \
	  --authorization-mode=RBAC \
	  --anonymous-auth=false \
	  --client-ca-file=/pki/ca.crt

.PHONY: start-function-runner
start-function-runner:
	IMAGE_NAME="$(PORCH_FUNCTION_RUNNER_IMAGE)" $(MAKE) -C ./func build-image
	docker stop function-runner || true
	docker rm -f function-runner || true
	docker run --detach \
	  --network=porch \
	  --ip 192.168.8.202 \
	  --name function-runner \
	  $(IMAGE_REPO)/$(PORCH_FUNCTION_RUNNER_IMAGE):$(IMAGE_TAG) \
	  -disable-runtimes pod

.PHONY: generate-api
generate-api:
	KUBE_VERBOSE=2 $(CURDIR)/scripts/generate-api.sh

.PHONY: generate
generate: generate-api
	@for f in $(MODULES); do (cd $$f; echo "Generating $$f"; go generate -v ./...) || exit 1; done

.PHONY: tidy
tidy:
	@for f in $(MODULES); do (cd $$f; echo "Tidying $$f"; go mod tidy) || exit 1; done

.PHONY: test-e2e
test-e2e:
	E2E=1 go test -v -race --count=1 -failfast ./test/e2e 

.PHONY: configure-git
configure-git:
	git config --global --add user.name test
	git config --global --add user.email test@nephio.org

.PHONY: ci-unit
ci-unit: configure-git test


PORCH = $(BUILDDIR)/porch
PORCHCTL = $(BUILDDIR)/porchctl

.PHONY: run-local
run-local: porch
	KUBECONFIG=$(KUBECONFIG) kubectl apply -f deployments/local/localconfig.yaml
	KUBECONFIG=$(KUBECONFIG) kubectl apply -f api/porchconfig/v1alpha1/
	KUBECONFIG=$(KUBECONFIG) kubectl apply -f internal/api/porchinternal/v1alpha1/
	$(PORCH) \
	--secure-port 9443 \
	--standalone-debug-mode \
	--kubeconfig="$(KUBECONFIG)" \
	--cache-directory="$(CACHEDIR)" \
	--function-runner 192.168.8.202:9445 \
	--repo-sync-frequency=60s

.PHONY: run-jaeger
run-jaeger:
	docker run --rm --name jaeger -d -p4317:55680 -p6831:6831/udp -p16686:16686 jaegertracing/opentelemetry-all-in-one:latest

.PHONY: porch
porch:
	go build -o $(PORCH) ./cmd/porch

.PHONY: porchctl
porchctl:
	go build -o $(PORCHCTL) ./cmd/porchctl

.PHONY: fix-headers
fix-headers:
	../scripts/update-license.sh

.PHONY: fix-all
fix-all: fix-headers fmt tidy

.PHONY: push-images
push-images:
	docker buildx build --push --tag $(IMAGE_REPO)/$(PORCH_SERVER_IMAGE):$(IMAGE_TAG) -f ./build/Dockerfile "$(KPTDIR)"
	IMAGE_NAME="$(PORCH_CONTROLLERS_IMAGE)" make -C controllers/ push-image
	IMAGE_NAME="$(PORCH_FUNCTION_RUNNER_IMAGE)" WRAPPER_SERVER_IMAGE_NAME="$(PORCH_WRAPPER_SERVER_IMAGE)" make -C func/ push-image
	IMAGE_NAME="$(TEST_GIT_SERVER_IMAGE)" make -C test/ push-image

.PHONY: build-images
build-images:
	docker buildx build --load --tag $(IMAGE_REPO)/$(PORCH_SERVER_IMAGE):$(IMAGE_TAG) -f ./build/Dockerfile "$(KPTDIR)"
	IMAGE_NAME="$(PORCH_CONTROLLERS_IMAGE)" make -C controllers/ build-image
	IMAGE_NAME="$(PORCH_FUNCTION_RUNNER_IMAGE)" WRAPPER_SERVER_IMAGE_NAME="$(PORCH_WRAPPER_SERVER_IMAGE)" make -C func/ build-image
	IMAGE_NAME="$(TEST_GIT_SERVER_IMAGE)" make -C test/ build-image

.PHONY: dev-server
dev-server:
	docker buildx build --push --tag $(IMAGE_REPO)/$(PORCH_SERVER_IMAGE):$(IMAGE_TAG) -f ./build/Dockerfile "$(KPTDIR)"
	kubectl set image -n porch-system deployment/porch-server porch-server=$(IMAGE_REPO)/$(PORCH_SERVER_IMAGE):${IMAGE_TAG}

.PHONY: apply-dev-config
apply-dev-config:
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

.PHONY: deployment-config
deployment-config:
	rm -rf $(DEPLOYCONFIGDIR) || true
	mkdir -p $(DEPLOYCONFIGDIR)
	./scripts/create-deployment-blueprint.sh \
	  --destination "$(DEPLOYCONFIGDIR)" \
	  --server-image "$(IMAGE_REPO)/$(PORCH_SERVER_IMAGE):$(IMAGE_TAG)" \
	  --controllers-image "$(IMAGE_REPO)/$(PORCH_CONTROLLERS_IMAGE):$(IMAGE_TAG)" \
	  --function-image "$(IMAGE_REPO)/$(PORCH_FUNCTION_RUNNER_IMAGE):$(IMAGE_TAG)" \
	  --wrapper-server-image "$(IMAGE_REPO)/$(PORCH_WRAPPER_SERVER_IMAGE):$(IMAGE_TAG)" \
	  --enabled-reconcilers "$(ENABLED_RECONCILERS)" \
	  --project "$(GCP_PROJECT_ID)"

.PHONY: deploy
deploy: deployment-config
	kubectl apply -R -f $(DEPLOYCONFIGDIR)

.PHONY: push-and-deploy
push-and-deploy: push-images deploy

# Builds deployment config without configuring GCP workload identity for
# Porch server. This is sufficient for working with GitHub repositories.
# Workload identity is currently required for Porch to integrate with GCP
# Container and Artifact Registries; for those use cases, use the make
# targets without the `-no-sa` suffix (i.e. `deployment-config`,
# `push-and-deploy` etc.)
.PHONY: deployment-config-no-sa
deployment-config-no-sa:
	mkdir -p $(DEPLOYCONFIG_NO_SA_DIR)
	find $(DEPLOYCONFIG_NO_SA_DIR) ! -name 'resourcegroup.yaml' -type f -exec rm -f {} +
	./scripts/create-deployment-blueprint.sh \
	  --destination "$(DEPLOYCONFIG_NO_SA_DIR)" \
	  --server-image "$(IMAGE_REPO)/$(PORCH_SERVER_IMAGE):$(IMAGE_TAG)" \
	  --controllers-image "$(IMAGE_REPO)/$(PORCH_CONTROLLERS_IMAGE):$(IMAGE_TAG)" \
	  --function-image "$(IMAGE_REPO)/$(PORCH_FUNCTION_RUNNER_IMAGE):$(IMAGE_TAG)" \
	  --wrapper-server-image "$(IMAGE_REPO)/$(PORCH_WRAPPER_SERVER_IMAGE):$(IMAGE_TAG)" \
	  --enabled-reconcilers "$(ENABLED_RECONCILERS)"

.PHONY: deploy-no-sa
deploy-no-sa: deployment-config-no-sa
	kubectl apply -R -f $(DEPLOYCONFIG_NO_SA_DIR)

.PHONY: push-and-deploy-no-sa
push-and-deploy-no-sa: push-images deploy-no-sa

KIND_CONTEXT_NAME ?= kind

.PHONY: run-in-kind
run-in-kind: IMAGE_REPO=porch-kind
run-in-kind:
	make build-images
	kind load docker-image $(IMAGE_REPO)/$(PORCH_SERVER_IMAGE):${IMAGE_TAG} -n ${KIND_CONTEXT_NAME}
	kind load docker-image $(IMAGE_REPO)/$(PORCH_CONTROLLERS_IMAGE):${IMAGE_TAG} -n ${KIND_CONTEXT_NAME}
	kind load docker-image $(IMAGE_REPO)/$(PORCH_FUNCTION_RUNNER_IMAGE):${IMAGE_TAG} -n ${KIND_CONTEXT_NAME}
	kind load docker-image $(IMAGE_REPO)/$(PORCH_WRAPPER_SERVER_IMAGE):${IMAGE_TAG} -n ${KIND_CONTEXT_NAME}
	kind load docker-image $(IMAGE_REPO)/$(TEST_GIT_SERVER_IMAGE):${IMAGE_TAG} -n ${KIND_CONTEXT_NAME}
	make deployment-config
	KUBECONFIG=$(KUBECONFIG) kubectl apply --wait --recursive --filename $(DEPLOYCONFIGDIR)
	KUBECONFIG=$(KUBECONFIG) kubectl rollout status deployment function-runner --namespace porch-system
	KUBECONFIG=$(KUBECONFIG) kubectl rollout status deployment porch-controllers --namespace porch-system
	KUBECONFIG=$(KUBECONFIG) kubectl rollout status deployment porch-server --namespace porch-system

.PHONY: deployment-config-kpt
deployment-config-kpt:
	rm -rf $(DEPLOYKPTCONFIGDIR) || true
	mkdir -p $(DEPLOYKPTCONFIGDIR)
	./scripts/create-deployment-kpt.sh \
	  --destination $(DEPLOYKPTCONFIGDIR) \
      --server-image "$(IMAGE_REPO)/$(PORCH_SERVER_IMAGE):$(IMAGE_TAG)" \
	  --controllers-image "$(IMAGE_REPO)/$(PORCH_CONTROLLERS_IMAGE):$(IMAGE_TAG)" \
	  --function-image "$(IMAGE_REPO)/$(PORCH_FUNCTION_RUNNER_IMAGE):$(IMAGE_TAG)" \
	  --wrapper-server-image "$(IMAGE_REPO)/$(PORCH_WRAPPER_SERVER_IMAGE):$(IMAGE_TAG)" \
	  --test-git-server-image "$(IMAGE_REPO)/$(TEST_GIT_SERVER_IMAGE):${IMAGE_TAG}" \
	  --enabled-reconcilers "$(ENABLED_RECONCILERS)" \
	  --kind-context "$(KIND_CONTEXT_NAME)"

.PHONY: run-in-kind-kpt
run-in-kind-kpt: IMAGE_REPO=porch-kind
run-in-kind-kpt:
  ifeq ($(SKIP_IMG_BUILD), false)
	make build-images; 
  endif
	make deployment-config-kpt

PKG=gitea-dev
.PHONY: deploy-gitea-dev-pkg
deploy-gitea-dev-pkg:
	PKG=gitea-dev
	rm -rf $(DEPLOYKPTCONFIGDIR)/${PKG} || true
	mkdir -p $(DEPLOYKPTCONFIGDIR)/${PKG}
	./scripts/install-local-kpt-pkg.sh \
	  --destination $(DEPLOYKPTCONFIGDIR) \
	  --pkg ${PKG} \
	  --kubeconfig $(KUBECONFIG)

.PHONY: vulncheck
vulncheck: build
	# Scan the source
	GOFLAGS= go run golang.org/x/vuln/cmd/govulncheck@latest ./...
