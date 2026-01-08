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
	"fmt"
	"testing"
	"time"

	configapi "github.com/nephio-project/porch/controllers/repositories/api/v1alpha1"
	"github.com/nephio-project/porch/pkg/repository"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestNewSyncManager(t *testing.T) {
	handler := &testSyncHandler{}
	manager := NewSyncManager(handler, nil)

	if manager == nil {
		t.Error("Expected non-nil sync manager")
	}
	if manager.handler != handler {
		t.Error("Expected handler to be set")
	}
}

func TestNewSyncManagerWithEventRecorder(t *testing.T) {
	handler := &testSyncHandler{
		repoKey: repository.RepositoryKey{Name: "test-repo", Namespace: "test-ns"},
	}
	eventRecorder := &testEventRecorder{}

	manager := NewSyncManagerWithEventRecorder(handler, nil, eventRecorder)

	if manager == nil {
		t.Error("Expected non-nil sync manager")
	}
	if manager.eventRecorder != eventRecorder {
		t.Error("Expected event recorder to be set")
	}

	// Cleanup
	manager.Stop()
}

func TestSyncManager_EventRecorderIntegration(t *testing.T) {
	eventRecorder := &testEventRecorder{}
	handler := &testSyncHandler{
		repoKey: repository.RepositoryKey{Name: "test-repo", Namespace: "test-ns"},
		spec: &configapi.Repository{
			ObjectMeta: metav1.ObjectMeta{Name: "test-repo", Namespace: "test-ns"},
		},
	}
	manager := NewSyncManagerWithEventRecorder(handler, nil, eventRecorder)
	nextTime := time.Now().Add(5 * time.Minute)
	manager.nextSyncTime = &nextTime

	manager.updateRepositoryCondition(context.Background())

	if len(eventRecorder.events) != 1 {
		t.Errorf("Expected 1 event, got %d", len(eventRecorder.events))
	}

	event := eventRecorder.events[0]
	if event.eventType != "Normal" {
		t.Errorf("Expected Normal event, got %s", event.eventType)
	}
	if event.reason != "SyncCompleted" {
		t.Errorf("Expected SyncCompleted reason, got %s", event.reason)
	}

	// Cleanup
	manager.Stop()
}

func TestSyncManager_NotifySyncInProgress(t *testing.T) {
	eventRecorder := &testEventRecorder{}
	handler := &testSyncHandler{
		repoKey: repository.RepositoryKey{Name: "test-repo", Namespace: "test-ns"},
		spec: &configapi.Repository{
			ObjectMeta: metav1.ObjectMeta{Name: "test-repo", Namespace: "test-ns"},
		},
	}
	manager := NewSyncManagerWithEventRecorder(handler, nil, eventRecorder)

	manager.NotifySyncInProgress(context.Background())

	if len(eventRecorder.events) != 1 {
		t.Errorf("Expected 1 event, got %d", len(eventRecorder.events))
	}

	event := eventRecorder.events[0]
	if event.reason != "SyncStarted" {
		t.Errorf("Expected SyncStarted reason, got %s", event.reason)
	}

	// Cleanup
	manager.Stop()
}

func TestSyncManager_ContextCancellation(t *testing.T) {
	handler := &testSyncHandler{
		repoKey: repository.RepositoryKey{Name: "test-repo", Namespace: "test-ns"},
	}
	manager := NewSyncManager(handler, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	manager.Start(ctx, 1*time.Second)
	<-ctx.Done()

	// Give goroutines time to exit
	time.Sleep(50 * time.Millisecond)

	if handler.syncCount < 1 {
		t.Errorf("Expected at least 1 sync, got %d", handler.syncCount)
	}
}

func TestSyncManager_Stop(t *testing.T) {
	handler := &testSyncHandler{}
	manager := NewSyncManager(handler, nil)

	// Test stop with valid cancel function
	manager.Stop()

	// Test stop with nil cancel (should not panic)
	manager.cancel = nil
	manager.Stop()
}

func TestSyncManager_GetLastSyncError(t *testing.T) {
	handler := &testSyncHandler{}
	manager := NewSyncManager(handler, nil)

	// Initially no error
	if err := manager.GetLastSyncError(); err != nil {
		t.Errorf("Expected no error initially, got %v", err)
	}

	// Set an error
	testErr := fmt.Errorf("test error")
	manager.lastSyncError = testErr
	if err := manager.GetLastSyncError(); err != testErr {
		t.Errorf("Expected error %v, got %v", testErr, err)
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
					Sync: &configapi.CacheSync{},
				},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := &testSyncHandler{spec: tt.spec}
			manager := NewSyncManager(handler, nil)
			if got := manager.hasValidSyncSpec(); got != tt.expected {
				t.Errorf("hasValidSyncSpec() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestGlobalDeletionWatcher_RegisterUnregister(t *testing.T) {
	// Reset global state
	globalDeletionWatcher.mu.Lock()
	globalDeletionWatcher.syncManagers = make(map[string]*SyncManager)
	globalDeletionWatcher.started = false
	globalDeletionWatcher.mu.Unlock()

	handler := &testSyncHandler{
		repoKey: repository.RepositoryKey{Name: "test-repo", Namespace: "test-ns"},
	}
	manager := NewSyncManager(handler, nil)

	// Test registration
	RegisterSyncManager(handler.Key(), manager)

	globalDeletionWatcher.mu.RLock()
	registeredManager, exists := globalDeletionWatcher.syncManagers[handler.Key().String()]
	globalDeletionWatcher.mu.RUnlock()

	if !exists {
		t.Error("Expected sync manager to be registered")
	}
	if registeredManager != manager {
		t.Error("Expected registered manager to match")
	}

	// Test unregistration
	UnregisterSyncManager(handler.Key())

	globalDeletionWatcher.mu.RLock()
	_, stillExists := globalDeletionWatcher.syncManagers[handler.Key().String()]
	globalDeletionWatcher.mu.RUnlock()

	if stillExists {
		t.Error("Expected sync manager to be unregistered")
	}
}

func TestGlobalDeletionWatcher_HandleRepositoryDeletion(t *testing.T) {
	// Reset global state
	globalDeletionWatcher.mu.Lock()
	globalDeletionWatcher.syncManagers = make(map[string]*SyncManager)
	globalDeletionWatcher.started = false
	globalDeletionWatcher.mu.Unlock()

	handler := &testSyncHandler{
		repoKey: repository.RepositoryKey{Name: "test-repo", Namespace: "test-ns"},
	}
	manager := NewSyncManager(handler, nil)
	
	// Track if Stop was called
	stopCalled := false
	originalCancel := manager.cancel
	manager.cancel = func() {
		stopCalled = true
		if originalCancel != nil {
			originalCancel()
		}
	}

	RegisterSyncManager(handler.Key(), manager)

	// Create repository for deletion
	repo := &configapi.Repository{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-repo",
			Namespace: "test-ns",
		},
	}

	// Handle deletion
	globalDeletionWatcher.handleRepositoryDeletion(repo)

	if !stopCalled {
		t.Error("Expected Stop to be called on sync manager when repository is deleted")
	}

	// Verify unregistration
	globalDeletionWatcher.mu.RLock()
	_, exists := globalDeletionWatcher.syncManagers[handler.Key().String()]
	globalDeletionWatcher.mu.RUnlock()
	if exists {
		t.Error("Expected sync manager to be unregistered after deletion")
	}
}