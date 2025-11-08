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

# Security scanning tools

##@ Security

.PHONY: gosec
gosec: ## Inspect the source code for security problems by scanning the Go Abstract Syntax Tree
ifeq ($(CONTAINER_RUNNABLE), 0)
	$(RUN_CONTAINER_COMMAND) securego/gosec:latest \
		-fmt=html \
		-out=gosec-results.html \
		-stdout -verbose=text \
		-exclude-dir=generated \
		-exclude-dir=test \
		-exclude-dir=third_party \
		-exclude-dir=examples \
		-exclude-generated \
		-severity=medium \
		-exclude=G401,G501,G505,G304 ./...
else
		gosec -fmt=html -out=gosec-results.html -stdout -verbose=text \
		-exclude-dir=generated \
		-exclude-dir=third_party \
		-exclude-dir=test \
		-exclude-dir=examples \
		-exclude-generated -severity=medium -exclude=G401,G501,G505,G304 ./...
endif

.PHONY: gosec-sarif
gosec-sarif:  ## Generate SARIF security report
ifeq ($(CONTAINER_RUNNABLE), 0)
	$(RUN_CONTAINER_COMMAND) securego/gosec:latest \
		-fmt=sarif \
		-out=gosec-results.sarif \
		-stdout -verbose=text \
		-exclude-dir=generated \
		-exclude-dir=test \
		-exclude-dir=third_party \
		-exclude-dir=examples \
		-exclude-generated \
		-severity=medium \
		-exclude=G401,G501,G505,G304 ./...
else
		gosec -fmt=sarif -out=gosec-results.sarif -stdout -verbose=text \
		-exclude-dir=generated \
		-exclude-dir=third_party \
		-exclude-dir=test \
		-exclude-dir=examples \
		-exclude-generated -severity=medium -exclude=G401,G501,G505,G304 ./...
endif