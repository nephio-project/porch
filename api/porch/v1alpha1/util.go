// Copyright 2022-2024 The kpt and Nephio Authors
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
// limitations under the License

package v1alpha1

func LifecycleIsPublished(lifecycle PackageRevisionLifecycle) bool {
	return lifecycle == PackageRevisionLifecyclePublished || lifecycle == PackageRevisionLifecycleDeletionProposed
}

// Check ReadinessGates checks if the package has met all readiness gates
func PackageRevisionIsReady(readinessGates []ReadinessGate, conditions []Condition) bool {
	// Index our conditions
	conds := make(map[string]Condition)
	for _, c := range conditions {
		conds[c.Type] = c
	}

	// Check if the readiness gates are met
	for _, g := range readinessGates {
		if _, ok := conds[g.ConditionType]; !ok {
			return false
		}
		if conds[g.ConditionType].Status != ConditionTrue {
			return false
		}
	}

	return true
}
