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
	mockrepo "github.com/nephio-project/porch/test/mockery/mocks/porch/pkg/repository"
)

// Test helpers

func setupMockStatusWriter(t *testing.T, mockClient *mockclient.MockClient, returnErr error) *mockclient.MockSubResourceWriter {
	mockStatusWriter := mockclient.NewMockSubResourceWriter(t)
	mockClient.EXPECT().Status().Return(mockStatusWriter).Maybe()
	mockStatusWriter.EXPECT().Patch(
		mock.Anything,
		mock.Anything,
		mock.Anything,
		mock.Anything,
	).Return(returnErr).Maybe()
	return mockStatusWriter
}

func setupSuccessfulSync(t *testing.T, ctx context.Context, mockCache *cachetypes.MockCache, repo *api.Repository) *mockrepo.MockRepository {
	mockRepository := mockrepo.NewMockRepository(t)
	mockCache.EXPECT().OpenRepository(ctx, repo).Return(mockRepository, nil)
	mockRepository.EXPECT().Refresh(ctx).Return(nil)
	mockRepository.EXPECT().ListPackageRevisions(ctx, mock.Anything).Return(nil, nil)
	mockRepository.EXPECT().BranchCommitHash(ctx).Return("abc123", nil)
	return mockRepository
}

func newTestReconciler(mockClient *mockclient.MockClient, mockCache *cachetypes.MockCache) *RepositoryReconciler {
	r := &RepositoryReconciler{
		Client:               mockClient,
		Cache:                mockCache,
		HealthCheckFrequency: 5 * time.Minute,
		FullSyncFrequency:    1 * time.Hour,
	}
	r.InitializeSyncLimiter()
	return r
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
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			repo := tt.repo.DeepCopy()
			mockClient := mockclient.NewMockClient(t)
			if !controllerutil.ContainsFinalizer(repo, RepositoryFinalizer) {
				mockClient.EXPECT().Patch(mock.Anything, mock.Anything, mock.Anything).Return(tt.updateError)
			}

			reconciler := &RepositoryReconciler{Client: mockClient}
			added, err := reconciler.ensureFinalizer(ctx, repo)

			assertError(t, tt.expectError, err)
			if added != tt.expectAdded {
				t.Errorf("Expected added %v, got %v", tt.expectAdded, added)
			}
		})
	}
}

func TestReconcileNotFound(t *testing.T) {
	ctx := context.Background()
	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: "test-repo", Namespace: "test-ns"}}

	mockClient := mockclient.NewMockClient(t)
	cache := cachetypes.NewMockCache(t)

	mockClient.EXPECT().Get(ctx, req.NamespacedName, &api.Repository{}).Return(apierrors.NewNotFound(schema.GroupResource{}, "test-repo"))

	reconciler := &RepositoryReconciler{
		Client: mockClient,
		Cache:  cache,
	}

	result, err := reconciler.Reconcile(ctx, req)

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if result.Requeue {
		t.Error("Expected no requeue")
	}
}

func TestReconcileGetError(t *testing.T) {
	ctx := context.Background()
	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: "test-repo", Namespace: "test-ns"}}

	mockClient := mockclient.NewMockClient(t)
	cache := cachetypes.NewMockCache(t)

	mockClient.EXPECT().Get(ctx, req.NamespacedName, &api.Repository{}).Return(errors.New("get failed"))

	reconciler := &RepositoryReconciler{
		Client: mockClient,
		Cache:  cache,
	}

	_, err := reconciler.Reconcile(ctx, req)

	if err == nil {
		t.Error("Expected error, got nil")
	}
}

func TestReconcileCacheNil(t *testing.T) {
	ctx := context.Background()
	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: "test-repo", Namespace: "test-ns"}}

	mockClient := mockclient.NewMockClient(t)

	reconciler := &RepositoryReconciler{
		Client: mockClient,
		Cache:  nil,
	}

	_, err := reconciler.Reconcile(ctx, req)

	if err == nil {
		t.Error("Expected error when cache is nil")
	}
}

func TestInitializeSyncLimiter(t *testing.T) {
	tests := []struct {
		name                string
		maxConcurrentSyncs  int
		expectedCapacity    int
	}{
		{
			name:                "uses custom value",
			maxConcurrentSyncs:  50,
			expectedCapacity:    50,
		},
		{
			name:                "uses default when zero",
			maxConcurrentSyncs:  0,
			expectedCapacity:    100,
		},
		{
			name:                "uses default when negative",
			maxConcurrentSyncs:  -1,
			expectedCapacity:    100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &RepositoryReconciler{
				MaxConcurrentSyncs: tt.maxConcurrentSyncs,
			}
			r.InitializeSyncLimiter()

			if r.MaxConcurrentSyncs != tt.expectedCapacity {
				t.Errorf("Expected MaxConcurrentSyncs %d, got %d", tt.expectedCapacity, r.MaxConcurrentSyncs)
			}
			if cap(r.syncLimiter) != tt.expectedCapacity {
				t.Errorf("Expected channel capacity %d, got %d", tt.expectedCapacity, cap(r.syncLimiter))
			}
		})
	}
}

func TestPerformAsyncSync(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name      string
		syncError error
	}{
		{name: "successful sync", syncError: nil},
		{name: "sync fails", syncError: errors.New("sync failed")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := createTestRepo("test-repo", "test-ns")
			mockClient := mockclient.NewMockClient(t)
			mockCache := cachetypes.NewMockCache(t)

			if tt.syncError != nil {
				mockCache.EXPECT().OpenRepository(ctx, repo).Return(nil, tt.syncError)
			} else {
				setupSuccessfulSync(t, ctx, mockCache, repo)
			}

			setupMockStatusWriter(t, mockClient, nil)
			mockClient.EXPECT().Patch(mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()

			r := &RepositoryReconciler{Client: mockClient, Cache: mockCache}
			r.performAsyncSync(ctx, repo)
		})
	}
}

func TestReconcileDecisionBranches(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name           string
		repo           *api.Repository
		expectRequeue  bool
		expectError    bool
	}{
		{
			name: "repo being deleted",
			repo: func() *api.Repository {
				repo := createTestRepo("test-repo", "test-ns")
				now := metav1.Now()
				repo.DeletionTimestamp = &now
				controllerutil.AddFinalizer(repo, RepositoryFinalizer)
				return repo
			}(),
			expectRequeue: false,
			expectError:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := tt.repo.DeepCopy()
			req := ctrl.Request{NamespacedName: types.NamespacedName{Name: repo.Name, Namespace: repo.Namespace}}

			mockClient := mockclient.NewMockClient(t)
			mockCache := cachetypes.NewMockCache(t)

			// Mock Get to return our repo
			mockClient.EXPECT().Get(
				ctx,
				req.NamespacedName,
				mock.AnythingOfType("*v1alpha1.Repository"),
			).Run(func(
				ctx context.Context,
				key types.NamespacedName,
				obj client.Object,
				opts ...client.GetOption,
			) {
				if r, ok := obj.(*api.Repository); ok {
					*r = *repo
				}
			}).Return(nil)

			// Mock deletion flow
			if !repo.DeletionTimestamp.IsZero() {
				mockClient.EXPECT().List(
					mock.Anything,
					mock.Anything,
					mock.Anything,
				).Return(nil).Maybe()
				mockCache.EXPECT().CloseRepository(
					mock.Anything,
					repo,
					mock.Anything,
				).Return(nil).Maybe()
				mockClient.EXPECT().Update(
					mock.Anything,
					mock.Anything,
				).Return(nil).Maybe()
				mockClient.EXPECT().Patch(
					mock.Anything,
					mock.Anything,
					mock.Anything,
				).Return(nil).Maybe()
			}

			r := &RepositoryReconciler{
				Client: mockClient,
				Cache:  mockCache,
			}

			result, err := r.Reconcile(ctx, req)

			assertError(t, tt.expectError, err)
			if result.Requeue != tt.expectRequeue {
				t.Errorf("Expected requeue %v, got %v", tt.expectRequeue, result.Requeue)
			}
		})
	}
}

func TestPerformHealthCheckSync(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name            string
		connectivityErr error
	}{
		{name: "health check passes", connectivityErr: nil},
		{name: "health check fails", connectivityErr: errors.New("connection failed")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := createTestRepo("test-repo", "test-ns")
			mockClient := mockclient.NewMockClient(t)
			mockCache := cachetypes.NewMockCache(t)

			mockCache.EXPECT().CheckRepositoryConnectivity(ctx, repo).Return(tt.connectivityErr)
			setupMockStatusWriter(t, mockClient, nil)

			r := newTestReconciler(mockClient, mockCache)
			result, err := r.performHealthCheckSync(ctx, repo)

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			if result.RequeueAfter == 0 {
				t.Error("Expected requeue")
			}
		})
	}
}

func TestPerformFullSync(t *testing.T) {
	ctx := context.Background()
	repo := createTestRepo("test-repo", "test-ns")

	t.Run("capacity exceeded", func(t *testing.T) {
		mockClient := mockclient.NewMockClient(t)
		setupMockStatusWriter(t, mockClient, nil) // For sync in progress
		setupMockStatusWriter(t, mockClient, nil) // For capacity exceeded

		r := &RepositoryReconciler{Client: mockClient, MaxConcurrentSyncs: 1}
		r.InitializeSyncLimiter()
		r.syncLimiter <- struct{}{} // Fill the limiter

		result, err := r.performFullSync(ctx, repo)

		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if result.RequeueAfter != 30*time.Second {
			t.Errorf("Expected 30s requeue, got %v", result.RequeueAfter)
		}
		<-r.syncLimiter // Clean up
	})

	t.Run("status update error", func(t *testing.T) {
		mockClient := mockclient.NewMockClient(t)
		setupMockStatusWriter(t, mockClient, errors.New("status update failed"))

		r := &RepositoryReconciler{Client: mockClient}
		r.InitializeSyncLimiter()

		_, err := r.performFullSync(ctx, repo)
		if err == nil {
			t.Error("Expected error from status update")
		}
	})
}

func TestPerformAsyncSyncEdgeCases(t *testing.T) {
	ctx := context.Background()

	t.Run("clear flag error", func(t *testing.T) {
		repo := createTestRepo("test-repo", "test-ns")
		repo.Spec.Sync = &api.RepositorySync{RunOnceAt: &metav1.Time{Time: time.Now()}}

		mockClient := mockclient.NewMockClient(t)
		mockCache := cachetypes.NewMockCache(t)

		setupSuccessfulSync(t, ctx, mockCache, repo)
		setupMockStatusWriter(t, mockClient, nil)
		mockClient.EXPECT().Patch(mock.Anything, mock.Anything, mock.Anything).Return(errors.New("patch failed"))

		r := &RepositoryReconciler{Client: mockClient, Cache: mockCache}
		r.performAsyncSync(ctx, repo)
	})

	t.Run("repo deleted during sync", func(t *testing.T) {
		repo := createTestRepo("test-repo", "test-ns")
		mockClient := mockclient.NewMockClient(t)
		mockCache := cachetypes.NewMockCache(t)

		setupSuccessfulSync(t, ctx, mockCache, repo)
		setupMockStatusWriter(t, mockClient, apierrors.NewNotFound(schema.GroupResource{}, "test-repo"))

		r := &RepositoryReconciler{Client: mockClient, Cache: mockCache}
		r.performAsyncSync(ctx, repo)
	})
}
