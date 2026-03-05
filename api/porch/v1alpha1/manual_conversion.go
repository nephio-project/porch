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

package v1alpha1

import (
	porch "github.com/nephio-project/porch/api/porch"
	"k8s.io/apimachinery/pkg/conversion"
)

// Convert_porch_PackageRevisionStatus_To_v1alpha1_PackageRevisionStatus is a manual conversion function
// that handles the conversion from internal PackageRevisionStatus to v1alpha1 PackageRevisionStatus.
// The new fields (ObservedPrrResourceVersion, RenderingPrrResourceVersion, PackageConditions) are dropped
// since they don't exist in v1alpha1.
func Convert_porch_PackageRevisionStatus_To_v1alpha1_PackageRevisionStatus(in *porch.PackageRevisionStatus, out *PackageRevisionStatus, s conversion.Scope) error {
	return autoConvert_porch_PackageRevisionStatus_To_v1alpha1_PackageRevisionStatus(in, out, s)
}
