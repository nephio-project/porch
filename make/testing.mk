#  Copyright 2025-2026 The Nephio Authors.
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

# Testing tools and targets

TEST_COVERAGE_FILE=coverage.out
TEST_COVERAGE_HTML_FILE=coverage_unit.html
TEST_COVERAGE_FUNC_FILE=func_coverage.out
TEST_OUTPUT_LOG_FILE=test_output.log
TEST_COVERAGE_TMP_DIR := $(shell mktemp --directory)

##@ Testing

.PHONY: unit test
unit: test

test: ## Run unit tests (go test)
ifeq ($(CONTAINER_RUNNABLE), 0)
	$(RUN_CONTAINER_COMMAND) -e CONTAINER_RUNNABLE golang:1.25.6-bookworm \
	sh -c "useradd -m -s /bin/sh porch && \
	         mkdir -p ${TEST_COVERAGE_TMP_DIR} && chown porch:porch ${TEST_COVERAGE_TMP_DIR} && \
	         su porch -c 'export TEST_COVERAGE_TMP_DIR=${TEST_COVERAGE_TMP_DIR}; \
	                       export PORCHDIR=${PORCHDIR}; \
	                       git config --global user.name test; \
	                       git config --global user.email test@nephio.org; \
	                       go test -short ./... -v -coverprofile=${TEST_COVERAGE_TMP_DIR}/${TEST_COVERAGE_FILE} && \
	                       go tool cover -html=${TEST_COVERAGE_TMP_DIR}/${TEST_COVERAGE_FILE} -o ${TEST_COVERAGE_TMP_DIR}/${TEST_COVERAGE_HTML_FILE} && \
	                       go tool cover -func=${TEST_COVERAGE_TMP_DIR}/${TEST_COVERAGE_FILE} -o ${TEST_COVERAGE_TMP_DIR}/${TEST_COVERAGE_FUNC_FILE}' && \
	         cp ${TEST_COVERAGE_TMP_DIR}/${TEST_COVERAGE_HTML_FILE} ${TEST_COVERAGE_TMP_DIR}/${TEST_COVERAGE_FILE} ${TEST_COVERAGE_TMP_DIR}/${TEST_COVERAGE_FUNC_FILE} ."
else
	go test -short ./... -v -coverprofile=${TEST_COVERAGE_FILE}
	go tool cover -html=${TEST_COVERAGE_FILE} -o ${TEST_COVERAGE_HTML_FILE}
	go tool cover -func=${TEST_COVERAGE_FILE} -o ${TEST_COVERAGE_FUNC_FILE}
endif

.PHONY: unit-clean
unit-clean: ## Clean up the artifacts created by the unit tests
ifeq ($(CONTAINER_RUNNABLE), 0)
		$(CONTAINER_RUNTIME) system prune -f
endif
		rm -f ${TEST_COVERAGE_FILE} ${TEST_COVERAGE_HTML_FILE} ${TEST_COVERAGE_FUNC_FILE} ${TEST_OUTPUT_LOG_FILE} > /dev/null 2>&1

.PHONY: vulncheck
vulncheck: build
	# Scan the source
	GOFLAGS= go run golang.org/x/vuln/cmd/govulncheck@latest ./...

.PHONY: test-e2e
test-e2e: ## Run end-to-end tests
	E2E=1 go test -v -failfast ./test/e2e/api

.PHONY: test-e2e-cli
test-e2e-cli: ## Run cli end-to-end tests
test-e2e-cli: run-in-kind-no-git
	E2E=1 go test -v -failfast ./test/e2e/cli

.PHONY: test-e2e-cli-db-cache
test-e2e-cli-db-cache: ## Run cli end-to-end tests with db-cache
test-e2e-cli-db-cache: run-in-kind-db-cache-no-git
	E2E=1 go test -v -failfast ./test/e2e/cli

.PHONY: test-e2e-clean
test-e2e-clean: porchctl ## Run end-to-end tests against a newly deployed porch in a newly created kind cluster
	./scripts/clean-e2e-test.sh

.PHONY: test-3pp-fns
test-3pp-fns: ## Run 3pp fn-runner end-to-end tests
	TPP=1 go test -v -failfast ./test/e2e/fn_runner
