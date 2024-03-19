// Copyright 2022 The kpt and Nephio Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:generate go run k8s.io/code-generator/cmd/deepcopy-gen --input-dirs ../porch,v1alpha1 -O zz_generated.deepcopy --go-header-file ../../scripts/boilerplate.go.txt
//go:generate go run k8s.io/code-generator/cmd/openapi-gen@v0.29.2 --input-dirs github.com/nephio-project/porch/api/porch/v1alpha1 --input-dirs k8s.io/apimachinery/pkg/apis/meta/v1,k8s.io/apimachinery/pkg/runtime,k8s.io/apimachinery/pkg/version -p github.com/nephio-project/porch/api/generated/openapi -O zz_generated.openapi --go-header-file ../../scripts/boilerplate.go.txt
//go:generate rm -fr ../generated/openapi
//go:generate mv github.com/nephio-project/porch/api/generated/openapi ../generated
//go:generate rm -fr github.com

// +k8s:deepcopy-gen=package,register
// +groupName=porch.kpt.dev

// PorchPkg api is the internal version of the API.
package porch
