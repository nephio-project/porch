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
	"flag"
	"time"

	cachetypes "github.com/nephio-project/porch/pkg/cache/types"
)

// InitDefaults initializes default values for standalone controller
func (r *RepositoryReconciler) InitDefaults() {
	// Controller behavior defaults
	r.MaxConcurrentReconciles = 100 // High: reconcile is lightweight
	r.MaxConcurrentSyncs = 50       // Limited by git cache locks (per-repo serialization)
	r.HealthCheckFrequency = 5 * time.Minute
	r.FullSyncFrequency = 1 * time.Hour
	r.SyncStaleTimeout = 20 * time.Minute
	r.RepoOperationRetryAttempts = 3
	// Cache type for standalone mode
	r.cacheType = string(cachetypes.DBCacheType)
	// Validate configuration
	r.validateConfig()
}

// EmbeddedConfig holds configuration for embedded controller mode
type EmbeddedConfig struct {
	MaxConcurrentReconciles    int
	MaxConcurrentSyncs         int
	HealthCheckFrequency       time.Duration
	FullSyncFrequency          time.Duration
	RepoOperationRetryAttempts int
}

// SetEmbeddedDefaults sets configuration for embedded controller mode (cache already injected)
// Zero values in config will be replaced with internal defaults during validation
func (r *RepositoryReconciler) SetEmbeddedDefaults(config EmbeddedConfig) {
	r.MaxConcurrentReconciles = config.MaxConcurrentReconciles
	r.MaxConcurrentSyncs = config.MaxConcurrentSyncs
	r.HealthCheckFrequency = config.HealthCheckFrequency
	r.FullSyncFrequency = config.FullSyncFrequency
	r.RepoOperationRetryAttempts = config.RepoOperationRetryAttempts
	// Validate and apply defaults for zero values
	r.validateConfig()
}

// DefaultEmbeddedConfig returns default configuration for embedded mode
func DefaultEmbeddedConfig() EmbeddedConfig {
	return EmbeddedConfig{
		MaxConcurrentReconciles:    100,
		MaxConcurrentSyncs:         50,
		HealthCheckFrequency:       5 * time.Minute,
		FullSyncFrequency:          1 * time.Hour,
		RepoOperationRetryAttempts: 3,
	}
}

// BindFlags binds controller-specific command line flags
func (r *RepositoryReconciler) BindFlags(prefix string, flags *flag.FlagSet) {
	flags.IntVar(&r.MaxConcurrentReconciles, prefix+"max-concurrent-reconciles", 100, "Maximum number of concurrent repository reconciles")
	flags.IntVar(&r.MaxConcurrentSyncs, prefix+"max-concurrent-syncs", 50, "Maximum number of concurrent sync operations (limited by per-repo git cache locks)")
	flags.StringVar(&r.cacheType, prefix+"cache-type", string(cachetypes.DBCacheType), "Cache type (DB or CR)")
	flags.DurationVar(&r.HealthCheckFrequency, prefix+"health-check-frequency", 5*time.Minute, "Frequency of repository health checks (connectivity verification)")
	flags.DurationVar(&r.FullSyncFrequency, prefix+"full-sync-frequency", 1*time.Hour, "Frequency of full repository sync if Spec.Sync.Schedule is not specified")
	flags.DurationVar(&r.SyncStaleTimeout, prefix+"sync-stale-timeout", 20*time.Minute, "Timeout for considering a sync stale")
	flags.IntVar(&r.RepoOperationRetryAttempts, prefix+"repo-operation-retry-attempts", 3, "Number of retry attempts for git operations (fetch, push, delete)")
	flags.BoolVar(&r.useUserDefinedCaBundle, prefix+"use-user-defined-ca-bundle", false, "Enable custom CA bundle support from secrets")
}

// validateConfig ensures configuration values are valid
func (r *RepositoryReconciler) validateConfig() {
	// Ensure HealthCheckFrequency is not zero to prevent infinite loops
	if r.HealthCheckFrequency <= 0 {
		r.HealthCheckFrequency = 5 * time.Minute
	}
	// Ensure FullSyncFrequency is not zero
	if r.FullSyncFrequency <= 0 {
		r.FullSyncFrequency = 1 * time.Hour
	}
	// Ensure SyncStaleTimeout is reasonable
	if r.SyncStaleTimeout <= 0 {
		r.SyncStaleTimeout = 20 * time.Minute
	}
	// Ensure concurrency limits are positive
	if r.MaxConcurrentReconciles <= 0 {
		r.MaxConcurrentReconciles = 10
	}
	if r.MaxConcurrentSyncs <= 0 {
		r.MaxConcurrentSyncs = 50
	}
	// Ensure retry attempts is positive
	if r.RepoOperationRetryAttempts <= 0 {
		r.RepoOperationRetryAttempts = 3
	}
}
