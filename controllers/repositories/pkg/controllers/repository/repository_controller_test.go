// Copyright 2026 The kpt and Nephio Authors
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

package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	api "github.com/nephio-project/porch/controllers/repositories/api/v1alpha1"
	mockclient "github.com/nephio-project/porch/test/mockery/mocks/external/sigs.k8s.io/controller-runtime/pkg/client"
	cachetypes "github.com/nephio-project/porch/test/mockery/mocks/porch/pkg/cache/types"
	mockRepo "github.com/nephio-project/porch/test/mockery/mocks/porch/pkg/repository"
)

func TestReconcile(t *testing.T) {
	ctx := context.Background()
	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: "test-repo", Namespace: "test-ns"}}

	tests := []struct {
		name           string
		repo           *api.Repository
		getError       error
		cacheNil       bool
		expectError    bool
		expectRequeue  bool
		expectDeletion bool
		needsSync      bool
	}{
		{
			name:     "cache not available",
			repo:     createTestRepo("cache-nil-repo", "test-ns"),
			cacheNil: true,
			expectError: true,
		},
		{
			name:     "repository not found",
			getError: apierrors.NewNotFound(schema.GroupResource{}, "test-repo"),
		},
		{
			name:        "get repository error",
			getError:    errors.New("get failed"),
			expectError: true,
		},
		{
			name: "repository marked for deletion",
			repo: func() *api.Repository {
				repo := createTestRepo("test-repo", "test-ns")
				now := metav1.Now()
				repo.DeletionTimestamp = &now
				return repo
			}(),
			expectDeletion: true,
		},
		{
			name: "spec changed - triggers sync",
			repo: createTestRepo("test-repo", "test-ns"),
			needsSync: true,
			expectRequeue: true,
		},
		{
			name: "no sync needed - recently synced",
			repo: func() *api.Repository {
				repo := createTestRepo("test-repo", "test-ns")
				repo.Generation = 1
				// Use a fixed recent time to avoid timing issues
				recentTime := metav1.NewTime(time.Now().Add(-50 * time.Millisecond))
				repo.Status.Conditions = []metav1.Condition{{
					Type: api.RepositoryReady,
					Status: metav1.ConditionTrue,
					ObservedGeneration: 1,
					LastTransitionTime: recentTime,
				}}
				// Set annotation to indicate recent full sync (within 500ms window)
				repo.ObjectMeta.Annotations = map[string]string{
					"config.porch.kpt.dev/last-full-sync": recentTime.Format(time.RFC3339),
				}
				return repo
			}(),
			needsSync: false,
			expectRequeue: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := mockclient.NewMockClient(t)
			var mockCache *cachetypes.MockCache

			if !tt.cacheNil {
				mockCache = cachetypes.NewMockCache(t)
			}

			// Setup Get expectation - only if cache is available
			if !tt.cacheNil {
				if tt.getError != nil {
					mockClient.EXPECT().Get(ctx, req.NamespacedName, &api.Repository{}).Return(tt.getError)
				} else if tt.repo != nil {
					mockClient.EXPECT().Get(ctx, req.NamespacedName, &api.Repository{}).Run(func(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) {
						repo := obj.(*api.Repository)
						*repo = *tt.repo
					}).Return(nil)
				} else {
					mockClient.EXPECT().Get(ctx, req.NamespacedName, &api.Repository{}).Return(nil)
				}
			}

			reconciler := &RepositoryReconciler{
				Client:                    mockClient,
				HealthCheckFrequency:      100 * time.Millisecond,
				FullSyncFrequency:         500 * time.Millisecond,
				MaxConcurrentSyncs:        100,
			}
			
			// Set cache - explicitly nil for cacheNil test case
			if tt.cacheNil {
				reconciler.Cache = nil
			} else {
				reconciler.Cache = mockCache
				// Initialize sync limiter for non-nil cache
				reconciler.syncLimiter = make(chan struct{}, 100)
			}

			// Mock deletion handling if needed
			if tt.expectDeletion && mockCache != nil {
				mockClient.EXPECT().List(mock.Anything, &api.RepositoryList{}).Return(nil)
				mockCache.EXPECT().CloseRepository(mock.Anything, tt.repo, mock.Anything).Return(nil)
				mockClient.EXPECT().Update(mock.Anything, tt.repo).Return(nil)
			}

			// Mock finalizer addition for non-deletion cases - only if cache available
			if tt.repo != nil && !tt.expectDeletion && !tt.cacheNil {
				if !controllerutil.ContainsFinalizer(tt.repo, RepositoryFinalizer) {
					mockClient.EXPECT().Patch(mock.Anything, mock.Anything, mock.Anything).Return(nil)
				}
			}

			// Mock sync operations if needed
			if tt.needsSync && mockCache != nil {
				// Mock status update to sync-in-progress
				mockStatusWriter := mockclient.NewMockSubResourceWriter(t)
				mockClient.EXPECT().Status().Return(mockStatusWriter)
				mockStatusWriter.EXPECT().Patch(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
				
				// Mock async full sync operations (run in goroutine)
				mockRepository := mockRepo.NewMockRepository(t)
				mockCache.EXPECT().OpenRepository(mock.Anything, mock.Anything).Return(mockRepository, nil).Maybe()
				mockRepository.EXPECT().Refresh(mock.Anything).Return(nil).Maybe()
				mockRepository.EXPECT().ListPackageRevisions(mock.Anything, mock.Anything).Return(nil, nil).Maybe()
				
				// Mock annotation update for full sync timestamp
				mockClient.EXPECT().Patch(mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
				
				// Mock async status update
				mockClient.EXPECT().Status().Return(mockStatusWriter).Maybe()
				mockStatusWriter.EXPECT().Patch(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
				
				// Mock clearing one-time sync flag
				mockClient.EXPECT().Patch(mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
			} else if !tt.needsSync && mockCache != nil && tt.repo != nil && !tt.expectDeletion {
				// Health check path for no sync needed
				mockCache.EXPECT().CheckRepositoryConnectivity(mock.Anything, mock.Anything).Return(nil).Maybe()
				
				// Mock status update for health check
				mockStatusWriter := mockclient.NewMockSubResourceWriter(t)
				mockClient.EXPECT().Status().Return(mockStatusWriter).Maybe()
				mockStatusWriter.EXPECT().Patch(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
			}

			result, err := reconciler.Reconcile(ctx, req)

			// Give async goroutines time to complete for sync cases
			if tt.needsSync {
				time.Sleep(10 * time.Millisecond)
			}

			assertError(t, tt.expectError, err)
			assertRequeue(t, tt.expectRequeue, result)
		})
	}
}

func TestEnsureFinalizer(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name        string
		repo        *api.Repository
		updateError error
		expectAdded bool
		expectError bool
	}{
		{
			name:        "adds finalizer when missing",
			repo:        createTestRepo("test-repo", "test-ns"),
			expectAdded: true,
		},
		{
			name: "finalizer already present",
			repo: func() *api.Repository {
				repo := createTestRepo("test-repo", "test-ns")
				controllerutil.AddFinalizer(repo, RepositoryFinalizer)
				return repo
			}(),
			expectAdded: false,
		},
		{
			name:        "update fails",
			repo:        createTestRepo("test-repo", "test-ns"),
			updateError: errors.New("update failed"),
			expectAdded: false,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := mockclient.NewMockClient(t)
			if !controllerutil.ContainsFinalizer(tt.repo, RepositoryFinalizer) {
				mockClient.EXPECT().Patch(mock.Anything, mock.Anything, mock.Anything).Return(tt.updateError)
			}

			reconciler := &RepositoryReconciler{Client: mockClient}
			added, err := reconciler.ensureFinalizer(ctx, tt.repo)

			assertError(t, tt.expectError, err)
			if added != tt.expectAdded {
				t.Errorf("Expected added %v, got %v", tt.expectAdded, added)
			}
		})
	}
}

func TestSyncRepository(t *testing.T) {
	ctx := context.Background()
	repo := createTestRepo("test-repo", "test-ns")

	tests := []struct {
		name          string
		openError     error
		refreshError  error
		listError     error
		expectError   bool
	}{
		{
			name: "successful sync",
		},
		{
			name:        "open repository fails",
			openError:   errors.New("open failed"),
			expectError: true,
		},
		{
			name:         "refresh fails",
			refreshError: errors.New("refresh failed"),
			expectError:  true,
		},
		{
			name:        "list packages fails",
			listError:   errors.New("list failed"),
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCache := cachetypes.NewMockCache(t)
			mockRepository := mockRepo.NewMockRepository(t)

			if tt.openError != nil {
				mockCache.EXPECT().OpenRepository(ctx, repo).Return(nil, tt.openError)
			} else {
				mockCache.EXPECT().OpenRepository(ctx, repo).Return(mockRepository, nil)
				
				if tt.refreshError != nil {
					mockRepository.EXPECT().Refresh(ctx).Return(tt.refreshError)
				} else {
					mockRepository.EXPECT().Refresh(ctx).Return(nil)
					
					if tt.listError != nil {
						mockRepository.EXPECT().ListPackageRevisions(ctx, mock.Anything).Return(nil, tt.listError)
					} else {
						mockRepository.EXPECT().ListPackageRevisions(ctx, mock.Anything).Return(nil, nil)
					}
				}
			}

			reconciler := &RepositoryReconciler{Cache: mockCache}
			err := reconciler.syncRepository(ctx, repo)

			assertError(t, tt.expectError, err)
		})
	}
}
func TestPerformAsyncSync(t *testing.T) {
	ctx := context.Background()
	repo := createTestRepo("test-repo", "test-ns")

	tests := []struct {
		name      string
		syncError error
		expectLog string
	}{
		{
			name:      "successful sync",
			syncError: nil,
		},
		{
			name:      "sync fails",
			syncError: errors.New("sync failed"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCache := cachetypes.NewMockCache(t)
			mockRepository := mockRepo.NewMockRepository(t)
			mockClient := mockclient.NewMockClient(t)
			mockStatusWriter := mockclient.NewMockSubResourceWriter(t)

			// Mock sync operations
			mockCache.EXPECT().OpenRepository(ctx, repo).Return(mockRepository, nil)
			mockRepository.EXPECT().Refresh(ctx).Return(tt.syncError)
			if tt.syncError == nil {
				mockRepository.EXPECT().ListPackageRevisions(ctx, mock.Anything).Return(nil, nil)
				// Mock annotation update for full sync timestamp
				mockClient.EXPECT().Patch(ctx, mock.Anything, mock.Anything).Return(nil).Maybe()
				// Mock clearing one-time sync flag
				mockClient.EXPECT().Patch(ctx, mock.Anything, mock.Anything).Return(nil).Maybe()
			}

			// Mock status update
			mockClient.EXPECT().Status().Return(mockStatusWriter)
			mockStatusWriter.EXPECT().Patch(ctx, mock.Anything, mock.Anything, mock.Anything).Return(nil)

			reconciler := &RepositoryReconciler{
				Client:                mockClient,
				Cache:                 mockCache,
				HealthCheckFrequency:  100 * time.Millisecond,
				FullSyncFrequency:     500 * time.Millisecond,
			}

			reconciler.performAsyncSync(ctx, repo)
		})
	}
}

func TestPerformHealthCheckSync(t *testing.T) {
	ctx := context.Background()
	repo := createTestRepo("test-repo", "test-ns")

	tests := []struct {
		name              string
		connectivityError error
		expectError       bool
	}{
		{
			name: "successful health check",
		},
		{
			name:              "connectivity check fails",
			connectivityError: errors.New("connection failed"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCache := cachetypes.NewMockCache(t)
			mockClient := mockclient.NewMockClient(t)
			mockStatusWriter := mockclient.NewMockSubResourceWriter(t)

			// Mock connectivity check
			mockCache.EXPECT().CheckRepositoryConnectivity(ctx, repo).Return(tt.connectivityError)

			// Mock status update
			mockClient.EXPECT().Status().Return(mockStatusWriter)
			mockStatusWriter.EXPECT().Patch(ctx, mock.Anything, mock.Anything, mock.Anything).Return(nil)

			reconciler := &RepositoryReconciler{
				Client:               mockClient,
				Cache:                mockCache,
				HealthCheckFrequency: 100 * time.Millisecond,
			}

			result, err := reconciler.performHealthCheckSync(ctx, repo)

			assertError(t, tt.expectError, err)
			if result.RequeueAfter == 0 {
				t.Error("Expected requeue interval to be set")
			}
		})
	}
}

func TestPerformFullSync(t *testing.T) {
	ctx := context.Background()
	repo := createTestRepo("test-repo", "test-ns")

	tests := []struct {
		name        string
		expectError bool
	}{
		{
			name: "starts async sync successfully",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCache := cachetypes.NewMockCache(t)
			mockClient := mockclient.NewMockClient(t)
			mockStatusWriter := mockclient.NewMockSubResourceWriter(t)
			mockRepository := mockRepo.NewMockRepository(t)

			// Mock status update to sync-in-progress
			mockClient.EXPECT().Status().Return(mockStatusWriter)
			mockStatusWriter.EXPECT().Patch(ctx, mock.Anything, mock.Anything, mock.Anything).Return(nil)

			// Mock async sync operations
			mockCache.EXPECT().OpenRepository(mock.Anything, mock.Anything).Return(mockRepository, nil).Maybe()
			mockRepository.EXPECT().Refresh(mock.Anything).Return(nil).Maybe()
			mockRepository.EXPECT().ListPackageRevisions(mock.Anything, mock.Anything).Return(nil, nil).Maybe()

			// Mock annotation update
			mockClient.EXPECT().Patch(mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()

			// Mock async status update
			mockClient.EXPECT().Status().Return(mockStatusWriter).Maybe()
			mockStatusWriter.EXPECT().Patch(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()

			reconciler := &RepositoryReconciler{
				Client:            mockClient,
				Cache:             mockCache,
				FullSyncFrequency: 500 * time.Millisecond,
				SyncStaleTimeout:  1 * time.Second,
			}
			reconciler.InitializeSyncLimiter()

			result, err := reconciler.performFullSync(ctx, repo)

			// Give async goroutine time to start
			time.Sleep(10 * time.Millisecond)

			assertError(t, tt.expectError, err)
			if result.RequeueAfter == 0 {
				t.Error("Expected requeue interval to be set for stale detection")
			}
		})
	}
}
