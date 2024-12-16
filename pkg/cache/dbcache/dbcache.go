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
	"sync"
	"time"

	configapi "github.com/nephio-project/porch/api/porchconfig/v1alpha1"
	"github.com/nephio-project/porch/pkg/cache"
	"github.com/nephio-project/porch/pkg/meta"
	"github.com/nephio-project/porch/pkg/repository"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"k8s.io/apimachinery/pkg/watch"
)

var _ cache.Cache = &dbCache{}
var tracer = otel.Tracer("dbcache")

// Cache allows us to keep state for repositories, rather than querying them every time.
//
// Cache Structure:
// <cacheDir>/git/
// * Caches bare git repositories in directories named based on the repository address.
// <cacheDir>/oci/
// * Caches oci images with further hierarchy underneath
// * We Cache image layers in <cacheDir>/oci/layers/ (this might be obsolete with the flattened Cache)
// * We Cache flattened tar files in <cacheDir>/oci/ (so we don't need to pull to read resources)
// * We poll the repositories (every minute) and Cache the discovered images in memory.
type dbCache struct {
	mutex              sync.Mutex
	cacheDir           string
	credentialResolver repository.CredentialResolver
	userInfoProvider   repository.UserInfoProvider
	repoSyncFrequency  time.Duration
	objectNotifier     objectNotifier
	useGitCaBundle     bool
}

type objectNotifier interface {
	NotifyPackageRevisionChange(eventType watch.EventType, obj repository.PackageRevision) int
}

type CacheOptions struct {
	Driver             string
	DataSource         string
	CredentialResolver repository.CredentialResolver
	UserInfoProvider   repository.UserInfoProvider
	MetadataStore      meta.MetadataStore
	ObjectNotifier     objectNotifier
}

func NewCache(cacheDir string, repoSyncFrequency time.Duration, useGitCaBundle bool, opts CacheOptions) cache.Cache {

	if err := OpenDBConnection(opts); err != nil {
		return nil
	}

	return &dbCache{
		cacheDir:           cacheDir,
		credentialResolver: opts.CredentialResolver,
		userInfoProvider:   opts.UserInfoProvider,
		objectNotifier:     opts.ObjectNotifier,
		repoSyncFrequency:  repoSyncFrequency,
		useGitCaBundle:     useGitCaBundle,
	}
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
