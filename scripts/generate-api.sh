#!/bin/bash
set -e

HERE=$(dirname "$(readlink --canonicalize "$BASH_SOURCE")")
. "$HERE/_env"
. "$HERE/_trap"

ORG=github.com/nephio-project
REPO=$ORG/porch
API_PKG=$REPO/api
INPUT_PKG=$API_PKG/porch
CLIENT_PKG=$API_PKG/generated

BOILERPLATE=$HERE/boilerplate.go.txt
OPENAPI_REPORT=$ROOT/gen_openapi.report

go get -d "k8s.io/code-generator@v$KUBERNETES_VERSION"
CODE_GENERATOR=$(go list -f '{{.Dir}}' -m "k8s.io/code-generator@v$KUBERNETES_VERSION")
# . "${CODE_GENERATOR}/kube_codegen.sh"
. "${HERE}/kube_code_gen.sh"

# kube_codegen expects a specific directory structure, so we will create it via a symlink

WORK=$(mktemp --directory)

m "work directory: $WORK"

copy_function goodbye old_goodbye
function goodbye () {
	m "deleting work directory: $WORK"
	rm --recursive "$WORK"
	old_goodbye $1
}

mkdir --parents "$WORK/$ORG"
ln --symbolic "$ROOT" "$WORK/$REPO"

m 'gen_helpers...'

kube::codegen::gen_helpers \
	--output-base "$WORK" \
	--input-pkg-root "$INPUT_PKG" \
	--boilerplate "$BOILERPLATE" \
	--extra-peer-dir "k8s.io/apimachinery/pkg/apis/meta/v1" \
	--extra-peer-dir "k8s.io/apimachinery/pkg/runtime" \
	--extra-peer-dir "k8s.io/apimachinery/pkg/version"

m 'gen_openapi...'

rm --recursive --force "$CLIENT_PKG/openapi"

# Note: lots of validation errors from Kubernetes meta package; can be ignored
kube::codegen::gen_openapi \
	--output-base "$WORK" \
	--input-pkg-root "$API_PKG" \
	--output-pkg-root "$CLIENT_PKG" \
	--boilerplate "$BOILERPLATE" \
	--report-filename "$OPENAPI_REPORT" \
	--update-report

m 'gen_client...'

#rm --recursive --force "$ROOT/api/kubernetes-client"

kube::codegen::gen_client \
	--output-base "$WORK" \
	--with-watch \
	--input-pkg-root "$API_PKG" \
	--one-input-api "porch" \
	--output-pkg-root "$CLIENT_PKG" \
	--plural-exceptions "PackageRevisionResources:PackageRevisionResources" \
	--boilerplate "$BOILERPLATE"

# Our "go get" added dependencies that we don't need
cd "$ROOT"
go mod tidy
