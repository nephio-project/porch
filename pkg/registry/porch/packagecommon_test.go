package porch

import (
	"context"
	"fmt"
	"testing"

	api "github.com/nephio-project/porch/api/porch/v1alpha1"
	configapi "github.com/nephio-project/porch/api/porchconfig/v1alpha1"
	"github.com/nephio-project/porch/pkg/engine"
	kptfilev1 "github.com/nephio-project/porch/pkg/kpt/api/kptfile/v1"
	"github.com/nephio-project/porch/pkg/repository"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/apiserver/pkg/endpoints/request"
)

type fakePackageRevision struct {
	namespace string
}

func (f *fakePackageRevision) GetPackageRevision(ctx context.Context) (*api.PackageRevision, error) {
	return &api.PackageRevision{ObjectMeta: metav1.ObjectMeta{Namespace: f.namespace}}, nil
}
func (f *fakePackageRevision) KubeObjectNamespace() string { return f.namespace }
func (f *fakePackageRevision) Key() repository.PackageRevisionKey {
	return repository.PackageRevisionKey{}
}
func (f *fakePackageRevision) KubeObjectName() string                                 { return "" }
func (f *fakePackageRevision) UID() types.UID                                         { return "" }
func (f *fakePackageRevision) SetMeta(context.Context, metav1.ObjectMeta) error       { return nil }
func (f *fakePackageRevision) ResourceVersion() string                                { return "" }
func (f *fakePackageRevision) Lifecycle(context.Context) api.PackageRevisionLifecycle { return "" }
func (f *fakePackageRevision) GetResources(context.Context) (*api.PackageRevisionResources, error) {
	return nil, nil
}
func (f *fakePackageRevision) UpdateLifecycle(context.Context, api.PackageRevisionLifecycle) error {
	return nil
}
func (f *fakePackageRevision) GetUpstreamLock(context.Context) (kptfilev1.Upstream, kptfilev1.UpstreamLock, error) {
	return kptfilev1.Upstream{}, kptfilev1.UpstreamLock{}, nil
}
func (f *fakePackageRevision) GetKptfile(context.Context) (kptfilev1.KptFile, error) {
	return kptfilev1.KptFile{}, nil
}
func (f *fakePackageRevision) GetLock() (kptfilev1.Upstream, kptfilev1.UpstreamLock, error) {
	return kptfilev1.Upstream{}, kptfilev1.UpstreamLock{}, nil
}
func (f *fakePackageRevision) ToMainPackageRevision(context.Context) repository.PackageRevision {
	return f
}
func (f *fakePackageRevision) GetMeta() metav1.ObjectMeta {
	return metav1.ObjectMeta{Namespace: f.namespace}
}

func TestNamespaceFilteringWatcher(t *testing.T) {
	called := false
	watcher := &namespaceFilteringWatcher{
		ns: "foo",
		delegate: &testWatcher{onChange: func(eventType watch.EventType, obj repository.PackageRevision) bool {
			called = true
			return true
		}},
	}
	// Should call delegate
	watcher.OnPackageRevisionChange(watch.Added, &fakePackageRevision{namespace: "foo"})
	if !called {
		t.Error("expected delegate to be called for matching namespace")
	}
	called = false
	// Should NOT call delegate
	watcher.OnPackageRevisionChange(watch.Added, &fakePackageRevision{namespace: "bar"})
	if called {
		t.Error("did not expect delegate to be called for non-matching namespace")
	}
}

type testWatcher struct {
	onChange func(eventType watch.EventType, obj repository.PackageRevision) bool
}

func (t *testWatcher) OnPackageRevisionChange(eventType watch.EventType, obj repository.PackageRevision) bool {
	return t.onChange(eventType, obj)
}

func TestListPackageRevisionsNamespaceFilter(t *testing.T) {
	ctx := context.TODO()
	ns := "test-ns"

	// Simulate two revisions, one in the right ns, one in another
	revisions := []repository.PackageRevision{
		&fakePackageRevision{namespace: "test-ns"},
		&fakePackageRevision{namespace: "other-ns"},
	}

	var got []*api.PackageRevision
	for _, rev := range revisions {
		apiPkgRev, _ := rev.GetPackageRevision(ctx)
		if ns != "" && apiPkgRev.Namespace != ns {
			continue
		}
		got = append(got, apiPkgRev)
	}
	for _, pr := range got {
		if pr.Namespace != ns {
			t.Errorf("got revision from wrong namespace: %s", pr.Namespace)
		}
	}
}

// --- TEST FOR watchPackages ---

type fakeWatcherManager struct {
	calledWithFilter   repository.ListPackageRevisionFilter
	calledWithCallback engine.ObjectWatcher
}

func (f *fakeWatcherManager) WatchPackageRevisions(ctx context.Context, filter repository.ListPackageRevisionFilter, callback engine.ObjectWatcher) error {
	f.calledWithFilter = filter
	f.calledWithCallback = callback
	// Simulate a package revision event
	if callback != nil {
		callback.OnPackageRevisionChange(watch.Added, &fakePackageRevision{namespace: "foo"})
	}
	return nil
}

type fakeCaDEngine struct {
	watcherManager engine.WatcherManager
}

func (f *fakeCaDEngine) ObjectCache() engine.WatcherManager {
	return f.watcherManager
}

// Implement other methods as no-ops for interface compliance
func (f *fakeCaDEngine) UpdatePackageResources(ctx context.Context, repositoryObj *configapi.Repository, oldPackage repository.PackageRevision, old, new *api.PackageRevisionResources) (repository.PackageRevision, *api.RenderStatus, error) {
	return nil, nil, nil
}
func (f *fakeCaDEngine) ListPackageRevisions(ctx context.Context, repositorySpec *configapi.Repository, filter repository.ListPackageRevisionFilter) ([]repository.PackageRevision, error) {
	return nil, nil
}
func (f *fakeCaDEngine) CreatePackageRevision(ctx context.Context, repositoryObj *configapi.Repository, obj *api.PackageRevision, parent repository.PackageRevision) (repository.PackageRevision, error) {
	return nil, nil
}
func (f *fakeCaDEngine) UpdatePackageRevision(ctx context.Context, version int, repositoryObj *configapi.Repository, oldPackage repository.PackageRevision, old, new *api.PackageRevision, parent repository.PackageRevision) (repository.PackageRevision, error) {
	return nil, nil
}
func (f *fakeCaDEngine) DeletePackageRevision(ctx context.Context, repositoryObj *configapi.Repository, obj repository.PackageRevision) error {
	return nil
}
func (f *fakeCaDEngine) ListPackages(ctx context.Context, repositorySpec *configapi.Repository, filter repository.ListPackageFilter) ([]repository.Package, error) {
	return nil, nil
}
func (f *fakeCaDEngine) CreatePackage(ctx context.Context, repositoryObj *configapi.Repository, obj *api.PorchPackage) (repository.Package, error) {
	return nil, nil
}
func (f *fakeCaDEngine) UpdatePackage(ctx context.Context, repositoryObj *configapi.Repository, oldPackage repository.Package, old, new *api.PorchPackage) (repository.Package, error) {
	return nil, nil
}
func (f *fakeCaDEngine) DeletePackage(ctx context.Context, repositoryObj *configapi.Repository, obj repository.Package) error {
	return nil
}
func TestWatchPackages_CallsCallback(t *testing.T) {
	fakeWatcher := &fakeWatcherManager{}
	fakeCad := &fakeCaDEngine{watcherManager: fakeWatcher}
	pc := &packageCommon{cad: fakeCad}

	called := false
	callback := &testWatcher{onChange: func(eventType watch.EventType, obj repository.PackageRevision) bool {
		called = true
		return false
	}}

	filter := packageRevisionFilter{Namespace: "test-ns"}
	ctx := context.TODO()
	err := pc.watchPackages(ctx, filter, callback)
	if err != nil {
		t.Fatalf("watchPackages returned error: %v", err)
	}
	if !called {
		t.Error("expected callback to be called by watchPackages")
	}
}
func TestWatchPackages_NoNamespace(t *testing.T) {
	fakeWatcher := &fakeWatcherManager{}
	fakeCad := &fakeCaDEngine{watcherManager: fakeWatcher}
	pc := &packageCommon{cad: fakeCad}

	called := false
	callback := &testWatcher{onChange: func(eventType watch.EventType, obj repository.PackageRevision) bool {
		called = true
		return false
	}}

	filter := packageRevisionFilter{}
	ctx := context.TODO() // No namespace set in context
	err := pc.watchPackages(ctx, filter, callback)
	if err != nil {
		t.Fatalf("watchPackages returned error: %v", err)
	}
	if !called {
		t.Error("expected callback to be called by watchPackages (no namespace)")
	}
}

type errorWatcherManager struct{}

func (e *errorWatcherManager) WatchPackageRevisions(ctx context.Context, filter repository.ListPackageRevisionFilter, callback engine.ObjectWatcher) error {
	return fmt.Errorf("simulated error")
}
func TestWatchPackages_ErrorPath(t *testing.T) {
	fakeCad := &fakeCaDEngine{watcherManager: &errorWatcherManager{}}
	pc := &packageCommon{cad: fakeCad}

	callback := &testWatcher{onChange: func(eventType watch.EventType, obj repository.PackageRevision) bool {
		return false
	}}

	filter := packageRevisionFilter{}
	ctx := context.TODO()
	err := pc.watchPackages(ctx, filter, callback)
	if err == nil {
		t.Error("expected error from watchPackages, got nil")
	}
}
func TestWatchPackages_WithNamespaceFilteringWatcher(t *testing.T) {
	fakeWatcher := &fakeWatcherManager{}
	fakeCad := &fakeCaDEngine{watcherManager: fakeWatcher}
	pc := &packageCommon{cad: fakeCad}

	called := false
	callback := &testWatcher{onChange: func(eventType watch.EventType, obj repository.PackageRevision) bool {
		called = true
		return false
	}}

	filter := packageRevisionFilter{}
	ctx := context.TODO()
	ctx = request.WithNamespace(ctx, "foo") // Set namespace in context

	err := pc.watchPackages(ctx, filter, callback)
	if err != nil {
		t.Fatalf("watchPackages returned error: %v", err)
	}
	if !called {
		t.Error("expected callback to be called by watchPackages with namespace filtering")
	}
}
