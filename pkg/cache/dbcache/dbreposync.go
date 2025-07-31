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

package dbcache

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"time"

	api "github.com/nephio-project/porch/api/porch/v1alpha1"
	cachetypes "github.com/nephio-project/porch/pkg/cache/types"
	"github.com/nephio-project/porch/pkg/engine"
	"github.com/nephio-project/porch/pkg/repository"
	pkgerrors "github.com/pkg/errors"
	"go.opentelemetry.io/otel/trace"
	"k8s.io/klog/v2"
)

const (
	delayBeforeResync           = 1 * time.Second
	delayBeforeErroredSyncRetry = 10 * time.Second
	delayBetweenExternalPushes  = 100 * time.Millisecond
	delayBetweenExternalDeletes = 100 * time.Millisecond
)

type RepositorySync interface {
	Stop()
	GetLastSyncError() error
	SyncAfter(delayBeforeSync time.Duration)
}

var _ RepositorySync = &repositorySyncImpl{}

type repositorySyncImpl struct {
	repo                    *dbRepository
	cancel                  context.CancelFunc
	mutex                   sync.Mutex
	lastExternalRepoVersion string
	lastExternalPRMap       map[repository.PackageRevisionKey]repository.PackageRevision
	lastSyncError           error
	lastSyncStats           repositorySyncStats
	syncNeeded              bool
}

type repositorySyncStats struct {
	cachedOnly   int
	externalOnly int
	both         int
}

func newRepositorySync(repo *dbRepository, options cachetypes.CacheOptions) RepositorySync {
	ctx, cancel := context.WithCancel(context.Background())

	s := &repositorySyncImpl{
		repo:       repo,
		cancel:     cancel,
		syncNeeded: false,
	}

	go s.syncForever(ctx, options.RepoSyncFrequency)

	return s
}

func (s *repositorySyncImpl) Stop() {
	if s != nil {
		s.cancel()
	}
}

func (s *repositorySyncImpl) GetLastSyncError() error {
	return s.lastSyncError
}

func (s *repositorySyncImpl) SyncAfter(delayBeforeSync time.Duration) {
	go s.syncWithStartDelay(delayBeforeSync)
}

func (s *repositorySyncImpl) syncForever(ctx context.Context, repoSyncFrequency time.Duration) {
	for {
		select {
		case <-ctx.Done():
			klog.V(2).Infof("repositorySync %+v: exiting repository sync, because context is done: %v", s.repo.Key(), ctx.Err())
			return
		default:
			s.lastSyncStats, s.lastSyncError = s.syncOnce(ctx)

			if s.lastSyncError == nil {
				if s.syncNeeded {
					klog.Infof("repositorySync %+v: waiting for %s before deferred sync starts . . .", s.repo.Key(), delayBeforeResync)
					time.Sleep(delayBeforeResync)
				} else {
					klog.Infof("repositorySync %+v: waiting for %s before next sync starts . . .", s.repo.Key(), repoSyncFrequency)
					time.Sleep(repoSyncFrequency)
				}
			} else {
				klog.Infof("repositorySync %+v: sync failed: %q", s.repo.Key(), s.lastSyncError)
				klog.Infof("repositorySync %+v: waiting for %s before resync . . .", s.repo.Key(), delayBeforeErroredSyncRetry)
				time.Sleep(delayBeforeErroredSyncRetry)
			}
		}
	}
}

func (s *repositorySyncImpl) syncWithStartDelay(delayBeforeSync time.Duration) {
	time.Sleep(delayBeforeSync)

	ctx, cancel := context.WithCancel(context.Background())

	s.lastSyncStats, s.lastSyncError = s.syncOnce(ctx)

	cancel()
}

func (s *repositorySyncImpl) syncOnce(ctx context.Context) (repositorySyncStats, error) {
	ctx, span := tracer.Start(ctx, "Repository::syncOnce", trace.WithAttributes())
	defer span.End()

	if !s.mutex.TryLock() {
		s.syncNeeded = true
		klog.Infof("repositorySync %+v: sync start deferred until sync already in progress completes", s.repo.Key())
		return repositorySyncStats{}, nil
	} else {
		s.syncNeeded = false
	}
	defer s.mutex.Unlock()

	start := time.Now()
	klog.Infof("repositorySync %+v: sync started", s.repo.Key())

	klog.Infof("repositorySync %+v: sync finished in %f secs", s.repo.Key(), time.Since(start).Seconds())

	syncStats, err := s.sync(ctx)
	if err != nil {
		return syncStats, pkgerrors.Wrapf(err, "repositorySync %+v: sync failed", s.repo.Key())
	}

	if s.repo.deployment {
		klog.Infof(" %d package revisions in both cache and external repository", syncStats.both)
		klog.Infof(" %d package revisions were deleted from the external repository", syncStats.externalOnly)
		klog.Infof(" %d cached package revisions not found in the external repo were pushed to the external repo", syncStats.cachedOnly)

	} else {
		klog.Infof(" %d package revisions were already cached", syncStats.both)
		klog.Infof(" %d package revisions were cached from the external repository", syncStats.externalOnly)
		klog.Infof(" %d cached package revisions not found in the external repo were removed from the cache", syncStats.cachedOnly)
	}

	return syncStats, nil
}

func (s *repositorySyncImpl) sync(ctx context.Context) (repositorySyncStats, error) {
	_, span := tracer.Start(ctx, "Repository::sync", trace.WithAttributes())
	defer span.End()

	cachedPrMap, err := s.getCachedPRMap(ctx)
	if err != nil {
		return repositorySyncStats{}, pkgerrors.Wrap(err, "sync failed reading cached pacakge revisions")
	}

	klog.Infof("repositorySync %+v: found %d deployed package revisions in cached repository", s.repo.Key(), len(cachedPrMap))

	externalPrMap, err := s.getExternalPRMap(ctx)
	if err != nil {
		return repositorySyncStats{}, pkgerrors.Wrap(err, "sync failed reading external pacakge revisions")
	}

	klog.Infof("repositorySync %+v: found %d package revisions in external repository", s.repo.Key(), len(externalPrMap))

	inCachedOnly, inBoth, inExternalOnly := s.comparePRMaps(ctx, cachedPrMap, externalPrMap)
	klog.Infof("repositorySync %+v: found %d cached only, %d in both, %d external only", s.repo.Key(), len(inCachedOnly), len(inBoth), len(inExternalOnly))

	if s.repo.deployment {
		return s.syncDeploymentRepo(ctx, cachedPrMap, externalPrMap, inCachedOnly, inBoth, inExternalOnly)
	} else {

		return s.syncNonDeploymentRepo(ctx, cachedPrMap, externalPrMap, inCachedOnly, inBoth, inExternalOnly)
	}
}

func (s *repositorySyncImpl) syncDeploymentRepo(ctx context.Context, cachedPrMap, externalPrMap map[repository.PackageRevisionKey]repository.PackageRevision, inCachedOnly, inBoth, inExternalOnly []repository.PackageRevisionKey) (repositorySyncStats, error) {
	if err := s.deletePRsOnlyOnExternal(ctx, externalPrMap, inExternalOnly); err != nil {
		return repositorySyncStats{}, err
	}

	markedPRsDeleted, err := s.deletePRsMarkedForDeletion(ctx, cachedPrMap, inBoth)
	if err != nil {
		return repositorySyncStats{}, err
	}

	if err := s.pushCachedPRsToExternal(ctx, cachedPrMap, inCachedOnly); err != nil {
		return repositorySyncStats{}, err
	}

	return repositorySyncStats{
		cachedOnly:   len(inCachedOnly),
		externalOnly: len(inExternalOnly) + markedPRsDeleted,
		both:         len(inBoth) - markedPRsDeleted,
	}, nil
}

func (s *repositorySyncImpl) syncNonDeploymentRepo(ctx context.Context, cachedPrMap, externalPrMap map[repository.PackageRevisionKey]repository.PackageRevision, inCachedOnly, inBoth, inExternalOnly []repository.PackageRevisionKey) (repositorySyncStats, error) {

	if err := s.deletePRsOnlyInCache(ctx, cachedPrMap, inCachedOnly); err != nil {
		return repositorySyncStats{}, err
	}

	if err := s.cacheExternalPRs(ctx, externalPrMap, inExternalOnly); err != nil {
		return repositorySyncStats{}, err
	}

	return repositorySyncStats{
		cachedOnly:   len(inCachedOnly),
		externalOnly: len(inExternalOnly),
		both:         len(inBoth),
	}, nil
}

func (s *repositorySyncImpl) getCachedPRMap(ctx context.Context) (map[repository.PackageRevisionKey]repository.PackageRevision, error) {
	deployedFilter := repository.ListPackageRevisionFilter{
		Lifecycles: []api.PackageRevisionLifecycle{
			api.PackageRevisionLifecyclePublished,
			api.PackageRevisionLifecycleDeletionProposed,
		},
	}
	cachedPrList, err := s.repo.ListPackageRevisions(ctx, deployedFilter)
	if err != nil {
		klog.Errorf("repositorySync %+v: failed to list cached package revisions", s.repo.Key())
		return nil, err
	}

	return repository.PrSlice2Map(cachedPrList), nil
}

func (s *repositorySyncImpl) getExternalPRMap(ctx context.Context) (map[repository.PackageRevisionKey]repository.PackageRevision, error) {
	externalRepoVersion, err := s.repo.Version(ctx)
	if err != nil {
		return nil, pkgerrors.Wrapf(err, "fetch of external repository %+v version failed", s.repo.Key())
	}

	if s.lastExternalRepoVersion == externalRepoVersion {
		klog.Infof("repositorySync %+v: external repository is still on cached version %s, new read of external repo not required", s.repo.Key(), s.lastExternalRepoVersion)
		return s.lastExternalPRMap, nil
	}

	externalPRList, err := s.repo.externalRepo.ListPackageRevisions(ctx, repository.ListPackageRevisionFilter{})
	if err != nil {
		klog.Errorf("repositorySync %+v: failed to list external package revisions", s.repo.Key())
		return nil, err
	}

	externalPRMap := repository.PrSlice2Map(externalPRList)

	s.lastExternalPRMap = externalPRMap
	s.lastExternalRepoVersion = externalRepoVersion

	return externalPRMap, nil
}

func (s *repositorySyncImpl) comparePRMaps(ctx context.Context, leftMap, rightMap map[repository.PackageRevisionKey]repository.PackageRevision) (leftOnly, both, rightOnly []repository.PackageRevisionKey) {
	_, span := tracer.Start(ctx, "Repository::comparePRMaps", trace.WithAttributes())
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

func (s *repositorySyncImpl) deletePRsOnlyOnExternal(ctx context.Context, externalPrMap map[repository.PackageRevisionKey]repository.PackageRevision, inExternalOnly []repository.PackageRevisionKey) error {
	for _, extPRKey := range inExternalOnly {
		extPR2Delete := externalPrMap[extPRKey]

		if err := s.repo.externalRepo.DeletePackageRevision(ctx, extPR2Delete); err != nil {
			klog.Warningf("dbPackageRevision:Delete: deletion of %+v failed on external repository %q", extPR2Delete.Key(), err)
			return err
		}
	}
	return nil
}

func (s *repositorySyncImpl) deletePRsMarkedForDeletion(ctx context.Context, cachedPrMap map[repository.PackageRevisionKey]repository.PackageRevision, inBoth []repository.PackageRevisionKey) (int, error) {
	var markedPRsDeleted = 0

	for _, pr := range cachedPrMap {
		dbPR := pr.(*dbPackageRevision)

		if !dbPR.deletingOnExt {
			continue
		}

		if slices.Contains(inBoth, dbPR.Key()) {
			if err := s.repo.externalRepo.DeletePackageRevision(ctx, dbPR); err != nil {
				klog.Warningf("dbPackageRevision:Delete: deletion of %+v failed on external repository %q", dbPR.Key(), err)
				return markedPRsDeleted, err
			}
		}

		if err := s.repo.deletePackageRevision(ctx, dbPR); err != nil {
			klog.Errorf("repositorySync %+v: failed to delete cached PR %+v that was deleted on external repo", s.repo.Key(), dbPR.Key())
			return markedPRsDeleted, err
		}
		markedPRsDeleted++

		time.Sleep(delayBetweenExternalDeletes)
	}
	return markedPRsDeleted, nil
}

func (s *repositorySyncImpl) pushCachedPRsToExternal(ctx context.Context, cachedPrMap map[repository.PackageRevisionKey]repository.PackageRevision, inCachedOnly []repository.PackageRevisionKey) error {
	for _, cachedPRKey := range inCachedOnly {
		cachedPR := cachedPrMap[cachedPRKey]
		dbPR := cachedPR.(*dbPackageRevision)

		pushedPRExtID, err := engine.PushPackageRevision(ctx, s.repo.externalRepo, dbPR)
		if err != nil {
			return pkgerrors.Wrapf(err, "repositorySync %+v: push of package revision %+v to external repo failed", s.repo.Key(), dbPR.Key())
		}

		dbPR.existsOnExt = true
		dbPR.extPRID = pushedPRExtID
		_, err = s.repo.savePackageRevision(ctx, dbPR, false)
		if err != nil {
			return pkgerrors.Wrapf(err, "repositorySync %+v: failed to save package revision %+v to database after push to external repo", s.repo.Key(), dbPR.Key())
		}

		if err = dbPR.publishPlaceholderPRForPR(ctx); err != nil {
			return pkgerrors.Wrapf(err, "repositorySync %+v: failed to save placeholder package revision for package revisino %+v to database", s.repo.Key(), dbPR.Key())
		}

		time.Sleep(delayBetweenExternalPushes)
	}

	return nil
}

func (s *repositorySyncImpl) deletePRsOnlyInCache(ctx context.Context, cachedPrMap map[repository.PackageRevisionKey]repository.PackageRevision, inCachedOnly []repository.PackageRevisionKey) error {
	for _, dbPRKey := range inCachedOnly {
		dbPR := cachedPrMap[dbPRKey]

		pkgList, err := s.repo.ListPackages(ctx, repository.ListPackageFilter{Key: dbPR.Key().PKey()})
		if err != nil {
			return err
		}

		if len(pkgList) != 1 {
			err := fmt.Errorf("deletePRsOnlyInCache: reading package %+v should return 1 package, it returned %d packages", dbPR.Key().PKey(), len(pkgList))
			klog.Warning(err.Error())
			return err
		}

		dbPkg := pkgList[0].(*dbPackage)
		if err = dbPkg.DeletePackageRevision(ctx, dbPR); err != nil {
			klog.Errorf("repositorySync %+v: failed to delete cached PR %+v not in external repo", s.repo.Key(), dbPRKey)
			return err
		}
	}
	return nil
}

func (s *repositorySyncImpl) cacheExternalPRs(ctx context.Context, externalPrMap map[repository.PackageRevisionKey]repository.PackageRevision, inExternalOnly []repository.PackageRevisionKey) error {
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

		_, extPRUpstreamLock, _ := extPR.GetLock()

		dbPR := dbPackageRevision{
			repo:          s.repo,
			pkgRevKey:     extPRKey,
			meta:          extAPIPR.ObjectMeta,
			spec:          &extAPIPR.Spec,
			updated:       extAPIPR.Status.PublishedAt.Time,
			updatedBy:     extAPIPR.Status.PublishedBy,
			lifecycle:     extAPIPR.Spec.Lifecycle,
			existsOnExt:   true,
			deletingOnExt: false,
			extPRID:       extPRUpstreamLock,
			tasks:         extAPIPR.Spec.Tasks,
			resources:     extPRResources.Spec.Resources,
		}
		_, err = s.repo.savePackageRevision(ctx, &dbPR, true)
		if err != nil {
			klog.Errorf("repositorySync %+v: failed to save external package revision %+v to database", s.repo.Key(), extPRKey)
			return err
		}
	}

	return nil
}
