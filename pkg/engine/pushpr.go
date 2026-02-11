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

	kptfilev1 "github.com/kptdev/kpt/pkg/api/kptfile/v1"
	porchapi "github.com/nephio-project/porch/api/porch/v1alpha1"
	"github.com/nephio-project/porch/pkg/repository"
	pkgerrors "github.com/pkg/errors"
	"go.opentelemetry.io/otel/trace"
	"k8s.io/klog/v2"
)

func PushPackageRevision(ctx context.Context, repo repository.Repository, pr repository.PackageRevision) (kptfilev1.UpstreamLock, error) {
	ctx, span := tracer.Start(ctx, "PushPackageRevision", trace.WithAttributes())
	defer span.End()

	prLifecycle := pr.Lifecycle(ctx)
	if prLifecycle != porchapi.PackageRevisionLifecyclePublished {
		return kptfilev1.UpstreamLock{}, fmt.Errorf("cannot push package revision %+v, package revision lifecycle is %q, it should be \"Published\"", pr.Key(), prLifecycle)
	}

	apiPr, err := pr.GetPackageRevision(ctx)
	if err != nil {
		return kptfilev1.UpstreamLock{}, pkgerrors.Wrapf(err, "push of package revision %+v to repository %+v failed, could not get API definition:", pr.Key(), repo.Key())
	}

	resources, err := pr.GetResources(ctx)
	if err != nil {
		return kptfilev1.UpstreamLock{}, pkgerrors.Wrapf(err, "push of package revision %+v to repository %+v failed, could not get package revision resources:", pr.Key(), repo.Key())
	}

	draft, err := repo.CreatePackageRevisionDraft(ctx, apiPr)
	if err != nil {
		return kptfilev1.UpstreamLock{}, pkgerrors.Wrapf(err, "push of package revision %+v to repository %+v failed, could not create package revision draft:", pr.Key(), repo.Key())
	}

	commitTask := &porchapi.Task{Type: porchapi.TaskTypePush}
	if len(apiPr.Spec.Tasks) > 0 {
		commitTask = &apiPr.Spec.Tasks[0]
	}

	if err = draft.UpdateResources(ctx, resources, commitTask); err != nil {
		return kptfilev1.UpstreamLock{}, pkgerrors.Wrapf(err, "push of package revision %+v to repository %+v failed, could not update package revision resources:", pr.Key(), repo.Key())
	}

	if err = draft.UpdateLifecycle(ctx, porchapi.PackageRevisionLifecyclePublished); err != nil {
		return kptfilev1.UpstreamLock{}, pkgerrors.Wrapf(err, "push of package revision %+v to repository %+v failed, could not update package revision draft lifecycle to \"Published\":", pr.Key(), repo.Key())
	}

	pushedPR, err := repo.ClosePackageRevisionDraft(ctx, draft, pr.Key().Revision)
	if err != nil {
		return kptfilev1.UpstreamLock{}, pkgerrors.Wrapf(err, "push of package revision %+v to repository %+v failed, could not close package revision draft:", pr.Key(), repo.Key())
	}

	_, pushedPRUpstreamLock, err := pushedPR.GetLock(ctx)
	if err != nil {
		return kptfilev1.UpstreamLock{}, pkgerrors.Wrapf(err, "read of upstream lock for package revision %+v pushed to repository %+v failed", pr.Key(), repo.Key())
	}

	klog.Infof("PushPackageRevision: package %+v pushed to repository %+v", pr.Key(), repo.Key())
	return pushedPRUpstreamLock, nil
}
