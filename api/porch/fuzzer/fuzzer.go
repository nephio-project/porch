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

package fuzzer

import (
	"github.com/nephio-project/porch/api/porch"
	runtimeserializer "k8s.io/apimachinery/pkg/runtime/serializer"
	"sigs.k8s.io/randfill"
)

// Funcs provides custom fuzzing functions for roundtrip tests.
// These functions nil out fields that were intentionally removed in v1alpha2
// to ensure lossless roundtrip conversion during testing.
//
// Fields removed in v1alpha2:
// - UpstreamPackage.Oci: OCI support removed (incomplete implementation)
// - PackageRevisionSpec.Parent: Already deprecated in v1alpha1
// - PackageRevisionResourcesSpec fields: Redundant with metadata.name
var Funcs = func(codecs runtimeserializer.CodecFactory) []any {
	return []any{
		func(obj *porch.UpstreamPackage, c randfill.Continue) {
			// v1alpha2 removed OCI support
			obj.Oci = nil
		},
		func(obj *porch.PackageRevisionSpec, c randfill.Continue) {
			// v1alpha2 removed deprecated Parent field
			obj.Parent = nil
		},
		func(obj *porch.PackageRevisionResourcesSpec, c randfill.Continue) {
			// v1alpha2 removed fields that duplicate metadata.name
			obj.PackageName = ""
			obj.WorkspaceName = ""
			obj.Revision = 0
			obj.RepositoryName = ""
		},
	}
}
