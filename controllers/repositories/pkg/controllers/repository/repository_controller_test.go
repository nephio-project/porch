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
	"net/http"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/mock"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	api "github.com/nephio-project/porch/controllers/repositories/api/v1alpha1"
	"github.com/nephio-project/porch/pkg/repository"
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
	}{
		{
			name: "cache not available",
			repo: createTestRepo("cache-nil-repo", "test-ns"),
			cacheNil: true,
			expectError: false, // Controller returns early without error after finalizer addition
		},
		{
			name: "repository not found",
			getError: apierrors.NewNotFound(schema.GroupResource{}, "test-repo"),
		},
		{
			name: "get repository error",
			getError: errors.New("get failed"),
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
			name: "spec changed - triggers upsert",
			repo: createTestRepo("test-repo", "test-ns"),
		},
		{
			name: "no spec change - handles cache events",
			repo: createTestRepoWithCondition("test-repo", "test-ns", metav1.Condition{
				Type: api.RepositoryReady,
				ObservedGeneration: 1,
			}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := mockclient.NewMockClient(t)
			var mockCache *cachetypes.MockCache
			
			if !tt.cacheNil {
				mockCache = cachetypes.NewMockCache(t)
			}

			// Setup Get expectation (always called first)
			if tt.getError != nil {
				mockClient.EXPECT().Get(ctx, req.NamespacedName, &api.Repository{}).Return(tt.getError)
			} else if tt.repo != nil {
				mockClient.EXPECT().Get(ctx, req.NamespacedName, &api.Repository{}).Run(func(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) {
					repo := obj.(*api.Repository)
					*repo = *tt.repo
				}).Return(nil)
			} else {
				// For cache nil test, still need Get expectation
				mockClient.EXPECT().Get(ctx, req.NamespacedName, &api.Repository{}).Return(nil)
			}

			reconciler := &RepositoryReconciler{
				Client: mockClient,
				Cache: mockCache,
				connectivityRetryInterval: 10 * time.Second,
			}

			// Mock deletion handling if needed
			if tt.expectDeletion && mockCache != nil {
				mockClient.EXPECT().List(mock.Anything, &api.RepositoryList{}).Return(nil)
				mockCache.EXPECT().CloseRepository(mock.Anything, tt.repo, mock.Anything).Return(nil)
				mockClient.EXPECT().Update(mock.Anything, tt.repo).Return(nil)
			}

			// Mock upsert handling - finalizer addition happens even with cache nil
			if tt.repo != nil && !tt.expectDeletion {
				if reconciler.hasSpecChanged(tt.repo) || tt.cacheNil {
					mockClient.EXPECT().Update(mock.Anything, mock.MatchedBy(func(repo *api.Repository) bool {
						return repo.Name == tt.repo.Name
					})).Return(nil)
				}
			}

			result, err := reconciler.Reconcile(ctx, req)

			assertError(t, tt.expectError, err)
			assertRequeue(t, tt.expectRequeue, result)
		})
	}
}

func TestEnsureFinalizer(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name         string
		repo         *api.Repository
		updateError  error
		expectAdded  bool
		expectError  bool
	}{
		{
			name: "adds finalizer when missing",
			repo: createTestRepo("test-repo", "test-ns"),
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
			name: "update fails",
			repo: createTestRepo("test-repo", "test-ns"),
			updateError: errors.New("update failed"),
			expectAdded: false,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := mockclient.NewMockClient(t)
			// Set up mock expectation if finalizer needs to be added
			if !controllerutil.ContainsFinalizer(tt.repo, RepositoryFinalizer) {
				mockClient.EXPECT().Update(mock.Anything, mock.Anything, mock.Anything).Return(tt.updateError)
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

func TestOpenRepository(t *testing.T) {
	ctx := context.Background()
	repo := createTestRepo("test-repo", "test-ns")

	tests := []struct {
		name              string
		connectivityError error
		openRepoError     error
		expectError       bool
	}{
		{
			name: "successful open",
		},
		{
			name:              "connectivity fails",
			connectivityError: errors.New("connectivity failed"),
			expectError:       true,
		},
		{
			name:          "open repository fails",
			openRepoError: errors.New("open failed"),
			expectError:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCache := cachetypes.NewMockCache(t)
			mockRepo := mockRepo.NewMockRepository(t)
			mockClient := mockclient.NewMockClient(t)

			if tt.connectivityError != nil {
				mockCache.EXPECT().CheckRepositoryConnectivity(ctx, repo).Return(tt.connectivityError)
				// Mock status update for connectivity failure
				mockClient.EXPECT().Status().Return(&mockStatusWriter{})
			} else {
				mockCache.EXPECT().CheckRepositoryConnectivity(ctx, repo).Return(nil)
				if tt.openRepoError != nil {
					mockCache.EXPECT().OpenRepository(ctx, repo).Return(nil, tt.openRepoError)
				} else {
					mockCache.EXPECT().OpenRepository(ctx, repo).Return(mockRepo, nil)
				}
			}

			reconciler := &RepositoryReconciler{Client: mockClient, Cache: mockCache}
			_, err := reconciler.openRepository(ctx, repo)

			assertError(t, tt.expectError, err)
		})
	}
}

func TestRefreshAndValidateRepository(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name              string
		repo              *api.Repository
		refreshError      error
		listPackagesError error
		expectError       bool
	}{
		{
			name: "no spec change - skips refresh",
			repo: createTestRepoWithCondition("test-repo", "test-ns", metav1.Condition{
				ObservedGeneration: 1,
			}),
		},
		{
			name: "spec changed - refresh and validation succeeds",
			repo: createTestRepo("test-repo", "test-ns"),
		},
		{
			name:         "refresh fails",
			repo:         createTestRepo("test-repo", "test-ns"),
			refreshError: errors.New("git fetch failed"),
			expectError:  true,
		},
		{
			name:              "package listing fails",
			repo:              createTestRepo("test-repo", "test-ns"),
			listPackagesError: errors.New("package scan failed"),
			expectError:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := mockRepo.NewMockRepository(t)
			reconciler := &RepositoryReconciler{}

			if reconciler.hasSpecChanged(tt.repo) {
				if tt.refreshError != nil {
					mockRepo.EXPECT().Refresh(ctx).Return(tt.refreshError)
				} else {
					mockRepo.EXPECT().Refresh(ctx).Return(nil)
					if tt.listPackagesError != nil {
						mockRepo.EXPECT().ListPackageRevisions(ctx, repository.ListPackageRevisionFilter{}).Return(nil, tt.listPackagesError)
					} else {
						mockRepo.EXPECT().ListPackageRevisions(ctx, repository.ListPackageRevisionFilter{}).Return(nil, nil)
					}
				}
			}

			err := reconciler.refreshAndValidateRepository(ctx, tt.repo, mockRepo)

			assertError(t, tt.expectError, err)
		})
	}
}

func TestHandleUpsertRepo(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name          string
		repo          *api.Repository
		expectError   bool
		expectRequeue bool
	}{
		{
			name: "successful reconciliation",
			repo: func() *api.Repository {
				repo := createTestRepo("test-repo", "test-ns")
				controllerutil.AddFinalizer(repo, RepositoryFinalizer)
				return repo
			}(),
		},
		{
			name: "finalizer needs to be added",
			repo: createTestRepo("test-repo", "test-ns"),
			expectError: false, // Returns early after adding finalizer
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := mockclient.NewMockClient(t)
			mockCache := cachetypes.NewMockCache(t)
			mockRepo := mockRepo.NewMockRepository(t)

			switch tt.name {
			case "successful reconciliation":
				// Mock successful path
				mockCache.EXPECT().CheckRepositoryConnectivity(ctx, tt.repo).Return(nil)
				mockCache.EXPECT().OpenRepository(ctx, tt.repo).Return(mockRepo, nil)
				// Mock refresh and validation for spec changes
				mockRepo.EXPECT().Refresh(ctx).Return(nil)
				mockRepo.EXPECT().ListPackageRevisions(ctx, repository.ListPackageRevisionFilter{}).Return(nil, nil)
				// Mock status updates
				mockClient.EXPECT().Status().Return(&mockStatusWriter{})
			case "finalizer needs to be added":
				// Mock finalizer addition
				mockClient.EXPECT().Update(mock.Anything, mock.Anything, mock.Anything).Return(nil)
			}

			reconciler := &RepositoryReconciler{
				Client: mockClient,
				Cache:  mockCache,
				connectivityRetryInterval: 10 * time.Second,
			}

			result, err := reconciler.handleUpsertRepo(ctx, tt.repo)

			assertError(t, tt.expectError, err)
			assertRequeue(t, tt.expectRequeue, result)
		})
	}
}

func TestSetupWithManager(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = api.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	// Create fake manager
	mgr := &fakeManager{
		scheme: scheme,
		client: mockclient.NewMockClient(t),
	}

	reconciler := &RepositoryReconciler{
		Cache:                   cachetypes.NewMockCache(t),
		maxConcurrentReconciles: 5,
	}

	err := reconciler.SetupWithManager(mgr)
	if err != nil {
		t.Errorf("SetupWithManager failed: %v", err)
	}

	// Verify client was set
	if reconciler.Client == nil {
		t.Error("Client should be set by SetupWithManager")
	}
}

// Mock status writer for testing
type mockStatusWriter struct {
	updateError error
}

func (m *mockStatusWriter) Update(ctx context.Context, obj client.Object, opts ...client.SubResourceUpdateOption) error {
	return m.updateError
}

func (m *mockStatusWriter) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) error {
	return m.updateError
}

func (m *mockStatusWriter) Create(ctx context.Context, obj client.Object, subResource client.Object, opts ...client.SubResourceCreateOption) error {
	return m.updateError
}

// fakeManager implements minimal ctrl.Manager interface for testing
type fakeManager struct {
	scheme *runtime.Scheme
	client client.Client
}

func (f *fakeManager) GetConfig() *rest.Config { return nil }
func (f *fakeManager) GetScheme() *runtime.Scheme { return f.scheme }
func (f *fakeManager) GetClient() client.Client { return f.client }
func (f *fakeManager) GetFieldIndexer() client.FieldIndexer { return &fakeFieldIndexer{} }
func (f *fakeManager) GetCache() cache.Cache { return nil }
func (f *fakeManager) GetEventRecorderFor(name string) record.EventRecorder { return nil }
func (f *fakeManager) GetRESTMapper() meta.RESTMapper { return nil }
func (f *fakeManager) GetAPIReader() client.Reader { return nil }
func (f *fakeManager) Start(ctx context.Context) error { return nil }
func (f *fakeManager) Add(manager.Runnable) error { return nil }
func (f *fakeManager) Elected() <-chan struct{} { return nil }
func (f *fakeManager) AddMetricsServerExtraHandler(path string, handler http.Handler) error { return nil }
func (f *fakeManager) AddHealthzCheck(name string, check healthz.Checker) error { return nil }
func (f *fakeManager) AddReadyzCheck(name string, check healthz.Checker) error { return nil }
func (f *fakeManager) GetWebhookServer() webhook.Server { return nil }
func (f *fakeManager) GetLogger() logr.Logger { return logr.Discard() }
func (f *fakeManager) GetControllerOptions() config.Controller { return config.Controller{} }
func (f *fakeManager) GetHTTPClient() *http.Client { return nil }

type fakeFieldIndexer struct{}
func (f *fakeFieldIndexer) IndexField(ctx context.Context, obj client.Object, field string, extractValue client.IndexerFunc) error {
	return nil
}
func TestSetEmbeddedDefaults(t *testing.T) {
	reconciler := &RepositoryReconciler{}
	reconciler.SetEmbeddedDefaults()
	
	// Verify embedded defaults are set
	if reconciler.maxConcurrentReconciles == 0 {
		t.Error("maxConcurrentReconciles should be set")
	}
	if reconciler.connectivityRetryInterval == 0 {
		t.Error("connectivityRetryInterval should be set")
	}
}
