// Copyright 2025-2026 The kpt and Nephio Authors
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
	"testing"
	"time"

	configapi "github.com/nephio-project/porch/controllers/repositories/api/v1alpha1"
	"github.com/nephio-project/porch/pkg/repository"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type mockSyncHandler struct {
	syncCount int
	syncError error
	repoKey   repository.RepositoryKey
	spec      *configapi.Repository
}

func (m *mockSyncHandler) SyncOnce(ctx context.Context) error {
	m.syncCount++
	return m.syncError
}

func (m *mockSyncHandler) Key() repository.RepositoryKey {
	return m.repoKey
}

func (m *mockSyncHandler) GetSpec() *configapi.Repository {
	return m.spec
}

func TestSyncManager_SyncOnce(t *testing.T) {
	handler := &mockSyncHandler{
		repoKey: repository.RepositoryKey{Name: "test-repo", Namespace: "test-ns"},
		spec: &configapi.Repository{
			Spec: configapi.RepositorySpec{
				Sync: &configapi.RepositorySync{
					Schedule: "*/5 * * * *",
				},
			},
		},
	}

	manager := NewSyncManager(handler, nil)

	// Test that sync is called
	err := manager.handler.SyncOnce(context.Background())
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if handler.syncCount != 1 {
		t.Errorf("Expected sync count to be 1, got %d", handler.syncCount)
	}
}

func TestSyncManager_CalculateWaitDuration(t *testing.T) {
	tests := []struct {
		name           string
		spec           *configapi.Repository
		expectDefault  bool
		expectNextSync bool
	}{
		{
			name: "valid cron schedule",
			spec: &configapi.Repository{
				Spec: configapi.RepositorySpec{
					Sync: &configapi.RepositorySync{
						Schedule: "0 0 * * *",
					},
				},
			},
			expectDefault:  false,
			expectNextSync: true,
		},
		{
			name:           "nil spec fallback",
			spec:           nil,
			expectDefault:  true,
			expectNextSync: true,
		},
		{
			name: "empty schedule fallback",
			spec: &configapi.Repository{
				Spec: configapi.RepositorySpec{
					Sync: &configapi.RepositorySync{
						Schedule: "",
					},
				},
			},
			expectDefault:  true,
			expectNextSync: true,
		},
		{
			name: "invalid cron expression fallback",
			spec: &configapi.Repository{
				Spec: configapi.RepositorySpec{
					Sync: &configapi.RepositorySync{
						Schedule: "invalid-cron",
					},
				},
			},
			expectDefault:  true,
			expectNextSync: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := &mockSyncHandler{
				repoKey: repository.RepositoryKey{Name: "test-repo", Namespace: "test-ns"},
				spec:    tt.spec,
			}

			manager := NewSyncManager(handler, nil)
			defaultDuration := 5 * time.Minute

			duration := manager.calculateWaitDuration(defaultDuration)

			if tt.expectDefault {
				assert.Equal(t, defaultDuration, duration)
			} else {
				assert.Less(t, duration, 24*time.Hour)
				assert.Greater(t, duration, time.Duration(0))
			}

			if tt.expectNextSync {
				assert.NotNil(t, manager.nextSyncTime)
			}
		})
	}
}

func TestSyncManager_ShouldScheduleRunOnce(t *testing.T) {
	handler := &mockSyncHandler{}
	manager := NewSyncManager(handler, nil)

	now := time.Now()
	future := metav1.Time{Time: now.Add(1 * time.Hour)}

	// Should schedule if runOnceAt is set and scheduled is zero
	if !manager.shouldScheduleRunOnce(&future, time.Time{}) {
		t.Error("Expected to schedule run once when scheduled time is zero")
	}

	// Should not schedule if times are equal
	if manager.shouldScheduleRunOnce(&future, future.Time) {
		t.Error("Expected not to schedule run once when times are equal")
	}

	// Should not schedule if runOnceAt is nil
	if manager.shouldScheduleRunOnce(nil, time.Time{}) {
		t.Error("Expected not to schedule run once when runOnceAt is nil")
	}
}

func TestSyncManager_SetDefaultNextSyncTime(t *testing.T) {
	handler := &mockSyncHandler{
		repoKey: repository.RepositoryKey{Name: "test-repo", Namespace: "test-ns"},
	}
	manager := NewSyncManager(handler, nil)

	defaultDuration := 10 * time.Minute
	before := time.Now()

	duration := manager.setDefaultNextSyncTime(defaultDuration)

	after := time.Now()

	assert.Equal(t, defaultDuration, duration)

	require.NotNil(t, manager.nextSyncTime)

	// Check that nextSyncTime is approximately now + defaultDuration
	expectedTime := before.Add(defaultDuration)
	if manager.nextSyncTime.Before(expectedTime) || manager.nextSyncTime.After(after.Add(defaultDuration)) {
		t.Errorf("Expected nextSyncTime to be around %v, got %v", expectedTime, *manager.nextSyncTime)
	}
}

func TestSyncManager_Stop(t *testing.T) {
	handler := &mockSyncHandler{}
	manager := NewSyncManager(handler, nil)

	// Test stop with valid cancel function
	manager.Stop()

	// Test stop with nil cancel (should not panic)
	manager.cancel = nil
	manager.Stop()
}

func TestSyncManager_GetLastSyncError(t *testing.T) {
	handler := &mockSyncHandler{}
	manager := NewSyncManager(handler, nil)

	// Initially no error
	if err := manager.GetLastSyncError(); err != nil {
		t.Errorf("Expected no error initially, got %v", err)
	}

	// Set an error
	testErr := context.DeadlineExceeded
	manager.lastSyncError = testErr
	if err := manager.GetLastSyncError(); err != testErr {
		t.Errorf("Expected error %v, got %v", testErr, err)
	}
}

func TestSyncManager_UpdateRepositoryCondition(t *testing.T) {
	handler := &mockSyncHandler{
		repoKey: repository.RepositoryKey{Name: "test-repo", Namespace: "test-ns"},
	}
	manager := NewSyncManager(handler, nil)

	// Test with no error - should not panic even with nil client
	// The method will log a warning but should not crash
	manager.updateRepositoryCondition(context.Background())

	// Test with error - should not panic even with nil client
	manager.lastSyncError = context.DeadlineExceeded
	manager.updateRepositoryCondition(context.Background())

	// Verify the error was set
	if manager.lastSyncError != context.DeadlineExceeded {
		t.Errorf("Expected error to be preserved, got %v", manager.lastSyncError)
	}
}

func TestSyncManager_SyncForever(t *testing.T) {
	t.Run("default duration countdown", func(t *testing.T) {
		handler := &mockSyncHandler{
			repoKey: repository.RepositoryKey{Name: "test-repo", Namespace: "test-ns"},
			// Use nil spec to force fallback to default duration
			spec: nil,
		}
		manager := NewSyncManager(handler, nil)

		// Test with short frequency to trigger countdown sync
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		go manager.syncForever(ctx, 1*time.Second)
		<-ctx.Done()

		// Should have multiple syncs due to countdown (startup + at least one more)
		if handler.syncCount < 2 {
			t.Errorf("Expected at least 2 sync calls, got %d", handler.syncCount)
		}
	})

	t.Run("cron expression change detection", func(t *testing.T) {
		handler := &mockSyncHandler{
			repoKey: repository.RepositoryKey{Name: "test-repo", Namespace: "test-ns"},
			spec: &configapi.Repository{
				Spec: configapi.RepositorySpec{
					Sync: &configapi.RepositorySync{
						Schedule: "*/1 * * * *",
					},
				},
			},
		}
		manager := NewSyncManager(handler, nil)

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		// Change cron expression after startup to trigger change detection
		go func() {
			time.Sleep(1 * time.Second)
			handler.spec.Spec.Sync.Schedule = "*/2 * * * *"
		}()

		manager.syncForever(ctx, 1*time.Second)

		// Should have at least startup sync
		if handler.syncCount < 1 {
			t.Errorf("Expected at least 1 sync call, got %d", handler.syncCount)
		}
	})
}

func TestSyncManager_HandleRunOnceAt(t *testing.T) {
	t.Run("runOnceAt execution", func(t *testing.T) {
		// Set runOnceAt time to be 6 seconds in the future to ensure the 5-second ticker catches it
		futureTime := metav1.NewTime(time.Now().Add(6 * time.Second))
		handler := &mockSyncHandler{
			repoKey: repository.RepositoryKey{Name: "test-repo", Namespace: "test-ns"},
			spec: &configapi.Repository{
				Spec: configapi.RepositorySpec{
					Sync: &configapi.RepositorySync{
						RunOnceAt: &futureTime,
					},
				},
			},
		}
		manager := NewSyncManager(handler, nil)

		// Allow enough time for the ticker to detect and execute the runOnceAt
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()

		go manager.handleRunOnceAt(ctx)
		<-ctx.Done()

		// Should have executed the runOnceAt sync
		if handler.syncCount < 1 {
			t.Errorf("Expected at least 1 sync call from runOnceAt, got %d", handler.syncCount)
		}
	})
}

func TestSyncManager_Start(t *testing.T) {
	handler := &mockSyncHandler{
		repoKey: repository.RepositoryKey{Name: "test-repo", Namespace: "test-ns"},
	}
	manager := NewSyncManager(handler, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Start should launch both goroutines
	manager.Start(ctx, 1*time.Second)

	// Wait for goroutines to run
	<-ctx.Done()

	// Should have at least one sync from startup
	if handler.syncCount < 2 {
		t.Errorf("Expected at least 1 sync call, got %d", handler.syncCount)
	}
}

func TestSyncManager_HasValidSyncSpec(t *testing.T) {
	tests := []struct {
		name     string
		spec     *configapi.Repository
		expected bool
	}{
		{
			name:     "nil spec",
			spec:     nil,
			expected: false,
		},
		{
			name:     "spec without sync",
			spec:     &configapi.Repository{},
			expected: false,
		},
		{
			name: "valid sync spec",
			spec: &configapi.Repository{
				Spec: configapi.RepositorySpec{
					Sync: &configapi.RepositorySync{},
				},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := &mockSyncHandler{spec: tt.spec}
			manager := NewSyncManager(handler, nil)
			if got := manager.hasValidSyncSpec(); got != tt.expected {
				t.Errorf("hasValidSyncSpec() = %v, want %v", got, tt.expected)
			}
		})
	}
}
