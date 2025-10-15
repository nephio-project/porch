// Copyright 2024-2025 The kpt and Nephio Authors
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

// Package dbcache implements a database cache for Porch.
package dbcache

import (
	"context"
	"fmt"
	"sync"
	"time"

	configapi "github.com/nephio-project/porch/api/porchconfig/v1alpha1"
	cachetypes "github.com/nephio-project/porch/pkg/cache/types"
	"github.com/nephio-project/porch/pkg/externalrepo"
	"github.com/nephio-project/porch/pkg/repository"
	pkgerrors "github.com/pkg/errors"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"

	_ "github.com/jackc/pgx/v5/stdlib"
)

var tracer = otel.Tracer("dbcache")

var _ cachetypes.Cache = &dbCache{}

type dbCache struct {
	repositories map[repository.RepositoryKey]*dbRepository
	repoLocks    map[string]*sync.Mutex
	mainLock     *sync.RWMutex
	options      cachetypes.CacheOptions
}

func (c *dbCache) OpenRepository(ctx context.Context, repositorySpec *configapi.Repository) (repository.Repository, error) {
	_, span := tracer.Start(ctx, "dbCache::OpenRepository", trace.WithAttributes())
	defer span.End()

	repoKey, err := externalrepo.RepositoryKey(repositorySpec)
	if err != nil {
		return nil, err
	}

	c.mainLock.RLock()

	if dbRepo, ok := c.repositories[repoKey]; ok {
		c.mainLock.RUnlock()
		// Keep the spec updated in the cache.
		dbRepo.spec = repositorySpec
		return dbRepo, nil
	}
	c.mainLock.RUnlock()

	dbRepo := &dbRepository{
		repoKey:              repoKey,
		meta:                 repositorySpec.ObjectMeta,
		spec:                 repositorySpec,
		updated:              time.Now(),
		updatedBy:            getCurrentUser(),
		deployment:           repositorySpec.Spec.Deployment,
		repoPRChangeNotifier: c.options.RepoPRChangeNotifier,
	}

	err = dbRepo.OpenRepository(ctx, c.options.ExternalRepoOptions)
	if err != nil {
		return nil, err
	}

	c.mainLock.Lock()
	c.repositories[repoKey] = dbRepo
	c.mainLock.Unlock()

	dbRepo.repositorySync = newRepositorySync(dbRepo, c.options)

	return dbRepo, nil
}

func (c *dbCache) UpdateRepository(ctx context.Context, repositorySpec *configapi.Repository) error {
	_, span := tracer.Start(ctx, "dbCache::UpdateRepository", trace.WithAttributes())
	defer span.End()

	repoKey, err := externalrepo.RepositoryKey(repositorySpec)
	if err != nil {
		return err
	}

	c.mainLock.RLock()
	dbRepo, ok := c.repositories[repoKey]
	c.mainLock.RUnlock()
	if !ok {
		return fmt.Errorf("dbcache.UpdateRepository: repo %+v not found", repoKey)
	}

	return repoUpdateDB(ctx, dbRepo)
}

func (c *dbCache) CloseRepository(ctx context.Context, repositorySpec *configapi.Repository, allRepos []configapi.Repository) error {
	_, span := tracer.Start(ctx, "dbCache::CloseRepository", trace.WithAttributes())
	defer span.End()

	repoKey, err := externalrepo.RepositoryKey(repositorySpec)
	if err != nil {
		return err
	}

	c.mainLock.RLock()
	dbRepo, ok := c.repositories[repoKey]
	c.mainLock.RUnlock()
	if !ok {
		return pkgerrors.Errorf("dbcache.CloseRepository: repo %+v not found", repoKey)
	}

	cacheKey := c.getCacheKey(repositorySpec)
	repoLock := c.getOrCreateLock(cacheKey)
	repoLock.Lock()
	defer repoLock.Unlock()

	// For Git repositories, check sharing based on URL only
	if repositorySpec.Spec.Type == configapi.RepositoryTypeGit {
		for _, r := range allRepos {
			if r.Name == repositorySpec.Name && r.Namespace == repositorySpec.Namespace {
				continue
			}
			if r.Spec.Type == configapi.RepositoryTypeGit &&
				r.Spec.Git.Repo == repositorySpec.Spec.Git.Repo {
				// do not close cached repo if it is shared
				dbRepo.repositorySync.Stop()
				return nil
			}
		}
	}

	// TODO: should we still delete if close fails?
	defer func() {
		c.mainLock.Lock()
		delete(c.repositories, repoKey)
		c.mainLock.Unlock()
	}()

	if err := dbRepo.Close(ctx); err != nil {
		return pkgerrors.Wrapf(err, "failed to close db repository %+v", repoKey)
	}

	return nil
}

func (c *dbCache) GetRepositories() []*configapi.Repository {
	var repositories []*configapi.Repository

	c.mainLock.RLock()
	defer c.mainLock.RUnlock()
	for _, repo := range c.repositories {
		repositories = append(repositories, repo.spec)
	}

	return repositories
}

func (c *dbCache) GetRepository(repoKey repository.RepositoryKey) repository.Repository {
	c.mainLock.RLock()
	defer c.mainLock.RUnlock()
	return c.repositories[repoKey]
}

func (c *dbCache) CheckRepositoryConnectivity(ctx context.Context, repositorySpec *configapi.Repository) error {
	return externalrepo.CheckRepositoryConnection(ctx, repositorySpec, c.options.ExternalRepoOptions)
}
