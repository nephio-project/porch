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

package fake

import (
	"context"

	"github.com/nephio-project/porch/api/porch/v1alpha1"
	"github.com/nephio-project/porch/pkg/repository"
)

// Implementation of the repository.Repository interface for testing.
// TODO(mortent): Implement stub functionality for all functions from the interface.
type Repository struct {
	key              repository.RepositoryKey
	PackageRevisions []repository.PackageRevision
	Packages         []repository.Package
	CurrentVersion   string
}

var _ repository.Repository = &Repository{}

func (r *Repository) KubeObjectNamespace() string {
	return r.Key().Namespace
}

func (r *Repository) KubeObjectName() string {
	return r.Key().Name
}

func (r *Repository) Key() repository.RepositoryKey {
	return r.key
}

func (r *Repository) Close(context.Context) error {
	return nil
}

func (r *Repository) Version(ctx context.Context) (string, error) {
	return r.CurrentVersion, nil
}

func (r *Repository) ListPackageRevisions(_ context.Context, filter repository.ListPackageRevisionFilter) ([]repository.PackageRevision, error) {
	var revs []repository.PackageRevision
	for _, rev := range r.PackageRevisions {
		if filter.Key.Matches(rev.Key()) {
			revs = append(revs, rev)
		}
	}
	return revs, nil
}

func (r *Repository) CreatePackageRevisionDraft(_ context.Context, pr *v1alpha1.PackageRevision) (repository.PackageRevisionDraft, error) {
	return &FakePackageRevision{}, nil
}

func (r *Repository) ClosePackageRevisionDraft(ctx context.Context, prd repository.PackageRevisionDraft, version int) (repository.PackageRevision, error) {
	return nil, nil
}

func (r *Repository) DeletePackageRevision(context.Context, repository.PackageRevision) error {
	return nil
}

func (r *Repository) UpdatePackageRevision(context.Context, repository.PackageRevision) (repository.PackageRevisionDraft, error) {
	return nil, nil
}

func (r *Repository) ListPackages(context.Context, repository.ListPackageFilter) ([]repository.Package, error) {
	return r.Packages, nil
}

func (r *Repository) CreatePackage(_ context.Context, pr *v1alpha1.PorchPackage) (repository.Package, error) {
	return nil, nil
}

func (r *Repository) DeletePackage(_ context.Context, pr repository.Package) error {
	return nil
}

func (r *Repository) Refresh(_ context.Context) error {
	return nil
}
