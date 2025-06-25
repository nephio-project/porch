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

package engine

import (
	"context"
	"fmt"

	"github.com/nephio-project/porch/api/porch/v1alpha1"
	"github.com/nephio-project/porch/pkg/repository"
	"go.opentelemetry.io/otel/trace"
	"k8s.io/klog/v2"
)

func PushPackageRevision(ctx context.Context, repo repository.Repository, pr repository.PackageRevision) error {
	ctx, span := tracer.Start(ctx, "PushPackageRevision", trace.WithAttributes())
	defer span.End()

	prLifecycle := pr.Lifecycle(ctx)
	if prLifecycle != v1alpha1.PackageRevisionLifecyclePublished {
		return fmt.Errorf("cannot push package revision %+v, package revision lifecycle is %q, it should be \"Published\"", pr.Key(), prLifecycle)
	}

	apiPr, err := pr.GetPackageRevision(ctx)
	if err != nil {
		return err
	}

	resources, err := pr.GetResources(ctx)
	if err != nil {
		return err
	}

	draft, err := repo.CreatePackageRevisionDraft(ctx, apiPr)
	if err != nil {
		return err
	}

	if err = draft.UpdateResources(ctx, resources, nil); err != nil {
		return err
	}

	if err = draft.UpdateLifecycle(ctx, v1alpha1.PackageRevisionLifecyclePublished); err != nil {
		return err
	}

	if _, err = repo.ClosePackageRevisionDraft(ctx, draft, pr.Key().Revision); err != nil {
		return err
	}

	klog.Infof("PushPackageRevision: package %v pushed to main", pr.Key())
	return nil
}
