#!/usr/bin/env bash

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

set -e

if [[ "$OSTYPE" == "darwin"* ]]
then
       READLINK=greadlink
else
       READLINK=readlink
fi

HERE=$(dirname "$($READLINK --canonicalize "$BASH_SOURCE")")
. "$HERE/_trap"

ROOT=$($READLINK --canonicalize "$HERE/..")

ORG=github.com/nephio-project
REPO=$ORG/porch
API_PKG=$REPO/api
INPUT_PKG=$API_PKG/porch
CLIENT_PKG=$API_PKG/generated

BOILERPLATE=$HERE/boilerplate.go.txt
OPENAPI_REPORT=$ROOT/gen_openapi.report

KUBERNETES_VERSION=0.29.8
go get -d "k8s.io/code-generator@v$KUBERNETES_VERSION"
CODE_GENERATOR=$(go list -f '{{.Dir}}' -m "k8s.io/code-generator@v$KUBERNETES_VERSION")
. "${CODE_GENERATOR}/kube_codegen.sh"

# kube_codegen expects a specific directory structure, so we will create it via a symlink

WORK=$(mktemp --directory)

echo "work directory: $WORK"

copy_function goodbye old_goodbye
function goodbye () {
	echo "deleting work directory: $WORK"
	rm -r "$WORK"
	old_goodbye $1
}

mkdir -p "$WORK/$ORG"
ln -s "$ROOT" "$WORK/$REPO"

echo 'gen_helpers...'

kube::codegen::gen_helpers \
	--output-base "$WORK" \
	--input-pkg-root "$INPUT_PKG" \
	--boilerplate "$BOILERPLATE" \
	--extra-peer-dir "k8s.io/apimachinery/pkg/apis/meta/v1" \
	--extra-peer-dir "k8s.io/apimachinery/pkg/runtime" \
	--extra-peer-dir "k8s.io/apimachinery/pkg/version"

echo 'gen_openapi...'

rm -fr "$CLIENT_PKG/openapi"

# Note: lots of validation errors from Kubernetes meta package; can be ignored
kube::codegen::gen_openapi \
	--output-base "$WORK" \
	--input-pkg-root "$API_PKG" \
	--output-pkg-root "$CLIENT_PKG" \
	--boilerplate "$BOILERPLATE" \
	--report-filename "$OPENAPI_REPORT" \
	--update-report

echo 'gen_client...'

. "${HERE}/kube_code_gen.sh"

kube::codegen::gen_client \
	--output-base "$WORK" \
	--with-watch \
	--input-pkg-root "$API_PKG" \
	--one-input-api "porch" \
	--output-pkg-root "$CLIENT_PKG" \
	--plural-exceptions "PackageRevisionResources:PackageRevisionResources" \
	--boilerplate "$BOILERPLATE"

go get -d "k8s.io/kube-aggregator@v0.30.3"
go get -d "sigs.k8s.io/controller-runtime@v0.18.5"
# Our "go get" added dependencies that we don't need
cd "$ROOT"
go mod tidy
