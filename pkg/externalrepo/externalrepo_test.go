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
	"testing"

	configapi "github.com/nephio-project/porch/api/porchconfig/v1alpha1"
	externalrepotypes "github.com/nephio-project/porch/pkg/externalrepo/types"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestExternaRepo(t *testing.T) {
	ctx := context.TODO()
	repoSpec := configapi.Repository{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "repo-ns",
			Name:      "repo-name",
		},
		Spec: configapi.RepositorySpec{
			Type: configapi.RepositoryTypeOCI,
		},
	}

	repo, err := CreateRepositoryImpl(ctx, &repoSpec, externalrepotypes.ExternalRepoOptions{})
	assert.NotNil(t, err)
	assert.Nil(t, repo)

	_, err = RepositoryKey(&repoSpec)
	assert.NotNil(t, err)
	assert.Equal(t, "oci not configured", err.Error())

	repoSpec.Spec.Oci = &configapi.OciRepository{
		Registry: "OCIRegistry",
	}

	ociRepoKey, err := RepositoryKey(&repoSpec)
	assert.Nil(t, err)
	assert.NotNil(t, ociRepoKey)
	assert.Equal(t, "oci://OCIRegistry", ociRepoKey.Path)

	repoSpec.Spec.Type = configapi.RepositoryTypeGit
	repo, err = CreateRepositoryImpl(ctx, &repoSpec, externalrepotypes.ExternalRepoOptions{})
	assert.NotNil(t, err)
	assert.Nil(t, repo)

	_, err = RepositoryKey(&repoSpec)
	assert.NotNil(t, err)
	assert.Equal(t, "git property is required", err.Error())

	repoSpec.Spec.Git = &configapi.GitRepository{}
	_, err = RepositoryKey(&repoSpec)
	assert.NotNil(t, err)
	assert.Equal(t, "git.repo property is required", err.Error())

	repoSpec.Spec.Git.Repo = "repo-name"
	repoSpec.Spec.Git.Branch = "git-branch"
	gitRepoKey, err := RepositoryKey(&repoSpec)
	assert.Nil(t, err)
	assert.Equal(t, "git-branch", gitRepoKey.PlaceholderWSname)

	repoSpec.Spec.Type = "some-type"
	repo, err = CreateRepositoryImpl(ctx, &repoSpec, externalrepotypes.ExternalRepoOptions{})
	assert.NotNil(t, err)
	assert.Nil(t, repo)

	_, err = RepositoryKey(&repoSpec)
	assert.NotNil(t, err)
	assert.Equal(t, "repository type \"some-type\" not supported", err.Error())

	ExternalRepoInUnitTestMode = true
	repoSpec.Spec.Type = ""
	repo, err = CreateRepositoryImpl(ctx, &repoSpec, externalrepotypes.ExternalRepoOptions{})
	assert.Nil(t, err)
	assert.NotNil(t, repo)

	defaultRepoKey, err := RepositoryKey(&repoSpec)
	assert.Nil(t, err)
	assert.Equal(t, "repo-name", defaultRepoKey.Name)

}
