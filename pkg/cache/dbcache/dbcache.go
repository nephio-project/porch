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

package dbcache

import (
	"context"
	"fmt"
	"time"

	configapi "github.com/nephio-project/porch/api/porchconfig/v1alpha1"
	cachetypes "github.com/nephio-project/porch/pkg/cache/types"
	"github.com/nephio-project/porch/pkg/repository"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"

	_ "github.com/jackc/pgx/v5/stdlib"
)

var tracer = otel.Tracer("dbcache")

var _ cachetypes.Cache = &dbCache{}

type dbCache struct {
	repositories map[repository.RepositoryKey]*dbRepository
	options      cachetypes.CacheOptions
}

func (c *dbCache) OpenRepository(ctx context.Context, repositorySpec *configapi.Repository) (repository.Repository, error) {
	_, span := tracer.Start(ctx, "dbCache::OpenRepository", trace.WithAttributes())
	defer span.End()

	repoKey := repository.RepositoryKey{
		Namespace: repositorySpec.Namespace,
		Name:      repositorySpec.Name,
	}

	if dbRepo, ok := c.repositories[repoKey]; ok {
		return dbRepo, nil
	}

	dbRepo := &dbRepository{
		repoKey:    repoKey,
		meta:       repositorySpec.ObjectMeta,
		spec:       repositorySpec,
		updated:    time.Now(),
		updatedBy:  getCurrentUser(),
		deployment: repositorySpec.Spec.Deployment,
	}

	err := dbRepo.OpenRepository(ctx, c.options.ExternalRepoOptions)
	if err != nil {
		return nil, err
	}

	c.repositories[repoKey] = dbRepo

	dbRepo.repositorySync = newRepositorySync(dbRepo, c.options)

	return dbRepo, nil
}

func (c *dbCache) UpdateRepository(ctx context.Context, repositorySpec *configapi.Repository) error {
	_, span := tracer.Start(ctx, "dbCache::UpdateRepository", trace.WithAttributes())
	defer span.End()

	repoKey := repository.RepositoryKey{
		Namespace: repositorySpec.Namespace,
		Name:      repositorySpec.Name,
	}

	dbRepo, ok := c.repositories[repoKey]
	if !ok {
		return fmt.Errorf("dbcache.UpdateRepository: repo %q not found", repoKey.String())
	}

	return repoUpdateDB(ctx, dbRepo)
}

func (c *dbCache) CloseRepository(ctx context.Context, repositorySpec *configapi.Repository, allRepos []configapi.Repository) error {
	_, span := tracer.Start(ctx, "dbCache::CloseRepository", trace.WithAttributes())
	defer span.End()

	repoKey := repository.RepositoryKey{
		Namespace: repositorySpec.Namespace,
		Name:      repositorySpec.Name,
	}

	defer delete(c.repositories, repoKey)

	dbRepo, ok := c.repositories[repoKey]
	if !ok {
		return fmt.Errorf("dbcache.CloseRepository: repo %q not found", repoKey.String())
	}

	return dbRepo.Close(ctx)
}

func (c *dbCache) GetRepositories() []*configapi.Repository {
	var repositories []*configapi.Repository

	for _, repo := range c.repositories {
		repositories = append(repositories, repo.spec)
	}

	return repositories
}

func (c *dbCache) GetRepository(repoKey repository.RepositoryKey) repository.Repository {
	return c.repositories[repoKey]
}
