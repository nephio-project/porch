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

package memory

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	kptoci "github.com/GoogleContainerTools/kpt/pkg/oci"
	configapi "github.com/nephio-project/porch/api/porchconfig/v1alpha1"
	"github.com/nephio-project/porch/pkg/cache"
	"github.com/nephio-project/porch/pkg/git"
	"github.com/nephio-project/porch/pkg/meta"
	"github.com/nephio-project/porch/pkg/oci"
	"github.com/nephio-project/porch/pkg/repository"
	"go.opentelemetry.io/otel/trace"
	"k8s.io/apimachinery/pkg/watch"
)

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
type Cache struct {
	mutex              sync.Mutex
	repositories       map[string]*cachedRepository
	cacheDir           string
	credentialResolver repository.CredentialResolver
	userInfoProvider   repository.UserInfoProvider
	metadataStore      meta.MetadataStore
	repoSyncFrequency  time.Duration
	objectNotifier     objectNotifier
	useGitCaBundle     bool
}

var _ cache.Cache = &Cache{}

type objectNotifier interface {
	NotifyPackageRevisionChange(eventType watch.EventType, obj repository.PackageRevision) int
}

type CacheOptions struct {
	CredentialResolver repository.CredentialResolver
	UserInfoProvider   repository.UserInfoProvider
	MetadataStore      meta.MetadataStore
	ObjectNotifier     objectNotifier
}

func NewCache(cacheDir string, repoSyncFrequency time.Duration, useGitCaBundle bool, opts CacheOptions) *Cache {
	return &Cache{
		repositories:       make(map[string]*cachedRepository),
		cacheDir:           cacheDir,
		credentialResolver: opts.CredentialResolver,
		userInfoProvider:   opts.UserInfoProvider,
		metadataStore:      opts.MetadataStore,
		objectNotifier:     opts.ObjectNotifier,
		repoSyncFrequency:  repoSyncFrequency,
		useGitCaBundle:     useGitCaBundle,
	}
}

func getCacheKey(repositorySpec *configapi.Repository) (string, error) {
	switch repositoryType := repositorySpec.Spec.Type; repositoryType {
	case configapi.RepositoryTypeOCI:
		ociSpec := repositorySpec.Spec.Oci
		if ociSpec == nil {
			return "", fmt.Errorf("oci not configured")
		}
		return "oci://" + ociSpec.Registry, nil

	case configapi.RepositoryTypeGit:
		gitSpec := repositorySpec.Spec.Git
		if gitSpec == nil {
			return "", errors.New("git property is required")
		}
		if gitSpec.Repo == "" {
			return "", errors.New("git.repo property is required")
		}
		return fmt.Sprintf("git://%s/%s@%s/%s", gitSpec.Repo, gitSpec.Directory, repositorySpec.Namespace, repositorySpec.Name), nil

	default:
		return "", fmt.Errorf("repository type %q not supported", repositoryType)
	}
}

func (c *Cache) OpenRepository(ctx context.Context, repositorySpec *configapi.Repository) (repository.Repository, error) {
	ctx, span := tracer.Start(ctx, "Cache::OpenRepository", trace.WithAttributes())
	defer span.End()

	key, err := getCacheKey(repositorySpec)
	if err != nil {
		return nil, err
	}
	c.mutex.Lock()
	defer c.mutex.Unlock()
	cachedRepo := c.repositories[key]

	switch repositoryType := repositorySpec.Spec.Type; repositoryType {
	case configapi.RepositoryTypeOCI:
		ociSpec := repositorySpec.Spec.Oci
		if cachedRepo == nil {
			cacheDir := filepath.Join(c.cacheDir, "oci")
			storage, err := kptoci.NewStorage(cacheDir)
			if err != nil {
				return nil, err
			}

			r, err := oci.OpenRepository(repositorySpec.Name, repositorySpec.Namespace, ociSpec, repositorySpec.Spec.Deployment, storage)
			if err != nil {
				return nil, err
			}
			cachedRepo = newRepository(key, repositorySpec, r, c.objectNotifier, c.metadataStore, c.repoSyncFrequency)
			c.repositories[key] = cachedRepo
		}
		return cachedRepo, nil

	case configapi.RepositoryTypeGit:
		gitSpec := repositorySpec.Spec.Git
		if cachedRepo == nil {
			var mbs git.MainBranchStrategy
			if gitSpec.CreateBranch {
				mbs = git.CreateIfMissing
			} else {
				mbs = git.ErrorIfMissing
			}

			r, err := git.OpenRepository(ctx, repositorySpec.Name, repositorySpec.Namespace, gitSpec, repositorySpec.Spec.Deployment, filepath.Join(c.cacheDir, "git"), git.GitRepositoryOptions{
				CredentialResolver: c.credentialResolver,
				UserInfoProvider:   c.userInfoProvider,
				MainBranchStrategy: mbs,
				UseGitCaBundle:     c.useGitCaBundle,
			})
			if err != nil {
				return nil, err
			}

			cachedRepo = newRepository(key, repositorySpec, r, c.objectNotifier, c.metadataStore, c.repoSyncFrequency)
			c.repositories[key] = cachedRepo
		} else {
			// If there is an error from the background refresh goroutine, return it.
			if err := cachedRepo.getRefreshError(); err != nil {
				return nil, err
			}
		}
		return cachedRepo, nil

	default:
		return nil, fmt.Errorf("type %q not supported", repositoryType)
	}
}

func (c *Cache) CloseRepository(repositorySpec *configapi.Repository, allRepos []configapi.Repository) error {
	key, err := getCacheKey(repositorySpec)
	if err != nil {
		return err
	}

	// check if repositorySpec shares the underlying cached repo with another repository
	for _, r := range allRepos {
		if r.Name == repositorySpec.Name && r.Namespace == repositorySpec.Namespace {
			continue
		}
		otherKey, err := getCacheKey(&r)
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
