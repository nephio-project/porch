# Integration test targets for Repository Controller

.PHONY: test-integration
test-integration: setup-envtest ## Run integration tests only
	export KUBEBUILDER_ASSETS=$$(setup-envtest use -p path) && cd pkg/controllers/repository/integration && ginkgo -v --trace

.PHONY: test-integration-watch
test-integration-watch: setup-envtest ## Run integration tests in watch mode
	export KUBEBUILDER_ASSETS=$$(setup-envtest use -p path) && cd pkg/controllers/repository/integration && ginkgo watch -v --trace

.PHONY: test-unit
test-unit: ## Run unit tests only
	cd pkg/controllers/repository && go test -v .

.PHONY: test-all
test-all: test-unit test-integration ## Run all tests (unit + integration)

.PHONY: setup-envtest
setup-envtest: ## Setup test environment binaries
	@which setup-envtest > /dev/null || go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest
	@which ginkgo > /dev/null || go install github.com/onsi/ginkgo/v2/ginkgo@latest
	@setup-envtest use --bin-dir ./testbin

.PHONY: clean-testbin
clean-testbin: ## Clean test binaries
	rm -rf ./testbin

.PHONY: help
help: ## Display this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)