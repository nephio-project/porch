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
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	configapi "github.com/nephio-project/porch/controllers/repositories/api/v1alpha1"
)

// createTestRepo creates a basic Repository for testing
func createTestRepo(name, namespace string) *configapi.Repository {
	return &configapi.Repository{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: configapi.RepositorySpec{
			Git: &configapi.GitRepository{
				Repo: "https://github.com/test/repo",
			},
		},
	}
}

// createTestRepoWithCondition creates a Repository with a specific condition
func createTestRepoWithCondition(name, namespace string, condition metav1.Condition) *configapi.Repository {
	repo := createTestRepo(name, namespace)
	repo.Status.Conditions = []metav1.Condition{condition}
	return repo
}

// createSyncEvent creates a sync event for testing
func createSyncEvent(repoName, namespace, reason, message string, timestamp time.Time) corev1.Event {
	return corev1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Name:              repoName + "-" + reason,
			Namespace:         namespace,
			CreationTimestamp: metav1.NewTime(timestamp),
		},
		InvolvedObject: corev1.ObjectReference{
			Name:       repoName,
			Namespace:  namespace,
			APIVersion: configapi.GroupVersion.Identifier(),
			Kind:       configapi.TypeRepository.Kind,
		},
		Source: corev1.EventSource{
			Component: PorchCacheComponent,
		},
		Reason:  reason,
		Message: message,
		Type:    "Normal",
	}
}

// Test assertion helpers
func assertError(t *testing.T, expectError bool, err error) {
	if expectError && err == nil {
		t.Error("Expected error but got none")
	}
	if !expectError && err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
}

func assertRequeue(t *testing.T, expectRequeue bool, result ctrl.Result) {
	if expectRequeue && result.RequeueAfter == 0 {
		t.Error("Expected requeue but got none")
	}
}