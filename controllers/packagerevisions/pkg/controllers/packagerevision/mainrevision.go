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
	"time"

	kptfilev1 "github.com/kptdev/kpt/api/kptfile/v1"
	porchkey "github.com/kptdev/porch/api/porch/key"
	porchv1alpha2 "github.com/kptdev/porch/api/porch/v1alpha2"
	configapi "github.com/kptdev/porch/api/porchconfig/v1alpha1"
	"github.com/kptdev/porch/pkg/repository"
	pkgerrors "github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// fieldManagerPRControllerMain is the SSA field manager used by the PR
// controller when it owns the "main" placeholder PackageRevision CR. It is
// distinct from the repo controller's managers so ownership is unambiguous.
const fieldManagerPRControllerMain = "packagerev-controller-main"

// createMainPackageRevision creates or updates the "main" branch PackageRevision
// CR for a package the moment one of its workspaces is published. This mirrors
// v1alpha1's cache behavior (ClosePackageRevisionDraft -> ToMainPackageRevision
// -> createMainPackageRevision), but is driven entirely by the PackageRevision
// controller so it does NOT depend on repository sync.
//
// The main revision is the placeholder that tracks the HEAD of the repository's
// main branch: Key().Revision == -1 and Key().WorkspaceName == the branch
// placeholder. It is created with Source == nil (repo-discovered semantics), so
// the validating webhook permits it as a Published CR and the repo controller
// treats it as a managed/discovered revision rather than a user-created one.
func (r *PackageRevisionReconciler) createMainPackageRevision(
	ctx context.Context,
	publishedPR *porchv1alpha2.PackageRevision,
	repoKey porchkey.RepositoryKey,
) error {
	log := log.FromContext(ctx)

	mainContent, err := r.ContentCache.GetMainPackageContent(ctx, repoKey, publishedPR.Spec.PackageName, publishedPR.Spec.WorkspaceName)
	if err != nil {
		return err
	}

	key := mainContent.Key()
	name := porchkey.ComposePkgRevObjName(key)
	if name == "" {
		log.Info("skipping main revision: could not compose object name",
			"package", publishedPR.Spec.PackageName, "workspace", publishedPR.Spec.WorkspaceName)
		return nil
	}

	// Resolve the owning Repository for the ownerReference. Best-effort: if it
	// cannot be read we still create the main CR (the repo controller / label
	// self-healing will reconcile ownership later).
	repo := &configapi.Repository{}
	var ownerRefs []metav1.OwnerReference
	if err := r.Get(ctx, types.NamespacedName{Namespace: publishedPR.Namespace, Name: publishedPR.Spec.RepositoryName}, repo); err != nil {
		log.V(3).Info("could not get owner repository for main revision ownerRef", "error", err.Error())
	} else {
		controller := true
		blockOwnerDeletion := true
		ownerRefs = []metav1.OwnerReference{{
			APIVersion:         configapi.GroupVersion.Identifier(),
			Kind:               configapi.TypeRepository.Kind,
			Name:               repo.Name,
			UID:                repo.UID,
			Controller:         &controller,
			BlockOwnerDeletion: &blockOwnerDeletion,
		}}
	}

	_, upstreamLock, _ := mainContent.GetUpstreamLock(ctx)
	_, selfLock, _ := mainContent.GetLock(ctx)
	commitTime, commitAuthor := mainContent.GetCommitInfo()

	var sizeBytes int64
	if resources, err := mainContent.GetResourceContents(ctx); err == nil {
		sizeBytes = repository.CalculateResourcesSize(resources)
	}

	// Look up the existing main CR to decide create vs. refresh.
	// IMPORTANT: the main revision's spec.lifecycle is USER-OWNED once the CR
	// exists. v1alpha1's createMainPackageRevision only creates the record when
	// absent and never rewrites lifecycle; a user may propose-delete main
	// (Published -> DeletionProposed) and that must stick. We must NOT re-assert
	// Published here, or every published sibling's reconcile would stomp the
	// user's propose-delete.
	existing := &porchv1alpha2.PackageRevision{}
	getErr := r.Get(ctx, types.NamespacedName{Namespace: publishedPR.Namespace, Name: name}, existing)
	switch {
	case getErr == nil:
		// Already exists. Do not touch spec.lifecycle (user-owned). If it is
		// being deleted or has been moved out of Published, leave it entirely
		// alone — the normal deletion/lifecycle path owns it now.
		if !existing.DeletionTimestamp.IsZero() ||
			existing.Spec.Lifecycle != porchv1alpha2.PackageRevisionLifecyclePublished {
			log.V(4).Info("main PackageRevision exists and is not steady-state Published; leaving untouched",
				"name", name, "lifecycle", existing.Spec.Lifecycle)
			return nil
		}
		// Steady-state Published: refresh only the HEAD-tracking status fields.
		if err := r.patchMainStatus(ctx, publishedPR.Namespace, name, key.Revision, commitAuthor, commitTime, upstreamLock, selfLock, repo.Spec.Deployment, sizeBytes); err != nil {
			return pkgerrors.Wrapf(err, "unable to update main PackageRevision status %s/%s", publishedPR.Namespace, name)
		}
		return nil

	case apierrors.IsNotFound(getErr):
		// Does not exist yet: create it as Published. Source is intentionally nil
		// (repo-discovered), so the validating webhook permits a Published create
		// and the user-created lifecycle restrictions don't apply.
		newMain := &porchv1alpha2.PackageRevision{
			TypeMeta: metav1.TypeMeta{
				Kind:       "PackageRevision",
				APIVersion: porchv1alpha2.SchemeGroupVersion.Identifier(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: publishedPR.Namespace,
				Labels: map[string]string{
					porchv1alpha2.RepositoryLabelKey: key.RKey().Name,
					// The main revision is never the "latest" numbered revision.
					porchv1alpha2.LatestPackageRevisionKey: "false",
				},
				OwnerReferences: ownerRefs,
			},
			Spec: porchv1alpha2.PackageRevisionSpec{
				PackageName:    key.PKey().ToPkgPathname(),
				RepositoryName: key.RKey().Name,
				WorkspaceName:  key.WorkspaceName,
				Lifecycle:      porchv1alpha2.PackageRevisionLifecyclePublished,
			},
		}
		if err := r.Create(ctx, newMain); err != nil {
			if apierrors.IsAlreadyExists(err) {
				// Lost a race with another reconcile; the winner created it. Fine.
				return nil
			}
			return pkgerrors.Wrapf(err, "unable to create main PackageRevision %s/%s", publishedPR.Namespace, name)
		}
		if err := r.patchMainStatus(ctx, publishedPR.Namespace, name, key.Revision, commitAuthor, commitTime, upstreamLock, selfLock, repo.Spec.Deployment, sizeBytes); err != nil {
			return pkgerrors.Wrapf(err, "unable to update main PackageRevision status %s/%s", publishedPR.Namespace, name)
		}
		log.V(2).Info("created main PackageRevision on publish", "name", name, "package", key.PKey().ToPkgPathname())
		return nil

	default:
		return pkgerrors.Wrapf(getErr, "unable to look up main PackageRevision %s/%s", publishedPR.Namespace, name)
	}
}

// patchMainStatus refreshes the HEAD-tracking status fields of the main CR via
// SSA. status.revision is -1 for the placeholder (matching v1alpha1
// ToMainPackageRevision). It never touches spec, so the user-owned lifecycle is
// unaffected.
func (r *PackageRevisionReconciler) patchMainStatus(
	ctx context.Context,
	namespace, name string,
	revision int,
	commitAuthor string,
	commitTime time.Time,
	upstreamLock, selfLock kptfilev1.Locator,
	deployment bool,
	sizeBytes int64,
) error {
	status := porchv1alpha2.PackageRevisionStatus{
		Revision:           revision,
		PublishedBy:        commitAuthor,
		UpstreamLock:       porchv1alpha2.KptLocatorToLocator(upstreamLock),
		SelfLock:           porchv1alpha2.KptLocatorToLocator(selfLock),
		Deployment:         deployment,
		ResourcesSizeBytes: sizeBytes,
	}
	if !commitTime.IsZero() {
		t := metav1.NewTime(commitTime)
		status.PublishedAt = &t
	}

	statusObj := &porchv1alpha2.PackageRevision{
		TypeMeta: metav1.TypeMeta{
			Kind:       "PackageRevision",
			APIVersion: porchv1alpha2.SchemeGroupVersion.Identifier(),
		},
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Status:     status,
	}
	return r.Status().Patch(ctx, statusObj, client.Apply,
		client.FieldOwner(fieldManagerPRControllerMain), client.ForceOwnership)
}
