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

package repoimpltypes

import (
	"context"

	configapi "github.com/nephio-project/porch/api/porchconfig/v1alpha1"
	"github.com/nephio-project/porch/pkg/repository"
)

type RepoImplFactory interface {
	NewRepositoryImpl(ctx context.Context, repositorySpec *configapi.Repository, options RepoImplOptions) (repository.Repository, error)
}

type RepoImplOptions struct {
	LocalDirectory         string
	UseUserDefinedCaBundle bool
	CredentialResolver     repository.CredentialResolver
	UserInfoProvider       repository.UserInfoProvider
}