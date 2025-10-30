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
	"fmt"
	"path/filepath"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing/transport"
	configapi "github.com/nephio-project/porch/api/porchconfig/v1alpha1"
	externalrepotypes "github.com/nephio-project/porch/pkg/externalrepo/types"
	"github.com/nephio-project/porch/pkg/repository"
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

func (f *GitRepoFactory) CheckRepositoryConnection(ctx context.Context, repositorySpec *configapi.Repository, options externalrepotypes.ExternalRepoOptions) error {
	// Nil checks
	if repositorySpec == nil || repositorySpec.Spec.Git == nil {
		return fmt.Errorf("repositorySpec is nil or missing Git configuration")
	}
	if repositorySpec.Spec.Git.Repo == "" {
		return fmt.Errorf("repository URL is empty")
	}
	if repositorySpec.Spec.Git.Branch == "" {
		return fmt.Errorf("target branch is empty")
	}

	// Fetch credentials from secret
	secretName := repositorySpec.Spec.Git.SecretRef.Name
	namespace := repositorySpec.Namespace
	var auth transport.AuthMethod
	if secretName != "" {
		creds, err := options.CredentialResolver.ResolveCredential(ctx, namespace, secretName)
		if err != nil {
			return fmt.Errorf("failed to resolve credentials: %w", err)
		}

		if !creds.Valid() {
			return fmt.Errorf("resolved credentials are invalid")
		}

		// Use the credentials for Git authentication
		auth = creds.ToAuthMethod()
	}
	// Check if branch exists
	remote := gogit.NewRemote(nil, &config.RemoteConfig{
		Name: "origin",
		URLs: []string{repositorySpec.Spec.Git.Repo},
	})

	refs, err := remote.List(&gogit.ListOptions{
		Auth: auth,
	})
	if err != nil {
		return fmt.Errorf("failed to list remote refs: %w", err)
	}

	branchExists := false
	for _, ref := range refs {
		if ref.Name().IsBranch() && ref.Name().Short() == repositorySpec.Spec.Git.Branch {
			branchExists = true
			break
		}
	}

	if !branchExists {
		return fmt.Errorf("branch %q not found in repository", repositorySpec.Spec.Git.Branch)
	}

	return nil
}
