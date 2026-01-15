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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	api "github.com/nephio-project/porch/controllers/repositories/api/v1alpha1"
	mockclient "github.com/nephio-project/porch/test/mockery/mocks/external/sigs.k8s.io/controller-runtime/pkg/client"
	cachetypes "github.com/nephio-project/porch/test/mockery/mocks/porch/pkg/cache/types"
	mockRepo "github.com/nephio-project/porch/test/mockery/mocks/porch/pkg/repository"
)

func TestIsSyncInProgress(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name     string
		repo     *api.Repository
		expected bool
	}{
		{
			name: "sync in progress",
			repo: &api.Repository{
				Status: api.RepositoryStatus{
					Conditions: []metav1.Condition{{
						Type:               api.RepositoryReady,
						Status:             metav1.ConditionFalse,
						Reason:             api.ReasonReconciling,
						LastTransitionTime: metav1.NewTime(now.Add(-5 * time.Minute)),
					}},
				},
			},
			expected: true,
		},
		{
			name: "sync stale - over 20 minutes",
			repo: &api.Repository{
				ObjectMeta: metav1.ObjectMeta{Name: "stale-repo"},
				Status: api.RepositoryStatus{
					Conditions: []metav1.Condition{{
						Type:               api.RepositoryReady,
						Status:             metav1.ConditionFalse,
						Reason:             api.ReasonReconciling,
						LastTransitionTime: metav1.NewTime(now.Add(-25 * time.Minute)),
					}},
				},
			},
			expected: false, // Stale sync is treated as not in progress
		},
		{
			name: "sync not in progress - ready",
			repo: &api.Repository{
				Status: api.RepositoryStatus{
					Conditions: []metav1.Condition{{
						Type:   api.RepositoryReady,
						Status: metav1.ConditionTrue,
					}},
				},
			},
			expected: false,
		},
		{
			name: "sync not in progress - failed",
			repo: &api.Repository{
				Status: api.RepositoryStatus{
					Conditions: []metav1.Condition{{
						Type:   api.RepositoryReady,
						Status: metav1.ConditionFalse,
						Reason: "Failed",
					}},
				},
			},
			expected: false,
		},
		{
			name: "no conditions",
			repo: &api.Repository{
				Status: api.RepositoryStatus{},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &RepositoryReconciler{}
			result := r.isSyncInProgress(context.Background(), tt.repo)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCalculateNextSyncInterval(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name     string
		repo     *api.Repository
		expected time.Duration
	}{
		{
			name: "returns health check interval when sooner",
			repo: &api.Repository{
				Status: api.RepositoryStatus{
					Conditions: []metav1.Condition{{
						Type:               api.RepositoryReady,
						Status:             metav1.ConditionTrue,
						LastTransitionTime: metav1.NewTime(now.Add(-2 * time.Minute)),
					}},
				},
			},
			expected: 3 * time.Minute, // 5m health check - 2m elapsed
		},
		{
			name: "returns full sync interval when sooner",
			repo: &api.Repository{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"config.porch.kpt.dev/last-full-sync": now.Add(-55 * time.Minute).Format(time.RFC3339),
					},
				},
				Status: api.RepositoryStatus{
					Conditions: []metav1.Condition{{
						Type:               api.RepositoryReady,
						Status:             metav1.ConditionTrue,
						LastTransitionTime: metav1.NewTime(now.Add(-1 * time.Minute)),
					}},
				},
			},
			expected: 4 * time.Minute, // Health check in 4m, full sync in 5m
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &RepositoryReconciler{
				HealthCheckFrequency: 5 * time.Minute,
				FullSyncFrequency:    1 * time.Hour,
			}
			result := r.calculateNextSyncInterval(tt.repo)
			assert.InDelta(t, tt.expected.Seconds(), result.Seconds(), 2.0)
		})
	}
}

func TestIsFullSyncDue(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name     string
		repo     *api.Repository
		expected bool
	}{
		{
			name: "never synced - no annotation",
			repo: &api.Repository{
				Status: api.RepositoryStatus{},
			},
			expected: true,
		},
		{
			name: "full sync due - annotation shows old sync",
			repo: &api.Repository{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"config.porch.kpt.dev/last-full-sync": now.Add(-2 * time.Hour).Format(time.RFC3339),
					},
				},
			},
			expected: true,
		},
		{
			name: "full sync not due - recent annotation",
			repo: &api.Repository{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"config.porch.kpt.dev/last-full-sync": now.Add(-30 * time.Minute).Format(time.RFC3339),
					},
				},
			},
			expected: false,
		},
		{
			name: "blocked by error condition",
			repo: &api.Repository{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"config.porch.kpt.dev/last-full-sync": now.Add(-2 * time.Hour).Format(time.RFC3339),
					},
				},
				Status: api.RepositoryStatus{
					Conditions: []metav1.Condition{{
						Type:   api.RepositoryReady,
						Status: metav1.ConditionFalse,
						Reason: api.ReasonError,
					}},
				},
			},
			expected: false,
		},
		{
			name: "blocked by pending RunOnceAt",
			repo: &api.Repository{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"config.porch.kpt.dev/last-full-sync": now.Add(-2 * time.Hour).Format(time.RFC3339),
					},
				},
				Spec: api.RepositorySpec{
					Sync: &api.RepositorySync{
						RunOnceAt: &metav1.Time{Time: now.Add(time.Hour)},
					},
				},
			},
			expected: false,
		},
		{
			name: "custom schedule - sync due",
			repo: &api.Repository{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"config.porch.kpt.dev/last-full-sync": now.Add(-2 * time.Minute).Format(time.RFC3339),
					},
				},
				Spec: api.RepositorySpec{
					Sync: &api.RepositorySync{
						Schedule: "*/1 * * * *", // Every minute
					},
				},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &RepositoryReconciler{
				FullSyncFrequency: 1 * time.Hour,
			}
			result := r.isFullSyncDue(tt.repo)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetLastSyncTime(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name     string
		repo     *api.Repository
		expected time.Time
	}{
		{
			name: "has ready condition",
			repo: &api.Repository{
				Status: api.RepositoryStatus{
					Conditions: []metav1.Condition{{
						Type:               api.RepositoryReady,
						Status:             metav1.ConditionTrue,
						LastTransitionTime: metav1.NewTime(now),
					}},
				},
			},
			expected: now,
		},
		{
			name: "has ready condition false",
			repo: &api.Repository{
				Status: api.RepositoryStatus{
					Conditions: []metav1.Condition{{
						Type:               api.RepositoryReady,
						Status:             metav1.ConditionFalse,
						LastTransitionTime: metav1.NewTime(now),
					}},
				},
			},
			expected: time.Time{}, // Only true conditions count
		},
		{
			name: "no ready condition",
			repo: &api.Repository{
				Status: api.RepositoryStatus{
					Conditions: []metav1.Condition{{
						Type:   "Other",
						Status: metav1.ConditionTrue,
					}},
				},
			},
			expected: time.Time{},
		},
		{
			name: "no conditions",
			repo: &api.Repository{
				Status: api.RepositoryStatus{},
			},
			expected: time.Time{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &RepositoryReconciler{}
			result := r.getLastSyncTime(tt.repo)
			assert.Equal(t, tt.expected.Unix(), result.Unix())
		})
	}
}

func TestIsOneTimeSyncDue(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name     string
		repo     *api.Repository
		expected bool
	}{
		{
			name: "one-time sync due",
			repo: &api.Repository{
				Spec: api.RepositorySpec{
					Sync: &api.RepositorySync{
						RunOnceAt: &metav1.Time{Time: now.Add(-time.Hour)},
					},
				},
			},
			expected: true,
		},
		{
			name: "one-time sync not due",
			repo: &api.Repository{
				Spec: api.RepositorySpec{
					Sync: &api.RepositorySync{
						RunOnceAt: &metav1.Time{Time: now.Add(time.Hour)},
					},
				},
			},
			expected: false,
		},
		{
			name: "no one-time sync",
			repo: &api.Repository{
				Spec: api.RepositorySpec{
					Sync: &api.RepositorySync{},
				},
			},
			expected: false,
		},
		{
			name: "nil sync spec",
			repo: &api.Repository{
				Spec: api.RepositorySpec{
					Sync: nil,
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &RepositoryReconciler{}
			result := r.isOneTimeSyncDue(tt.repo)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDetermineRetryInterval(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected time.Duration
	}{
		{
			name:     "network error - no such host",
			err:      errors.New("no such host"),
			expected: 30 * time.Second,
		},
		{
			name:     "network error - connection refused",
			err:      errors.New("connection refused"),
			expected: 30 * time.Second,
		},
		{
			name:     "auth error - authentication",
			err:      errors.New("authentication failed"),
			expected: 10 * time.Minute,
		},
		{
			name:     "auth error - permission denied",
			err:      errors.New("permission denied"),
			expected: 10 * time.Minute,
		},
		{
			name:     "auth error - failed to resolve credentials",
			err:      errors.New("failed to resolve credentials"),
			expected: 10 * time.Minute,
		},
		{
			name:     "auth error - resolved credentials are invalid",
			err:      errors.New("resolved credentials are invalid"),
			expected: 10 * time.Minute,
		},
		{
			name:     "repo error - not found",
			err:      errors.New("repository not found"),
			expected: 2 * time.Minute,
		},
		{
			name:     "repo error - invalid",
			err:      errors.New("invalid repository"),
			expected: 2 * time.Minute,
		},
		{
			name:     "repo error - branch",
			err:      errors.New("branch not found"),
			expected: 2 * time.Minute,
		},
		{
			name:     "timeout error",
			err:      errors.New("timeout occurred"),
			expected: 1 * time.Minute,
		},
		{
			name:     "deadline error",
			err:      errors.New("deadline exceeded"),
			expected: 1 * time.Minute,
		},
		{
			name:     "tls error - certificate",
			err:      errors.New("certificate error"),
			expected: 5 * time.Minute,
		},
		{
			name:     "tls error - tls",
			err:      errors.New("tls handshake failed"),
			expected: 5 * time.Minute,
		},
		{
			name:     "tls error - ssl",
			err:      errors.New("ssl error"),
			expected: 5 * time.Minute,
		},
		{
			name:     "rate limit error",
			err:      errors.New("rate limit exceeded"),
			expected: 5 * time.Minute,
		},
		{
			name:     "rate limit error - too many requests",
			err:      errors.New("too many requests"),
			expected: 5 * time.Minute,
		},
		{
			name:     "unknown error",
			err:      errors.New("unknown error"),
			expected: 30 * time.Second, // Default retry interval
		},
		{
			name:     "case insensitive matching",
			err:      errors.New("NO SUCH HOST"),
			expected: 30 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &RepositoryReconciler{}
			result := r.determineRetryInterval(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestClearOneTimeSyncFlag(t *testing.T) {
	ctx := context.Background()
	now := metav1.NewTime(time.Now())

	tests := []struct {
		name        string
		repo        *api.Repository
		patchError  error
		expectPatch bool
		expectError bool
	}{
		{
			name: "clears one-time sync flag",
			repo: &api.Repository{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-repo",
					Namespace: "test-ns",
				},
				Spec: api.RepositorySpec{
					Sync: &api.RepositorySync{
						RunOnceAt: &now,
					},
				},
			},
			expectPatch: true,
		},
		{
			name: "no one-time sync flag",
			repo: &api.Repository{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-repo",
					Namespace: "test-ns",
				},
				Spec: api.RepositorySpec{
					Sync: &api.RepositorySync{},
				},
			},
			expectPatch: false,
		},
		{
			name: "nil sync spec",
			repo: &api.Repository{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-repo",
					Namespace: "test-ns",
				},
				Spec: api.RepositorySpec{
					Sync: nil,
				},
			},
			expectPatch: false,
		},
		{
			name: "patch fails",
			repo: &api.Repository{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-repo",
					Namespace: "test-ns",
				},
				Spec: api.RepositorySpec{
					Sync: &api.RepositorySync{
						RunOnceAt: &now,
					},
				},
			},
			patchError:  errors.New("patch failed"),
			expectPatch: true,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := mockclient.NewMockClient(t)

			if tt.expectPatch {
				mockClient.EXPECT().Patch(ctx, mock.Anything, mock.Anything).Return(tt.patchError)
			}

			r := &RepositoryReconciler{
				Client: mockClient,
			}

			err := r.clearOneTimeSyncFlag(ctx, tt.repo)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSyncRepositoryDetailed(t *testing.T) {
	ctx := context.Background()
	repo := &api.Repository{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-repo",
			Namespace: "test-ns",
		},
	}

	tests := []struct {
		name         string
		openError    error
		refreshError error
		listError    error
		expectError  bool
	}{
		{
			name: "successful sync",
		},
		{
			name:        "open repository fails",
			openError:   errors.New("failed to open repository"),
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

			r := &RepositoryReconciler{
				Cache: mockCache,
			}

			err := r.syncRepository(ctx, repo)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestIsHealthCheckDue(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name     string
		repo     *api.Repository
		expected bool
	}{
		{
			name: "never checked",
			repo: &api.Repository{
				Status: api.RepositoryStatus{},
			},
			expected: true,
		},
		{
			name: "health check due",
			repo: &api.Repository{
				Status: api.RepositoryStatus{
					Conditions: []metav1.Condition{{
						Type:               api.RepositoryReady,
						Status:             metav1.ConditionTrue,
						LastTransitionTime: metav1.NewTime(now.Add(-6 * time.Minute)),
					}},
				},
			},
			expected: true,
		},
		{
			name: "health check not due",
			repo: &api.Repository{
				Status: api.RepositoryStatus{
					Conditions: []metav1.Condition{{
						Type:               api.RepositoryReady,
						Status:             metav1.ConditionTrue,
						LastTransitionTime: metav1.NewTime(now.Add(-2 * time.Minute)),
					}},
				},
			},
			expected: false,
		},
		{
			name: "blocked by error condition",
			repo: &api.Repository{
				Status: api.RepositoryStatus{
					Conditions: []metav1.Condition{{
						Type:               api.RepositoryReady,
						Status:             metav1.ConditionFalse,
						Reason:             api.ReasonError,
						LastTransitionTime: metav1.NewTime(now.Add(-10 * time.Minute)),
					}},
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &RepositoryReconciler{
				HealthCheckFrequency: 5 * time.Minute,
			}
			result := r.isHealthCheckDue(tt.repo)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetLastFullSyncTime(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name     string
		repo     *api.Repository
		expected time.Time
	}{
		{
			name: "has annotation",
			repo: &api.Repository{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"config.porch.kpt.dev/last-full-sync": now.Format(time.RFC3339),
					},
				},
			},
			expected: now,
		},
		{
			name: "no annotation - fallback to condition",
			repo: &api.Repository{
				Status: api.RepositoryStatus{
					Conditions: []metav1.Condition{{
						Type:               api.RepositoryReady,
						Status:             metav1.ConditionTrue,
						LastTransitionTime: metav1.NewTime(now),
					}},
				},
			},
			expected: now,
		},
		{
			name: "invalid annotation - fallback to condition",
			repo: &api.Repository{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"config.porch.kpt.dev/last-full-sync": "invalid",
					},
				},
				Status: api.RepositoryStatus{
					Conditions: []metav1.Condition{{
						Type:               api.RepositoryReady,
						Status:             metav1.ConditionTrue,
						LastTransitionTime: metav1.NewTime(now),
					}},
				},
			},
			expected: now,
		},
		{
			name: "no annotation or condition",
			repo: &api.Repository{
				Status: api.RepositoryStatus{},
			},
			expected: time.Time{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &RepositoryReconciler{}
			result := r.getLastFullSyncTime(tt.repo)
			assert.Equal(t, tt.expected.Unix(), result.Unix())
		})
	}
}

func TestGetLastStatusUpdateTime(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name     string
		repo     *api.Repository
		expected time.Time
	}{
		{
			name: "has ready condition",
			repo: &api.Repository{
				Status: api.RepositoryStatus{
					Conditions: []metav1.Condition{{
						Type:               api.RepositoryReady,
						Status:             metav1.ConditionTrue,
						LastTransitionTime: metav1.NewTime(now),
					}},
				},
			},
			expected: now,
		},
		{
			name: "has ready condition false",
			repo: &api.Repository{
				Status: api.RepositoryStatus{
					Conditions: []metav1.Condition{{
						Type:               api.RepositoryReady,
						Status:             metav1.ConditionFalse,
						LastTransitionTime: metav1.NewTime(now),
					}},
				},
			},
			expected: now,
		},
		{
			name: "no conditions",
			repo: &api.Repository{
				Status: api.RepositoryStatus{},
			},
			expected: time.Time{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &RepositoryReconciler{}
			result := r.getLastStatusUpdateTime(tt.repo)
			assert.Equal(t, tt.expected.Unix(), result.Unix())
		})
	}
}

func TestDetermineSyncDecision(t *testing.T) {
	now := time.Now()
	ctx := context.Background()

	tests := []struct {
		name         string
		repo         *api.Repository
		expectedType SyncType
		expectedNeed bool
	}{
		{
			name: "sync in progress",
			repo: &api.Repository{
				Status: api.RepositoryStatus{
					Conditions: []metav1.Condition{{
						Type:               api.RepositoryReady,
						Status:             metav1.ConditionFalse,
						Reason:             api.ReasonReconciling,
						LastTransitionTime: metav1.NewTime(now.Add(-5 * time.Minute)),
					}},
				},
			},
			expectedType: HealthCheck,
			expectedNeed: false,
		},
		{
			name: "RunOnceAt due",
			repo: &api.Repository{
				Spec: api.RepositorySpec{
					Sync: &api.RepositorySync{
						RunOnceAt: &metav1.Time{Time: now.Add(-time.Hour)},
					},
				},
			},
			expectedType: FullSync,
			expectedNeed: true,
		},
		{
			name: "spec changed",
			repo: &api.Repository{
				ObjectMeta: metav1.ObjectMeta{
					Generation: 2,
				},
				Status: api.RepositoryStatus{
					Conditions: []metav1.Condition{{
						Type:               api.RepositoryReady,
						Status:             metav1.ConditionTrue,
						ObservedGeneration: 1,
					}},
				},
			},
			expectedType: FullSync,
			expectedNeed: true,
		},
		{
			name: "error retry due",
			repo: &api.Repository{
				Status: api.RepositoryStatus{
					Conditions: []metav1.Condition{{
						Type:               api.RepositoryReady,
						Status:             metav1.ConditionFalse,
						Reason:             api.ReasonError,
						Message:            "error (next retry at: " + now.Add(-time.Minute).Format(time.RFC3339) + ")",
						LastTransitionTime: metav1.NewTime(now.Add(-10 * time.Minute)),
					}},
				},
			},
			expectedType: FullSync,
			expectedNeed: true,
		},
		{
			name: "full sync due",
			repo: &api.Repository{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"config.porch.kpt.dev/last-full-sync": now.Add(-2 * time.Hour).Format(time.RFC3339),
					},
				},
				Status: api.RepositoryStatus{
					Conditions: []metav1.Condition{{
						Type:               api.RepositoryReady,
						Status:             metav1.ConditionTrue,
						LastTransitionTime: metav1.NewTime(now.Add(-2 * time.Minute)),
					}},
				},
			},
			expectedType: FullSync,
			expectedNeed: true,
		},
		{
			name: "health check due",
			repo: &api.Repository{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"config.porch.kpt.dev/last-full-sync": now.Add(-30 * time.Minute).Format(time.RFC3339),
					},
				},
				Status: api.RepositoryStatus{
					Conditions: []metav1.Condition{{
						Type:               api.RepositoryReady,
						Status:             metav1.ConditionTrue,
						LastTransitionTime: metav1.NewTime(now.Add(-6 * time.Minute)),
					}},
				},
			},
			expectedType: HealthCheck,
			expectedNeed: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &RepositoryReconciler{
				HealthCheckFrequency: 5 * time.Minute,
				FullSyncFrequency:    1 * time.Hour,
			}
			decision := r.determineSyncDecision(ctx, tt.repo)
			assert.Equal(t, tt.expectedType, decision.Type)
			assert.Equal(t, tt.expectedNeed, decision.Needed)
		})
	}
}
