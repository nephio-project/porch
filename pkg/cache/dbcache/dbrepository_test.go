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
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/nephio-project/porch/api/porch/v1alpha1"
	configapi "github.com/nephio-project/porch/api/porchconfig/v1alpha1"
	cachetypes "github.com/nephio-project/porch/pkg/cache/types"
	"github.com/nephio-project/porch/pkg/externalrepo"
	"github.com/nephio-project/porch/pkg/externalrepo/fake"
	externalrepotypes "github.com/nephio-project/porch/pkg/externalrepo/types"
	"github.com/nephio-project/porch/pkg/repository"
	mocksql "github.com/nephio-project/porch/test/mockery/mocks/porch/pkg/cache/dbcache"
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

func TestDBRepositoryPRCrud(t *testing.T) {
	mockCache := mockcachetypes.NewMockCache(t)
	cachetypes.CacheInstance = mockCache

	externalrepo.ExternalRepoInUnitTestMode = true

	ctx := context.TODO()

	testRepo := createTestRepo(t, "my-ns", "my-repo-name")
	mockCache.EXPECT().GetRepository(mock.Anything).Return(&testRepo).Maybe()

	err := testRepo.OpenRepository(ctx, externalrepotypes.ExternalRepoOptions{})
	assert.Nil(t, err)

	version, err := testRepo.Version(context.TODO())
	assert.Nil(t, err)
	assert.Equal(t, "", version)

	pkgList, err := testRepo.ListPackages(ctx, repository.ListPackageFilter{})
	assert.Nil(t, err)
	assert.Equal(t, 0, len(pkgList))

	newPkgDef := v1alpha1.PorchPackage{
		Spec: v1alpha1.PackageSpec{
			RepositoryName: "my-repo-name",
			PackageName:    "my-new-package",
		},
	}

	newPkg, err := testRepo.CreatePackage(ctx, &newPkgDef)
	assert.Nil(t, err)
	assert.NotNil(t, newPkg)

	pkgDef := newPkg.GetPackage(ctx)
	assert.NotNil(t, pkgDef)

	err = testRepo.DeletePackage(ctx, newPkg)
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

	err = updatedPRDraft.UpdateLifecycle(ctx, v1alpha1.PackageRevisionLifecycleProposed)
	assert.Nil(t, err)

	updatedPR, err := testRepo.ClosePackageRevisionDraft(ctx, updatedPRDraft, -1)
	assert.Nil(t, err)
	assert.NotNil(t, updatedPR)

	updatedPRDraft, err = testRepo.UpdatePackageRevision(ctx, newPR)
	assert.Nil(t, err)
	assert.NotNil(t, updatedPRDraft)

	err = updatedPRDraft.UpdateLifecycle(ctx, v1alpha1.PackageRevisionLifecyclePublished)
	assert.Nil(t, err)

	prMeta := updatedPR.GetMeta()
	prMeta.Finalizers = []string{"a-finalizer"}
	err = updatedPR.SetMeta(ctx, prMeta)
	assert.Nil(t, err)

	updatedPR, err = testRepo.ClosePackageRevisionDraft(ctx, updatedPRDraft, -1)
	assert.Nil(t, err)
	assert.NotNil(t, updatedPR)

	fakeRepo := testRepo.externalRepo.(*fake.Repository)
	fakeExtPR := fake.FakePackageRevision{
		PrKey: updatedPR.Key(),
	}
	fakeRepo.PackageRevisions = append(fakeRepo.PackageRevisions, &fakeExtPR)

	err = testRepo.DeletePackageRevision(ctx, updatedPR)
	assert.Nil(t, err)

	prMeta = updatedPR.GetMeta()
	prMeta.Finalizers = nil
	err = updatedPR.SetMeta(ctx, prMeta)
	assert.Nil(t, err)

	err = testRepo.DeletePackageRevision(ctx, updatedPR)
	assert.Nil(t, err)

	err = testRepo.Close(ctx)
	assert.Nil(t, err)
}

func TestDBRepositorySync(t *testing.T) {
	mockCache := mockcachetypes.NewMockCache(t)
	cachetypes.CacheInstance = mockCache

	externalrepo.ExternalRepoInUnitTestMode = true

	ctx := context.TODO()

	testRepo := createTestRepo(t, "my-ns", "my-repo-name")
	testRepo.spec = &configapi.Repository{
		Spec: configapi.RepositorySpec{},
	}
	mockCache.EXPECT().GetRepository(mock.Anything).Return(&testRepo).Maybe()

	err := testRepo.OpenRepository(ctx, externalrepotypes.ExternalRepoOptions{})
	assert.Nil(t, err)

	cacheOptions := cachetypes.CacheOptions{
		RepoSyncFrequency: 2 * time.Second,
	}

	testRepo.repositorySync = newRepositorySync(&testRepo, cacheOptions)
	assert.NotNil(t, testRepo.repositorySync)

	err = testRepo.Refresh(ctx)
	assert.True(t, err == nil || strings.Contains(err.Error(), "already in progress"))

	err = testRepo.Close(ctx)
	assert.Nil(t, err)
}

func TestDBRepositorCorner(t *testing.T) {
	switchToMockSQL(t)

	mockSQL := dbHandler.db.(*mocksql.MockdbSQLInterface)
	mockSQL.EXPECT().Exec(
		mock.Anything,
		mock.Anything,
		mock.Anything,
		mock.Anything,
		mock.Anything,
		mock.Anything,
		mock.Anything,
		mock.Anything,
		mock.Anything,
		mock.Anything).Return(nil, errors.New("DB exec failed")).Maybe()

	mockSQL.EXPECT().Close().Return(errors.New("DB close failed")).Maybe()

	mockCache := mockcachetypes.NewMockCache(t)
	cachetypes.CacheInstance = mockCache

	externalrepo.ExternalRepoInUnitTestMode = true

	ctx := context.TODO()

	dbRepo := dbRepository{
		repoKey: repository.RepositoryKey{
			Namespace: "my-ns",
			Name:      "my-repo-name",
		},
	}
	err := repoWriteToDB(ctx, &dbRepo)
	assert.NotNil(t, err)

	revertToPostgreSQL(t)
}
