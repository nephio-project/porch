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
	"testing"
	"time"
)

func TestInitDefaults(t *testing.T) {
	r := &RepositoryReconciler{}
	r.InitDefaults()

	if r.MaxConcurrentReconciles != 100 {
		t.Errorf("Expected MaxConcurrentReconciles 100, got %d", r.MaxConcurrentReconciles)
	}
	if r.MaxConcurrentSyncs != 50 {
		t.Errorf("Expected MaxConcurrentSyncs 50, got %d", r.MaxConcurrentSyncs)
	}
}

func TestBindFlags(t *testing.T) {
	r := &RepositoryReconciler{}
	flags := flag.NewFlagSet("test", flag.ContinueOnError)
	
	r.BindFlags("repo-", flags)
	
	// Parse test flags
	err := flags.Parse([]string{
		"--repo-max-concurrent-reconciles=100",
	})
	if err != nil {
		t.Fatalf("Failed to parse flags: %v", err)
	}

	if r.MaxConcurrentReconciles != 100 {
		t.Errorf("Expected MaxConcurrentReconciles 100, got %d", r.MaxConcurrentReconciles)
	}
}

func TestSetEmbeddedDefaults(t *testing.T) {
	tests := []struct {
		name   string
		config EmbeddedConfig
		want   EmbeddedConfig
	}{
		{
			name: "all values set",
			config: EmbeddedConfig{
				MaxConcurrentReconciles:    200,
				MaxConcurrentSyncs:         100,
				HealthCheckFrequency:       10 * time.Minute,
				FullSyncFrequency:          2 * time.Hour,
				RepoOperationRetryAttempts: 5,
			},
			want: EmbeddedConfig{
				MaxConcurrentReconciles:    200,
				MaxConcurrentSyncs:         100,
				HealthCheckFrequency:       10 * time.Minute,
				FullSyncFrequency:          2 * time.Hour,
				RepoOperationRetryAttempts: 5,
			},
		},
		{
			name:   "zero values get defaults",
			config: EmbeddedConfig{},
			want: EmbeddedConfig{
				MaxConcurrentReconciles:    10,
				MaxConcurrentSyncs:         50,
				HealthCheckFrequency:       5 * time.Minute,
				FullSyncFrequency:          1 * time.Hour,
				RepoOperationRetryAttempts: 3,
			},
		},
		{
			name: "negative values get defaults",
			config: EmbeddedConfig{
				MaxConcurrentReconciles:    -1,
				MaxConcurrentSyncs:         -1,
				HealthCheckFrequency:       -1 * time.Minute,
				FullSyncFrequency:          -1 * time.Hour,
				RepoOperationRetryAttempts: -1,
			},
			want: EmbeddedConfig{
				MaxConcurrentReconciles:    10,
				MaxConcurrentSyncs:         50,
				HealthCheckFrequency:       5 * time.Minute,
				FullSyncFrequency:          1 * time.Hour,
				RepoOperationRetryAttempts: 3,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &RepositoryReconciler{}
			r.SetEmbeddedDefaults(tt.config)

			if r.MaxConcurrentReconciles != tt.want.MaxConcurrentReconciles {
				t.Errorf("MaxConcurrentReconciles = %d, want %d", r.MaxConcurrentReconciles, tt.want.MaxConcurrentReconciles)
			}
			if r.MaxConcurrentSyncs != tt.want.MaxConcurrentSyncs {
				t.Errorf("MaxConcurrentSyncs = %d, want %d", r.MaxConcurrentSyncs, tt.want.MaxConcurrentSyncs)
			}
			if r.HealthCheckFrequency != tt.want.HealthCheckFrequency {
				t.Errorf("HealthCheckFrequency = %v, want %v", r.HealthCheckFrequency, tt.want.HealthCheckFrequency)
			}
			if r.FullSyncFrequency != tt.want.FullSyncFrequency {
				t.Errorf("FullSyncFrequency = %v, want %v", r.FullSyncFrequency, tt.want.FullSyncFrequency)
			}
			if r.RepoOperationRetryAttempts != tt.want.RepoOperationRetryAttempts {
				t.Errorf("RepoOperationRetryAttempts = %d, want %d", r.RepoOperationRetryAttempts, tt.want.RepoOperationRetryAttempts)
			}
		})
	}
}

func TestDefaultEmbeddedConfig(t *testing.T) {
	config := DefaultEmbeddedConfig()

	if config.MaxConcurrentReconciles != 100 {
		t.Errorf("MaxConcurrentReconciles = %d, want 100", config.MaxConcurrentReconciles)
	}
	if config.MaxConcurrentSyncs != 50 {
		t.Errorf("MaxConcurrentSyncs = %d, want 50", config.MaxConcurrentSyncs)
	}
	if config.HealthCheckFrequency != 5*time.Minute {
		t.Errorf("HealthCheckFrequency = %v, want 5m", config.HealthCheckFrequency)
	}
	if config.FullSyncFrequency != 1*time.Hour {
		t.Errorf("FullSyncFrequency = %v, want 1h", config.FullSyncFrequency)
	}
	if config.RepoOperationRetryAttempts != 3 {
		t.Errorf("RepoOperationRetryAttempts = %d, want 3", config.RepoOperationRetryAttempts)
	}
}