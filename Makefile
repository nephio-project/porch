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
BUILDDIR=$(CURDIR)/.build
CACHEDIR=$(CURDIR)/.cache
export DEPLOYPORCHCONFIGDIR ?= $(BUILDDIR)/deploy
DEPLOYKPTCONFIGDIR=$(BUILDDIR)/kpt_pkgs
PORCHDIR=$(abspath $(CURDIR))

# This includes the following targets:
#   test, unit, unit-clean,
#   gosec, lint,
#   fmt, vet
include default-go.mk

# This includes the 'help' target that prints out all targets with their descriptions organized by categories
include default-help.mk

KIND_CONTEXT_NAME ?= porch-test
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
SKIP_PORCHSERVER_BUILD ?= false
SKIP_CONTROLLER_BUILD ?= false

# Only enable a subset of reconcilers in porch controllers by default. Use the RECONCILERS
# env variable to specify a specific list of reconcilers or use
# RECONCILERS=* to enable all known reconcilers.
ALL_RECONCILERS="packagevariants,packagevariantsets"
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
	rm -rf $(BUILDDIR)/data/etcd || true
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
generate: generate-api ## Generate CRDs, other K8s manifests and helper go code
	@for f in $(MODULES); do (cd $$f; echo "Generating $$f"; go generate -v ./...) || exit 1; done

.PHONY: tidy
tidy:
	@for f in $(MODULES); do (cd $$f; echo "Tidying $$f"; go mod tidy) || exit 1; done

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
	KUBECONFIG=$(CURDIR)/deployments/local/kubeconfig kubectl apply -f deployments/local/localconfig.yaml
	KUBECONFIG=$(CURDIR)/deployments/local/kubeconfig kubectl apply -f api/porchconfig/v1alpha1/
	KUBECONFIG=$(CURDIR)/deployments/local/kubeconfig kubectl apply -f internal/api/porchinternal/v1alpha1/
	$(PORCH) \
	--secure-port 9443 \
	--standalone-debug-mode \
	--kubeconfig="$(CURDIR)/deployments/local/kubeconfig" \
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

.PHONY: run-in-kind-no-server
run-in-kind-no-server: IMAGE_REPO=porch-kind
run-in-kind-no-server: IMAGE_TAG=test
run-in-kind-no-server: SKIP_PORCHSERVER_BUILD=true
run-in-kind-no-server: load-images-to-kind deployment-config-no-server deploy-current-config ## Build and deploy porch without the porch-server into a kind cluster

.PHONY: run-in-kind-no-controller
run-in-kind-no-controller: IMAGE_REPO=porch-kind
run-in-kind-no-controller: IMAGE_TAG=test
run-in-kind-no-controller: SKIP_CONTROLLER_BUILD=true
run-in-kind-no-controller: load-images-to-kind deployment-config-no-controller deploy-current-config ## Build and deploy porch without the controllers into a kind cluster

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
	  --enabled-reconcilers "$(ENABLED_RECONCILERS)"

.PHONY: deployment-config-no-server
deployment-config-no-server: deployment-config ## Generate a deployment kpt package that contains all of porch except the porch-server into $(DEPLOYPORCHCONFIGDIR)
	./scripts/remove-porch-server-from-deployment-config.sh

.PHONY: deployment-config-no-controller
deployment-config-no-controller: deployment-config ## Generate a deployment kpt package that contains all of porch except the controllers into $(DEPLOYPORCHCONFIGDIR)
	./scripts/remove-controller-from-deployment-config.sh

.PHONY: load-images-to-kind
load-images-to-kind: ## Build porch images and load them into a kind cluster
  ifeq ($(SKIP_IMG_BUILD), false)
  # only build test-git-server & function-runner if they are not already loaded into kind
	@if ! docker exec "${KIND_CONTEXT_NAME}-control-plane" crictl images | grep -q "$(IMAGE_REPO)/$(TEST_GIT_SERVER_IMAGE)  *${IMAGE_TAG} " ; then \
		echo "Building $(IMAGE_REPO)/$(TEST_GIT_SERVER_IMAGE):${IMAGE_TAG}" ; \
		IMAGE_NAME="$(TEST_GIT_SERVER_IMAGE)" make -C test/ build-image ; \
		kind load docker-image $(IMAGE_REPO)/$(TEST_GIT_SERVER_IMAGE):${IMAGE_TAG} -n ${KIND_CONTEXT_NAME} ; \
	else \
		echo "Skipping building $(IMAGE_REPO)/$(TEST_GIT_SERVER_IMAGE):${IMAGE_TAG} as it is already loaded into kind" ; \
	fi
	@if ! docker exec "${KIND_CONTEXT_NAME}-control-plane" crictl images | grep -q "$(IMAGE_REPO)/$(PORCH_FUNCTION_RUNNER_IMAGE)  *${IMAGE_TAG} " ; then \
		echo "Building $(IMAGE_REPO)/$(PORCH_FUNCTION_RUNNER_IMAGE):${IMAGE_TAG}" ; \
		IMAGE_NAME="$(PORCH_FUNCTION_RUNNER_IMAGE)" WRAPPER_SERVER_IMAGE_NAME="$(PORCH_WRAPPER_SERVER_IMAGE)" make -C func/ build-image ; \
		kind load docker-image $(IMAGE_REPO)/$(PORCH_FUNCTION_RUNNER_IMAGE):${IMAGE_TAG} -n ${KIND_CONTEXT_NAME} ; \
		kind load docker-image $(IMAGE_REPO)/$(PORCH_WRAPPER_SERVER_IMAGE):${IMAGE_TAG} -n ${KIND_CONTEXT_NAME} ; \
	else \
		echo "Skipping building $(IMAGE_REPO)/$(PORCH_FUNCTION_RUNNER_IMAGE):${IMAGE_TAG} as it is already loaded into kind" ; \
	fi
    # NOTE: SKIP_PORCHSERVER_BUILD must be evaluated at runtime, hence the shell conditional (if) here
	@if [ "$(SKIP_PORCHSERVER_BUILD)" = "false"	]; then \
		echo "Building $(IMAGE_REPO)/$(PORCH_SERVER_IMAGE):${IMAGE_TAG}" ; \
		docker buildx build --load --tag $(IMAGE_REPO)/$(PORCH_SERVER_IMAGE):$(IMAGE_TAG) -f ./build/Dockerfile "$(PORCHDIR)" ; \
		kind load docker-image $(IMAGE_REPO)/$(PORCH_SERVER_IMAGE):${IMAGE_TAG} -n ${KIND_CONTEXT_NAME} ; \
	fi
	@if [ "$(SKIP_CONTROLLER_BUILD)" = "false"	]; then \
		echo "Building $(IMAGE_REPO)/$(PORCH_CONTROLLERS_IMAGE):${IMAGE_TAG}" ; \
		IMAGE_NAME="$(PORCH_CONTROLLERS_IMAGE)" make -C controllers/ build-image ; \
		kind load docker-image $(IMAGE_REPO)/$(PORCH_CONTROLLERS_IMAGE):${IMAGE_TAG} -n ${KIND_CONTEXT_NAME} ; \
	fi

  else
	kind load docker-image $(IMAGE_REPO)/$(TEST_GIT_SERVER_IMAGE):${IMAGE_TAG} -n ${KIND_CONTEXT_NAME} ; \
	kind load docker-image $(IMAGE_REPO)/$(PORCH_FUNCTION_RUNNER_IMAGE):${IMAGE_TAG} -n ${KIND_CONTEXT_NAME} ; \
	kind load docker-image $(IMAGE_REPO)/$(PORCH_WRAPPER_SERVER_IMAGE):${IMAGE_TAG} -n ${KIND_CONTEXT_NAME} ; \
	kind load docker-image $(IMAGE_REPO)/$(PORCH_SERVER_IMAGE):${IMAGE_TAG} -n ${KIND_CONTEXT_NAME}
	kind load docker-image $(IMAGE_REPO)/$(PORCH_CONTROLLERS_IMAGE):${IMAGE_TAG} -n ${KIND_CONTEXT_NAME}
  endif

.PHONY: deploy-current-config
deploy-current-config: ## Deploy the configuration that is currently in $(DEPLOYPORCHCONFIGDIR)
	kpt fn render $(DEPLOYPORCHCONFIGDIR)
	kpt live init $(DEPLOYPORCHCONFIGDIR) --name porch --namespace porch-system --inventory-id porch-test || true
	kpt live apply --inventory-policy=adopt --server-side --force-conflicts $(DEPLOYPORCHCONFIGDIR)
	@kubectl rollout status deployment function-runner --namespace porch-system 2>/dev/null || true
	@kubectl rollout status deployment porch-controllers --namespace porch-system 2>/dev/null || true
	@kubectl rollout status deployment porch-server --namespace porch-system 2>/dev/null || true
	@echo "Done."

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
	E2E=1 go test -v -failfast ./test/e2e/cli

.PHONY: test-e2e-clean
test-e2e-clean: ## Run end-to-end tests aginst a newly deployed porch in a newly created kind cluster
	./scripts/clean-e2e-test.sh
