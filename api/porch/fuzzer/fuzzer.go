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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtimeserializer "k8s.io/apimachinery/pkg/runtime/serializer"
	"sigs.k8s.io/randfill"
)

// Funcs provides custom fuzzing functions for roundtrip tests.
// These functions nil out fields that were intentionally removed in v1alpha2
// or normalized during conversion to ensure lossless roundtrip conversion during testing.
//
// Fields removed in v1alpha2:
// - UpstreamPackage.Oci: OCI support removed (incomplete implementation)
// - PackageRevisionSpec.Parent: Already deprecated in v1alpha1
// - PackageRevisionResourcesSpec fields: Redundant with metadata.name
//
// Fields not present in v1alpha1:
// - PackageRevisionStatus.ObservedPrrResourceVersion: Added in internal version
// - PackageRevisionStatus.RenderingPrrResourceVersion: Added in internal version
// - PackageRevisionStatus.PackageConditions: Added in internal version
//
// Fields normalized during porch.Condition <-> metav1.Condition conversion:
// - metav1.Condition.LastTransitionTime: Set to metav1.Now() when converting from porch.Condition
// - metav1.Condition.ObservedGeneration: Always 0 (not tracked in porch.Condition)
// - metav1.Condition.Reason: Set to "Unknown" if empty in porch.Condition (required field)
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
		func(obj *porch.PackageRevisionStatus, c randfill.Continue) {
			// v1alpha1 doesn't have these fields
			obj.ObservedPrrResourceVersion = ""
			obj.RenderingPrrResourceVersion = ""
			obj.PackageConditions = nil
		},
		func(obj *porch.PackageRevisionResourcesSpec, c randfill.Continue) {
			// v1alpha2 removed fields that duplicate metadata.name
			obj.PackageName = ""
			obj.WorkspaceName = ""
			obj.Revision = 0
			obj.RepositoryName = ""
		},
		func(obj *metav1.Condition, c randfill.Continue) {
			// ObservedGeneration is always 0 in conversion (not tracked in porch.Condition)
			obj.ObservedGeneration = 0
			// Note: LastTransitionTime and Reason will differ after roundtrip
			// - LastTransitionTime is set to metav1.Now() during conversion
			// - Reason is set to "Unknown" if empty
			// These are acceptable differences for CRD schema compliance
		},
	}
}
