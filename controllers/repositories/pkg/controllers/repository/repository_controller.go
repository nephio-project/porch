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
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	api "github.com/nephio-project/porch/controllers/repositories/api/v1alpha1"
	cachetypes "github.com/nephio-project/porch/pkg/cache/types"
	"github.com/nephio-project/porch/pkg/cache/util"
)

const (
	RepositoryFinalizer = "config.porch.kpt.dev/repository"
)

// SyncType defines the type of sync operation
type SyncType int

const (
	HealthCheck SyncType = iota // Lightweight connectivity check (like old background.go)
	FullSync                    // Complete data synchronization (like old SyncManager)
)

// RepositoryReconciler reconciles Repository objects
type RepositoryReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Cache  cachetypes.Cache // Injected cache instance

	// Public configuration - accessible for testing and injection
	HealthCheckFrequency       time.Duration // How often to check repository connectivity
	FullSyncFrequency          time.Duration // How often to perform full sync (if no schedule)
	MaxConcurrentReconciles    int
	MaxConcurrentSyncs         int           // Limit concurrent sync goroutines
	SyncStaleTimeout           time.Duration // How long before sync is considered stale
	RepoOperationRetryAttempts int           // Git operation retry attempts

	// Private implementation details
	cacheType              string        // Only cache type, will nedd DB details for dbcache
	useUserDefinedCaBundle bool          // Whether to use custom CA bundles from secrets
	syncLimiter            chan struct{} // Semaphore for sync concurrency
}

//go:generate go run sigs.k8s.io/controller-tools/cmd/controller-gen@v0.19.0 rbac:headerFile=../../../../../scripts/boilerplate.yaml.txt,roleName=porch-controllers-repositories webhook paths="." output:rbac:artifacts:config=../../../config/rbac

//+kubebuilder:rbac:groups=config.porch.kpt.dev,resources=repositories,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=config.porch.kpt.dev,resources=repositories/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=config.porch.kpt.dev,resources=repositories/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=events,verbs=get;list;watch

// Reconcile handles Repository reconciliation - replaces SyncManager functionality
func (r *RepositoryReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	log.Info("Repository reconcile triggered", "repository", req.NamespacedName)

	// Check if cache is available - this should never happen if SetupWithManager succeeded
	if r.Cache == nil {
		log.Error(nil, "Cache not available - controller initialization failed")
		return ctrl.Result{}, fmt.Errorf("cache not available - controller not properly initialized")
	}

	// Get Repository
	repo := &api.Repository{}
	if err := r.Get(ctx, req.NamespacedName, repo); err != nil {
		log.V(1).Info("Repository not found, likely deleted", "repository", req.NamespacedName)
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Handle deletion
	if !repo.DeletionTimestamp.IsZero() {
		log.Info("Repository deletion detected", "repository", repo.Name)
		return r.handleDeletion(ctx, repo)
	}

	// Ensure finalizer - Update will trigger reconcile automatically
	if _, err := r.ensureFinalizer(ctx, repo); err != nil {
		return ctrl.Result{}, err
	}

	// Determine what operation is needed
	decision := r.determineSyncDecision(ctx, repo)
	log.V(1).Info("Sync decision", "repository", repo.Name, "type", decision.Type, "needed", decision.Needed)

	if !decision.Needed {
		log.V(1).Info("No operation needed, requeuing", "repository", repo.Name, "after", decision.Interval)
		return ctrl.Result{RequeueAfter: decision.Interval}, nil
	}

	if decision.Type == HealthCheck {
		// Don't run health check if full sync is in progress (prevents concurrent status updates)
		if r.isSyncInProgress(ctx, repo) {
			log.V(1).Info("Skipping health check, full sync in progress", "repository", repo.Name)
			return ctrl.Result{RequeueAfter: r.HealthCheckFrequency}, nil
		}
		return r.performHealthCheckSync(ctx, repo)
	}

	return r.performFullSync(ctx, repo)
}

// ensureFinalizer adds finalizer if missing using patch to avoid generation increment
func (r *RepositoryReconciler) ensureFinalizer(ctx context.Context, repo *api.Repository) (bool, error) {
	if controllerutil.ContainsFinalizer(repo, RepositoryFinalizer) {
		return false, nil
	}
	log.FromContext(ctx).Info("Adding finalizer to repository", "repository", repo.Name)
	patch := client.MergeFrom(repo.DeepCopy())
	controllerutil.AddFinalizer(repo, RepositoryFinalizer)
	if err := r.Patch(ctx, repo, patch); err != nil {
		return false, err
	}
	return true, nil
}

// performHealthCheckSync executes synchronous health check
func (r *RepositoryReconciler) performHealthCheckSync(ctx context.Context, repo *api.Repository) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	log.Info("Starting repository health check", "repository", repo.Name)

	// Lightweight connectivity check only - don't call OpenRepository which can block
	if err := r.Cache.CheckRepositoryConnectivity(ctx, repo); err != nil {
		log.Error(err, "Repository health check failed", "repository", repo.Name)
		retryInterval := r.determineRetryInterval(err)
		// Apply minimum retry interval to prevent aggressive retries
		const minRetryInterval = 30 * time.Second
		if retryInterval < minRetryInterval {
			retryInterval = minRetryInterval
		}
		nextSyncTime := time.Now().Add(retryInterval)
		r.updateRepoStatusWithBackoff(ctx, repo, util.RepositoryStatusError, err, &nextSyncTime)
		return ctrl.Result{RequeueAfter: retryInterval}, nil
	}

	// Health check passed
	log.Info("Repository health check completed successfully", "repository", repo.Name)
	nextHealthCheck := time.Now().Add(r.HealthCheckFrequency)
	r.updateRepoStatusWithBackoff(ctx, repo, util.RepositoryStatusReady, nil, &nextHealthCheck)
	return ctrl.Result{RequeueAfter: r.HealthCheckFrequency}, nil
}

// performFullSync executes asynchronous full sync
func (r *RepositoryReconciler) performFullSync(ctx context.Context, repo *api.Repository) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	log.Info("Starting repository full sync", "repository", repo.Name)

	// Set sync in progress status
	if err := r.updateRepoStatusWithBackoff(ctx, repo, util.RepositoryStatusSyncInProgress, nil, nil); err != nil {
		return ctrl.Result{}, err
	}

	// Perform sync with concurrency limiting
	select {
	case r.syncLimiter <- struct{}{}:
		go func() {
			defer func() {
				<-r.syncLimiter
				// Recover from panics to prevent goroutine death
				if panicErr := recover(); panicErr != nil {
					log.Error(fmt.Errorf("panic in async sync: %v", panicErr), "Panic recovered", "repository", repo.Name)
					// Update status to error on panic
					panicCtx := context.Background()
					retryInterval := 5 * time.Minute
					nextSyncTime := time.Now().Add(retryInterval)
					r.updateRepoStatusWithBackoff(panicCtx, repo, util.RepositoryStatusError, fmt.Errorf("repo sync panic: %v", panicErr), &nextSyncTime)
				}
			}()
			// Use background context for async operation (prevents cancellation)
			asyncCtx := context.Background()
			r.performAsyncSync(asyncCtx, repo)
		}()
	default:
		// Too many syncs running
		r.updateRepoStatusWithBackoff(ctx, repo, util.RepositoryStatusError, fmt.Errorf("sync capacity exceeded"), nil)
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// Requeue for stale detection (use stale timeout)
	staleTimeout := r.getSyncStaleTimeout()
	return ctrl.Result{RequeueAfter: staleTimeout}, nil
}

// performAsyncSync performs repository sync in background goroutine
func (r *RepositoryReconciler) performAsyncSync(ctx context.Context, repo *api.Repository) {
	log := log.FromContext(ctx)

	// syncRepository calls OpenRepository which does connectivity check + fetch with retries
	err := r.syncRepository(ctx, repo)

	var status util.RepositoryStatus
	var nextSyncTime *time.Time
	if err == nil {
		status = util.RepositoryStatusReady
		log.Info("Repository full sync completed successfully", "repository", repo.Name)
		// Calculate next sync time
		next := time.Now().Add(r.calculateNextSyncInterval(repo))
		nextSyncTime = &next
		// Record full sync timestamp
		if annotErr := r.setLastFullSyncTime(ctx, repo); annotErr != nil {
			log.Error(annotErr, "Failed to set last full sync time", "repository", repo.Name)
		}
		// Clear one-time sync flag after successful sync
		if clearErr := r.clearOneTimeSyncFlag(ctx, repo); clearErr != nil {
			log.Error(clearErr, "Failed to clear one-time sync flag", "repository", repo.Name)
		}
	} else {
		status = util.RepositoryStatusError
		log.Error(err, "Repository sync failed", "repository", repo.Name)
		// Use error-specific retry interval for sync failures
		retryInterval := r.determineRetryInterval(err)
		next := time.Now().Add(retryInterval)
		nextSyncTime = &next
	}

	// Update status with result and next sync time
	r.updateRepoStatusWithBackoff(ctx, repo, status, err, nextSyncTime)
}

// GetSyncLimiter returns the sync limiter channel for testing
func (r *RepositoryReconciler) GetSyncLimiter() chan struct{} {
	return r.syncLimiter
}

// InitializeSyncLimiter initializes the sync limiter for testing
func (r *RepositoryReconciler) InitializeSyncLimiter() {
	if r.MaxConcurrentSyncs <= 0 {
		r.MaxConcurrentSyncs = 100 // Default limit
	}
	r.syncLimiter = make(chan struct{}, r.MaxConcurrentSyncs)
}

// SetupWithManager sets up the controller with the Manager
func (r *RepositoryReconciler) SetupWithManager(mgr ctrl.Manager) error {
	log := ctrl.Log.WithName("repository-controller")
	log.Info("SetupWithManager called", "reconcilerPtr", fmt.Sprintf("%p", r), "cacheIsNil", r.Cache == nil)

	// Initialize sync limiter
	r.InitializeSyncLimiter()
	log.Info("Sync limiter initialized", "maxConcurrentSyncs", r.MaxConcurrentSyncs)

	// Create cache if not already injected (standalone mode)
	if r.Cache == nil {
		// Create cache with type only - cache reads DB/global config from environment
		if err := r.createCacheFromEnv(context.Background(), mgr); err != nil {
			return fmt.Errorf("failed to create cache: %w", err)
		}
		log.Info("Cache created in SetupWithManager", "reconcilerPtr", fmt.Sprintf("%p", r))
	}

	// Set client explicitly before controller setup
	r.Client = mgr.GetClient()
	log.Info("Client injected", "reconcilerPtr", fmt.Sprintf("%p", r))

	// Watch Repository CRs only
	err := ctrl.NewControllerManagedBy(mgr).
		For(&api.Repository{}, builder.WithPredicates(predicate.Funcs{
			UpdateFunc: func(e event.UpdateEvent) bool {
				// Only trigger on user-initiated spec changes
				return e.ObjectNew.GetGeneration() != e.ObjectOld.GetGeneration()
			},
			CreateFunc: func(e event.CreateEvent) bool {
				return true
			},
			DeleteFunc: func(e event.DeleteEvent) bool {
				return true
			},
		})).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: r.MaxConcurrentReconciles,
		}).
		Named("repository").
		Complete(r)

	if err == nil {
		log.V(1).Info("Repository controller successfully registered")
	}
	return err
}
