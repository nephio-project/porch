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
	kptfile "github.com/nephio-project/porch/pkg/kpt/api/kptfile/v1"
	"github.com/nephio-project/porch/pkg/repository"
	"github.com/nephio-project/porch/pkg/util"
	"go.opentelemetry.io/otel/trace"
)

type editPackageMutation struct {
	pkgRev            *api.PackageRevision
	task              *api.Task
	repoOpener        repository.RepositoryOpener
	referenceResolver repository.ReferenceResolver
}

var _ mutation = &editPackageMutation{}

func (m *editPackageMutation) apply(ctx context.Context, resources repository.PackageResources) (repository.PackageResources, *api.TaskResult, error) {
	ctx, span := tracer.Start(ctx, "editPackageMutation::apply", trace.WithAttributes())
	defer span.End()

	sourceRef := m.task.Edit.Source
	newPkgRev := m.pkgRev

	existingPkgRev, err := (&repository.PackageFetcher{
		RepoOpener:        m.repoOpener,
		ReferenceResolver: m.referenceResolver,
	}).FetchRevision(ctx, sourceRef, newPkgRev.Namespace)
	if err != nil {
		return repository.PackageResources{}, nil, fmt.Errorf("failed to fetch package %q: %w", sourceRef.Name, err)
	}

	// We only allow edit to create new revision from the same package.
	if existingPkgRev.Key().PkgKey.ToPkgPathname() != newPkgRev.Spec.PackageName ||
		existingPkgRev.Key().PkgKey.RepoKey.Name != newPkgRev.Spec.RepositoryName {
		return repository.PackageResources{}, nil, fmt.Errorf(
			"source revision must be from same package %s/%s (got: %s/%s)",
			existingPkgRev.Key().PkgKey.RepoKey.Name,
			existingPkgRev.Key().PkgKey.ToPkgPathname(),
			newPkgRev.Spec.RepositoryName,
			newPkgRev.Spec.PackageName)
	}

	// We only allow edit to create new revisions from published packages.
	if !api.LifecycleIsPublished(existingPkgRev.Lifecycle(ctx)) {
		return repository.PackageResources{}, nil, fmt.Errorf("source revision must be published")
	}

	sourceResources, err := existingPkgRev.GetResources(ctx)
	if err != nil {
		return repository.PackageResources{}, nil, fmt.Errorf("cannot read contents of package %q: %w", sourceRef.Name, err)
	}

	editedResources := repository.PackageResources{
		Contents: sourceResources.Spec.Resources,
	}
	editedResources.EditKptfile(func(file *kptfile.KptFile) {
		file.Status = &kptfile.Status{
			Conditions: func() (inputConditions []kptfile.Condition) {
				if file.Status == nil || file.Status.Conditions == nil {
					inputConditions = kptfile.ConvertApiConditions(DefaultReadinessConditions)
				} else {
					inputConditions = file.Status.Conditions
				}

				return util.MergeFunc(inputConditions, kptfile.ConvertApiConditions(newPkgRev.Status.Conditions), func(inputCondition, oldCondition kptfile.Condition) bool {
					return oldCondition.Type == inputCondition.Type
				})
			}(),
		}

		file.Info.ReadinessGates = func() (kptfileGates []kptfile.ReadinessGate) {
			for _, each := range DefaultReadinessConditions {
				kptfileGates = append(kptfileGates, kptfile.ReadinessGate{
					ConditionType: each.Type,
				})
			}
			return
		}()
		file.Info.ReadinessGates = func() (kptfileGates []kptfile.ReadinessGate) {
			for _, each := range newPkgRev.Status.Conditions {
				kptfileGates = append(kptfileGates, kptfile.ReadinessGate{
					ConditionType: each.Type,
				})
			}
			return util.Merge(file.Info.ReadinessGates, kptfileGates)
		}()
	})

	return editedResources, &api.TaskResult{Task: m.task}, nil
}
