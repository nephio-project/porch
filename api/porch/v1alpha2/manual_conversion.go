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
	porch "github.com/nephio-project/porch/api/porch"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/conversion"
)

// Convert_porch_PackageRevisionSpec_To_v1alpha2_PackageRevisionSpec
// converts Tasks to Source and drops the Parent field
func Convert_porch_PackageRevisionSpec_To_v1alpha2_PackageRevisionSpec(in *porch.PackageRevisionSpec, out *PackageRevisionSpec, s conversion.Scope) error {
	out.PackageName = in.PackageName
	out.RepositoryName = in.RepositoryName
	out.WorkspaceName = in.WorkspaceName
	out.Revision = in.Revision
	out.Lifecycle = PackageRevisionLifecycle(in.Lifecycle)
	
	// Convert first task to Source
	if len(in.Tasks) > 0 {
		firstTask := in.Tasks[0]
		out.Source = &PackageSource{}
		switch firstTask.Type {
		case porch.TaskTypeInit:
			if firstTask.Init != nil {
				out.Source.Init = &PackageInitSpec{
					Subpackage:  firstTask.Init.Subpackage,
					Description: firstTask.Init.Description,
					Keywords:    firstTask.Init.Keywords,
					Site:        firstTask.Init.Site,
				}
			}
		case porch.TaskTypeClone:
			if firstTask.Clone != nil {
				out.Source.CloneFrom = &UpstreamPackage{}
				if err := Convert_porch_UpstreamPackage_To_v1alpha2_UpstreamPackage(&firstTask.Clone.Upstream, out.Source.CloneFrom, s); err != nil {
					return err
				}
			}
		case porch.TaskTypeEdit:
			if firstTask.Edit != nil && firstTask.Edit.Source != nil {
				out.Source.CopyFrom = &PackageRevisionRef{Name: firstTask.Edit.Source.Name}
			}
		case porch.TaskTypeUpgrade:
			if firstTask.Upgrade != nil {
				out.Source.Upgrade = &PackageUpgradeSpec{
					OldUpstream:             PackageRevisionRef{Name: firstTask.Upgrade.OldUpstream.Name},
					NewUpstream:             PackageRevisionRef{Name: firstTask.Upgrade.NewUpstream.Name},
					LocalPackageRevisionRef: PackageRevisionRef{Name: firstTask.Upgrade.LocalPackageRevisionRef.Name},
					Strategy:                PackageMergeStrategy(firstTask.Upgrade.Strategy),
				}
			}
		}
	}
	
	for i := range in.ReadinessGates {
		out.ReadinessGates = append(out.ReadinessGates, ReadinessGate{
			ConditionType: in.ReadinessGates[i].ConditionType,
		})
	}
	
	if in.PackageMetadata != nil {
		out.PackageMetadata = &PackageMetadata{
			Labels:      in.PackageMetadata.Labels,
			Annotations: in.PackageMetadata.Annotations,
		}
	}
	
	return nil
}

// Convert_v1alpha2_PackageRevisionSpec_To_porch_PackageRevisionSpec
// converts Source to Tasks
func Convert_v1alpha2_PackageRevisionSpec_To_porch_PackageRevisionSpec(in *PackageRevisionSpec, out *porch.PackageRevisionSpec, s conversion.Scope) error {
	out.PackageName = in.PackageName
	out.RepositoryName = in.RepositoryName
	out.WorkspaceName = in.WorkspaceName
	out.Revision = in.Revision
	out.Lifecycle = porch.PackageRevisionLifecycle(in.Lifecycle)
	
	// Convert Source to first task
	if in.Source != nil {
		task := porch.Task{}
		if in.Source.Init != nil {
			task.Type = porch.TaskTypeInit
			task.Init = &porch.PackageInitTaskSpec{
				Subpackage:  in.Source.Init.Subpackage,
				Description: in.Source.Init.Description,
				Keywords:    in.Source.Init.Keywords,
				Site:        in.Source.Init.Site,
			}
		} else if in.Source.CloneFrom != nil {
			task.Type = porch.TaskTypeClone
			task.Clone = &porch.PackageCloneTaskSpec{}
			if err := Convert_v1alpha2_UpstreamPackage_To_porch_UpstreamPackage(in.Source.CloneFrom, &task.Clone.Upstream, s); err != nil {
				return err
			}
		} else if in.Source.CopyFrom != nil {
			task.Type = porch.TaskTypeEdit
			task.Edit = &porch.PackageEditTaskSpec{
				Source: &porch.PackageRevisionRef{Name: in.Source.CopyFrom.Name},
			}
		} else if in.Source.Upgrade != nil {
			task.Type = porch.TaskTypeUpgrade
			task.Upgrade = &porch.PackageUpgradeTaskSpec{
				OldUpstream:             porch.PackageRevisionRef{Name: in.Source.Upgrade.OldUpstream.Name},
				NewUpstream:             porch.PackageRevisionRef{Name: in.Source.Upgrade.NewUpstream.Name},
				LocalPackageRevisionRef: porch.PackageRevisionRef{Name: in.Source.Upgrade.LocalPackageRevisionRef.Name},
				Strategy:                porch.PackageMergeStrategy(in.Source.Upgrade.Strategy),
			}
		}
		out.Tasks = []porch.Task{task}
	}
	
	for i := range in.ReadinessGates {
		out.ReadinessGates = append(out.ReadinessGates, porch.ReadinessGate{
			ConditionType: in.ReadinessGates[i].ConditionType,
		})
	}
	
	if in.PackageMetadata != nil {
		out.PackageMetadata = &porch.PackageMetadata{
			Labels:      in.PackageMetadata.Labels,
			Annotations: in.PackageMetadata.Annotations,
		}
	}
	
	return nil
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
// Sets required fields (LastTransitionTime, Reason) to satisfy metav1.Condition schema
func Convert_porch_Condition_To_v1_Condition(in *porch.Condition, out *metav1.Condition, s conversion.Scope) error {
	out.Type = in.Type
	out.Status = metav1.ConditionStatus(in.Status)
	
	// Reason is required by metav1.Condition (minLength: 1)
	// Use "Unknown" if not set in porch.Condition
	if in.Reason != "" {
		out.Reason = in.Reason
	} else {
		out.Reason = "Unknown"
	}
	
	out.Message = in.Message
	
	// LastTransitionTime is required by metav1.Condition
	// Set to current time since porch.Condition doesn't track this
	out.LastTransitionTime = metav1.Now()
	
	// ObservedGeneration is optional, leave at 0
	out.ObservedGeneration = 0
	
	return nil
}


// Convert_v1alpha2_PackageRevisionStatus_To_porch_PackageRevisionStatus
// converts metav1.Condition to porch.Condition and drops CreationSource
func Convert_v1alpha2_PackageRevisionStatus_To_porch_PackageRevisionStatus(in *PackageRevisionStatus, out *porch.PackageRevisionStatus, s conversion.Scope) error {
	return autoConvert_v1alpha2_PackageRevisionStatus_To_porch_PackageRevisionStatus(in, out, s)
}
