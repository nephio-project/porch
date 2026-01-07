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
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	configapi "github.com/nephio-project/porch/controllers/repositories/api/v1alpha1"
	"github.com/nephio-project/porch/pkg/cache/util"
)

// processSyncEvent updates Repository status based on cache sync events
func (r *RepositoryReconciler) processSyncEvent(ctx context.Context, repo *configapi.Repository, event *corev1.Event) error {
	log.FromContext(ctx).Info("Processing sync event details", "repository", repo.Name, "eventReason", event.Reason, "eventMessage", event.Message)
	
	if r.hasConnectivityFailure(ctx, repo, event) {
		return nil
	}

	var status util.RepositoryStatus
	var nextSyncTime *time.Time
	var syncError error

	switch {
	case strings.Contains(event.Reason, "SyncStarted"):
		status = util.RepositoryStatusSyncInProgress
		log.FromContext(ctx).Info("Setting repository to SyncInProgress", "repository", repo.Name)
	case strings.Contains(event.Reason, "SyncCompleted"):
		status = util.RepositoryStatusReady
		nextSyncTime = r.getNextSyncAt(event)
		log.FromContext(ctx).Info("Setting repository to Ready", "repository", repo.Name, "nextSyncTime", nextSyncTime)
	case strings.Contains(event.Reason, "SyncFailed"):
		status = util.RepositoryStatusError
		syncError = fmt.Errorf("%s", event.Message)
		log.FromContext(ctx).Info("Setting repository to Error", "repository", repo.Name, "error", syncError)
	default:
		// Other sync events - no status change needed
		log.FromContext(ctx).Info("Unknown sync event, no status change", "repository", repo.Name, "eventReason", event.Reason)
		return nil
	}

	condition, err := util.BuildRepositoryCondition(repo, status, "", nextSyncTime)
	if err != nil {
		log.FromContext(ctx).Error(err, "Failed to build repository condition", "repository", repo.Name)
		return err
	}
	if syncError != nil {
		condition.Message = syncError.Error()
	}

	log.FromContext(ctx).Info("Built condition", "repository", repo.Name, "conditionType", condition.Type, "conditionStatus", condition.Status, "conditionReason", condition.Reason)
	meta.SetStatusCondition(&repo.Status.Conditions, condition)
	log.FromContext(ctx).Info("About to update repository status", "repository", repo.Name)
	err = r.updateRepoStatusWithBackoff(ctx, repo)
	if err != nil {
		log.FromContext(ctx).Error(err, "Failed to update repository status", "repository", repo.Name)
	} else {
		log.FromContext(ctx).Info("Successfully updated repository status", "repository", repo.Name)
	}
	return err
}

// setCondition sets Repository condition
func (r *RepositoryReconciler) setCondition(repo *configapi.Repository, conditionType string, status metav1.ConditionStatus, reason, message string) {
	// Always set the condition - let meta.SetStatusCondition handle deduplication
	condition := metav1.Condition{
		Type:               conditionType,
		Status:             status,
		ObservedGeneration: repo.Generation,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	}
	meta.SetStatusCondition(&repo.Status.Conditions, condition)
}

// updateRepoStatusWithBackoff updates status with retry logic for the main reconcile loop
func (r *RepositoryReconciler) updateRepoStatusWithBackoff(ctx context.Context, repo *configapi.Repository) error {
	return wait.ExponentialBackoff(wait.Backoff{
		Duration: 25 * time.Millisecond,
		Factor:   1.5,
		Jitter:   0.1,
		Steps:    2, // Max 2 retries for main reconcile loop
	}, func() (bool, error) {
		if updateErr := r.Status().Update(ctx, repo); updateErr != nil {
			if errors.IsConflict(updateErr) {
				// Get latest version and retry
				key := client.ObjectKey{Namespace: repo.Namespace, Name: repo.Name}
				if getErr := r.Get(ctx, key, repo); getErr != nil {
					return false, getErr
				}
				return false, nil // Retry with fresh object
			}
			return false, updateErr // Don't retry on other errors
		}
		return true, nil // Success
	})
}

// updateRepoStatusOnError updates repository status when cache operations fail
func (r *RepositoryReconciler) updateRepoStatusOnError(ctx context.Context, repo *configapi.Repository, err error) {
	// Get fresh copy of repository
	key := client.ObjectKey{Namespace: repo.Namespace, Name: repo.Name}
	if getErr := r.Get(ctx, key, repo); getErr != nil {
		log.FromContext(ctx).Error(getErr, "Failed to get repository for status update", "repository", repo.Name)
		return
	}

	// Use shared util to build condition
	condition, buildErr := util.BuildRepositoryCondition(repo, util.RepositoryStatusError, err.Error(), nil)
	if buildErr != nil {
		log.FromContext(ctx).Error(buildErr, "Failed to build repository condition", "repository", repo.Name)
		return
	}

	meta.SetStatusCondition(&repo.Status.Conditions, condition)
	// Use status client directly to avoid triggering reconciles
	if updateErr := r.Status().Update(ctx, repo); updateErr != nil {
		log.FromContext(ctx).Error(updateErr, "Failed to update repository status", "repository", repo.Name)
	}
}

// hasSpecChanged determines if Repository spec changed using condition ObservedGeneration
func (r *RepositoryReconciler) hasSpecChanged(repo *configapi.Repository) bool {
	// Find the most recent condition
	for _, condition := range repo.Status.Conditions {
		if condition.Type == configapi.RepositoryReady {
			// Compare condition's ObservedGeneration with current Generation
			return condition.ObservedGeneration != repo.Generation
		}
	}
	// No condition exists, treat as spec changed
	return true
}

// getNextSyncAt extracts next sync time from event annotations (preferred) or message (fallback)
func (r *RepositoryReconciler) getNextSyncAt(event *corev1.Event) *time.Time {
	// Try annotations first (preferred)
	if nextSyncStr, ok := event.Annotations["porch.kpt.dev/next-sync-time"]; ok {
		if nextTime, err := time.Parse(time.RFC3339, nextSyncStr); err == nil && nextTime.After(time.Now()) {
			return &nextTime
		}
	}

	// Fallback to message parsing for backward compatibility
	if strings.Contains(event.Message, "next sync at:") {
		parts := strings.Split(event.Message, "next sync at: ")
		if len(parts) > 1 {
			timestamp := strings.TrimSuffix(parts[1], ")")
			if nextTime, err := time.Parse(time.RFC3339, timestamp); err == nil && nextTime.After(time.Now()) {
				return &nextTime
			}
		}
	}

	return nil
}

// hasConnectivityFailure checks if repository has connectivity failure and should ignore sync events
func (r *RepositoryReconciler) hasConnectivityFailure(ctx context.Context, repo *configapi.Repository, event *corev1.Event) bool {
	for _, condition := range repo.Status.Conditions {
		if condition.Type == configapi.RepositoryReady &&
			condition.Status == metav1.ConditionFalse &&
			condition.Reason == configapi.ReasonError &&
			strings.Contains(condition.Message, "Connectivity check failed") {
			log.FromContext(ctx).V(1).Info("Ignoring sync event due to connectivity failure", "repository", repo.Name, "event", event.Reason)
			return true
		}
	}
	return false
}
