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
	configapi "github.com/nephio-project/porch/api/porchconfig/v1alpha1"
	cachetypes "github.com/nephio-project/porch/pkg/cache/types"
	"github.com/nephio-project/porch/pkg/cache/util"
	"github.com/nephio-project/porch/pkg/repository"
	pkgerrors "github.com/pkg/errors"
	"github.com/robfig/cron/v3"
	"go.opentelemetry.io/otel/trace"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type repositorySync struct {
	repo                    *dbRepository
	cancel                  context.CancelFunc
	mutex                   sync.Mutex
	lastExternalRepoVersion string
	lastExternalPRMap       map[repository.PackageRevisionKey]repository.PackageRevision
	lastSyncError           error
	lastSyncStats           repositorySyncStats
	coreClient              client.WithWatch
}

type repositorySyncStats struct {
	cachedOnly   int
	externalOnly int
	both         int
}

func newRepositorySync(repo *dbRepository, options cachetypes.CacheOptions) *repositorySync {
	ctx, cancel := context.WithCancel(context.Background())
	s := repositorySync{
		repo:       repo,
		cancel:     cancel,
		coreClient: options.CoreClient,
	}

	go s.syncForever(ctx, options.RepoCrSyncFrequency)
	go s.handleRunOnceAt(ctx)
	return &s
}

func (s *repositorySync) Stop() {
	if s != nil {
		s.cancel()
	}
}

func (s *repositorySync) syncForever(ctx context.Context, repoCrSyncFrequency time.Duration) {
	s.lastSyncStats, s.lastSyncError = s.syncOnce(ctx)
	s.updateRepositoryCondition(ctx)

	for {
		waitDuration := s.calculateWaitDuration(repoCrSyncFrequency)
		ticker := time.NewTicker(waitDuration)

		select {
		case <-ctx.Done():
			klog.V(2).Infof("repositorySync %+v: exiting repository sync, because context is done: %v", s.repo.Key(), ctx.Err())
			ticker.Stop()
			return
		case <-ticker.C:
			ticker.Stop()
			s.lastSyncStats, s.lastSyncError = s.syncOnce(ctx)
			s.updateRepositoryCondition(ctx)
		}
	}
}

func (s *repositorySync) handleRunOnceAt(ctx context.Context) {
	var runOnceTimer *time.Timer
	var runOnceChan <-chan time.Time
	var scheduledRunOnceAt time.Time
	specPollInterval := 10 * time.Second
	ctxDoneLog := "repositorySync %+v: exiting repository handleRunOnceAt sync, because context is done: %v"

	for {
		select {
		case <-ctx.Done():
			klog.Infof(ctxDoneLog, s.repo.Key(), ctx.Err())
			if runOnceTimer != nil {
				runOnceTimer.Stop()
			}
			return
		default:
			if !s.hasValidSyncSpec() {
				klog.V(2).Infof("repositorySync %+v: repo or sync spec is nil, skipping runOnceAt check", s.repo.Key())
				select {
				case <-ctx.Done():
					klog.Infof(ctxDoneLog, s.repo.Key(), ctx.Err())
					return
				case <-time.After(specPollInterval):
				}
				continue
			}

			runOnceAt := s.repo.spec.Spec.Sync.RunOnceAt
			if s.shouldScheduleRunOnce(runOnceAt, scheduledRunOnceAt) {
				if runOnceTimer != nil {
					runOnceTimer.Stop()
				}
				delay := time.Until(runOnceAt.Time)
				if delay > 0 {
					klog.Infof("repositorySync %+v: Scheduling one-time sync at %s", s.repo.Key(), runOnceAt.Time.Format(time.RFC3339))
					runOnceTimer = time.NewTimer(delay)
					runOnceChan = runOnceTimer.C
					scheduledRunOnceAt = runOnceAt.Time
				} else {
					klog.V(2).Infof("repositorySync %+v: runOnceAt time is in the past (%s), skipping", s.repo.Key(), runOnceAt.Time.Format(time.RFC3339))
					runOnceTimer, runOnceChan, scheduledRunOnceAt = nil, nil, time.Time{}
				}
			}

			if runOnceChan != nil {
				select {
				case <-runOnceChan:
					klog.Infof("repositorySync %+v: Triggering scheduled one-time sync", s.repo.Key())
					s.lastSyncStats, s.lastSyncError = s.syncOnce(context.Background())
					s.updateRepositoryCondition(ctx)
					klog.Infof("repositorySync %+v: Finished one-time sync", s.repo.Key())
					runOnceTimer, runOnceChan, scheduledRunOnceAt = nil, nil, time.Time{}
				case <-ctx.Done():
					klog.Infof(ctxDoneLog, s.repo.Key(), ctx.Err())
					return
				case <-time.After(specPollInterval):
				}
			} else {
				select {
				case <-ctx.Done():
					klog.Infof(ctxDoneLog, s.repo.Key(), ctx.Err())
					return
				case <-time.After(specPollInterval):
				}
			}
		}
	}
}

func (s *repositorySync) updateRepositoryCondition(ctx context.Context) {
	status := "ready"
	if s.lastSyncError != nil {
		klog.Warningf("repositorySync %+v: sync error: %v", s.repo.Key(), s.lastSyncError)
		status = "error"
	}
	if err := s.setRepositoryCondition(ctx, status); err != nil {
		klog.Warningf("repositorySync %+v: failed to set repository condition: %v", s.repo.Key(), err)
	}
}

func (s *repositorySync) calculateWaitDuration(defaultDuration time.Duration) time.Duration {
	if !s.hasValidSyncSpec() {
		klog.Warningf("repositorySync %+v: repo or sync spec is nil, falling back to default interval: %v", s.repo.Key(), defaultDuration)
		return defaultDuration
	}

	cronExpr := s.repo.spec.Spec.Sync.Schedule
	if cronExpr == "" {
		klog.V(2).Infof("repositorySync %+v: sync.schedule is empty, falling back to repository cr sync interval: %v", s.repo.Key(), defaultDuration)
		return defaultDuration
	}

	schedule, err := cron.ParseStandard(cronExpr)
	if err != nil {
		klog.Warningf("repositorySync %+v: invalid cron expression '%s', falling back to default interval: %v", s.repo.Key(), cronExpr, defaultDuration)
		return defaultDuration
	}

	next := schedule.Next(time.Now())
	klog.Infof("repositorySync %+v: next scheduled time: %v", s.repo.Key(), next)
	return time.Until(next)
}

func (s *repositorySync) hasValidSyncSpec() bool {
	return s.repo != nil && s.repo.spec != nil && s.repo.spec.Spec.Sync != nil
}

func (s *repositorySync) shouldScheduleRunOnce(runOnceAt *metav1.Time, scheduled time.Time) bool {
	return runOnceAt != nil && !runOnceAt.IsZero() && (scheduled.IsZero() || !runOnceAt.Time.Equal(scheduled))
}

func (s *repositorySync) syncOnce(ctx context.Context) (repositorySyncStats, error) {
	ctx, span := tracer.Start(ctx, "Repository::syncOnce", trace.WithAttributes())
	defer span.End()

	if !s.mutex.TryLock() {
		return repositorySyncStats{}, fmt.Errorf("repositorySync %+v: sync start failed because sync is already in progress", s.repo.Key())
	}
	defer s.mutex.Unlock()

	start := time.Now()
	klog.Infof("repositorySync %+v: sync started", s.repo.Key())
	if err := s.setRepositoryCondition(ctx, "sync-in-progress"); err != nil {
		klog.Warningf("repositorySync %+v: failed to set repository condition: %v", s.repo.Key(), err)
	}

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
	_, span := tracer.Start(ctx, "Repository::sync", trace.WithAttributes())
	defer span.End()

	cachedPrMap, err := s.getCachedPRMap(ctx)
	if err != nil {
		return repositorySyncStats{}, pkgerrors.Wrap(err, "sync failed reading cached package revisions")
	}

	klog.Infof("repositorySync %+v: found %d deployed package revisions in cached repository", s.repo.Key(), len(cachedPrMap))

	externalPrMap, err := s.getExternalPRMap(ctx)
	if err != nil {
		return repositorySyncStats{}, pkgerrors.Wrap(err, "sync failed reading external package revisions")
	}

	klog.Infof("repositorySync %+v: found %d package revisions in external repository", s.repo.Key(), len(externalPrMap))

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

func (s *repositorySync) setRepositoryCondition(ctx context.Context, status string) error {
	if s.repo == nil {
		klog.Warning("repositorySync: repo is nil, cannot set repository condition")
		return fmt.Errorf("repository is nil")
	}
	repo := &configapi.Repository{}
	key := types.NamespacedName{
		Name:      s.repo.Key().Name,
		Namespace: s.repo.Key().Namespace,
	}

	if err := s.coreClient.Get(ctx, key, repo); err != nil {
		return fmt.Errorf("failed to get repository: %w", err)
	}

	errorMsg := ""
	if status == "error" && s.lastSyncError != nil {
		errorMsg = s.lastSyncError.Error()
	}

	condition, err := util.BuildRepositoryCondition(repo, status, errorMsg)
	if err != nil {
		return err
	}

	return util.ApplyRepositoryCondition(ctx, s.coreClient.Status(), repo, condition, status)
}

func (s *repositorySync) getCachedPRMap(ctx context.Context) (map[repository.PackageRevisionKey]repository.PackageRevision, error) {
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

func (s *repositorySync) getExternalPRMap(ctx context.Context) (map[repository.PackageRevisionKey]repository.PackageRevision, error) {
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

func (s *repositorySync) comparePRMaps(ctx context.Context, leftMap, rightMap map[repository.PackageRevisionKey]repository.PackageRevision) (leftOnly, both, rightOnly []repository.PackageRevisionKey) {
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

		if extAPIPR.CreationTimestamp.Time.IsZero() {
			extAPIPR.CreationTimestamp.Time = time.Now()
		}

		_, extPRUpstreamLock, _ := extPR.GetLock()

		dbPR := dbPackageRevision{
			repo:      s.repo,
			pkgRevKey: extPRKey,
			meta:      extAPIPR.ObjectMeta,
			spec:      &extAPIPR.Spec,
			updated:   time.Now(),
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

func (s *repositorySync) deletePRsOnlyInCache(ctx context.Context, cachedPrMap map[repository.PackageRevisionKey]repository.PackageRevision, inCachedOnly []repository.PackageRevisionKey) error {
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
		if err = dbPkg.DeletePackageRevision(ctx, dbPR, false); err != nil {
			klog.Errorf("repositorySync %+v: failed to delete cached PR %+v not in external repo", s.repo.Key(), dbPRKey)
			return err
		}
	}
	return nil
}
