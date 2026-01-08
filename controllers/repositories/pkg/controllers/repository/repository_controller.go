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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	api "github.com/nephio-project/porch/controllers/repositories/api/v1alpha1"
	cachetypes "github.com/nephio-project/porch/pkg/cache/types"
	"github.com/nephio-project/porch/pkg/repository"
)

const (
	RepositoryFinalizer = "config.porch.kpt.dev/repository"
)

// RepositoryReconciler reconciles Repository objects
type RepositoryReconciler struct {
	client.Client
	Scheme                    *runtime.Scheme
	Cache                     cachetypes.Cache // Injected cache instance
	connectivityRetryInterval time.Duration
	maxConcurrentReconciles   int
	repoSyncFrequency         time.Duration
	// Cache configuration
	cacheType  string
	dbDriver   string
	dbHost     string
	dbPort     string
	dbName     string
	dbUser     string
	dbPassword string
}

//go:generate go run sigs.k8s.io/controller-tools/cmd/controller-gen@v0.19.0 rbac:headerFile=../../../../../scripts/boilerplate.yaml.txt,roleName=porch-controllers-repositories webhook paths="." output:rbac:artifacts:config=../../../config/rbac

//+kubebuilder:rbac:groups=config.porch.kpt.dev,resources=repositories,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=config.porch.kpt.dev,resources=repositories/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=config.porch.kpt.dev,resources=repositories/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=events,verbs=get;list;watch

// Reconcile handles Repository reconciliation
func (r *RepositoryReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	if r.Cache == nil {
		return ctrl.Result{}, fmt.Errorf("cache not available")
	}

	// Get Repository
	repo := &api.Repository{}
	if err := r.Get(ctx, req.NamespacedName, repo); err != nil {
		if client.IgnoreNotFound(err) == nil {
			log.V(1).Info("Repository not found (likely deleted), ignoring reconcile", "repository", req.Name)
		}
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Handle deletion
	if !repo.DeletionTimestamp.IsZero() {
		log.Info("Repository marked for deletion", "repository", repo.Name)
		return r.handleDeletion(ctx, repo)
	}

	start := time.Now()
	triggerSource := "repository-change"
	if !r.hasSpecChanged(repo) {
		triggerSource = "sync-event"
	}
	reconcileType := map[bool]string{true: "spec-change", false: "event-driven"}[r.hasSpecChanged(repo)]

	log.V(1).Info("Repository reconciliation starting", "repository", repo.Name,
		"type", reconcileType, "trigger", triggerSource, "generation", repo.Generation)
	defer func() {
		log.V(1).Info("Repository reconciliation completed", "repository", repo.Name,
			"type", reconcileType, "trigger", triggerSource, "duration", time.Since(start))
	}()

	// Handle cache sync events
	if !r.hasSpecChanged(repo) {
		if result, handled := r.handleCacheEvents(ctx, repo); handled {
			return result, nil
		}
	}

	// Handle new or changed repositories
	return r.handleUpsertRepo(ctx, repo)
}

// handleUpsertRepo handles repositories with spec changes or new repositories
func (r *RepositoryReconciler) handleUpsertRepo(ctx context.Context, repo *api.Repository) (ctrl.Result, error) {
	// Add finalizer if not present
	if added, err := r.ensureFinalizer(ctx, repo); err != nil {
		return ctrl.Result{}, err
	} else if added {
		log.FromContext(ctx).Info("Finalizer added, waiting for next reconcile", "repository", repo.Name)
		return ctrl.Result{}, nil // Let next reconcile do the work
	}

	log.FromContext(ctx).Info("Opening repository", "repository", repo.Name)
	// Open repository
	repoHandle, err := r.openRepository(ctx, repo)
	if err != nil {
		r.updateRepoStatusOnError(ctx, repo, err)
		return ctrl.Result{RequeueAfter: r.connectivityRetryInterval}, nil
	}
	log.FromContext(ctx).Info("Repository opened successfully", "repository", repo.Name)

	// Refresh and validate repository for spec changes
	if err := r.refreshAndValidateRepository(ctx, repo, repoHandle); err != nil {
		log.FromContext(ctx).Error(err, "Repository refresh and validation failed", "repository", repo.Name)
		r.updateRepoStatusOnError(ctx, repo, err)
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}
	log.FromContext(ctx).Info("Repository validation completed", "repository", repo.Name)

	// Set reconciling status
	r.setCondition(repo, api.RepositoryReady, metav1.ConditionFalse, api.ReasonReconciling, "Repository reconciliation in progress")
	if err := r.updateRepoStatusWithBackoff(ctx, repo); err != nil {
		return ctrl.Result{}, err
	}
	log.FromContext(ctx).Info("Repository status set to reconciling", "repository", repo.Name)
	return ctrl.Result{}, nil
}

// ensureFinalizer adds finalizer if missing and returns true if added
func (r *RepositoryReconciler) ensureFinalizer(ctx context.Context, repo *api.Repository) (bool, error) {
	if controllerutil.ContainsFinalizer(repo, RepositoryFinalizer) {
		return false, nil
	}
	log.FromContext(ctx).Info("Adding finalizer to repository", "repository", repo.Name)
	controllerutil.AddFinalizer(repo, RepositoryFinalizer)
	if err := r.Update(ctx, repo); err != nil {
		return false, err
	}
	return true, nil
}

// openRepository checks connectivity and opens repository for use
func (r *RepositoryReconciler) openRepository(ctx context.Context, repo *api.Repository) (repository.Repository, error) {
	if err := r.Cache.CheckRepositoryConnectivity(ctx, repo); err != nil {
		log.FromContext(ctx).Error(err, "Repository connectivity check failed", "repository", repo.Name)
		r.setCondition(repo, api.RepositoryReady, metav1.ConditionFalse, api.ReasonError, fmt.Sprintf("Connectivity check failed: %v", err))
		if updateErr := r.updateRepoStatusWithBackoff(ctx, repo); updateErr != nil {
			log.FromContext(ctx).Error(updateErr, "Failed to update repository status", "repository", repo.Name)
		}
		return nil, err
	}
	return r.Cache.OpenRepository(ctx, repo)
}

// refreshAndValidateRepository performs full repository refresh and validation for spec changes
func (r *RepositoryReconciler) refreshAndValidateRepository(ctx context.Context, repo *api.Repository, repoHandle repository.Repository) error {
	if !r.hasSpecChanged(repo) {
		return nil
	}
	log.FromContext(ctx).Info("Spec changed, refreshing and validating repository", "repository", repo.Name)
	if err := repoHandle.Refresh(ctx); err != nil {
		return err
	}
	_, err := repoHandle.ListPackageRevisions(ctx, repository.ListPackageRevisionFilter{})
	return err
}

// SetupWithManager sets up the controller with the Manager
func (r *RepositoryReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Create cache if not already injected (standalone mode)
	if r.Cache == nil {
		// Create DB cache with sync events enabled
		if err := r.createDBCache(context.Background(), mgr); err != nil {
			return fmt.Errorf("failed to create database cache: %w", err)
		}
	}

	// Add field indexing for events to enable efficient lookups
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &corev1.Event{}, "involvedObject.name", func(rawObj client.Object) []string {
		event := rawObj.(*corev1.Event)
		return []string{event.InvolvedObject.Name}
	}); err != nil {
		return fmt.Errorf("failed to setup event field indexer: %w", err)
	}

	// Watch Repository CRs and Events on Repository objects
	_, err := ctrl.NewControllerManagedBy(mgr).
		For(&api.Repository{}, builder.WithPredicates(predicate.Funcs{
			UpdateFunc: func(e event.UpdateEvent) bool {
				// Trigger on generation changes OR finalizer changes
				return e.ObjectNew.GetGeneration() != e.ObjectOld.GetGeneration() ||
					len(e.ObjectNew.GetFinalizers()) != len(e.ObjectOld.GetFinalizers())
			},
			CreateFunc: func(e event.CreateEvent) bool {
				return true
			},
			DeleteFunc: func(e event.DeleteEvent) bool {
				return true
			},
		})).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: r.maxConcurrentReconciles,
		}).
		Watches(&corev1.Event{},
			handler.EnqueueRequestsFromMapFunc(r.mapEventToRepository),
			builder.WithPredicates(predicate.Funcs{
				UpdateFunc: func(e event.UpdateEvent) bool {
					newEvent := e.ObjectNew.(*corev1.Event)
					return newEvent.Source.Component == "porch-server" && 
						(strings.HasPrefix(newEvent.Reason, "SyncCompleted") || newEvent.Reason == "SyncFailed")
				},
				CreateFunc: func(e event.CreateEvent) bool {
					event := e.Object.(*corev1.Event)
					return event.Source.Component == "porch-server" && 
						(strings.HasPrefix(event.Reason, "SyncCompleted") || event.Reason == "SyncFailed")
				},
				DeleteFunc: func(e event.DeleteEvent) bool {
					return false // Never reconcile on event deletion
				},
			})).
		Named("repository").
		Build(r)

	if err == nil {
		log.Log.V(1).Info("Repository controller successfully registered")
	}
	return err
}
