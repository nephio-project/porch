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
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	api "github.com/nephio-project/porch/controllers/repositories/api/v1alpha1"
)

const (
	// PorchCacheComponent is the component name used by porch cache for events
	PorchCacheComponent = "porch-server"
)

// mapEventToRepository maps sync events to Repository reconcile requests
func (r *RepositoryReconciler) mapEventToRepository(ctx context.Context, obj client.Object) []ctrl.Request {
	event, ok := obj.(*corev1.Event)
	if !ok {
		return nil
	}

	log.FromContext(ctx).V(2).Info("Processing event", "reason", event.Reason, "source", event.Source.Component, "involvedObject", event.InvolvedObject.Kind, "name", event.InvolvedObject.Name)

	// Only handle Repository sync events from porch-server
	if event.InvolvedObject.APIVersion != api.GroupVersion.Identifier() ||
		event.InvolvedObject.Kind != api.TypeRepository.Kind ||
		event.Source.Component != PorchCacheComponent ||
		!strings.Contains(event.Reason, "Sync") {
		log.FromContext(ctx).V(2).Info("Event filtered out", "reason", event.Reason, "source", event.Source.Component, "involvedObject", event.InvolvedObject.Kind)
		return nil
	}

	return []ctrl.Request{{
		NamespacedName: client.ObjectKey{
			Name:      event.InvolvedObject.Name,
			Namespace: event.InvolvedObject.Namespace,
		},
	}}
}

// handleCacheEvents handles event-driven reconciliation
func (r *RepositoryReconciler) handleCacheEvents(ctx context.Context, repo *api.Repository) (ctrl.Result, bool) {
	latestSyncEvent, err := r.getLatestSyncEvent(ctx, repo)
	if err != nil {
		return ctrl.Result{}, true
	}

	// Update status based on latest sync event
	if latestSyncEvent != nil {
		log.FromContext(ctx).Info("Processing sync event", "repository", repo.Name, "event", latestSyncEvent.Reason, "message", latestSyncEvent.Message)
		if err := r.processSyncEvent(ctx, repo, latestSyncEvent); err != nil {
			log.FromContext(ctx).Error(err, "Failed to handle sync event", "repository", repo.Name)
			return ctrl.Result{}, false
		}
		return ctrl.Result{}, true
	}

	// Check existing conditions to determine next action
	readyCondition := r.getReadyCondition(repo)
	if readyCondition == nil {
		// No condition exists - set initial status
		log.FromContext(ctx).V(2).Info("No recent sync events found, setting initial status", "repository", repo.Name)
		r.setCondition(repo, api.RepositoryReady, metav1.ConditionFalse, api.ReasonReconciling, "Repository reconciliation in progress")
		if updateErr := r.updateRepoStatusWithBackoff(ctx, repo); updateErr != nil {
			log.FromContext(ctx).Error(updateErr, "Failed to update repository status", "repository", repo.Name)
		}
		return ctrl.Result{}, true
	}

	// Handle based on condition state
	switch {
	case readyCondition.Status == metav1.ConditionFalse && readyCondition.Reason == api.ReasonError:
		// Error state - preserve and skip
		log.FromContext(ctx).V(2).Info("Repository already has error condition, skipping cache check", "repository", repo.Name)
		return ctrl.Result{}, true
	case readyCondition.Status == metav1.ConditionTrue:
		// Ready state - trigger sync startup
		log.FromContext(ctx).Info("Repository Ready, triggering one-time sync manager startup", "repository", repo.Name)
		return ctrl.Result{}, false // Force full reconciliation once
	default:
		// Other states (Reconciling, etc.) - let full reconciliation handle
		return ctrl.Result{}, false
	}
}

// getLatestSyncEvent retrieves the most recent sync event for a repository
func (r *RepositoryReconciler) getLatestSyncEvent(ctx context.Context, repo *api.Repository) (*corev1.Event, error) {
	// Use field indexer for efficient event lookup by involved object name
	eventList := &corev1.EventList{}
	
	// Try using the field indexer first (if available)
	listOpts := []client.ListOption{
		client.InNamespace(repo.Namespace),
		client.MatchingFields{"involvedObject.name": repo.Name},
	}
	
	if err := r.List(ctx, eventList, listOpts...); err != nil {
		// If field indexer fails, fall back to listing all events (less efficient)
		log.FromContext(ctx).V(1).Info("Field indexer failed, falling back to full event list", "repository", repo.Name, "error", err)
		
		eventList = &corev1.EventList{}
		if err := r.List(ctx, eventList, client.InNamespace(repo.Namespace)); err != nil {
			log.FromContext(ctx).V(1).Info("Failed to list events for repository", "repository", repo.Name, "error", err)
			return nil, err
		}
		
		// Filter events in memory when using fallback
		var filteredEvents []corev1.Event
		for _, event := range eventList.Items {
			if event.InvolvedObject.Name == repo.Name && event.InvolvedObject.Kind == api.TypeRepository.Kind {
				filteredEvents = append(filteredEvents, event)
			}
		}
		eventList.Items = filteredEvents
	}

	// Find most recent sync event from porch-server
	var latestSyncEvent *corev1.Event
	for i := range eventList.Items {
		event := &eventList.Items[i]
		if event.Source.Component == PorchCacheComponent && strings.Contains(event.Reason, "Sync") {
			if latestSyncEvent == nil || event.CreationTimestamp.After(latestSyncEvent.CreationTimestamp.Time) {
				latestSyncEvent = event
			}
		}
	}

	log.FromContext(ctx).V(2).Info("Event search completed", "repository", repo.Name, "candidateEvents", len(eventList.Items), "foundSyncEvent", latestSyncEvent != nil)
	return latestSyncEvent, nil
}

// getReadyCondition finds the RepositoryReady condition in the status
func (r *RepositoryReconciler) getReadyCondition(repo *api.Repository) *metav1.Condition {
	for _, condition := range repo.Status.Conditions {
		if condition.Type == api.RepositoryReady {
			return &condition
		}
	}
	return nil
}


