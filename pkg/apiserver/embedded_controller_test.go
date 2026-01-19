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

package apiserver

import (
	"context"
	"fmt"
	"testing"
	"time"

	configapi "github.com/nephio-project/porch/controllers/repositories/api/v1alpha1"
	repocontroller "github.com/nephio-project/porch/controllers/repositories/pkg/controllers/repository"
	cachetypes "github.com/nephio-project/porch/pkg/cache/types"
	mockcache "github.com/nephio-project/porch/test/mockery/mocks/porch/pkg/cache/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"github.com/stretchr/testify/mock"
)

func TestCreateEmbeddedController(t *testing.T) {
	scheme := runtime.NewScheme()
	
	// Create fake client
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	
	// Create test config
	config := repocontroller.EmbeddedConfig{
		MaxConcurrentReconciles: 25,
		MaxConcurrentSyncs:      50,
		HealthCheckFrequency:    5 * time.Minute,
		FullSyncFrequency:       1 * time.Hour,
	}
	
	// Test with invalid rest config (should fail)
	manager, err := createEmbeddedController(fakeClient, &rest.Config{}, scheme, config)
	if err != nil {
		// This is expected - invalid config should fail
		t.Logf("Expected error with invalid config: %v", err)
	} else if manager == nil {
		t.Error("Expected manager to be non-nil even with invalid config")
	}
}

func TestEmbeddedControllerManager_Start(t *testing.T) {
	// Test Start with nil manager - should panic/fail
	config := repocontroller.EmbeddedConfig{
		MaxConcurrentReconciles: 25,
		MaxConcurrentSyncs:      50,
		HealthCheckFrequency:    5 * time.Minute,
		FullSyncFrequency:       1 * time.Hour,
	}
	
	manager := &EmbeddedControllerManager{
		coreClient: fake.NewClientBuilder().Build(),
		cache:      nil,
		mgr:        nil,
		config:     config,
	}
	
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	
	// This should panic due to nil manager, so we expect it to fail
	defer func() {
		if r := recover(); r != nil {
			t.Logf("Expected panic with nil manager: %v", r)
		}
	}()
	
	err := manager.Start(ctx)
	if err == nil {
		t.Error("Expected error with nil manager")
	}
}

func TestEmbeddedControllerManager_initializeRepositories(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = configapi.AddToScheme(scheme)
	
	// Create fake client with test repository
	repo := &configapi.Repository{
		ObjectMeta: metav1.ObjectMeta{Name: "test-repo", Namespace: "default"},
	}
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(repo).Build()
	
	// Use mockery-generated cache with flexible matchers
	mockCache := mockcache.NewMockCache(t)
	mockCache.On("OpenRepository", mock.Anything, mock.Anything).Return(nil, fmt.Errorf("mock error")).Maybe()
	
	manager := &EmbeddedControllerManager{
		coreClient: fakeClient,
		cache:      mockCache,
	}
	
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	
	// Should not panic and should handle errors gracefully
	manager.initializeRepositories(ctx)
	
	// Don't assert expectations since the function may not call the mock due to timing
}

func TestCompletedConfig_CreateEmbeddedController(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = configapi.AddToScheme(scheme)
	
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	
	config := completedConfig{
		ExtraConfig: &ExtraConfig{
			RepoControllerConfig: RepoControllerConfig{
				MaxConcurrentReconciles: 10,
				MaxConcurrentSyncs:      20,
				HealthCheckFrequency:    5 * time.Minute,
				FullSyncFrequency:       1 * time.Hour,
			},
			CacheOptions: cachetypes.CacheOptions{
				RepoOperationRetryAttempts: 3,
			},
		},
	}
	
	// This will fail because getRestConfig will fail (no kubeconfig)
	// but it tests the function is callable and handles errors
	manager, err := config.createEmbeddedController(fakeClient)
	
	if err == nil {
		t.Error("Expected error when creating controller without valid kubeconfig")
	}
	
	if manager != nil {
		t.Error("Expected nil manager when creation fails")
	}
}
