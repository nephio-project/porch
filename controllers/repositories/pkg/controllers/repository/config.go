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
	"os"
	"time"

	cachetypes "github.com/nephio-project/porch/pkg/cache/types"
)

// InitDefaults initializes default values to match API server defaults
func (r *RepositoryReconciler) InitDefaults() {
	// Match background.go's reconnect behavior for connectivity retries
	r.connectivityRetryInterval = 10 * time.Second
	// Controller concurrency default
	r.maxConcurrentReconciles = 50
	// Cache defaults
	r.cacheType = string(cachetypes.DBCacheType)
	r.dbDriver = cachetypes.DefaultDBCacheDriver
	r.repoSyncFrequency = 60 * time.Second
}

// SetEmbeddedDefaults sets defaults for embedded controller mode (cache already injected)
func (r *RepositoryReconciler) SetEmbeddedDefaults() {
	// Match background.go's reconnect behavior for connectivity retries
	r.connectivityRetryInterval = 10 * time.Second
	// Controller concurrency default
	r.maxConcurrentReconciles = 50
	// Sync frequency
	r.repoSyncFrequency = 60 * time.Second
}

// BindFlags binds command line flags
func (r *RepositoryReconciler) BindFlags(prefix string, flags *flag.FlagSet) {
	flags.DurationVar(&r.connectivityRetryInterval, prefix+"connectivity-retry-interval", 10*time.Second, "Retry interval for connectivity failures")
	flags.IntVar(&r.maxConcurrentReconciles, prefix+"max-concurrent-reconciles", 50, "Maximum number of concurrent repository reconciles")
	flags.StringVar(&r.cacheType, prefix+"cache-type", string(cachetypes.DBCacheType), "Cache type (DB or CR)")
	flags.StringVar(&r.dbDriver, prefix+"db-driver", cachetypes.DefaultDBCacheDriver, "Database driver")
	flags.StringVar(&r.dbHost, prefix+"db-host", os.Getenv("DB_HOST"), "Database host")
	flags.StringVar(&r.dbPort, prefix+"db-port", os.Getenv("DB_PORT"), "Database port")
	flags.StringVar(&r.dbName, prefix+"db-name", os.Getenv("DB_NAME"), "Database name")
	flags.StringVar(&r.dbUser, prefix+"db-user", os.Getenv("DB_USER"), "Database user")
	flags.StringVar(&r.dbPassword, prefix+"db-password", os.Getenv("DB_PASSWORD"), "Database password")
	flags.DurationVar(&r.repoSyncFrequency, prefix+"repo-sync-frequency", 60*time.Second, "Repository sync frequency")
}