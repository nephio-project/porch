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
	"testing"
	"time"

	configapi "github.com/nephio-project/porch/api/porchconfig/v1alpha1"
	"github.com/nephio-project/porch/pkg/cache/testutil"
	"github.com/nephio-project/porch/pkg/cache/util"
	"github.com/nephio-project/porch/pkg/repository"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
)

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
		status         util.RepositoryStatus
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
			status:         util.RepositoryStatusReady,
			expectedStatus: metav1.ConditionTrue,
			expectedReason: configapi.ReasonReady,
			expectedMsg:    "Repository Ready",
		},
		{
			name:           "ready with next sync time",
			status:         util.RepositoryStatusReady,
			nextSyncTime:   &time.Time{},
			expectedStatus: metav1.ConditionTrue,
			expectedReason: configapi.ReasonReady,
		},
		{
			name:           "error with message",
			status:         util.RepositoryStatusError,
			syncError:      errors.New("sync failed"),
			expectedStatus: metav1.ConditionFalse,
			expectedReason: configapi.ReasonError,
			expectedMsg:    "sync failed",
		},
		{
			name:           "error without message",
			status:         util.RepositoryStatusError,
			expectedStatus: metav1.ConditionFalse,
			expectedReason: configapi.ReasonError,
			expectedMsg:    "unknown error",
		},
		{
			name:           "sync-in-progress",
			status:         util.RepositoryStatusSyncInProgress,
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
