// Copyright 2022, 2025 The kpt and Nephio Authors
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

package repository

import (
	"fmt"
	"strconv"
	"strings"

	api "github.com/nephio-project/porch/api/porch/v1alpha1"
	kptfile "github.com/nephio-project/porch/pkg/kpt/api/kptfile/v1"
	"github.com/nephio-project/porch/pkg/util"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func ToApiReadinessGates(kf kptfile.KptFile) []api.ReadinessGate {
	var readinessGates []api.ReadinessGate
	if kf.Info != nil {
		for _, rg := range kf.Info.ReadinessGates {
			readinessGates = append(readinessGates, api.ReadinessGate{
				ConditionType: rg.ConditionType,
			})
		}
	}
	return readinessGates
}

func ToApiConditions(kf kptfile.KptFile) []api.Condition {
	var conditions []api.Condition
	if kf.Status != nil && kf.Status.Conditions != nil {
		for _, s := range kf.Status.Conditions {
			conditions = append(conditions, api.Condition{
				Type:    s.Type,
				Status:  toApiConditionStatus(s.Status),
				Reason:  s.Reason,
				Message: s.Message,
			})
		}
	}
	return conditions
}

func toApiConditionStatus(s kptfile.ConditionStatus) api.ConditionStatus {
	switch s {
	case kptfile.ConditionTrue:
		return api.ConditionTrue
	case kptfile.ConditionFalse:
		return api.ConditionFalse
	case kptfile.ConditionUnknown:
		return api.ConditionUnknown
	default:
		panic(fmt.Errorf("unknown condition status: %v", s))
	}
}

// AnyBlockOwnerDeletionSet checks whether there are any ownerReferences in the Object
// which have blockOwnerDeletion enabled (meaning either nil or true).
func AnyBlockOwnerDeletionSet(obj client.Object) bool {
	for _, owner := range obj.GetOwnerReferences() {
		if owner.BlockOwnerDeletion == nil || *owner.BlockOwnerDeletion {
			return true
		}
	}
	return false
}

func Revision2Int(revisionStr string) int {
	revisionStr = strings.TrimPrefix(revisionStr, "v")

	if revision, err := strconv.Atoi(revisionStr); err == nil {
		return revision
	} else {
		return -1
	}
}

func Revision2Str(revision int) string {
	return strconv.Itoa(revision)
}

func ComposePkgObjName(key PackageKey) string {
	return util.ComposePkgObjName(key.RepoKey.Name, key.Path, key.Package)
}

func ComposePkgRevObjName(key PackageRevisionKey) string {
	return util.ComposePkgRevObjName(key.PkgKey.RepoKey.Name, key.PkgKey.Path, key.PkgKey.Package, key.WorkspaceName)
}
