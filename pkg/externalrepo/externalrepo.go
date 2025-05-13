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

package externalrepo

import (
	"context"
	"errors"
	"fmt"

	configapi "github.com/nephio-project/porch/v4/api/porchconfig/v1alpha1"
	"github.com/nephio-project/porch/v4/pkg/externalrepo/git"
	"github.com/nephio-project/porch/v4/pkg/externalrepo/oci"
	externalrepotypes "github.com/nephio-project/porch/v4/pkg/externalrepo/types"
	"github.com/nephio-project/porch/v4/pkg/repository"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
)

var tracer = otel.Tracer("externalrepo")

func CreateRepositoryImpl(ctx context.Context, repositorySpec *configapi.Repository, options externalrepotypes.ExternalRepoOptions) (repository.Repository, error) {
	ctx, span := tracer.Start(ctx, "Repository::RepositoryFactory", trace.WithAttributes())
	defer span.End()

	var repoFactory externalrepotypes.ExternalRepoFactory

	switch repositoryType := repositorySpec.Spec.Type; repositoryType {
	case configapi.RepositoryTypeOCI:
		repoFactory = new(oci.OciRepoFactory)

	case configapi.RepositoryTypeGit:
		repoFactory = new(git.GitRepoFactory)

	default:
		return nil, fmt.Errorf("type %q not supported", repositoryType)
	}

	return repoFactory.NewRepositoryImpl(ctx, repositorySpec, options)
}

func RepositoryKey(repositorySpec *configapi.Repository) (string, error) {
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
