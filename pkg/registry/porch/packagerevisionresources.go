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

package porch

import (
	"context"
	"fmt"
	"sync"

	"k8s.io/apimachinery/pkg/runtime/schema"

	porchapi "github.com/nephio-project/porch/api/porch/v1alpha1"
	"github.com/nephio-project/porch/api/porchconfig/v1alpha1"
	"github.com/nephio-project/porch/pkg/repository"
	"go.opentelemetry.io/otel/trace"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metainternalversion "k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	genericapirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/klog/v2"
)

type packageRevisionResources struct {
	rest.TableConvertor
	packageCommon
}

var _ rest.Storage = &packageRevisionResources{}
var _ rest.Lister = &packageRevisionResources{}
var _ rest.Getter = &packageRevisionResources{}
var _ rest.Scoper = &packageRevisionResources{}
var _ rest.Updater = &packageRevisionResources{}
var _ rest.SingularNameProvider = &packageRevisionResources{}
var _ rest.Watcher = &packageRevisionResources{}

// GetSingularName implements the SingularNameProvider interface
func (r *packageRevisionResources) GetSingularName() string {
	return "packagerevisionresources"
}

func (r *packageRevisionResources) New() runtime.Object {
	return &porchapi.PackageRevisionResources{}
}

func (r *packageRevisionResources) Destroy() {}

func (r *packageRevisionResources) NewList() runtime.Object {
	return &porchapi.PackageRevisionResourcesList{}
}

func (r *packageRevisionResources) NamespaceScoped() bool {
	return true
}

// List selects resources in the storage which match to the selector. 'options' can be nil.
func (r *packageRevisionResources) List(ctx context.Context, options *metainternalversion.ListOptions) (runtime.Object, error) {
	ctx, span := tracer.Start(ctx, "[START]::packageRevisionResources::List", trace.WithAttributes())
	defer span.End()

	result := &porchapi.PackageRevisionResourcesList{
		TypeMeta: metav1.TypeMeta{
			Kind:       "PackageRevisionResourcesList",
			APIVersion: porchapi.SchemeGroupVersion.Identifier(),
		},
	}

	filter, err := parsePackageRevisionResourcesFieldSelector(options)
	if err != nil {
		return nil, err
	}

	if err := r.listPackageRevisions(ctx, *filter, func(ctx context.Context, p repository.PackageRevision) error {
		apiPkgResources, err := p.GetResources(ctx)
		if err != nil {
			return err
		}
		result.Items = append(result.Items, *apiPkgResources)
		return nil
	}); err != nil {
		return nil, err
	}

	klog.V(3).Infof("List packagerevisionresources completed: found %d items", len(result.Items))

	return result, nil
}

// Get implements the Getter interface
func (r *packageRevisionResources) Get(ctx context.Context, name string, options *metav1.GetOptions) (runtime.Object, error) {
	ctx, span := tracer.Start(ctx, "[START]::packageRevisionResources::Get", trace.WithAttributes())
	defer span.End()

	pkg, err := r.getRepoPkgRev(ctx, name)
	if err != nil {
		return nil, err
	}

	apiPkgResources, err := pkg.GetResources(ctx)
	if err != nil {
		return nil, err
	}

	klog.V(3).Infof("Get packagerevisionresources completed: %s", name)

	return apiPkgResources, nil
}

// Update finds a resource in the storage and updates it. Some implementations
// may allow updates creates the object - they should set the created boolean
// to true.
func (r *packageRevisionResources) Update(ctx context.Context, name string, objInfo rest.UpdatedObjectInfo, createValidation rest.ValidateObjectFunc, updateValidation rest.ValidateObjectUpdateFunc, forceAllowCreate bool, options *metav1.UpdateOptions) (runtime.Object, bool, error) {
	ctx, span := tracer.Start(ctx, "[START]::packageRevisionResources::Update", trace.WithAttributes())
	defer span.End()

	namespace, namespaced := genericapirequest.NamespaceFrom(ctx)
	if !namespaced {
		return nil, false, apierrors.NewBadRequest("namespace must be specified")
	}

	pkgMutexKey := getPackageMutexKey(namespace, name)
	pkgMutex := getMutexForPackage(pkgMutexKey)
	locked := pkgMutex.TryLock()
	if !locked {
		return nil, false,
			apierrors.NewConflict(
				porchapi.Resource("packagerevisionresources"),
				name,
				fmt.Errorf(GenericConflictErrorMsg, "package revision resources", pkgMutexKey))
	}
	defer pkgMutex.Unlock()

	oldRepoPkgRev, err := r.getRepoPkgRev(ctx, name)
	if err != nil {
		return nil, false, err
	}

	oldApiPkgRevResources, err := oldRepoPkgRev.GetResources(ctx)
	if err != nil {
		klog.Infof("update failed to retrieve old object: %v", err)
		return nil, false, err
	}

	newRuntimeObj, err := objInfo.UpdatedObject(ctx, oldApiPkgRevResources)
	if err != nil {
		klog.Infof("update failed to construct UpdatedObject: %v", err)
		return nil, false, err
	}
	newObj, ok := newRuntimeObj.(*porchapi.PackageRevisionResources)
	if !ok {
		return nil, false, apierrors.NewBadRequest(fmt.Sprintf("expected PackageRevisionResources object, got %T", newRuntimeObj))
	}

	if updateValidation != nil {
		err := updateValidation(ctx, newObj, oldApiPkgRevResources)
		if err != nil {
			klog.Infof("update failed validation: %v", err)
			return nil, false, err
		}
	}

	prKey, err := repository.PkgRevK8sName2Key(namespace, name)
	if err != nil {
		return nil, false, err
	}

	var repositoryObj v1alpha1.Repository
	repositoryID := types.NamespacedName{Namespace: prKey.RKey().Namespace, Name: prKey.RKey().Name}
	if err := r.coreClient.Get(ctx, repositoryID, &repositoryObj); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, false, apierrors.NewNotFound(schema.GroupResource(porchapi.PackageRevisionResourcesGVR.GroupResource()), repositoryID.Name)
		}
		return nil, false, apierrors.NewInternalError(fmt.Errorf("error getting repository %v: %w", repositoryID, err))
	}

	rev, renderStatus, err := r.cad.UpdatePackageResources(ctx, &repositoryObj, oldRepoPkgRev, oldApiPkgRevResources, newObj)
	if err != nil {
		return nil, false, apierrors.NewInternalError(err)
	}

	created, err := rev.GetResources(ctx)
	if err != nil {
		return nil, false, apierrors.NewInternalError(err)
	}
	if renderStatus != nil {
		created.Status.RenderStatus = *renderStatus
	}

	klog.Infof("Update operation completed for packagerevisionresources: %s", name)

	return created, false, nil
}

// Watch supports watching for changes to PackageRevisionResources.
func (r *packageRevisionResources) Watch(ctx context.Context, options *metainternalversion.ListOptions) (watch.Interface, error) {
	ctx, span := tracer.Start(ctx, "[START]::packageRevisionResources::Watch", trace.WithAttributes())
	defer span.End()

	filter, err := parsePackageRevisionResourcesFieldSelector(options)
	if err != nil {
		return nil, err
	}

	if namespace, namespaced := genericapirequest.NamespaceFrom(ctx); namespaced {
		if filter.Key.RKey().Namespace != "" && namespace != filter.Key.RKey().Namespace {
			return nil, fmt.Errorf("conflicting namespaces specified: %q and %q", namespace, filter.Key.RKey().Namespace)
		}
		filter.Key.PkgKey.RepoKey.Namespace = namespace
	}

	ctx, cancel := context.WithCancel(ctx)

	w := &packageRevisionResourcesWatcher{
		cancel:     cancel,
		resultChan: make(chan watch.Event, 64),
	}

	go w.listAndWatch(ctx, r, *filter, options.LabelSelector)

	return w, nil
}

// packageRevisionResourcesWatcher implements watch.Interface for PackageRevisionResources
type packageRevisionResourcesWatcher struct {
	cancel     func()
	resultChan chan watch.Event

	mutex         sync.Mutex
	eventCallback func(eventType watch.EventType, pr repository.PackageRevision) bool
	done          bool
	totalSent     int
}

var _ watch.Interface = &packageRevisionResourcesWatcher{}

func (w *packageRevisionResourcesWatcher) Stop() {
	w.cancel()
}

func (w *packageRevisionResourcesWatcher) ResultChan() <-chan watch.Event {
	return w.resultChan
}

func (w *packageRevisionResourcesWatcher) listAndWatch(ctx context.Context, r *packageRevisionResources, filter repository.ListPackageRevisionFilter, selector labels.Selector) {
	if err := w.listAndWatchInner(ctx, r, filter, selector); err != nil {
		klog.Warningf("sending error to watch stream: %v", err)
		ev := watch.Event{
			Type: watch.Error,
		}
		w.resultChan <- ev
	}
	w.cancel()
	close(w.resultChan)
}

func (w *packageRevisionResourcesWatcher) listAndWatchInner(ctx context.Context, r *packageRevisionResources, filter repository.ListPackageRevisionFilter, selector labels.Selector) error {
	errorResult := make(chan error, 4)

	var backlog []watch.Event
	w.mutex.Lock()
	w.eventCallback = func(eventType watch.EventType, pr repository.PackageRevision) bool {
		if w.done {
			return false
		}
		obj, err := pr.GetResources(ctx)
		if err != nil {
			w.done = true
			errorResult <- err
			return false
		}

		backlog = append(backlog, watch.Event{
			Type:   eventType,
			Object: obj,
		})

		return true
	}
	w.mutex.Unlock()

	if err := r.packageCommon.watchPackages(ctx, filter, w); err != nil {
		return err
	}

	sentAdd := 0
	if err := r.packageCommon.listPackageRevisions(ctx, filter, func(ctx context.Context, p repository.PackageRevision) error {
		obj, err := p.GetResources(ctx)
		if err != nil {
			w.mutex.Lock()
			w.done = true
			w.mutex.Unlock()
			return err
		}
		ev := watch.Event{
			Type:   watch.Added,
			Object: obj,
		}
		sentAdd += 1
		w.sendWatchEvent(ev)
		return nil
	}); err != nil {
		w.mutex.Lock()
		w.done = true
		w.mutex.Unlock()
		return err
	}

	sentBacklog := 0
	for {
		w.mutex.Lock()
		chunk := backlog
		backlog = nil
		w.mutex.Unlock()

		if len(chunk) == 0 {
			break
		}

		for _, ev := range chunk {
			sentBacklog += 1
			w.sendWatchEvent(ev)
		}
	}

	w.mutex.Lock()
	sentNewBacklog := 0
	for _, ev := range backlog {
		sentNewBacklog += 1
		w.sendWatchEvent(ev)
	}

	klog.Infof("packageRevisionResources watch %p: moving into streaming mode after sentAdd %d, sentBacklog %d, sentNewBacklog %d", w, sentAdd, sentBacklog, sentNewBacklog)
	w.eventCallback = func(eventType watch.EventType, pr repository.PackageRevision) bool {
		if w.done {
			return false
		}
		obj, err := pr.GetResources(ctx)
		if err != nil {
			w.done = true
			errorResult <- err
			return false
		}
		ev := watch.Event{
			Type:   eventType,
			Object: obj,
		}
		w.sendWatchEvent(ev)
		return true
	}
	w.mutex.Unlock()

	select {
	case <-ctx.Done():
		w.mutex.Lock()
		defer w.mutex.Unlock()
		w.done = true
		return ctx.Err()

	case err := <-errorResult:
		w.mutex.Lock()
		defer w.mutex.Unlock()
		w.done = true
		return err
	}
}

func (w *packageRevisionResourcesWatcher) sendWatchEvent(ev watch.Event) {
	w.resultChan <- ev
	w.totalSent += 1
}

// OnPackageRevisionChange is the callback called when a PackageRevision changes.
func (w *packageRevisionResourcesWatcher) OnPackageRevisionChange(eventType watch.EventType, pr repository.PackageRevision) bool {
	w.mutex.Lock()
	defer w.mutex.Unlock()

	return w.eventCallback(eventType, pr)
}
