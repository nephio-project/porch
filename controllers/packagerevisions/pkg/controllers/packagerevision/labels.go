// Copyright 2026 The kpt Authors
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

	porchv1alpha2 "github.com/kptdev/porch/api/porch/v1alpha2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// ensureRepositoryLabel ensures the porch.kpt.dev/repository label matches spec.repositoryName.
// Called on every reconcile — self-healing if the label is removed or incorrect.
// Returns an error to trigger requeue, since the repo controller's label selector
// depends on this label for correctness.
func (r *PackageRevisionReconciler) ensureRepositoryLabel(ctx context.Context, pr *porchv1alpha2.PackageRevision) error {
	if pr.Labels != nil && pr.Labels[porchv1alpha2.RepositoryLabelKey] == pr.Spec.RepositoryName {
		return nil
	}
	original := pr.DeepCopy()
	if pr.Labels == nil {
		pr.Labels = map[string]string{}
	}
	pr.Labels[porchv1alpha2.RepositoryLabelKey] = pr.Spec.RepositoryName
	if err := r.Patch(ctx, pr, client.MergeFrom(original)); err != nil {
		// Revert in-memory mutation so callers see the real state.
		pr.Labels = original.Labels
		return fmt.Errorf("failed to set repository label %q on PackageRevision %s/%s: %w", porchv1alpha2.RepositoryLabelKey, pr.Namespace, pr.Name, err)
	}
	return nil
}

// ensureLatestRevisionLabel backfills the latest-revision label with "false" when
// absent (self-healing). It only adds when missing — never overwrites the "true"
// winner set by updateLatestRevisionLabels. Returns an error so the caller requeues.
func (r *PackageRevisionReconciler) ensureLatestRevisionLabel(ctx context.Context, pr *porchv1alpha2.PackageRevision) error {
	if _, ok := pr.Labels[porchv1alpha2.LatestPackageRevisionKey]; ok {
		return nil
	}
	original := pr.DeepCopy()
	if pr.Labels == nil {
		pr.Labels = map[string]string{}
	}
	pr.Labels[porchv1alpha2.LatestPackageRevisionKey] = "false"
	if err := r.Patch(ctx, pr, client.MergeFrom(original)); err != nil {
		// Revert in-memory mutation so callers see the real persisted state.
		pr.Labels = original.Labels
		log.FromContext(ctx).Error(err, "failed to set latest-revision label")
		return fmt.Errorf("failed to set latest-revision label %q on PackageRevision %s/%s: %w", porchv1alpha2.LatestPackageRevisionKey, pr.Namespace, pr.Name, err)
	}
	return nil
}

// updateLatestRevisionLabels sets latest-revision=true on the highest published
// revision and false on the rest. Returns an error so the caller can requeue when
// a stale cached read would otherwise strand a wrong label.
func (r *PackageRevisionReconciler) updateLatestRevisionLabels(ctx context.Context, pr *porchv1alpha2.PackageRevision) error {
	log := log.FromContext(ctx)

	var allRevs porchv1alpha2.PackageRevisionList
	if err := r.List(ctx, &allRevs,
		client.InNamespace(pr.Namespace),
		client.MatchingFields{
			string(porchv1alpha2.PkgRevSelectorRepository):  pr.Spec.RepositoryName,
			string(porchv1alpha2.PkgRevSelectorPackageName): pr.Spec.PackageName,
		},
	); err != nil {
		return fmt.Errorf("failed to list package revisions for latest-revision update: %w", err)
	}

	// Seed the winner with the in-hand pr (fresher than its cached copy).
	highestRev := 0
	var latestName string
	if pr.DeletionTimestamp.IsZero() && porchv1alpha2.LifecycleIsPublished(pr.Spec.Lifecycle) && pr.Status.Revision > 0 {
		highestRev = pr.Status.Revision
		latestName = pr.Name
	}

	for i := range allRevs.Items {
		rev := &allRevs.Items[i]
		if !rev.DeletionTimestamp.IsZero() {
			continue
		}
		if !porchv1alpha2.LifecycleIsPublished(rev.Spec.Lifecycle) {
			continue
		}
		revNum := rev.Status.Revision
		if rev.Name == pr.Name && pr.Status.Revision > revNum {
			revNum = pr.Status.Revision
		}
		// Unobserved revision number = stale read; requeue to recompute.
		if revNum == 0 {
			return fmt.Errorf("published revision %q has unobserved revision number; requeuing latest-revision update", rev.Name)
		}
		if revNum > highestRev {
			highestRev = revNum
			latestName = rev.Name
		}
	}

	for i := range allRevs.Items {
		rev := &allRevs.Items[i]
		if !rev.DeletionTimestamp.IsZero() {
			continue
		}
		desiredLabel := "false"
		if rev.Name == latestName {
			desiredLabel = porchv1alpha2.LatestPackageRevisionValue
		}
		currentLabel := rev.Labels[porchv1alpha2.LatestPackageRevisionKey]
		if currentLabel == desiredLabel {
			continue
		}
		log.V(1).Info("updating latest-revision label", "name", rev.Name, "value", desiredLabel)
		patch := client.MergeFrom(rev.DeepCopy())
		if rev.Labels == nil {
			rev.Labels = map[string]string{}
		}
		rev.Labels[porchv1alpha2.LatestPackageRevisionKey] = desiredLabel
		if err := r.Patch(ctx, rev, patch); err != nil {
			return fmt.Errorf("failed to update latest-revision label on %q: %w", rev.Name, err)
		}
	}
	return nil
}
