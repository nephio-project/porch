// Copyright 2025 The kpt and Nephio Authors
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

package merge3

import (
	"sigs.k8s.io/kustomize/kyaml/kio/filters"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

// GetHandlingStrategy is an implementation of the ResourceHandler.Handle method from
// kyaml. It is used to decide how a resource should be handled during the
// 3-way merge. This differs from the default implementation in that if a
// resource is deleted from upstream, it will only be deleted from local if
// there is no diff between origin and local.
func GetHandlingStrategy(original, updated, dest *yaml.RNode) filters.ResourceMergeStrategy {
	switch {
	// Keep the resource if added locally.
	case original == nil && updated == nil && dest != nil:
		return filters.KeepDest
	// Add the resource if added in upstream.
	case original == nil && updated != nil && dest == nil:
		return filters.KeepUpdated
	// Do not re-add the resource if deleted from both upstream and local
	case updated == nil && dest == nil:
		return filters.Skip
	// If deleted from upstream, delete it.
	case original != nil && updated == nil:
		return filters.Skip
	// Do not re-add if deleted from local.
	case original != nil && dest == nil:
		return filters.Skip
	default:
		return filters.Merge
	}
}
