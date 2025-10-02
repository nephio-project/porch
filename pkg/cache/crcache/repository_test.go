// Copyright 2024-2025 The kpt and Nephio Authors
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

package crcache

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	porchtypes "github.com/nephio-project/porch/api/porch/v1alpha1"
	configapi "github.com/nephio-project/porch/api/porchconfig/v1alpha1"
	cachetypes "github.com/nephio-project/porch/pkg/cache/types"
	"github.com/nephio-project/porch/pkg/externalrepo/fake"
	"github.com/nephio-project/porch/pkg/repository"
	mockmeta "github.com/nephio-project/porch/test/mockery/mocks/porch/pkg/cache/crcache/meta"
	mockcachetypes "github.com/nephio-project/porch/test/mockery/mocks/porch/pkg/cache/types"
	mockrepo "github.com/nephio-project/porch/test/mockery/mocks/porch/pkg/repository"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"sigs.k8s.io/controller-runtime/pkg/client"
	k8sfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// FakeClientWithStatusUpdate is a fake client that supports status updates
type FakeClientWithStatusUpdate struct {
	client.Client
	statusStore map[types.NamespacedName]configapi.RepositoryStatus
}

func NewFakeClientWithStatus(scheme *runtime.Scheme, objs ...client.Object) *FakeClientWithStatusUpdate {
	baseClient := k8sfake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
	return &FakeClientWithStatusUpdate{
		Client:      baseClient,
		statusStore: make(map[types.NamespacedName]configapi.RepositoryStatus),
	}
}
func (f *FakeClientWithStatusUpdate) Status() client.StatusWriter {
	return &fakeStatusWriter{f}
}

type fakeStatusWriter struct {
	f *FakeClientWithStatusUpdate
}

func (w *fakeStatusWriter) Update(ctx context.Context, obj client.Object, opts ...client.SubResourceUpdateOption) error {
	repo, ok := obj.(*configapi.Repository)
	if !ok {
		return fmt.Errorf("status update only supported for Repository objects")
	}
	key := types.NamespacedName{Name: repo.Name, Namespace: repo.Namespace}
	w.f.statusStore[key] = repo.Status
	return nil
}

func (w *fakeStatusWriter) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) error {
	return nil
}

func (w *fakeStatusWriter) Create(ctx context.Context, obj client.Object, subresource client.Object, opts ...client.SubResourceCreateOption) error {
	return nil
}

func (f *FakeClientWithStatusUpdate) Watch(ctx context.Context, list client.ObjectList, opts ...client.ListOption) (watch.Interface, error) {
	return watch.NewEmptyWatch(), nil
}

func TestCachedRepoRefresh(t *testing.T) {
	mockRepo := mockrepo.NewMockRepository(t)
	mockMeta := mockmeta.NewMockMetadataStore(t)
	mockNotifier := mockcachetypes.NewMockRepoPRChangeNotifier(t)
	repoName := "the-name"
	namespace := "the-ns"
	repoSpec := configapi.Repository{
		ObjectMeta: metav1.ObjectMeta{
			Name:      repoName,
			Namespace: namespace,
		},
	}
	scheme := runtime.NewScheme()
	_ = configapi.AddToScheme(scheme)
	fakeClient := NewFakeClientWithStatus(scheme, &repoSpec)

	options := cachetypes.CacheOptions{
		RepoPRChangeNotifier: mockNotifier,
		RepoCrSyncFrequency:  time.Minute,
		CoreClient:           fakeClient,
	}

	metaMap := []metav1.ObjectMeta{}

	mockRepo.On("Key").Return(repository.RepositoryKey{Namespace: namespace, Name: repoName}).Maybe()
	mockRepo.EXPECT().Refresh(mock.Anything).Return(nil).Maybe()
	repoVersionCall := mockRepo.EXPECT().Version(mock.Anything).Return("v1.0", nil).Maybe()
	repoListPRCall := mockRepo.EXPECT().ListPackageRevisions(mock.Anything, mock.Anything).Return(nil, nil).Maybe()
	repoClosePRDCall := mockRepo.EXPECT().ClosePackageRevisionDraft(mock.Anything, mock.Anything, 1).Return(nil, errors.New("create draft error")).Maybe()

	metaListCall := mockMeta.EXPECT().List(mock.Anything, mock.Anything).Return(metaMap, nil).Maybe()
	mockNotifier.EXPECT().NotifyPackageRevisionChange(mock.Anything, mock.Anything).Return(0).Maybe()

	repoKey := repository.RepositoryKey{
		Namespace: "my-namespace",
		Name:      "my-cached-repo",
	}
	cr := newRepository(repoKey, &repoSpec, mockRepo, mockMeta, options)
	assert.Equal(t, repoKey, cr.Key())

	prKey := repository.PackageRevisionKey{
		PkgKey: repository.PackageKey{
			RepoKey: repository.RepositoryKey{
				Namespace: namespace,
				Name:      repoName,
			},
			Path:    "",
			Package: "the-package",
		},
		WorkspaceName: "the-ws",
		Revision:      0,
	}

	fpr := fake.FakePackageRevision{
		PrKey: prKey,
	}

	cr.cachedPackageRevisions = make(map[repository.PackageRevisionKey]*cachedPackageRevision)
	cr.cachedPackageRevisions[prKey] = &cachedPackageRevision{
		PackageRevision:  &fpr,
		metadataStore:    mockMeta,
		isLatestRevision: false,
	}
	assert.False(t, cr.cachedPackageRevisions[prKey].IsLatestRevision())

	err := cr.Refresh(context.TODO())
	assert.True(t, err == nil)

	repoVersionCall.Return("", errors.New("version error")).Maybe()
	err = cr.Refresh(context.TODO())
	assert.False(t, err == nil)
	repoVersionCall.Return("v2.0", nil).Maybe()

	metaListCall.Return(nil, errors.New("meta list error")).Maybe()
	err = cr.Refresh(context.TODO())
	assert.False(t, err == nil)
	metaListCall.Return(metaMap, nil).Maybe()

	repoListPRCall.Return(nil, errors.New("list error")).Maybe()
	err = cr.Refresh(context.TODO())
	assert.False(t, err == nil)
	repoListPRCall.Return(nil, nil).Maybe()

	repoVersionCall.Return("v3.0", nil).Maybe()
	metaDeleteCall := mockMeta.EXPECT().Delete(mock.Anything, mock.Anything, mock.Anything).Return(metav1.ObjectMeta{}, errors.New("delete error")).Maybe()
	err = cr.Refresh(context.TODO())
	assert.True(t, err == nil)
	metaDeleteCall.Return(metav1.ObjectMeta{}, nil).Maybe()

	cr.flush()
	assert.True(t, cr.cachedPackageRevisions == nil)

	prMeta := porchtypes.PackageRevision{}

	repoCreatePRDCall := mockRepo.EXPECT().CreatePackageRevisionDraft(mock.Anything, mock.Anything).Return(nil, errors.New("create draft error")).Maybe()
	_, err = cr.CreatePackageRevisionDraft(context.TODO(), &prMeta)
	assert.True(t, err != nil)

	repoCreatePRDCall.Return(&fpr, nil).Maybe()
	prd, err := cr.CreatePackageRevisionDraft(context.TODO(), &prMeta)
	assert.True(t, err == nil)
	assert.Equal(t, 0, prd.Key().Revision)

	repoVersionCall.Return("", errors.New("version 2 error")).Maybe()
	_, err = cr.ClosePackageRevisionDraft(context.TODO(), prd, 1)
	assert.True(t, err != nil)

	repoVersionCall.Return("v3.2.4", nil).Maybe()
	_, err = cr.ClosePackageRevisionDraft(context.TODO(), prd, 1)
	assert.True(t, err != nil)

	_, err = cr.ClosePackageRevisionDraft(context.TODO(), prd, 1)
	assert.True(t, err != nil)
	repoClosePRDCall.Return(&fpr, nil).Maybe()

	repoClosePRDCall.Return(prd, nil).Maybe()
	metaCreateCall := mockMeta.EXPECT().Create(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(metav1.ObjectMeta{}, errors.New("meta create error")).Maybe()

	_, err = cr.ClosePackageRevisionDraft(context.TODO(), prd, 1)
	assert.True(t, err != nil)
	repoClosePRDCall.Return(&fpr, nil).Maybe()
	metaCreateCall.Return(metav1.ObjectMeta{}, nil)

	mockGet := mockMeta.EXPECT().Get(mock.Anything, mock.Anything).Return(metav1.ObjectMeta{}, nil).Maybe()
	mockUpdate := mockMeta.EXPECT().Update(mock.Anything, mock.Anything).Return(metav1.ObjectMeta{}, nil).Maybe()
	pr, err := cr.ClosePackageRevisionDraft(context.TODO(), prd, 1)
	assert.True(t, err == nil)
	assert.True(t, pr != nil)

	mockUpdate.Return(metav1.ObjectMeta{}, errors.New("meta update error")).Maybe()
	err = cr.cachedPackageRevisions[prKey].SetMeta(context.TODO(), metav1.ObjectMeta{Name: "Hello"})
	assert.True(t, err != nil)
	mockUpdate.Return(metav1.ObjectMeta{}, nil).Maybe()

	mockGet.Return(metav1.ObjectMeta{}, errors.New("meta get error")).Maybe()
	err = cr.cachedPackageRevisions[prKey].SetMeta(context.TODO(), metav1.ObjectMeta{Name: "Hello"})
	assert.True(t, err != nil)
	mockGet.Return(metav1.ObjectMeta{}, nil).Maybe()

	returnedMeta := metav1.ObjectMeta{
		Finalizers: []string{
			"finalizer",
		},
	}
	metaDeleteCall.Return(returnedMeta, errors.New("Delete on meta error")).Maybe()
	err = cr.DeletePackageRevision(context.TODO(), cr.cachedPackageRevisions[prKey])
	assert.True(t, err == nil)

}

const (
	failMsg   = "sync failed"
	repoName  = "the-name"
	namespace = "the-ns"
)

func TestHandleRunOnceAt(t *testing.T) {
	mockRepo := mockrepo.NewMockRepository(t)
	mockMeta := mockmeta.NewMockMetadataStore(t)
	mockNotifier := mockcachetypes.NewMockRepoPRChangeNotifier(t)

	runOnceTime := time.Now().Add(100 * time.Millisecond)

	repoSpec := configapi.Repository{
		ObjectMeta: metav1.ObjectMeta{
			Name:      repoName,
			Namespace: namespace,
		},
		Spec: configapi.RepositorySpec{
			Sync: &configapi.RepositorySync{
				RunOnceAt: &metav1.Time{Time: runOnceTime},
			},
		},
	}

	scheme := runtime.NewScheme()
	_ = configapi.AddToScheme(scheme)
	fakeClient := NewFakeClientWithStatus(scheme, &repoSpec)

	options := cachetypes.CacheOptions{
		RepoPRChangeNotifier: mockNotifier,
		RepoCrSyncFrequency:  time.Minute,
		CoreClient:           fakeClient,
	}

	mockRepo.On("Key").Return(repository.RepositoryKey{Namespace: namespace, Name: repoName}).Maybe()
	mockRepo.EXPECT().Refresh(mock.Anything).Return(nil).Maybe()
	mockRepo.EXPECT().Version(mock.Anything).Return("v1.0", nil).Maybe()
	mockRepo.EXPECT().ListPackageRevisions(mock.Anything, mock.Anything).Return(nil, nil).Maybe()

	metaMap := []metav1.ObjectMeta{}
	mockMeta.EXPECT().List(mock.Anything, mock.Anything).Return(metaMap, nil).Maybe()
	mockNotifier.EXPECT().NotifyPackageRevisionChange(mock.Anything, mock.Anything).Return(0).Maybe()

	repoKey := repository.RepositoryKey{
		Namespace: namespace,
		Name:      repoName,
	}
	cr := newRepository(repoKey, &repoSpec, mockRepo, mockMeta, options)

	time.Sleep(3 * time.Second)
	// Verify status was updated
	key := types.NamespacedName{Name: repoName, Namespace: namespace}
	status := fakeClient.statusStore[key]

	mockRepo.On("Close", mock.Anything).Return(nil).Maybe()
	cr.cancel()
	cr.Close(context.TODO())

	assert.NotNil(t, status, "Expected repository status to be updated")
	assert.Contains(t, []string{"Ready", "Error", "Reconciling"}, status.Conditions[0].Reason)
}

func TestSetRepositoryCondition(t *testing.T) {
	ctx := context.Background()

	mockRepo := mockrepo.NewMockRepository(t)
	mockRepo.On("Key").Return(repository.RepositoryKey{Namespace: namespace, Name: repoName}).Maybe()
	mockRepo.EXPECT().Refresh(mock.Anything).Return(nil).Maybe()
	mockRepo.EXPECT().Version(mock.Anything).Return("v1.0", nil).Maybe()
	mockRepo.EXPECT().ListPackageRevisions(mock.Anything, mock.Anything).Return(nil, nil).Maybe()
	mockRepo.On("Close", mock.Anything).Return(nil).Maybe()

	mockMeta := mockmeta.NewMockMetadataStore(t)
	metaMap := []metav1.ObjectMeta{}
	mockMeta.EXPECT().List(mock.Anything, mock.Anything).Return(metaMap, nil).Maybe()

	mockNotifier := mockcachetypes.NewMockRepoPRChangeNotifier(t)
	mockNotifier.EXPECT().NotifyPackageRevisionChange(mock.Anything, mock.Anything).Return(0).Maybe()

	repoSpec := configapi.Repository{
		ObjectMeta: metav1.ObjectMeta{
			Name:      repoName,
			Namespace: namespace,
		},
		Spec: configapi.RepositorySpec{
			Sync: &configapi.RepositorySync{
				RunOnceAt: &metav1.Time{Time: time.Now().Add(100 * time.Millisecond)},
			},
		},
		Status: configapi.RepositoryStatus{
			Conditions: []metav1.Condition{},
		},
	}

	scheme := runtime.NewScheme()
	_ = configapi.AddToScheme(scheme)
	fakeClient := NewFakeClientWithStatus(scheme, &repoSpec)

	options := cachetypes.CacheOptions{
		RepoPRChangeNotifier: mockNotifier,
		RepoCrSyncFrequency:  time.Minute,
		CoreClient:           fakeClient,
	}

	repoKey := repository.RepositoryKey{
		Name:      repoName,
		Namespace: namespace,
	}

	tests := []struct {
		name                  string
		status                string
		refreshRevisionsError error
		expectErr             bool
		expectedStatus        metav1.ConditionStatus
		expectedReason        string
		expectedMsg           string
	}{
		{
			name:           "ready",
			status:         "ready",
			expectedStatus: metav1.ConditionTrue,
			expectedReason: configapi.ReasonReady,
			expectedMsg:    "Repository Ready",
		},
		{
			name:                  "error with message",
			status:                "error",
			refreshRevisionsError: errors.New(failMsg),
			expectedStatus:        metav1.ConditionFalse,
			expectedReason:        configapi.ReasonError,
			expectedMsg:           failMsg,
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
			cr := newRepository(repoKey, &repoSpec, mockRepo, mockMeta, options)

			// Only set refreshRevisionsError if explicitly provided
			if tt.status == "error" {
				if tt.refreshRevisionsError != nil {
					cr.refreshRevisionsError = tt.refreshRevisionsError
				} else {
					cr.refreshRevisionsError = nil
				}
			}

			err := cr.setRepositoryCondition(ctx, tt.status)

			if tt.expectErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "unknown status type")
				return
			}

			assert.NoError(t, err)

			key := types.NamespacedName{Name: repoName, Namespace: namespace}
			updatedRepo := fakeClient.statusStore[key]
			assert.NotNil(t, updatedRepo)

			condition := updatedRepo.Conditions[0]
			assert.Equal(t, configapi.RepositoryReady, condition.Type)
			assert.Equal(t, tt.expectedStatus, condition.Status)
			assert.Equal(t, tt.expectedReason, condition.Reason)
			assert.Equal(t, tt.expectedMsg, condition.Message)

			err = cr.Close(ctx)
			assert.NoError(t, err)
		})
	}
}

func TestUpdateRepositoryCondition(t *testing.T) {
	ctx := context.Background()

	mockRepo := mockrepo.NewMockRepository(t)
	mockRepo.On("Key").Return(repository.RepositoryKey{Namespace: namespace, Name: repoName}).Maybe()
	mockRepo.EXPECT().Refresh(mock.Anything).Return(nil).Maybe()
	mockRepo.EXPECT().Version(mock.Anything).Return("v1.0", nil).Maybe()
	mockRepo.EXPECT().ListPackageRevisions(mock.Anything, mock.Anything).Return(nil, nil).Maybe()
	mockRepo.On("Close", mock.Anything).Return(nil).Maybe()

	mockMeta := mockmeta.NewMockMetadataStore(t)
	metaMap := []metav1.ObjectMeta{}
	mockMeta.EXPECT().List(mock.Anything, mock.Anything).Return(metaMap, nil).Maybe()

	mockNotifier := mockcachetypes.NewMockRepoPRChangeNotifier(t)
	mockNotifier.EXPECT().NotifyPackageRevisionChange(mock.Anything, mock.Anything).Return(0).Maybe()

	repoSpec := configapi.Repository{
		ObjectMeta: metav1.ObjectMeta{Name: repoName, Namespace: namespace},
		Spec:       configapi.RepositorySpec{Sync: &configapi.RepositorySync{}},
		Status:     configapi.RepositoryStatus{Conditions: []metav1.Condition{}},
	}

	scheme := runtime.NewScheme()
	_ = configapi.AddToScheme(scheme)
	fakeClient := NewFakeClientWithStatus(scheme, &repoSpec)

	options := cachetypes.CacheOptions{
		RepoPRChangeNotifier: mockNotifier,
		RepoCrSyncFrequency:  time.Minute,
		CoreClient:           fakeClient,
	}

	repoKey := repository.RepositoryKey{Name: repoName, Namespace: namespace}

	tests := []struct {
		name                  string
		refreshRevisionsError error
		expectedStatus        metav1.ConditionStatus
		expectedReason        string
		expectedMsg           string
	}{
		{
			name:           "ready",
			expectedStatus: metav1.ConditionTrue,
			expectedReason: configapi.ReasonReady,
			expectedMsg:    "Repository Ready",
		},
		{
			name:                  "error",
			refreshRevisionsError: errors.New(failMsg),
			expectedStatus:        metav1.ConditionFalse,
			expectedReason:        configapi.ReasonError,
			expectedMsg:           failMsg,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cr := newRepository(repoKey, &repoSpec, mockRepo, mockMeta, options)
			cr.refreshRevisionsError = tt.refreshRevisionsError

			cr.updateRepositoryCondition(ctx)

			key := types.NamespacedName{Name: repoName, Namespace: namespace}
			updatedRepo := fakeClient.statusStore[key]
			assert.NotNil(t, updatedRepo)

			condition := updatedRepo.Conditions[0]
			assert.Equal(t, configapi.RepositoryReady, condition.Type)
			assert.Equal(t, tt.expectedStatus, condition.Status)
			assert.Equal(t, tt.expectedReason, condition.Reason)
			assert.Equal(t, tt.expectedMsg, condition.Message)

			err := cr.Close(ctx)
			assert.NoError(t, err)
		})
	}
}

func TestCalculateWaitDuration(t *testing.T) {
	defaultDuration := 10 * time.Minute

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
			name:      "empty schedule",
			syncSpec:  &configapi.RepositorySync{Schedule: ""},
			expectMin: defaultDuration,
			expectMax: defaultDuration,
		},
		{
			name:      "invalid cron",
			syncSpec:  &configapi.RepositorySync{Schedule: "invalid"},
			expectMin: defaultDuration,
			expectMax: defaultDuration,
		},
		{
			name:      "valid cron",
			syncSpec:  &configapi.RepositorySync{Schedule: "* * * * *"},
			expectMin: 0,
			expectMax: time.Minute,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &cachedRepository{
				repo:     mockrepo.NewMockRepository(t),
				repoSpec: &configapi.Repository{Spec: configapi.RepositorySpec{Sync: tt.syncSpec}},
			}
			d := repo.calculateWaitDuration(defaultDuration)
			assert.GreaterOrEqual(t, d, tt.expectMin)
			assert.LessOrEqual(t, d, tt.expectMax)
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
			repo:     mockrepo.NewMockRepository(t),
			repoSpec: &configapi.Repository{},
			expected: false,
		},
		{
			name:     "valid sync spec",
			repo:     mockrepo.NewMockRepository(t),
			repoSpec: &configapi.Repository{Spec: configapi.RepositorySpec{Sync: &configapi.RepositorySync{}}},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &cachedRepository{
				repo:     tt.repo,
				repoSpec: tt.repoSpec,
			}
			assert.Equal(t, tt.expected, repo.hasValidSyncSpec())
		})
	}
}

func TestShouldScheduleRunOnce(t *testing.T) {
	now := time.Now()

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
			runOnceAt: &metav1.Time{Time: now},
			scheduled: time.Time{},
			expected:  true,
		},
		{
			name:      "runOnceAt equals scheduled",
			runOnceAt: &metav1.Time{Time: now},
			scheduled: now,
			expected:  false,
		},
		{
			name:      "runOnceAt not equal scheduled",
			runOnceAt: &metav1.Time{Time: now},
			scheduled: now.Add(time.Second),
			expected:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &cachedRepository{}
			result := repo.shouldScheduleRunOnce(tt.runOnceAt, tt.scheduled)
			assert.Equal(t, tt.expected, result)
		})
	}
}
