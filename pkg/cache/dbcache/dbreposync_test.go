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
	"time"

	"github.com/nephio-project/porch/api/porch/v1alpha1"
	cachetypes "github.com/nephio-project/porch/pkg/cache/types"
	"github.com/nephio-project/porch/pkg/externalrepo"
	"github.com/nephio-project/porch/pkg/externalrepo/fake"
	externalrepotypes "github.com/nephio-project/porch/pkg/externalrepo/types"
	v1 "github.com/nephio-project/porch/pkg/kpt/api/kptfile/v1"
	"github.com/nephio-project/porch/pkg/repository"
	mockcachetypes "github.com/nephio-project/porch/test/mockery/mocks/porch/pkg/cache/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestDBUpstreamRepoSync(t *testing.T) {
	mockCache := mockcachetypes.NewMockCache(t)
	cachetypes.CacheInstance = mockCache

	externalrepo.ExternalRepoInUnitTestMode = true

	ctx := context.TODO()

	testRepo := createTestRepo(t, "my-ns", "my-repo-name")
	mockCache.EXPECT().GetRepository(mock.Anything).Return(&testRepo).Maybe()

	err := testRepo.OpenRepository(ctx, externalrepotypes.ExternalRepoOptions{})
	assert.Nil(t, err)

	cacheOptions := cachetypes.CacheOptions{
		RepoSyncFrequency: 1 * time.Second,
	}

	testRepo.repositorySync = newRepositorySync(&testRepo, cacheOptions)

	newPRDef := v1alpha1.PackageRevision{
		Spec: v1alpha1.PackageRevisionSpec{
			RepositoryName: "my-repo-name",
			PackageName:    "my-package",
			WorkspaceName:  "my-workspace",
			Lifecycle:      v1alpha1.PackageRevisionLifecyclePublished,
		},
	}
	dbPRDraft, err := testRepo.CreatePackageRevisionDraft(ctx, &newPRDef)
	assert.Nil(t, err)
	assert.NotNil(t, dbPRDraft)

	dbPR, err := testRepo.ClosePackageRevisionDraft(ctx, dbPRDraft, 0)
	assert.Nil(t, err)

	err = dbPR.UpdateLifecycle(ctx, v1alpha1.PackageRevisionLifecycleProposed)
	assert.Nil(t, err)

	dbPR, err = testRepo.ClosePackageRevisionDraft(ctx, dbPR.(repository.PackageRevisionDraft), 0)
	assert.Nil(t, err)
	assert.NotNil(t, dbPR)

	err = dbPR.UpdateLifecycle(ctx, v1alpha1.PackageRevisionLifecyclePublished)
	assert.Nil(t, err)

	dbPR, err = testRepo.ClosePackageRevisionDraft(ctx, dbPR.(repository.PackageRevisionDraft), 0)
	assert.Nil(t, err)
	assert.NotNil(t, dbPR)

	time.Sleep(2 * time.Second)

	prList, err := testRepo.ListPackageRevisions(ctx, repository.ListPackageRevisionFilter{})
	assert.Nil(t, err)
	assert.Equal(t, 0, len(prList)) // Sync should have deleted the cached PR that is not in the external repo

	// Add the PR to the external repo
	fakeRepo := testRepo.externalRepo.(*fake.Repository)
	fakeExtPR := fake.FakePackageRevision{
		PrKey:           dbPR.Key(),
		PackageRevision: &newPRDef,
		Resources:       &v1alpha1.PackageRevisionResources{},
		Kptfile: v1.KptFile{
			Upstream:     &v1.Upstream{},
			UpstreamLock: &v1.UpstreamLock{},
		},
	}
	fakeRepo.PackageRevisions = append(fakeRepo.PackageRevisions, &fakeExtPR)

	time.Sleep(2 * time.Second)

	prList, err = testRepo.ListPackageRevisions(ctx, repository.ListPackageRevisionFilter{})
	assert.Nil(t, err)
	assert.Equal(t, 0, len(prList)) // The version of the external repo has not changed

	fakeRepo.CurrentVersion = "bar"

	time.Sleep(2 * time.Second)

	prList, err = testRepo.ListPackageRevisions(ctx, repository.ListPackageRevisionFilter{})
	assert.Nil(t, err)
	assert.Equal(t, 1, len(prList)) // Sync should have added a cached PR that is in the external repo

	testRepo.repositorySync.Stop()

	err = testRepo.Close(ctx)
	assert.Nil(t, err)
}

func TestDBDeployRepoSync(t *testing.T) {
	mockCache := mockcachetypes.NewMockCache(t)
	cachetypes.CacheInstance = mockCache

	externalrepo.ExternalRepoInUnitTestMode = true

	ctx := context.TODO()

	testRepo := createTestRepo(t, "my-ns", "my-repo-name")
	testRepo.deployment = true

	mockCache.EXPECT().GetRepository(mock.Anything).Return(&testRepo).Maybe()

	err := testRepo.OpenRepository(ctx, externalrepotypes.ExternalRepoOptions{})
	assert.Nil(t, err)

	cacheOptions := cachetypes.CacheOptions{
		RepoSyncFrequency: 1 * time.Second,
	}

	testRepo.repositorySync = newRepositorySync(&testRepo, cacheOptions)

	newPRDef := v1alpha1.PackageRevision{
		Spec: v1alpha1.PackageRevisionSpec{
			RepositoryName: "my-repo-name",
			PackageName:    "my-package",
			WorkspaceName:  "my-workspace",
			Lifecycle:      v1alpha1.PackageRevisionLifecyclePublished,
		},
	}
	dbPRDraft, err := testRepo.CreatePackageRevisionDraft(ctx, &newPRDef)
	assert.Nil(t, err)
	assert.NotNil(t, dbPRDraft)

	dbPR, err := testRepo.ClosePackageRevisionDraft(ctx, dbPRDraft, 0)
	assert.Nil(t, err)

	// Add the PR to the external repo
	fakeRepo := testRepo.externalRepo.(*fake.Repository)
	fakeExtPR := fake.FakePackageRevision{
		PrKey:           dbPR.Key(),
		PackageRevision: &newPRDef,
		Resources:       &v1alpha1.PackageRevisionResources{},
	}
	fakeRepo.PackageRevisions = append(fakeRepo.PackageRevisions, &fakeExtPR)

	err = dbPR.UpdateLifecycle(ctx, v1alpha1.PackageRevisionLifecycleProposed)
	assert.Nil(t, err)

	dbPR, err = testRepo.ClosePackageRevisionDraft(ctx, dbPR.(repository.PackageRevisionDraft), 0)
	assert.Nil(t, err)
	assert.NotNil(t, dbPR)

	err = dbPR.UpdateLifecycle(ctx, v1alpha1.PackageRevisionLifecyclePublished)
	assert.Nil(t, err)

	dbPR, err = testRepo.ClosePackageRevisionDraft(ctx, dbPR.(repository.PackageRevisionDraft), 0)
	assert.Nil(t, err)
	assert.NotNil(t, dbPR)

	time.Sleep(2 * time.Second)

	prList, err := testRepo.ListPackageRevisions(ctx, repository.ListPackageRevisionFilter{})
	assert.Nil(t, err)
	assert.Equal(t, 2, len(prList))

	time.Sleep(2 * time.Second)

	prList, err = testRepo.ListPackageRevisions(ctx, repository.ListPackageRevisionFilter{})
	assert.Nil(t, err)
	assert.Equal(t, 2, len(prList))

	fakeRepo.CurrentVersion = "bar"

	time.Sleep(2 * time.Second)

	prList, err = testRepo.ListPackageRevisions(ctx, repository.ListPackageRevisionFilter{})
	assert.Nil(t, err)
	assert.Equal(t, 2, len(prList))

	testRepo.repositorySync.Stop()

	err = testRepo.Close(ctx)
	assert.Nil(t, err)
}

func TestRepoSyncTickTock(t *testing.T) {
	mockCache := mockcachetypes.NewMockCache(t)
	cachetypes.CacheInstance = mockCache

	externalrepo.ExternalRepoInUnitTestMode = true

	ctx := context.TODO()

	testRepo := createTestRepo(t, "my-ns", "my-repo-name")
	mockCache.EXPECT().GetRepository(mock.Anything).Return(&testRepo).Maybe()

	err := testRepo.OpenRepository(ctx, externalrepotypes.ExternalRepoOptions{})
	assert.Nil(t, err)

	cacheOptions := cachetypes.CacheOptions{
		RepoSyncFrequency: 4 * time.Second,
	}

	testRepo.repositorySync = newRepositorySync(&testRepo, cacheOptions)
	testRepo.repositorySync.Stop()

	repoSyncImpl := testRepo.repositorySync.(*repositorySyncImpl)
	assert.Equal(t, time.Duration(4*time.Second), repoSyncImpl.syncFrequency)

	repoSyncImpl.syncCountdown = time.Duration(4 * time.Second)

	repoSyncImpl.tickTock(ctx)
	assert.Equal(t, time.Duration(3*time.Second), repoSyncImpl.syncCountdown)

	repoSyncImpl.tickTock(ctx)
	assert.Equal(t, time.Duration(2*time.Second), repoSyncImpl.syncCountdown)

	repoSyncImpl.tickTock(ctx)
	assert.Equal(t, time.Duration(1*time.Second), repoSyncImpl.syncCountdown)

	repoSyncImpl.tickTock(ctx)
	assert.Equal(t, time.Duration(4*time.Second), repoSyncImpl.syncCountdown)

	fakeRepo := testRepo.externalRepo.(*fake.Repository)
	fakeExtPR := fake.FakePackageRevision{
		PrKey:     repository.PackageRevisionKey{},
		Resources: &v1alpha1.PackageRevisionResources{},
	}
	fakeRepo.PackageRevisions = append(fakeRepo.PackageRevisions, &fakeExtPR)
	fakeRepo.CurrentVersion = "A Different Version"
	fakeRepo.ThrowError = true

	repoSyncImpl.syncFrequency = time.Duration(60 * time.Second)
	repoSyncImpl.syncCountdown = time.Duration(1 * time.Second)
	repoSyncImpl.tickTock(ctx)
	assert.Equal(t, time.Duration(10*time.Second), repoSyncImpl.syncCountdown)
}
