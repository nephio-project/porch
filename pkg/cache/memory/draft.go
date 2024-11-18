// Copyright 2022, 2024 The kpt and Nephio Authors
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

package memory

import (
	"context"

	"github.com/nephio-project/porch/api/porch/v1alpha1"
	"github.com/nephio-project/porch/pkg/repository"
	"go.opentelemetry.io/otel/trace"
)

type cachedDraft struct {
	repository.PackageDraft
	cache *cachedRepository
}

var _ repository.PackageDraft = &cachedDraft{}

func (cd *cachedDraft) Close(ctx context.Context, version string) (repository.PackageRevision, error) {
	ctx, span := tracer.Start(ctx, "cachedDraft::Close", trace.WithAttributes())
	defer span.End()
	v, err := cd.cache.Version(ctx)
	if err != nil {
		return nil, err
	}
	if v != cd.cache.lastVersion {
		_, _, err = cd.cache.refreshAllCachedPackages(ctx)
		if err != nil {
			return nil, err
		}
	}

	revisions, err := cd.cache.ListPackageRevisions(ctx, repository.ListPackageRevisionFilter{
		Package: cd.GetName(),
	})
	if err != nil {
		return nil, err
	}

	var publishedRevisions []string
	for _, rev := range revisions {
		if v1alpha1.LifecycleIsPublished(rev.Lifecycle()) {
			publishedRevisions = append(publishedRevisions, rev.Key().Revision)
		}
	}

	nextVersion, err := repository.NextRevisionNumber(publishedRevisions)
	if err != nil {
		return nil, err
	}

	if closed, err := cd.PackageDraft.Close(ctx, nextVersion); err != nil {
		return nil, err
	} else {
		return cd.cache.update(ctx, closed)
	}
}
