// Copyright 2022, 2024-2025 The kpt and Nephio Authors
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
	"github.com/nephio-project/porch/pkg/repository"
	"github.com/nephio-project/porch/third_party/GoogleContainerTools/kpt-functions-sdk/go/fn"
	"go.opentelemetry.io/otel/trace"
	"k8s.io/klog/v2"
)

type editPackageMutation struct {
	task              *api.Task
	namespace         string
	repositoryName    string
	packageName       string
	repoOpener        repository.RepositoryOpener
	referenceResolver repository.ReferenceResolver
}

var _ mutation = &editPackageMutation{}

func (m *editPackageMutation) apply(ctx context.Context, resources repository.PackageResources) (repository.PackageResources, *api.TaskResult, error) {
	ctx, span := tracer.Start(ctx, "editPackageMutation::apply", trace.WithAttributes())
	defer span.End()

	sourceRef := m.task.Edit.Source

	revision, err := (&repository.PackageFetcher{
		RepoOpener:        m.repoOpener,
		ReferenceResolver: m.referenceResolver,
	}).FetchRevision(ctx, sourceRef, m.namespace)
	if err != nil {
		return repository.PackageResources{}, nil, fmt.Errorf("failed to fetch package %q: %w", sourceRef.Name, err)
	}

	// We only allow edit to create new revision from the same package.
	if revision.Key().PkgKey.ToPkgPathname() != m.packageName ||
		revision.Key().PkgKey.RKey().Name != m.repositoryName {
		return repository.PackageResources{}, nil, fmt.Errorf("source revision must be from same package %s/%s", m.repositoryName, m.packageName)
	}

	// We only allow edit to create new revisions from published packages.
	if !api.LifecycleIsPublished(revision.Lifecycle(ctx)) {
		return repository.PackageResources{}, nil, fmt.Errorf("source revision must be published")
	}

	sourceResources, err := revision.GetResources(ctx)
	if err != nil {
		return repository.PackageResources{}, nil, fmt.Errorf("cannot read contents of package %q: %w", sourceRef.Name, err)
	}

	// We erase any labels that may have come in with the copied Kptfile
	if kptfile, err := fn.NewKptfileFromPackage(sourceResources.Spec.Resources); err == nil {
		if _, err := kptfile.Obj.RemoveNestedField("metadata", "labels"); err != nil {
			klog.Errorf("error removing metadata.labels from resources' Kptfile: %q", err)
		}
		if err := kptfile.WriteToPackage(sourceResources.Spec.Resources); err != nil {
			klog.Errorf("error writing Kptfile back to resources: %q", err)
		}
	} else {
		klog.Warningf("unable to get resources' Kptfile to remove labels: %q", err)
	}

	return repository.PackageResources{
		Contents: sourceResources.Spec.Resources,
	}, &api.TaskResult{Task: m.task}, nil
}
