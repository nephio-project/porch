// Copyright 2025 The kpt and Nephio Authors
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

package sync

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	configapi "github.com/nephio-project/porch/api/porchconfig/v1alpha1"
	"github.com/nephio-project/porch/pkg/repository"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"sigs.k8s.io/controller-runtime/pkg/client"
	k8sfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type fakeClientWithStatus struct {
	client.Client
	statusStore map[types.NamespacedName]configapi.RepositoryStatus
}

func newFakeClientWithStatus(scheme *runtime.Scheme, objs ...client.Object) *fakeClientWithStatus {
	baseClient := k8sfake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
	return &fakeClientWithStatus{
		Client:      baseClient,
		statusStore: make(map[types.NamespacedName]configapi.RepositoryStatus),
	}
}

func (f *fakeClientWithStatus) Status() client.StatusWriter {
	return &fakeStatusWriter{f}
}

type fakeStatusWriter struct {
	f *fakeClientWithStatus
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

func (f *fakeClientWithStatus) Watch(ctx context.Context, list client.ObjectList, opts ...client.ListOption) (watch.Interface, error) {
	return watch.NewEmptyWatch(), nil
}

func TestSetRepositoryCondition(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = configapi.AddToScheme(scheme)

	repoName := "test-repo"
	namespace := "test-ns"
	repoKey := repository.RepositoryKey{Name: repoName, Namespace: namespace}

	repoObj := &configapi.Repository{
		ObjectMeta: metav1.ObjectMeta{Name: repoName, Namespace: namespace},
	}
	repoObj.SetGroupVersionKind(configapi.GroupVersion.WithKind("Repository"))

	tests := []struct {
		name           string
		status         string
		syncError      error
		nextSyncTime   *time.Time
		expectedStatus metav1.ConditionStatus
		expectedReason string
		expectedMsg    string
		expectErr      bool
	}{
		{
			name:           "ready status",
			status:         "ready",
			expectedStatus: metav1.ConditionTrue,
			expectedReason: configapi.ReasonReady,
			expectedMsg:    "Repository Ready",
		},
		{
			name:           "ready with next sync time",
			status:         "ready",
			nextSyncTime:   &time.Time{},
			expectedStatus: metav1.ConditionTrue,
			expectedReason: configapi.ReasonReady,
		},
		{
			name:           "error with message",
			status:         "error",
			syncError:      errors.New("sync failed"),
			expectedStatus: metav1.ConditionFalse,
			expectedReason: configapi.ReasonError,
			expectedMsg:    "sync failed",
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
			name:      "unknown status",
			status:    "unknown",
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := newFakeClientWithStatus(scheme, repoObj.DeepCopy())

			err := SetRepositoryCondition(context.TODO(), client, repoKey, tt.status, tt.syncError, tt.nextSyncTime)

			if tt.expectErr {
				if err == nil {
					t.Error("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			status, ok := client.statusStore[types.NamespacedName{Name: repoName, Namespace: namespace}]
			if !ok {
				t.Error("Expected status to be updated")
				return
			}

			if len(status.Conditions) != 1 {
				t.Errorf("Expected 1 condition, got %d", len(status.Conditions))
				return
			}

			cond := status.Conditions[0]
			if cond.Type != configapi.RepositoryReady {
				t.Errorf("Expected condition type %s, got %s", configapi.RepositoryReady, cond.Type)
			}
			if cond.Status != tt.expectedStatus {
				t.Errorf("Expected status %s, got %s", tt.expectedStatus, cond.Status)
			}
			if cond.Reason != tt.expectedReason {
				t.Errorf("Expected reason %s, got %s", tt.expectedReason, cond.Reason)
			}
			if tt.expectedMsg != "" && cond.Message != tt.expectedMsg {
				t.Errorf("Expected message %s, got %s", tt.expectedMsg, cond.Message)
			}
		})
	}
}