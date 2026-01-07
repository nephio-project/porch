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
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	api "github.com/nephio-project/porch/controllers/repositories/api/v1alpha1"
)

func TestMapEventToRepository(t *testing.T) {
	r := &RepositoryReconciler{}
	ctx := context.Background()
	testRepo := "test-repo"
	testNs := "test-ns"

	createEvent := func(apiVersion, kind, component, reason string) *corev1.Event {
		return &corev1.Event{
			ObjectMeta: metav1.ObjectMeta{Name: "test-event", Namespace: testNs},
			InvolvedObject: corev1.ObjectReference{
				APIVersion: apiVersion, Kind: kind, Name: testRepo, Namespace: testNs,
			},
			Source: corev1.EventSource{Component: component},
			Reason: reason,
		}
	}

	expectedRequest := []ctrl.Request{{
		NamespacedName: client.ObjectKey{Name: testRepo, Namespace: testNs},
	}}

	tests := []struct {
		name     string
		event    *corev1.Event
		expected []ctrl.Request
	}{
		{"valid sync event", createEvent(api.GroupVersion.Identifier(), api.TypeRepository.Kind, PorchCacheComponent, "SyncCompleted"), expectedRequest},
		{"sync failed event", createEvent(api.GroupVersion.Identifier(), api.TypeRepository.Kind, PorchCacheComponent, "SyncFailed"), expectedRequest},
		{"wrong api version", createEvent("v1", api.TypeRepository.Kind, PorchCacheComponent, "SyncCompleted"), nil},
		{"wrong kind", createEvent(api.GroupVersion.Identifier(), "Pod", PorchCacheComponent, "SyncCompleted"), nil},
		{"wrong component", createEvent(api.GroupVersion.Identifier(), api.TypeRepository.Kind, "other", "SyncCompleted"), nil},
		{"non-sync reason", createEvent(api.GroupVersion.Identifier(), api.TypeRepository.Kind, PorchCacheComponent, "Created"), nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var obj client.Object = tt.event
			if tt.event == nil {
				obj = &corev1.Pod{}
			}

			result := r.mapEventToRepository(ctx, obj)

			if len(result) != len(tt.expected) {
				t.Errorf("expected %d requests, got %d", len(tt.expected), len(result))
				return
			}

			for i, req := range result {
				if req.NamespacedName != tt.expected[i].NamespacedName {
					t.Errorf("expected %v, got %v", tt.expected[i].NamespacedName, req.NamespacedName)
				}
			}
		})
	}
}

func TestGetLatestSyncEvent(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = api.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	createRepo := func(name, ns string) *api.Repository {
		return &api.Repository{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		}
	}

	createEvent := func(name, ns, reason, message string, timestamp time.Time) corev1.Event {
		return corev1.Event{
			ObjectMeta: metav1.ObjectMeta{Name: reason, Namespace: ns, CreationTimestamp: metav1.NewTime(timestamp)},
			InvolvedObject: corev1.ObjectReference{Name: name, Namespace: ns},
			Source: corev1.EventSource{Component: PorchCacheComponent},
			Reason: reason, Message: message,
		}
	}

	tests := []struct {
		name     string
		events   []corev1.Event
		expected *corev1.Event
	}{
		{
			"returns latest sync event",
			[]corev1.Event{
				createEvent("test-repo", "test-ns", "SyncCompleted", "old", time.Now().Add(-5*time.Minute)),
				createEvent("test-repo", "test-ns", "SyncFailed", "latest", time.Now()),
			},
			&corev1.Event{Reason: "SyncFailed", Message: "latest"},
		},
		{
			"filters non-sync events",
			[]corev1.Event{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "other", Namespace: "test-ns", CreationTimestamp: metav1.NewTime(time.Now())},
					InvolvedObject: corev1.ObjectReference{Name: "test-repo", Namespace: "test-ns"},
					Source: corev1.EventSource{Component: PorchCacheComponent},
					Reason: "OtherEvent",
				},
				createEvent("test-repo", "test-ns", "SyncCompleted", "sync event", time.Now()),
			},
			&corev1.Event{Reason: "SyncCompleted", Message: "sync event"},
		},
		{"no events", []corev1.Event{}, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := createRepo("test-repo", "test-ns")
			objects := []client.Object{repo}
			for i := range tt.events {
				objects = append(objects, &tt.events[i])
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(objects...).
				WithIndex(&corev1.Event{}, "involvedObject.name", func(obj client.Object) []string {
					return []string{obj.(*corev1.Event).InvolvedObject.Name}
				}).
				Build()

			r := &RepositoryReconciler{Client: fakeClient}
			event, err := r.getLatestSyncEvent(context.Background(), repo)

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			if tt.expected == nil {
				if event != nil {
					t.Errorf("Expected nil, got %v", event.Reason)
				}
			} else if event == nil {
				t.Error("Expected event but got nil")
			} else if event.Reason != tt.expected.Reason || event.Message != tt.expected.Message {
				t.Errorf("Expected %s/%s, got %s/%s", tt.expected.Reason, tt.expected.Message, event.Reason, event.Message)
			}
		})
	}
}

func TestGetReadyCondition(t *testing.T) {
	r := &RepositoryReconciler{}

	tests := []struct {
		name     string
		repo     *api.Repository
		expected *metav1.Condition
	}{
		{
			"finds ready condition",
			&api.Repository{
				Status: api.RepositoryStatus{
					Conditions: []metav1.Condition{
						{Type: api.RepositoryReady, Status: metav1.ConditionTrue, Reason: api.ReasonReady},
					},
				},
			},
			&metav1.Condition{Type: api.RepositoryReady, Status: metav1.ConditionTrue, Reason: api.ReasonReady},
		},
		{
			"no conditions returns nil",
			&api.Repository{},
			nil,
		},
		{
			"no ready condition returns nil",
			&api.Repository{
				Status: api.RepositoryStatus{
					Conditions: []metav1.Condition{
						{Type: "OtherCondition", Status: metav1.ConditionTrue},
					},
				},
			},
			nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := r.getReadyCondition(tt.repo)

			if tt.expected == nil {
				if result != nil {
					t.Errorf("Expected nil, got %v", result)
				}
			} else if result == nil {
				t.Error("Expected condition, got nil")
			} else if result.Type != tt.expected.Type || result.Status != tt.expected.Status || result.Reason != tt.expected.Reason {
				t.Errorf("Expected %+v, got %+v", tt.expected, result)
			}
		})
	}
}