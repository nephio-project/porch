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

package util

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	configapi "github.com/nephio-project/porch/api/porchconfig/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
)

// FakeClientWithStatusUpdate is a fake client that supports status updates
type FakeClientWithStatusUpdate struct {
	client.Client
	statusStore map[types.NamespacedName]configapi.RepositoryStatus
}

func NewFakeClientWithStatus(scheme *runtime.Scheme, objs ...client.Object) *FakeClientWithStatusUpdate {
	baseClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
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

func TestBuildRepositoryCondition(t *testing.T) {
	repo := &configapi.Repository{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-repo",
			Namespace:  "default",
			Generation: 1,
		},
	}

	nextSync := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name      string
		status    string
		errorMsg  string
		expected  metav1.ConditionStatus
		reason    string
		message   string
		expectErr bool
	}{
		{
			name:     "SyncInProgress",
			status:   "sync-in-progress",
			errorMsg: "",
			expected: metav1.ConditionFalse,
			reason:   configapi.ReasonReconciling,
			message:  "Repository reconciliation in progress",
		},
		{
			name:     "Ready",
			status:   "ready",
			errorMsg: "",
			expected: metav1.ConditionTrue,
			reason:   configapi.ReasonReady,
			message:  "Repository Ready",
		},
		{
			name:     "ErrorWithMessage",
			status:   "error",
			errorMsg: "some error",
			expected: metav1.ConditionFalse,
			reason:   configapi.ReasonError,
			message:  "some error",
		},
		{
			name:     "ErrorWithoutMessage",
			status:   "error",
			errorMsg: "",
			expected: metav1.ConditionFalse,
			reason:   configapi.ReasonError,
			message:  "unknown error",
		},
		{
			name:      "UnknownStatus",
			status:    "unknown",
			errorMsg:  "",
			expectErr: true,
		},
		{
			name:     "ReadyWithNextSync",
			status:   "ready",
			errorMsg: "",
			expected: metav1.ConditionTrue,
			reason:   configapi.ReasonReady,
			message:  "Repository Ready (next sync scheduled at: 2025-01-01T12:00:00Z)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var nextSyncTime *time.Time
			if tt.name == "ReadyWithNextSync" {
				nextSyncTime = &nextSync
			}
			cond, err := BuildRepositoryCondition(repo, tt.status, tt.errorMsg, nextSyncTime)

			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, cond.Status)
				assert.Equal(t, tt.reason, cond.Reason)
				assert.Equal(t, tt.message, cond.Message)
				assert.Equal(t, repo.Generation, cond.ObservedGeneration)
				assert.False(t, cond.LastTransitionTime.IsZero())
			}
		})
	}
}

func TestApplyRepositoryCondition(t *testing.T) {
	ctx := context.Background()
	repo := &configapi.Repository{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-repo",
			Namespace: "default",
		},
	}

	condition := metav1.Condition{
		Type:               configapi.RepositoryReady,
		Status:             metav1.ConditionTrue,
		ObservedGeneration: 1,
		LastTransitionTime: metav1.Now(),
		Reason:             configapi.ReasonReady,
		Message:            "Repository Ready",
	}

	scheme := runtime.NewScheme()
	require.NoError(t, configapi.AddToScheme(scheme))

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(repo).
		WithObjects(repo).
		Build()

	err := ApplyRepositoryCondition(ctx, fakeClient, repo, condition, "ready")
	require.NoError(t, err)

	// Fetch updated repo to verify condition
	updatedRepo := &configapi.Repository{}
	err = fakeClient.Get(ctx, types.NamespacedName{
		Namespace: repo.Namespace,
		Name:      repo.Name,
	}, updatedRepo)
	require.NoError(t, err)

	assert.Len(t, updatedRepo.Status.Conditions, 1)
	assert.Equal(t, condition.Type, updatedRepo.Status.Conditions[0].Type)
	assert.Equal(t, condition.Status, updatedRepo.Status.Conditions[0].Status)
}
