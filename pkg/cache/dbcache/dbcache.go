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
	"time"

	configapi "github.com/nephio-project/porch/api/porchconfig/v1alpha1"
	cachetypes "github.com/nephio-project/porch/pkg/cache/types"
	"github.com/nephio-project/porch/pkg/repository"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
)

var _ cachetypes.Cache = &dbCache{}
var tracer = otel.Tracer("dbcache")

type dbCache struct {
	options cachetypes.CacheOptions
}

func (c *dbCache) OpenRepository(ctx context.Context, repositorySpec *configapi.Repository) (repository.Repository, error) {
	_, span := tracer.Start(ctx, "dbCache::OpenRepository", trace.WithAttributes())
	defer span.End()

	dbRepo := dbRepository{
		repoKey: repository.RepositoryKey{
			Namespace:  repositorySpec.Namespace,
			Repository: repositorySpec.Name,
		},
		meta:       &repositorySpec.ObjectMeta,
		spec:       &repositorySpec.Spec,
		updated:    time.Now(),
		updatedBy:  getCurrentUser(),
		deployment: repositorySpec.Spec.Deployment,
	}

	return dbRepo.OpenRepository()
}

func (c *dbCache) UpdateRepository(ctx context.Context, repositorySpec *configapi.Repository) error {
	_, span := tracer.Start(ctx, "dbCache::OpenRepository", trace.WithAttributes())
	defer span.End()

	dbRepo := dbRepository{
		repoKey: repository.RepositoryKey{
			Namespace:  repositorySpec.Namespace,
			Repository: repositorySpec.Name,
		},
		meta:       &repositorySpec.ObjectMeta,
		spec:       &repositorySpec.Spec,
		updated:    time.Now(),
		updatedBy:  getCurrentUser(),
		deployment: repositorySpec.Spec.Deployment,
	}

	return repoUpdateDB(&dbRepo)
}

func (c *dbCache) CloseRepository(ctx context.Context, repositorySpec *configapi.Repository, allRepos []configapi.Repository) error {
	_, span := tracer.Start(ctx, "dbCache::OpenRepository", trace.WithAttributes())
	defer span.End()

	if dbRepo, err := repoReadFromDB(repository.RepositoryKey{
		Namespace:  repositorySpec.Namespace,
		Repository: repositorySpec.Name,
	}); err == nil {
		return dbRepo.Close()
	} else {
		return err
	}
}

func (c *dbCache) GetRepositories(ctx context.Context) []configapi.Repository {
	_, span := tracer.Start(ctx, "dbCache::GetRepositories", trace.WithAttributes())
	defer span.End()

	return []configapi.Repository{}
}
