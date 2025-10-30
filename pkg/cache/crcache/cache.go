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

package crcache

import (
	"context"
	"sync"
	"time"

	configapi "github.com/nephio-project/porch/api/porchconfig/v1alpha1"
	"github.com/nephio-project/porch/pkg/cache/crcache/meta"
	cachetypes "github.com/nephio-project/porch/pkg/cache/types"
	"github.com/nephio-project/porch/pkg/externalrepo"
	"github.com/nephio-project/porch/pkg/repository"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"k8s.io/klog/v2"
)

var tracer = otel.Tracer("crcache")

type Cache struct {
	repositories  map[repository.RepositoryKey]*cachedRepository
	mainLock      *sync.RWMutex
	locks         map[repository.RepositoryKey]*sync.Mutex
	cacheLocks    map[string]*sync.Mutex
	metadataStore meta.MetadataStore
	options       cachetypes.CacheOptions
}

var _ cachetypes.Cache = &Cache{}

func (c *Cache) OpenRepository(ctx context.Context, repositorySpec *configapi.Repository) (repository.Repository, error) {
	ctx, span := tracer.Start(ctx, "Cache::OpenRepository", trace.WithAttributes())
	defer span.End()
	start := time.Now()
	defer func() { klog.V(4).Infof("Cache::OpenRepository (%s) took %s", repositorySpec.Name, time.Since(start)) }()

	key, err := externalrepo.RepositoryKey(repositorySpec)
	if err != nil {
		return nil, err
	}

	cacheKey := c.getCacheKey(repositorySpec)
	cacheLock := c.getOrCreateCacheLock(cacheKey)
	cacheLock.Lock()
	defer cacheLock.Unlock()

	lock := c.getOrInsertLock(key)
	lock.Lock()
	defer lock.Unlock()

	c.mainLock.RLock()

	if repo, ok := c.repositories[key]; ok && repo != nil {
		c.mainLock.RUnlock()
		// Keep the spec updated in the cache.
		repo.repoSpec = repositorySpec
		// If there is an error from the background refresh goroutine, return it.
		if err := repo.getRefreshError(); err != nil {
			return nil, err
		}
		return repo, nil
	}
	c.mainLock.RUnlock()

	externalRepo, err := externalrepo.CreateRepositoryImpl(ctx, repositorySpec, c.options.ExternalRepoOptions)
	if err != nil {
		return nil, err
	}

	cachedRepo := newRepository(key, repositorySpec, externalRepo, c.metadataStore, c.options)

	c.mainLock.Lock()
	c.repositories[key] = cachedRepo
	c.mainLock.Unlock()

	return cachedRepo, nil
}

func (c *Cache) UpdateRepository(context.Context, *configapi.Repository) error {
	panic("Update on CR cached repositories is not applicable")
}

func (c *Cache) CloseRepository(ctx context.Context, repositorySpec *configapi.Repository, allRepos []configapi.Repository) error {
	_, span := tracer.Start(ctx, "Cache::CloseRepository", trace.WithAttributes())
	defer span.End()

	key, err := externalrepo.RepositoryKey(repositorySpec)
	if err != nil {
		return err
	}

	c.mainLock.RLock()
	repo, ok := c.repositories[key]
	c.mainLock.RUnlock()
	if !ok {
		return nil
	}

	cacheKey := c.getCacheKey(repositorySpec)
	cacheLock := c.getOrCreateCacheLock(cacheKey)
	cacheLock.Lock()
	defer cacheLock.Unlock()

	// check if repositorySpec shares the underlying cached repo with another repository
	for _, r := range allRepos {
		if r.Name == repositorySpec.Name && r.Namespace == repositorySpec.Namespace {
			continue
		}
		// For Git repositories, check sharing based on URL only
		if repositorySpec.Spec.Type == configapi.RepositoryTypeGit &&
			r.Spec.Type == configapi.RepositoryTypeGit &&
			r.Spec.Git.Repo == repositorySpec.Spec.Git.Repo {
			// do not close cached repo if it is shared, but cancel the polling goroutine
			klog.Infof("Not closing cached repository %q because it is shared", key)
			return nil
		}
	}

	lock := c.getOrInsertLock(key)
	lock.Lock()
	defer lock.Unlock()

	if ok {
		c.mainLock.Lock()
		delete(c.locks, key)
		delete(c.repositories, key)
		c.mainLock.Unlock()

		if repo != nil {
			return repo.Close(ctx)
		} else {
			klog.Warningf("cached repository with key %q had stored value nil", key)
		}
	} else {
		c.mainLock.Lock()
		delete(c.locks, key)
		c.mainLock.Unlock()
	}

	return nil
}

func (c *Cache) GetRepositories() []*configapi.Repository {
	repoSlice := []*configapi.Repository{}

	c.mainLock.RLock()
	defer c.mainLock.RUnlock()
	for _, repo := range c.repositories {
		repoSlice = append(repoSlice, repo.repoSpec)
	}

	return repoSlice
}

func (c *Cache) GetRepository(repoKey repository.RepositoryKey) repository.Repository {
	c.mainLock.RLock()
	defer c.mainLock.RUnlock()
	return c.repositories[repoKey]
}

func (c *Cache) CheckRepositoryConnectivity(ctx context.Context, repositorySpec *configapi.Repository) error {
	return externalrepo.CheckRepositoryConnection(ctx, repositorySpec, c.options.ExternalRepoOptions)
}

func (c *Cache) getOrInsertLock(key repository.RepositoryKey) *sync.Mutex {
	c.mainLock.RLock()
	if lock, exists := c.locks[key]; exists {
		c.mainLock.RUnlock()
		return lock
	}
	c.mainLock.RUnlock()

	c.mainLock.Lock()
	lock := &sync.Mutex{}
	c.locks[key] = lock
	c.mainLock.Unlock()

	return lock
}

func (c *Cache) getCacheKey(repositorySpec *configapi.Repository) string {
	if repositorySpec.Spec.Type == configapi.RepositoryTypeGit {
		return repositorySpec.Spec.Git.Repo
	}
	return repositorySpec.Name + "---" + repositorySpec.Namespace
}

func (c *Cache) getOrCreateCacheLock(cacheKey string) *sync.Mutex {
	c.mainLock.Lock()
	defer c.mainLock.Unlock()

	if c.cacheLocks == nil {
		c.cacheLocks = make(map[string]*sync.Mutex)
	}

	if lock, exists := c.cacheLocks[cacheKey]; exists {
		return lock
	}

	lock := &sync.Mutex{}
	c.cacheLocks[cacheKey] = lock
	return lock
}
