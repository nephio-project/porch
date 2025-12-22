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
	"time"

	configapi "github.com/nephio-project/porch/api/porchconfig/v1alpha1"
	"github.com/nephio-project/porch/pkg/cache/repomap"
	cachetypes "github.com/nephio-project/porch/pkg/cache/types"
	"github.com/nephio-project/porch/pkg/externalrepo"
	"github.com/nephio-project/porch/pkg/repository"
	pkgerrors "github.com/pkg/errors"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"k8s.io/klog/v2"

	_ "github.com/jackc/pgx/v5/stdlib"
)

var tracer = otel.Tracer("dbcache")

var _ cachetypes.Cache = &dbCache{}

type dbCache struct {
	repositories repomap.SafeRepoMap
	options      cachetypes.CacheOptions
}

func (c *dbCache) OpenRepository(ctx context.Context, repositorySpec *configapi.Repository) (repository.Repository, error) {
	_, span := tracer.Start(ctx, "dbCache::OpenRepository", trace.WithAttributes())
	defer span.End()

	repoKey, err := externalrepo.RepositoryKey(repositorySpec)
	if err != nil {
		return nil, err
	}

	repo, err := c.repositories.LoadOrCreate(repoKey, func() (repository.Repository, error) {
		return c.createRepository(ctx, repoKey, repositorySpec)
	})
	if err != nil {
		return nil, err
	}

	dbRepo := repo.(*dbRepository)
	dbRepo.spec = repositorySpec
	return dbRepo, nil
}

func (c *dbCache) createRepository(ctx context.Context, key repository.RepositoryKey, repositorySpec *configapi.Repository) (repository.Repository, error) {
	klog.Infof("Creating repository : %v", key)

	dbRepo := &dbRepository{
		repoKey:              key,
		meta:                 repositorySpec.ObjectMeta,
		spec:                 repositorySpec,
		updated:              time.Now(),
		updatedBy:            getCurrentUser(),
		deployment:           repositorySpec.Spec.Deployment,
		repoPRChangeNotifier: c.options.RepoPRChangeNotifier,
	}

	err := dbRepo.OpenRepository(ctx, c.options.ExternalRepoOptions)
	if err != nil {
		return nil, err
	}

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

	dbRepo, ok := c.repositories.Load(repoKey)
	if !ok {
		return fmt.Errorf("dbcache.UpdateRepository: repo %+v not found", repoKey)
	}

	return repoUpdateDB(ctx, dbRepo.(*dbRepository))
}

func (c *dbCache) CloseRepository(ctx context.Context, repositorySpec *configapi.Repository, allRepos []configapi.Repository) error {
	_, span := tracer.Start(ctx, "dbCache::CloseRepository", trace.WithAttributes())
	defer span.End()

	repoKey, err := externalrepo.RepositoryKey(repositorySpec)
	if err != nil {
		return err
	}

	dbRepo, ok := c.repositories.Load(repoKey)
	if !ok {
		return pkgerrors.Errorf("dbcache.CloseRepository: repo %+v not found", repoKey)
	}

	defer func() {
		c.repositories.LoadAndDelete(repoKey)
	}()

	if dbRepo != nil {
		if err := dbRepo.Close(ctx); err != nil {
			return pkgerrors.Wrapf(err, "failed to close db repository %+v", repoKey)
		}
	}

	return nil
}

func (c *dbCache) GetRepositories() []*configapi.Repository {
	var repositories []*configapi.Repository

	c.repositories.Range(func(key, value any) bool {
		repositories = append(repositories, value.(*dbRepository).spec)
		return true
	})

	return repositories
}

func (c *dbCache) GetRepository(repoKey repository.RepositoryKey) repository.Repository {
	repo, ok := c.repositories.Load(repoKey)
	if !ok || repo == nil {
		return nil
	}
	return repo.(*dbRepository)
}

func (c *dbCache) CheckRepositoryConnectivity(ctx context.Context, repositorySpec *configapi.Repository) error {
	return externalrepo.CheckRepositoryConnection(ctx, repositorySpec, c.options.ExternalRepoOptions)
}
