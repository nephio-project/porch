package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	api "github.com/nephio-project/porch/controllers/repositories/api/v1alpha1"
	"github.com/nephio-project/porch/pkg/repository"
	"github.com/robfig/cron/v3"
)

// syncRepository performs repository synchronization
func (r *RepositoryReconciler) syncRepository(ctx context.Context, repo *api.Repository) error {
	log := log.FromContext(ctx)
	
	// Open repository - connectivity already checked in reconcile loop
	repoHandle, err := r.Cache.OpenRepository(ctx, repo)
	if err != nil {
		log.Error(err, "Failed to open repository for sync", "repository", repo.Name)
		return err
	}
	
	if err := repoHandle.Refresh(ctx); err != nil {
		log.Error(err, "Repository refresh failed", "repository", repo.Name)
		return err
	}
	
	_, err = repoHandle.ListPackageRevisions(ctx, repository.ListPackageRevisionFilter{})
	if err != nil {
		log.Error(err, "Repository package listing failed", "repository", repo.Name)
		return err
	}
	
	log.Info("Repository sync completed successfully", "repository", repo.Name)
	return nil
}

// SyncDecision represents what type of operation is needed
type SyncDecision struct {
	Type     SyncType
	Needed   bool
	Interval time.Duration // How long to wait before next check
}

// determineSyncDecision decides what operation is needed and when to requeue
func (r *RepositoryReconciler) determineSyncDecision(ctx context.Context, repo *api.Repository) SyncDecision {
	// Don't start new operations if already in progress
	if r.isSyncInProgress(ctx, repo) {
		return SyncDecision{Type: HealthCheck, Needed: false, Interval: r.getRequeueInterval(repo)}
	}

	// 1. One-time sync due → Full sync (check before spec change)
	if r.isOneTimeSyncDue(repo) {
		return SyncDecision{Type: FullSync, Needed: true, Interval: 0}
	}
	
	// 2. Spec changed → Full sync immediately (but not if RunOnceAt is set for future)
	if r.hasSpecChanged(repo) {
		// If RunOnceAt is set but not yet due, don't sync on spec change
		if repo.Spec.Sync != nil && repo.Spec.Sync.RunOnceAt != nil && time.Now().Before(repo.Spec.Sync.RunOnceAt.Time) {
			// Wait for scheduled time
			return SyncDecision{Type: HealthCheck, Needed: false, Interval: time.Until(repo.Spec.Sync.RunOnceAt.Time)}
		}
		return SyncDecision{Type: FullSync, Needed: true, Interval: 0}
	}
	
	// 3. Error retry due → Retry with full sync to ensure repository gets cached
	if r.isErrorRetryDue(repo) {
		return SyncDecision{Type: FullSync, Needed: true, Interval: 0}
	}
	
	// 4. Full sync due (check before health check - higher priority)
	if r.isFullSyncDue(repo) {
		return SyncDecision{Type: FullSync, Needed: true, Interval: 0}
	}
	
	// 5. Health check due (frequent)
	if r.isHealthCheckDue(repo) {
		return SyncDecision{Type: HealthCheck, Needed: true, Interval: 0}
	}
	
	// Nothing needed, return next check interval
	return SyncDecision{Type: HealthCheck, Needed: false, Interval: r.getRequeueInterval(repo)}
}

// isHealthCheckDue checks if health check is needed (every HealthCheckFrequency)
func (r *RepositoryReconciler) isHealthCheckDue(repo *api.Repository) bool {
	// Don't do health check if repository has error condition - wait for error retry
	for _, condition := range repo.Status.Conditions {
		if condition.Type == api.RepositoryReady && condition.Status == metav1.ConditionFalse && condition.Reason == api.ReasonError {
			return false
		}
	}
	
	// Get last operation time (any status update)
	lastUpdate := r.getLastStatusUpdateTime(repo)
	if lastUpdate.IsZero() {
		return true // Never checked
	}
	return time.Since(lastUpdate) >= r.HealthCheckFrequency
}

// getLastStatusUpdateTime gets the most recent status update time
func (r *RepositoryReconciler) getLastStatusUpdateTime(repo *api.Repository) time.Time {
	if len(repo.Status.Conditions) == 0 {
		return time.Time{}
	}
	// Return the most recent condition update time
	for _, condition := range repo.Status.Conditions {
		if condition.Type == api.RepositoryReady {
			return condition.LastTransitionTime.Time
		}
	}
	return time.Time{}
}

// isFullSyncDue checks if full sync is needed (less frequent than health checks)
func (r *RepositoryReconciler) isFullSyncDue(repo *api.Repository) bool {
	// Don't do full sync if RunOnceAt is set and pending (one-time sync takes precedence)
	if repo.Spec.Sync != nil && repo.Spec.Sync.RunOnceAt != nil && time.Now().Before(repo.Spec.Sync.RunOnceAt.Time) {
		return false
	}
	
	// Don't do full sync if repository has error condition
	for _, condition := range repo.Status.Conditions {
		if condition.Type == api.RepositoryReady && condition.Status == metav1.ConditionFalse && condition.Reason == api.ReasonError {
			return false
		}
	}
	
	lastFullSync := r.getLastFullSyncTime(repo)
	if lastFullSync.IsZero() {
		return true // Never did full sync
	}
	
	// If custom schedule exists, use it instead of default frequency
	if repo.Spec.Sync != nil && repo.Spec.Sync.Schedule != "" {
		schedule, err := cron.ParseStandard(repo.Spec.Sync.Schedule)
		if err != nil {
			log.FromContext(context.Background()).Error(err, "Invalid cron expression, using default frequency", "schedule", repo.Spec.Sync.Schedule)
			return time.Since(lastFullSync) >= r.FullSyncFrequency
		}
		next := schedule.Next(lastFullSync)
		return time.Now().After(next)
	}
	
	// Use default full sync frequency
	return time.Since(lastFullSync) >= r.FullSyncFrequency
}

// getLastFullSyncTime gets timestamp of last successful full sync
func (r *RepositoryReconciler) getLastFullSyncTime(repo *api.Repository) time.Time {
	// Check annotation first (explicit tracking)
	if repo.Annotations != nil {
		if lastSync, ok := repo.Annotations["config.porch.kpt.dev/last-full-sync"]; ok {
			if t, err := time.Parse(time.RFC3339, lastSync); err == nil {
				return t
			}
		}
	}
	// Fallback to condition timestamp for backward compatibility
	return r.getLastSyncTime(repo)
}

// isSyncInProgress checks if repository sync is currently in progress
func (r *RepositoryReconciler) isSyncInProgress(ctx context.Context, repo *api.Repository) bool {
	for _, condition := range repo.Status.Conditions {
		if condition.Type == api.RepositoryReady && condition.Status == metav1.ConditionFalse && condition.Reason == api.ReasonReconciling {
			// If sync has been "in progress" for more than the stale timeout, consider it stale
			staleTimeout := r.getSyncStaleTimeout()
			if time.Since(condition.LastTransitionTime.Time) > staleTimeout {
				log.FromContext(ctx).Info("Sync appears stale, will retry", "repository", repo.Name, "duration", time.Since(condition.LastTransitionTime.Time), "timeout", staleTimeout)
				return false // Allow new sync to start
			}
			return true
		}
	}
	return false
}

// getRequeueInterval determines appropriate requeue interval based on repository status
func (r *RepositoryReconciler) getRequeueInterval(repo *api.Repository) time.Duration {
	// Check if repository has error condition with retry timestamp
	for _, condition := range repo.Status.Conditions {
		if condition.Type == api.RepositoryReady && condition.Status == metav1.ConditionFalse && condition.Reason == api.ReasonError {
			// Extract retry time from status message if available
			if strings.Contains(condition.Message, "next retry at:") {
				start := strings.Index(condition.Message, "next retry at: ") + len("next retry at: ")
				end := strings.Index(condition.Message[start:], ")")
				if end != -1 {
					timestampStr := condition.Message[start : start+end]
					if retryTime, err := time.Parse(time.RFC3339, timestampStr); err == nil {
						duration := time.Until(retryTime)
						if duration > 0 {
							return duration
						}
						return 1 * time.Second // Retry immediately if time has passed
					}
				}
			}
			// Fallback: use error-specific retry interval from error message
			if condition.Message != "" {
				err := fmt.Errorf("%s", condition.Message)
				return r.determineRetryInterval(err)
			}
			return 30 * time.Second // Final fallback
		}
	}
	// No error condition - use normal sync schedule
	return r.calculateNextSyncInterval(repo)
}

// calculateNextSyncInterval determines when to requeue for next sync
func (r *RepositoryReconciler) calculateNextSyncInterval(repo *api.Repository) time.Duration {
	// If RunOnceAt is set, prioritize it
	if repo.Spec.Sync != nil && repo.Spec.Sync.RunOnceAt != nil {
		untilRunOnce := time.Until(repo.Spec.Sync.RunOnceAt.Time)
		if untilRunOnce > 0 {
			return untilRunOnce
		}
		// RunOnceAt time has passed, will be handled by isOneTimeSyncDue
		return 0
	}

	// Calculate time until next health check
	lastUpdate := r.getLastStatusUpdateTime(repo)
	nextHealthCheck := r.HealthCheckFrequency
	if !lastUpdate.IsZero() {
		nextHealthCheck = r.HealthCheckFrequency - time.Since(lastUpdate)
		if nextHealthCheck < 0 {
			nextHealthCheck = 0
		}
	}

	// Calculate time until next full sync
	lastFullSync := r.getLastFullSyncTime(repo)
	nextFullSync := r.FullSyncFrequency
	if !lastFullSync.IsZero() {
		nextFullSync = r.FullSyncFrequency - time.Since(lastFullSync)
		if nextFullSync < 0 {
			nextFullSync = 0
		}
	}

	// If custom schedule exists, calculate next scheduled sync
	if repo.Spec.Sync != nil && repo.Spec.Sync.Schedule != "" {
		schedule, err := cron.ParseStandard(repo.Spec.Sync.Schedule)
		if err != nil {
			log.FromContext(context.Background()).Error(err, "Invalid cron expression, ignoring schedule", "schedule", repo.Spec.Sync.Schedule)
		} else {
			lastSyncTime := r.getLastSyncTime(repo)
			if lastSyncTime.IsZero() {
				lastSyncTime = time.Now()
			}
			next := schedule.Next(lastSyncTime)
			nextScheduled := time.Until(next)
			if nextScheduled > 0 && nextScheduled < nextFullSync {
				nextFullSync = nextScheduled
			}
		}
	}

	// Return the minimum of health check and full sync intervals
	if nextHealthCheck < nextFullSync {
		return nextHealthCheck
	}
	return nextFullSync
}

// getLastSyncTime extracts last sync time from status conditions
func (r *RepositoryReconciler) getLastSyncTime(repo *api.Repository) time.Time {
	for _, condition := range repo.Status.Conditions {
		if condition.Type == api.RepositoryReady && condition.Status == metav1.ConditionTrue {
			return condition.LastTransitionTime.Time
		}
	}
	return time.Time{} // Zero time if never synced successfully
}

// getSyncStaleTimeout returns the timeout for considering a sync stale
func (r *RepositoryReconciler) getSyncStaleTimeout() time.Duration {
	if r.SyncStaleTimeout > 0 {
		return r.SyncStaleTimeout
	}
	// Default: 20 minutes to handle customer reports of 15-minute syncs
	return 20 * time.Minute
}

// isErrorRetryDue checks if error retry time has elapsed
func (r *RepositoryReconciler) isErrorRetryDue(repo *api.Repository) bool {
	for _, condition := range repo.Status.Conditions {
		if condition.Type == api.RepositoryReady && condition.Status == metav1.ConditionFalse && condition.Reason == api.ReasonError {
			// Extract retry time from status message
			if strings.Contains(condition.Message, "next retry at:") {
				// Parse timestamp from message like "error (next retry at: 2026-01-13T00:08:50Z)"
				start := strings.Index(condition.Message, "next retry at: ") + len("next retry at: ")
				end := strings.Index(condition.Message[start:], ")")
				if end != -1 {
					timestampStr := condition.Message[start : start+end]
					if retryTime, err := time.Parse(time.RFC3339, timestampStr); err == nil {
						return time.Now().After(retryTime)
					}
				}
			}
			// Fallback: use error-specific interval from condition timestamp
			if condition.Message != "" {
				err := fmt.Errorf("%s", condition.Message)
				retryInterval := r.determineRetryInterval(err)
				return time.Since(condition.LastTransitionTime.Time) >= retryInterval
			}
		}
	}
	return false
}

// isOneTimeSyncDue checks if one-time sync is scheduled and due
func (r *RepositoryReconciler) isOneTimeSyncDue(repo *api.Repository) bool {
	if repo.Spec.Sync == nil || repo.Spec.Sync.RunOnceAt == nil {
		return false
	}
	return time.Now().After(repo.Spec.Sync.RunOnceAt.Time)
}


// determineRetryInterval returns appropriate retry interval based on error type
func (r *RepositoryReconciler) determineRetryInterval(err error) time.Duration {
	errStr := strings.ToLower(err.Error())
	switch {
	case strings.Contains(errStr, "no such host"), strings.Contains(errStr, "connection refused"):
		return 30 * time.Second // Network issues - retry quickly
	case strings.Contains(errStr, "authentication"), strings.Contains(errStr, "permission denied"),
		 strings.Contains(errStr, "failed to resolve credentials"), strings.Contains(errStr, "resolved credentials are invalid"):
		return 10 * time.Minute // Auth/credential issues - retry less frequently
	case strings.Contains(errStr, "not found"), strings.Contains(errStr, "invalid"), strings.Contains(errStr, "branch"):
		return 2 * time.Minute // Git repo/branch issues - medium retry
	case strings.Contains(errStr, "timeout"), strings.Contains(errStr, "deadline exceeded"):
		return 1 * time.Minute // Timeout issues - retry moderately
	case strings.Contains(errStr, "certificate"), strings.Contains(errStr, "tls"), strings.Contains(errStr, "ssl"):
		return 5 * time.Minute // TLS/SSL issues - retry less frequently
	case strings.Contains(errStr, "rate limit"), strings.Contains(errStr, "too many requests"):
		return 5 * time.Minute // Rate limiting - back off
	default:
		return 30 * time.Second // Default retry interval for unknown errors
	}
}

// clearOneTimeSyncFlag clears the RunOnceAt flag after successful one-time sync using merge patch
func (r *RepositoryReconciler) clearOneTimeSyncFlag(ctx context.Context, repo *api.Repository) error {
	if repo.Spec.Sync == nil || repo.Spec.Sync.RunOnceAt == nil {
		return nil
	}

	patch := client.MergeFrom(repo.DeepCopy())
	repo.Spec.Sync.RunOnceAt = nil
	return r.Patch(ctx, repo, patch)
}

// setLastFullSyncTime records timestamp of last successful full sync in annotation
func (r *RepositoryReconciler) setLastFullSyncTime(ctx context.Context, repo *api.Repository) error {
	patch := client.MergeFrom(repo.DeepCopy())
	if repo.Annotations == nil {
		repo.Annotations = make(map[string]string)
	}
	repo.Annotations["config.porch.kpt.dev/last-full-sync"] = time.Now().Format(time.RFC3339)
	return r.Patch(ctx, repo, patch)
}
