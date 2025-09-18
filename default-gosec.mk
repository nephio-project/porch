#  Copyright 2023-2025 The Nephio Authors.
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

GIT_ROOT_DIR ?= $(dir $(lastword $(MAKEFILE_LIST)))
include $(GIT_ROOT_DIR)/detect-container-runtime.mk

# Install link at https://github.com/securego/gosec#install if not running inside a container

gosec: ## Inspect the source code for security problems by scanning the Go AST
ifeq ($(CONTAINER_RUNNABLE), 0)
	$(RUN_CONTAINER_COMMAND) securego/gosec:latest \
		sh -c "\
			# Run Gosec once in JSON format \
			gosec -fmt=json -out=gosec-results.json \
			      -exclude-dir=generated -exclude-dir=test -exclude-dir=third_party -exclude-dir=examples -exclude-generated \
			      -severity=medium -exclude=G401,G501,G505,G304 ./... && \
			# Generate HTML from JSON \
			gosec -fmt=html -out=gosec-results.html -f=gosec-results.json && \
			# Generate text output to stdout from JSON \
			gosec -fmt=text -stdout -verbose=text -f=gosec-results.json && \
			# Generate SARIF for GitHub Security Tab \
			gosec -fmt=sarif -out=gosec-results.sarif -f=gosec-results.json"
else
	# Run Gosec once in JSON format
	gosec -fmt=json -out=gosec-results.json \
	      -exclude-dir=generated -exclude-dir=test -exclude-dir=third_party -exclude-dir=examples -exclude-generated \
	      -severity=medium -exclude=G401,G501,G505,G304 ./...
	# Generate HTML from JSON
	gosec -fmt=html -out=gosec-results.html -f=gosec-results.json
	# Generate text output to stdout from JSON
	gosec -fmt=text -stdout -verbose=text -f=gosec-results.json
	# Generate SARIF for GitHub Security Tab
	gosec -fmt=sarif -out=gosec-results.sarif -f=gosec-results.json
endif

# Excluding the following gosec rules:
# G401 (CWE-328): Use of weak cryptographic primitive (Used internally for creating unique hashed object names)
# G501 (CWE-327): Blocklisted import crypto/md5: weak cryptographic primitive (Used internally for creating unique hashed repo names)
# G505 (CWE-327): Blocklisted import crypto/sha1: weak cryptographic primitive (Used internally for creating unique hashed object names)
# G304 (CWE-22): Potential file inclusion via variable (Confidence: HIGH, Severity: MEDIUM)
