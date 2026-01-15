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
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	configapi "github.com/nephio-project/porch/controllers/repositories/api/v1alpha1"
	"github.com/nephio-project/porch/pkg/cache/util"
)

// updateRepoStatusWithBackoff updates status with retry logic using Server-Side Apply
// Uses shared util for all status updates to ensure API consistency
func (r *RepositoryReconciler) updateRepoStatusWithBackoff(ctx context.Context, repo *configapi.Repository, status util.RepositoryStatus, syncError error, nextSyncTime *time.Time) error {
	// Use shared util to build condition for API consistency
	errorMsg := ""
	if syncError != nil {
		errorMsg = syncError.Error()
	}
	condition, buildErr := util.BuildRepositoryCondition(repo, status, errorMsg, nextSyncTime)
	if buildErr != nil {
		log.FromContext(ctx).Error(buildErr, "Failed to build repository condition", "repository", repo.Name)
		return buildErr
	}

	// Create status patch with Server-Side Apply
	patch := &configapi.Repository{
		TypeMeta: metav1.TypeMeta{
			APIVersion: configapi.TypeRepository.APIVersion(),
			Kind:       configapi.TypeRepository.Kind,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      repo.Name,
			Namespace: repo.Namespace,
		},
		Status: configapi.RepositoryStatus{
			Conditions: []metav1.Condition{condition},
		},
	}

	return wait.ExponentialBackoff(wait.Backoff{
		Duration: 25 * time.Millisecond,
		Factor:   1.5,
		Jitter:   0.1,
		Steps:    2,
	}, func() (bool, error) {
		if err := r.Status().Patch(ctx, patch, client.Apply, client.FieldOwner("repository-controller")); err != nil {
			if errors.IsConflict(err) {
				return false, nil // Retry
			}
			return false, err
		}
		return true, nil
	})
}

// setCondition sets Repository condition
func (r *RepositoryReconciler) setCondition(repo *configapi.Repository, conditionType string, status metav1.ConditionStatus, reason, message string) {
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

// hasSpecChanged determines if Repository spec changed using condition ObservedGeneration
func (r *RepositoryReconciler) hasSpecChanged(repo *configapi.Repository) bool {
	for _, condition := range repo.Status.Conditions {
		if condition.Type == configapi.RepositoryReady {
			return condition.ObservedGeneration != repo.Generation
		}
	}
	return true // No condition exists, treat as spec changed
}
