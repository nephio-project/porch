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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	configapi "github.com/nephio-project/porch/controllers/repositories/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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


