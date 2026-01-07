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
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	configapi "github.com/nephio-project/porch/controllers/repositories/api/v1alpha1"
	"github.com/nephio-project/porch/pkg/cache/testutil"
	"github.com/nephio-project/porch/pkg/repository"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
)

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
		status    RepositoryStatus
		errorMsg  string
		expected  metav1.ConditionStatus
		reason    string
		message   string
		expectErr bool
	}{
		{
			name:     "SyncInProgress",
			status:   RepositoryStatusSyncInProgress,
			errorMsg: "",
			expected: metav1.ConditionFalse,
			reason:   configapi.ReasonReconciling,
			message:  "Repository reconciliation in progress",
		},
		{
			name:     "Ready",
			status:   RepositoryStatusReady,
			errorMsg: "",
			expected: metav1.ConditionTrue,
			reason:   configapi.ReasonReady,
			message:  "Repository Ready",
		},
		{
			name:     "ErrorWithMessage",
			status:   RepositoryStatusError,
			errorMsg: "some error",
			expected: metav1.ConditionFalse,
			reason:   configapi.ReasonError,
			message:  "some error",
		},
		{
			name:     "ErrorWithoutMessage",
			status:   RepositoryStatusError,
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
			status:   RepositoryStatusReady,
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

	err := applyRepositoryCondition(ctx, fakeClient, repo, condition, RepositoryStatusReady)
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
		status         RepositoryStatus
		syncError      error
		nextSyncTime   *time.Time
		expectedStatus metav1.ConditionStatus
		expectedReason string
		expectedMsg    string
		expectErr      bool
		expectedErr    string
	}{
		{
			name:           "ready status",
			status:         RepositoryStatusReady,
			expectedStatus: metav1.ConditionTrue,
			expectedReason: configapi.ReasonReady,
			expectedMsg:    "Repository Ready",
		},
		{
			name:           "ready with next sync time",
			status:         RepositoryStatusReady,
			nextSyncTime:   &time.Time{},
			expectedStatus: metav1.ConditionTrue,
			expectedReason: configapi.ReasonReady,
		},
		{
			name:           "error with message",
			status:         RepositoryStatusError,
			syncError:      errors.New("sync failed"),
			expectedStatus: metav1.ConditionFalse,
			expectedReason: configapi.ReasonError,
			expectedMsg:    "sync failed",
		},
		{
			name:           "error without message",
			status:         RepositoryStatusError,
			expectedStatus: metav1.ConditionFalse,
			expectedReason: configapi.ReasonError,
			expectedMsg:    "unknown error",
		},
		{
			name:           "sync-in-progress",
			status:         RepositoryStatusSyncInProgress,
			expectedStatus: metav1.ConditionFalse,
			expectedReason: configapi.ReasonReconciling,
			expectedMsg:    "Repository reconciliation in progress",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewFakeClientWithStatus(scheme, repoObj.DeepCopy())

			err := SetRepositoryCondition(context.TODO(), client, repoKey, tt.status, tt.syncError, tt.nextSyncTime)

			if tt.expectErr {
				require.ErrorContains(t, err, tt.expectedErr)
				return
			}

			require.NoError(t, err)

			status, ok := client.GetStatusStore()[types.NamespacedName{Name: repoName, Namespace: namespace}]
			if !ok {
				t.Error("Expected status to be updated")
				return
			}

			if len(status.Conditions) != 1 {
				t.Errorf("Expected 1 condition, got %d", len(status.Conditions))
				return
			}

			cond := status.Conditions[0]
			assert.Equal(t, configapi.RepositoryReady, cond.Type)
			assert.Equal(t, tt.expectedStatus, cond.Status)
			assert.Equal(t, tt.expectedReason, cond.Reason)
			if tt.expectedMsg != "" {
				assert.Equal(t, tt.expectedMsg, cond.Message)
			}
		})
	}
}

