// Copyright 2024 The Nephio Authors
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

package db

import (
	"context"
	"time"

	"github.com/nephio-project/porch/api/porch/v1alpha1"
	"github.com/nephio-project/porch/pkg/repository"
	"go.opentelemetry.io/otel/trace"
)

var _ repository.PackageDraft = &dbPackageDraft{}

type dbPackageDraft struct {
	repo          *dbRepository
	packageName   string
	revision      string
	lifecycle     v1alpha1.PackageRevisionLifecycle
	updated       time.Time
	updatedBy     string
	workspaceName v1alpha1.WorkspaceName
	tasks         []v1alpha1.Task
	resources     map[string]string
}

var _ repository.PackageDraft = &dbPackageDraft{}

func (d *dbPackageDraft) UpdateResources(ctx context.Context, new *v1alpha1.PackageRevisionResources, change *v1alpha1.Task) error {
	d.resources = new.Spec.Resources
	return nil
}

func (d *dbPackageDraft) UpdateLifecycle(ctx context.Context, new v1alpha1.PackageRevisionLifecycle) error {
	d.lifecycle = new
	return nil
}

// Finish round of updates.
func (d *dbPackageDraft) Close(ctx context.Context) (repository.PackageRevision, error) {
	ctx, span := tracer.Start(ctx, "dbPackageDraft::Close", trace.WithAttributes())
	defer span.End()

	return d.repo.CloseDraft(ctx, d)
}
