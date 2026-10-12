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
	"path"
	"time"

	kptfilev1 "github.com/kptdev/kpt/api/kptfile/v1"
	"github.com/kptdev/krm-functions-sdk/go/fn/kptfileko"
	porchapi "github.com/kptdev/porch/api/porch"
	"github.com/kptdev/porch/api/porch/key"
	porchv1alpha2 "github.com/kptdev/porch/api/porch/v1alpha2"
	configapi "github.com/kptdev/porch/api/porchconfig/v1alpha1"
	"github.com/kptdev/porch/controllers/functionconfigs"
	"github.com/kptdev/porch/internal/telemetry"
	"github.com/kptdev/porch/pkg/repository"
	pkgerrors "github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

//go:generate go run sigs.k8s.io/controller-tools/cmd/controller-gen@v0.22.0 rbac:headerFile=../../../../../scripts/boilerplate.yaml.txt,roleName=porch-controllers-packagerevisions,year=$YEAR_GEN webhook:headerFile=../../../../../scripts/boilerplate.yaml.txt,year=$YEAR_GEN paths="." output:rbac:artifacts:config=../../../config/rbac output:webhook:artifacts:config=../../../config/webhook

//+kubebuilder:webhookconfiguration:mutating=false,name=packagerevision-validating-webhook-configuration
//+kubebuilder:webhook:path=/validate-porch-kpt-dev-v1alpha2-packagerevision,mutating=false,failurePolicy=fail,groups=porch.kpt.dev,resources=packagerevisions,verbs=create;update;delete,versions=v1alpha2,name=packagerevision-validator.porch.kpt.dev,admissionReviewVersions=v1,sideEffects=None,serviceName=porch-controllers,serviceNamespace=porch-system,servicePort=9443,timeoutSeconds=30
//+kubebuilder:rbac:groups=porch.kpt.dev,resources=packagerevisions,verbs=get;list;watch;update;patch
//+kubebuilder:rbac:groups=porch.kpt.dev,resources=packagerevisions/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=porch.kpt.dev,resources=packagerevisions/finalizers,verbs=update
//+kubebuilder:rbac:groups=config.porch.kpt.dev,resources=repositories,verbs=get
//+kubebuilder:rbac:groups=config.porch.kpt.dev,resources=functionconfigs,verbs=get;list;watch;patch
//+kubebuilder:rbac:groups=config.porch.kpt.dev,resources=functionconfigs/status,verbs=get;update;patch

const reconcilerName = "packagerevisions"

// contentCacheRetryDelay is how long to wait before requeuing when the content
// cache cannot yet resolve a package's repository (e.g. right after a controller
// restart, before the repository cache is populated). Keeps the package from
// being stuck Ready=False with no retry.
const contentCacheRetryDelay = 5 * time.Second

// contentCacheFailureDeadline bounds how long content-cache resolution may stay
// in the transient Pending/retry state before it is escalated to a terminal
// Failed condition. This guards against reconciling forever when the failure is
// not actually transient (e.g. the repository is genuinely misconfigured or
// gone) rather than the cache simply still warming up after a restart.
const contentCacheFailureDeadline = 2 * time.Minute

// renderRetryRequeueDelay is how long to wait before requeuing a freshly created
// draft so its render step (reconcileRender) runs. After a source operation
// (clone/copy/upgrade/init) closes the draft, the package is left Ready=False
// "awaiting render" with observedGeneration == generation, so no watch event will
// wake it again — only an explicit requeue drives the render. Using RequeueAfter
// (rather than a bare, best-effort Requeue that can be dropped under heavy load)
// guarantees the render is retried and the package does not get stuck.
const renderRetryRequeueDelay = 2 * time.Second

// PackageRevisionReconciler reconciles v1alpha2 PackageRevision CRDs.
// It handles lifecycle transitions (draft/proposed/published) by executing
// git operations via the shared cache.
type PackageRevisionReconciler struct {
	client.Client
	Scheme                 *runtime.Scheme
	ContentCache           repository.ContentCache
	ExternalPackageFetcher repository.ExternalPackageFetcher
	FunctionConfigStore    *functionconfigs.FunctionConfigStore
	Renderer               renderer // nil = skip rendering

	MaxConcurrentReconciles    int
	MaxConcurrentRenders       int
	RenderRequeueDelay         time.Duration
	RepoOperationRetryAttempts int
	MaxGRPCMessageSize         int
	renderLimiter              chan struct{} // bounds concurrent fn-runner calls
	apiReader                  client.Reader // bypasses informer cache for direct etcd reads
}

func (r *PackageRevisionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var pr porchv1alpha2.PackageRevision
	if err := r.Get(ctx, req.NamespacedName, &pr); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if result, err := r.reconcileFinalizer(ctx, &pr); err != nil || result != nil {
		return resultOrDefault(result), err
	}

	// Ensure the repository label is always correct — self-healing invariant.
	if err := r.ensureRepositoryLabel(ctx, &pr); err != nil {
		return ctrl.Result{}, err
	}

	// Backfill the latest-revision label if it is missing — self-healing invariant.
	// This only adds the default "false" when absent; it never overwrites the "true"
	// winner set on published revisions by updateLatestRevisionLabels. Done here (not
	// just once in the create path) so a draft whose one-shot label patch failed
	// mid-reconcile still gets the label on a later pass.
	if err := r.ensureLatestRevisionLabel(ctx, &pr); err != nil {
		return ctrl.Result{}, err
	}

	desired := string(pr.Spec.Lifecycle)
	if desired == "" {
		return ctrl.Result{}, nil
	}

	repoKey := key.RepositoryKey{
		Namespace: pr.Namespace,
		Name:      pr.Spec.RepositoryName,
	}

	// TODO: Errors from sub-reconciles are swallowed (returned as nil to controller-runtime),
	// so the work queue doesn't apply exponential backoff on persistent failures.
	// Consider returning errors to enable backoff, but note the side effects:
	// double logging, error metrics, and inconsistency with reconcileLifecycle.
	if result, err := r.reconcileSource(ctx, &pr, repoKey); err != nil || result != nil {
		return resultOrDefault(result), nil
	}

	if result, err := r.reconcilePackageMetadata(ctx, &pr, repoKey); err != nil || result != nil {
		return resultOrDefault(result), nil
	}

	if result, err := r.reconcileSubpackageOperation(ctx, &pr, repoKey); err != nil || result != nil {
		return resultOrDefault(result), nil
	}

	if result, err := r.reconcileRender(ctx, &pr, repoKey); err != nil || result != nil {
		return resultOrDefault(result), nil
	}

	// Re-read to pick up spec changes (e.g. lifecycle transitions) that
	// occurred while render was in-flight. The validating webhook and controller
	// guard now block lifecycle transitions during render, but re-reading ensures
	// we have fresh state before the guard validates render completion.
	if err := r.Get(ctx, req.NamespacedName, &pr); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	return r.reconcileLifecycle(ctx, &pr, repoKey)
}

// reconcileFinalizer ensures the finalizer and ownerReference are present,
// and handles deletion gating.
// Published packages require DeletionProposed before deletion, unless the
// owner Repository has been deleted (GC cascade).
// Returns (nil, nil) when reconciliation should continue.
func (r *PackageRevisionReconciler) reconcileFinalizer(ctx context.Context, pr *porchv1alpha2.PackageRevision) (*ctrl.Result, error) {
	if !pr.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, pr)
	}
	return nil, r.ensureFinalizerAndOwner(ctx, pr)
}

func (r *PackageRevisionReconciler) reconcileLifecycle(ctx context.Context, pr *porchv1alpha2.PackageRevision, repoKey key.RepositoryKey) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	desired := string(pr.Spec.Lifecycle)

	content, err := r.ContentCache.GetPackageContent(ctx, repoKey, pr.Spec.PackageName, pr.Spec.WorkspaceName)
	if err != nil {
		// This commonly fails transiently: right after a controller restart the
		// repository cache may not be populated yet, so the content lookup returns
		// "repository ... not found". Requeue so the package self-heals once the
		// cache is ready, instead of being left permanently stuck Ready=False with
		// no status.revision.
		//
		// But don't retry forever: if the condition has been Pending for longer
		// than contentCacheFailureDeadline, the failure is almost certainly not
		// transient, so escalate to a terminal Failed condition and stop
		// requeuing. The next watch event / periodic resync will re-evaluate.
		cond := readyCondition(pr.Generation, metav1.ConditionFalse, porchv1alpha2.ReasonPending, err.Error())
		if pendingExpired(pr, contentCacheFailureDeadline) {
			log.Error(err, "failed to get package content; giving up after deadline",
				"deadline", contentCacheFailureDeadline)
			cond.Reason = porchv1alpha2.ReasonFailed
			r.updateStatus(ctx, pr, nil, "", "", cond)
			return ctrl.Result{}, nil
		}
		// Preserve the first-Pending time across retries so the deadline measures
		// elapsed failure time, not time since the last reconcile.
		if t := pendingSince(pr); !t.IsZero() {
			cond.LastTransitionTime = metav1.NewTime(t)
		}
		log.Error(err, "failed to get package content; requeuing")
		r.updateStatus(ctx, pr, nil, "", "", cond)
		return ctrl.Result{RequeueAfter: contentCacheRetryDelay}, nil
	}

	current := content.Lifecycle(ctx)
	if current == desired {
		r.updateStatus(ctx, pr, content, "", "", readyCondition(pr.Generation, metav1.ConditionTrue, porchv1alpha2.ReasonReady, ""))
		// The "main" placeholder revision (Status.Revision == -1) is a HEAD
		// tracker, not a numbered revision. It must NOT drive latest-revision
		// labeling or re-derive the main CR from itself — that is the job of the
		// published numbered revisions. Running it here is redundant work and
		// produces misleading logs tagged with the main CR's name. Only numbered
		// published revisions manage the main branch and latest labels.
		if porchv1alpha2.LifecycleIsPublished(porchv1alpha2.PackageRevisionLifecycle(desired)) && pr.Status.Revision != -1 {
			r.updateLatestRevisionLabels(ctx, pr)
			// Keep the main-branch revision CR in sync with the published HEAD.
			// Created/updated directly by this controller (independent of repo sync).
			if err := r.createMainPackageRevision(ctx, pr, repoKey); err != nil {
				log.Error(err, "failed to create/update main PackageRevision")
				return ctrl.Result{Requeue: true}, nil
			}
		}
		return ctrl.Result{}, nil
	}

	// Render race guard: prevent publishing/proposing while render is incomplete
	if desiredLC := porchv1alpha2.PackageRevisionLifecycle(desired); desiredLC == porchv1alpha2.PackageRevisionLifecyclePublished ||
		desiredLC == porchv1alpha2.PackageRevisionLifecycleProposed {
		if err := r.validateRenderStateBeforePublish(ctx, pr); err != nil {
			log.Info("lifecycle transition blocked by render guard", "reason", err.Error())
			r.updateStatus(ctx, pr, content, "", "", readyCondition(pr.Generation, metav1.ConditionFalse, porchv1alpha2.ReasonPending, err.Error()))
			return ctrl.Result{Requeue: true}, nil
		}
	}

	log.Info("lifecycle transition", "name", pr.Name, "current", current, "desired", desired)

	start := time.Now()
	updated, err := r.ContentCache.UpdateLifecycle(ctx, repoKey, pr.Spec.PackageName, pr.Spec.WorkspaceName, desired)
	telemetry.RecordControllerOperation(telemetry.ResourcePackageRevision, "UPDATE", start)
	if err != nil {
		log.Error(err, "lifecycle transition failed")
		r.updateStatus(ctx, pr, nil, "", "", readyCondition(pr.Generation, metav1.ConditionFalse, porchv1alpha2.ReasonFailed, err.Error()))
		return ctrl.Result{Requeue: true}, nil
	}

	r.updateStatus(ctx, pr, updated, "", "", readyCondition(pr.Generation, metav1.ConditionTrue, porchv1alpha2.ReasonReady, ""))

	if porchv1alpha2.LifecycleIsPublished(porchv1alpha2.PackageRevisionLifecycle(desired)) {
		// Create/update the main-branch revision CR immediately on publish,
		// directly from the just-published content — independent of repo sync.
		if err := r.createMainPackageRevision(ctx, pr, repoKey); err != nil {
			log.Error(err, "failed to create/update main PackageRevision")
			// Non-fatal: the requeue below (and subsequent reconciles) will retry.
		}
		// Requeue so the informer cache indexes the new status.revision
		// before updateLatestRevisionLabels runs its List query.
		return ctrl.Result{Requeue: true}, nil
	}

	return ctrl.Result{}, nil
}

func resultOrDefault(result *ctrl.Result) ctrl.Result {
	if result != nil {
		return *result
	}
	return ctrl.Result{}
}

// reconcileSource handles one-time package creation from spec.source.
// Returns (nil, nil) if no source needs to be applied.
// Returns (result, nil) if source was applied and status was updated.
// Returns (nil, err) on failure.
func (r *PackageRevisionReconciler) reconcileSource(ctx context.Context, pr *porchv1alpha2.PackageRevision, repoKey key.RepositoryKey) (*ctrl.Result, error) {
	// Skip if already created or no source to apply (same conditions as applySource).
	if pr.Status.CreationSource != "" || pr.Spec.Source == nil {
		return nil, nil
	}

	// Reject source execution on repos missing the v1alpha2-migration annotation,
	// before applySource so no source work runs. Backstops the webhook if bypassed.
	if err := r.verifyRepoMigrated(ctx, repoKey); err != nil {
		return nil, r.setFailedConditionsAndLog(ctx, pr, "", err)
	}

	resources, sourceOperationType, err := r.applySource(ctx, pr)
	if err != nil {
		return nil, r.setFailedConditionsAndLog(ctx, pr, sourceOperationType, err)
	}
	if resources == nil {
		return nil, nil
	}

	log := log.FromContext(ctx)
	log.Info("applying operation", "type", sourceOperationType, "name", pr.Name)

	// TODO: CreateNewDraft always receives lifecycle=Draft — consider removing the lifecycle parameter from the interface.
	draft, err := r.ContentCache.CreateNewDraft(ctx, repoKey, pr.Spec.PackageName, pr.Spec.WorkspaceName, string(porchv1alpha2.PackageRevisionLifecycleDraft))
	if err != nil {
		return nil, r.setFailedConditionsAndLog(ctx, pr, sourceOperationType, pkgerrors.Wrapf(err, "create draft"))
	}

	return r.finalizeDraftAndUpdateStatus(ctx, pr, repoKey, draft, resources, sourceOperationType, "")
}

// reconcileSubpackageOperation handles one-time package creation from spec.source.
// Returns (nil, nil) if no source needs to be applied.
// Returns (result, nil) if source was applied and status was updated.
// Returns (nil, err) on failure.
func (r *PackageRevisionReconciler) reconcileSubpackageOperation(ctx context.Context, pr *porchv1alpha2.PackageRevision, repoKey key.RepositoryKey) (*ctrl.Result, error) {
	subpackageResources, subpackageOperationType, err := r.applySubpackageOperation(ctx, pr)
	if err != nil {
		return nil, r.setFailedConditionsAndLog(ctx, pr, subpackageOperationType, err)
	}
	if subpackageResources == nil {
		return nil, nil
	}

	kptFile, err := kptfileko.NewFromPackage(subpackageResources)
	if err != nil {
		return nil, r.setFailedConditionsAndLog(ctx, pr, subpackageOperationType, pkgerrors.Wrap(err, "failed to parse subpackage Kptfile"))
	}

	subpackageName, err := porchapi.ComposeSubpkgObjName(pr.Spec.SubpackageOperation.SubpackageDir)
	if err != nil {
		return nil, r.setFailedConditionsAndLog(ctx, pr, subpackageOperationType, pkgerrors.Wrap(err, "failed to compose subpackage name for subpackage"))
	}

	if err := kptFile.SetName(subpackageName); err != nil {
		return nil,
			r.setFailedConditionsAndLog(ctx, pr, subpackageOperationType,
				pkgerrors.Wrapf(err, "failed to write package name %q to subpackage Kptfile", path.Base(pr.Spec.SubpackageOperation.SubpackageDir)))
	}

	if err := kptFile.WriteToPackage(subpackageResources); err != nil {
		return nil, pkgerrors.Wrapf(err, "failed to write to subpackage Kptfile %q", path.Join(pr.Spec.SubpackageOperation.SubpackageDir, kptfilev1.KptFileName))
	}

	log := log.FromContext(ctx)
	log.Info("applying operation", "type", subpackageOperationType, "name", pr.Name)

	parentResources, err := r.getPackageResources(ctx, pr)
	if err != nil {
		return nil, r.setFailedConditionsAndLog(ctx, pr, subpackageOperationType, pkgerrors.Wrapf(err, "failed to read parent resources"))
	}

	parentResources, err = r.upsertSubpackageResourcesInDraftResources(ctx, pr, parentResources, subpackageResources)
	if err != nil {
		return nil, r.setFailedConditionsAndLog(ctx, pr, subpackageOperationType, pkgerrors.Wrapf(err, "failed to upsert subpackage resources into parent resources"))
	}

	draft, err := r.ContentCache.CreateDraftFromExisting(ctx, repoKey, pr.Spec.PackageName, pr.Spec.WorkspaceName)
	if err != nil {
		return nil, r.setFailedConditionsAndLog(ctx, pr, subpackageOperationType, pkgerrors.Wrapf(err, "create draft on existing package revision"))
	}

	return r.finalizeDraftAndUpdateStatus(ctx, pr, repoKey, draft, parentResources, "", r.getSubpackageOperationHash(pr))
}

// verifyRepoMigrated errors if the target repo lacks the v1alpha2-migration annotation.
func (r *PackageRevisionReconciler) verifyRepoMigrated(ctx context.Context, repoKey key.RepositoryKey) error {
	var repo configapi.Repository
	if err := r.Get(ctx, types.NamespacedName{Namespace: repoKey.Namespace, Name: repoKey.Name}, &repo); err != nil {
		return fmt.Errorf("get repository %s/%s: %w", repoKey.Namespace, repoKey.Name, err)
	}
	if repo.Annotations[configapi.AnnotationKeyV1Alpha2Migration] != configapi.AnnotationValueMigrationEnabled {
		return fmt.Errorf("repository %s/%s not enabled for v1alpha2", repoKey.Namespace, repoKey.Name)
	}
	return nil
}

// finalizeDraftAndUpdateStatus completes the draft operation by updating resources,
// closing the draft, and updating the package revision status.
// The status patch (which records CreationSource / LastSubpackageOperationHash) is
// retried: if it fails the function returns an error so the work-queue retries the
// whole reconcile rather than requeueing with stale completion state, which would
// cause a replay of the already-committed git mutation.
func (r *PackageRevisionReconciler) finalizeDraftAndUpdateStatus(
	ctx context.Context,
	pr *porchv1alpha2.PackageRevision,
	repoKey key.RepositoryKey,
	draft repository.PackageRevisionDraftSlim,
	resources map[string]string,
	creationSource string,
	subpackageOperationHash string) (*ctrl.Result, error) {
	log := log.FromContext(ctx)

	operationType := creationSource
	if operationType == "" {
		operationType = subpackageOperationHash
	}
	if err := draft.UpdateResources(ctx, resources, operationType); err != nil {
		return nil, r.setFailedConditionsAndLog(ctx, pr, operationType, pkgerrors.Wrapf(err, "update resources"))
	}

	if err := r.ContentCache.CloseDraft(ctx, repoKey, draft, 0); err != nil {
		return nil, r.setFailedConditionsAndLog(ctx, pr, operationType, pkgerrors.Wrapf(err, "close draft"))
	}

	// Read back the created package to get lock info for status.
	content, err := r.ContentCache.GetPackageContent(ctx, repoKey, pr.Spec.PackageName, pr.Spec.WorkspaceName)
	if err != nil {
		log.Error(err, "failed to read back package content after source execution")
	}

	// Persist completion markers (CreationSource / LastSubpackageOperationHash) with
	// retry. These are the idempotency guards that prevent the committed git mutation
	// from being replayed on the next reconcile. A transient patch failure must not
	// cause a requeue with stale state.
	if err := r.updateStatusWithRetry(ctx, pr, content, creationSource, subpackageOperationHash,
		readyCondition(pr.Generation, metav1.ConditionFalse, porchv1alpha2.ReasonPending, "awaiting render")); err != nil {
		return nil, pkgerrors.Wrap(err, "failed to persist completion status after draft close")
	}
	// Set Rendered=Unknown via the render field manager. If this patch fails we must
	// still requeue (below) so the render step is retried: the draft content is already
	// committed to git and CreationSource is persisted, so the follow-up reconcile will
	// skip the (idempotent) source step and run reconcileRender. The source trigger
	// (CreationSource set && Rendered != True) fires regardless of whether the
	// Rendered=Unknown marker landed, so a dropped marker does not prevent recovery —
	// but we log it to surface transient apiserver pressure under load.
	if err := r.updateRenderStatus(ctx, pr, "", "",
		renderedCondition(pr.Generation, metav1.ConditionUnknown, porchv1alpha2.ReasonPending, "awaiting render")); err != nil {
		log.Error(err, "failed to set Rendered=Unknown marker after draft close; render will be retried via requeue")
	}
	// Note: the latest-revision label is backfilled at the top of Reconcile
	// (self-healing), so it is not set here. That makes it robust to this
	// reconcile being cancelled mid-way under load.

	telemetry.RecordControllerOperation(telemetry.ResourcePackageRevision, "CREATE", time.Now())

	// Requeue with a delay rather than a bare Requeue so the render step is reliably
	// retried. A bare ctrl.Result{Requeue: true} is a best-effort re-enqueue that can
	// be dropped from the workqueue under heavy load; because this draft now has
	// observedGeneration == generation, no watch event (GenerationChangedPredicate)
	// will wake it again, so a dropped requeue leaves it permanently stuck in
	// "awaiting render". RequeueAfter guarantees the item is re-enqueued so
	// reconcileRender runs. The follow-up reconcile is idempotent (source is skipped,
	// render clears the condition and stops requeuing once Rendered=True).
	result := ctrl.Result{RequeueAfter: renderRetryRequeueDelay}
	return &result, nil
}

// reconcileRender checks if rendering is needed and renders if so.
// Two triggers:
//   - Annotation porch.kpt.dev/render-request differs from status.observedPrrResourceVersion (push path)
//   - Source was executed but Rendered != True (source execution path)
//
// Returns (nil, nil) if no render is needed.
// TODO: Consider centralising all ctrl.Result creation in packagerevision_controller.go
// so requeue decisions are visible in one place. Sub-functions would return signals
// and the controller translates them into ctrl.Result.
func (r *PackageRevisionReconciler) reconcileRender(ctx context.Context, pr *porchv1alpha2.PackageRevision, repoKey key.RepositoryKey) (*ctrl.Result, error) {
	if r.Renderer == nil {
		return nil, nil
	}

	requested, annotationTrigger, sourceTrigger := renderTrigger(pr)
	if !annotationTrigger && !sourceTrigger {
		r.refreshRenderedGeneration(ctx, pr)
		return nil, nil
	}

	log := log.FromContext(ctx)
	log.Info("render requested", "requested", requested)
	// Best-effort "rendering" marker; a failure here is non-fatal because the
	// render proceeds below and writes the terminal Rendered=True/False condition.
	_ = r.updateRenderStatus(ctx, pr, requested, "",
		renderedCondition(pr.Generation, metav1.ConditionUnknown, porchv1alpha2.ReasonPending, "rendering"),
	)

	result, err := r.executeRender(ctx, pr, repoKey)
	if err != nil {
		return nil, err
	}
	if result != nil {
		return result, nil
	}

	observed := observedVersionAfterRender(requested, pr.Annotations)

	_, err = r.verifyResourcesAvailable(ctx, repoKey, pr)
	if err != nil {
		log.Error(err, "resources not available after render - render verification failed")
		r.setRenderFailed(ctx, pr, err)
		return nil, err
	}

	if err := r.updateRenderStatus(ctx, pr, "", observed,
		renderedCondition(pr.Generation, metav1.ConditionTrue, porchv1alpha2.ReasonRendered, ""),
	); err != nil {
		// The render itself succeeded and resources are written, but the terminal
		// Rendered=True condition failed to persist. Requeue so it is retried;
		// otherwise the package would stay at Rendered=Unknown "rendering" with no
		// watch event to wake it (same stuck-draft failure mode as "awaiting render").
		log.Error(err, "failed to persist Rendered=True condition; requeuing to retry")
		return &ctrl.Result{RequeueAfter: renderRetryRequeueDelay}, nil
	}
	return nil, nil
}

func (r *PackageRevisionReconciler) Name() string { return reconcilerName }

func (r *PackageRevisionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	log := ctrl.Log.WithName(r.Name())

	r.Client = mgr.GetClient()
	r.apiReader = mgr.GetAPIReader()

	if r.MaxConcurrentRenders > 0 {
		r.renderLimiter = make(chan struct{}, r.MaxConcurrentRenders)
	}

	if err := setupFieldIndexes(mgr); err != nil {
		return pkgerrors.Wrapf(err, "failed to setup field indexes")
	}

	err := ctrl.NewControllerManagedBy(mgr).
		For(&porchv1alpha2.PackageRevision{}).
		WithEventFilter(predicate.Or(
			predicate.GenerationChangedPredicate{},
			renderRequestChanged(),
		)).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: r.MaxConcurrentReconciles,
		}).
		Named("packagerevision").
		Complete(r)

	if err == nil {
		log.V(1).Info("PackageRevision controller successfully registered")
	}
	return err
}
