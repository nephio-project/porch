// Copyright 2026 The kpt and Nephio Authors
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

package packagerevision

import (
	"context"
	"fmt"

	porchv1alpha2 "github.com/nephio-project/porch/api/porch/v1alpha2"
	configapi "github.com/nephio-project/porch/api/porchconfig/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// handleDeletion gates deletion of Published packages: they must be
// DeletionProposed first, unless the owner Repository is already gone
// (GC cascade). For all other lifecycles the finalizer is removed immediately.
func (r *PackageRevisionReconciler) handleDeletion(ctx context.Context, pr *porchv1alpha2.PackageRevision) (*ctrl.Result, error) {
	if pr.Spec.Lifecycle == porchv1alpha2.PackageRevisionLifecyclePublished && r.ownerRepoExists(ctx, pr) {
		log.FromContext(ctx).Info("blocking deletion: published package must be DeletionProposed first", "lifecycle", pr.Spec.Lifecycle)
		return &ctrl.Result{}, nil
	}
	patch := client.MergeFrom(pr.DeepCopy())
	if controllerutil.RemoveFinalizer(pr, porchv1alpha2.PackageRevisionFinalizer) {
		if err := r.Patch(ctx, pr, patch); client.IgnoreNotFound(err) != nil {
			return nil, fmt.Errorf("failed to remove finalizer: %w", err)
		}
	}

	// Re-evaluate latest-revision labels for the remaining revisions of
	// the same package, so the previous revision gets promoted to latest.
	r.updateLatestRevisionLabels(ctx, pr)

	return &ctrl.Result{}, nil
}

// ensureFinalizerAndOwner adds the finalizer and Repository ownerReference
// if not already present, in a single patch.
func (r *PackageRevisionReconciler) ensureFinalizerAndOwner(ctx context.Context, pr *porchv1alpha2.PackageRevision) error {
	patch := client.MergeFrom(pr.DeepCopy())
	needsPatch := controllerutil.AddFinalizer(pr, porchv1alpha2.PackageRevisionFinalizer)

	if !hasOwnerReference(pr, pr.Spec.RepositoryName) {
		if err := r.setOwnerReference(ctx, pr); err != nil {
			log.FromContext(ctx).Error(err, "failed to set owner reference")
		} else {
			needsPatch = true
		}
	}

	if needsPatch {
		if err := r.Patch(ctx, pr, patch); err != nil {
			return fmt.Errorf("failed to patch finalizer/ownerReference: %w", err)
		}
	}
	return nil
}

func hasOwnerReference(pr *porchv1alpha2.PackageRevision, repoName string) bool {
	for _, ref := range pr.OwnerReferences {
		if ref.Kind == configapi.TypeRepository.Kind && ref.Name == repoName {
			return true
		}
	}
	return false
}

func (r *PackageRevisionReconciler) setOwnerReference(ctx context.Context, pr *porchv1alpha2.PackageRevision) error {
	var repo configapi.Repository
	if err := r.Get(ctx, types.NamespacedName{Namespace: pr.Namespace, Name: pr.Spec.RepositoryName}, &repo); err != nil {
		return err
	}
	pr.OwnerReferences = append(pr.OwnerReferences, metav1.OwnerReference{
		APIVersion: configapi.GroupVersion.Identifier(),
		Kind:       configapi.TypeRepository.Kind,
		Name:       repo.Name,
		UID:        repo.UID,
	})
	return nil
}

func (r *PackageRevisionReconciler) ownerRepoExists(ctx context.Context, pr *porchv1alpha2.PackageRevision) bool {
	var repo configapi.Repository
	err := r.Get(ctx, types.NamespacedName{Namespace: pr.Namespace, Name: pr.Spec.RepositoryName}, &repo)
	return err == nil
}
