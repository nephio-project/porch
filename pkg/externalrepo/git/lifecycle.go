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

package git

import (
	"context"
	"fmt"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/nephio-project/porch/api/porch/v1alpha1"
	pkgerrors "github.com/pkg/errors"
	"go.opentelemetry.io/otel/trace"
	"k8s.io/klog/v2"
)


func (r *gitRepository) GetLifecycle(ctx context.Context, pkgRev *gitPackageRevision) v1alpha1.PackageRevisionLifecycle {
	_, span := tracer.Start(ctx, "gitRepository::GetLifecycle", trace.WithAttributes())
	defer span.End()
	r.mutex.Lock()
	defer r.mutex.Unlock()

	return r.getLifecycle(pkgRev)
}

func (r *gitRepository) getLifecycle(pkgRev *gitPackageRevision) v1alpha1.PackageRevisionLifecycle {
	switch ref := pkgRev.ref; {
	case ref == nil:
		return r.checkPublishedLifecycle(pkgRev)
	case isDraftBranchNameInLocal(ref.Name()):
		return v1alpha1.PackageRevisionLifecycleDraft
	case isProposedBranchNameInLocal(ref.Name()):
		return v1alpha1.PackageRevisionLifecycleProposed
	default:
		return r.checkPublishedLifecycle(pkgRev)
	}
}

func (r *gitRepository) checkPublishedLifecycle(pkgRev *gitPackageRevision) v1alpha1.PackageRevisionLifecycle {
	if r.deletionProposedCache == nil {
		if err := r.updateDeletionProposedCache(); err != nil {
			klog.Errorf("failed to update deletionProposed cache: %v", err)
			return v1alpha1.PackageRevisionLifecyclePublished
		}
	}

	branchName := createDeletionProposedName(pkgRev.Key())
	if _, found := r.deletionProposedCache[branchName]; found {
		return v1alpha1.PackageRevisionLifecycleDeletionProposed
	}

	return v1alpha1.PackageRevisionLifecyclePublished
}

func (r *gitRepository) UpdateLifecycle(ctx context.Context, pkgRev *gitPackageRevision, newLifecycle v1alpha1.PackageRevisionLifecycle) error {
	ctx, span := tracer.Start(ctx, "gitRepository::UpdateLifecycle", trace.WithAttributes())
	defer span.End()

	r.mutex.Lock()
	defer r.mutex.Unlock()

	old := r.getLifecycle(pkgRev)
	if !v1alpha1.LifecycleIsPublished(old) {
		return fmt.Errorf("cannot update lifecycle for draft package revision")
	}
	refSpecs := newPushRefSpecBuilder()
	deletionProposedBranch := createDeletionProposedName(pkgRev.Key())

	if old == v1alpha1.PackageRevisionLifecyclePublished {
		if newLifecycle != v1alpha1.PackageRevisionLifecycleDeletionProposed {
			return fmt.Errorf("invalid new lifecycle value: %q", newLifecycle)
		}
		// Push the package revision into a deletionProposed branch.
		r.deletionProposedCache[deletionProposedBranch] = true
		refSpecs.AddRefToPush(pkgRev.commit, deletionProposedBranch.RefInLocal())
	}
	if old == v1alpha1.PackageRevisionLifecycleDeletionProposed {
		if newLifecycle != v1alpha1.PackageRevisionLifecyclePublished {
			return fmt.Errorf("invalid new lifecycle value: %q", newLifecycle)
		}

		// Delete the deletionProposed branch
		delete(r.deletionProposedCache, deletionProposedBranch)
		ref := plumbing.NewHashReference(deletionProposedBranch.RefInLocal(), pkgRev.commit)
		refSpecs.AddRefToDelete(ref)
	}

	if err := r.pushAndCleanup(ctx, refSpecs); err != nil {
		if !pkgerrors.Is(err, git.NoErrAlreadyUpToDate) {
			return err
		}
	}

	return nil
}
