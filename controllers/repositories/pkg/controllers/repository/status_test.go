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

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	configapi "github.com/nephio-project/porch/controllers/repositories/api/v1alpha1"
)

// Test helper functions
func setupTest() (*runtime.Scheme, context.Context) {
	scheme := runtime.NewScheme()
	_ = configapi.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	return scheme, context.Background()
}

func createFakeClient(scheme *runtime.Scheme, objs ...client.Object) client.Client {
	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).WithStatusSubresource(&configapi.Repository{}).Build()
}

func createReconciler(client client.Client) *RepositoryReconciler {
	return &RepositoryReconciler{Client: client}
}

// Unified mock client for all test scenarios
type configurableMockClient struct {
	client.Client
	getError    error
	updateError error
	callCount   int
}

func (m *configurableMockClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if m.getError != nil {
		return m.getError
	}
	return m.Client.Get(ctx, key, obj, opts...)
}

func (m *configurableMockClient) Status() client.StatusWriter {
	return &configurableMockStatusWriter{
		StatusWriter: m.Client.Status(),
		updateError:  m.updateError,
		client:       m,
	}
}

type configurableMockStatusWriter struct {
	client.StatusWriter
	updateError error
	client      *configurableMockClient
}

func (m *configurableMockStatusWriter) Update(ctx context.Context, obj client.Object, opts ...client.SubResourceUpdateOption) error {
	m.client.callCount++
	// Return error only on first call for conflict test
	if m.updateError != nil && m.client.callCount == 1 {
		return m.updateError
	}
	return m.StatusWriter.Update(ctx, obj, opts...)
}

func TestProcessSyncEvent(t *testing.T) {
	scheme, ctx := setupTest()

	tests := []struct {
		name        string
		repo        *configapi.Repository
		event       *corev1.Event
		expectError bool
		expectCall  bool
	}{
		{
			name: "sync started event",
			repo: createTestRepo("test-repo", "test-ns"),
			event: &corev1.Event{
				Reason:  "SyncStarted",
				Message: "Sync started",
			},
			expectCall: true,
		},
		{
			name: "sync completed event",
			repo: createTestRepo("test-repo", "test-ns"),
			event: &corev1.Event{
				Reason:  "SyncCompleted",
				Message: "Sync completed",
			},
			expectCall: true,
		},
		{
			name: "sync failed event",
			repo: createTestRepo("test-repo", "test-ns"),
			event: &corev1.Event{
				Reason:  "SyncFailed",
				Message: "Connection failed",
			},
			expectCall: true,
		},
		{
			name: "connectivity failure blocks sync event",
			repo: createTestRepoWithCondition("test-repo", "test-ns", metav1.Condition{
				Type:    configapi.RepositoryReady,
				Status:  metav1.ConditionFalse,
				Reason:  configapi.ReasonError,
				Message: "Connectivity check failed: invalid branch",
			}),
			event: &corev1.Event{
				Reason:  "SyncFailed",
				Message: "Connection failed",
			},
			expectCall: false,
		},
		{
			name: "non-sync event ignored",
			repo: createTestRepo("test-repo", "test-ns"),
			event: &corev1.Event{
				Reason:  "Created",
				Message: "Repository created",
			},
			expectCall: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient := createFakeClient(scheme, tt.repo)
			reconciler := createReconciler(fakeClient)

			err := reconciler.processSyncEvent(ctx, tt.repo, tt.event)
			assertError(t, tt.expectError, err)
		})
	}
}

func TestSetCondition(t *testing.T) {
	repo := createTestRepo("test-repo", "test-ns")
	repo.Generation = 5

	reconciler := &RepositoryReconciler{}
	reconciler.setCondition(repo, configapi.RepositoryReady, metav1.ConditionTrue, configapi.ReasonReady, "Repository is ready")

	if len(repo.Status.Conditions) != 1 {
		t.Errorf("Expected 1 condition, got %d", len(repo.Status.Conditions))
	}

	condition := repo.Status.Conditions[0]
	if condition.Type != configapi.RepositoryReady {
		t.Errorf("Expected type %s, got %s", configapi.RepositoryReady, condition.Type)
	}
	if condition.Status != metav1.ConditionTrue {
		t.Errorf("Expected status True, got %s", condition.Status)
	}
	if condition.Reason != configapi.ReasonReady {
		t.Errorf("Expected reason %s, got %s", configapi.ReasonReady, condition.Reason)
	}
	if condition.ObservedGeneration != 5 {
		t.Errorf("Expected ObservedGeneration 5, got %d", condition.ObservedGeneration)
	}
}

func TestUpdateRepoStatusWithBackoff(t *testing.T) {
	scheme, ctx := setupTest()

	tests := []struct {
		name        string
		clientError error
		expectError bool
	}{
		{
			name:        "successful update",
			clientError: nil,
			expectError: false,
		},
		{
			name:        "conflict error retries",
			clientError: apierrors.NewConflict(schema.GroupResource{}, "test", errors.New("conflict")),
			expectError: false,
		},
		{
			name:        "non-conflict error fails",
			clientError: errors.New("network error"),
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := createTestRepo("test-repo", "test-ns")
			
			var fakeClient client.Client
			if tt.clientError != nil {
				fakeClient = &configurableMockClient{
					Client:      createFakeClient(scheme, repo),
					updateError: tt.clientError,
				}
			} else {
				fakeClient = createFakeClient(scheme, repo)
			}

			reconciler := createReconciler(fakeClient)
			err := reconciler.updateRepoStatusWithBackoff(ctx, repo)
			assertError(t, tt.expectError, err)
		})
	}
}

func TestHasSpecChanged(t *testing.T) {
	tests := []struct {
		name     string
		repo     *configapi.Repository
		expected bool
	}{
		{
			name:     "no conditions - spec changed",
			repo:     createTestRepo("test-repo", "test-ns"),
			expected: true,
		},
		{
			name: "observed generation matches - no change",
			repo: func() *configapi.Repository {
				repo := createTestRepo("test-repo", "test-ns")
				repo.Generation = 5
				repo.Status.Conditions = []metav1.Condition{{
					Type:               configapi.RepositoryReady,
					ObservedGeneration: 5,
				}}
				return repo
			}(),
			expected: false,
		},
		{
			name: "observed generation differs - spec changed",
			repo: func() *configapi.Repository {
				repo := createTestRepo("test-repo", "test-ns")
				repo.Generation = 6
				repo.Status.Conditions = []metav1.Condition{{
					Type:               configapi.RepositoryReady,
					ObservedGeneration: 5,
				}}
				return repo
			}(),
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reconciler := &RepositoryReconciler{}
			result := reconciler.hasSpecChanged(tt.repo)

			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestGetNextSyncAt(t *testing.T) {
	// Use RFC3339 formatted times to match parsing precision
	futureTime, _ := time.Parse(time.RFC3339, time.Now().Add(1*time.Hour).Format(time.RFC3339))
	pastTime, _ := time.Parse(time.RFC3339, time.Now().Add(-1*time.Hour).Format(time.RFC3339))

	tests := []struct {
		name     string
		event    *corev1.Event
		expected *time.Time
	}{
		{
			name: "annotation with future time",
			event: &corev1.Event{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"porch.kpt.dev/next-sync-time": futureTime.Format(time.RFC3339),
					},
				},
			},
			expected: &futureTime,
		},
		{
			name: "annotation with past time ignored",
			event: &corev1.Event{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"porch.kpt.dev/next-sync-time": pastTime.Format(time.RFC3339),
					},
				},
			},
			expected: nil,
		},
		{
			name: "message fallback with future time",
			event: &corev1.Event{
				Message: "Sync completed (next sync at: " + futureTime.Format(time.RFC3339) + ")",
			},
			expected: &futureTime,
		},
		{
			name: "no next sync time",
			event: &corev1.Event{
				Message: "Sync completed",
			},
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reconciler := &RepositoryReconciler{}
			result := reconciler.getNextSyncAt(tt.event)

			if tt.expected == nil {
				if result != nil {
					t.Errorf("Expected nil, got %v", result)
				}
			} else {
				if result == nil {
					t.Error("Expected time, got nil")
				} else if !result.Equal(*tt.expected) {
					t.Errorf("Expected %v, got %v", tt.expected, result)
				}
			}
		})
	}
}

func TestHasConnectivityFailure(t *testing.T) {
	tests := []struct {
		name     string
		repo     *configapi.Repository
		expected bool
	}{
		{
			name: "connectivity failure present",
			repo: createTestRepoWithCondition("test-repo", "test-ns", metav1.Condition{
				Type:    configapi.RepositoryReady,
				Status:  metav1.ConditionFalse,
				Reason:  configapi.ReasonError,
				Message: "Connectivity check failed: branch not found",
			}),
			expected: true,
		},
		{
			name: "different error message",
			repo: createTestRepoWithCondition("test-repo", "test-ns", metav1.Condition{
				Type:    configapi.RepositoryReady,
				Status:  metav1.ConditionFalse,
				Reason:  configapi.ReasonError,
				Message: "Authentication failed",
			}),
			expected: false,
		},
		{
			name: "ready condition",
			repo: createTestRepoWithCondition("test-repo", "test-ns", metav1.Condition{
				Type:   configapi.RepositoryReady,
				Status: metav1.ConditionTrue,
				Reason: configapi.ReasonReady,
			}),
			expected: false,
		},
		{
			name:     "no conditions",
			repo:     createTestRepo("test-repo", "test-ns"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, ctx := setupTest()
			event := &corev1.Event{Reason: "SyncFailed"}

			reconciler := &RepositoryReconciler{}
			result := reconciler.hasConnectivityFailure(ctx, tt.repo, event)

			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestUpdateRepoStatusOnError(t *testing.T) {
	scheme, ctx := setupTest()

	tests := []struct {
		name        string
		getError    error
		updateError error
		testError   error
	}{
		{
			name:      "successful status update",
			testError: errors.New("cache operation failed"),
		},
		{
			name:      "get repository error",
			getError:  errors.New("repository not found"),
			testError: errors.New("cache operation failed"),
		},
		{
			name:        "status update error",
			updateError: errors.New("status update failed"),
			testError:   errors.New("cache operation failed"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := createTestRepo("test-repo", "test-ns")
			
			fakeClient := &configurableMockClient{
				Client:      createFakeClient(scheme, repo),
				getError:    tt.getError,
				updateError: tt.updateError,
			}

			reconciler := createReconciler(fakeClient)
			// This function doesn't return errors, it logs them
			reconciler.updateRepoStatusOnError(ctx, repo, tt.testError)
			// Test passes if no panic occurs
		})
	}
}
