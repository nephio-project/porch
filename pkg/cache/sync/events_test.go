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

package sync

import (
	"context"
	"testing"
	"time"

	configapi "github.com/nephio-project/porch/controllers/repositories/api/v1alpha1"
	mockclient "github.com/nephio-project/porch/test/mockery/mocks/external/sigs.k8s.io/controller-runtime/pkg/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestNewRepositoryEventRecorder(t *testing.T) {
	mockClient := mockclient.NewMockWithWatch(t)
	recorder := NewRepositoryEventRecorder(mockClient)
	
	assert.NotNil(t, recorder)
	simpleRecorder, ok := recorder.(*SimpleEventRecorder)
	assert.True(t, ok)
	assert.Equal(t, PorchSyncComponent, simpleRecorder.component)
	assert.Equal(t, mockClient, simpleRecorder.coreClient)
}

func TestSimpleEventRecorder_Event(t *testing.T) {
	mockClient := mockclient.NewMockWithWatch(t)
	repo := &configapi.Repository{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-repo",
			Namespace: "default",
			UID:       "test-uid",
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       "Repository",
			APIVersion: "config.porch.kpt.dev/v1alpha1",
		},
	}
	
	mockClient.EXPECT().Create(mock.Anything, mock.MatchedBy(func(event *corev1.Event) bool {
		return event.Reason == "TestReason" &&
			event.Message == "test message" &&
			event.Type == "Normal" &&
			event.Source.Component == PorchSyncComponent &&
			event.InvolvedObject.Name == "test-repo"
	}), mock.Anything).Return(nil)
	
	recorder := &SimpleEventRecorder{
		coreClient: mockClient,
		component:  PorchSyncComponent,
	}
	
	recorder.Event(repo, "Normal", "TestReason", "test message")
	
	// Give goroutine time to execute
	time.Sleep(10 * time.Millisecond)
}

func TestCleanupOldEvents_NoCleanupNeeded(t *testing.T) {
	mockClient := mockclient.NewMockWithWatch(t)
	ctx := context.Background()
	
	events := &corev1.EventList{
		Items: []corev1.Event{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "event1"},
				Source:     corev1.EventSource{Component: PorchSyncComponent},
			},
		},
	}
	
	mockClient.EXPECT().List(ctx, mock.AnythingOfType("*v1.EventList"), mock.Anything, mock.Anything).RunAndReturn(func(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
		eventList := list.(*corev1.EventList)
		*eventList = *events
		return nil
	})
	
	cleanupOldEvents(ctx, mockClient, "test-repo", "default", 5)
	
	// Should not call Delete since we have fewer events than keepCount
	mockClient.AssertNotCalled(t, "Delete")
}

func TestCleanupOldEvents_DeletesOldEvents(t *testing.T) {
	mockClient := mockclient.NewMockWithWatch(t)
	ctx := context.Background()
	
	now := time.Now()
	events := &corev1.EventList{
		Items: []corev1.Event{
			{
				ObjectMeta:     metav1.ObjectMeta{Name: "event1"},
				Source:         corev1.EventSource{Component: PorchSyncComponent},
				FirstTimestamp: metav1.NewTime(now.Add(-6 * time.Hour)),
			},
			{
				ObjectMeta:     metav1.ObjectMeta{Name: "event2"},
				Source:         corev1.EventSource{Component: PorchSyncComponent},
				FirstTimestamp: metav1.NewTime(now.Add(-5 * time.Hour)),
			},
			{
				ObjectMeta:     metav1.ObjectMeta{Name: "event3"},
				Source:         corev1.EventSource{Component: PorchSyncComponent},
				FirstTimestamp: metav1.NewTime(now.Add(-4 * time.Hour)),
			},
			{
				ObjectMeta:     metav1.ObjectMeta{Name: "event4"},
				Source:         corev1.EventSource{Component: PorchSyncComponent},
				FirstTimestamp: metav1.NewTime(now.Add(-3 * time.Hour)),
			},
			{
				ObjectMeta:     metav1.ObjectMeta{Name: "event5"},
				Source:         corev1.EventSource{Component: PorchSyncComponent},
				FirstTimestamp: metav1.NewTime(now.Add(-2 * time.Hour)),
			},
			{
				ObjectMeta:     metav1.ObjectMeta{Name: "event6"},
				Source:         corev1.EventSource{Component: PorchSyncComponent},
				FirstTimestamp: metav1.NewTime(now.Add(-1 * time.Hour)),
			},
			{
				ObjectMeta:     metav1.ObjectMeta{Name: "event7"},
				Source:         corev1.EventSource{Component: PorchSyncComponent},
				FirstTimestamp: metav1.NewTime(now),
			},
		},
	}
	
	mockClient.EXPECT().List(ctx, mock.AnythingOfType("*v1.EventList"), mock.Anything, mock.Anything).RunAndReturn(func(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
		eventList := list.(*corev1.EventList)
		*eventList = *events
		return nil
	})
	
	// Should delete 2 oldest events when keepCount=5 (7 events - 5 = 2 to delete)
	mockClient.EXPECT().Delete(ctx, mock.MatchedBy(func(event *corev1.Event) bool {
		return event.Name == "event1"
	}), mock.Anything).Return(nil)
	mockClient.EXPECT().Delete(ctx, mock.MatchedBy(func(event *corev1.Event) bool {
		return event.Name == "event2"
	}), mock.Anything).Return(nil)
	
	cleanupOldEvents(ctx, mockClient, "test-repo", "default", 5)
}

func TestCleanupOldEvents_IgnoresNonPorchEvents(t *testing.T) {
	mockClient := mockclient.NewMockWithWatch(t)
	ctx := context.Background()
	
	events := &corev1.EventList{
		Items: []corev1.Event{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "porch-event"},
				Source:     corev1.EventSource{Component: PorchSyncComponent},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: "other-event"},
				Source:     corev1.EventSource{Component: "other-component"},
			},
		},
	}
	
	mockClient.EXPECT().List(ctx, mock.AnythingOfType("*v1.EventList"), mock.Anything, mock.Anything).RunAndReturn(func(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
		eventList := list.(*corev1.EventList)
		*eventList = *events
		return nil
	})
	
	cleanupOldEvents(ctx, mockClient, "test-repo", "default", 5)
	
	// Should not delete anything since we only have 1 Porch event
	mockClient.AssertNotCalled(t, "Delete")
}