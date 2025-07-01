// Copyright 2025 The kpt and Nephio Authors
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
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	api "github.com/nephio-project/porch/api/porch/v1alpha1"
	configapi "github.com/nephio-project/porch/api/porchconfig/v1alpha1"
	"github.com/nephio-project/porch/pkg/engine"
	"github.com/nephio-project/porch/pkg/externalrepo/fake"
	kptfilev1 "github.com/nephio-project/porch/pkg/kpt/api/kptfile/v1"
	"github.com/nephio-project/porch/pkg/repository"
	mockclient "github.com/nephio-project/porch/test/mockery/mocks/external/sigs.k8s.io/controller-runtime/pkg/client"
	mockengine "github.com/nephio-project/porch/test/mockery/mocks/porch/pkg/engine"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	mockclient "github.com/nephio-project/porch/test/mockery/mocks/external/sigs.k8s.io/controller-runtime/pkg/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/apiserver/pkg/endpoints/request"
	genericapirequest "k8s.io/apiserver/pkg/endpoints/request"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
	mock.Mock
}

func (f *fakeCaDEngine) ObjectCache() engine.WatcherManager {
	return f.watcherManager
}

// Implement other methods as no-ops for interface compliance
func (f *fakeCaDEngine) UpdatePackageResources(ctx context.Context, repositoryObj *configapi.Repository, oldPackage repository.PackageRevision, old, new *api.PackageRevisionResources) (repository.PackageRevision, *api.RenderStatus, error) {
	return nil, nil, nil
}
func (f *fakeCaDEngine) ListPackageRevisions(ctx context.Context, repositorySpec *configapi.Repository, filter repository.ListPackageRevisionFilter) ([]repository.PackageRevision, error) {
	args := f.Called(ctx, repositorySpec, filter)
	return args.Get(0).([]repository.PackageRevision), args.Error(1)
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

	nsFilter := repository.ListPackageRevisionFilter{
		Key: repository.PackageRevisionKey{
			PkgKey: repository.PackageKey{
				RepoKey: repository.RepositoryKey{
					Namespace: "test-ns",
				},
			},
		},
	}

	err := pc.watchPackages(context.TODO(), nsFilter, callback)
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

	filter := repository.ListPackageRevisionFilter{}
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

	filter := repository.ListPackageRevisionFilter{}
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

	filter := repository.ListPackageRevisionFilter{}
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

func TestListPackageRevisions(t *testing.T) {
	var currentConcurrent int32
	var maxConcurrent int32
	var mutex sync.Mutex

	tests := []struct {
		name          string
		setupMocks    func(*fakeCaDEngine, *mockclient.MockClient)
		filter        packageRevisionFilter
		selector      labels.Selector
		expectedError error
		expectedCalls int
		ctx           context.Context
	}{
		{
			name: "Successful package revision listing",
			setupMocks: func(cad *fakeCaDEngine, c *mockclient.MockClient) {
				c.On("List", mock.Anything, &configapi.RepositoryList{}, mock.Anything).
					Run(func(args mock.Arguments) {
						list := args.Get(1).(*configapi.RepositoryList)
						list.Items = []configapi.Repository{
							{
								ObjectMeta: metav1.ObjectMeta{
									Name: "repo1",
								},
							},
						}
					}).Return(nil)

				pkgRev := &fakeextrepo.FakePackageRevision{
					PackageRevision: &api.PackageRevision{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{"test": "true"},
						},
					},
				}

				cad.On("ListPackageRevisions", mock.Anything, mock.Anything, mock.Anything).
					Return([]repository.PackageRevision{pkgRev}, nil)
			},
			filter:        packageRevisionFilter{},
			selector:      labels.Everything(),
			expectedError: nil,
			expectedCalls: 1,
			ctx:           context.Background(),
		},
		{
			name: "Successful namespace filtering",
			setupMocks: func(cad *fakeCaDEngine, c *mockclient.MockClient) {
				c.On("List", mock.Anything, &configapi.RepositoryList{}, client.InNamespace("test-namespace")).
					Run(func(args mock.Arguments) {
						list := args.Get(1).(*configapi.RepositoryList)
						list.Items = []configapi.Repository{
							{ObjectMeta: metav1.ObjectMeta{Name: "repo1", Namespace: "test-ns"}},
						}
					}).Return(nil)

				pkgRev := &fakeextrepo.FakePackageRevision{
					PackageRevision: &api.PackageRevision{
						ObjectMeta: metav1.ObjectMeta{
							Name: "test-namespace",
						},
					},
				}

				cad.On("ListPackageRevisions", mock.Anything, mock.Anything, mock.Anything).
					Return([]repository.PackageRevision{pkgRev}, nil)
			},
			filter:        packageRevisionFilter{Namespace: "test-namespace"},
			selector:      labels.Everything(),
			expectedError: nil,
			expectedCalls: 1,
			ctx:           genericapirequest.WithNamespace(context.Background(), "test-namespace"),
		},
		{
			name:       "Conflicting namespaces",
			setupMocks: func(cad *fakeCaDEngine, c *mockclient.MockClient) {},
			filter: packageRevisionFilter{
				Namespace: "namespace-1",
			},
			selector:      labels.Everything(),
			expectedError: fmt.Errorf("conflicting namespaces specified: %q and %q", "namespace-2", "namespace-1"),
			expectedCalls: 0,
			ctx:           genericapirequest.WithNamespace(context.Background(), "namespace-2"),
		},
		{
			name: "Failed repository listing",
			setupMocks: func(cad *fakeCaDEngine, c *mockclient.MockClient) {
				c.On("List", mock.Anything, &configapi.RepositoryList{}, mock.Anything).
					Return(fmt.Errorf("list error"))
			},
			filter:        packageRevisionFilter{},
			selector:      labels.Everything(),
			expectedError: fmt.Errorf("error listing repository objects: list error"),
			expectedCalls: 0,
			ctx:           context.Background(),
		}, {
			name: "Successful repository filtering",
			setupMocks: func(cad *fakeCaDEngine, c *mockclient.MockClient) {
				c.On("List", mock.Anything, &configapi.RepositoryList{}, mock.Anything).
					Run(func(args mock.Arguments) {
						list := args.Get(1).(*configapi.RepositoryList)
						list.Items = []configapi.Repository{
							{ObjectMeta: metav1.ObjectMeta{Name: "repo1"}},
							{ObjectMeta: metav1.ObjectMeta{Name: "repo2"}},
						}
					}).Return(nil)

				pkgRev := &fakeextrepo.FakePackageRevision{
					PackageRevision: &api.PackageRevision{},
				}

				cad.On("ListPackageRevisions", mock.Anything, mock.Anything, mock.Anything).
					Return([]repository.PackageRevision{pkgRev}, nil)
			},
			filter:        packageRevisionFilter{Repository: "repo1"},
			selector:      labels.Everything(),
			expectedError: nil,
			expectedCalls: 1,
			ctx:           context.Background(),
		},
		{
			name: "Label selector filtering",
			setupMocks: func(cad *fakeCaDEngine, c *mockclient.MockClient) {
				c.On("List", mock.Anything, &configapi.RepositoryList{}, mock.Anything).
					Run(func(args mock.Arguments) {
						list := args.Get(1).(*configapi.RepositoryList)
						list.Items = []configapi.Repository{{}}
					}).Return(nil)

				pkgRev := &fakeextrepo.FakePackageRevision{
					PackageRevision: &api.PackageRevision{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{"test": "true"},
						},
					},
				}

				cad.On("ListPackageRevisions", mock.Anything, mock.Anything, mock.Anything).
					Return([]repository.PackageRevision{pkgRev}, nil)
			},
			filter:        packageRevisionFilter{},
			selector:      labels.SelectorFromSet(labels.Set{"test": "true"}),
			expectedError: nil,
			expectedCalls: 1,
			ctx:           context.Background(),
		},
		{
			name: "Timeout error",
			setupMocks: func(cad *fakeCaDEngine, c *mockclient.MockClient) {
				c.On("List", mock.Anything, &configapi.RepositoryList{}, mock.Anything).
					Run(func(args mock.Arguments) {
						list := args.Get(1).(*configapi.RepositoryList)
						list.Items = []configapi.Repository{{}}
					}).Return(nil)

				cad.On("ListPackageRevisions", mock.Anything, mock.Anything, mock.Anything).
					Return([]repository.PackageRevision{}, context.DeadlineExceeded)
			},
			filter:        packageRevisionFilter{},
			selector:      labels.Everything(),
			expectedError: nil,
			expectedCalls: 0,
		},
		{
			name: "CaD engine error",
			setupMocks: func(cad *fakeCaDEngine, c *mockclient.MockClient) {
				c.On("List", mock.Anything, &configapi.RepositoryList{}, mock.Anything).
					Run(func(args mock.Arguments) {
						list := args.Get(1).(*configapi.RepositoryList)
						list.Items = []configapi.Repository{{}}
					}).Return(nil)

				cad.On("ListPackageRevisions", mock.Anything, mock.Anything, mock.Anything).
					Return([]repository.PackageRevision{}, fmt.Errorf("CaD engine error"))
			},
			filter:        packageRevisionFilter{},
			selector:      labels.Everything(),
			expectedError: nil,
			expectedCalls: 0,
			ctx:           context.Background(),
		},
		{
			name: "Package revision retrieval error",
			setupMocks: func(cad *fakeCaDEngine, c *mockclient.MockClient) {
				c.On("List", mock.Anything, &configapi.RepositoryList{}, mock.Anything).
					Run(func(args mock.Arguments) {
						list := args.Get(1).(*configapi.RepositoryList)
						list.Items = []configapi.Repository{{}}
					}).Return(nil)

				pkgRev := &fakeextrepo.FakePackageRevision{
					PackageRevision: &api.PackageRevision{},
					Err:             fmt.Errorf("revision retrieval error"),
				}

				cad.On("ListPackageRevisions", mock.Anything, mock.Anything, mock.Anything).
					Return([]repository.PackageRevision{pkgRev}, nil)
			},
			filter:        packageRevisionFilter{},
			selector:      labels.Everything(),
			expectedError: nil,
			expectedCalls: 0,
			ctx:           context.Background(),
		},
		{
			name: "Callback error",
			setupMocks: func(cad *fakeCaDEngine, c *mockclient.MockClient) {
				c.On("List", mock.Anything, &configapi.RepositoryList{}, mock.Anything).
					Run(func(args mock.Arguments) {
						list := args.Get(1).(*configapi.RepositoryList)
						list.Items = []configapi.Repository{{}}
					}).Return(nil)

				pkgRev := &fakeextrepo.FakePackageRevision{
					PackageRevision: &api.PackageRevision{},
				}

				cad.On("ListPackageRevisions", mock.Anything, mock.Anything, mock.Anything).
					Return([]repository.PackageRevision{pkgRev}, nil)
			},
			filter:        packageRevisionFilter{},
			selector:      labels.Everything(),
			expectedError: nil,
			expectedCalls: 1,
			ctx:           context.Background(),
		},
		{
			name: "Concurrent listing",
			setupMocks: func(cad *fakeCaDEngine, c *mockclient.MockClient) {
				c.On("List", mock.Anything, &configapi.RepositoryList{}, mock.Anything).
					Run(func(args mock.Arguments) {
						list := args.Get(1).(*configapi.RepositoryList)
						list.Items = []configapi.Repository{
							{ObjectMeta: metav1.ObjectMeta{Name: "repo1"}},
							{ObjectMeta: metav1.ObjectMeta{Name: "repo2"}},
							{ObjectMeta: metav1.ObjectMeta{Name: "repo3"}},
							{ObjectMeta: metav1.ObjectMeta{Name: "repo4"}},
						}
					}).Return(nil)

				pkgRev := &fakeextrepo.FakePackageRevision{
					PackageRevision: &api.PackageRevision{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{"test": "true"},
						},
					},
				}

				cad.On("ListPackageRevisions", mock.Anything, mock.Anything, mock.Anything).
					Run(func(args mock.Arguments) {
						atomic.AddInt32(&currentConcurrent, 1)
						mutex.Lock()
						if currentConcurrent > maxConcurrent {
							maxConcurrent = currentConcurrent
						}
						mutex.Unlock()
						time.Sleep(500 * time.Millisecond)
						atomic.AddInt32(&currentConcurrent, -1)
					}).
					Return([]repository.PackageRevision{pkgRev}, nil)
			},
			filter:        packageRevisionFilter{},
			selector:      labels.Everything(),
			expectedError: nil,
			expectedCalls: 4,
			ctx:           context.Background(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCaD := &fakeCaDEngine{}
			mockCoreClient := &mockclient.MockClient{}

			pc := &packageCommon{
				cad:                      mockCaD,
				coreClient:               mockCoreClient,
				ListTimeoutPerRepository: 1 * time.Second,
				MaxConcurrentLists:       3,
			}

			mockCaD.ExpectedCalls = nil
			mockCoreClient.ExpectedCalls = nil
			tt.setupMocks(mockCaD, mockCoreClient)

			callCount := 0
			callback := func(ctx context.Context, p repository.PackageRevision) error {
				callCount++
				return nil
			}

			ctx := tt.ctx
			if tt.name == "Timeout error" {
				tCtx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
				ctx = tCtx
				defer cancel()
			}

			err := pc.listPackageRevisions(ctx, tt.filter, tt.selector, callback)

			if tt.expectedError != nil {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError.Error())
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, tt.expectedCalls, callCount)
			assert.LessOrEqual(t, maxConcurrent, int32(pc.MaxConcurrentLists), "Maximum concurrent operations exceeded limit of 2")
			mockCaD.AssertExpectations(t)
			mockCoreClient.AssertExpectations(t)
		})
	}
}

func TestGetRepoPkgRev(t *testing.T) {
	tests := []struct {
		name                 string
		pkgRevName           string
		namespace            string
		setupMocks           func(*fakeCaDEngine, *mockclient.MockClient)
		expectedFailure      bool
		expectedErrorMessage string
	}{
		{
			name:       "Successful package revision retrieval",
			pkgRevName: "repo.path.pkg.wsn",
			namespace:  "test-ns",
			setupMocks: func(cad *fakeCaDEngine, c *mockclient.MockClient) {
				repo := &configapi.Repository{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "repo",
						Namespace: "test-ns",
					},
				}

				pkgRev := &fakeextrepo.FakePackageRevision{
					PrKey: repository.PackageRevisionKey{
						PkgKey: repository.PackageKey{
							RepoKey: repository.RepositoryKey{
								Name: "repo",
							},
							Path:    "path",
							Package: "pkg",
						},
						WorkspaceName: "wsn",
					},
				}

				c.On("Get", mock.Anything, types.NamespacedName{Name: "repo", Namespace: "test-ns"},
					&configapi.Repository{}).
					Run(func(args mock.Arguments) {
						arg := args.Get(2).(*configapi.Repository)
						*arg = *repo
					}).Return(nil)

				cad.On("ListPackageRevisions", mock.Anything, repo,
					repository.ListPackageRevisionFilter{KubeObjectName: "repo.path.pkg.wsn"}).
					Return([]repository.PackageRevision{pkgRev}, nil)
			},
			expectedFailure: false,
		},
		{
			name:                 "Invalid package revision name format",
			pkgRevName:           "invalid-name",
			namespace:            "test-ns",
			setupMocks:           func(cad *fakeCaDEngine, c *mockclient.MockClient) {},
			expectedFailure:      true,
			expectedErrorMessage: "not found",
		},
		{
			name:       "Repository not found",
			pkgRevName: "repo.pkg.v1",
			namespace:  "test-ns",
			setupMocks: func(cad *fakeCaDEngine, c *mockclient.MockClient) {
				c.On("Get", mock.Anything, types.NamespacedName{Name: "repo", Namespace: "test-ns"},
					&configapi.Repository{}, mock.Anything).
					Return(apierrors.NewNotFound(schema.GroupResource{}, "repo"))
			},
			expectedFailure:      true,
			expectedErrorMessage: "not found",
		},
		{
			name:       "Listing package revisions timeout",
			pkgRevName: "repo.pkg.v1",
			namespace:  "test-ns",
			setupMocks: func(cad *fakeCaDEngine, c *mockclient.MockClient) {
				repo := &configapi.Repository{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "repo",
						Namespace: "test-ns",
					},
				}

				c.On("Get", mock.Anything, types.NamespacedName{Name: "repo", Namespace: "test-ns"},
					&configapi.Repository{}, mock.Anything).
					Run(func(args mock.Arguments) {
						arg := args.Get(2).(*configapi.Repository)
						*arg = *repo
					}).Return(nil)

				cad.On("ListPackageRevisions", mock.Anything, repo,
					repository.ListPackageRevisionFilter{KubeObjectName: "repo.pkg.v1"}).
					After(2*time.Second).
					Return([]repository.PackageRevision{}, fmt.Errorf("seconds trying to list package revisions"))

			},
			expectedFailure:      true,
			expectedErrorMessage: "seconds trying to list package revisions",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCoreClient := &mockclient.MockClient{}
			mockCaDEngine := &fakeCaDEngine{}

			pc := &packageCommon{
				coreClient:               mockCoreClient,
				cad:                      mockCaDEngine,
				ListTimeoutPerRepository: 1 * time.Second,
			}

			tt.setupMocks(mockCaDEngine, mockCoreClient)

			ctx := genericapirequest.WithNamespace(context.Background(), tt.namespace)
			result, err := pc.getRepoPkgRev(ctx, tt.pkgRevName)

			if !tt.expectedFailure {
				require.NoError(t, err)
				assert.NotNil(t, result)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedErrorMessage)
			}

			mockCoreClient.AssertExpectations(t)
			mockCaDEngine.AssertExpectations(t)
		})
	}
}

func TestGetPackage(t *testing.T) {
	ctx := context.TODO()
	mockClient := mockclient.NewMockClient(t)
	mockCaD := mockengine.NewMockCaDEngine(t)

	pc := packageCommon{
		coreClient: mockClient,
		cad:        mockCaD,
	}

	_, err := pc.getPackage(ctx, "")
	assert.NotNil(t, err)
	assert.Equal(t, "namespace must be specified", err.Error())

	ctx = request.NewDefaultContext()
	_, err = pc.getPackage(ctx, "")
	assert.NotNil(t, err)
	assert.True(t, strings.Contains(err.Error(), "package kubernetes resource name invalid"))

	_, err = pc.getPackage(ctx, "repo")
	assert.NotNil(t, err)
	assert.True(t, strings.Contains(err.Error(), "package kubernetes resource name invalid"))

	mockClient.EXPECT().Get(mock.Anything, mock.Anything, mock.Anything).Return(nil)
	mockCaD.EXPECT().ListPackages(mock.Anything, mock.Anything, mock.Anything).Return(nil, errors.New("list error")).Once()

	_, err = pc.getPackage(ctx, "repo.pkg.ws")
	assert.NotNil(t, err)
	assert.Equal(t, "list error", err.Error())

	pkgList := []repository.Package{}
	mockCaD.EXPECT().ListPackages(mock.Anything, mock.Anything, mock.Anything).Return(pkgList, nil).Once()

	_, err = pc.getPackage(ctx, "repo.pkg.ws")
	assert.True(t, strings.Contains(err.Error(), "not found"))

	pkgList = []repository.Package{
		&fake.FakePackage{
			PkgKey: repository.PackageKey{
				RepoKey: repository.RepositoryKey{
					Name: "repo",
				},
				Package: "pkg",
			},
		},
	}

	mockCaD.EXPECT().ListPackages(mock.Anything, mock.Anything, mock.Anything).Return(pkgList, nil).Once()

	_, err = pc.getPackage(ctx, "repo.pkg")
	assert.Nil(t, err)
}

func TestGetRepoPkgRev(t *testing.T) {
	ctx := context.TODO()
	mockClient := mockclient.NewMockClient(t)
	mockCaD := mockengine.NewMockCaDEngine(t)

	pc := packageCommon{
		coreClient: mockClient,
		cad:        mockCaD,
	}

	_, err := pc.getRepoPkgRev(ctx, "")
	assert.NotNil(t, err)
	assert.Equal(t, "namespace must be specified", err.Error())

	ctx = request.NewDefaultContext()
	_, err = pc.getRepoPkgRev(ctx, "")
	assert.NotNil(t, err)
	assert.True(t, strings.Contains(err.Error(), "package revision kubernetes resource name invalid"))

	_, err = pc.getRepoPkgRev(ctx, "repo")
	assert.NotNil(t, err)
	assert.True(t, strings.Contains(err.Error(), "package revision kubernetes resource name invalid"))

	_, err = pc.getRepoPkgRev(ctx, "repo.pkg")
	assert.NotNil(t, err)
	assert.True(t, strings.Contains(err.Error(), "package revision kubernetes resource name invalid"))

	mockClient.EXPECT().Get(mock.Anything, mock.Anything, mock.Anything).Return(nil)
	mockCaD.EXPECT().ListPackageRevisions(mock.Anything, mock.Anything, mock.Anything).Return(nil, errors.New("list error")).Once()

	_, err = pc.getRepoPkgRev(ctx, "repo.pkg.ws")
	assert.NotNil(t, err)
	assert.Equal(t, "list error", err.Error())

	pkgRevList := []repository.PackageRevision{}
	mockCaD.EXPECT().ListPackageRevisions(mock.Anything, mock.Anything, mock.Anything).Return(pkgRevList, nil).Once()

	_, err = pc.getRepoPkgRev(ctx, "repo.pkg.ws")
	assert.True(t, strings.Contains(err.Error(), "not found"))

	pkgRevList = []repository.PackageRevision{
		&fake.FakePackageRevision{
			PrKey: repository.PackageRevisionKey{
				PkgKey: repository.PackageKey{
					RepoKey: repository.RepositoryKey{
						Name: "repo",
					},
					Package: "pkg",
				},
				WorkspaceName: "ws",
			},
		},
	}

	mockCaD.EXPECT().ListPackageRevisions(mock.Anything, mock.Anything, mock.Anything).Return(pkgRevList, nil).Once()

	_, err = pc.getRepoPkgRev(ctx, "repo.pkg.ws")
	assert.Nil(t, err)
}
