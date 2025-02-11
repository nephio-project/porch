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

package memorycache

import (
	"context"
	"errors"
	"sync"

	configapi "github.com/nephio-project/porch/api/porchconfig/v1alpha1"
	cachetypes "github.com/nephio-project/porch/pkg/cache/types"
	"github.com/nephio-project/porch/pkg/externalrepo"
	"github.com/nephio-project/porch/pkg/repository"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
)

var tracer = otel.Tracer("memorycache")

// Cache allows us to keep state for repositories, rather than querying them every time.
//
// Cache Structure:
// <cacheDir>/git/
// * Caches bare git repositories in directories named based on the repository address.
// <cacheDir>/oci/
// * Caches oci images with further hierarchy underneath
// * We Cache image layers in <cacheDir>/oci/layers/ (this might be obsolete with the flattened Cache)
// * We Cache flattened tar files in <cacheDir>/oci/ (so we don't need to pull to read resources)
// * We poll the repositories periodically (configurable) and cache the discovered images in memory.
type Cache struct {
	mutex        sync.Mutex
	repositories map[string]*cachedRepository
	options      cachetypes.CacheOptions
}

var _ cachetypes.Cache = &Cache{}

func (c *Cache) OpenRepository(ctx context.Context, repositorySpec *configapi.Repository) (repository.Repository, error) {
	ctx, span := tracer.Start(ctx, "Cache::OpenRepository", trace.WithAttributes())
	defer span.End()

	key, err := externalrepo.RepositoryKey(repositorySpec)
	if err != nil {
		return nil, err
	}

	c.mutex.Lock()
	defer c.mutex.Unlock()

	if cachedRepo := c.repositories[key]; cachedRepo != nil {
		// If there is an error from the background refresh goroutine, return it.
		if err := cachedRepo.getRefreshError(); err == nil {
			return cachedRepo, nil
		} else {
			return nil, err
		}
	}

	externalRepo, err := externalrepo.CreateRepositoryImpl(ctx, repositorySpec, c.options.ExternalRepoOptions)
	if err != nil {
		return nil, err
	}

	cachedRepo := newRepository(key, repositorySpec, externalRepo, c.options)
	c.repositories[key] = cachedRepo

	return cachedRepo, nil
}

func (c *Cache) UpdateRepository(ctx context.Context, repositorySpec *configapi.Repository) error {
	return errors.New("update on memory cached repositories is not supported")
}

func (c *Cache) CloseRepository(ctx context.Context, repositorySpec *configapi.Repository, allRepos []configapi.Repository) error {
	_, span := tracer.Start(ctx, "Cache::CloseRepository", trace.WithAttributes())
	defer span.End()

	key, err := externalrepo.RepositoryKey(repositorySpec)
	if err != nil {
		return err
	}

	// check if repositorySpec shares the underlying cached repo with another repository
	for _, r := range allRepos {
		if r.Name == repositorySpec.Name && r.Namespace == repositorySpec.Namespace {
			continue
		}
		otherKey, err := externalrepo.RepositoryKey(&r)
		if err != nil {
			return err
		}
		if otherKey == key {
			// do not close cached repo if it is shared
			return nil
		}
	}

	var repository *cachedRepository
	{
		c.mutex.Lock()
		if r, ok := c.repositories[key]; ok {
			delete(c.repositories, key)
			repository = r
		}
		c.mutex.Unlock()
	}

	if repository != nil {
		return repository.Close()
	} else {
		return nil
	}
}

func (c *Cache) GetRepositories(ctx context.Context) []configapi.Repository {
	repoSlice := []configapi.Repository{}

	for _, repo := range c.repositories {
		repoSlice = append(repoSlice, *repo.repoSpec)
	}
	return repoSlice
}
