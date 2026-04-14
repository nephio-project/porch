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

package repository

import (
	"context"
	"fmt"

	porchv1alpha2 "github.com/nephio-project/porch/api/porch/v1alpha2"
	configapi "github.com/nephio-project/porch/api/porchconfig/v1alpha1"
	"github.com/nephio-project/porch/pkg/repository"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	fieldManagerRepoController = "repository-controller"
	RepositoryLabel            = "porch.kpt.dev/repository"
)

// syncPackageRevisionCRDs creates, updates, or deletes PackageRevision CRDs
// to match the packages discovered by ListPackageRevisions.
//
// Individual CRD apply/delete failures are logged but do not fail the sync.
// The repo is considered healthy if git is reachable — CRD plumbing errors
// (etcd conflicts, quota, etc.) are transient and resolve on the next sync.
//
// TODO: When the system is fully async (PR controller active), introduce a
// degraded condition on the Repository CR (e.g. PackageRevisionSyncComplete=False)
// to surface partial sync failures to users without marking the repo as unhealthy.
func (r *RepositoryReconciler) syncPackageRevisionCRDs(ctx context.Context, repo *configapi.Repository, pkgRevs []repository.PackageRevision) error {
	existingByName, err := r.listExistingCRDs(ctx, repo)
	if err != nil {
		return err
	}

	desiredNames := r.applyDesiredCRDs(ctx, repo, pkgRevs, existingByName)
	r.deleteStaleCRDs(ctx, existingByName, desiredNames)

	return nil
}

func (r *RepositoryReconciler) listExistingCRDs(ctx context.Context, repo *configapi.Repository) (map[string]*porchv1alpha2.PackageRevision, error) {
	existing := &porchv1alpha2.PackageRevisionList{}
	if err := r.List(ctx, existing,
		client.InNamespace(repo.Namespace),
		client.MatchingLabels{RepositoryLabel: repo.Name},
	); err != nil {
		return nil, fmt.Errorf("failed to list existing PackageRevision CRDs: %w", err)
	}
	result := make(map[string]*porchv1alpha2.PackageRevision, len(existing.Items))
	for i := range existing.Items {
		result[existing.Items[i].Name] = &existing.Items[i]
	}
	return result, nil
}

func (r *RepositoryReconciler) applyDesiredCRDs(ctx context.Context, repo *configapi.Repository, pkgRevs []repository.PackageRevision, existingByName map[string]*porchv1alpha2.PackageRevision) map[string]bool {
	log := log.FromContext(ctx)
	desiredNames := make(map[string]bool, len(pkgRevs))

	for _, pkgRev := range pkgRevs {
		name := pkgRev.KubeObjectName()
		ex, isUpdate := existingByName[name]

		var desired *porchv1alpha2.PackageRevision
		var err error
		if isUpdate {
			desired, err = buildPackageRevisionCRDForUpdate(ctx, repo, pkgRev)
		} else {
			desired, err = buildPackageRevisionCRD(ctx, repo, pkgRev)
		}
		if err != nil {
			log.Error(err, "Failed to build PackageRevision CRD", "key", pkgRev.Key())
			continue
		}
		desiredNames[desired.Name] = true

		if isUpdate && packageRevisionCRDUpToDate(ex, desired) {
			log.V(5).Info("PackageRevision CRD unchanged, skipping", "name", desired.Name)
			continue
		}

		if err := r.applyPackageRevisionCRD(ctx, desired, !isUpdate); err != nil {
			continue
		}
		if isUpdate {
			log.V(3).Info("Updated PackageRevision CRD", "name", desired.Name)
		} else {
			log.V(3).Info("Created PackageRevision CRD", "name", desired.Name)
		}
	}

	return desiredNames
}

func (r *RepositoryReconciler) deleteStaleCRDs(ctx context.Context, existingByName map[string]*porchv1alpha2.PackageRevision, desiredNames map[string]bool) {
	log := log.FromContext(ctx)
	for name, ex := range existingByName {
		if !desiredNames[name] {
			if err := r.Delete(ctx, ex); err != nil {
				log.Error(err, "Failed to delete stale PackageRevision CRD", "name", name)
			}
		}
	}
}

// applyPackageRevisionCRD applies the spec and status of a PackageRevision CRD.
// forceOwnership should be true on create (to claim field ownership even if the
// object already exists from a prior incomplete sync) and false on update (to
// avoid stealing fields owned by the PR controller or the user).
func (r *RepositoryReconciler) applyPackageRevisionCRD(ctx context.Context, crd *porchv1alpha2.PackageRevision, forceOwnership bool) error {
	log := log.FromContext(ctx)

	// Save status before spec apply — r.Patch mutates crd with the server response,
	// which returns status: {} since status is a separate subresource.
	savedStatus := crd.Status

	patchOpts := []client.PatchOption{client.FieldOwner(fieldManagerRepoController)}
	statusOpts := []client.SubResourcePatchOption{client.FieldOwner(fieldManagerRepoController)}
	if forceOwnership {
		patchOpts = append(patchOpts, client.ForceOwnership)
		statusOpts = append(statusOpts, client.ForceOwnership)
	}

	if err := r.Patch(ctx, crd, client.Apply, patchOpts...); err != nil {
		log.Error(err, "Failed to apply PackageRevision CRD", "name", crd.Name)
		return err
	}

	statusObj := &porchv1alpha2.PackageRevision{
		TypeMeta: crd.TypeMeta,
		ObjectMeta: metav1.ObjectMeta{
			Name:      crd.Name,
			Namespace: crd.Namespace,
		},
		Status: savedStatus,
	}
	if err := r.Status().Patch(ctx, statusObj, client.Apply, statusOpts...); err != nil {
		log.Error(err, "Failed to apply PackageRevision CRD status", "name", crd.Name)
		return err
	}

	return nil
}

// packageRevisionCRDUpToDate returns true if the repo-controller-owned fields
// in the existing CRD match the desired state. Skips immutable identity fields
// (package name, repo, workspace) and fields owned by other controllers
// (lifecycle, conditions, publish metadata).
func packageRevisionCRDUpToDate(existing, desired *porchv1alpha2.PackageRevision) bool {
	return equality.Semantic.DeepEqual(existing.Labels, desired.Labels) &&
		existing.Status.Deployment == desired.Status.Deployment &&
		equality.Semantic.DeepEqual(existing.Status.UpstreamLock, desired.Status.UpstreamLock) &&
		equality.Semantic.DeepEqual(existing.Status.SelfLock, desired.Status.SelfLock)
}

// buildPackageRevisionCRD constructs a full PackageRevision CRD for initial creation.
func buildPackageRevisionCRD(ctx context.Context, repo *configapi.Repository, pkgRev repository.PackageRevision) (*porchv1alpha2.PackageRevision, error) {
	crd, err := buildPackageRevisionCRDForUpdate(ctx, repo, pkgRev)
	if err != nil {
		return nil, err
	}

	// Client-owned fields — only set on creation, not on update.
	lifecycle := pkgRev.Lifecycle(ctx)
	crd.Spec.Lifecycle = porchv1alpha2.PackageRevisionLifecycle(lifecycle)

	// Kptfile-derived fields — set on creation. PR controller takes
	// ownership after first render via ForceOwnership.
	kf, _ := pkgRev.GetKptfile(ctx)
	crd.Spec.ReadinessGates = porchv1alpha2.KptfileToReadinessGates(kf)
	crd.Spec.PackageMetadata = porchv1alpha2.KptfileToPackageMetadata(kf)
	crd.Status.PackageConditions = porchv1alpha2.KptfileToPackageConditions(kf)

	// PR-controller-owned publish fields — set on creation for packages
	// discovered as already-published. PR controller takes ownership on
	// first reconcile via ForceOwnership.
	if porchv1alpha2.LifecycleIsPublished(crd.Spec.Lifecycle) {
		key := pkgRev.Key()
		crd.Status.Revision = key.Revision
		commitTime, commitAuthor := pkgRev.GetCommitInfo()
		crd.Status.PublishedBy = commitAuthor
		if !commitTime.IsZero() {
			t := metav1.NewTime(commitTime)
			crd.Status.PublishedAt = &t
		}
	}

	return crd, nil
}

// buildPackageRevisionCRDForUpdate constructs a partial PackageRevision CRD
// containing only repo-controller-owned fields. Omits spec.lifecycle (client-owned),
// spec.source (client-owned), and publish metadata (PR-controller-owned).
func buildPackageRevisionCRDForUpdate(ctx context.Context, repo *configapi.Repository, pkgRev repository.PackageRevision) (*porchv1alpha2.PackageRevision, error) {
	key := pkgRev.Key()
	_, upstreamLock, _ := pkgRev.GetUpstreamLock(ctx)
	_, selfLock, _ := pkgRev.GetLock(ctx)

	status := porchv1alpha2.PackageRevisionStatus{
		UpstreamLock: porchv1alpha2.KptLocatorToLocator(upstreamLock),
		SelfLock:     porchv1alpha2.KptLocatorToLocator(selfLock),
		Deployment:   repo.Spec.Deployment,
		// PackageConditions omitted — PR controller owns after first render.
	}

	crd := &porchv1alpha2.PackageRevision{
		TypeMeta: metav1.TypeMeta{
			Kind:       "PackageRevision",
			APIVersion: porchv1alpha2.SchemeGroupVersion.Identifier(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      pkgRev.KubeObjectName(),
			Namespace: pkgRev.KubeObjectNamespace(),
			Labels:    packageRevisionLabelsForUpdate(repo.Name),
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: configapi.GroupVersion.Identifier(),
					Kind:       configapi.TypeRepository.Kind,
					Name:       repo.Name,
					UID:        repo.UID,
				},
			},
		},
		Spec: porchv1alpha2.PackageRevisionSpec{
			PackageName:    key.PkgKey.ToPkgPathname(),
			RepositoryName: key.RKey().Name,
			WorkspaceName:  key.WorkspaceName,
			// ReadinessGates and PackageMetadata omitted — PR controller owns after first render.
		},
		Status: status,
	}

	return crd, nil
}

// packageRevisionLabels returns the standard labels for a PackageRevision CRD,
// including the repository label and the latest-revision indicator.
func packageRevisionLabels(repoName string, pkgRev repository.PackageRevision) map[string]string {
	labels := map[string]string{
		RepositoryLabel: repoName,
	}
	if pkgRev.IsLatestRevision() {
		labels[porchv1alpha2.LatestPackageRevisionKey] = porchv1alpha2.LatestPackageRevisionValue
	} else {
		labels[porchv1alpha2.LatestPackageRevisionKey] = "false"
	}
	return labels
}

// packageRevisionLabelsForUpdate returns labels for the update path.
// Omits latest-revision — that's PR-controller-owned after initial creation.
func packageRevisionLabelsForUpdate(repoName string) map[string]string {
	return map[string]string{
		RepositoryLabel: repoName,
	}
}
