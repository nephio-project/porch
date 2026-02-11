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

	repocontroller "github.com/nephio-project/porch/controllers/repositories/pkg/controllers/repository"
	cachetypes "github.com/nephio-project/porch/pkg/cache/types"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
)

// EmbeddedControllerManager manages embedded Repository controller
type EmbeddedControllerManager struct {
	cache        cachetypes.Cache
	mgr          ctrl.Manager
	config       repocontroller.EmbeddedConfig
	webhookReady <-chan struct{}
}

// createEmbeddedControllerManager creates controller manager
func createEmbeddedControllerManager(
	restConfig *rest.Config,
	scheme *runtime.Scheme,
	config repocontroller.EmbeddedConfig,
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
		mgr:    mgr,
		config: config,
	}, nil
}

// Start sets up the controller manager, and the controller for it
func (e *EmbeddedControllerManager) Start(ctx context.Context) error {
	repoController := &repocontroller.RepositoryReconciler{
		Client: e.mgr.GetClient(),
		Scheme: e.mgr.GetScheme(),
		Cache:  e.cache,
	}
	repoController.SetEmbeddedDefaults(e.config)
	repoController.SetLogger("repositories")

	if err := repoController.SetupWithManager(e.mgr); err != nil {
		return fmt.Errorf("failed to setup controller: %w", err)
	}

	// Wait for external webhook to be ready
	if e.webhookReady != nil {
		select {
		case <-e.webhookReady:
			klog.Info("Webhook ready, starting controller")
		case <-time.After(30 * time.Second):
			klog.Warning("Webhook readiness timeout, starting controller anyway")
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	return e.mgr.Start(ctx)
}
