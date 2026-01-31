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
	"time"

	kptfilev1 "github.com/kptdev/kpt/pkg/api/kptfile/v1"
	porchapi "github.com/nephio-project/porch/api/porch/v1alpha1"
	configapi "github.com/nephio-project/porch/api/porchconfig/v1alpha1"
	"github.com/nephio-project/porch/pkg/cache/testutil"
	cachetypes "github.com/nephio-project/porch/pkg/cache/types"
	"github.com/nephio-project/porch/pkg/externalrepo"
	"github.com/nephio-project/porch/pkg/externalrepo/fake"
	externalrepotypes "github.com/nephio-project/porch/pkg/externalrepo/types"
	"github.com/nephio-project/porch/pkg/repository"
	mockcachetypes "github.com/nephio-project/porch/test/mockery/mocks/porch/pkg/cache/types"
	"github.com/stretchr/testify/mock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
)

func (t *DbTestSuite) TestDBRepoSync() {
	mockCache := mockcachetypes.NewMockCache(t.T())
	cachetypes.CacheInstance = mockCache
	repoName := "my-repo-name"
	namespace := "my-ns"
	externalrepo.ExternalRepoInUnitTestMode = true

	ctx := t.Context()
	scheme := runtime.NewScheme()
	_ = configapi.AddToScheme(scheme)

	repoObj := &configapi.Repository{
		ObjectMeta: metav1.ObjectMeta{
			Name:      repoName,
			Namespace: namespace,
		},
	}
	repoObj.SetGroupVersionKind(configapi.GroupVersion.WithKind("Repository"))

	fakeClient := testutil.NewFakeClientWithStatus(scheme, repoObj)

	testRepo := t.createTestRepo(namespace, repoName)
	mockCache.EXPECT().GetRepository(mock.Anything).Return(testRepo).Maybe()

	err := testRepo.OpenRepository(ctx, externalrepotypes.ExternalRepoOptions{})
	t.Require().NoError(err)

	cacheOptions := cachetypes.CacheOptions{
		RepoSyncFrequency: 1 * time.Second,
		CoreClient:        fakeClient,
	}

	testRepo.repositorySync = newRepositorySync(testRepo, cacheOptions)
	newPRDef := porchapi.PackageRevision{
		Spec: porchapi.PackageRevisionSpec{
			RepositoryName: repoName,
			PackageName:    "my-package",
			WorkspaceName:  "my-workspace",
			Lifecycle:      porchapi.PackageRevisionLifecyclePublished,
		},
	}
	dbPRDraft, err := testRepo.CreatePackageRevisionDraft(ctx, &newPRDef)
	t.Require().NoError(err)
	t.Require().NotNil(dbPRDraft)

	dbPR, err := testRepo.ClosePackageRevisionDraft(ctx, dbPRDraft, 0)
	t.Require().NoError(err)

	err = dbPR.UpdateLifecycle(ctx, porchapi.PackageRevisionLifecycleProposed)
	t.Require().NoError(err)

	dbPR, err = testRepo.ClosePackageRevisionDraft(ctx, dbPR.(repository.PackageRevisionDraft), 0)
	t.Require().NoError(err)
	t.Require().NotNil(dbPR)

	err = dbPR.UpdateLifecycle(ctx, porchapi.PackageRevisionLifecyclePublished)
	t.Require().NoError(err)

	dbPR, err = testRepo.ClosePackageRevisionDraft(ctx, dbPR.(repository.PackageRevisionDraft), 0)
	t.Require().NoError(err)
	t.Require().NotNil(dbPR)

	time.Sleep(2 * time.Second)

	prList, err := testRepo.ListPackageRevisions(ctx, repository.ListPackageRevisionFilter{})
	t.Require().NoError(err)
	t.Equal(0, len(prList)) // Sync should have deleted the cached PR that is not in the external repo

	// Add the PR to the external repo
	fakeRepo := testRepo.externalRepo.(*fake.Repository)
	fakeExtPR := fake.FakePackageRevision{
		PrKey:           dbPR.Key(),
		PackageRevision: &newPRDef,
		Resources:       &porchapi.PackageRevisionResources{},
		Kptfile: kptfilev1.KptFile{
			Upstream:     &kptfilev1.Upstream{},
			UpstreamLock: &kptfilev1.UpstreamLock{},
		},
	}
	fakeRepo.PackageRevisions = append(fakeRepo.PackageRevisions, &fakeExtPR)
	time.Sleep(2 * time.Second)

	prList, err = testRepo.ListPackageRevisions(ctx, repository.ListPackageRevisionFilter{})
	t.Require().NoError(err)
	t.Equal(0, len(prList)) // The version of the external repo has not changed

	fakeRepo.CurrentVersion = "bar"
	time.Sleep(2 * time.Second)

	prList, err = testRepo.ListPackageRevisions(ctx, repository.ListPackageRevisionFilter{})
	t.Require().NoError(err)
	t.Equal(1, len(prList)) // Sync should have added a cached PR that is in the external repo

	testRepo.repositorySync.Stop()

	err = testRepo.Close(ctx)
	t.Require().NoError(err)
}

func (t *DbTestSuite) TestDBSyncRunOnceAt() {
	mockCache := mockcachetypes.NewMockCache(t.T())
	cachetypes.CacheInstance = mockCache
	repoName := "runonce-repo"
	namespace := "runonce-ns"

	externalrepo.ExternalRepoInUnitTestMode = true

	ctx := t.Context()
	scheme := runtime.NewScheme()
	_ = configapi.AddToScheme(scheme)

	runOnceTime := metav1.NewTime(time.Now().Add(7 * time.Second))

	repoObj := &configapi.Repository{
		ObjectMeta: metav1.ObjectMeta{
			Name:      repoName,
			Namespace: namespace,
		},
		Spec: configapi.RepositorySpec{
			Sync: &configapi.RepositorySync{
				RunOnceAt: &runOnceTime,
			},
		},
	}
	repoObj.SetGroupVersionKind(configapi.GroupVersion.WithKind("Repository"))

	fakeClient := testutil.NewFakeClientWithStatus(scheme, repoObj)
	testRepo := t.createTestRepo(namespace, repoName)
	testRepo.spec.Spec.Sync = &configapi.RepositorySync{
		RunOnceAt: &runOnceTime,
	}
	mockCache.EXPECT().GetRepository(mock.Anything).Return(testRepo).Maybe()

	err := testRepo.OpenRepository(ctx, externalrepotypes.ExternalRepoOptions{})
	t.Require().NoError(err)

	cacheOptions := cachetypes.CacheOptions{
		RepoSyncFrequency: 30 * time.Second,
		CoreClient:        fakeClient,
	}

	sync := newRepositorySync(testRepo, cacheOptions)
	testRepo.repositorySync = sync

	newPRDef := porchapi.PackageRevision{
		Spec: porchapi.PackageRevisionSpec{
			RepositoryName: repoName,
			PackageName:    "my-package",
			WorkspaceName:  "my-workspace",
			Lifecycle:      porchapi.PackageRevisionLifecyclePublished,
		},
	}
	dbPRDraft, err := testRepo.CreatePackageRevisionDraft(ctx, &newPRDef)
	t.Require().NoError(err)
	t.Require().NotNil(dbPRDraft)

	dbPR, err := testRepo.ClosePackageRevisionDraft(ctx, dbPRDraft, 0)
	t.Require().NoError(err)

	err = dbPR.UpdateLifecycle(ctx, porchapi.PackageRevisionLifecycleProposed)
	t.Require().NoError(err)

	dbPR, err = testRepo.ClosePackageRevisionDraft(ctx, dbPR.(repository.PackageRevisionDraft), 0)
	t.Require().NoError(err)
	t.Require().NotNil(dbPR)

	err = dbPR.UpdateLifecycle(ctx, porchapi.PackageRevisionLifecyclePublished)
	t.Require().NoError(err)

	dbPR, err = testRepo.ClosePackageRevisionDraft(ctx, dbPR.(repository.PackageRevisionDraft), 0)
	t.Require().NoError(err)
	t.Require().NotNil(dbPR)

	time.Sleep(2 * time.Second)

	// Add the PR to the external repo
	fakeRepo := testRepo.externalRepo.(*fake.Repository)
	fakeExtPR := fake.FakePackageRevision{
		PrKey:           dbPR.Key(),
		PackageRevision: &newPRDef,
		Resources:       &porchapi.PackageRevisionResources{},
		Kptfile: kptfilev1.KptFile{
			Upstream:     &kptfilev1.Upstream{},
			UpstreamLock: &kptfilev1.UpstreamLock{},
		},
	}
	fakeRepo.PackageRevisions = append(fakeRepo.PackageRevisions, &fakeExtPR)
	time.Sleep(2 * time.Second)
	testRepo.externalRepo.(*fake.Repository).CurrentVersion = "bar"

	// Wait until externalRepo.Version(ctx) returns "bar"
	timeout := time.After(5 * time.Second)
	tick := time.Tick(100 * time.Millisecond)

	versionReady := false
	for !versionReady {
		select {
		case <-timeout:
			t.T().Fatal("Timed out waiting for externalRepo version to be 'bar'")
		case <-tick:
			version, _ := testRepo.externalRepo.Version(ctx)
			if version == "bar" {
				t.T().Log("externalRepo version is 'bar'")
				versionReady = true
			}
		}
	}

	t.T().Log("Starting 5 second sleep...")
	time.Sleep(5 * time.Second)
	t.T().Log("Finished 5 second sleep")

	prList, err := testRepo.ListPackageRevisions(ctx, repository.ListPackageRevisionFilter{})
	t.Require().NoError(err)
	t.Equal(1, len(prList)) // Sync should have deleted the cached PR that is not in the external repo and
	// it should have added a cached PR that is in the external repo

	// Check that sync stats were updated
	t.Require().NotNil(sync.lastSyncStats)
	t.Require().Nil(sync.getLastSyncError())

	// Check statusStore for condition update
	status, ok := fakeClient.GetStatusStore()[types.NamespacedName{Name: repoName, Namespace: namespace}]
	t.Require().True(ok)
	t.Require().Equal(configapi.RepositoryReady, status.Conditions[0].Type)
	t.Require().Equal(metav1.ConditionTrue, status.Conditions[0].Status)
	t.Require().Equal(configapi.ReasonReady, status.Conditions[0].Reason)

	sync.Stop()
	err = testRepo.Close(ctx)
	t.Require().NoError(err)
}
func (t *DbTestSuite) TestRepositorySync_SyncOnce() {
	ctx := t.Context()
	externalrepo.ExternalRepoInUnitTestMode = true
	testRepo := t.createTestRepo("test-ns", "sync-once-repo")
	defer t.deleteTestRepo(testRepo.Key())

	err := testRepo.OpenRepository(ctx, externalrepotypes.ExternalRepoOptions{})
	t.Require().NoError(err)
	defer func() {
		if err := testRepo.Close(ctx); err != nil {
			t.T().Logf("Failed to close test repo: %v", err)
		}
	}()

	sync := &repositorySync{
		repo: testRepo,
	}

	err = sync.SyncOnce(ctx)
	t.Require().NoError(err)
}

func (t *DbTestSuite) TestRepositorySync_Key() {
	testRepo := t.createTestRepo("test-ns", "key-test-repo")
	defer t.deleteTestRepo(testRepo.Key())

	sync := &repositorySync{
		repo: testRepo,
	}

	key := sync.Key()
	t.Equal(testRepo.Key(), key)
}

func (t *DbTestSuite) TestRepositorySync_GetSpec() {
	testRepo := t.createTestRepo("test-ns", "spec-test-repo")
	defer t.deleteTestRepo(testRepo.Key())

	sync := &repositorySync{
		repo: testRepo,
	}

	spec := sync.GetSpec()
	t.Equal(testRepo.spec, spec)
}

func (t *DbTestSuite) TestRepositorySync_getLastSyncError() {
	testRepo := t.createTestRepo("test-ns", "error-test-repo")
	defer t.deleteTestRepo(testRepo.Key())

	// Test with nil syncManager
	sync := &repositorySync{
		repo: testRepo,
	}

	err := sync.getLastSyncError()
	t.Nil(err)
}
func (t *DbTestSuite) TestNewRepositorySync() {
	ctx := t.Context()
	externalrepo.ExternalRepoInUnitTestMode = true
	testRepo := t.createTestRepo("test-ns", "new-sync-repo")
	defer t.deleteTestRepo(testRepo.Key())

	err := testRepo.OpenRepository(ctx, externalrepotypes.ExternalRepoOptions{})
	t.Require().NoError(err)
	defer func() {
		if err := testRepo.Close(ctx); err != nil {
			t.T().Logf("Failed to close test repo: %v", err)
		}
	}()

	scheme := runtime.NewScheme()
	_ = configapi.AddToScheme(scheme)
	fakeClient := testutil.NewFakeClientWithStatus(scheme)

	options := cachetypes.CacheOptions{
		RepoSyncFrequency: 1 * time.Second,
		CoreClient:        fakeClient,
	}

	sync := newRepositorySync(testRepo, options)
	defer func() {
		if sync != nil {
			sync.Stop()
		}
	}()

	t.NotNil(sync)
	t.Equal(testRepo, sync.repo)
	t.NotNil(sync.syncManager)
}

func (t *DbTestSuite) TestRepositorySync_Stop() {
	testRepo := t.createTestRepo("test-ns", "stop-test-repo")
	defer t.deleteTestRepo(testRepo.Key())

	// Test Stop with nil syncManager
	sync := &repositorySync{
		repo: testRepo,
	}
	sync.Stop() // Should not panic

	// Test Stop with nil repositorySync
	var nilSync *repositorySync
	nilSync.Stop() // Should not panic
}

// TestCacheExternalPRs_SkipsBinaryFiles 驗證 sync 會跳過 binary files
// 不讓 invalid UTF-8 內容進到 DB 導致 PostgreSQL 報錯
func (t *DbTestSuite) TestCacheExternalPRs_SkipsBinaryFiles() {
	ctx := t.Context()
	externalrepo.ExternalRepoInUnitTestMode = true

	testRepo := t.createTestRepo("binary-ns", "binary-repo")
	defer t.deleteTestRepo(testRepo.Key())

	err := testRepo.OpenRepository(ctx, externalrepotypes.ExternalRepoOptions{})
	t.Require().NoError(err)

	repoSync := &repositorySync{
		repo: testRepo,
	}

	// 準備測試資料：混合 text 和 binary files
	prKey := repository.PackageRevisionKey{
		PkgKey: repository.PackageKey{
			RepoKey: testRepo.Key(),
			Package: "test-pkg",
		},
		Revision:      1,
		WorkspaceName: "ws",
	}

	prDef := &porchapi.PackageRevision{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-pr",
			Namespace:         "binary-ns",
			CreationTimestamp: metav1.Now(),
		},
		Spec: porchapi.PackageRevisionSpec{
			RepositoryName: "binary-repo",
			PackageName:    "test-pkg",
			WorkspaceName:  "ws",
			Lifecycle:      porchapi.PackageRevisionLifecyclePublished,
		},
	}

	// 模擬 external repo 回傳的 resources
	// 其中 image.png 含有 invalid UTF-8 bytes
	resources := &porchapi.PackageRevisionResources{
		Spec: porchapi.PackageRevisionResourcesSpec{
			Resources: map[string]string{
				"Kptfile":     "apiVersion: kpt.dev/v1\nkind: Kptfile\n",
				"config.yaml": "key: value\n",
				"image.png":   "\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR", // PNG header，不是 valid UTF-8
			},
		},
	}

	fakeExtPR := &fake.FakePackageRevision{
		PrKey:            prKey,
		PackageRevision:  prDef,
		PackageLifecycle: porchapi.PackageRevisionLifecyclePublished,
		Resources:        resources,
		Kptfile: kptfilev1.KptFile{
			Upstream:     &kptfilev1.Upstream{},
			UpstreamLock: &kptfilev1.UpstreamLock{},
		},
	}

	extPRMap := map[repository.PackageRevisionKey]repository.PackageRevision{
		prKey: fakeExtPR,
	}
	inExternalOnly := []repository.PackageRevisionKey{prKey}

	// 執行 cacheExternalPRs - 應該成功，不會因為 binary file 而失敗
	err = repoSync.cacheExternalPRs(ctx, extPRMap, inExternalOnly)
	t.Require().NoError(err, "sync 不該因為 binary file 而失敗")

	// 驗證 resources 直接從 DB 讀取
	cachedResources, err := pkgRevResourcesReadFromDB(ctx, prKey)
	t.Require().NoError(err)

	// text files 應該存在
	_, hasKptfile := cachedResources["Kptfile"]
	_, hasConfig := cachedResources["config.yaml"]
	t.True(hasKptfile, "Kptfile 應該被 cached")
	t.True(hasConfig, "config.yaml 應該被 cached")

	// binary file 應該被 skip
	_, hasBinary := cachedResources["image.png"]
	t.False(hasBinary, "image.png (binary) 應該被 skip")
}

// TestCacheExternalPRs_AllTextFiles 驗證純文字檔案全部被 cache
func (t *DbTestSuite) TestCacheExternalPRs_AllTextFiles() {
	ctx := t.Context()
	externalrepo.ExternalRepoInUnitTestMode = true

	testRepo := t.createTestRepo("text-ns", "text-repo")
	defer t.deleteTestRepo(testRepo.Key())

	err := testRepo.OpenRepository(ctx, externalrepotypes.ExternalRepoOptions{})
	t.Require().NoError(err)

	repoSync := &repositorySync{
		repo: testRepo,
	}

	prKey := repository.PackageRevisionKey{
		PkgKey: repository.PackageKey{
			RepoKey: testRepo.Key(),
			Package: "text-pkg",
		},
		Revision:      1,
		WorkspaceName: "ws",
	}

	prDef := &porchapi.PackageRevision{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "text-pr",
			Namespace:         "text-ns",
			CreationTimestamp: metav1.Now(),
		},
		Spec: porchapi.PackageRevisionSpec{
			RepositoryName: "text-repo",
			PackageName:    "text-pkg",
			WorkspaceName:  "ws",
			Lifecycle:      porchapi.PackageRevisionLifecyclePublished,
		},
	}

	// 全部都是 valid UTF-8 文字檔
	resources := &porchapi.PackageRevisionResources{
		Spec: porchapi.PackageRevisionResourcesSpec{
			Resources: map[string]string{
				"Kptfile":         "apiVersion: kpt.dev/v1\nkind: Kptfile\n",
				"deployment.yaml": "apiVersion: apps/v1\nkind: Deployment\n",
				"README.md":       "# Hello World\n",
			},
		},
	}

	fakeExtPR := &fake.FakePackageRevision{
		PrKey:            prKey,
		PackageRevision:  prDef,
		PackageLifecycle: porchapi.PackageRevisionLifecyclePublished,
		Resources:        resources,
		Kptfile: kptfilev1.KptFile{
			Upstream:     &kptfilev1.Upstream{},
			UpstreamLock: &kptfilev1.UpstreamLock{},
		},
	}

	extPRMap := map[repository.PackageRevisionKey]repository.PackageRevision{
		prKey: fakeExtPR,
	}
	inExternalOnly := []repository.PackageRevisionKey{prKey}

	err = repoSync.cacheExternalPRs(ctx, extPRMap, inExternalOnly)
	t.Require().NoError(err)

	cachedResources, err := pkgRevResourcesReadFromDB(ctx, prKey)
	t.Require().NoError(err)

	// 全部 3 個檔案都該存在
	t.Equal(3, len(cachedResources), "所有 text files 都應該被 cached")
	_, hasKptfile := cachedResources["Kptfile"]
	_, hasDeployment := cachedResources["deployment.yaml"]
	_, hasReadme := cachedResources["README.md"]
	t.True(hasKptfile, "Kptfile 應該被 cached")
	t.True(hasDeployment, "deployment.yaml 應該被 cached")
	t.True(hasReadme, "README.md 應該被 cached")
}

// TestCacheExternalPRs_AllBinaryFiles 驗證全部是 binary 時不報錯但全部 skip
func (t *DbTestSuite) TestCacheExternalPRs_AllBinaryFiles() {
	ctx := t.Context()
	externalrepo.ExternalRepoInUnitTestMode = true

	testRepo := t.createTestRepo("allbin-ns", "allbin-repo")
	defer t.deleteTestRepo(testRepo.Key())

	err := testRepo.OpenRepository(ctx, externalrepotypes.ExternalRepoOptions{})
	t.Require().NoError(err)

	repoSync := &repositorySync{
		repo: testRepo,
	}

	prKey := repository.PackageRevisionKey{
		PkgKey: repository.PackageKey{
			RepoKey: testRepo.Key(),
			Package: "allbin-pkg",
		},
		Revision:      1,
		WorkspaceName: "ws",
	}

	prDef := &porchapi.PackageRevision{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "allbin-pr",
			Namespace:         "allbin-ns",
			CreationTimestamp: metav1.Now(),
		},
		Spec: porchapi.PackageRevisionSpec{
			RepositoryName: "allbin-repo",
			PackageName:    "allbin-pkg",
			WorkspaceName:  "ws",
			Lifecycle:      porchapi.PackageRevisionLifecyclePublished,
		},
	}

	// 全部都是 binary files (invalid UTF-8)
	resources := &porchapi.PackageRevisionResources{
		Spec: porchapi.PackageRevisionResourcesSpec{
			Resources: map[string]string{
				"image.png": "\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR",
				"data.bin":  "\x00\x01\x02\x03\xff\xfe\xfd",
			},
		},
	}

	fakeExtPR := &fake.FakePackageRevision{
		PrKey:            prKey,
		PackageRevision:  prDef,
		PackageLifecycle: porchapi.PackageRevisionLifecyclePublished,
		Resources:        resources,
		Kptfile: kptfilev1.KptFile{
			Upstream:     &kptfilev1.Upstream{},
			UpstreamLock: &kptfilev1.UpstreamLock{},
		},
	}

	extPRMap := map[repository.PackageRevisionKey]repository.PackageRevision{
		prKey: fakeExtPR,
	}
	inExternalOnly := []repository.PackageRevisionKey{prKey}

	// 不該報錯
	err = repoSync.cacheExternalPRs(ctx, extPRMap, inExternalOnly)
	t.Require().NoError(err, "全部 binary 不該導致 error")

	cachedResources, err := pkgRevResourcesReadFromDB(ctx, prKey)
	t.Require().NoError(err)

	// 全部都該被 skip
	t.Equal(0, len(cachedResources), "所有 binary files 都應該被 skip")
}

// TestCacheExternalPRs_EmptyResources 驗證空 resources 不報錯
func (t *DbTestSuite) TestCacheExternalPRs_EmptyResources() {
	ctx := t.Context()
	externalrepo.ExternalRepoInUnitTestMode = true

	testRepo := t.createTestRepo("empty-ns", "empty-repo")
	defer t.deleteTestRepo(testRepo.Key())

	err := testRepo.OpenRepository(ctx, externalrepotypes.ExternalRepoOptions{})
	t.Require().NoError(err)

	repoSync := &repositorySync{
		repo: testRepo,
	}

	prKey := repository.PackageRevisionKey{
		PkgKey: repository.PackageKey{
			RepoKey: testRepo.Key(),
			Package: "empty-pkg",
		},
		Revision:      1,
		WorkspaceName: "ws",
	}

	prDef := &porchapi.PackageRevision{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "empty-pr",
			Namespace:         "empty-ns",
			CreationTimestamp: metav1.Now(),
		},
		Spec: porchapi.PackageRevisionSpec{
			RepositoryName: "empty-repo",
			PackageName:    "empty-pkg",
			WorkspaceName:  "ws",
			Lifecycle:      porchapi.PackageRevisionLifecyclePublished,
		},
	}

	// 空的 resources map
	resources := &porchapi.PackageRevisionResources{
		Spec: porchapi.PackageRevisionResourcesSpec{
			Resources: map[string]string{},
		},
	}

	fakeExtPR := &fake.FakePackageRevision{
		PrKey:            prKey,
		PackageRevision:  prDef,
		PackageLifecycle: porchapi.PackageRevisionLifecyclePublished,
		Resources:        resources,
		Kptfile: kptfilev1.KptFile{
			Upstream:     &kptfilev1.Upstream{},
			UpstreamLock: &kptfilev1.UpstreamLock{},
		},
	}

	extPRMap := map[repository.PackageRevisionKey]repository.PackageRevision{
		prKey: fakeExtPR,
	}
	inExternalOnly := []repository.PackageRevisionKey{prKey}

	err = repoSync.cacheExternalPRs(ctx, extPRMap, inExternalOnly)
	t.Require().NoError(err, "空 resources 不該報錯")

	cachedResources, err := pkgRevResourcesReadFromDB(ctx, prKey)
	t.Require().NoError(err)
	t.Equal(0, len(cachedResources), "空 resources 應該回傳空 map")
}
