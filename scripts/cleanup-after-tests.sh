#!/usr/bin/env bash
# Stricter error handling
set -e # Exit on error
set -u # Must predefine variables
set -o pipefail # Check errors in piped commands
self_dir="$(dirname "$(readlink -f "$0")")"

rm -rf "$self_dir/../.cache"

kubectl get packagerev -A --no-headers | awk '{ print "-n", $1, $2 }' | xargs -L 1 --no-run-if-empty kubectl patch packagerev --type='json' -p='[{"op": "remove", "path": "/metadata/finalizers"}]'

kubectl get ns -o name --no-headers | grep test | xargs -L 1 --no-run-if-empty kubectl delete
