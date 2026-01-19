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

// syncRepository performs repository synchronization and returns sync metadata
func (r *RepositoryReconciler) syncRepository(ctx context.Context, repo *api.Repository) (packageCount int, commitHash string, err error) {
	log := log.FromContext(ctx)

	repoHandle, err := r.Cache.OpenRepository(ctx, repo)
	if err != nil {
		repoURL, _, _ := getRepoFields(repo)
		log.Error(err, "Failed to open repository for sync", "repoURL", repoURL)
		return 0, "", err
	}

	if err := repoHandle.Refresh(ctx); err != nil {
		repoURL, branch, _ := getRepoFields(repo)
		log.Error(err, "Repository refresh failed", "repoURL", repoURL, "branch", branch)
		return 0, "", err
	}

	pkgRevs, err := repoHandle.ListPackageRevisions(ctx, repository.ListPackageRevisionFilter{})
	if err != nil {
		repoURL, _, directory := getRepoFields(repo)
		log.Error(err, "Repository package listing failed", "repoURL", repoURL, "directory", directory)
		return 0, "", err
	}

	// Get commit hash (best effort - don't fail sync if this fails)
	commitHash, _ = repoHandle.BranchCommitHash(ctx)

	log.Info("Repository sync completed successfully", "packageCount", len(pkgRevs), "commitHash", commitHash)
	return len(pkgRevs), commitHash, nil
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
		// Use health check frequency to avoid tight loops while waiting for sync to complete
		return SyncDecision{Type: HealthCheck, Needed: false, Interval: r.HealthCheckFrequency}
	}

	// 1. One-time sync due → Full sync
	if r.isOneTimeSyncDue(repo) {
		return SyncDecision{Type: FullSync, Needed: true, Interval: 0}
	}

	// 2. Spec changed → Full sync immediately
	if r.hasSpecChanged(repo) {
		// If RunOnceAt is set but not yet due, wait for scheduled time
		if repo.Spec.Sync != nil && repo.Spec.Sync.RunOnceAt != nil && time.Now().Before(repo.Spec.Sync.RunOnceAt.Time) {
			untilRunOnce := time.Until(repo.Spec.Sync.RunOnceAt.Time)
			if untilRunOnce <= 0 {
				// Race condition: time passed while we were checking, run sync now
				return SyncDecision{Type: FullSync, Needed: true, Interval: 0}
			}
			return SyncDecision{Type: HealthCheck, Needed: false, Interval: untilRunOnce}
		}
		return SyncDecision{Type: FullSync, Needed: true, Interval: 0}
	}

	// 3. Error retry due → Health check (not full sync)
	if r.isErrorRetryDue(repo) {
		return SyncDecision{Type: HealthCheck, Needed: true, Interval: 0}
	}

	// 4. Full sync due → Full sync (only if not in error state)
	if r.isFullSyncDue(repo) {
		return SyncDecision{Type: FullSync, Needed: true, Interval: 0}
	}

	// 5. Health check due → Health check
	if r.isHealthCheckDue(repo) {
		return SyncDecision{Type: HealthCheck, Needed: true, Interval: 0}
	}

	// Nothing needed, return next check interval
	return SyncDecision{Type: HealthCheck, Needed: false, Interval: r.getRequeueInterval(repo)}
}

// isSyncInProgress checks if repository sync is currently in progress
func (r *RepositoryReconciler) isSyncInProgress(ctx context.Context, repo *api.Repository) bool {
	for _, condition := range repo.Status.Conditions {
		if condition.Type == api.RepositoryReady && condition.Status == metav1.ConditionFalse && condition.Reason == api.ReasonReconciling {
			staleTimeout := r.getSyncStaleTimeout()
			if time.Since(condition.LastTransitionTime.Time) > staleTimeout {
				repoURL, _, _ := getRepoFields(repo)
				log.FromContext(ctx).Info("Sync appears stale, will retry", "duration", time.Since(condition.LastTransitionTime.Time), "timeout", staleTimeout, "repoURL", repoURL)
				return false
			}
			return true
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

// isErrorRetryDue checks if error retry time has elapsed
func (r *RepositoryReconciler) isErrorRetryDue(repo *api.Repository) bool {
	for _, condition := range repo.Status.Conditions {
		if condition.Type == api.RepositoryReady && condition.Status == metav1.ConditionFalse && condition.Reason == api.ReasonError {
			if strings.Contains(condition.Message, "next retry at:") {
				start := strings.Index(condition.Message, "next retry at: ") + len("next retry at: ")
				end := strings.Index(condition.Message[start:], ")")
				if end != -1 {
					timestampStr := condition.Message[start : start+end]
					if retryTime, err := time.Parse(time.RFC3339, timestampStr); err == nil {
						return time.Now().After(retryTime)
					}
				}
			}
			if condition.Message != "" {
				err := fmt.Errorf("%s", condition.Message)
				retryInterval := r.determineRetryInterval(err)
				return time.Since(condition.LastTransitionTime.Time) >= retryInterval
			}
		}
	}
	return false
}

// isFullSyncDue checks if full sync is needed
func (r *RepositoryReconciler) isFullSyncDue(repo *api.Repository) bool {
	if repo.Spec.Sync != nil && repo.Spec.Sync.RunOnceAt != nil && time.Now().Before(repo.Spec.Sync.RunOnceAt.Time) {
		return false
	}

	// Don't attempt full sync if repository is in error state
	for _, condition := range repo.Status.Conditions {
		if condition.Type == api.RepositoryReady && condition.Status == metav1.ConditionFalse && condition.Reason == api.ReasonError {
			return false
		}
	}

	lastFullSync := r.getLastFullSyncTime(repo)
	if lastFullSync.IsZero() {
		log.FromContext(context.Background()).Info("Full sync due: no previous sync")
		return true
	}

	if repo.Spec.Sync != nil && repo.Spec.Sync.Schedule != "" {
		schedule, err := cron.ParseStandard(repo.Spec.Sync.Schedule)
		if err != nil {
			log.FromContext(context.Background()).Error(err, "Invalid cron expression, using default frequency", "schedule", repo.Spec.Sync.Schedule)
			return time.Since(lastFullSync) >= r.FullSyncFrequency
		}
		next := schedule.Next(lastFullSync)
		isDue := time.Now().After(next)
		log.FromContext(context.Background()).Info("Cron schedule check", "schedule", repo.Spec.Sync.Schedule, "lastFullSync", lastFullSync.Format(time.RFC3339), "nextScheduled", next.Format(time.RFC3339), "now", time.Now().Format(time.RFC3339), "isDue", isDue)
		return isDue
	}

	timeSinceLastSync := time.Since(lastFullSync)
	isDue := timeSinceLastSync >= r.FullSyncFrequency
	log.FromContext(context.Background()).Info("Full sync check", "lastFullSync", lastFullSync.Format(time.RFC3339), "timeSince", timeSinceLastSync, "frequency", r.FullSyncFrequency, "isDue", isDue)
	return isDue
}

// isHealthCheckDue checks if health check is needed
func (r *RepositoryReconciler) isHealthCheckDue(repo *api.Repository) bool {
	// Always perform health checks when in error state (handled by isErrorRetryDue)
	// This function only checks if routine health check is due for healthy repos
	for _, condition := range repo.Status.Conditions {
		if condition.Type == api.RepositoryReady && condition.Status == metav1.ConditionFalse && condition.Reason == api.ReasonError {
			return false
		}
	}

	lastUpdate := r.getLastStatusUpdateTime(repo)
	if lastUpdate.IsZero() {
		return true
	}
	return time.Since(lastUpdate) >= r.HealthCheckFrequency
}

// getLastStatusUpdateTime gets the most recent status update time
func (r *RepositoryReconciler) getLastStatusUpdateTime(repo *api.Repository) time.Time {
	if len(repo.Status.Conditions) == 0 {
		return time.Time{}
	}
	for _, condition := range repo.Status.Conditions {
		if condition.Type == api.RepositoryReady {
			return condition.LastTransitionTime.Time
		}
	}
	return time.Time{}
}

// getLastFullSyncTime gets timestamp of last successful full sync
func (r *RepositoryReconciler) getLastFullSyncTime(repo *api.Repository) time.Time {
	if repo.Status.LastFullSyncTime != nil {
		return repo.Status.LastFullSyncTime.Time
	}
	return time.Time{}
}

// getSyncStaleTimeout returns the timeout for considering a sync stale
func (r *RepositoryReconciler) getSyncStaleTimeout() time.Duration {
	if r.SyncStaleTimeout > 0 {
		return r.SyncStaleTimeout
	}
	return 20 * time.Minute
}

// getRequeueInterval determines appropriate requeue interval based on repository status
func (r *RepositoryReconciler) getRequeueInterval(repo *api.Repository) time.Duration {
	for _, condition := range repo.Status.Conditions {
		if condition.Type == api.RepositoryReady && condition.Status == metav1.ConditionFalse && condition.Reason == api.ReasonError {
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
						return 1 * time.Second
					}
				}
			}
			if condition.Message != "" {
				err := fmt.Errorf("%s", condition.Message)
				return r.determineRetryInterval(err)
			}
			return 30 * time.Second
		}
	}
	return r.calculateNextSyncInterval(repo)
}

// calculateNextSyncInterval determines when to requeue for next sync
func (r *RepositoryReconciler) calculateNextSyncInterval(repo *api.Repository) time.Duration {
	if repo.Spec.Sync != nil && repo.Spec.Sync.RunOnceAt != nil {
		untilRunOnce := time.Until(repo.Spec.Sync.RunOnceAt.Time)
		if untilRunOnce > 0 {
			return untilRunOnce
		}
	}

	// If no status exists yet, use health check frequency to avoid tight loops
	lastUpdate := r.getLastStatusUpdateTime(repo)
	if lastUpdate.IsZero() {
		return r.HealthCheckFrequency
	}

	nextHealthCheck := r.HealthCheckFrequency - time.Since(lastUpdate)

	lastFullSync := r.getLastFullSyncTime(repo)
	nextFullSync := r.FullSyncFrequency
	if !lastFullSync.IsZero() {
		nextFullSync = r.FullSyncFrequency - time.Since(lastFullSync)
	}

	if repo.Spec.Sync != nil && repo.Spec.Sync.Schedule != "" {
		schedule, err := cron.ParseStandard(repo.Spec.Sync.Schedule)
		if err != nil {
			log.FromContext(context.Background()).Error(err, "Invalid cron expression, ignoring schedule", "schedule", repo.Spec.Sync.Schedule)
		} else {
			lastSyncTime := r.getLastFullSyncTime(repo)
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

// determineRetryInterval returns appropriate retry interval based on error type
func (r *RepositoryReconciler) determineRetryInterval(err error) time.Duration {
	errStr := strings.ToLower(err.Error())
	switch {
	case strings.Contains(errStr, "no such host"), strings.Contains(errStr, "connection refused"):
		return 30 * time.Second
	case strings.Contains(errStr, "authentication"), strings.Contains(errStr, "permission denied"),
		strings.Contains(errStr, "failed to resolve credentials"), strings.Contains(errStr, "resolved credentials are invalid"):
		return 10 * time.Minute
	case strings.Contains(errStr, "not found"), strings.Contains(errStr, "invalid"), strings.Contains(errStr, "branch"):
		return 2 * time.Minute
	case strings.Contains(errStr, "timeout"), strings.Contains(errStr, "deadline exceeded"):
		return 1 * time.Minute
	case strings.Contains(errStr, "certificate"), strings.Contains(errStr, "tls"), strings.Contains(errStr, "ssl"):
		return 5 * time.Minute
	case strings.Contains(errStr, "rate limit"), strings.Contains(errStr, "too many requests"):
		return 5 * time.Minute
	default:
		return 30 * time.Second
	}
}

// clearOneTimeSyncFlag clears the RunOnceAt flag after successful one-time sync
func (r *RepositoryReconciler) clearOneTimeSyncFlag(ctx context.Context, repo *api.Repository) error {
	if repo.Spec.Sync == nil || repo.Spec.Sync.RunOnceAt == nil {
		return nil
	}

	patch := client.MergeFrom(repo.DeepCopy())
	repo.Spec.Sync.RunOnceAt = nil
	return r.Patch(ctx, repo, patch)
}

// calculateNextFullSyncTime calculates when the next full sync should occur
// This must match the logic in isFullSyncDue to show accurate status
func (r *RepositoryReconciler) calculateNextFullSyncTime(repo *api.Repository) time.Time {
	if repo.Spec.Sync != nil && repo.Spec.Sync.Schedule != "" {
		if schedule, err := cron.ParseStandard(repo.Spec.Sync.Schedule); err == nil {
			lastSync := r.getLastFullSyncTime(repo)
			if lastSync.IsZero() {
				lastSync = time.Now()
			}
			// Use same logic as isFullSyncDue: next run after last sync
			return schedule.Next(lastSync)
		}
	}
	return time.Now().Add(r.FullSyncFrequency)
}
