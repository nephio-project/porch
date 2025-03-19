// Copyright 2022, 2024-2025 The kpt and Nephio Authors
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
	"errors"
	"fmt"
	"reflect"
	"strings"

	api "github.com/nephio-project/porch/api/porch/v1alpha1"
	configapi "github.com/nephio-project/porch/api/porchconfig/v1alpha1"
	cachetypes "github.com/nephio-project/porch/pkg/cache/types"
	"github.com/nephio-project/porch/pkg/meta"
	"github.com/nephio-project/porch/pkg/repository"
	"github.com/nephio-project/porch/pkg/task"
	"github.com/nephio-project/porch/pkg/util"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/klog/v2"
)

var tracer = otel.Tracer("engine")

const (
	OptimisticLockErrorMsg = "the object has been modified; please apply your changes to the latest version and try again"
)

type CaDEngine interface {
	// ObjectCache() is a cache of all our objects.
	ObjectCache() WatcherManager

	UpdatePackageResources(ctx context.Context, repositoryObj *configapi.Repository, oldPackage repository.PackageRevision, old, new *api.PackageRevisionResources) (repository.PackageRevision, *api.RenderStatus, error)

	ListPackageRevisions(ctx context.Context, repositorySpec *configapi.Repository, filter repository.ListPackageRevisionFilter) ([]repository.PackageRevision, error)
	CreatePackageRevision(ctx context.Context, repositoryObj *configapi.Repository, obj *api.PackageRevision, parent repository.PackageRevision) (repository.PackageRevision, error)
	UpdatePackageRevision(ctx context.Context, version string, repositoryObj *configapi.Repository, oldPackage repository.PackageRevision, old, new *api.PackageRevision, parent repository.PackageRevision) (repository.PackageRevision, error)
	DeletePackageRevision(ctx context.Context, repositoryObj *configapi.Repository, obj repository.PackageRevision) error

	ListPackages(ctx context.Context, repositorySpec *configapi.Repository, filter repository.ListPackageFilter) ([]repository.Package, error)
	CreatePackage(ctx context.Context, repositoryObj *configapi.Repository, obj *api.PorchPackage) (repository.Package, error)
	UpdatePackage(ctx context.Context, repositoryObj *configapi.Repository, oldPackage repository.Package, old, new *api.PorchPackage) (repository.Package, error)
	DeletePackage(ctx context.Context, repositoryObj *configapi.Repository, obj repository.Package) error
}

func NewCaDEngine(opts ...EngineOption) (CaDEngine, error) {
	engine := &cadEngine{
		taskHandler: task.GetDefaultTaskHandler(),
	}

	for _, opt := range opts {
		if err := opt.apply(engine); err != nil {
			return nil, err
		}
	}
	return engine, nil
}

type cadEngine struct {
	cache cachetypes.Cache

	userInfoProvider repository.UserInfoProvider
	metadataStore    meta.MetadataStore
	watcherManager   *watcherManager
	taskHandler      task.TaskHandler
}

var _ CaDEngine = &cadEngine{}

// ObjectCache is a cache of all our objects.
func (cad *cadEngine) ObjectCache() WatcherManager {
	return cad.watcherManager
}

func (cad *cadEngine) OpenRepository(ctx context.Context, repositorySpec *configapi.Repository) (repository.Repository, error) {
	ctx, span := tracer.Start(ctx, "cadEngine::OpenRepository", trace.WithAttributes())
	defer span.End()

	return cad.cache.OpenRepository(ctx, repositorySpec)
}

func (cad *cadEngine) ListPackageRevisions(ctx context.Context, repositorySpec *configapi.Repository, filter repository.ListPackageRevisionFilter) ([]repository.PackageRevision, error) {
	ctx, span := tracer.Start(ctx, "cadEngine::ListPackageRevisions", trace.WithAttributes())
	defer span.End()

	repo, err := cad.cache.OpenRepository(ctx, repositorySpec)
	if err != nil {
		return nil, err
	}
	pkgRevs, err := repo.ListPackageRevisions(ctx, filter)
	if err != nil {
		return nil, err
	}

	var packageRevisions []repository.PackageRevision
	for _, pr := range pkgRevs {
		pkgRevMeta, err := cad.metadataStore.Get(ctx, types.NamespacedName{
			Name:      pr.KubeObjectName(),
			Namespace: pr.KubeObjectNamespace(),
		})
		if err != nil {
			// If a PackageRev CR doesn't exist, we treat the
			// Packagerevision as not existing.
			if apierrors.IsNotFound(err) {
				continue
			}
			return nil, err
		}
		pr.SetMeta(pkgRevMeta)
		packageRevisions = append(packageRevisions, pr)
	}
	return packageRevisions, nil
}

func (cad *cadEngine) CreatePackageRevision(ctx context.Context, repositoryObj *configapi.Repository, obj *api.PackageRevision, parent repository.PackageRevision) (repository.PackageRevision, error) {
	ctx, span := tracer.Start(ctx, "cadEngine::CreatePackageRevision", trace.WithAttributes())
	defer span.End()

	packageConfig, err := repository.BuildPackageConfig(ctx, obj, parent)
	if err != nil {
		return nil, err
	}

	// Validate package lifecycle. Cannot create a final package
	switch obj.Spec.Lifecycle {
	case "":
		// Set draft as default
		obj.Spec.Lifecycle = api.PackageRevisionLifecycleDraft
	case api.PackageRevisionLifecycleDraft, api.PackageRevisionLifecycleProposed:
		// These values are ok
	case api.PackageRevisionLifecyclePublished, api.PackageRevisionLifecycleDeletionProposed:
		// TODO: generate errors that can be translated to correct HTTP responses
		return nil, fmt.Errorf("cannot create a package revision with lifecycle value 'Final'")
	default:
		return nil, fmt.Errorf("unsupported lifecycle value: %s", obj.Spec.Lifecycle)
	}

	repo, err := cad.cache.OpenRepository(ctx, repositoryObj)
	if err != nil {
		return nil, err
	}

	if err := util.ValidPkgRevObjName(repositoryObj.ObjectMeta.Name, repositoryObj.Spec.Git.Directory, obj.Spec.PackageName, string(obj.Spec.WorkspaceName)); err != nil {
		return nil, fmt.Errorf("failed to create packagerevision: %w", err)
	}

	revs, err := repo.ListPackageRevisions(ctx, repository.ListPackageRevisionFilter{
		Package: obj.Spec.PackageName})
	if err != nil {
		return nil, fmt.Errorf("error listing package revisions: %w", err)
	}

	if err := ensureUniqueWorkspaceName(obj, revs); err != nil {
		return nil, err
	}

	draft, err := repo.CreatePackageRevision(ctx, obj)
	if err != nil {
		return nil, err
	}

	if err := cad.taskHandler.ApplyTasks(ctx, draft, repositoryObj, obj, packageConfig); err != nil {
		return nil, err
	}

	if err := draft.UpdateLifecycle(ctx, obj.Spec.Lifecycle); err != nil {
		return nil, err
	}

	// Updates are done.
	repoPkgRev, err := repo.ClosePackageRevisionDraft(ctx, draft, "")
	if err != nil {
		return nil, err
	}
	pkgRevMeta := metav1.ObjectMeta{
		Name:            repoPkgRev.KubeObjectName(),
		Namespace:       repoPkgRev.KubeObjectNamespace(),
		Labels:          obj.Labels,
		Annotations:     obj.Annotations,
		Finalizers:      obj.Finalizers,
		OwnerReferences: obj.OwnerReferences,
	}
	pkgRevMeta, err = cad.metadataStore.Create(ctx, pkgRevMeta, repositoryObj.Name, repoPkgRev.UID())
	if err != nil {
		if (apierrors.IsUnauthorized(err) || apierrors.IsForbidden(err)) && repository.AnyBlockOwnerDeletionSet(obj) {
			return nil, fmt.Errorf("failed to create internal PackageRev object, because blockOwnerDeletion is enabled for some ownerReference "+
				"(it is likely that the serviceaccount of porch-server does not have the rights to update finalizers in the owner object): %w", err)
		}
		return nil, err
	}
	repoPkgRev.SetMeta(pkgRevMeta)
	sent := cad.watcherManager.NotifyPackageRevisionChange(watch.Added, repoPkgRev)
	klog.Infof("engine: sent %d for new PackageRevision %s/%s", sent, repoPkgRev.KubeObjectNamespace(), repoPkgRev.KubeObjectName())
	return repoPkgRev, nil
}

// The workspaceName must be unique, because it used to generate the package revision's metadata.name.
func ensureUniqueWorkspaceName(obj *api.PackageRevision, existingRevs []repository.PackageRevision) error {
	for _, r := range existingRevs {
		k := r.Key()
		if k.WorkspaceName == obj.Spec.WorkspaceName {
			return fmt.Errorf("package revision workspaceNames must be unique; package revision with name %s in repo %s with "+
				"workspaceName %s already exists", obj.Spec.PackageName, obj.Spec.RepositoryName, obj.Spec.WorkspaceName)
		}
	}
	return nil
}

func (cad *cadEngine) UpdatePackageRevision(ctx context.Context, version string, repositoryObj *configapi.Repository, repoPr repository.PackageRevision, oldObj, newObj *api.PackageRevision, parent repository.PackageRevision) (repository.PackageRevision, error) {
	ctx, span := tracer.Start(ctx, "cadEngine::UpdatePackageRevision", trace.WithAttributes())
	defer span.End()

	newRV := newObj.GetResourceVersion()
	if len(newRV) == 0 {
		return nil, fmt.Errorf("resourceVersion must be specified for an update")
	}

	if newRV != oldObj.GetResourceVersion() {
		return nil, apierrors.NewConflict(api.Resource("packagerevisions"), oldObj.GetName(), fmt.Errorf("%s", OptimisticLockErrorMsg))
	}

	repo, err := cad.cache.OpenRepository(ctx, repositoryObj)
	if err != nil {
		return nil, err
	}

	// Check if the PackageRevision is in the terminating state and
	// and this request removes the last finalizer.
	repoPkgRev := repoPr
	pkgRevMetaNN := types.NamespacedName{
		Name:      repoPkgRev.KubeObjectName(),
		Namespace: repoPkgRev.KubeObjectNamespace(),
	}
	pkgRevMeta, err := cad.metadataStore.Get(ctx, pkgRevMetaNN)
	if err != nil {
		return nil, err
	}
	repoPkgRev.SetMeta(pkgRevMeta)
	// If this is in the terminating state and we are removing the last finalizer,
	// we delete the resource instead of updating it.
	if pkgRevMeta.DeletionTimestamp != nil && len(newObj.Finalizers) == 0 {
		if err := cad.deletePackageRevision(ctx, repo, repoPkgRev); err != nil {
			return nil, err
		}
		return repoPkgRev, nil
	}

	// Validate package lifecycle. Can only update a draft.
	switch lifecycle := oldObj.Spec.Lifecycle; lifecycle {

	case api.PackageRevisionLifecycleDraft, api.PackageRevisionLifecycleProposed:
		// Draft or proposed can be updated.

	case api.PackageRevisionLifecyclePublished, api.PackageRevisionLifecycleDeletionProposed:
		// Only metadata (currently labels and annotations) and lifecycle can be updated for published packages.
		if oldObj.Spec.Lifecycle != newObj.Spec.Lifecycle {
			if err := repoPr.UpdateLifecycle(ctx, newObj.Spec.Lifecycle); err != nil {
				return nil, err
			}
		}

		err = cad.updatePkgRevMeta(ctx, repoPkgRev, newObj)
		if err != nil {
			return nil, err
		}
		sent := cad.watcherManager.NotifyPackageRevisionChange(watch.Modified, repoPkgRev)
		klog.Infof("engine: sent %d for updated PackageRevision metadata %s/%s", sent, repoPkgRev.KubeObjectNamespace(), repoPkgRev.KubeObjectName())
		return repoPkgRev, nil

	default:
		return nil, fmt.Errorf("invalid original lifecycle value: %q", lifecycle)
	}

	switch lifecycle := newObj.Spec.Lifecycle; lifecycle {

	case api.PackageRevisionLifecycleDraft, api.PackageRevisionLifecycleProposed, api.PackageRevisionLifecyclePublished, api.PackageRevisionLifecycleDeletionProposed:
		// These values are ok

	default:
		return nil, fmt.Errorf("invalid desired lifecycle value: %q", lifecycle)
	}

	if isRecloneAndReplay(oldObj, newObj) {
		return cad.RecloneAndReplay(ctx, parent, version, repo, repositoryObj, newObj)
	}

	// Do we need to clean up this draft later?
	draft, err := repo.UpdatePackageRevision(ctx, repoPr)
	if err != nil {
		return nil, err
	}

	if err := cad.taskHandler.DoPRMutations(ctx, repositoryObj.Namespace, repoPr, oldObj, newObj, draft); err != nil {
		return nil, err
	}

	if err := draft.UpdateLifecycle(ctx, newObj.Spec.Lifecycle); err != nil {
		return nil, err
	}

	// Updates are done.
	repoPkgRev, err = repo.ClosePackageRevisionDraft(ctx, draft, version)
	if err != nil {
		return nil, err
	}

	err = cad.updatePkgRevMeta(ctx, repoPkgRev, newObj)
	if err != nil {
		if (apierrors.IsUnauthorized(err) || apierrors.IsForbidden(err)) && repository.AnyBlockOwnerDeletionSet(newObj) {
			return nil, fmt.Errorf("failed to update internal PackageRev object, because blockOwnerDeletion is enabled for some ownerReference "+
				"(it is likely that the serviceaccount of porch-server does not have the rights to update finalizers in the owner object): %w", err)
		}
		return nil, err
	}

	sent := cad.watcherManager.NotifyPackageRevisionChange(watch.Modified, repoPkgRev)
	klog.Infof("engine: sent %d for updated PackageRevision %s/%s", sent, repoPkgRev.KubeObjectNamespace(), repoPkgRev.KubeObjectName())

	return repoPkgRev, nil
}

func (cad *cadEngine) updatePkgRevMeta(ctx context.Context, repoPkgRev repository.PackageRevision, apiPkgRev *api.PackageRevision) error {
	pkgRevMeta := metav1.ObjectMeta{
		Name:            repoPkgRev.KubeObjectName(),
		Namespace:       repoPkgRev.KubeObjectNamespace(),
		Labels:          apiPkgRev.Labels,
		Annotations:     apiPkgRev.Annotations,
		Finalizers:      apiPkgRev.Finalizers,
		OwnerReferences: apiPkgRev.OwnerReferences,
	}

	repoPkgRev.SetMeta(pkgRevMeta)

	if storedPkgRevMeta, err := cad.metadataStore.Update(ctx, pkgRevMeta); err == nil {
		repoPkgRev.SetMeta(storedPkgRevMeta)
		return nil
	} else {
		return err
	}
}

func (cad *cadEngine) DeletePackageRevision(ctx context.Context, repositoryObj *configapi.Repository, pr2Del repository.PackageRevision) error {
	ctx, span := tracer.Start(ctx, "cadEngine::DeletePackageRevision", trace.WithAttributes())
	defer span.End()

	repo, err := cad.cache.OpenRepository(ctx, repositoryObj)
	if err != nil {
		return err
	}

	// We delete the PackageRev regardless of any finalizers, since it
	// will always have the same finalizers as the PackageRevision. This
	// will put the PackageRev, and therefore the PackageRevision in the
	// terminating state.
	// But we only delete the PackageRevision from the repo once all finalizers
	// have been removed.
	namespacedName := types.NamespacedName{
		Name:      pr2Del.KubeObjectName(),
		Namespace: pr2Del.KubeObjectNamespace(),
	}
	pkgRevMeta, err := cad.metadataStore.Delete(ctx, namespacedName, false)
	if err != nil {
		return err
	}

	if len(pkgRevMeta.Finalizers) > 0 {
		klog.Infof("PackageRevision %s deleted, but still have finalizers: %s", pr2Del.KubeObjectName(), strings.Join(pkgRevMeta.Finalizers, ","))
		sent := cad.watcherManager.NotifyPackageRevisionChange(watch.Modified, pr2Del)
		klog.Infof("engine: sent %d modified for deleted PackageRevision %s/%s with finalizers", sent, pr2Del.KubeObjectNamespace(), pr2Del.KubeObjectName())
		return nil
	}
	klog.Infof("PackageRevision %s deleted for real since no finalizers", pr2Del.KubeObjectName())

	return cad.deletePackageRevision(ctx, repo, pr2Del)
}

func (cad *cadEngine) deletePackageRevision(ctx context.Context, repo repository.Repository, repoPkgRev repository.PackageRevision) error {
	ctx, span := tracer.Start(ctx, "cadEngine::deletePackageRevision", trace.WithAttributes())
	defer span.End()

	if err := repo.DeletePackageRevision(ctx, repoPkgRev); err != nil {
		return err
	}

	nn := types.NamespacedName{
		Name:      repoPkgRev.GetMeta().Name,
		Namespace: repoPkgRev.GetMeta().Namespace,
	}
	if _, err := cad.metadataStore.Delete(ctx, nn, true); err != nil {
		// If this fails, the CR will be cleaned up by the background job.
		if !apierrors.IsNotFound(err) {
			klog.Warningf("Error deleting PkgRevMeta %s: %v", nn.String(), err)
		}
	}

	sent := cad.watcherManager.NotifyPackageRevisionChange(watch.Deleted, repoPkgRev)
	klog.Infof("engine: sent %d for deleted PackageRevision %s/%s", sent, repoPkgRev.KubeObjectNamespace(), repoPkgRev.KubeObjectName())
	return nil
}

func (cad *cadEngine) ListPackages(ctx context.Context, repositorySpec *configapi.Repository, filter repository.ListPackageFilter) ([]repository.Package, error) {
	ctx, span := tracer.Start(ctx, "cadEngine::ListPackages", trace.WithAttributes())
	defer span.End()

	repo, err := cad.cache.OpenRepository(ctx, repositorySpec)
	if err != nil {
		return nil, err
	}

	pkgs, err := repo.ListPackages(ctx, filter)
	if err != nil {
		return nil, err
	}
	var packages []repository.Package
	packages = append(packages, pkgs...)

	return packages, nil
}

func (cad *cadEngine) CreatePackage(ctx context.Context, repositoryObj *configapi.Repository, obj *api.PorchPackage) (repository.Package, error) {
	ctx, span := tracer.Start(ctx, "cadEngine::CreatePackage", trace.WithAttributes())
	defer span.End()

	repo, err := cad.cache.OpenRepository(ctx, repositoryObj)
	if err != nil {
		return nil, err
	}
	pkg, err := repo.CreatePackage(ctx, obj)
	if err != nil {
		return nil, err
	}

	return pkg, nil
}

func (cad *cadEngine) UpdatePackage(ctx context.Context, repositoryObj *configapi.Repository, pkg2Update repository.Package, oldObj, newObj *api.PorchPackage) (repository.Package, error) {
	_, span := tracer.Start(ctx, "cadEngine::UpdatePackage", trace.WithAttributes())
	defer span.End()

	// TODO
	var pkg repository.Package
	return pkg, errors.New("updating packages is not yet supported")
}

func (cad *cadEngine) DeletePackage(ctx context.Context, repositoryObj *configapi.Repository, pkg2Del repository.Package) error {
	ctx, span := tracer.Start(ctx, "cadEngine::DeletePackage", trace.WithAttributes())
	defer span.End()

	repo, err := cad.cache.OpenRepository(ctx, repositoryObj)
	if err != nil {
		return err
	}

	if err := repo.DeletePackage(ctx, pkg2Del); err != nil {
		return err
	}

	return nil
}

func (cad *cadEngine) UpdatePackageResources(ctx context.Context, repositoryObj *configapi.Repository, pr2Update repository.PackageRevision, oldRes, newRes *api.PackageRevisionResources) (repository.PackageRevision, *api.RenderStatus, error) {
	ctx, span := tracer.Start(ctx, "cadEngine::UpdatePackageResources", trace.WithAttributes())
	defer span.End()

	rev, err := pr2Update.GetPackageRevision(ctx)
	if err != nil {
		return nil, nil, err
	}

	newRV := newRes.GetResourceVersion()
	if len(newRV) == 0 {
		return nil, nil, fmt.Errorf("resourceVersion must be specified for an update")
	}

	if newRV != oldRes.GetResourceVersion() {
		return nil, nil, apierrors.NewConflict(api.Resource("packagerevisionresources"), oldRes.GetName(), errors.New(OptimisticLockErrorMsg))
	}

	// Validate package lifecycle. Can only update a draft.
	switch lifecycle := rev.Spec.Lifecycle; lifecycle {
	default:
		return nil, nil, fmt.Errorf("invalid original lifecycle value: %q", lifecycle)
	case api.PackageRevisionLifecycleDraft:
		// Only drafts can be updated.
	case api.PackageRevisionLifecycleProposed, api.PackageRevisionLifecyclePublished, api.PackageRevisionLifecycleDeletionProposed:
		// TODO: generate errors that can be translated to correct HTTP responses
		return nil, nil, fmt.Errorf("cannot update a package revision with lifecycle value %q; package must be Draft", lifecycle)
	}

	repo, err := cad.cache.OpenRepository(ctx, repositoryObj)
	if err != nil {
		return nil, nil, err
	}
	draft, err := repo.UpdatePackageRevision(ctx, pr2Update)
	if err != nil {
		return nil, nil, err
	}

	renderStatus, err := cad.taskHandler.DoPRResourceMutations(ctx, pr2Update, draft, oldRes, newRes)
	if err != nil {
		return nil, renderStatus, err
	}
	// No lifecycle change when updating package resources; updates are done.
	repoPkgRev, err := repo.ClosePackageRevisionDraft(ctx, draft, "")
	if err != nil {
		return nil, renderStatus, err
	}
	repoPkgRev.SetMeta(rev.ObjectMeta)

	return repoPkgRev, renderStatus, nil
}

// isRecloneAndReplay determines if an update should be handled using reclone-and-replay semantics.
// We detect this by checking if both old and new versions start by cloning a package, but the version has changed.
// We may expand this scope in future.
func isRecloneAndReplay(oldObj, newObj *api.PackageRevision) bool {
	oldTasks := oldObj.Spec.Tasks
	newTasks := newObj.Spec.Tasks
	if len(oldTasks) == 0 || len(newTasks) == 0 {
		return false
	}

	if oldTasks[0].Type != api.TaskTypeClone || newTasks[0].Type != api.TaskTypeClone {
		return false
	}

	if reflect.DeepEqual(oldTasks[0], newTasks[0]) {
		return false
	}
	return true
}

// recloneAndReplay performs an update by recloning the upstream package and replaying all tasks.
// This is more like a git rebase operation than the "classic" kpt update algorithm, which is more like a git merge.
func (cad *cadEngine) RecloneAndReplay(ctx context.Context, parentPR repository.PackageRevision, version string, repo repository.Repository, repositoryObj *configapi.Repository, newObj *api.PackageRevision) (repository.PackageRevision, error) {
	ctx, span := tracer.Start(ctx, "cadEngine::recloneAndReplay", trace.WithAttributes())
	defer span.End()

	packageConfig, err := repository.BuildPackageConfig(ctx, newObj, parentPR)
	if err != nil {
		return nil, err
	}

	// For reclone and replay, we create a new package every time
	// the version should be in newObj so we will overwrite.
	draft, err := repo.CreatePackageRevision(ctx, newObj)
	if err != nil {
		return nil, err
	}

	if err := cad.taskHandler.ApplyTasks(ctx, draft, repositoryObj, newObj, packageConfig); err != nil {
		return nil, err
	}

	if err := draft.UpdateLifecycle(ctx, newObj.Spec.Lifecycle); err != nil {
		return nil, err
	}

	repoPkgRev, err := repo.ClosePackageRevisionDraft(ctx, draft, version)

	if err != nil {
		return nil, err
	}

	err = cad.updatePkgRevMeta(ctx, repoPkgRev, newObj)
	if err != nil {
		return nil, err
	}

	sent := cad.watcherManager.NotifyPackageRevisionChange(watch.Modified, repoPkgRev)
	klog.Infof("engine: sent %d for reclone and replay PackageRevision %s/%s", sent, repoPkgRev.KubeObjectNamespace(), repoPkgRev.KubeObjectName())
	return repoPkgRev, nil
}
