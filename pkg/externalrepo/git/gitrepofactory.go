// Copyright 2025 The kpt and Nephio Authors
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

package git

import (
	"context"
	"errors"
	"path/filepath"

	configapi "github.com/nephio-project/porch/v4/api/porchconfig/v1alpha1"
	externalrepotypes "github.com/nephio-project/porch/v4/pkg/externalrepo/types"
	"github.com/nephio-project/porch/v4/pkg/repository"
)

var _ externalrepotypes.ExternalRepoFactory = &GitRepoFactory{}

type GitRepoFactory struct {
}

func (f *GitRepoFactory) NewRepositoryImpl(ctx context.Context, repositorySpec *configapi.Repository, options externalrepotypes.ExternalRepoOptions) (repository.Repository, error) {
	if repositorySpec.Spec.Git == nil {
		return nil, errors.New("git property is required")
	}
	if repositorySpec.Spec.Git.Repo == "" {
		return nil, errors.New("git.repo property is required")
	}

	var mbs MainBranchStrategy
	if repositorySpec.Spec.Git.CreateBranch {
		mbs = CreateIfMissing
	} else {
		mbs = ErrorIfMissing
	}

	repo, err := OpenRepository(ctx,
		repositorySpec.Name,
		repositorySpec.Namespace,
		repositorySpec.Spec.Git,
		repositorySpec.Spec.Deployment,
		filepath.Join(options.LocalDirectory, "git"),
		GitRepositoryOptions{
			ExternalRepoOptions: options,
			MainBranchStrategy:  mbs,
		})
	if err != nil {
		return nil, err
	}

	return repo, nil
}
