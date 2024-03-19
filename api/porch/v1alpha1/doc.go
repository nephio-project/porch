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

//go:generate go run k8s.io/code-generator/cmd/deepcopy-gen --input-dirs ../v1alpha1 -O zz_generated.deepcopy --go-header-file ../../../scripts/boilerplate.go.txt
//go:generate go run k8s.io/code-generator/cmd/defaulter-gen --input-dirs ../v1alpha1 -O zz_generated.defaults --go-header-file ../../../scripts/boilerplate.go.txt
//go:generate go run k8s.io/code-generator/cmd/client-gen --clientset-name versioned --input-base "" --input github.com/nephio-project/porch/api/porch/v1alpha1 -p github.com/nephio-project/porch/api/generated/clientset --plural-exceptions PorchPkgRevisionResources:PorchPkgRevisionResources --go-header-file ../../../scripts/boilerplate.go.txt
//go:generate go run k8s.io/code-generator/cmd/lister-gen --input-dirs github.com/nephio-project/porch/api/porch/v1alpha1 -p github.com/nephio-project/porch/api/generated/listers --go-header-file ../../../scripts/boilerplate.go.txt
//go:generate go run k8s.io/code-generator/cmd/informer-gen --input-dirs github.com/nephio-project/porch/api/porch/v1alpha1 --versioned-clientset-package github.com/nephio-project/porch/api/generated/clientset/versioned --listers-package github.com/nephio-project/porch/api/generated/listers --internal-clientset-package github.com/nephio-project/porch/api/generated/informers/internalversion -p github.com/nephio-project/porch/api/generated/informers --plural-exceptions PorchPkgRevisionResources:PorchPkgRevisionResources --go-header-file ../../../scripts/boilerplate.go.txt
//go:generate go run k8s.io/code-generator/cmd/conversion-gen --input-dirs ../../porch,../v1alpha1 -O zz_generated.conversion --go-header-file ../../../scripts/boilerplate.go.txt
//go:generate rm -fr ../../generated/clientset
//go:generate mv github.com/nephio-project/porch/api/generated/clientset ../../generated/
//go:generate rm -fr ../../generated/informers
//go:generate mv github.com/nephio-project/porch/api/generated/informers ../../generated/
//go:generate rm -fr ../../generated/listers
//go:generate mv github.com/nephio-project/porch/api/generated/listers ../../generated/
//go:generate rm -fr github.com

// Api versions allow the api contract for a resource to be changed while keeping
// backward compatibility by support multiple concurrent versions
// of the same resource

// +k8s:openapi-gen=true
// +k8s:deepcopy-gen=package,register
// +k8s:conversion-gen=github.com/nephio-project/porch/api/porch
// +k8s:defaulter-gen=TypeMeta
// +groupName=porch.kpt.dev
package v1alpha1 // import "github.com/nephio-project/porch/api/porch/v1alpha1"
