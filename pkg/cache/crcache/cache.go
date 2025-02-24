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
	mutex         sync.Mutex
	repositories  map[string]*cachedRepository
	metadataStore meta.MetadataStore
	options       cachetypes.CacheOptions
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

	cachedRepo := newRepository(key, repositorySpec, externalRepo, c.metadataStore, c.options)
	c.repositories[key] = cachedRepo

	return cachedRepo, nil
}

func (c *Cache) UpdateRepository(ctx context.Context, repositorySpec *configapi.Repository) error {
	klog.Infof("Update on CR cached repositories is not applicable")

	return nil
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

func (c *Cache) GetRepositories(ctx context.Context) []*configapi.Repository {
	repoSlice := []*configapi.Repository{}

	for _, repo := range c.repositories {
		repoSlice = append(repoSlice, repo.repoSpec)
	}
	return repoSlice
}
