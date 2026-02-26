// Copyright 2026 The kpt and Nephio Authors
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

package v1alpha2

import (
	"unsafe"

	porch "github.com/nephio-project/porch/api/porch"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/conversion"
)

// Convert_porch_PackageRevisionResourcesSpec_To_v1alpha2_PackageRevisionResourcesSpec
// drops fields that don't exist in v1alpha2 (PackageName, WorkspaceName, Revision, RepositoryName)
func Convert_porch_PackageRevisionResourcesSpec_To_v1alpha2_PackageRevisionResourcesSpec(in *porch.PackageRevisionResourcesSpec, out *PackageRevisionResourcesSpec, s conversion.Scope) error {
	out.Resources = *(*map[string]string)(unsafe.Pointer(&in.Resources))
	return nil
}

// Convert_porch_PackageRevisionSpec_To_v1alpha2_PackageRevisionSpec
// drops the Parent field that doesn't exist in v1alpha2
func Convert_porch_PackageRevisionSpec_To_v1alpha2_PackageRevisionSpec(in *porch.PackageRevisionSpec, out *PackageRevisionSpec, s conversion.Scope) error {
	return autoConvert_porch_PackageRevisionSpec_To_v1alpha2_PackageRevisionSpec(in, out, s)
}

// Convert_porch_UpstreamPackage_To_v1alpha2_UpstreamPackage
// drops the Oci field that doesn't exist in v1alpha2
func Convert_porch_UpstreamPackage_To_v1alpha2_UpstreamPackage(in *porch.UpstreamPackage, out *UpstreamPackage, s conversion.Scope) error {
	return autoConvert_porch_UpstreamPackage_To_v1alpha2_UpstreamPackage(in, out, s)
}

// Convert_v1_Condition_To_porch_Condition converts metav1.Condition to porch.Condition
func Convert_v1_Condition_To_porch_Condition(in *metav1.Condition, out *porch.Condition, s conversion.Scope) error {
	out.Type = in.Type
	out.Status = porch.ConditionStatus(in.Status)
	out.Reason = in.Reason
	out.Message = in.Message
	return nil
}

// Convert_porch_Condition_To_v1_Condition converts porch.Condition to metav1.Condition
func Convert_porch_Condition_To_v1_Condition(in *porch.Condition, out *metav1.Condition, s conversion.Scope) error {
	out.Type = in.Type
	out.Status = metav1.ConditionStatus(in.Status)
	out.Reason = in.Reason
	out.Message = in.Message
	return nil
}


