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

import (
	"flag"
	"fmt"
	"os"

	"github.com/kptdev/kpt/pkg/lib/runneroptions"
	"github.com/nephio-project/porch/pkg/cache/contentcache"
	"github.com/nephio-project/porch/pkg/engine"
	porch "github.com/nephio-project/porch/pkg/registry/porch"
	ctrl "sigs.k8s.io/controller-runtime"
)

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

// Init wires runtime dependencies (credential resolvers, renderer)
// that require the manager. ContentCache must be set before calling Init.
func (r *PackageRevisionReconciler) Init(mgr ctrl.Manager) error {
	log := ctrl.Log.WithName(r.Name())
	log.Info("PackageRevision controller configuration",
		"maxConcurrentReconciles", r.MaxConcurrentReconciles,
		"maxConcurrentRenders", r.MaxConcurrentRenders,
		"repoOperationRetryAttempts", r.RepoOperationRetryAttempts,
	)

	coreClient := mgr.GetClient()
	credResolver := porch.NewCredentialResolver(coreClient, []porch.Resolver{
		porch.NewBasicAuthResolver(),
		porch.NewBearerTokenAuthResolver(),
	})
	caBundleResolver := porch.NewCredentialResolver(coreClient, []porch.Resolver{
		porch.NewCaBundleResolver(),
	})
	r.ExternalPackageFetcher = contentcache.NewExternalPackageFetcher(
		credResolver, caBundleResolver, r.RepoOperationRetryAttempts,
	)

	if fnRunnerAddr := os.Getenv("FUNCTION_RUNNER_ADDRESS"); fnRunnerAddr != "" {
		rt, err := engine.NewMultiFunctionRuntime(fnRunnerAddr, 6*1024*1024, "")
		if err != nil {
			return fmt.Errorf("failed to create function runtime: %w", err)
		}
		opts := runneroptions.RunnerOptions{}
		prefix := os.Getenv("DEFAULT_IMAGE_PREFIX")
		if prefix == "" {
			prefix = runneroptions.GHCRImagePrefix
		}
		opts.InitDefaults(prefix)
		r.Renderer = newKptRenderer(rt, opts)
		ctrl.Log.WithName(r.Name()).Info("function runtime enabled", "address", fnRunnerAddr)
	} else {
		ctrl.Log.WithName(r.Name()).Info("function runtime not configured — rendering disabled")
	}
	return nil
}
