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
	"testing"
	"time"

	"github.com/nephio-project/porch/api/porch/v1alpha1"
	configapi "github.com/nephio-project/porch/api/porchconfig/v1alpha1"
	cachetypes "github.com/nephio-project/porch/pkg/cache/types"
	"github.com/nephio-project/porch/pkg/externalrepo"
	"github.com/nephio-project/porch/pkg/externalrepo/fake"
	externalrepotypes "github.com/nephio-project/porch/pkg/externalrepo/types"
	v1 "github.com/nephio-project/porch/pkg/kpt/api/kptfile/v1"
	"github.com/nephio-project/porch/pkg/repository"
	mockcachetypes "github.com/nephio-project/porch/test/mockery/mocks/porch/pkg/cache/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
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

	fakeClient := NewFakeClientWithStatus(scheme, repoObj)

	testRepo := t.createTestRepo(namespace, repoName)
	mockCache.EXPECT().GetRepository(mock.Anything).Return(testRepo).Maybe()

	err := testRepo.OpenRepository(ctx, externalrepotypes.ExternalRepoOptions{})
	t.Require().NoError(err)

	cacheOptions := cachetypes.CacheOptions{
		RepoCrSyncFrequency: 1 * time.Second,
		CoreClient:          fakeClient,
	}

	testRepo.repositorySync = newRepositorySync(testRepo, cacheOptions)
	newPRDef := v1alpha1.PackageRevision{
		Spec: v1alpha1.PackageRevisionSpec{
			RepositoryName: repoName,
			PackageName:    "my-package",
			WorkspaceName:  "my-workspace",
			Lifecycle:      v1alpha1.PackageRevisionLifecyclePublished,
		},
	}
	dbPRDraft, err := testRepo.CreatePackageRevisionDraft(ctx, &newPRDef)
	t.Require().NoError(err)
	t.Require().NotNil(dbPRDraft)

	dbPR, err := testRepo.ClosePackageRevisionDraft(ctx, dbPRDraft, 0)
	t.Require().NoError(err)

	err = dbPR.UpdateLifecycle(ctx, v1alpha1.PackageRevisionLifecycleProposed)
	t.Require().NoError(err)

	dbPR, err = testRepo.ClosePackageRevisionDraft(ctx, dbPR.(repository.PackageRevisionDraft), 0)
	t.Require().NoError(err)
	t.Require().NotNil(dbPR)

	err = dbPR.UpdateLifecycle(ctx, v1alpha1.PackageRevisionLifecyclePublished)
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
		Resources:       &v1alpha1.PackageRevisionResources{},
		Kptfile: v1.KptFile{
			Upstream:     &v1.Upstream{},
			UpstreamLock: &v1.UpstreamLock{},
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

	fakeClient := NewFakeClientWithStatus(scheme, repoObj)
	testRepo := t.createTestRepo(namespace, repoName)
	testRepo.spec.Spec.Sync = &configapi.RepositorySync{
		RunOnceAt: &runOnceTime,
	}
	mockCache.EXPECT().GetRepository(mock.Anything).Return(testRepo).Maybe()

	err := testRepo.OpenRepository(ctx, externalrepotypes.ExternalRepoOptions{})
	t.Require().NoError(err)

	cacheOptions := cachetypes.CacheOptions{
		RepoCrSyncFrequency: 30 * time.Second,
		CoreClient:          fakeClient,
	}

	sync := newRepositorySync(testRepo, cacheOptions)
	testRepo.repositorySync = sync

	newPRDef := v1alpha1.PackageRevision{
		Spec: v1alpha1.PackageRevisionSpec{
			RepositoryName: repoName,
			PackageName:    "my-package",
			WorkspaceName:  "my-workspace",
			Lifecycle:      v1alpha1.PackageRevisionLifecyclePublished,
		},
	}
	dbPRDraft, err := testRepo.CreatePackageRevisionDraft(ctx, &newPRDef)
	t.Require().NoError(err)
	t.Require().NotNil(dbPRDraft)

	dbPR, err := testRepo.ClosePackageRevisionDraft(ctx, dbPRDraft, 0)
	t.Require().NoError(err)

	err = dbPR.UpdateLifecycle(ctx, v1alpha1.PackageRevisionLifecycleProposed)
	t.Require().NoError(err)

	dbPR, err = testRepo.ClosePackageRevisionDraft(ctx, dbPR.(repository.PackageRevisionDraft), 0)
	t.Require().NoError(err)
	t.Require().NotNil(dbPR)

	err = dbPR.UpdateLifecycle(ctx, v1alpha1.PackageRevisionLifecyclePublished)
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
		Resources:       &v1alpha1.PackageRevisionResources{},
		Kptfile: v1.KptFile{
			Upstream:     &v1.Upstream{},
			UpstreamLock: &v1.UpstreamLock{},
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

	time.Sleep(5 * time.Second)

	prList, err := testRepo.ListPackageRevisions(ctx, repository.ListPackageRevisionFilter{})
	t.Require().NoError(err)
	t.Equal(1, len(prList)) // Sync should have deleted the cached PR that is not in the external repo and
	// it should have added a cached PR that is in the external repo

	// Check that sync stats were updated
	t.Require().NotNil(sync.lastSyncStats)
	t.Require().Nil(sync.lastSyncError)

	// Check statusStore for condition update
	status, ok := fakeClient.statusStore[types.NamespacedName{Name: repoName, Namespace: namespace}]
	t.Require().True(ok)
	t.Require().Equal(configapi.RepositoryReady, status.Conditions[0].Type)
	t.Require().Equal(metav1.ConditionTrue, status.Conditions[0].Status)
	t.Require().Equal(configapi.ReasonReady, status.Conditions[0].Reason)

	sync.Stop()
	err = testRepo.Close(ctx)
	t.Require().NoError(err)
}

const (
	failMsg = "sync failed"
)

func TestSetRepositoryCondition(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = configapi.AddToScheme(scheme)

	repoName := "test-repo"
	namespace := "test-ns"

	repoObj := &configapi.Repository{
		ObjectMeta: metav1.ObjectMeta{
			Name:      repoName,
			Namespace: namespace,
		},
	}
	repoObj.SetGroupVersionKind(configapi.GroupVersion.WithKind("Repository"))
	repoObj.ResourceVersion = ""

	client := NewFakeClientWithStatus(scheme, repoObj.DeepCopy())

	dbRepo := &dbRepository{}
	dbRepo.repoKey = repository.RepositoryKey{
		Namespace: namespace,
		Name:      repoName,
	}
	dbRepo.spec = &configapi.Repository{
		Spec: configapi.RepositorySpec{
			Git: &configapi.GitRepository{
				Repo: "http://example.repo.org/my-repo",
			},
		},
	}

	tests := []struct {
		name           string
		status         string
		lastSyncError  error
		expectErr      bool
		expectedStatus metav1.ConditionStatus
		expectedReason string
		expectedMsg    string
	}{
		{
			name:           "ready",
			status:         "ready",
			expectedStatus: metav1.ConditionTrue,
			expectedReason: configapi.ReasonReady,
			expectedMsg:    "Repository Ready",
		},
		{
			name:           "error with message",
			status:         "error",
			lastSyncError:  errors.New(failMsg),
			expectedStatus: metav1.ConditionFalse,
			expectedReason: configapi.ReasonError,
			expectedMsg:    failMsg,
		},
		{
			name:           "error without message",
			status:         "error",
			expectedStatus: metav1.ConditionFalse,
			expectedReason: configapi.ReasonError,
			expectedMsg:    "unknown error",
		},
		{
			name:           "sync-in-progress",
			status:         "sync-in-progress",
			expectedStatus: metav1.ConditionFalse,
			expectedReason: configapi.ReasonReconciling,
			expectedMsg:    "Repository reconciliation in progress",
		},
		{
			name:      "unknown",
			status:    "unknown",
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sync := &repositorySync{
				repo:          dbRepo,
				coreClient:    client,
				lastSyncError: tt.lastSyncError,
			}

			err := sync.setRepositoryCondition(context.TODO(), tt.status)
			if tt.expectErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			status, ok := client.statusStore[types.NamespacedName{Name: repoName, Namespace: namespace}]
			require.True(t, ok)
			require.Len(t, status.Conditions, 1)

			cond := status.Conditions[0]
			require.Equal(t, configapi.RepositoryReady, cond.Type)
			require.Equal(t, tt.expectedStatus, cond.Status)
			require.Equal(t, tt.expectedReason, cond.Reason)
			require.Equal(t, tt.expectedMsg, cond.Message)
		})
	}
}

func TestUpdateRepositoryCondition(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = configapi.AddToScheme(scheme)

	repoName := "test-repo"
	namespace := "test-ns"

	repoObj := &configapi.Repository{
		ObjectMeta: metav1.ObjectMeta{
			Name:      repoName,
			Namespace: namespace,
		},
	}
	repoObj.SetGroupVersionKind(configapi.GroupVersion.WithKind("Repository"))
	repoObj.ResourceVersion = ""

	client := NewFakeClientWithStatus(scheme, repoObj.DeepCopy())

	dbRepo := &dbRepository{}
	dbRepo.repoKey = repository.RepositoryKey{Name: repoName, Namespace: namespace}
	dbRepo.spec = &configapi.Repository{}

	tests := []struct {
		name           string
		lastSyncError  error
		expectedStatus metav1.ConditionStatus
		expectedReason string
	}{
		{
			name:           "ready condition",
			lastSyncError:  nil,
			expectedStatus: metav1.ConditionTrue,
			expectedReason: configapi.ReasonReady,
		},
		{
			name:           "error condition",
			lastSyncError:  errors.New(failMsg),
			expectedStatus: metav1.ConditionFalse,
			expectedReason: configapi.ReasonError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sync := &repositorySync{
				repo:          dbRepo,
				coreClient:    client,
				lastSyncError: tt.lastSyncError,
			}

			sync.updateRepositoryCondition(context.TODO())

			status, ok := client.statusStore[types.NamespacedName{Name: repoName, Namespace: namespace}]
			require.True(t, ok)
			require.Len(t, status.Conditions, 1)

			cond := status.Conditions[0]
			require.Equal(t, configapi.RepositoryReady, cond.Type)
			require.Equal(t, tt.expectedStatus, cond.Status)
			require.Equal(t, tt.expectedReason, cond.Reason)
		})
	}
}

func TestCalculateWaitDuration(t *testing.T) {
	defaultDuration := 30 * time.Second

	tests := []struct {
		name      string
		syncSpec  *configapi.RepositorySync
		expectMin time.Duration
		expectMax time.Duration
	}{
		{
			name:      "nil sync spec",
			syncSpec:  nil,
			expectMin: defaultDuration,
			expectMax: defaultDuration,
		},
		{
			name: "empty schedule",
			syncSpec: &configapi.RepositorySync{
				Schedule: "",
			},
			expectMin: defaultDuration,
			expectMax: defaultDuration,
		},
		{
			name: "invalid cron",
			syncSpec: &configapi.RepositorySync{
				Schedule: "invalid-cron",
			},
			expectMin: defaultDuration,
			expectMax: defaultDuration,
		},
		{
			name: "valid cron with @every",
			syncSpec: &configapi.RepositorySync{
				Schedule: "@every 1m",
			},
			expectMin: 0,
			expectMax: time.Minute,
		},
		{
			name: "valid cron",
			syncSpec: &configapi.RepositorySync{
				Schedule: "*/10 * * * *",
			},
			expectMin: 0,
			expectMax: 10 * time.Minute,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dbRepo := &dbRepository{}
			if tt.syncSpec != nil {
				dbRepo.spec = &configapi.Repository{
					Spec: configapi.RepositorySpec{
						Sync: tt.syncSpec,
					},
				}
			}

			sync := &repositorySync{
				repo: dbRepo,
			}

			d := sync.calculateWaitDuration(defaultDuration)
			require.GreaterOrEqual(t, d, tt.expectMin)
			require.LessOrEqual(t, d, tt.expectMax)
		})
	}
}

func TestHasValidSyncSpec(t *testing.T) {
	tests := []struct {
		name     string
		repo     repository.Repository
		repoSpec *configapi.Repository
		expected bool
	}{
		{
			name:     "nil repo and spec",
			repo:     nil,
			repoSpec: nil,
			expected: false,
		},
		{
			name:     "spec without sync",
			repo:     &dbRepository{},
			repoSpec: &configapi.Repository{},
			expected: false,
		},
		{
			name: "valid sync spec",
			repo: &dbRepository{
				spec: &configapi.Repository{
					Spec: configapi.RepositorySpec{
						Sync: &configapi.RepositorySync{},
					},
				},
			},
			repoSpec: &configapi.Repository{
				Spec: configapi.RepositorySpec{
					Sync: &configapi.RepositorySync{},
				},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var sync *repositorySync
			if tt.repo != nil {
				dbRepo, ok := tt.repo.(*dbRepository)
				if !ok {
					t.Fatalf("repo is not of type *dbRepository")
				}
				sync = &repositorySync{repo: dbRepo}
			} else {
				sync = &repositorySync{repo: nil}
			}
			assert.Equal(t, tt.expected, sync.hasValidSyncSpec())
		})
	}

}

func TestShouldScheduleRunOnce(t *testing.T) {
	now := time.Now()
	runOnce := metav1.NewTime(now.Add(1 * time.Second))

	tests := []struct {
		name      string
		runOnceAt *metav1.Time
		scheduled time.Time
		expected  bool
	}{
		{
			name:      "nil runOnceAt",
			runOnceAt: nil,
			scheduled: time.Time{},
			expected:  false,
		},
		{
			name:      "zero runOnceAt",
			runOnceAt: &metav1.Time{},
			scheduled: time.Time{},
			expected:  false,
		},
		{
			name:      "scheduled is zero",
			runOnceAt: &runOnce,
			scheduled: time.Time{},
			expected:  true,
		},
		{
			name:      "runOnceAt != scheduled",
			runOnceAt: &runOnce,
			scheduled: now,
			expected:  true,
		},
		{
			name:      "runOnceAt == scheduled",
			runOnceAt: &runOnce,
			scheduled: runOnce.Time,
			expected:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sync := &repositorySync{}
			result := sync.shouldScheduleRunOnce(tt.runOnceAt, tt.scheduled)
			require.Equal(t, tt.expected, result)
		})
	}
}
