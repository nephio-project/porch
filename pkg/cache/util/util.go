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

// Package cachetypes contains type definitions for caches in Porch.

package util

import (
	"context"
	"fmt"

	configapi "github.com/nephio-project/porch/api/porchconfig/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func BuildRepositoryCondition(repo *configapi.Repository, status string, errorMsg string) (metav1.Condition, error) {
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
		return metav1.Condition{
			Type:               configapi.RepositoryReady,
			Status:             metav1.ConditionTrue,
			ObservedGeneration: repo.Generation,
			LastTransitionTime: metav1.Now(),
			Reason:             configapi.ReasonReady,
			Message:            "Repository Ready",
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

func ApplyRepositoryCondition(ctx context.Context, client client.StatusWriter, repo *configapi.Repository, condition metav1.Condition, status string) error {
	if repo.Status.Conditions == nil {
		repo.Status.Conditions = []metav1.Condition{}
	}

	if len(repo.Status.Conditions) > 0 {
		repo.Status.Conditions[0] = condition
	} else {
		repo.Status.Conditions = append(repo.Status.Conditions, condition)
	}

	if err := client.Update(ctx, repo); err != nil {
		return fmt.Errorf("failed to update repository status: %w", err)
	}

	klog.V(2).Infof("Repository %s status updated to %s", repo.Name, status)
	return nil
}
