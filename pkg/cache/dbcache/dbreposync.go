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
	"sync"
	"time"

	api "github.com/nephio-project/porch/api/porch/v1alpha1"
	cachetypes "github.com/nephio-project/porch/pkg/cache/types"
	"github.com/nephio-project/porch/pkg/repository"
	pkgerrors "github.com/pkg/errors"
	"go.opentelemetry.io/otel/trace"
	"k8s.io/klog/v2"
)

const (
	syncTickInterval            = 1 * time.Second
	delayBeforeErroredSyncRetry = 10 * time.Second
)

type RepositorySync interface {
	Stop()
	GetLastSyncError() error
	SyncAfter(delayBeforeSync time.Duration)
}

var _ RepositorySync = &repositorySyncImpl{}

type repositorySyncImpl struct {
	ticker                  *time.Ticker
	syncFrequency           time.Duration
	syncCountdown           time.Duration
	repo                    *dbRepository
	cancel                  context.CancelFunc
	mutex                   sync.Mutex
	lastExternalRepoVersion string
	lastExternalPRMap       map[repository.PackageRevisionKey]repository.PackageRevision
	lastSyncError           error
	lastSyncStats           repositorySyncStats
}

type repositorySyncStats struct {
	cachedOnly   int
	externalOnly int
	both         int
}

func newRepositorySync(repo *dbRepository, options cachetypes.CacheOptions) RepositorySync {
	ctx, cancel := context.WithCancel(context.Background())

	s := &repositorySyncImpl{
		repo:          repo,
		cancel:        cancel,
		syncFrequency: options.RepoSyncFrequency,
		syncCountdown: 0,
	}

	s.ticker = time.NewTicker(syncTickInterval)

	go s.syncLoop(ctx)

	klog.V(4).Infof("repositorySync %+v: creating sync ticker with countdown of: %s", s.repo.Key(), s.syncCountdown)
	return s
}

func (s *repositorySyncImpl) Stop() {
	if s != nil {
		s.ticker.Stop()
		s.cancel()
	}
}

func (s *repositorySyncImpl) GetLastSyncError() error {
	return s.lastSyncError
}

func (s *repositorySyncImpl) SyncAfter(delayBeforeSync time.Duration) {
	go s.doSyncAfter(delayBeforeSync)
}

func (s *repositorySyncImpl) syncLoop(ctx context.Context) {
	klog.V(4).Infof("repositorySync %+v: sync ticker running, countdown=%s", s.repo.Key(), s.syncCountdown)

	s.tickTock(ctx)

	for {
		select {
		case <-ctx.Done():
			klog.V(4).Infof("repositorySync %+v: exiting sync because context stopped: %v", s.repo.Key(), ctx.Err())
			s.syncCountdown = 0
			return
		case <-s.ticker.C:
			s.tickTock(ctx)
		}
	}
}

func (s *repositorySyncImpl) tickTock(ctx context.Context) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.syncCountdown = s.syncCountdown - syncTickInterval

	if s.syncCountdown >= syncTickInterval {
		klog.V(7).Infof("repositorySync %+v: sync tick counting down, countdown=%s . . .", s.repo.Key(), s.syncCountdown)
		return
	}

	klog.V(7).Infof("repositorySync %+v: sync tick countdown %s expired, running sync . . .", s.repo.Key(), s.syncCountdown)

	s.lastSyncStats, s.lastSyncError = s.syncOnce(ctx)

	if s.lastSyncError == nil {
		s.syncCountdown = s.syncFrequency
		klog.V(4).Infof("repositorySync %+v: sync tick countdown to %s for next sync . . .", s.repo.Key(), s.syncFrequency)
	} else {
		klog.Errorf("repositorySync %+v: sync failed: %q", s.repo.Key(), s.lastSyncError)

		s.syncCountdown = delayBeforeErroredSyncRetry
		klog.V(4).Infof("repositorySync %+v: resetting sync tick countdown to %s for resync following error . . .", s.repo.Key(), delayBeforeErroredSyncRetry)
	}
}

func (s *repositorySyncImpl) doSyncAfter(delayBeforeSync time.Duration) {
	klog.V(4).Infof("repositorySync %+v: waiting for mutex to set sync tick countdown to %s to bring forward next sync . . .", s.repo.Key(), delayBeforeSync)
	s.mutex.Lock()
	s.syncCountdown = delayBeforeSync
	s.mutex.Unlock()
	klog.V(4).Infof("repositorySync %+v: set sync tick countdown to %s to bring forward next sync", s.repo.Key(), s.syncCountdown)
}

func (s *repositorySyncImpl) syncOnce(ctx context.Context) (repositorySyncStats, error) {
	ctx, span := tracer.Start(ctx, "Repository::syncOnce", trace.WithAttributes())
	defer span.End()

	start := time.Now()
	klog.V(4).Infof("repositorySync %+v: sync started", s.repo.Key())

	klog.V(4).Infof("repositorySync %+v: sync finished in %f secs", s.repo.Key(), time.Since(start).Seconds())

	syncStats, err := s.sync(ctx)
	if err != nil {
		return syncStats, pkgerrors.Wrapf(err, "repositorySync %+v: sync failed", s.repo.Key())
	}

	klog.V(4).Infof(" %d package revisions were already cached", syncStats.both)
	klog.V(4).Infof(" %d package revisions were cached from the external repository", syncStats.externalOnly)
	klog.V(4).Infof(" %d cached package revisions not found in the external repo were removed from the cache", syncStats.cachedOnly)

	return syncStats, nil
}

func (s *repositorySyncImpl) sync(ctx context.Context) (repositorySyncStats, error) {
	_, span := tracer.Start(ctx, "Repository::sync", trace.WithAttributes())
	defer span.End()

	cachedPrMap, err := s.getCachedPRMap(ctx)
	if err != nil {
		return repositorySyncStats{}, pkgerrors.Wrap(err, "sync failed reading cached pacakge revisions")
	}

	klog.V(4).Infof("repositorySync %+v: found %d deployed package revisions in cached repository", s.repo.Key(), len(cachedPrMap))

	externalPrMap, err := s.getExternalPRMap(ctx)
	if err != nil {
		return repositorySyncStats{}, pkgerrors.Wrap(err, "sync failed reading external pacakge revisions")
	}

	klog.V(4).Infof("repositorySync %+v: found %d package revisions in external repository", s.repo.Key(), len(externalPrMap))

	inCachedOnly, inBoth, inExternalOnly := s.comparePRMaps(ctx, cachedPrMap, externalPrMap)
	klog.V(4).Infof("repositorySync %+v: found %d cached only, %d in both, %d external only", s.repo.Key(), len(inCachedOnly), len(inBoth), len(inExternalOnly))

	return s.syncRepo(ctx, cachedPrMap, externalPrMap, inCachedOnly, inBoth, inExternalOnly)
}

func (s *repositorySyncImpl) syncRepo(ctx context.Context, cachedPrMap, externalPrMap map[repository.PackageRevisionKey]repository.PackageRevision, inCachedOnly, inBoth, inExternalOnly []repository.PackageRevisionKey) (repositorySyncStats, error) {

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
		klog.Errorf("repositorySync %+v: failed to list cached published package revisions", s.repo.Key())
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
		klog.V(4).Infof("repositorySync %+v: external repository is still on cached version %s, new read of external repo not required", s.repo.Key(), s.lastExternalRepoVersion)
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
		if err = dbPkg.DeletePackageRevision(ctx, dbPR, true); err != nil {
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
			repo:      s.repo,
			pkgRevKey: extPRKey,
			meta:      extAPIPR.ObjectMeta,
			spec:      &extAPIPR.Spec,
			updated:   extAPIPR.Status.PublishedAt.Time,
			updatedBy: extAPIPR.Status.PublishedBy,
			lifecycle: extAPIPR.Spec.Lifecycle,
			extPRID:   extPRUpstreamLock,
			tasks:     extAPIPR.Spec.Tasks,
			resources: extPRResources.Spec.Resources,
		}
		_, err = s.repo.savePackageRevision(ctx, &dbPR, true)
		if err != nil {
			klog.Errorf("repositorySync %+v: failed to save external package revision %+v to database", s.repo.Key(), extPRKey)
			return err
		}
	}

	return nil
}
