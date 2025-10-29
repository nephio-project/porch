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
	"errors"
	"strings"
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
	"github.com/stretchr/testify/mock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	k8sfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func (t *DbTestSuite) TestDBRepository() {
	shellRepo := dbRepository{
		repoKey: repository.RepositoryKey{
			Namespace: "my-ns",
			Name:      "my-repo-name",
		},
	}

	t.Equal("my-ns", shellRepo.KubeObjectNamespace())
	t.Equal("my-repo-name", shellRepo.KubeObjectName())
	t.Equal(types.UID("82d3ab92-4a01-5679-8c52-b1c3daf6f016"), shellRepo.UID())
	t.Equal(repository.RepositoryKey{Namespace: "my-ns", Name: "my-repo-name"}, shellRepo.Key())
}

func (t *DbTestSuite) TestDBRepositoryPRCrud() {
	mockCache := mockcachetypes.NewMockCache(t.T())
	cachetypes.CacheInstance = mockCache

	externalrepo.ExternalRepoInUnitTestMode = true

	ctx := t.Context()

	testRepo := t.createTestRepo("my-ns", "my-repo-name")
	mockCache.EXPECT().GetRepository(mock.Anything).Return(testRepo).Maybe()

	err := testRepo.OpenRepository(ctx, externalrepotypes.ExternalRepoOptions{})
	t.Require().NoError(err)

	version, err := testRepo.Version(t.Context())
	t.Require().NoError(err)
	t.Equal("", version)

	pkgList, err := testRepo.ListPackages(ctx, repository.ListPackageFilter{})
	t.Require().NoError(err)
	t.Equal(0, len(pkgList))

	newPkgDef := v1alpha1.PorchPackage{
		Spec: v1alpha1.PackageSpec{
			RepositoryName: "my-repo-name",
			PackageName:    "my-new-package",
		},
	}

	newPkg, err := testRepo.CreatePackage(ctx, &newPkgDef)
	t.Require().NoError(err)
	t.Require().NotNil(newPkg)

	pkgDef := newPkg.GetPackage(ctx)
	t.Require().NotNil(pkgDef)

	err = testRepo.DeletePackage(ctx, newPkg)
	t.Require().NoError(err)

	prList, err := testRepo.ListPackageRevisions(ctx, repository.ListPackageRevisionFilter{})
	t.Require().NoError(err)
	t.Equal(0, len(prList))

	newPRDef := v1alpha1.PackageRevision{
		Spec: v1alpha1.PackageRevisionSpec{
			RepositoryName: "my-repo-name",
			PackageName:    "my-package",
			WorkspaceName:  "my-workspace",
		},
	}
	newPRDraft, err := testRepo.CreatePackageRevisionDraft(ctx, &newPRDef)
	t.Require().NoError(err)
	t.Require().NotNil(newPRDraft)

	newPR, err := testRepo.ClosePackageRevisionDraft(ctx, newPRDraft, -1)
	t.Require().NoError(err)
	t.Require().NotNil(newPR)

	updatedPRDraft, err := testRepo.UpdatePackageRevision(ctx, newPR)
	t.Require().NoError(err)
	t.Require().NotNil(updatedPRDraft)

	err = updatedPRDraft.UpdateLifecycle(ctx, v1alpha1.PackageRevisionLifecycleProposed)
	t.Require().NoError(err)

	updatedPR, err := testRepo.ClosePackageRevisionDraft(ctx, updatedPRDraft, -1)
	t.Require().NoError(err)
	t.Require().NotNil(updatedPR)

	updatedPRDraft, err = testRepo.UpdatePackageRevision(ctx, newPR)
	t.Require().NoError(err)
	t.Require().NotNil(updatedPRDraft)

	err = updatedPRDraft.UpdateLifecycle(ctx, v1alpha1.PackageRevisionLifecyclePublished)
	t.Require().NoError(err)

	prMeta := updatedPR.GetMeta()
	prMeta.Finalizers = []string{"a-finalizer"}
	err = updatedPR.SetMeta(ctx, prMeta)
	t.Require().NoError(err)

	updatedPR, err = testRepo.ClosePackageRevisionDraft(ctx, updatedPRDraft, -1)
	t.Require().NoError(err)
	t.Require().NotNil(updatedPR)

	fakeRepo := testRepo.externalRepo.(*fake.Repository)
	fakeExtPR := fake.FakePackageRevision{
		PrKey: updatedPR.Key(),
	}
	fakeRepo.PackageRevisions = append(fakeRepo.PackageRevisions, &fakeExtPR)

	err = testRepo.DeletePackageRevision(ctx, updatedPR)
	t.Require().NoError(err)

	prMeta = updatedPR.GetMeta()
	prMeta.Finalizers = nil
	err = updatedPR.SetMeta(ctx, prMeta)
	t.Require().NoError(err)

	err = testRepo.DeletePackageRevision(ctx, updatedPR)
	t.Require().NoError(err)

	err = testRepo.Close(ctx)
	t.Require().NoError(err)
}

func (t *DbTestSuite) TestDBRepositorySync() {
	mockCache := mockcachetypes.NewMockCache(t.T())
	cachetypes.CacheInstance = mockCache

	externalrepo.ExternalRepoInUnitTestMode = true

	ctx := t.Context()

	testRepo := t.createTestRepo("my-ns", "my-repo-name")
	testRepo.spec = &configapi.Repository{
		Spec: configapi.RepositorySpec{},
	}
	mockCache.EXPECT().GetRepository(mock.Anything).Return(testRepo).Maybe()

	err := testRepo.OpenRepository(ctx, externalrepotypes.ExternalRepoOptions{})
	t.Require().NoError(err)

	scheme := runtime.NewScheme()
	_ = configapi.AddToScheme(scheme)

	repoObj := &configapi.Repository{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-repo-name",
			Namespace: "my-ns",
		},
	}
	fakeClient := k8sfake.NewClientBuilder().WithScheme(scheme).WithObjects(repoObj).Build()
	cacheOptions := cachetypes.CacheOptions{
		RepoCrSyncFrequency: 2 * time.Second,
		CoreClient:          fakeClient,
	}

	testRepo.repositorySync = newRepositorySync(testRepo, cacheOptions)

	err = testRepo.Refresh(ctx)
	t.Require().True(err == nil || strings.Contains(err.Error(), "already in progress"))

	err = testRepo.Close(ctx)
	t.Require().NoError(err)
}

func (t *DbTestSuite) TestDBRepositorCorner() {
	t.switchToMockSQL()

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

	mockCache := mockcachetypes.NewMockCache(t.T())
	cachetypes.CacheInstance = mockCache

	externalrepo.ExternalRepoInUnitTestMode = true

	ctx := t.Context()

	dbRepo := dbRepository{
		repoKey: repository.RepositoryKey{
			Namespace: "my-ns",
			Name:      "my-repo-name",
		},
	}
	err := repoWriteToDB(ctx, &dbRepo)
	t.Require().NotNil(err)

	t.revertToPostgreSQL()
}
