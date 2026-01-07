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
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	api "github.com/nephio-project/porch/controllers/repositories/api/v1alpha1"
)

const (
	// PorchSyncComponent is the component name for sync events
	PorchSyncComponent = "porch-server"
)

// NewRepositoryEventRecorder creates a new event recorder for repository sync operations
func NewRepositoryEventRecorder(coreClient client.WithWatch) record.EventRecorder {
	return &SimpleEventRecorder{
		coreClient: coreClient,
		component:  PorchSyncComponent,
	}
}

type SimpleEventRecorder struct {
	coreClient client.WithWatch
	component  string
}

func (r *SimpleEventRecorder) Event(object runtime.Object, eventtype, reason, message string) {
	r.AnnotatedEventf(object, nil, eventtype, reason, "%s", message)
}

func (r *SimpleEventRecorder) Eventf(object runtime.Object, eventtype, reason, messageFmt string, args ...any) {
	r.AnnotatedEventf(object, nil, eventtype, reason, messageFmt, args...)
}

func (r *SimpleEventRecorder) AnnotatedEventf(object runtime.Object, annotations map[string]string, eventtype, reason, messageFmt string, args ...any) {
	r.createEvent(object, annotations, eventtype, reason, messageFmt, args...)
}

func (r *SimpleEventRecorder) createEvent(object runtime.Object, annotations map[string]string, eventtype, reason, messageFmt string, args ...any) {
	ctx := context.Background()
	obj := object.(client.Object)
	now := metav1.Now()
	
	// For SyncStarted events, offset by 1s to ensure correct ordering due to rapid sync cycles
	if reason == "SyncStarted" {
		now.Time = now.Time.Add(-1 * time.Second)
	}
	
	event := &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Name:        fmt.Sprintf("%s.%s.%x", obj.GetName(), reason, now.UnixNano()),
			Namespace:   obj.GetNamespace(),
			Annotations: annotations,
		},
		InvolvedObject: corev1.ObjectReference{
			Kind:       api.TypeRepository.Kind,
			Name:       obj.GetName(),
			Namespace:  obj.GetNamespace(),
			UID:        obj.GetUID(),
			APIVersion: api.TypeRepository.APIVersion(),
		},
		Reason:  reason,
		Message: fmt.Sprintf(messageFmt, args...),
		Type:    eventtype,
		Source: corev1.EventSource{
			Component: r.component,
		},
		FirstTimestamp: now,
		LastTimestamp:  now,
		Count:          1,
	}
	
	if err := r.coreClient.Create(ctx, event); err != nil {
		klog.Errorf("Failed to create event %s: %v", event.Name, err)
	} else {
		klog.Infof("Successfully created event %s for %s/%s (reason: %s, type: %s)", event.Name, event.Namespace, event.InvolvedObject.Name, event.Reason, event.Type)
	}
}

// cleanupOldEvents removes old Porch events for a repository, keeping only the most recent ones
func cleanupOldEvents(ctx context.Context, coreClient client.WithWatch, repoName, repoNamespace string, keepCount int) {
	klog.V(2).Infof("Starting cleanup of old events for %s/%s (keepCount=%d)", repoNamespace, repoName, keepCount)
	
	events := &corev1.EventList{}
	listOpts := []client.ListOption{
		client.InNamespace(repoNamespace),
		client.MatchingFields{"involvedObject.name": repoName},
	}
	
	if err := coreClient.List(ctx, events, listOpts...); err != nil {
		klog.Errorf("Failed to list events for cleanup: %v", err)
		return
	}
	
	klog.V(2).Infof("Found %d total events for %s/%s", len(events.Items), repoNamespace, repoName)
	
	// Filter to only Porch events and sort by timestamp (newest first)
	var porchEvents []corev1.Event
	for _, event := range events.Items {
		klog.V(2).Infof("Checking event: %s (component: %s, reason: %s)", event.Name, event.Source.Component, event.Reason)
		if event.Source.Component == PorchSyncComponent {
			porchEvents = append(porchEvents, event)
			klog.V(3).Infof("Found Porch event: %s (timestamp: %v)", event.Name, event.FirstTimestamp)
		} else {
			klog.V(3).Infof("Skipping non-Porch event: %s (component: %s)", event.Name, event.Source.Component)
		}
	}
	
	klog.V(2).Infof("Found %d Porch events for %s/%s", len(porchEvents), repoNamespace, repoName)
	
	if len(porchEvents) <= keepCount {
		klog.V(2).Infof("No cleanup needed: %d events <= %d keepCount", len(porchEvents), keepCount)
		return // Nothing to clean up
	}
	
	// Sort by FirstTimestamp (newest first)
	for i := 0; i < len(porchEvents); i++ {
		for j := i + 1; j < len(porchEvents); j++ {
			if porchEvents[i].FirstTimestamp.Time.Before(porchEvents[j].FirstTimestamp.Time) {
				porchEvents[i], porchEvents[j] = porchEvents[j], porchEvents[i]
			}
		}
	}
	
	// Delete events beyond keepCount in batch
	var toDelete []client.Object
	for i := keepCount; i < len(porchEvents); i++ {
		event := porchEvents[i]
		toDelete = append(toDelete, &event)
		klog.V(2).Infof("Marking event for deletion: %s (timestamp: %v)", event.Name, event.FirstTimestamp)
	}
	
	deleted := 0
	for _, event := range toDelete {
		if err := coreClient.Delete(ctx, event); err != nil {
			klog.Errorf("Failed to delete old event %s: %v", event.GetName(), err)
		} else {
			klog.V(2).Infof("Successfully deleted event: %s", event.GetName())
			deleted++
		}
	}
	
	if deleted > 0 {
		klog.V(1).Infof("Cleaned up %d old events for repository %s/%s (kept %d most recent)", deleted, repoNamespace, repoName, keepCount)
	}
}