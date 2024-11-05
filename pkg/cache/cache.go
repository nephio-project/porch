package cache

import (
	"context"

	configapi "github.com/nephio-project/porch/api/porchconfig/v1alpha1"
	"github.com/nephio-project/porch/pkg/repository"
)

type Cache interface {
	OpenRepository(ctx context.Context, repositorySpec *configapi.Repository) (CachedRepository, error)
	CloseRepository(repositorySpec *configapi.Repository, allRepos []configapi.Repository) error
}

type CachedRepository interface {
	repository.Repository
	RefreshCache(ctx context.Context) error
}

type CachedPackageRevision interface {
	repository.PackageRevision
}

type CachedPackageDraft interface {
	repository.PackageDraft
}

// Remove?
type CachedPackage interface {
	repository.Package
}
