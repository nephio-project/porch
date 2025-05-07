// Copyright 2025 The kpt and Nephio Authors
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

package task

import (
	"context"
	"fmt"

	api "github.com/nephio-project/porch/api/porch/v1alpha1"
	"github.com/nephio-project/porch/pkg/kpt"
	"github.com/nephio-project/porch/pkg/repository"
	"go.opentelemetry.io/otel/trace"
	"k8s.io/klog/v2"
)

var _ mutation = &upgradePackageMutation{}

type upgradePackageMutation struct {
	upgradeTask       *api.Task
	repoOpener        repository.RepositoryOpener
	referenceResolver repository.ReferenceResolver
	namespace         string
	pkgName           string
}

func (m *upgradePackageMutation) apply(ctx context.Context, resources repository.PackageResources) (repository.PackageResources, *api.TaskResult, error) {
	ctx, span := tracer.Start(ctx, "upgradePackageMutation::apply", trace.WithAttributes())
	defer span.End()

	currUpstreamPkgRef := m.upgradeTask.Upgrade.OldUpstream

	targetUpstreamRef := m.upgradeTask.Upgrade.NewUpstream

	packageFetcher := &repository.PackageFetcher{
		RepoOpener:        m.repoOpener,
		ReferenceResolver: m.referenceResolver,
	}

	originalResources, err := packageFetcher.FetchResources(ctx, &currUpstreamPkgRef, m.namespace)
	if err != nil {
		return repository.PackageResources{}, nil, fmt.Errorf("error fetching the resources for package %s with ref %+v",
			m.pkgName, currUpstreamPkgRef)
	}

	upstreamRevision, err := packageFetcher.FetchRevision(ctx, &targetUpstreamRef, m.namespace)
	if err != nil {
		return repository.PackageResources{}, nil, fmt.Errorf("error fetching revision for target upstream %s", targetUpstreamRef.Name)
	}
	upstreamResources, err := upstreamRevision.GetResources(ctx)
	if err != nil {
		return repository.PackageResources{}, nil, fmt.Errorf("error fetching resources for target upstream %s", targetUpstreamRef.Name)
	}

	klog.Infof("performing pkg upgrade operation for pkg %s resource counts local[%d] original[%d] upstream[%d]",
		m.pkgName, len(resources.Contents), len(originalResources.Spec.Resources), len(upstreamResources.Spec.Resources))

	//TODO: May be have packageUpdater part of the Porch core to make it easy for testing ?
	updatedResources, err := (&repository.DefaultPackageUpdater{}).Update(ctx,
		resources,
		repository.PackageResources{
			Contents: originalResources.Spec.Resources,
		},
		repository.PackageResources{
			Contents: upstreamResources.Spec.Resources,
		},
		string(m.upgradeTask.Upgrade.Strategy))
	if err != nil {
		return repository.PackageResources{}, nil, fmt.Errorf("error updating the package to revision %s", targetUpstreamRef.Name)
	}

	newUpstream, newUpstreamLock, err := upstreamRevision.GetLock()
	if err != nil {
		return repository.PackageResources{}, nil, fmt.Errorf("error fetching the resources for package revisions %s", targetUpstreamRef.Name)
	}
	if err := kpt.UpdateKptfileUpstream("", updatedResources.Contents, newUpstream, newUpstreamLock); err != nil {
		return repository.PackageResources{}, nil, fmt.Errorf("failed to apply upstream lock to package %q: %w", m.pkgName, err)
	}

	// ensure merge-key comment is added to newly added resources.
	result, err := ensureMergeKey(ctx, updatedResources)
	if err != nil {
		klog.Infof("failed to add merge key comments: %v", err)
	}
	return result, &api.TaskResult{Task: m.upgradeTask}, nil
}

func findUpgradeTask(pr *api.PackageRevision) *api.Task {
	if len(pr.Spec.Tasks) == 0 {
		return nil
	}
	if firstTask := pr.Spec.Tasks[0]; firstTask.Type == api.TaskTypeUpgrade {
		return &firstTask
	}
	return nil
}
