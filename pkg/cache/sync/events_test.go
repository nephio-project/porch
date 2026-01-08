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
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"sigs.k8s.io/controller-runtime/pkg/client"

	api "github.com/nephio-project/porch/controllers/repositories/api/v1alpha1"
)

// mockClientWithCreate tracks created events
type mockClientWithCreate struct {
	client.Client
	createdEvents []*corev1.Event
}

func (m *mockClientWithCreate) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	if event, ok := obj.(*corev1.Event); ok {
		m.createdEvents = append(m.createdEvents, event)
	}
	return nil
}

func (m *mockClientWithCreate) Watch(ctx context.Context, list client.ObjectList, opts ...client.ListOption) (watch.Interface, error) {
	return nil, nil
}

// mockClientWithList supports listing and deleting events
type mockClientWithList struct {
	client.Client
	events        []*corev1.Event
	deletedEvents []string
}

func (m *mockClientWithList) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	if eventList, ok := list.(*corev1.EventList); ok {
		eventList.Items = make([]corev1.Event, len(m.events))
		for i, event := range m.events {
			eventList.Items[i] = *event
		}
	}
	return nil
}

func (m *mockClientWithList) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	m.deletedEvents = append(m.deletedEvents, obj.GetName())
	return nil
}

func (m *mockClientWithList) Watch(ctx context.Context, list client.ObjectList, opts ...client.ListOption) (watch.Interface, error) {
	return nil, nil
}

func TestNewRepositoryEventRecorder(t *testing.T) {
	mockClient := &mockClientWithCreate{}
	recorder := NewRepositoryEventRecorder(mockClient)

	if recorder == nil {
		t.Error("Expected non-nil event recorder")
	}

	simpleRecorder, ok := recorder.(*SimpleEventRecorder)
	if !ok {
		t.Error("Expected SimpleEventRecorder type")
	}
	if simpleRecorder.coreClient == nil {
		t.Error("Expected client to be set")
	}
	if simpleRecorder.component != PorchSyncComponent {
		t.Errorf("Expected component %s, got %s", PorchSyncComponent, simpleRecorder.component)
	}
}

func TestSimpleEventRecorder_Event(t *testing.T) {
	mockClient := &mockClientWithCreate{
		createdEvents: make([]*corev1.Event, 0),
	}
	recorder := &SimpleEventRecorder{
		coreClient: mockClient,
		component:  PorchSyncComponent,
	}

	repo := &api.Repository{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-repo",
			Namespace: "test-ns",
			UID:       "test-uid",
		},
	}

	recorder.Event(repo, "Normal", "SyncCompleted", "Repository sync completed")

	if len(mockClient.createdEvents) != 1 {
		t.Errorf("Expected 1 event, got %d", len(mockClient.createdEvents))
	}

	event := mockClient.createdEvents[0]
	if event.Type != "Normal" {
		t.Errorf("Expected Normal type, got %s", event.Type)
	}
	if event.Reason != "SyncCompleted" {
		t.Errorf("Expected SyncCompleted reason, got %s", event.Reason)
	}
	if event.Message != "Repository sync completed" {
		t.Errorf("Expected specific message, got %s", event.Message)
	}
	if event.Source.Component != PorchSyncComponent {
		t.Errorf("Expected component %s, got %s", PorchSyncComponent, event.Source.Component)
	}
	if event.InvolvedObject.Name != "test-repo" {
		t.Errorf("Expected test-repo name, got %s", event.InvolvedObject.Name)
	}
}

func TestSimpleEventRecorder_AnnotatedEventf(t *testing.T) {
	mockClient := &mockClientWithCreate{
		createdEvents: make([]*corev1.Event, 0),
	}
	recorder := &SimpleEventRecorder{
		coreClient: mockClient,
		component:  PorchSyncComponent,
	}

	repo := &api.Repository{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-repo",
			Namespace: "test-ns",
		},
	}

	annotations := map[string]string{
		"porch.kpt.dev/next-sync-time": "2025-01-01T12:00:00Z",
	}

	recorder.AnnotatedEventf(repo, annotations, "Normal", "SyncCompleted", "Sync completed at %s", "12:00")

	if len(mockClient.createdEvents) != 1 {
		t.Errorf("Expected 1 event, got %d", len(mockClient.createdEvents))
	}

	event := mockClient.createdEvents[0]
	if event.Type != "Normal" {
		t.Errorf("Expected Normal type, got %s", event.Type)
	}
	if event.Reason != "SyncCompleted" {
		t.Errorf("Expected SyncCompleted reason, got %s", event.Reason)
	}
	if event.Message != "Sync completed at 12:00" {
		t.Errorf("Expected formatted message, got %s", event.Message)
	}
	if event.Annotations["porch.kpt.dev/next-sync-time"] != "2025-01-01T12:00:00Z" {
		t.Error("Expected annotation to be set")
	}
}

func TestSimpleEventRecorder_SyncStartedTimeOffset(t *testing.T) {
	mockClient := &mockClientWithCreate{
		createdEvents: make([]*corev1.Event, 0),
	}
	recorder := &SimpleEventRecorder{
		coreClient: mockClient,
		component:  PorchSyncComponent,
	}

	repo := &api.Repository{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-repo",
			Namespace: "test-ns",
		},
	}

	beforeTime := time.Now()
	recorder.Event(repo, "Normal", "SyncStarted", "Repository sync started")

	if len(mockClient.createdEvents) != 1 {
		t.Errorf("Expected 1 event, got %d", len(mockClient.createdEvents))
	}

	event := mockClient.createdEvents[0]
	
	// SyncStarted events should be offset by 1 second in the past
	if !event.FirstTimestamp.Time.Before(beforeTime) {
		t.Error("Expected SyncStarted event to be offset in the past")
	}
	if !event.FirstTimestamp.Time.After(beforeTime.Add(-2*time.Second)) {
		t.Error("Expected SyncStarted event to not be too far in the past")
	}
}

func createTestEvent(name, reason string, timestamp time.Time) *corev1.Event {
	return &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Source:     corev1.EventSource{Component: PorchSyncComponent},
		Reason:     reason,
		FirstTimestamp: metav1.NewTime(timestamp),
	}
}

func TestCleanupOldEvents(t *testing.T) {
	mockClient := &mockClientWithList{
		events: []*corev1.Event{
			createTestEvent("event1", "SyncCompleted", time.Now().Add(-10*time.Minute)),
			createTestEvent("event2", "SyncFailed", time.Now().Add(-8*time.Minute)),
			createTestEvent("event3", "SyncCompleted", time.Now().Add(-6*time.Minute)),
			createTestEvent("event4", "SyncCompleted", time.Now().Add(-4*time.Minute)),
			createTestEvent("event5", "SyncStarted", time.Now().Add(-2*time.Minute)),
			createTestEvent("event6", "SyncCompleted", time.Now().Add(-1*time.Minute)),
			// Non-Porch event (should be ignored)
			{
				ObjectMeta: metav1.ObjectMeta{Name: "other-event"},
				Source:     corev1.EventSource{Component: "other-component"},
				Reason:     "SomeReason",
				FirstTimestamp: metav1.NewTime(time.Now().Add(-5 * time.Minute)),
			},
		},
		deletedEvents: make([]string, 0),
	}

	cleanupOldEvents(context.Background(), mockClient, "test-repo", "test-ns", 3)

	// Should delete 3 oldest Porch events (event1, event2, event3)
	if len(mockClient.deletedEvents) != 3 {
		t.Errorf("Expected 3 deleted events, got %d", len(mockClient.deletedEvents))
	}

	expectedDeleted := map[string]bool{"event1": true, "event2": true, "event3": true}
	for _, deleted := range mockClient.deletedEvents {
		if !expectedDeleted[deleted] {
			t.Errorf("Unexpected event deleted: %s", deleted)
		}
	}
}

func TestCleanupOldEvents_NoCleanupNeeded(t *testing.T) {
	mockClient := &mockClientWithList{
		events: []*corev1.Event{
			createTestEvent("event1", "SyncCompleted", time.Now().Add(-2*time.Minute)),
			createTestEvent("event2", "SyncCompleted", time.Now().Add(-1*time.Minute)),
		},
		deletedEvents: make([]string, 0),
	}

	cleanupOldEvents(context.Background(), mockClient, "test-repo", "test-ns", 5)

	// Should not delete any events
	if len(mockClient.deletedEvents) != 0 {
		t.Errorf("Expected 0 deleted events, got %d", len(mockClient.deletedEvents))
	}
}