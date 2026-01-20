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

// Package util provides shared utilities for repository status management.
//
// BuildRepositoryCondition is used by the repository controller for API consistency.
package util

import (
	"fmt"
	"time"

	configapi "github.com/nephio-project/porch/controllers/repositories/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RepositoryStatus represents the status of a repository sync operation
type RepositoryStatus string

const (
	RepositoryStatusSyncInProgress RepositoryStatus = "sync-in-progress"
	RepositoryStatusReady          RepositoryStatus = "ready"
	RepositoryStatusError          RepositoryStatus = "error"
)

// BuildRepositoryCondition builds a repository condition for the given status.
// Used by the repository controller for API consistency.
func BuildRepositoryCondition(repo *configapi.Repository, status RepositoryStatus, errorMsg string, nextSyncTime *time.Time) (metav1.Condition, error) {
	switch status {
	case RepositoryStatusSyncInProgress:
		return metav1.Condition{
			Type:               configapi.RepositoryReady,
			Status:             metav1.ConditionFalse,
			ObservedGeneration: repo.Generation,
			LastTransitionTime: metav1.Now(),
			Reason:             configapi.ReasonReconciling,
			Message:            "Repository reconciliation in progress",
		}, nil
	case RepositoryStatusReady:
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
	case RepositoryStatusError:
		if errorMsg == "" {
			errorMsg = "unknown error"
		}
		message := errorMsg
		if nextSyncTime != nil {
			message = fmt.Sprintf("%s (next retry at: %s)", errorMsg, nextSyncTime.Format(time.RFC3339))
		}
		return metav1.Condition{
			Type:               configapi.RepositoryReady,
			Status:             metav1.ConditionFalse,
			ObservedGeneration: repo.Generation,
			LastTransitionTime: metav1.Now(),
			Reason:             configapi.ReasonError,
			Message:            message,
		}, nil
	default:
		return metav1.Condition{}, fmt.Errorf("unknown status type: %s", status)
	}
}

