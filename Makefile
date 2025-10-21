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

# Core configuration
BUILDDIR=$(CURDIR)/.build
CACHEDIR=$(CURDIR)/.cache
export PORCHDIR=$(abspath $(CURDIR))

export DEPLOYPORCHCONFIGDIR ?= $(BUILDDIR)/deploy
DEPLOYKPTCONFIGDIR=$(BUILDDIR)/kpt_pkgs

# Image configuration
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

# Image names
export PORCH_SERVER_IMAGE ?= porch-server
export PORCH_FUNCTION_RUNNER_IMAGE ?= porch-function-runner
export PORCH_CONTROLLERS_IMAGE ?= porch-controllers
export PORCH_WRAPPER_SERVER_IMAGE ?= porch-wrapper-server
export TEST_GIT_SERVER_IMAGE ?= test-git-server

# Kind configuration
KIND_CONTEXT_NAME ?= porch-test

# Include environment file if it exists
ifneq ("$(wildcard .env)", "")
    include .env
endif

# Include module makefiles
include make/build.mk        # generate, tidy, porch, porchctl, build-images, push-images
include make/deploy.mk       # deploy, run-in-kind*, destroy, deployment-config*, load-images-to-kind, reload-*
include make/local-dev.mk    # all, network, stop, start-etcd, start-kube-apiserver, start-function-runner, run-local
include make/infra.mk        # docker-build, docker-push, help (+ container runtime detection)
include make/go.mk           # fmt, vet, lint, fix-headers, fix-all
include make/testing.mk      # test, unit, unit-clean, vulncheck, test-e2e*
include make/security.mk     # gosec, gosec-sarif
include make/mocks.mk        # install-mockery, generate-mocks, clean-mocks

.DEFAULT_GOAL := help

##@ Common Workflows

.PHONY: build
build: porch porchctl ## Build all binaries

.PHONY: check
check: fmt vet lint test ## Run all code quality checks

.PHONY: clean
clean: unit-clean clean-mocks ## Clean all build artifacts

.PHONY: dev
dev: build check ## Full development cycle (build + check)

.PHONY: quick-test
quick-test: fmt vet test ## Quick development test cycle

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

.PHONY: run-local
run-local: porch
	KUBECONFIG=$(CURDIR)/deployments/local/kubeconfig kubectl apply -f deployments/local/localconfig.yaml
	KUBECONFIG=$(CURDIR)/deployments/local/kubeconfig kubectl apply -f api/porchconfig/v1alpha1/
	KUBECONFIG=$(CURDIR)/deployments/local/kubeconfig kubectl apply -f internal/api/porchinternal/v1alpha1/
	$(PORCH) \
	--secure-port 9443 \
	--standalone-debug-mode \
	--kubeconfig="$(CURDIR)/deployments/local/kubeconfig" \
	--cache-directory="$(CACHEDIR)" \
	--function-runner 192.168.8.202:9445 \
	--repo-sync-frequency=3m \
	--max-request-body-size=6291456


# API Modules
API_MODULES = \
 api \
 internal/api \
 pkg/kpt/api \
 controllers \

.PHONY: generate-api
generate-api:
	rm -fr $(CURDIR)/api/porch/generated
	KUBE_VERBOSE=2 $(CURDIR)/scripts/generate-api.sh

.PHONY: generate
generate: generate-api ## Generate CRDs, other K8s manifests and helper go code
	@for f in $(API_MODULES); do (cd $$f; echo "Generating for $$f ..."; go generate -v ./...) || exit 1; done
	find $(CURDIR)/api/config/apiresources/bases -name 'porch.kpt.dev_*.yaml' -exec sed -i "s/^kind: CustomResourceDefinition$$/kind: PorchAPIRResourceDefinition/" {} \;

.PHONY: tidy
tidy:
	go mod tidy

.PHONY: run-jaeger
run-jaeger:
	docker run --rm --name jaeger -d -p4317:55680 -p6831:6831/udp -p16686:16686 jaegertracing/opentelemetry-all-in-one:latest

.PHONY: porch
porch:
	go build -o $(PORCH) ./cmd/porch

.PHONY: porchctl
porchctl:
	go build -ldflags="-X github.com/nephio-project/porch/cmd/porchctl/run.version=$(PORCHCTL_VERSION)" -o $(PORCHCTL) ./cmd/porchctl

.PHONY: fix-headers
fix-headers:
	../scripts/update-license.sh

.PHONY: fix-all
fix-all: fix-headers fmt tidy

.PHONY: push-images
push-images:
	docker buildx build --push --tag $(IMAGE_REPO)/$(PORCH_SERVER_IMAGE):$(IMAGE_TAG) -f ./build/Dockerfile "$(PORCHDIR)"
	IMAGE_NAME="$(PORCH_CONTROLLERS_IMAGE)" make -C controllers/ push-image
	IMAGE_NAME="$(PORCH_FUNCTION_RUNNER_IMAGE)" WRAPPER_SERVER_IMAGE_NAME="$(PORCH_WRAPPER_SERVER_IMAGE)" make -C func/ push-image
	IMAGE_NAME="$(TEST_GIT_SERVER_IMAGE)" make -C test/ push-image

.PHONY: build-images
build-images:
	docker buildx build --load --tag $(IMAGE_REPO)/$(PORCH_SERVER_IMAGE):$(IMAGE_TAG) -f ./build/Dockerfile "$(PORCHDIR)"
	IMAGE_NAME="$(PORCH_CONTROLLERS_IMAGE)" make -C controllers/ build-image
	IMAGE_NAME="$(PORCH_FUNCTION_RUNNER_IMAGE)" WRAPPER_SERVER_IMAGE_NAME="$(PORCH_WRAPPER_SERVER_IMAGE)" make -C func/ build-image
	IMAGE_NAME="$(TEST_GIT_SERVER_IMAGE)" make -C test/ build-image

.PHONY: dev-server
dev-server:
	docker buildx build --push --tag $(IMAGE_REPO)/$(PORCH_SERVER_IMAGE):$(IMAGE_TAG) -f ./build/Dockerfile "$(PORCHDIR)"
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

##@ Build and deploy porch for development and testing

.PHONY: deploy
deploy: deployment-config
	kubectl apply -R -f $(DEPLOYPORCHCONFIGDIR)

.PHONY: push-and-deploy
push-and-deploy: push-images deploy

.PHONY: run-in-kind 
run-in-kind: IMAGE_REPO=porch-kind
run-in-kind: IMAGE_TAG=test
run-in-kind: load-images-to-kind deployment-config deploy-current-config ## Build and deploy porch into a kind cluster

.PHONY: run-in-kind-db-cache
run-in-kind-db-cache: IMAGE_REPO=porch-kind
run-in-kind-db-cache: IMAGE_TAG=test
run-in-kind-db-cache: load-images-to-kind deployment-config-db-cache deploy-current-config ## Build and deploy porch into a kind cluster with postgres backend

-.PHONY: run-in-kind-no-git
run-in-kind-no-git: IMAGE_REPO=porch-kind
run-in-kind-no-git: IMAGE_TAG=test
run-in-kind-no-git: SKIP_LOCAL_GIT=true
run-in-kind-no-git: load-images-to-kind deployment-config deploy-current-config ## Build and deploy porch into a kind cluster

.PHONY: run-in-kind-db-cache-no-git
run-in-kind-db-cache-no-git: IMAGE_REPO=porch-kind
run-in-kind-db-cache-no-git: IMAGE_TAG=test
run-in-kind-db-cache-no-git: SKIP_LOCAL_GIT=true
run-in-kind-db-cache-no-git: load-images-to-kind deployment-config-db-cache deploy-current-config ## Build and deploy porch into a kind cluster

.PHONY: run-in-kind-no-server
run-in-kind-no-server: IMAGE_REPO=porch-kind
run-in-kind-no-server: IMAGE_TAG=test
run-in-kind-no-server: SKIP_PORCHSERVER_BUILD=true
run-in-kind-no-server: load-images-to-kind deployment-config-no-server deploy-current-config ## Build and deploy porch without the porch-server into a kind cluster

.PHONY: run-in-kind-db-cache-no-server
run-in-kind-db-cache-no-server: IMAGE_REPO=porch-kind
run-in-kind-db-cache-no-server: IMAGE_TAG=test
run-in-kind-db-cache-no-server: load-images-to-kind deployment-config-db-cache deployment-config-no-server deploy-current-config ## Build and deploy porch into a kind cluster with postgres backend without the porch-server

.PHONY: run-in-kind-no-controller
run-in-kind-no-controller: IMAGE_REPO=porch-kind
run-in-kind-no-controller: IMAGE_TAG=test
run-in-kind-no-controller: SKIP_CONTROLLER_BUILD=true
run-in-kind-no-controller: load-images-to-kind deployment-config-no-controller deploy-current-config ## Build and deploy porch without the controllers into a kind cluster

.PHONY: run-in-kind-db-cache-no-controller
run-in-kind-db-cache-no-controller: IMAGE_REPO=porch-kind
run-in-kind-db-cache-no-controller: IMAGE_TAG=test
run-in-kind-db-cache-no-controller: SKIP_CONTROLLER_BUILD=true
run-in-kind-db-cache-no-controller: load-images-to-kind deployment-config-db-cache deployment-config-no-controller deploy-current-config ## Build and deploy porch without the controllers into a kind cluster with postgres backend

.PHONY: destroy
destroy: ## Deletes all porch resources installed by the last run-in-kind-* command
	kpt live destroy $(DEPLOYPORCHCONFIGDIR)

.PHONY: deployment-config 
deployment-config: ## Generate a porch deployment kpt package into $(DEPLOYPORCHCONFIGDIR)
	rm -rf $(DEPLOYPORCHCONFIGDIR) || true
	mkdir -p $(DEPLOYPORCHCONFIGDIR)
	./scripts/create-deployment-blueprint.sh \
	  --destination "$(DEPLOYPORCHCONFIGDIR)" \
	  --server-image "$(IMAGE_REPO)/$(PORCH_SERVER_IMAGE):$(IMAGE_TAG)" \
	  --controllers-image "$(IMAGE_REPO)/$(PORCH_CONTROLLERS_IMAGE):$(IMAGE_TAG)" \
	  --function-image "$(IMAGE_REPO)/$(PORCH_FUNCTION_RUNNER_IMAGE):$(IMAGE_TAG)" \
	  --wrapper-server-image "$(IMAGE_REPO)/$(PORCH_WRAPPER_SERVER_IMAGE):$(IMAGE_TAG)" \
	  --enabled-reconcilers "$(ENABLED_RECONCILERS)" \
	  $(if $(PORCH_GCR_PREFIX_URL),--gcr-image-prefix "$(PORCH_GCR_PREFIX_URL)")

.PHONY: deployment-config-no-server
deployment-config-no-server: deployment-config ## Generate a deployment kpt package that contains all of porch except the porch-server into $(DEPLOYPORCHCONFIGDIR)
	./scripts/remove-porch-server-from-deployment-config.sh

.PHONY: deployment-config-no-controller
deployment-config-no-controller: deployment-config ## Generate a deployment kpt package that contains all of porch except the controllers into $(DEPLOYPORCHCONFIGDIR)
	./scripts/remove-controller-from-deployment-config.sh

.PHONY: deployment-config-db-cache
deployment-config-db-cache: deployment-config
	./scripts/upsert-db-cache-data.sh

.PHONY: load-images-to-kind
load-images-to-kind: ## Build porch images and load them into a kind cluster
ifeq ($(SKIP_IMG_BUILD), false)
# only build test-git-server & function-runner if they are not already loaded into kind
	@if [ "$(SKIP_LOCAL_GIT)" = "false" ] && ! docker exec "${KIND_CONTEXT_NAME}-control-plane" crictl images | grep -q "$(IMAGE_REPO)/$(TEST_GIT_SERVER_IMAGE)  *${IMAGE_TAG}"; then \
		echo "Building $(IMAGE_REPO)/$(TEST_GIT_SERVER_IMAGE):${IMAGE_TAG}"; \
		IMAGE_NAME="$(TEST_GIT_SERVER_IMAGE)" make -C test/ build-image && \
		kind load docker-image $(IMAGE_REPO)/$(TEST_GIT_SERVER_IMAGE):${IMAGE_TAG} -n ${KIND_CONTEXT_NAME}; \
	else \
		echo "Skipping building and loading $(IMAGE_REPO)/$(TEST_GIT_SERVER_IMAGE):${IMAGE_TAG}"; \
	fi
	@if ! docker exec "${KIND_CONTEXT_NAME}-control-plane" crictl images | grep -q "$(IMAGE_REPO)/$(PORCH_FUNCTION_RUNNER_IMAGE)  *${IMAGE_TAG} " ; then \
		echo "Building $(IMAGE_REPO)/$(PORCH_FUNCTION_RUNNER_IMAGE):${IMAGE_TAG}" ; \
		IMAGE_NAME="$(PORCH_FUNCTION_RUNNER_IMAGE)" WRAPPER_SERVER_IMAGE_NAME="$(PORCH_WRAPPER_SERVER_IMAGE)" make -C func/ build-image && \
		kind load docker-image $(IMAGE_REPO)/$(PORCH_FUNCTION_RUNNER_IMAGE):${IMAGE_TAG} -n ${KIND_CONTEXT_NAME} && \
		kind load docker-image $(IMAGE_REPO)/$(PORCH_WRAPPER_SERVER_IMAGE):${IMAGE_TAG} -n ${KIND_CONTEXT_NAME} ; \
	else \
		echo "Skipping building $(IMAGE_REPO)/$(PORCH_FUNCTION_RUNNER_IMAGE):${IMAGE_TAG} as it is already loaded into kind" ; \
	fi
# NOTE: SKIP_PORCHSERVER_BUILD must be evaluated at runtime, hence the shell conditional (if) here
	@if [ "$(SKIP_PORCHSERVER_BUILD)" = "false" ]; then \
		echo "Building $(IMAGE_REPO)/$(PORCH_SERVER_IMAGE):${IMAGE_TAG}" ; \
		docker buildx build --load --tag $(IMAGE_REPO)/$(PORCH_SERVER_IMAGE):$(IMAGE_TAG) -f ./build/Dockerfile "$(PORCHDIR)" && \
		kind load docker-image $(IMAGE_REPO)/$(PORCH_SERVER_IMAGE):${IMAGE_TAG} -n ${KIND_CONTEXT_NAME} && \
		kubectl delete deployment -n porch-system --ignore-not-found=true porch-server ; \
	fi
	@if [ "$(SKIP_CONTROLLER_BUILD)" = "false" ]; then \
		echo "Building $(IMAGE_REPO)/$(PORCH_CONTROLLERS_IMAGE):${IMAGE_TAG}" ; \
		IMAGE_NAME="$(PORCH_CONTROLLERS_IMAGE)" make -C controllers/ build-image && \
		kind load docker-image $(IMAGE_REPO)/$(PORCH_CONTROLLERS_IMAGE):${IMAGE_TAG} -n ${KIND_CONTEXT_NAME} && \
		kubectl delete deployment -n porch-system --ignore-not-found=true porch-controllers ; \
	fi

else
	@if [ "$(SKIP_LOCAL_GIT)" = "false" ]; then \
		kind load docker-image $(IMAGE_REPO)/$(TEST_GIT_SERVER_IMAGE):${IMAGE_TAG} -n ${KIND_CONTEXT_NAME}; \
	fi
	kind load docker-image $(IMAGE_REPO)/$(PORCH_FUNCTION_RUNNER_IMAGE):${IMAGE_TAG} -n ${KIND_CONTEXT_NAME}
	kind load docker-image $(IMAGE_REPO)/$(PORCH_WRAPPER_SERVER_IMAGE):${IMAGE_TAG} -n ${KIND_CONTEXT_NAME}
	kind load docker-image $(IMAGE_REPO)/$(PORCH_SERVER_IMAGE):${IMAGE_TAG} -n ${KIND_CONTEXT_NAME}
	kind load docker-image $(IMAGE_REPO)/$(PORCH_CONTROLLERS_IMAGE):${IMAGE_TAG} -n ${KIND_CONTEXT_NAME}
endif

.PHONY: deploy-current-config
deploy-current-config: ## Deploy the configuration that is currently in $(DEPLOYPORCHCONFIGDIR)
	kpt fn render $(DEPLOYPORCHCONFIGDIR)
	kpt live init $(DEPLOYPORCHCONFIGDIR) --name porch --namespace porch-system --inventory-id nephio || true
	kpt live apply --inventory-policy=adopt --server-side --force-conflicts $(DEPLOYPORCHCONFIGDIR)
	@kubectl rollout status deployment function-runner --namespace porch-system 2>/dev/null || true
	@kubectl rollout status deployment porch-controllers --namespace porch-system 2>/dev/null || true
	@kubectl rollout status deployment porch-server --namespace porch-system 2>/dev/null || true
	@echo "Done."

.PHONY: reload-function-runner
reload-function-runner: IMAGE_REPO=porch-kind
reload-function-runner: IMAGE_TAG:=test
reload-function-runner:
	echo "Building $(IMAGE_REPO)/$(PORCH_FUNCTION_RUNNER_IMAGE):${IMAGE_TAG}"
	IMAGE_NAME="$(PORCH_FUNCTION_RUNNER_IMAGE)" WRAPPER_SERVER_IMAGE_NAME="$(PORCH_WRAPPER_SERVER_IMAGE)" make -C func/ build-image
	kind load docker-image $(IMAGE_REPO)/$(PORCH_FUNCTION_RUNNER_IMAGE):${IMAGE_TAG} $(IMAGE_REPO)/$(PORCH_WRAPPER_SERVER_IMAGE):${IMAGE_TAG} -n ${KIND_CONTEXT_NAME}
	kubectl rollout restart -n porch-system deployment/function-runner
	@kubectl rollout status deployment function-runner -n porch-system 2>/dev/null || true

.PHONY: reload-server
reload-server: IMAGE_REPO=porch-kind
reload-server: IMAGE_TAG:=test
reload-server:
	echo "Building $(IMAGE_REPO)/$(PORCH_SERVER_IMAGE):${IMAGE_TAG}"
	docker build --load --tag $(IMAGE_REPO)/$(PORCH_SERVER_IMAGE):${IMAGE_TAG} -f ./build/Dockerfile "$(PORCHDIR)"
	kind load docker-image $(IMAGE_REPO)/$(PORCH_SERVER_IMAGE):${IMAGE_TAG} -n ${KIND_CONTEXT_NAME}
	kubectl rollout restart -n porch-system deployment/porch-server
	@kubectl rollout status deployment porch-server -n porch-system 2>/dev/null || true

.PHONY: reload-controllers
reload-controllers: IMAGE_REPO=porch-kind
reload-controllers: IMAGE_TAG:=test
reload-controllers:
	echo "Building $(IMAGE_REPO)/$(PORCH_CONTROLLERS_IMAGE):${IMAGE_TAG}"
	IMAGE_NAME="$(PORCH_CONTROLLERS_IMAGE)" make -C controllers/ build-image
	kind load docker-image $(IMAGE_REPO)/$(PORCH_CONTROLLERS_IMAGE):${IMAGE_TAG} -n ${KIND_CONTEXT_NAME}
	kubectl rollout restart -n porch-system deployment/porch-controllers
	@kubectl rollout status deployment porch-controllers -n porch-system 2>/dev/null || true

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

##@ Testing

.PHONY: vulncheck
vulncheck: build
	# Scan the source
	GOFLAGS= go run golang.org/x/vuln/cmd/govulncheck@latest ./...

.PHONY: test-e2e
test-e2e: ## Run end-to-end tests
	E2E=1 go test -v -failfast ./test/e2e

.PHONY: test-e2e-cli
test-e2e-cli: ## Run cli end-to-end tests
test-e2e-cli: run-in-kind-no-git
	E2E=1 go test -v -failfast ./test/e2e/cli

.PHONY: test-e2e-cli-db-cache
test-e2e-cli-db-cache: ## Run cli end-to-end tests
test-e2e-cli-db-cache: run-in-kind-db-cache-no-git
	E2E=1 go test -v -failfast ./test/e2e/cli

.PHONY: test-e2e-clean
test-e2e-clean: porchctl ## Run end-to-end tests against a newly deployed porch in a newly created kind cluster
	./scripts/clean-e2e-test.sh

.PHONY: update-kube-apiserver-vendoring
update-kube-apiserver-vendoring:
	./scripts/update-kube-apiserver-vendoring.sh
