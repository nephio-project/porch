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

PORCH_API_OPENAPI_YAML=$ROOT/api/generated/openapi/yaml

sed -i -e 's/^kind: CustomResourceDefinition$/kind: PorchResourceDefinition/' -e "s/^# Copyright  The kpt and Nephio Authors$/# Copyright $(date '+%Y') The kpt and Nephio Authors/" "$PORCH_API_OPENAPI_YAML"/*.yaml
