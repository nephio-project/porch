#!/bin/bash
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

KUBERNETES_VERSION=0.29.2
go get -d "k8s.io/code-generator@v$KUBERNETES_VERSION"
CODE_GENERATOR=$(go list -f '{{.Dir}}' -m "k8s.io/code-generator@v$KUBERNETES_VERSION")
# . "${CODE_GENERATOR}/kube_codegen.sh"
. "${HERE}/kube_code_gen.sh"

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

#rm -fr "$ROOT/api/kubernetes-client"

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
