// Copyright 2025 The Nephio Authors
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

package dbcache

import (
	"context"
	"testing"

	"github.com/nephio-project/porch/api/porch/v1alpha1"
	configapi "github.com/nephio-project/porch/api/porchconfig/v1alpha1"
	cachetypes "github.com/nephio-project/porch/pkg/cache/types"
	"github.com/nephio-project/porch/pkg/externalrepo"
	externalrepotypes "github.com/nephio-project/porch/pkg/externalrepo/types"
	"github.com/nephio-project/porch/pkg/repository"
	mockcachetypes "github.com/nephio-project/porch/test/mockery/mocks/porch/pkg/cache/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"k8s.io/apimachinery/pkg/types"
)

func TestDBRepository(t *testing.T) {
	shellRepo := dbRepository{
		repoKey: repository.RepositoryKey{
			Namespace: "my-ns",
			Name:      "my-repo-name",
		},
	}

	assert.Equal(t, "my-ns", shellRepo.KubeObjectNamespace())
	assert.Equal(t, "my-repo-name", shellRepo.KubeObjectName())
	assert.Equal(t, types.UID("82d3ab92-4a01-5679-8c52-b1c3daf6f016"), shellRepo.UID())
	assert.Equal(t, repository.RepositoryKey{Namespace: "my-ns", Name: "my-repo-name"}, shellRepo.Key())
}

func TestDBRepositoryCrud(t *testing.T) {
	mockCache := mockcachetypes.NewMockCache(t)
	cachetypes.CacheInstance = mockCache

	externalrepo.ExternalRepoInUnitTestMode = true

	ctx := context.TODO()

	testRepo := createTestRepo(t, "my-ns", "my-repo-name")
	testRepo.spec = &configapi.Repository{
		Spec: configapi.RepositorySpec{},
	}
	mockCache.EXPECT().GetRepository(mock.Anything).Return(&testRepo)

	err := testRepo.OpenRepository(ctx, externalrepotypes.ExternalRepoOptions{})
	assert.Nil(t, err)

	prList, err := testRepo.ListPackageRevisions(ctx, repository.ListPackageRevisionFilter{})
	assert.Nil(t, err)
	assert.Equal(t, 0, len(prList))

	newPRDef := v1alpha1.PackageRevision{
		Spec: v1alpha1.PackageRevisionSpec{
			RepositoryName: "my-repo-name",
			PackageName:    "my-package",
			WorkspaceName:  "my-workspace",
		},
	}
	newPRDraft, err := testRepo.CreatePackageRevisionDraft(ctx, &newPRDef)
	assert.Nil(t, err)
	assert.NotNil(t, newPRDraft)

	newPR, err := testRepo.ClosePackageRevisionDraft(ctx, newPRDraft, -1)
	assert.Nil(t, err)
	assert.NotNil(t, newPR)

	updatedPRDraft, err := testRepo.UpdatePackageRevision(ctx, newPR)
	assert.Nil(t, err)
	assert.NotNil(t, updatedPRDraft)

	updatedPR, err := testRepo.ClosePackageRevisionDraft(ctx, updatedPRDraft, -1)
	assert.Nil(t, err)
	assert.NotNil(t, updatedPR)

	err = testRepo.DeletePackageRevision(ctx, updatedPR)
	assert.Nil(t, err)

	err = testRepo.Close(ctx)
	assert.Nil(t, err)
}
