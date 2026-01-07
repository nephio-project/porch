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
	"time"

	configapi "github.com/nephio-project/porch/controllers/repositories/api/v1alpha1"
	repocontroller "github.com/nephio-project/porch/controllers/repositories/pkg/controllers/repository"
	cachetypes "github.com/nephio-project/porch/pkg/cache/types"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// EmbeddedControllerManager manages embedded Repository controller
type EmbeddedControllerManager struct {
	coreClient client.WithWatch
	cache      cachetypes.Cache
	mgr        ctrl.Manager
}

// createEmbeddedController creates controller manager
func createEmbeddedController(
	coreClient client.WithWatch,
	restConfig *rest.Config,
	scheme *runtime.Scheme,
) (*EmbeddedControllerManager, error) {
	mgr, err := ctrl.NewManager(restConfig, ctrl.Options{
		Scheme:                 scheme,
		LeaderElection:         false,
		HealthProbeBindAddress: "0",
		WebhookServer:          nil,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create controller manager: %w", err)
	}
	
	return &EmbeddedControllerManager{
		coreClient: coreClient,
		mgr:        mgr,
	}, nil
}

// Start starts the embedded Repository controller
func (e *EmbeddedControllerManager) Start(ctx context.Context) error {
	repoController := &repocontroller.RepositoryReconciler{
		Client: e.coreClient,
		Scheme: e.mgr.GetScheme(),
		Cache:  e.cache,
	}
	repoController.SetEmbeddedDefaults()
	
	if err := repoController.SetupWithManager(e.mgr); err != nil {
		return fmt.Errorf("failed to setup controller: %w", err)
	}
	
	// Initialize existing repositories
	go e.initializeRepositories(ctx)
	
	return e.mgr.Start(ctx)
}

func (e *EmbeddedControllerManager) initializeRepositories(ctx context.Context) {
	time.Sleep(2 * time.Second)
	klog.Info("Initializing existing repositories")
	
	repos := &configapi.RepositoryList{}
	if err := e.coreClient.List(ctx, repos); err != nil {
		klog.Error(err, "Failed to list repositories")
		return
	}
	
	klog.Infof("Found %d repositories to initialize", len(repos.Items))
	for _, repo := range repos.Items {
		if _, err := e.cache.OpenRepository(ctx, &repo); err != nil {
			klog.Error(err, "Failed to initialize repository", "repository", repo.Name)
		} else {
			klog.V(1).Infof("Initialized repository %s", repo.Name)
		}
	}
	klog.Info("Repository initialization completed")
}