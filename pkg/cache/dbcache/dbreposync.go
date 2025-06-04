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
	s.cancel()
}

func (s *repositorySync) syncForever(ctx context.Context, repoSyncFrequency time.Duration) {
	for {
		select {
		case <-ctx.Done():
			klog.V(2).Infof("repositorySync %+v: exiting repository sync, because context is done: %v", s.repo.Key(), ctx.Err())
			return
		default:
			s.lastSyncError = s.syncOnce(ctx)
			time.Sleep(repoSyncFrequency)
		}
	}
}

func (s *repositorySync) syncOnce(ctx context.Context) error {
	ctx, span := tracer.Start(ctx, "[START]::Repository::syncOnce", trace.WithAttributes())
	defer span.End()

	if !s.mutex.TryLock() {
		return fmt.Errorf("repositorySync %+v: sync start failed because sync is already in progress", s.repo.Key())
	}
	defer s.mutex.Unlock()

	start := time.Now()
	klog.Infof("repositorySync %+v: sync started", s.repo.Key())

	defer func() {
		klog.Infof("repositorySync %+v: sync finished in %f secs", s.repo.Key(), time.Since(start).Seconds())
	}()

	return s.sync(ctx)
}

func (s *repositorySync) getLastSyncError() error {
	return s.lastSyncError
}

func (s *repositorySync) sync(ctx context.Context) error {
	_, span := tracer.Start(ctx, "[START]::Repository::syncOnce", trace.WithAttributes())
	defer span.End()

	cachedPrList, err := s.repo.ListPackageRevisions(ctx, repository.ListPackageRevisionFilter{})
	if err != nil {
		klog.Errorf("repositorySync %+v: failed to list cached package revisions", s.repo.Key())
	}
	klog.Infof("repositorySync %+v: found %d package revisions in cached repository", s.repo.Key(), len(cachedPrList))
	cachedPrMap := repository.PrSlice2Map(cachedPrList)

	externalPRList, err := s.repo.externalRepo.ListPackageRevisions(ctx, repository.ListPackageRevisionFilter{})
	if err != nil {
		klog.Errorf("repositorySync %+v: failed to list external package revisions", s.repo.Key())
	}
	klog.Infof("repositorySync %+v: found %d package revisions in external repository", s.repo.Key(), len(externalPRList))
	externalPrMap := repository.PrSlice2Map(externalPRList)

	inCachedOnly, inBoth, inExternalOnly := s.comparePRMaps(ctx, cachedPrMap, externalPrMap)
	klog.Infof("repositorySync %+v: found %d cached, %d in both, %d external", s.repo.Key(), len(inCachedOnly), len(inBoth), len(inExternalOnly))

	return nil
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
