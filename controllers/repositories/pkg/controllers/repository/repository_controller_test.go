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

	"github.com/stretchr/testify/mock"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	api "github.com/nephio-project/porch/controllers/repositories/api/v1alpha1"
	mockclient "github.com/nephio-project/porch/test/mockery/mocks/external/sigs.k8s.io/controller-runtime/pkg/client"
	cachetypes "github.com/nephio-project/porch/test/mockery/mocks/porch/pkg/cache/types"
	mockrepo "github.com/nephio-project/porch/test/mockery/mocks/porch/pkg/repository"
)

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
			cache := cachetypes.NewMockCache(t)
			mockRepository := mockrepo.NewMockRepository(t)

			if tt.openError != nil {
				cache.EXPECT().OpenRepository(ctx, repo).Return(nil, tt.openError)
			} else {
				cache.EXPECT().OpenRepository(ctx, repo).Return(mockRepository, nil)
				if tt.refreshError != nil {
					mockRepository.EXPECT().Refresh(ctx).Return(tt.refreshError)
				} else {
					mockRepository.EXPECT().Refresh(ctx).Return(nil)
					if tt.listError != nil {
						mockRepository.EXPECT().ListPackageRevisions(ctx, mock.Anything).Return(nil, tt.listError)
					} else {
						mockRepository.EXPECT().ListPackageRevisions(ctx, mock.Anything).Return(nil, nil)
						mockRepository.EXPECT().BranchCommitHash(ctx).Return("", nil)
					}
				}
			}

			reconciler := &RepositoryReconciler{Cache: cache}
			_, _, err := reconciler.syncRepository(ctx, repo)

			assertError(t, tt.expectError, err)
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
