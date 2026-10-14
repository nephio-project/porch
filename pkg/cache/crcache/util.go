// Copyright 2022, 2024-2026 The kpt Authors
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

package crcache

import (
	"cmp"
	"context"
	"slices"
	"strings"

	"github.com/kptdev/porch/api/porch/key"
	porchapi "github.com/kptdev/porch/api/porch/v1alpha1"
	"github.com/kptdev/porch/pkg/repository"
)

func identifyLatestRevisions(ctx context.Context, result map[key.PackageRevisionKey]*cachedPackageRevision) {
	// Compute the latest among the different revisions of the same package.
	// The map is keyed by the package name; Values are the latest revision found so far.

	// TODO: Should map[string] be map[repository.PackageKey]?
	latest := map[string]*cachedPackageRevision{}
	for _, current := range result {
		current.mutex.Lock()
		current.isLatestRevision = false // Clear all values
		current.mutex.Unlock()

		// Check if the current package revision is more recent than the one seen so far.
		// Only consider Published packages
		if !porchapi.LifecycleIsPublished(current.Lifecycle(ctx)) {
			continue
		}

		currentKey := current.Key()
		if previous, ok := latest[currentKey.PkgKey.Package]; ok {
			previousKey := previous.Key()
			if currentKey.Revision > previousKey.Revision {
				// currentKey.Revision > previousKey.Revision; update latest
				latest[currentKey.PkgKey.Package] = current
			}
		} else if currentKey.Revision != -1 { // The working repository PR (usually main) can never be the latest PR
			// First revision of the specific package; candidate for the latest.
			latest[currentKey.PkgKey.Package] = current
		}
	}

	// Mark the winners as latest
	for _, v := range latest {
		v.mutex.Lock()
		v.isLatestRevision = true
		v.mutex.Unlock()
	}
}

func toPackageRevisionSlice(
	ctx context.Context, cached map[key.PackageRevisionKey]*cachedPackageRevision, filter repository.ListPackageRevisionFilter) []repository.PackageRevision {
	result := make([]repository.PackageRevision, 0, len(cached))
	for _, p := range cached {
		if filter.Matches(ctx, p) {
			result = append(result, p)
		}
	}
	slices.SortFunc(result, func(a, b repository.PackageRevision) int {
		ka, kb := a.Key(), b.Key()
		if res := strings.Compare(ka.PkgKey.Package, kb.PkgKey.Package); res != 0 {
			return res
		}
		if res := cmp.Compare(ka.Revision, kb.Revision); res != 0 {
			return res
		}
		if res := strings.Compare(string(a.Lifecycle(ctx)), string(b.Lifecycle(ctx))); res != 0 {
			return res
		}
		return strings.Compare(a.KubeObjectName(), b.KubeObjectName())
	})
	return result
}

func toPackageSlice(cached map[key.PackageKey]*cachedPackage, filter repository.ListPackageFilter) []repository.Package {
	result := make([]repository.Package, 0, len(cached))
	for _, p := range cached {
		if filter.Matches(p) {
			result = append(result, p)
		}
	}
	slices.SortFunc(result, func(a, b repository.Package) int {
		return strings.Compare(a.Key().Package, b.Key().Package)
	})

	return result
}
