#  Copyright 2025 The Nephio Authors.
#
#  Licensed under the Apache License, Version 2.0 (the "License");
#  you may not use this file except in compliance with the License.
#  You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
#  Unless required by applicable law or agreed to in writing, software
#  distributed under the License is distributed on an "AS IS" BASIS,
#  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#  See the License for the specific language governing permissions and
#  limitations under the License.

# Deployment targets and configuration

# Skip flags
export SKIP_IMG_BUILD ?= false
export SKIP_PORCHSERVER_BUILD ?= false
export SKIP_CONTROLLER_BUILD ?= false
export SKIP_LOCAL_GIT ?= false

# Reconciler configuration
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

##@ Build and deploy porch for development and testing

.PHONY: run-in-kind
run-in-kind: IMAGE_REPO=porch-kind ## Build and deploy porch into a kind cluster
run-in-kind: IMAGE_TAG=test
run-in-kind: load-images-to-kind deployment-config deploy-current-config 

.PHONY: run-in-kind-db-cache
run-in-kind-db-cache: IMAGE_REPO=porch-kind ## Build and deploy porch into a kind cluster with postgres backend
run-in-kind-db-cache: IMAGE_TAG=test
run-in-kind-db-cache: load-images-to-kind deployment-config-db-cache deploy-current-config 

.PHONY: run-in-kind-no-git
run-in-kind-no-git: IMAGE_REPO=porch-kind ## Build and deploy porch into a kind cluster without mock git server
run-in-kind-no-git: IMAGE_TAG=test
run-in-kind-no-git: SKIP_LOCAL_GIT=true
run-in-kind-no-git: load-images-to-kind deployment-config deploy-current-config 

.PHONY: run-in-kind-db-cache-no-git
run-in-kind-db-cache-no-git: IMAGE_REPO=porch-kind ## Build and deploy porch into a kind cluster with postgres backend without mock git server
run-in-kind-db-cache-no-git: IMAGE_TAG=test
run-in-kind-db-cache-no-git: SKIP_LOCAL_GIT=true
run-in-kind-db-cache-no-git: load-images-to-kind deployment-config-db-cache deploy-current-config 

.PHONY: run-in-kind-no-server
run-in-kind-no-server: IMAGE_REPO=porch-kind ## Build and deploy porch without the porch-server into a kind cluster
run-in-kind-no-server: IMAGE_TAG=test
run-in-kind-no-server: SKIP_PORCHSERVER_BUILD=true
run-in-kind-no-server: load-images-to-kind deployment-config-no-server deploy-current-config 

.PHONY: run-in-kind-db-cache-no-server
run-in-kind-db-cache-no-server: IMAGE_REPO=porch-kind ## Build and deploy porch into a kind cluster with postgres backend without the porch-server
run-in-kind-db-cache-no-server: IMAGE_TAG=test
run-in-kind-db-cache-no-server: load-images-to-kind deployment-config-db-cache deployment-config-no-server deploy-current-config 

.PHONY: run-in-kind-no-controller
run-in-kind-no-controller: IMAGE_REPO=porch-kind ## Build and deploy porch without the controllers into a kind cluster
run-in-kind-no-controller: IMAGE_TAG=test
run-in-kind-no-controller: SKIP_CONTROLLER_BUILD=true
run-in-kind-no-controller: load-images-to-kind deployment-config-no-controller deploy-current-config 

.PHONY: run-in-kind-db-cache-no-controller
run-in-kind-db-cache-no-controller: IMAGE_REPO=porch-kind ## Build and deploy porch without the controllers into a kind cluster with postgres backend
run-in-kind-db-cache-no-controller: IMAGE_TAG=test
run-in-kind-db-cache-no-controller: SKIP_CONTROLLER_BUILD=true
run-in-kind-db-cache-no-controller: load-images-to-kind deployment-config-db-cache deployment-config-no-controller deploy-current-config 

.PHONY: destroy
destroy: ## Deletes all porch resources installed by the last run-in-kind-* command
	kpt live destroy $(DEPLOYPORCHCONFIGDIR)

.PHONY: deployment-config 
deployment-config: ## Generate a porch deployment kpt package into $(DEPLOYPORCHCONFIGDIR)
	./scripts/create-deployment-config.sh

.PHONY: deployment-config-no-server
deployment-config-no-server: deployment-config ## Generate a deployment kpt package that contains all of porch except the porch-server into $(DEPLOYPORCHCONFIGDIR)
	./scripts/remove-porch-server-from-deployment-config.sh

.PHONY: deployment-config-no-controller
deployment-config-no-controller: deployment-config ## Generate a deployment kpt package that contains all of porch except the controllers into $(DEPLOYPORCHCONFIGDIR)
	./scripts/remove-controller-from-deployment-config.sh

.PHONY: deployment-config-db-cache
deployment-config-db-cache: deployment-config ## Generate deployment config with postgres database cache
	./scripts/upsert-db-cache-data.sh

.PHONY: load-images-to-kind
load-images-to-kind: ## Build porch images and load them into a kind cluster
	./scripts/load-images-to-kind.sh

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
reload-function-runner: IMAGE_REPO=porch-kind ## Rebuild and reload function-runner in kind cluster
reload-function-runner: IMAGE_TAG:=test
reload-function-runner:
	./scripts/reload-component.sh function-runner

.PHONY: reload-server
reload-server: IMAGE_REPO=porch-kind ## Rebuild and reload porch-server in kind cluster
reload-server: IMAGE_TAG:=test
reload-server:
	./scripts/reload-component.sh server

.PHONY: reload-controllers
reload-controllers: IMAGE_REPO=porch-kind ## Rebuild and reload porch-controllers in kind cluster
reload-controllers: IMAGE_TAG:=test
reload-controllers:
	./scripts/reload-component.sh controllers

PKG=gitea-dev
.PHONY: deploy-gitea-dev-pkg
deploy-gitea-dev-pkg: ## Deploy gitea development package
	PKG=gitea-dev
	rm -rf $(DEPLOYKPTCONFIGDIR)/${PKG} || true
	mkdir -p $(DEPLOYKPTCONFIGDIR)/${PKG}
	./scripts/install-local-kpt-pkg.sh \
	  --destination $(DEPLOYKPTCONFIGDIR) \
	  --pkg ${PKG} \
	  --kubeconfig $(KUBECONFIG)