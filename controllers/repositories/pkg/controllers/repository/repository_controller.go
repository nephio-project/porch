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
	"runtime/debug"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
)

const (
	RepositoryFinalizer = "config.porch.kpt.dev/repository"
)

// SyncType defines the type of sync operation
type SyncType int

const (
	HealthCheck SyncType = iota // Lightweight connectivity check
	FullSync                    // Complete data synchronization
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

	// Configuration (set via flags or defaults)
	cacheType              string // Cache type (DB or CR)
	cacheDirectory         string // Directory for git repository cache
	useUserDefinedCaBundle bool   // Whether to use custom CA bundles from secrets

	// Private implementation details
	syncLimiter chan struct{} // Semaphore for sync concurrency
	loggerName  string        // Logger name for this reconciler
}

//go:generate go run sigs.k8s.io/controller-tools/cmd/controller-gen@v0.19.0 rbac:headerFile=../../../../../scripts/boilerplate.yaml.txt,roleName=porch-controllers-repositories webhook paths="." output:rbac:artifacts:config=../../../config/rbac

//+kubebuilder:rbac:groups=config.porch.kpt.dev,resources=repositories,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=config.porch.kpt.dev,resources=repositories/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=config.porch.kpt.dev,resources=repositories/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=events,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

// Reconcile handles Repository reconciliation
func (r *RepositoryReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	log.V(2).Info("Repository reconcile triggered")

	// Check if cache is available - this should never happen if SetupWithManager succeeded
	if r.Cache == nil {
		log.Error(nil, "Cache not available - controller initialization failed")
		return ctrl.Result{}, fmt.Errorf("cache not available - controller not properly initialized")
	}

	// Get Repository
	repo := &api.Repository{}
	if err := r.Get(ctx, req.NamespacedName, repo); err != nil {
		log.V(1).Info("Repository not found, likely deleted")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Handle deletion
	if !repo.DeletionTimestamp.IsZero() {
		log.Info("Repository deletion detected")
		return r.handleDeletion(ctx, repo)
	}

	// Ensure finalizer - Update will trigger reconcile automatically
	if _, err := r.ensureFinalizer(ctx, repo); err != nil {
		return ctrl.Result{}, err
	}

	// Determine what operation is needed
	decision := r.determineSyncDecision(ctx, repo)
	log.V(2).Info("Sync decision made", "type", decision.Type, "needed", decision.Needed, "interval", decision.Interval)

	// If status was modified (e.g., ObservedRunOnceAt), persist it using SSA
	if !decision.Needed {
		// Update status if it changed (e.g., ObservedRunOnceAt was set)
		if err := r.updateRepoStatusWithBackoff(ctx, repo, RepositoryStatusReady, nil, nil); err != nil {
			log.Error(err, "Failed to update repository status")
			return ctrl.Result{}, err
		}
		
		// Defense in depth: Ensure we never return 0 requeue interval
		// Primary mitigation is in calculateNextSyncInterval() which uses HealthCheckFrequency as floor
		// This should never trigger in normal operation - if it does, indicates a bug
		if decision.Interval <= 0 {
			decision.Interval = 1 * time.Second
			log.Info("Requeue interval was zero or negative, using 1s minimum")
		}
		log.V(2).Info("No operation needed, requeuing", "after", decision.Interval)
		return ctrl.Result{RequeueAfter: decision.Interval}, nil
	}

	if decision.Type == HealthCheck {
		// Don't run health check if full sync is in progress (prevents concurrent status updates)
		if r.isSyncInProgress(ctx, repo) {
			log.V(1).Info("Skipping health check, full sync in progress")
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
	log.FromContext(ctx).Info("Adding finalizer to repository")
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
	log.V(2).Info("Starting repository health check")

	// Check if repo was previously in error state
	wasInError := false
	for _, condition := range repo.Status.Conditions {
		if condition.Type == api.RepositoryReady && condition.Status == metav1.ConditionFalse && condition.Reason == api.ReasonError {
			wasInError = true
			break
		}
	}

	// Lightweight connectivity check only - don't call OpenRepository which can block
	if err := r.Cache.CheckRepositoryConnectivity(ctx, repo); err != nil {
		repoURL, _, _ := getRepoFields(repo)
		log.Error(err, "Repository health check failed", "repoURL", repoURL)
		retryInterval := r.determineRetryInterval(err)
		// Apply minimum retry interval to prevent aggressive retries
		const minRetryInterval = 30 * time.Second
		if retryInterval < minRetryInterval {
			retryInterval = minRetryInterval
		}
		nextSyncTime := time.Now().Add(retryInterval)
		if statusErr := r.updateRepoStatusWithBackoff(ctx, repo, RepositoryStatusError, err, &nextSyncTime); statusErr != nil {
			log.Error(statusErr, "Failed to update repository status after health check failure")
		}
		return ctrl.Result{RequeueAfter: retryInterval}, nil
	}

	// Health check passed - if repo was in error, trigger immediate full sync to catch up
	if wasInError {
		log.Info("Repository recovered from error state, triggering immediate full sync")
		// Clear error status first
		nextHealthCheck := time.Now().Add(r.HealthCheckFrequency)
		if statusErr := r.updateRepoStatusWithBackoff(ctx, repo, RepositoryStatusReady, nil, &nextHealthCheck); statusErr != nil {
			log.Error(statusErr, "Failed to update repository status after error recovery")
		}
		// Trigger full sync immediately
		return r.performFullSync(ctx, repo)
	}

	// Normal health check passed
	log.V(2).Info("Repository health check completed successfully")
	nextFullSync := r.calculateNextFullSyncTime(repo)
	if statusErr := r.updateRepoStatusWithBackoff(ctx, repo, RepositoryStatusReady, nil, &nextFullSync); statusErr != nil {
		log.Error(statusErr, "Failed to update repository status after successful health check")
	}
	// Always requeue after HealthCheckFrequency - don't try to calculate based on potentially stale data
	return ctrl.Result{RequeueAfter: r.HealthCheckFrequency}, nil
}

// performFullSync executes asynchronous full sync
func (r *RepositoryReconciler) performFullSync(ctx context.Context, repo *api.Repository) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	log.V(2).Info("Starting repository full sync")

	// Set sync in progress status
	if err := r.updateRepoStatusWithBackoff(ctx, repo, RepositoryStatusSyncInProgress, nil, nil); err != nil {
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
					retryInterval := 5 * time.Minute
					panicErr := fmt.Errorf("panic in async sync: %v\nStack: %s", panicErr, debug.Stack())
					log.Error(panicErr, "Panic recovered, will retry", "retryAfter", retryInterval)
					// Update status to error on panic
					panicCtx := ctrl.LoggerInto(context.Background(), log)
					nextSyncTime := time.Now().Add(retryInterval)
					statusErr := fmt.Errorf("repo sync panic: %v, retrying after %v",
						panicErr, retryInterval)
					if err := r.updateRepoStatusWithBackoff(panicCtx, repo,
						RepositoryStatusError, statusErr, &nextSyncTime); err != nil {
						log.Error(err, "Failed to update repository status after panic recovery")
					}
				}
			}()
			// Use background context for async operation (prevents cancellation)
			asyncCtx := ctrl.LoggerInto(context.Background(), log)
			// Make a copy to avoid race conditions with caller's repo object
			repoCopy := repo.DeepCopy()
			r.performAsyncSync(asyncCtx, repoCopy)
		}()
	default:
		// Too many syncs running
		retryAfter := 30 * time.Second
		log.V(0).Info("Sync capacity exceeded, will retry",
			"retryAfter", retryAfter,
			"maxConcurrentSyncs", r.MaxConcurrentSyncs)
		statusErr := fmt.Errorf("sync capacity exceeded, retrying after %v", retryAfter)
		if err := r.updateRepoStatusWithBackoff(ctx, repo,
			RepositoryStatusError, statusErr, nil); err != nil {
			log.Error(err, "Failed to update repository status after sync capacity exceeded")
		}
		return ctrl.Result{RequeueAfter: retryAfter}, nil
	}

	// Requeue after health check frequency to allow health checks between full syncs
	// The decision logic will determine when next full sync is actually due
	log.V(2).Info("Full sync launched in background, requeuing for next health check", "requeueAfter", r.HealthCheckFrequency)
	return ctrl.Result{RequeueAfter: r.HealthCheckFrequency}, nil
}

// performAsyncSync performs repository sync in background goroutine
func (r *RepositoryReconciler) performAsyncSync(ctx context.Context, repo *api.Repository) {
	log := log.FromContext(ctx)

	packageCount, commitHash, err := r.syncRepository(ctx, repo)

	var status RepositoryStatus
	var nextSyncTime *time.Time
	if err == nil {
		status = RepositoryStatusReady
		// Determine sync trigger reason
		syncReason := "scheduled"
		if repo.Spec.Sync != nil && repo.Spec.Sync.RunOnceAt != nil {
			syncReason = "runOnceAt"
		} else if repo.Spec.Sync != nil && repo.Spec.Sync.Schedule != "" {
			syncReason = "cron:" + repo.Spec.Sync.Schedule
		}
		log.Info("Repository full sync completed successfully", "trigger", syncReason)
		// Update status fields
		now := metav1.Now()
		repo.Status.LastFullSyncTime = &now
		repo.Status.ObservedGeneration = repo.Generation
		// Update ObservedRunOnceAt if runOnceAt was set (will be cleared after status update)
		if repo.Spec.Sync != nil && repo.Spec.Sync.RunOnceAt != nil {
			repo.Status.ObservedRunOnceAt = repo.Spec.Sync.RunOnceAt
		} else if repo.Status.ObservedRunOnceAt != nil {
			// Clear ObservedRunOnceAt if runOnceAt is no longer set
			repo.Status.ObservedRunOnceAt = nil
		}
		repo.Status.PackageCount = packageCount
		repo.Status.GitCommitHash = commitHash
		// Calculate next sync time from current time
		next := r.calculateNextFullSyncTime(repo)
		nextSyncTime = &next
		repo.Status.NextFullSyncTime = &metav1.Time{Time: next}
	} else {
		status = RepositoryStatusError
		repoURL, _, _ := getRepoFields(repo)
		log.Error(err, "Repository sync failed", "repoURL", repoURL)
		retryInterval := r.determineRetryInterval(err)
		next := time.Now().Add(retryInterval)
		nextSyncTime = &next
	}

	// Update status with result and next sync time
	if statusErr := r.updateRepoStatusWithBackoff(ctx, repo, status, err, nextSyncTime); statusErr != nil {
		if client.IgnoreNotFound(statusErr) == nil {
			log.V(1).Info("Repository deleted during sync")
			return
		}
		log.Error(statusErr, "Failed to update repository status after sync")
		return // Don't clear runOnceAt if status update failed
	}

	// Clear one-time sync flag after successful sync
	if err == nil && repo.Spec.Sync != nil && repo.Spec.Sync.RunOnceAt != nil {
		if clearErr := r.clearOneTimeSyncFlag(ctx, repo); clearErr != nil && client.IgnoreNotFound(clearErr) != nil {
			log.Error(clearErr, "Failed to clear one-time sync flag (non-critical)")
		}
	}
}

// InitializeSyncLimiter initializes the sync limiter for testing
func (r *RepositoryReconciler) InitializeSyncLimiter() {
	if r.MaxConcurrentSyncs <= 0 {
		r.MaxConcurrentSyncs = 100 // Default limit
	}
	r.syncLimiter = make(chan struct{}, r.MaxConcurrentSyncs)
}

// SetLogger sets the logger name for this reconciler
func (r *RepositoryReconciler) SetLogger(name string) {
	r.loggerName = name
}

// getRepoFields extracts repository-specific fields for logging
func getRepoFields(repo *api.Repository) (repoURL, branch, directory string) {
	if repo.Spec.Git != nil {
		return repo.Spec.Git.Repo, repo.Spec.Git.Branch, repo.Spec.Git.Directory
	}
	if repo.Spec.Oci != nil {
		return repo.Spec.Oci.Registry, "", ""
	}
	return "", "", ""
}

// SetupWithManager sets up the controller with the Manager
func (r *RepositoryReconciler) SetupWithManager(mgr ctrl.Manager) error {
	log := ctrl.Log.WithName(r.loggerName)
	log.Info("SetupWithManager called", "reconcilerPtr", fmt.Sprintf("%p", r), "cacheIsNil", r.Cache == nil)

	// Log controller configuration
	r.LogConfig(log.V(0))

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
	// Note: Predicates only filter watch events, not requeues from RequeueAfter
	err := ctrl.NewControllerManagedBy(mgr).
		For(&api.Repository{}, builder.WithPredicates(predicate.Funcs{
			UpdateFunc: func(e event.UpdateEvent) bool {
				// Only trigger on user-initiated spec changes (Generation increment)
				// Status-only updates are filtered out to prevent reconciliation loops
				// Health checks and periodic syncs are triggered via RequeueAfter, not watch events
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
