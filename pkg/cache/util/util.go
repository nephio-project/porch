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

package util

import (
	"context"
	"fmt"
	"time"

	configapi "github.com/nephio-project/porch/api/porchconfig/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func BuildRepositoryCondition(repo *configapi.Repository, status string, errorMsg string, nextSyncTime *time.Time) (metav1.Condition, error) {
	switch status {
	case "sync-in-progress":
		return metav1.Condition{
			Type:               configapi.RepositoryReady,
			Status:             metav1.ConditionFalse,
			ObservedGeneration: repo.Generation,
			LastTransitionTime: metav1.Now(),
			Reason:             configapi.ReasonReconciling,
			Message:            "Repository reconciliation in progress",
		}, nil
	case "ready":
		message := "Repository Ready"
		if nextSyncTime != nil {
			message = fmt.Sprintf("Repository Ready (next sync scheduled at: %s)", nextSyncTime.Format(time.RFC3339))
		}
		return metav1.Condition{
			Type:               configapi.RepositoryReady,
			Status:             metav1.ConditionTrue,
			ObservedGeneration: repo.Generation,
			LastTransitionTime: metav1.Now(),
			Reason:             configapi.ReasonReady,
			Message:            message,
		}, nil
	case "error":
		if errorMsg == "" {
			errorMsg = "unknown error"
		}
		return metav1.Condition{
			Type:               configapi.RepositoryReady,
			Status:             metav1.ConditionFalse,
			ObservedGeneration: repo.Generation,
			LastTransitionTime: metav1.Now(),
			Reason:             configapi.ReasonError,
			Message:            errorMsg,
		}, nil
	default:
		return metav1.Condition{}, fmt.Errorf("unknown status type: %s", status)
	}
}

func ApplyRepositoryCondition(ctx context.Context, client client.Client, repo *configapi.Repository, condition metav1.Condition, status string) error {
	for _, attempt := range []int{1, 2, 3} {
		latestRepo := &configapi.Repository{}
		err := client.Get(ctx, types.NamespacedName{
			Namespace: repo.Namespace,
			Name:      repo.Name,
		}, latestRepo)
		if err != nil {
			return fmt.Errorf("failed to get latest repository object: %w", err)
		}

		if latestRepo.Status.Conditions == nil {
			latestRepo.Status.Conditions = []metav1.Condition{}
		}

		if len(latestRepo.Status.Conditions) > 0 {
			latestRepo.Status.Conditions[0] = condition
		} else {
			latestRepo.Status.Conditions = append(latestRepo.Status.Conditions, condition)
		}

		err = client.Status().Update(ctx, latestRepo)
		if err == nil {
			klog.V(2).Infof("Repository %s status updated to %s", repo.Name, status)
			return nil
		}

		if apierrors.IsConflict(err) {
			klog.V(3).Infof("Retrying status update for repository %q in namespace %q due to conflict (attempt %d)", repo.Name, repo.Namespace, attempt)
			time.Sleep(100 * time.Millisecond)
			continue
		}

		return fmt.Errorf("error updating repository status: %w", err)
	}

	return fmt.Errorf("failed to update repository status after retries")
}
