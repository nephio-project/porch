// Copyright 2022, 2024-2025 The kpt and Nephio Authors
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

package dbcache

import (
	"context"
	"fmt"
	"sync"
	"time"

	api "github.com/nephio-project/porch/api/porch/v1alpha1"
	cachetypes "github.com/nephio-project/porch/pkg/cache/types"
	"github.com/nephio-project/porch/pkg/repository"
	"go.opentelemetry.io/otel/trace"
	"k8s.io/klog/v2"
)

type repositorySync struct {
	repo          *dbRepository
	cancel        context.CancelFunc
	mutex         sync.Mutex
	lastSyncError error
	lastSyncStats repositorySyncStats
}

type repositorySyncStats struct {
	cachedOnly   int
	externalOnly int
	both         int
}

func newRepositorySync(repo *dbRepository, options cachetypes.CacheOptions) *repositorySync {
	ctx, cancel := context.WithCancel(context.Background())

	s := &repositorySync{
		repo:   repo,
		cancel: cancel,
	}

	go s.syncForever(ctx, options.RepoSyncFrequency)

	return s
}

func (s *repositorySync) stop() {
	if s != nil {
		s.cancel()
	}
}

func (s *repositorySync) syncForever(ctx context.Context, repoSyncFrequency time.Duration) {
	for {
		select {
		case <-ctx.Done():
			klog.V(2).Infof("repositorySync %+v: exiting repository sync, because context is done: %v", s.repo.Key(), ctx.Err())
			return
		default:
			s.lastSyncStats, s.lastSyncError = s.syncOnce(ctx)
			time.Sleep(repoSyncFrequency)
		}
	}
}

func (s *repositorySync) syncOnce(ctx context.Context) (repositorySyncStats, error) {
	ctx, span := tracer.Start(ctx, "[START]::Repository::syncOnce", trace.WithAttributes())
	defer span.End()

	if !s.mutex.TryLock() {
		return repositorySyncStats{}, fmt.Errorf("repositorySync %+v: sync start failed because sync is already in progress", s.repo.Key())
	}
	defer s.mutex.Unlock()

	start := time.Now()
	klog.Infof("repositorySync %+v: sync started", s.repo.Key())

	defer func() {
		klog.Infof("repositorySync %+v: sync finished in %f secs", s.repo.Key(), time.Since(start).Seconds())
		klog.Infof(" %d package revisions were already cached", s.lastSyncStats.both)
		klog.Infof(" %d package revisions were cached from the external repository", s.lastSyncStats.externalOnly)
		klog.Infof(" %d cached package revisions not found in the external repo were removed from the cache", s.lastSyncStats.cachedOnly)
	}()

	return s.sync(ctx)
}

func (s *repositorySync) getLastSyncError() error {
	return s.lastSyncError
}

func (s *repositorySync) sync(ctx context.Context) (repositorySyncStats, error) {
	_, span := tracer.Start(ctx, "[START]::Repository::syncOnce", trace.WithAttributes())
	defer span.End()

	deployedFilter := repository.ListPackageRevisionFilter{
		Lifecycles: []api.PackageRevisionLifecycle{
			api.PackageRevisionLifecyclePublished,
			api.PackageRevisionLifecycleDeletionProposed,
		},
	}
	cachedPrList, err := s.repo.ListPackageRevisions(ctx, deployedFilter)
	if err != nil {
		klog.Errorf("repositorySync %+v: failed to list cached package revisions", s.repo.Key())
		return repositorySyncStats{}, err
	}

	klog.Infof("repositorySync %+v: found %d deployed package revisions in cached repository", s.repo.Key(), len(cachedPrList))
	cachedPrMap := repository.PrSlice2Map(cachedPrList)

	externalPRList, err := s.repo.externalRepo.ListPackageRevisions(ctx, repository.ListPackageRevisionFilter{})
	if err != nil {
		klog.Errorf("repositorySync %+v: failed to list external package revisions", s.repo.Key())
		return repositorySyncStats{}, err
	}

	klog.Infof("repositorySync %+v: found %d package revisions in external repository", s.repo.Key(), len(externalPRList))
	externalPrMap := repository.PrSlice2Map(externalPRList)

	inCachedOnly, inBoth, inExternalOnly := s.comparePRMaps(ctx, cachedPrMap, externalPrMap)
	klog.Infof("repositorySync %+v: found %d cached only, %d in both, %d external only", s.repo.Key(), len(inCachedOnly), len(inBoth), len(inExternalOnly))

	if err = s.deletePRsOnlyInCache(ctx, cachedPrMap, inCachedOnly); err != nil {
		return repositorySyncStats{}, err
	}

	if err = s.cacheExternalPRs(ctx, externalPrMap, inExternalOnly); err != nil {
		return repositorySyncStats{}, err
	}

	return repositorySyncStats{
		cachedOnly:   len(inCachedOnly),
		externalOnly: len(inExternalOnly),
		both:         len(inBoth),
	}, nil
}

func (s *repositorySync) comparePRMaps(ctx context.Context, leftMap, rightMap map[repository.PackageRevisionKey]repository.PackageRevision) (leftOnly, both, rightOnly []repository.PackageRevisionKey) {
	_, span := tracer.Start(ctx, "[START]::Repository::syncOnce", trace.WithAttributes())
	defer span.End()

	var inLeftOnly, inBoth, inRightOnly []repository.PackageRevisionKey

	for leftPrKey := range leftMap {
		if _, ok := rightMap[leftPrKey]; ok {
			inBoth = append(inBoth, leftPrKey)
		} else {
			inLeftOnly = append(inLeftOnly, leftPrKey)
		}
	}

	for rightPrKey := range rightMap {
		if _, ok := leftMap[rightPrKey]; !ok {
			inRightOnly = append(inRightOnly, rightPrKey)
		}
	}

	return inLeftOnly, inBoth, inRightOnly
}

func (s *repositorySync) cacheExternalPRs(ctx context.Context, externalPrMap map[repository.PackageRevisionKey]repository.PackageRevision, inExternalOnly []repository.PackageRevisionKey) error {
	for _, extPRKey := range inExternalOnly {
		extPR := externalPrMap[extPRKey]

		extAPIPR, err := externalPrMap[extPRKey].GetPackageRevision(ctx)
		if err != nil {
			klog.Errorf("repositorySync %+v: failed to get API version of external package revision %+v", s.repo.Key(), extPRKey)
			return err
		}

		extPRResources, err := extPR.GetResources(ctx)
		if err != nil {
			klog.Errorf("repositorySync %+v: failed to get resources for external package revision %+v", s.repo.Key(), extPRKey)
			return err
		}

		dbPR := dbPackageRevision{
			repo:      s.repo,
			pkgRevKey: extPRKey,
			meta:      extAPIPR.ObjectMeta,
			spec:      &extAPIPR.Spec,
			updated:   extAPIPR.CreationTimestamp.Time,
			lifecycle: extAPIPR.Spec.Lifecycle,
			tasks:     extAPIPR.Spec.Tasks,
			resources: extPRResources.Spec.Resources,
		}
		_, err = s.repo.savePackageRevision(ctx, &dbPR)
		if err != nil {
			klog.Errorf("repositorySync %+v: failed to save external package revision %+v to database", s.repo.Key(), extPRKey)
			return err
		}
	}

	return nil
}

func (s *repositorySync) deletePRsOnlyInCache(ctx context.Context, cachedPrMap map[repository.PackageRevisionKey]repository.PackageRevision, inCachedOnly []repository.PackageRevisionKey) error {
	for _, dbPRKey := range inCachedOnly {
		dbPR := cachedPrMap[dbPRKey]

		if err := s.repo.DeletePackageRevision(ctx, dbPR); err != nil {
			klog.Errorf("repositorySync %+v: failed to delete cached PR %+v not in external repo", s.repo.Key(), dbPRKey)
			return err
		}
	}
	return nil
}
