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

	if r.connectivityRetryInterval != 10*time.Second {
		t.Errorf("Expected connectivityRetryInterval 10s, got %v", r.connectivityRetryInterval)
	}
	if r.maxConcurrentReconciles != 50 {
		t.Errorf("Expected maxConcurrentReconciles 50, got %d", r.maxConcurrentReconciles)
	}
}

func TestBindFlags(t *testing.T) {
	r := &RepositoryReconciler{}
	flags := flag.NewFlagSet("test", flag.ContinueOnError)
	
	r.BindFlags("repo-", flags)
	
	// Parse test flags
	err := flags.Parse([]string{
		"--repo-connectivity-retry-interval=30s",
		"--repo-max-concurrent-reconciles=100",
	})
	if err != nil {
		t.Fatalf("Failed to parse flags: %v", err)
	}

	if r.connectivityRetryInterval != 30*time.Second {
		t.Errorf("Expected connectivityRetryInterval 30s, got %v", r.connectivityRetryInterval)
	}
	if r.maxConcurrentReconciles != 100 {
		t.Errorf("Expected maxConcurrentReconciles 100, got %d", r.maxConcurrentReconciles)
	}
}