// Copyright 2022, 2024 The kpt and Nephio Authors
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

var _ mutation = &updatePackageMutation{}

type updatePackageMutation struct {
	cloneTask         *api.Task
	updateTask        *api.Task
	repoOpener        repository.RepositoryOpener
	referenceResolver repository.ReferenceResolver
	namespace         string
	pkgName           string
}

func (m *updatePackageMutation) apply(ctx context.Context, resources repository.PackageResources) (repository.PackageResources, *api.TaskResult, error) {
	ctx, span := tracer.Start(ctx, "updatePackageMutation::Apply", trace.WithAttributes())
	defer span.End()

	currUpstreamPkgRef, err := m.currUpstream()
	if err != nil {
		return repository.PackageResources{}, nil, err
	}

	targetUpstream := m.updateTask.Update.Upstream
	if targetUpstream.Type == api.RepositoryTypeGit || targetUpstream.Type == api.RepositoryTypeOCI {
		return repository.PackageResources{}, nil, fmt.Errorf("update is not supported for non-porch upstream packages")
	}

	originalResources, err := (&repository.PackageFetcher{
		RepoOpener:        m.repoOpener,
		ReferenceResolver: m.referenceResolver,
	}).FetchResources(ctx, currUpstreamPkgRef, m.namespace)
	if err != nil {
		return repository.PackageResources{}, nil, fmt.Errorf("error fetching the resources for package %s with ref %+v",
			m.pkgName, *currUpstreamPkgRef)
	}

	upstreamRevision, err := (&repository.PackageFetcher{
		RepoOpener:        m.repoOpener,
		ReferenceResolver: m.referenceResolver,
	}).FetchRevision(ctx, targetUpstream.UpstreamRef, m.namespace)
	if err != nil {
		return repository.PackageResources{}, nil, fmt.Errorf("error fetching revision for target upstream %s", targetUpstream.UpstreamRef.Name)
	}
	upstreamResources, err := upstreamRevision.GetResources(ctx)
	if err != nil {
		return repository.PackageResources{}, nil, fmt.Errorf("error fetching resources for target upstream %s", targetUpstream.UpstreamRef.Name)
	}

	klog.Infof("performing pkg upgrade operation for pkg %s resource counts local[%d] original[%d] upstream[%d]",
		m.pkgName, len(resources.Contents), len(originalResources.Spec.Resources), len(upstreamResources.Spec.Resources))

	// May be have packageUpdater part of the Porch core to make it easy for testing ?
	updatedResources, err := (&repository.DefaultPackageUpdater{}).Update(ctx,
		resources,
		repository.PackageResources{
			Contents: originalResources.Spec.Resources,
		},
		repository.PackageResources{
			Contents: upstreamResources.Spec.Resources,
		})
	if err != nil {
		return repository.PackageResources{}, nil, fmt.Errorf("error updating the package to revision %s", targetUpstream.UpstreamRef.Name)
	}

	newUpstream, newUpstreamLock, err := upstreamRevision.GetLock()
	if err != nil {
		return repository.PackageResources{}, nil, fmt.Errorf("error fetching the resources for package revisions %s", targetUpstream.UpstreamRef.Name)
	}
	if err := kpt.UpdateKptfileUpstream("", updatedResources.Contents, newUpstream, newUpstreamLock); err != nil {
		return repository.PackageResources{}, nil, fmt.Errorf("failed to apply upstream lock to package %q: %w", m.pkgName, err)
	}

	// ensure merge-key comment is added to newly added resources.
	result, err := ensureMergeKey(ctx, updatedResources)
	if err != nil {
		klog.Infof("failed to add merge key comments: %v", err)
	}
	return result, &api.TaskResult{Task: m.updateTask}, nil
}

// Currently assumption is that downstream packages will be forked from a porch package.
// As per current implementation, upstream package ref is stored in a new update task but this may
// change so the logic of figuring out current upstream will live in this function.
func (m *updatePackageMutation) currUpstream() (*api.PackageRevisionRef, error) {
	if m.cloneTask == nil || m.cloneTask.Clone == nil {
		return nil, fmt.Errorf("package %s does not have original upstream info", m.pkgName)
	}
	upstream := m.cloneTask.Clone.Upstream
	if upstream.Type == api.RepositoryTypeGit || upstream.Type == api.RepositoryTypeOCI {
		return nil, fmt.Errorf("upstream package must be porch native package. Found it to be %s", upstream.Type)
	}
	return upstream.UpstreamRef, nil
}

func findCloneTask(pr *api.PackageRevision) *api.Task {
	if len(pr.Spec.Tasks) == 0 {
		return nil
	}
	firstTask := pr.Spec.Tasks[0]
	if firstTask.Type == api.TaskTypeClone {
		return &firstTask
	}
	return nil
}
