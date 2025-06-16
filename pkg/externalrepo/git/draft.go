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

package git

import (
	"context"
	"time"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/nephio-project/porch/api/porch/v1alpha1"
	"github.com/nephio-project/porch/pkg/repository"
	"go.opentelemetry.io/otel/trace"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type gitPackageRevisionDraft struct {
	prKey    repository.PackageRevisionKey
	repo     *gitRepository // repo is repo containing the package
	metadata metav1.ObjectMeta
	updated  time.Time
	tasks    []v1alpha1.Task

	// New value of the package revision lifecycle
	lifecycle v1alpha1.PackageRevisionLifecycle

	// ref to the base of the package update commit chain (used for conditional push)
	base *plumbing.Reference

	// name of the branch where the changes will be pushed
	branch BranchName

	// Current HEAD of the package changes (commit sha)
	commit plumbing.Hash

	// Cached tree of the package itself, some descendent of commit.Tree()
	tree plumbing.Hash
}

var _ repository.PackageRevisionDraft = &gitPackageRevisionDraft{}

func (d *gitPackageRevisionDraft) Key() repository.PackageRevisionKey {
	return d.prKey
}

func (d *gitPackageRevisionDraft) GetMeta() metav1.ObjectMeta {
	return d.metadata
}

func (d *gitPackageRevisionDraft) GetRepo() repository.Repository {
	return d.repo
}

func (d *gitPackageRevisionDraft) UpdateResources(ctx context.Context, new *v1alpha1.PackageRevisionResources, changeTask *v1alpha1.Task) error {
	ctx, span := tracer.Start(ctx, "gitPackageDraft::UpdateResources", trace.WithAttributes())
	defer span.End()

	return d.repo.UpdateDraftResources(ctx, d, new, changeTask)
}

func (d *gitPackageRevisionDraft) UpdateLifecycle(ctx context.Context, new v1alpha1.PackageRevisionLifecycle) error {
	d.lifecycle = new
	return nil
}
