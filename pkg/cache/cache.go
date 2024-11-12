package cache

import (
	"context"

	configapi "github.com/nephio-project/porch/api/porchconfig/v1alpha1"
	"github.com/nephio-project/porch/pkg/repository"
)

type Cache interface {
	OpenRepository(ctx context.Context, repositorySpec *configapi.Repository) (repository.Repository, error)
	CloseRepository(repositorySpec *configapi.Repository, allRepos []configapi.Repository) error
}
