// Copyright 2022, 2024 The kpt and Nephio Authors
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
	cache "github.com/nephio-project/porch/pkg/cache"
	"github.com/nephio-project/porch/pkg/meta"
	"github.com/nephio-project/porch/pkg/repository"
	"github.com/nephio-project/porch/pkg/task"
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
	cache cache.Cache

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

	if err := repository.ValidateWorkspaceName(obj.Spec.WorkspaceName); err != nil {
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
	return &PackageRevision{
		repoPackageRevision: repoPkgRev,
	}, renderStatus, nil
}

// applyResourceMutations mutates the resources and returns the most recent renderResult.
func applyResourceMutations(ctx context.Context, draft repository.PackageDraft, baseResources repository.PackageResources, mutations []mutation) (applied repository.PackageResources, renderStatus *api.RenderStatus, err error) {
	var lastApplied mutation
	for _, m := range mutations {
		updatedResources, taskResult, err := m.Apply(ctx, baseResources)
		if taskResult == nil && err == nil {
			// a nil taskResult means nothing changed
			baseResources = updatedResources
			applied = updatedResources
			continue
		}

		var task *api.Task
		if taskResult != nil {
			task = taskResult.Task
		}
		if taskResult != nil && task.Type == api.TaskTypeEval {
			renderStatus = taskResult.RenderStatus
		}
		if err != nil {
			return updatedResources, renderStatus, err
		}

		// if the last applied mutation was a render mutation, and so is this one, skip it
		if lastApplied != nil && isRenderMutation(m) && isRenderMutation(lastApplied) {
			continue
		}
		lastApplied = m

		if err := draft.UpdateResources(ctx, &api.PackageRevisionResources{
			Spec: api.PackageRevisionResourcesSpec{
				Resources: updatedResources.Contents,
			},
		}, task); err != nil {
			return updatedResources, renderStatus, err
		}
		baseResources = updatedResources
		applied = updatedResources
	}

	return applied, renderStatus, nil
}

type updatePackageMutation struct {
	cloneTask         *api.Task
	updateTask        *api.Task
	repoOpener        RepositoryOpener
	referenceResolver ReferenceResolver
	namespace         string
	pkgName           string
}

func (m *updatePackageMutation) Apply(ctx context.Context, resources repository.PackageResources) (repository.PackageResources, *api.TaskResult, error) {
	ctx, span := tracer.Start(ctx, "updatePackageMutation::Apply", trace.WithAttributes())
	defer span.End()

	currUpstreamPkgRef, err := m.currUpstream()
	if err != nil {
		return repository.PackageResources{}, nil, err
	}

	targetUpstream := m.updateTask.Update.Upstream
	if targetUpstream.Type == api.RepositoryTypeGit || targetUpstream.Type == api.RepositoryTypeOCI || targetUpstream.Type == api.RepositoryTypeDB {
		return repository.PackageResources{}, nil, fmt.Errorf("update is not supported for non-porch upstream packages")
	}

	originalResources, err := (&PackageFetcher{
		repoOpener:        m.repoOpener,
		referenceResolver: m.referenceResolver,
	}).FetchResources(ctx, currUpstreamPkgRef, m.namespace)
	if err != nil {
		return repository.PackageResources{}, nil, fmt.Errorf("error fetching the resources for package %s with ref %+v",
			m.pkgName, *currUpstreamPkgRef)
	}

	upstreamRevision, err := (&PackageFetcher{
		repoOpener:        m.repoOpener,
		referenceResolver: m.referenceResolver,
	}).FetchRevision(ctx, targetUpstream.UpstreamRef, m.namespace)
	if err != nil {
		return repository.PackageResources{}, nil, fmt.Errorf("error fetching revision for target upstream %s", targetUpstream.UpstreamRef.Name)
	}
	upstreamResources, err := upstreamRevision.GetResources(ctx)
	if err != nil {
		return repository.PackageResources{}, nil, fmt.Errorf("error fetching resources for target upstream %s", targetUpstream.UpstreamRef.Name)
	}

	klog.Infof("performing pkg upgrade operation for pkg %s resource counts local[%d] original[%d] upstream[%d]",
		m.pkgName, len(resources.Contents), len(originalResources.Spec.Resources), len(upstreamResources.Spec.Resources))

	// May be have packageUpdater part of engine to make it easy for testing ?
	updatedResources, err := (&defaultPackageUpdater{}).Update(ctx,
		resources,
		repository.PackageResources{
			Contents: originalResources.Spec.Resources,
		},
		repository.PackageResources{
			Contents: upstreamResources.Spec.Resources,
		})
	if err != nil {
		return repository.PackageResources{}, nil, fmt.Errorf("error updating the package to revision %s", targetUpstream.UpstreamRef.Name)
	}

	newUpstream, newUpstreamLock, err := upstreamRevision.GetLock()
	if err != nil {
		return repository.PackageResources{}, nil, fmt.Errorf("error fetching the resources for package revisions %s", targetUpstream.UpstreamRef.Name)
	}
	if err := kpt.UpdateKptfileUpstream("", updatedResources.Contents, newUpstream, newUpstreamLock); err != nil {
		return repository.PackageResources{}, nil, fmt.Errorf("failed to apply upstream lock to package %q: %w", m.pkgName, err)
	}

	// ensure merge-key comment is added to newly added resources.
	result, err := ensureMergeKey(ctx, updatedResources)
	if err != nil {
		klog.Infof("failed to add merge key comments: %v", err)
	}
	return result, &api.TaskResult{Task: m.updateTask}, nil
}

// Currently assumption is that downstream packages will be forked from a porch package.
// As per current implementation, upstream package ref is stored in a new update task but this may
// change so the logic of figuring out current upstream will live in this function.
func (m *updatePackageMutation) currUpstream() (*api.PackageRevisionRef, error) {
	if m.cloneTask == nil || m.cloneTask.Clone == nil {
		return nil, fmt.Errorf("package %s does not have original upstream info", m.pkgName)
	}
	upstream := m.cloneTask.Clone.Upstream
	if upstream.Type == api.RepositoryTypeGit || upstream.Type == api.RepositoryTypeOCI || upstream.Type == api.RepositoryTypeDB {
		return nil, fmt.Errorf("upstream package must be porch native package. Found it to be %s", upstream.Type)
	}
	return upstream.UpstreamRef, nil
}

func findCloneTask(pr *api.PackageRevision) *api.Task {
	if len(pr.Spec.Tasks) == 0 {
		return nil
	}
	firstTask := pr.Spec.Tasks[0]
	if firstTask.Type == api.TaskTypeClone {
		return &firstTask
	}
	return nil
}

func writeResourcesToDirectory(dir string, resources repository.PackageResources) error {
	for k, v := range resources.Contents {
		p := filepath.Join(dir, k)
		dir := filepath.Dir(p)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %q: %w", dir, err)
		}
		if err := os.WriteFile(p, []byte(v), 0644); err != nil {
			return fmt.Errorf("failed to write file %q: %w", dir, err)
		}
	}
	return nil
}

func loadResourcesFromDirectory(dir string) (repository.PackageResources, error) {
	// TODO: return abstraction instead of loading everything
	result := repository.PackageResources{
		Contents: map[string]string{},
	}
	if err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return fmt.Errorf("cannot compute relative path %q, %q, %w", dir, path, err)
		}

		contents, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("cannot read file %q: %w", dir, err)
		}
		result.Contents[rel] = string(contents)
		return nil
	}); err != nil {
		return repository.PackageResources{}, err
	}

	return result, nil
}

type mutationReplaceResources struct {
	newResources *api.PackageRevisionResources
	oldResources *api.PackageRevisionResources
}

func (m *mutationReplaceResources) Apply(ctx context.Context, resources repository.PackageResources) (repository.PackageResources, *api.TaskResult, error) {
	ctx, span := tracer.Start(ctx, "mutationReplaceResources::Apply", trace.WithAttributes())
	defer span.End()

	patch := &api.PackagePatchTaskSpec{}

	old := resources.Contents
	new, err := healConfig(old, m.newResources.Spec.Resources)
	if err != nil {
		return repository.PackageResources{}, nil, fmt.Errorf("failed to heal resources: %w", err)
	}

	for k, newV := range new {
		oldV, ok := old[k]
		// New config or changed config
		if !ok {
			patchSpec := api.PatchSpec{
				File:      k,
				PatchType: api.PatchTypeCreateFile,
				Contents:  newV,
			}
			patch.Patches = append(patch.Patches, patchSpec)
		} else if newV != oldV {
			patchSpec, err := GeneratePatch(k, oldV, newV)
			if err != nil {
				return repository.PackageResources{}, nil, fmt.Errorf("error generating patch: %w", err)
			}
			if patchSpec.Contents == "" {
				continue
			}
			patch.Patches = append(patch.Patches, patchSpec)
		}
	}
	for k := range old {
		// Deleted config
		if _, ok := new[k]; !ok {
			patchSpec := api.PatchSpec{
				File:      k,
				PatchType: api.PatchTypeDeleteFile,
			}
			patch.Patches = append(patch.Patches, patchSpec)
		}
	}
	// If patch is empty, don't create a Task.
	var taskResult *api.TaskResult
	if len(patch.Patches) > 0 {
		taskResult = &api.TaskResult{
			Task: &api.Task{
				Type:  api.TaskTypePatch,
				Patch: patch,
			},
		}
	}
	return repository.PackageResources{Contents: new}, taskResult, nil
}

func healConfig(old, new map[string]string) (map[string]string, error) {
	// Copy comments from old config to new
	oldResources, err := (&packageReader{
		input: repository.PackageResources{Contents: old},
		extra: map[string]string{},
	}).Read()
	if err != nil {
		return nil, fmt.Errorf("failed to read old packge resources: %w", err)
	}

	var filter kio.FilterFunc = func(r []*yaml.RNode) ([]*yaml.RNode, error) {
		for _, n := range r {
			for _, original := range oldResources {
				if n.GetNamespace() == original.GetNamespace() &&
					n.GetName() == original.GetName() &&
					n.GetApiVersion() == original.GetApiVersion() &&
					n.GetKind() == original.GetKind() {
					comments.CopyComments(original, n)
				}
			}
		}
		return r, nil
	}

	out := &packageWriter{
		output: repository.PackageResources{
			Contents: map[string]string{},
		},
	}

	extra := map[string]string{}

	if err := (kio.Pipeline{
		Inputs: []kio.Reader{&packageReader{
			input: repository.PackageResources{Contents: new},
			extra: extra,
		}},
		Filters:               []kio.Filter{filter},
		Outputs:               []kio.Writer{out},
		ContinueOnEmptyResult: true,
	}).Execute(); err != nil {
		return nil, err
	}

	healed := out.output.Contents

	for k, v := range extra {
		healed[k] = v
	}

	return healed, nil
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
