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

package packagerevision

import "flag"

const (
	defaultMaxConcurrentReconciles    = 50
	defaultMaxConcurrentRenders       = 20
	defaultRepoOperationRetryAttempts = 3
)

func (r *PackageRevisionReconciler) InitDefaults() {
	r.MaxConcurrentReconciles = defaultMaxConcurrentReconciles
	r.MaxConcurrentRenders = defaultMaxConcurrentRenders
	r.RepoOperationRetryAttempts = defaultRepoOperationRetryAttempts
}

func (r *PackageRevisionReconciler) BindFlags(prefix string, flags *flag.FlagSet) {
	flags.IntVar(&r.MaxConcurrentReconciles, prefix+"max-concurrent-reconciles", defaultMaxConcurrentReconciles, "Maximum number of concurrent PackageRevision reconciles")
	flags.IntVar(&r.MaxConcurrentRenders, prefix+"max-concurrent-renders", defaultMaxConcurrentRenders, "Maximum number of concurrent renders (0 = unbounded)")
	flags.IntVar(&r.RepoOperationRetryAttempts, prefix+"repo-operation-retry-attempts", defaultRepoOperationRetryAttempts, "Number of retry attempts for git operations")
}
