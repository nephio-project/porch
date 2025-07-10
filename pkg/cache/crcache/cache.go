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
	repositories  *sync.Map
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

	if cachedRepo, ok := c.repositories.Load(key); ok {
		repo := cachedRepo.(*cachedRepository)
		// Test if credentials are okay for the cached repo and update the status accordingly
		if _, err := externalrepo.CreateRepositoryImpl(ctx, repositorySpec, c.options.ExternalRepoOptions); err == nil {
			return repo, err
		} else {
			// If there is an error from the background refresh goroutine, return it.
			if err := repo.getRefreshError(); err == nil {
				return repo, nil
			}
		}
		return nil, err
	}

	externalRepo, err := externalrepo.CreateRepositoryImpl(ctx, repositorySpec, c.options.ExternalRepoOptions)
	if err != nil {
		return nil, err
	}

	cachedRepo := newRepository(key, repositorySpec, externalRepo, c.metadataStore, c.options)
	c.repositories.Store(key, cachedRepo)

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

	var repo *cachedRepository
	{
		if r, ok := c.repositories.LoadAndDelete(key); ok {
			repo = r.(*cachedRepository)
		}
	}

	if repo != nil {
		return repo.Close(ctx)
	}

	return nil
}

func (c *Cache) GetRepositories() []*configapi.Repository {
	repoSlice := []*configapi.Repository{}

	c.repositories.Range(func(key, repo any) bool {
		repoSlice = append(repoSlice, repo.(*cachedRepository).repoSpec)
		return true
	})

	return repoSlice
}

func (c *Cache) GetRepository(repoKey repository.RepositoryKey) repository.Repository {
	repo, _ := c.repositories.Load(repoKey)
	return repo.(*cachedRepository)
}
