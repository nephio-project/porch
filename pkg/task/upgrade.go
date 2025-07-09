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

	api "github.com/nephio-project/porch/api/porch/v1alpha1"
	"github.com/nephio-project/porch/pkg/kpt"
	"github.com/nephio-project/porch/pkg/repository"
	pkgerrors "github.com/pkg/errors"
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

func (m *upgradePackageMutation) apply(ctx context.Context, _ repository.PackageResources) (repository.PackageResources, *api.TaskResult, error) {
	ctx, span := tracer.Start(ctx, "upgradePackageMutation::apply", trace.WithAttributes())
	defer span.End()

	currUpstreamPkgRef := m.upgradeTask.Upgrade.OldUpstream
	targetUpstreamRef := m.upgradeTask.Upgrade.NewUpstream
	localRef := m.upgradeTask.Upgrade.LocalPackageRevisionRef

	packageFetcher := &repository.PackageFetcher{
		RepoOpener:        m.repoOpener,
		ReferenceResolver: m.referenceResolver,
	}

	currUpstreamResources, err := packageFetcher.FetchResources(ctx, &currUpstreamPkgRef, m.namespace)
	if err != nil {
		return repository.PackageResources{}, nil, pkgerrors.Wrapf(err, "error fetching the resources for package %q with ref %+v",
			m.pkgName, currUpstreamPkgRef)
	}

	targetUpstreamRevision, err := packageFetcher.FetchRevision(ctx, &targetUpstreamRef, m.namespace)
	if err != nil {
		return repository.PackageResources{}, nil, pkgerrors.Wrapf(err, "error fetching revision for target upstream %q", targetUpstreamRef.Name)
	}
	targetUpstreamResources, err := targetUpstreamRevision.GetResources(ctx)
	if err != nil {
		return repository.PackageResources{}, nil, pkgerrors.Wrapf(err, "error fetching resources for target upstream %q", targetUpstreamRef.Name)
	}

	localResources, err := packageFetcher.FetchResources(ctx, &localRef, m.namespace)
	if err != nil {
		return repository.PackageResources{}, nil, pkgerrors.Wrapf(err, "error fetching resources for local revision %q", localRef.Name)
	}

	klog.Infof("performing pkg upgrade operation for pkg %s resource counts local[%d] original[%d] upstream[%d]",
		m.pkgName, len(localResources.Spec.Resources), len(currUpstreamResources.Spec.Resources), len(targetUpstreamResources.Spec.Resources))

	//TODO: May be have packageUpdater part of the Porch core to make it easy for testing ?
	updatedResources, err := (&repository.DefaultPackageUpdater{}).Update(ctx,
		repository.PackageResources{
			Contents: localResources.Spec.Resources,
		},
		repository.PackageResources{
			Contents: currUpstreamResources.Spec.Resources,
		},
		repository.PackageResources{
			Contents: targetUpstreamResources.Spec.Resources,
		},
		string(m.upgradeTask.Upgrade.Strategy))
	if err != nil {
		return repository.PackageResources{}, nil, pkgerrors.Wrapf(err, "error updating the package %q to revision %q", m.pkgName, targetUpstreamRef.Name)
	}

	newUpstream, newUpstreamLock, err := targetUpstreamRevision.GetLock()
	if err != nil {
		return repository.PackageResources{}, nil, pkgerrors.Wrapf(err, "error fetching the resources for package revision %q", targetUpstreamRef.Name)
	}
	if err := kpt.UpdateKptfileUpstream("", updatedResources.Contents, newUpstream, newUpstreamLock); err != nil {
		return repository.PackageResources{}, nil, pkgerrors.Wrapf(err, "failed to apply upstream lock to package %q", m.pkgName)
	}

	// ensure merge-key comment is added to newly added resources.
	result, err := ensureMergeKey(ctx, updatedResources)
	if err != nil {
		klog.Infof("failed to add merge key comments: %v", err)
	}
	return result, &api.TaskResult{Task: m.upgradeTask}, nil
}
