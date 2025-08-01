// Copyright 2023-2025 The kpt and Nephio Authors
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

package packagevariant

import (
	"context"
	"flag"
	"fmt"
	"strconv"
	"strings"

	porchapi "github.com/nephio-project/porch/api/porch/v1alpha1"
	configapi "github.com/nephio-project/porch/api/porchconfig/v1alpha1"
	api "github.com/nephio-project/porch/controllers/packagevariants/api/v1alpha1"
	pkgerrors "github.com/pkg/errors"

	kptfilev1 "github.com/nephio-project/porch/pkg/kpt/api/kptfile/v1"
	"github.com/nephio-project/porch/pkg/kpt/kptfileutil"
	"github.com/nephio-project/porch/pkg/repository"
	"github.com/nephio-project/porch/third_party/GoogleContainerTools/kpt-functions-sdk/go/fn"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type Options struct{}

func (o *Options) InitDefaults()                       {}
func (o *Options) BindFlags(_ string, _ *flag.FlagSet) {}

// PackageVariantReconciler reconciles a PackageVariant object
type PackageVariantReconciler struct {
	client.Client
	Options
}

const (
	workspaceNamePrefix = "packagevariant-"

	ConditionTypeStalled = "Stalled" // whether or not the packagevariant object is making progress or not
	ConditionTypeReady   = "Ready"   // whether or not the reconciliation succeeded
)

//go:generate go run sigs.k8s.io/controller-tools/cmd/controller-gen@v0.16.1 rbac:headerFile=../../../../../scripts/boilerplate.yaml.txt,roleName=porch-controllers-packagevariants webhook paths="." output:rbac:artifacts:config=../../../config/rbac

//+kubebuilder:rbac:groups=config.porch.kpt.dev,resources=packagevariants,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=config.porch.kpt.dev,resources=packagevariants/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=config.porch.kpt.dev,resources=packagevariants/finalizers,verbs=update
//+kubebuilder:rbac:groups=porch.kpt.dev,resources=packagerevisions,verbs=create;delete;get;list;patch;update;watch
//+kubebuilder:rbac:groups=porch.kpt.dev,resources=packagerevisionresources,verbs=create;delete;get;list;patch;update;watch
//+kubebuilder:rbac:groups=config.porch.kpt.dev,resources=repositories,verbs=get;list;watch

// Reconcile implements the main kubernetes reconciliation loop.
func (r *PackageVariantReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	pv, prList, err := r.init(ctx, req)
	if err != nil {
		return ctrl.Result{}, err
	}
	if pv == nil {
		// maybe the pv was deleted
		return ctrl.Result{}, nil
	}

	defer func() {
		if err := r.Client.Status().Update(ctx, pv); err != nil {
			klog.Errorf("could not update status: %s\n", err.Error())
		}
	}()

	if !pv.ObjectMeta.DeletionTimestamp.IsZero() {
		// This object is being deleted, so we need to make sure the packagerevisions owned by this object
		// are deleted. Normally, garbage collection can handle this, but we have a special case here because
		// (a) we cannot delete published packagerevisions and instead have to propose deletion of them
		// (b) we may want to orphan packagerevisions instead of deleting them.
		for _, pr := range prList.Items {
			if r.hasOurOwnerReference(pv, pr.OwnerReferences) {
				r.deleteOrOrphan(ctx, &pr, pv)
				if pr.Spec.Lifecycle == porchapi.PackageRevisionLifecycleDeletionProposed {
					// We need to orphan this package revision; otherwise it will automatically
					// get deleted after its parent PackageVariant object is deleted.
					r.orphanPackageRevision(ctx, &pr, pv)
				}
			}
		}
		// Remove our finalizer from the list and update it.
		controllerutil.RemoveFinalizer(pv, api.Finalizer)
		if err := r.Update(ctx, pv); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to update %s after delete finalizer: %w", req.Name, err)
		}
		return ctrl.Result{}, nil
	}

	// the object is not being deleted, so let's ensure that our finalizer is here
	if !controllerutil.ContainsFinalizer(pv, api.Finalizer) {
		controllerutil.AddFinalizer(pv, api.Finalizer)
		if err := r.Update(ctx, pv); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to update %s after add finalizer: %w", req.Name, err)
		}
	}

	if errs := validatePackageVariant(pv); len(errs) > 0 {
		setStalledConditionsToTrue(pv, combineErrors(errs))
		// do not requeue; failed validation requires a PV change
		return ctrl.Result{}, nil
	}
	upstream, err := r.getUpstreamPR(pv.Spec.Upstream, prList)
	if err != nil {
		setStalledConditionsToTrue(pv, err.Error())
		// requeue, as the upstream may appear
		return ctrl.Result{}, err
	}
	meta.SetStatusCondition(&pv.Status.Conditions, metav1.Condition{
		Type:    ConditionTypeStalled,
		Status:  "False",
		Reason:  "Valid",
		Message: "all validation checks passed",
	})

	targets, err := r.ensurePackageVariant(ctx, pv, upstream, prList)
	if err != nil {
		meta.SetStatusCondition(&pv.Status.Conditions, metav1.Condition{
			Type:    ConditionTypeReady,
			Status:  "False",
			Reason:  "Error",
			Message: err.Error(),
		})
		// requeue; it may be an intermittent error
		return ctrl.Result{}, err
	}

	setTargetStatusConditions(pv, targets)

	return ctrl.Result{}, nil
}

func (r *PackageVariantReconciler) init(ctx context.Context,
	req ctrl.Request) (*api.PackageVariant, *porchapi.PackageRevisionList, error) {
	var pv api.PackageVariant
	if err := r.Client.Get(ctx, req.NamespacedName, &pv); err != nil {
		return nil, nil, client.IgnoreNotFound(err)
	}

	var prList porchapi.PackageRevisionList
	if err := r.Client.List(ctx, &prList, client.InNamespace(pv.Namespace)); err != nil {
		return nil, nil, err
	}

	return &pv, &prList, nil
}

func validatePackageVariant(pv *api.PackageVariant) []string {
	var allErrs []string

	if upstreamErrs := isValidUpstream(pv.Spec.Upstream); upstreamErrs != nil {
		allErrs = append(allErrs, upstreamErrs...)
	}

	if pv.Spec.Downstream == nil {
		allErrs = append(allErrs, "missing required field spec.downstream")
	} else {
		if pv.Spec.Downstream.Repo == "" {
			allErrs = append(allErrs, "missing required field spec.downstream.repo")
		}
		if pv.Spec.Downstream.Package == "" {
			allErrs = append(allErrs, "missing required field spec.downstream.package")
		}
	}
	if pv.Spec.AdoptionPolicy == "" {
		pv.Spec.AdoptionPolicy = api.AdoptionPolicyAdoptNone
	}
	if pv.Spec.DeletionPolicy == "" {
		pv.Spec.DeletionPolicy = api.DeletionPolicyDelete
	}
	if pv.Spec.AdoptionPolicy != api.AdoptionPolicyAdoptNone && pv.Spec.AdoptionPolicy != api.AdoptionPolicyAdoptExisting {
		allErrs = append(allErrs, fmt.Sprintf("spec.adoptionPolicy field can only be %q or %q",
			api.AdoptionPolicyAdoptNone, api.AdoptionPolicyAdoptExisting))
	}
	if pv.Spec.DeletionPolicy != api.DeletionPolicyOrphan && pv.Spec.DeletionPolicy != api.DeletionPolicyDelete {
		allErrs = append(allErrs, fmt.Sprintf("spec.deletionPolicy can only be %q or %q",
			api.DeletionPolicyOrphan, api.DeletionPolicyDelete))
	}
	if pc := pv.Spec.PackageContext; pc != nil {
		invalidKeys := []string{"name", "package-path"}
		for _, invalid := range invalidKeys {
			if len(pc.Data) > 0 {
				if _, ok := pc.Data[invalid]; ok {
					allErrs = append(allErrs, field.Invalid(
						field.NewPath("spec", "packageContext", "data"),
						pv.Spec.PackageContext.Data,
						fmt.Sprintf("must not contain the key %q", invalid)).Error())
				}
			}
			if len(pc.RemoveKeys) > 0 {
				for _, k := range pc.RemoveKeys {
					if k == invalid {
						allErrs = append(allErrs, field.Invalid(
							field.NewPath("spec", "packageContext", "removeKeys"),
							pv.Spec.PackageContext.RemoveKeys,
							fmt.Sprintf("must not contain the key %q", invalid)).Error())
					}
				}
			}
		}
	}
	if len(pv.Spec.Injectors) > 0 {
		for i, injector := range pv.Spec.Injectors {
			if injector.Name == "" {
				allErrs = append(allErrs, fmt.Sprintf("spec.injectors[%d].name must not be empty", i))
			}
		}
	}
	return allErrs
}

func isValidUpstream(upstream *api.Upstream) []string {
	var upstreamErrs []string

	if upstream == nil {
		upstreamErrs = append(upstreamErrs, "missing required field spec.upstream")
		return upstreamErrs
	}

	if upstream.Repo == "" {
		upstreamErrs = append(upstreamErrs, "missing required field spec.upstream.repo")
	}

	if upstream.Package == "" {
		upstreamErrs = append(upstreamErrs, "missing required field spec.upstream.package")
	}

	upstream.Revision = -1

	return upstreamErrs
}

func combineErrors(errs []string) string {
	var errMsgs []string
	for _, e := range errs {
		if e != "" {
			errMsgs = append(errMsgs, e)
		}
	}
	return strings.Join(errMsgs, "; ")
}

func (r *PackageVariantReconciler) getUpstreamPR(upstream *api.Upstream,
	prList *porchapi.PackageRevisionList) (*porchapi.PackageRevision, error) {
	for _, pr := range prList.Items {
		if pr.Spec.RepositoryName != upstream.Repo || pr.Spec.PackageName != upstream.Package {
			continue
		}

		if upstream.WorkspaceName != "" {
			if pr.Spec.WorkspaceName == upstream.WorkspaceName {
				upstream.Revision = pr.Spec.Revision
				return &pr, nil
			}
			continue
		}

		// Return the main revision if the revision is the placeholder revision
		if pr.Spec.WorkspaceName == "main" {
			upstream.Revision = pr.Spec.Revision
			return &pr, nil
		}
	}

	return nil, fmt.Errorf("could not find upstream package revision %#v", upstream)
}

// getPublishedUpstreamByRevision searches only for published PRs, and ignores workspaceName
func (r *PackageVariantReconciler) getPublishedUpstreamByRevision(upstream *api.Upstream, prList *porchapi.PackageRevisionList) (*porchapi.PackageRevision, error) {
	if upstream.Revision == 0 {
		return nil, pkgerrors.Errorf("upstream cannot be published with revision number 0")
	}

	for _, pr := range prList.Items {
		if pr.Spec.RepositoryName == upstream.Repo &&
			pr.Spec.PackageName == upstream.Package &&
			pr.Spec.Revision == upstream.Revision &&
			pr.IsPublished() {
			return &pr, nil
		}
	}

	return nil, pkgerrors.Errorf("could not find upstream package revision %#v", upstream)
}

func setStalledConditionsToTrue(pv *api.PackageVariant, message string) {
	meta.SetStatusCondition(&pv.Status.Conditions, metav1.Condition{
		Type:    ConditionTypeStalled,
		Status:  "True",
		Reason:  "ValidationError",
		Message: message,
	})
	meta.SetStatusCondition(&pv.Status.Conditions, metav1.Condition{
		Type:    ConditionTypeReady,
		Status:  "False",
		Reason:  "Error",
		Message: "invalid packagevariant object",
	})
}

// ensurePackageVariant needs to:
//   - Check if the downstream package revision already exists. If not, create it.
//   - If it does already exist, we need to make sure it is up-to-date. If there are
//     downstream package drafts, we look at all drafts. Otherwise, we look at the latest
//     published downstream package revision.
//   - Compare pd.Spec.Upstream.Revision to the revision number that the downstream
//     package is based on. If it is different, we need to do an update (could be an upgrade
//     or a downgrade).
//   - Delete or orphan other package revisions owned by this controller that are no
//     longer needed.
func (r *PackageVariantReconciler) ensurePackageVariant(ctx context.Context,
	pv *api.PackageVariant,
	upstream *porchapi.PackageRevision,
	prList *porchapi.PackageRevisionList) ([]*porchapi.PackageRevision, error) {

	existing, err := r.findAndUpdateExistingRevisions(ctx, pv, upstream, prList)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return existing, nil
	}

	// No downstream package created by this controller exists. Create one.
	newPR := &porchapi.PackageRevision{
		TypeMeta: metav1.TypeMeta{
			Kind:       "PackageRevision",
			APIVersion: porchapi.SchemeGroupVersion.Identifier(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace:       pv.Namespace,
			OwnerReferences: []metav1.OwnerReference{constructOwnerReference(pv)},
			Labels:          pv.Spec.Labels,
			Annotations:     pv.Spec.Annotations,
		},
		Spec: porchapi.PackageRevisionSpec{
			PackageName:    pv.Spec.Downstream.Package,
			RepositoryName: pv.Spec.Downstream.Repo,
			WorkspaceName:  newWorkspaceName(prList, pv.Spec.Downstream.Package, pv.Spec.Downstream.Repo),
			Tasks: []porchapi.Task{
				{
					Type: porchapi.TaskTypeClone,
					Clone: &porchapi.PackageCloneTaskSpec{
						Upstream: porchapi.UpstreamPackage{
							UpstreamRef: &porchapi.PackageRevisionRef{
								Name: upstream.Name,
							},
						},
					},
				},
			},
		},
	}

	if err = r.Client.Create(ctx, newPR); err != nil {
		return nil, err
	}
	klog.Infoln(fmt.Sprintf("package variant %q created package revision %q", pv.Name, newPR.Name))

	prr, changed, err := r.calculateDraftResources(ctx, pv, newPR)
	if err != nil {
		return nil, err
	}
	if changed {
		// Save the updated PackageRevisionResources
		if err = r.updatePackageResources(ctx, prr, pv); err != nil {
			return nil, err
		}
	}

	return []*porchapi.PackageRevision{newPR}, nil
}

func (r *PackageVariantReconciler) findAndUpdateExistingRevisions(ctx context.Context,
	pv *api.PackageVariant,
	upstream *porchapi.PackageRevision,
	prList *porchapi.PackageRevisionList) ([]*porchapi.PackageRevision, error) {
	downstreams := r.getDownstreamPRs(ctx, pv, prList)
	if downstreams == nil {
		// If there are no existing target downstream packages, just return nil. The
		// caller will create one.
		return nil, nil
	}

	var err error
	for i, downstream := range downstreams {
		if downstream.Spec.Lifecycle == porchapi.PackageRevisionLifecycleDeletionProposed {
			// We proposed this package revision for deletion in the past, but now it
			// matches our target, so we no longer want it to be deleted.
			downstream.Spec.Lifecycle = porchapi.PackageRevisionLifecyclePublished
			// We update this now, because later we may use a Porch call to clone or update
			// and we want to make sure the server is in sync with us
			if err := r.Client.Update(ctx, downstream); err != nil {
				klog.Errorf("error updating package revision lifecycle: %v", err)
				return nil, err
			}
		}

		// see if the package needs updating due to an upstream change
		if !r.isUpToDate(pv, downstream) {
			// we need to copy a published package to a new draft before updating
			if !porchapi.LifecycleIsPublished(downstream.Spec.Lifecycle) {
				return nil, pkgerrors.Errorf("cannot update upstream of downstream %q when it is not published", downstream.Name)
			}
			klog.Infoln(fmt.Sprintf("package variant %q needs to update package revision %q for new upstream revision, creating new draft", pv.Name, downstream.Name))
			oldDS := downstream
			downstream, err = r.createUpgradeDraft(ctx, downstream, pv, prList)
			if err != nil {
				klog.Errorf("package variant %q failed to copy %q: %s", pv.Name, oldDS.Name, err.Error())
				return nil, err
			}
			downstreams[i] = downstream
			klog.Infoln(fmt.Sprintf("package variant %q created upgrade %q based on %q to upstream revision %d", pv.Name, downstream.Name, oldDS.Name, upstream.Spec.Revision))
		}

		// finally, see if any other changes are needed to the resources
		prr, changed, err := r.calculateDraftResources(ctx, pv, downstreams[i])
		if err != nil {
			return nil, err
		}

		// if there are changes, save them
		if changed {
			// if no pkg update was needed, we may still be a published package
			// so, clone to a new Draft if that's the case
			if porchapi.LifecycleIsPublished(downstream.Spec.Lifecycle) {
				klog.Infoln(fmt.Sprintf("package variant %q needs to mutate to package revision %q, creating new draft", pv.Name, downstream.Name))
				oldDS := downstream
				downstream, err = r.createEditDraft(ctx, downstream, pv, prList)
				if err != nil {
					klog.Errorf("package variant %q failed to create edit of %q: %s", pv.Name, oldDS.Name, err.Error())
					return nil, err
				}
				klog.Infoln(fmt.Sprintf("package variant %q created edit %q based on %q", pv.Name, downstream.Name, oldDS.Name))
				downstreams[i] = downstream
				// recalculate from the new Draft
				prr, _, err = r.calculateDraftResources(ctx, pv, downstreams[i])
				if err != nil {
					return nil, err
				}
			}
			// Save the updated PackageRevisionResources
			if err := r.updatePackageResources(ctx, prr, pv); err != nil {
				return nil, err
			}
		}
	}
	return downstreams, nil
}

// If there are any drafts that are owned by us and match the target package
// revision, return them all. If there are no drafts, return the latest published
// package revision owned by us.
func (r *PackageVariantReconciler) getDownstreamPRs(ctx context.Context,
	pv *api.PackageVariant,
	prList *porchapi.PackageRevisionList) []*porchapi.PackageRevision {
	downstream := pv.Spec.Downstream

	var latestPublished *porchapi.PackageRevision
	var drafts []*porchapi.PackageRevision
	// the first package revision number that porch assigns is "v1",
	// so use v0 as a placeholder for comparison
	latestVersion := -1

	for _, pr := range prList.Items {
		// TODO: When we have a way to find the upstream packagerevision without
		//   listing all packagerevisions, we should add a label to the resources we
		//   own so that we can fetch only those packagerevisions. (A caveat here is
		//   that if the adoptionPolicy is set to adoptExisting, we will still have
		//   to fetch all the packagerevisions so that we can determine which ones
		//   we need to adopt. A mechanism to filter packagerevisions by repo/package
		//   would be helpful for that.)
		owned := r.hasOurOwnerReference(pv, pr.ObjectMeta.OwnerReferences)
		if !owned && pv.Spec.AdoptionPolicy != api.AdoptionPolicyAdoptExisting {
			// this package revision doesn't belong to us
			continue
		}

		// check that the repo and package name match
		if pr.Spec.RepositoryName != downstream.Repo ||
			pr.Spec.PackageName != downstream.Package {
			if owned {
				// We own this package, but it isn't a match for our downstream target,
				// which means that we created it but no longer need it.
				r.deleteOrOrphan(ctx, &pr, pv)
			}
			continue
		}

		// this package matches, check if we need to adopt it
		if !owned && pv.Spec.AdoptionPolicy == api.AdoptionPolicyAdoptExisting {
			klog.Infoln(fmt.Sprintf("package variant %q is adopting package revision %q", pv.Name, pr.Name))
			if err := r.adoptPackageRevision(ctx, &pr, pv); err != nil {
				klog.Errorf("error adopting package revision: %v", err)
			}
		}

		if porchapi.LifecycleIsPublished(pr.Spec.Lifecycle) {
			latestPublished, latestVersion = compare(&pr, latestPublished, latestVersion)
		} else {
			drafts = append(drafts, pr.DeepCopy())
		}
	}

	if len(drafts) > 0 {
		return drafts
	}
	if latestPublished != nil {
		return []*porchapi.PackageRevision{latestPublished}
	}
	return nil
}

func compare(pr, latestPublished *porchapi.PackageRevision, latestVersion int) (*porchapi.PackageRevision, int) {
	if pr.Spec.Revision > latestVersion {
		// current > latest; update latest
		latestVersion = pr.Spec.Revision
		latestPublished = pr.DeepCopy()
	}
	return latestPublished, latestVersion
}

// check that the downstream package was created by this PackageVariant object
func (r *PackageVariantReconciler) hasOurOwnerReference(pv *api.PackageVariant, owners []metav1.OwnerReference) bool {
	for _, owner := range owners {
		if owner.UID == pv.UID {
			return true
		}
	}
	return false
}

func (r *PackageVariantReconciler) deleteOrOrphan(ctx context.Context,
	pr *porchapi.PackageRevision,
	pv *api.PackageVariant) {
	switch pv.Spec.DeletionPolicy {
	case "", api.DeletionPolicyDelete:
		klog.Infoln(fmt.Sprintf("package variant %q is deleting package revision %q", pv.Name, pr.Name))
		r.deletePackageRevision(ctx, pr)
	case api.DeletionPolicyOrphan:
		klog.Infoln(fmt.Sprintf("package variant %q is orphaning package revision %q", pv.Name, pr.Name))
		r.orphanPackageRevision(ctx, pr, pv)
	default:
		// this should never happen, because the pv should already be validated beforehand
		klog.Errorf("invalid deletion policy %s", pv.Spec.DeletionPolicy)
	}
}

func (r *PackageVariantReconciler) orphanPackageRevision(ctx context.Context,
	pr *porchapi.PackageRevision,
	pv *api.PackageVariant) {
	pr.ObjectMeta.OwnerReferences = removeOwnerRefByUID(pr.OwnerReferences, pv.UID)
	if err := r.Client.Update(ctx, pr); err != nil {
		klog.Errorf("error orphaning package revision: %v", err)
	}
}

func removeOwnerRefByUID(ownerRefs []metav1.OwnerReference,
	ownerToRemove types.UID) []metav1.OwnerReference {
	var result []metav1.OwnerReference
	for _, owner := range ownerRefs {
		if owner.UID != ownerToRemove {
			result = append(result, owner)
		}
	}
	return result
}

// When we adopt a package revision, we need to make sure that the package revision
// has our owner reference and also the labels/annotations specified in pv.Spec.
func (r *PackageVariantReconciler) adoptPackageRevision(ctx context.Context,
	pr *porchapi.PackageRevision,
	pv *api.PackageVariant) error {
	pr.ObjectMeta.OwnerReferences = append(pr.OwnerReferences, constructOwnerReference(pv))
	if len(pv.Spec.Labels) > 0 && pr.ObjectMeta.Labels == nil {
		pr.ObjectMeta.Labels = make(map[string]string)
	}
	for k, v := range pv.Spec.Labels {
		pr.ObjectMeta.Labels[k] = v
	}
	if len(pv.Spec.Annotations) > 0 && pr.ObjectMeta.Annotations == nil {
		pr.ObjectMeta.Annotations = make(map[string]string)
	}
	for k, v := range pv.Spec.Annotations {
		pr.ObjectMeta.Annotations[k] = v
	}
	return r.Client.Update(ctx, pr)
}

func (r *PackageVariantReconciler) deletePackageRevision(ctx context.Context, pr *porchapi.PackageRevision) {
	switch pr.Spec.Lifecycle {
	case "", porchapi.PackageRevisionLifecycleDraft, porchapi.PackageRevisionLifecycleProposed:
		if err := r.Client.Delete(ctx, pr); err != nil {
			klog.Errorf("error deleting package revision: %v", err)
		}
	case porchapi.PackageRevisionLifecyclePublished:
		pr.Spec.Lifecycle = porchapi.PackageRevisionLifecycleDeletionProposed
		if err := r.Client.Update(ctx, pr); err != nil {
			klog.Errorf("error proposing deletion for published package revision: %v", err)
		}
	case porchapi.PackageRevisionLifecycleDeletionProposed:
		// we don't have to do anything
	default:
		// if this ever happens, there's something going wrong with porch
		klog.Errorf("invalid lifecycle value for package revision %s: %s", pr.Name, pr.Spec.Lifecycle)
	}
}

// determine if the downstream PR needs to be updated
func (r *PackageVariantReconciler) isUpToDate(pv *api.PackageVariant, downstream *porchapi.PackageRevision) bool {
	if downstream.Status.UpstreamLock == nil {
		klog.Warningf("status.upstreamLock field is empty/missing in downstream PackageRevision: %s", pv.ObjectMeta.Name)
		return true
	}
	upstreamLock := downstream.Status.UpstreamLock
	if upstreamLock.Git == nil || upstreamLock.Git.Ref == "" {
		klog.Warningf("status.upstreamLock.git or status.upstreamLock.git.ref field is empty/missing in downstream PackageRevision: %s", pv.ObjectMeta.Name)
		return true
	}
	if strings.HasPrefix(upstreamLock.Git.Ref, "drafts") {
		// The current upstream is a draft, and the target upstream
		// will always be a published revision, so we will need to do an update.
		return false
	}
	currentUpstreamRevision := revisionFromUpstreamLock(upstreamLock)
	return currentUpstreamRevision == pv.Spec.Upstream.Revision
}

func revisionFromUpstreamLock(lock *porchapi.UpstreamLock) int {
	lastIndex := strings.LastIndex(lock.Git.Ref, "/")
	return repository.Revision2Int(lock.Git.Ref[lastIndex+1:])
}

func (r *PackageVariantReconciler) createUpgradeDraft(ctx context.Context,
	source *porchapi.PackageRevision,
	pv *api.PackageVariant,
	prList *porchapi.PackageRevisionList) (*porchapi.PackageRevision, error) {

	newPr := createDraftTemplate(source, pv, prList)

	oldRevision := revisionFromUpstreamLock(source.Status.UpstreamLock)
	oldUpstreamRef := pv.Spec.Upstream
	oldUpstreamRef.Revision = oldRevision

	oldUpstream, err := r.getPublishedUpstreamByRevision(oldUpstreamRef, prList)
	if err != nil {
		return nil, err
	}

	newUpstream, err := r.getUpstreamPR(pv.Spec.Upstream, prList)
	if err != nil {
		return nil, err
	}

	newPr.Spec.Tasks = []porchapi.Task{
		{
			Type: porchapi.TaskTypeUpgrade,
			Upgrade: &porchapi.PackageUpgradeTaskSpec{
				OldUpstream: porchapi.PackageRevisionRef{
					Name: oldUpstream.Name,
				},
				NewUpstream: porchapi.PackageRevisionRef{
					Name: newUpstream.Name,
				},
				LocalPackageRevisionRef: porchapi.PackageRevisionRef{
					Name: source.Name,
				},
				Strategy: porchapi.ResourceMerge,
			},
		},
	}

	klog.Infoln(fmt.Sprintf("package variant %q is creating upgrade package revision from {old: %q, new: %q, local: %q}",
		pv.Name, oldUpstream.Name, newUpstream.Name, source.Name))
	err = r.Client.Create(ctx, newPr)

	return newPr, err
}

func (r *PackageVariantReconciler) createEditDraft(ctx context.Context,
	source *porchapi.PackageRevision,
	pv *api.PackageVariant,
	prList *porchapi.PackageRevisionList) (*porchapi.PackageRevision, error) {

	newPr := createDraftTemplate(source, pv, prList)

	newPr.Spec.Tasks = []porchapi.Task{
		{
			Type: porchapi.TaskTypeEdit,
			Edit: &porchapi.PackageEditTaskSpec{
				Source: &porchapi.PackageRevisionRef{
					Name: source.Name,
				},
			},
		},
	}

	klog.Infoln(fmt.Sprintf("package variant %q is creating edit package revision from %q", pv.Name, source.Name))
	err := r.Client.Create(ctx, newPr)

	return newPr, err
}

func createDraftTemplate(source *porchapi.PackageRevision,
	pv *api.PackageVariant,
	prList *porchapi.PackageRevisionList) *porchapi.PackageRevision {
	newPr := &porchapi.PackageRevision{
		TypeMeta: metav1.TypeMeta{
			Kind:       "PackageRevision",
			APIVersion: porchapi.SchemeGroupVersion.Identifier(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace:       source.Namespace,
			OwnerReferences: []metav1.OwnerReference{constructOwnerReference(pv)},
			Labels:          pv.Spec.Labels,
			Annotations:     pv.Spec.Annotations,
		},
		Spec: source.Spec,
	}

	newPr.Spec.Revision = 0
	newPr.Spec.WorkspaceName = newWorkspaceName(prList, source.Spec.PackageName, source.Spec.RepositoryName)
	newPr.Spec.Lifecycle = porchapi.PackageRevisionLifecycleDraft
	return newPr
}

func newWorkspaceName(prList *porchapi.PackageRevisionList, packageName, repo string) string {
	wsNum := 0
	for _, pr := range prList.Items {
		if pr.Spec.PackageName != packageName || pr.Spec.RepositoryName != repo {
			continue
		}
		oldWorkspaceName := pr.Spec.WorkspaceName
		if !strings.HasPrefix(oldWorkspaceName, workspaceNamePrefix) {
			continue
		}
		wsNumStr := strings.TrimPrefix(oldWorkspaceName, workspaceNamePrefix)
		newWsNum, _ := strconv.Atoi(wsNumStr)
		if newWsNum > wsNum {
			wsNum = newWsNum
		}
	}
	wsNum++
	return fmt.Sprintf(workspaceNamePrefix+"%d", wsNum)
}

func constructOwnerReference(pv *api.PackageVariant) metav1.OwnerReference {
	tr := true
	return metav1.OwnerReference{
		APIVersion:         pv.APIVersion,
		Kind:               pv.Kind,
		Name:               pv.Name,
		UID:                pv.UID,
		Controller:         &tr,
		BlockOwnerDeletion: nil,
	}
}

func setTargetStatusConditions(pv *api.PackageVariant, targets []*porchapi.PackageRevision) {
	downstreams := []api.DownstreamTarget{}
	// keep downstream status when possible
	for _, t := range targets {
		found := false
		for _, d := range pv.Status.DownstreamTargets {
			if d.Name == t.Name {
				found = true
				downstreams = append(downstreams, d)
				break
			}
		}
		if !found {
			downstreams = append(downstreams, api.DownstreamTarget{
				Name: t.GetName(),
			})
		}
	}
	pv.Status.DownstreamTargets = downstreams
	meta.SetStatusCondition(&pv.Status.Conditions, metav1.Condition{
		Type:    ConditionTypeReady,
		Status:  "True",
		Reason:  "NoErrors",
		Message: "successfully ensured downstream package variant",
	})
}

// SetupWithManager sets up the controller with the Manager.
func (r *PackageVariantReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if err := api.AddToScheme(mgr.GetScheme()); err != nil {
		return err
	}
	if err := porchapi.AddToScheme(mgr.GetScheme()); err != nil {
		return err
	}
	if err := configapi.AddToScheme(mgr.GetScheme()); err != nil {
		return err
	}

	r.Client = mgr.GetClient()

	//TODO: establish watches on resource types injected in all the Package Revisions
	//      we own, and use those to generate requests
	return ctrl.NewControllerManagedBy(mgr).
		For(&api.PackageVariant{}).
		Watches(&porchapi.PackageRevision{}, handler.EnqueueRequestsFromMapFunc(mapObjectsToRequests(r.Client))).
		Complete(r)
}

func mapObjectsToRequests(mgrClient client.Reader) handler.MapFunc {
	return func(ctx context.Context, obj client.Object) []reconcile.Request {
		attachedPackageVariants := &api.PackageVariantList{}
		err := mgrClient.List(ctx, attachedPackageVariants, &client.ListOptions{
			Namespace: obj.GetNamespace(),
		})
		if err != nil {
			return []reconcile.Request{}
		}
		requests := make([]reconcile.Request, len(attachedPackageVariants.Items))
		for i, item := range attachedPackageVariants.Items {
			requests[i] = reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      item.GetName(),
					Namespace: item.GetNamespace(),
				},
			}
		}
		return requests
	}
}

func (r *PackageVariantReconciler) calculateDraftResources(ctx context.Context,
	pv *api.PackageVariant,
	draft *porchapi.PackageRevision) (*porchapi.PackageRevisionResources, bool, error) {

	// Load the PackageRevisionResources
	var prr porchapi.PackageRevisionResources
	prrKey := types.NamespacedName{Name: draft.GetName(), Namespace: draft.GetNamespace()}
	if err := r.Client.Get(ctx, prrKey, &prr); err != nil {
		return nil, false, err
	}

	// Check if it's a valid PRR
	if prr.Spec.Resources == nil {
		return nil, false, fmt.Errorf("nil resources found for PackageRevisionResources '%s/%s'", prr.Namespace, prr.Name)
	}

	origResources := make(map[string]string, len(prr.Spec.Resources))
	for k, v := range prr.Spec.Resources {
		origResources[k] = v
	}

	// Apply our mutations
	if err := ensurePackageContext(pv, &prr); err != nil {
		return nil, false, err
	}

	if err := ensureKRMFunctions(pv, &prr); err != nil {
		return nil, false, err
	}

	if err := ensureConfigInjection(ctx, r.Client, pv, &prr); err != nil {
		return nil, false, err
	}

	if len(prr.Spec.Resources) != len(origResources) {
		// files were added or deleted
		klog.Infoln(fmt.Sprintf("PackageVariant %q, PackageRevision %q, resources changed: %d original files, %d new files", pv.Name, prr.Name, len(origResources), len(prr.Spec.Resources)))
		return &prr, true, nil
	}

	for k, v := range origResources {
		newValue, ok := prr.Spec.Resources[k]
		if !ok {
			// a file was deleted
			klog.Infoln(fmt.Sprintf("PackageVariant %q, PackageRevision %q, resources changed: %q in original files, not in new files", pv.Name, prr.Name, k))
			return &prr, true, nil
		}

		if newValue != v {
			// HACK ALERT - TODO(jbelamaric): Fix this
			// Currently nephio controllers and package variant controller are rendering Kptfiles slightly differently in YAML
			// not sure why, need to investigate more. It may be due to different versions of kyaml. So, here, just for Kptfiles,
			// we will parse and compare semantically.
			//
			if k == "Kptfile" && kptfilesEqual(v, newValue) {
				klog.Infoln(fmt.Sprintf("PackageVariant %q, PackageRevision %q, resources changed: Kptfiles differ, but not semantically", pv.Name, prr.Name))
				continue
			}

			// a file was changed
			klog.Infoln(fmt.Sprintf("PackageVariant %q, PackageRevision %q, resources changed: %q different", pv.Name, prr.Name, k))
			return &prr, true, nil
		}
	}

	// all files in orig are in new, no new files, and all contents match
	// so no change
	klog.Infoln(fmt.Sprintf("PackageVariant %q, PackageRevision %q, resources unchanged", pv.Name, prr.Name))
	return &prr, false, nil
}

func parseKptfile(kf string) (*kptfilev1.KptFile, error) {
	ko, err := fn.ParseKubeObject([]byte(kf))
	if err != nil {
		return nil, err
	}
	var kptfile kptfilev1.KptFile
	err = ko.As(&kptfile)
	if err != nil {
		return nil, err
	}

	return &kptfile, nil
}

func kptfilesEqual(a, b string) bool {
	akf, err := parseKptfile(a)
	if err != nil {
		return false
	}

	bkf, err := parseKptfile(b)
	if err != nil {
		return false
	}

	equal, err := kptfileutil.Equal(akf, bkf)
	if err != nil {
		return false
	}
	return equal
}

func ensurePackageContext(pv *api.PackageVariant,
	prr *porchapi.PackageRevisionResources) error {

	if pv.Spec.PackageContext == nil {
		return nil
	}

	if len(pv.Spec.PackageContext.Data) == 0 && len(pv.Spec.PackageContext.RemoveKeys) == 0 {
		return nil
	}

	cm, err := getFileKubeObject(prr, "package-context.yaml", "ConfigMap", "kptfile.kpt.dev")
	if err != nil {
		return err
	}

	// Set the data fields
	data, ok, err := cm.NestedStringMap("data")
	if err != nil {
		return fmt.Errorf("PackageRevisionResources %s/%s PackageContext invalid data field: %w", prr.Namespace, prr.Name, err)
	}

	if !ok {
		return fmt.Errorf("PackageRevisionResources %s/%s PackageContext no data field found", prr.Namespace, prr.Name)
	}

	// set or add keys that should be there
	for k, v := range pv.Spec.PackageContext.Data {
		data[k] = v
	}

	// remove any keys that should go
	for _, k := range pv.Spec.PackageContext.RemoveKeys {
		delete(data, k)
	}

	err = cm.SetNestedField(data, "data")
	if err != nil {
		return fmt.Errorf("could not set package context data: %w", err)
	}
	prr.Spec.Resources["package-context.yaml"] = cm.String()
	return nil
}

func getFileKubeObject(prr *porchapi.PackageRevisionResources, file, kind, name string) (*fn.KubeObject, error) {
	if prr.Spec.Resources == nil {
		return nil, fmt.Errorf("nil resources found for PackageRevisionResources '%s/%s'", prr.Namespace, prr.Name)
	}

	if _, ok := prr.Spec.Resources[file]; !ok {
		return nil, fmt.Errorf("%q not found in PackageRevisionResources '%s/%s'", file, prr.Namespace, prr.Name)
	}

	ko, err := fn.ParseKubeObject([]byte(prr.Spec.Resources[file]))
	if err != nil {
		return nil, fmt.Errorf("failed to parse %q of PackageRevisionResources %s/%s: %w", file, prr.Namespace, prr.Name, err)
	}
	if kind != "" && ko.GetKind() != kind {
		return nil, fmt.Errorf("%q does not contain kind %q in PackageRevisionResources '%s/%s'", file, kind, prr.Namespace, prr.Name)
	}
	if name != "" && ko.GetName() != name {
		return nil, fmt.Errorf("%q does not contain resource named %q in PackageRevisionResources '%s/%s'", file, name, prr.Namespace, prr.Name)
	}

	return ko, nil
}

// ensureKRMFunctions adds mutators and validators specified in the PackageVariant to the kptfile inside the PackageRevisionResources.
// It generates a unique name that identifies the func (see func generatePVFuncname) and moves it to the top of the mutator sequence.
// It does not preserve yaml indent-style.
func ensureKRMFunctions(pv *api.PackageVariant,
	prr *porchapi.PackageRevisionResources) error {

	// parse kptfile
	kptfile, err := getFileKubeObject(prr, kptfilev1.KptFileName, "", "")
	if err != nil {
		return err
	}
	pipeline := kptfile.UpsertMap("pipeline")

	fieldlist := map[string][]kptfilev1.Function{
		"validators": nil,
		"mutators":   nil,
	}
	// retrieve fields if pipeline is not nil, to avoid nilpointer exception
	if pv.Spec.Pipeline != nil {
		fieldlist["validators"] = pv.Spec.Pipeline.Validators
		fieldlist["mutators"] = pv.Spec.Pipeline.Mutators
	}

	for fieldname, field := range fieldlist {
		var newFieldVal = fn.SliceSubObjects{}

		existingFields, ok, err := pipeline.NestedSlice(fieldname)
		if err != nil {
			return err
		}
		if !ok || existingFields == nil {
			existingFields = fn.SliceSubObjects{}
		}

		for _, existingField := range existingFields {
			ok, err := isPackageVariantFunc(existingField, pv.ObjectMeta.Name)
			if err != nil {
				return err
			}
			if !ok {
				newFieldVal = append(newFieldVal, existingField)
			}
		}

		var newPVFieldVal = fn.SliceSubObjects{}
		for i, newFields := range field {
			newFieldVal := newFields.DeepCopy()
			newFieldVal.Name = generatePVFuncName(newFields.Name, pv.ObjectMeta.Name, i)
			f, err := fn.NewFromTypedObject(newFieldVal)
			if err != nil {
				return err
			}
			newPVFieldVal = append(newPVFieldVal, &f.SubObject)
		}

		newFieldVal = append(newPVFieldVal, newFieldVal...)

		// if there are new mutators/validators, set them. Otherwise delete the field. This avoids ugly dangling `mutators: []` fields in the final kptfile
		if len(newFieldVal) > 0 {
			if err := pipeline.SetSlice(newFieldVal, fieldname); err != nil {
				return err
			}
		} else {
			if _, err := pipeline.RemoveNestedField(fieldname); err != nil {
				return err
			}
		}
	}

	// if there are no mutators and no validators, remove the dangling pipeline field
	if pipeline.GetMap("mutators") == nil && pipeline.GetMap("validators") == nil {
		if _, err := kptfile.RemoveNestedField("pipeline"); err != nil {
			return err
		}
	}

	// update kptfile
	prr.Spec.Resources[kptfilev1.KptFileName] = kptfile.String()

	return nil
}

const PackageVariantFuncPrefix = "PackageVariant"

// isPackageVariantFunc returns true if a function has been created via a PackageVariant.
// It uses the name of the func to determine its origin and compares it with the supplied pvName.
func isPackageVariantFunc(fn *fn.SubObject, pvName string) (bool, error) {
	origname, ok, err := fn.NestedString("name")
	if err != nil {
		return false, fmt.Errorf("could not retrieve field name: %w", err)
	}
	if !ok {
		return false, nil
	}

	name := strings.Split(origname, ".")

	// if more or less than 3 dots have been used, return false
	if len(name) != 4 {
		return false, nil
	}

	// if PackageVariantFuncPrefix has not been used, return false
	if name[0] != PackageVariantFuncPrefix {
		return false, nil
	}

	// if pv-names don't match, return false
	if name[1] != pvName {
		return false, nil
	}

	// if the last segment is not an integer, return false
	if _, err := strconv.Atoi(name[3]); err != nil {
		return false, nil
	}

	return true, nil
}

func generatePVFuncName(funcName, pvName string, pos int) string {
	return fmt.Sprintf("%s.%s.%s.%d", PackageVariantFuncPrefix, pvName, funcName, pos)
}

func (r *PackageVariantReconciler) updatePackageResources(ctx context.Context, prr *porchapi.PackageRevisionResources, pv *api.PackageVariant) error {
	if err := r.Update(ctx, prr); err != nil {
		return err
	}
	for i, target := range pv.Status.DownstreamTargets {
		if target.Name == prr.Name {
			pv.Status.DownstreamTargets[i].RenderStatus = prr.Status.RenderStatus
			return nil
		}
	}
	pv.Status.DownstreamTargets = append(pv.Status.DownstreamTargets, api.DownstreamTarget{
		Name:         prr.Name,
		RenderStatus: prr.Status.RenderStatus,
	})
	return nil
}
