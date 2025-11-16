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
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	api "github.com/nephio-project/porch/api/porch/v1alpha1"
	configapi "github.com/nephio-project/porch/api/porchconfig/v1alpha1"
	"github.com/nephio-project/porch/pkg/engine"
	fakeextrepo "github.com/nephio-project/porch/pkg/externalrepo/fake"
	"github.com/nephio-project/porch/pkg/repository"
	mockclient "github.com/nephio-project/porch/test/mockery/mocks/external/sigs.k8s.io/controller-runtime/pkg/client"
	mockcad "github.com/nephio-project/porch/test/mockery/mocks/porch/pkg/engine"
	mockrepo "github.com/nephio-project/porch/test/mockery/mocks/porch/pkg/repository"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	genericapirequest "k8s.io/apiserver/pkg/endpoints/request"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

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
	watcher.OnPackageRevisionChange(watch.Added, &fakeextrepo.FakePackageRevision{
		PrKey: repository.PackageRevisionKey{
			PkgKey: repository.PackageKey{
				RepoKey: repository.RepositoryKey{
					Namespace: "foo",
				},
			},
		},
	})
	if !called {
		t.Error("expected delegate to be called for matching namespace")
	}
	called = false
	// Should NOT call delegate
	watcher.OnPackageRevisionChange(watch.Added, &fakeextrepo.FakePackageRevision{
		PrKey: repository.PackageRevisionKey{
			PkgKey: repository.PackageKey{
				RepoKey: repository.RepositoryKey{
					Namespace: "bar",
				},
			},
		},
	})
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
		&fakeextrepo.FakePackageRevision{
			PrKey: repository.PackageRevisionKey{
				PkgKey: repository.PackageKey{
					RepoKey: repository.RepositoryKey{
						Namespace: "foo",
					},
				},
			},
		},
		&fakeextrepo.FakePackageRevision{
			PrKey: repository.PackageRevisionKey{
				PkgKey: repository.PackageKey{
					RepoKey: repository.RepositoryKey{
						Namespace: "foo",
					},
				},
			},
		},
	}

	var got []*api.PackageRevision
	for _, rev := range revisions {
		apiPkgRev, _ := rev.GetPackageRevision(ctx)
		if apiPkgRev == nil || apiPkgRev.Namespace != ns {
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
		callback.OnPackageRevisionChange(watch.Added, &fakeextrepo.FakePackageRevision{
			PrKey: repository.PackageRevisionKey{
				PkgKey: repository.PackageKey{
					RepoKey: repository.RepositoryKey{
						Namespace: "foo",
					},
				},
			},
		})
	}
	return nil
}

func TestWatchPackages_CallsCallback(t *testing.T) {
	mockCad := &mockcad.MockCaDEngine{}
	mockCad.On("ObjectCache").Return(&fakeWatcherManager{})
	pc := &packageCommon{cad: mockCad}

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
	mockCad := &mockcad.MockCaDEngine{}
	mockCad.On("ObjectCache").Return(&fakeWatcherManager{})
	pc := &packageCommon{cad: mockCad}

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
	mockCad := &mockcad.MockCaDEngine{}
	mockCad.On("ObjectCache").Return(&errorWatcherManager{})
	pc := &packageCommon{cad: mockCad}

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
	mockCad := &mockcad.MockCaDEngine{}
	mockCad.On("ObjectCache").Return(&fakeWatcherManager{})
	pc := &packageCommon{cad: mockCad}

	called := false
	callback := &testWatcher{onChange: func(eventType watch.EventType, obj repository.PackageRevision) bool {
		called = true
		return false
	}}
	filter := repository.ListPackageRevisionFilter{}
	ctx := context.TODO()
	ctx = genericapirequest.WithNamespace(ctx, "foo") // Set namespace in context

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
		setupMocks    func(*mockclient.MockClient, *mockrepo.MockPackageRevision, *mockcad.MockCaDEngine)
		filter        repository.ListPackageRevisionFilter
		expectedError error
		expectedCalls int
		ctx           context.Context
	}{
		{
			name: "Successful package revision listing",
			setupMocks: func(c *mockclient.MockClient, pkgRev *mockrepo.MockPackageRevision, cad *mockcad.MockCaDEngine) {
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

				cad.On("ListPackageRevisions", mock.Anything, mock.Anything, mock.Anything).
					Return([]repository.PackageRevision{pkgRev}, nil)
			},
			filter:        repository.ListPackageRevisionFilter{},
			expectedError: nil,
			expectedCalls: 1,
			ctx:           context.Background(),
		},
		{
			name: "Successful namespace filtering",
			setupMocks: func(c *mockclient.MockClient, pkgRev *mockrepo.MockPackageRevision, cad *mockcad.MockCaDEngine) {
				c.On("List", mock.Anything, &configapi.RepositoryList{}, client.InNamespace("test-namespace")).
					Run(func(args mock.Arguments) {
						list := args.Get(1).(*configapi.RepositoryList)
						list.Items = []configapi.Repository{
							{ObjectMeta: metav1.ObjectMeta{Name: "repo1", Namespace: "test-ns"}},
						}
					}).Return(nil)

				cad.On("ListPackageRevisions", mock.Anything, mock.Anything, mock.Anything).
					Return([]repository.PackageRevision{pkgRev}, nil)
			},
			filter: repository.ListPackageRevisionFilter{
				Key: repository.PackageRevisionKey{
					PkgKey: repository.PackageKey{
						RepoKey: repository.RepositoryKey{
							Namespace: "test-namespace",
						},
					},
				},
			},
			expectedError: nil,
			expectedCalls: 1,
			ctx:           genericapirequest.WithNamespace(context.Background(), "test-namespace"),
		},
		{
			name:       "Conflicting namespaces",
			setupMocks: func(c *mockclient.MockClient, pkgRev *mockrepo.MockPackageRevision, cad *mockcad.MockCaDEngine) {},
			filter: repository.ListPackageRevisionFilter{
				Key: repository.PackageRevisionKey{
					PkgKey: repository.PackageKey{
						RepoKey: repository.RepositoryKey{
							Namespace: "namespace-1",
						},
					},
				},
			},
			expectedError: fmt.Errorf("conflicting namespaces specified: %q and %q", "namespace-2", "namespace-1"),
			expectedCalls: 0,
			ctx:           genericapirequest.WithNamespace(context.Background(), "namespace-2"),
		},
		{
			name: "Failed repository listing",
			setupMocks: func(c *mockclient.MockClient, pkgRev *mockrepo.MockPackageRevision, cad *mockcad.MockCaDEngine) {
				c.On("List", mock.Anything, &configapi.RepositoryList{}, mock.Anything).
					Return(fmt.Errorf("list error"))
			},
			filter:        repository.ListPackageRevisionFilter{},
			expectedError: fmt.Errorf("error listing repository objects: list error"),
			expectedCalls: 0,
			ctx:           context.Background(),
		}, {
			name: "Successful repository filtering",
			setupMocks: func(c *mockclient.MockClient, pkgRev *mockrepo.MockPackageRevision, cad *mockcad.MockCaDEngine) {
				c.On("List", mock.Anything, &configapi.RepositoryList{}, mock.Anything).
					Run(func(args mock.Arguments) {
						list := args.Get(1).(*configapi.RepositoryList)
						list.Items = []configapi.Repository{
							{ObjectMeta: metav1.ObjectMeta{Name: "repo1"}},
						}
					}).Return(nil)

				cad.On("ListPackageRevisions", mock.Anything, mock.Anything, mock.Anything).
					Return([]repository.PackageRevision{pkgRev}, nil)
			},
			filter: repository.ListPackageRevisionFilter{
				Key: repository.PackageRevisionKey{
					PkgKey: repository.PackageKey{
						RepoKey: repository.RepositoryKey{
							Name: "repo1",
						},
					},
				},
			},
			expectedError: nil,
			expectedCalls: 1,
			ctx:           context.Background(),
		},
		{
			name: "Label selector filtering",
			setupMocks: func(c *mockclient.MockClient, pkgRev *mockrepo.MockPackageRevision, cad *mockcad.MockCaDEngine) {
				c.On("List", mock.Anything, &configapi.RepositoryList{}, mock.Anything).
					Run(func(args mock.Arguments) {
						list := args.Get(1).(*configapi.RepositoryList)
						list.Items = []configapi.Repository{{}}
					}).Return(nil)

				cad.On("ListPackageRevisions", mock.Anything, mock.Anything, mock.Anything).
					Return([]repository.PackageRevision{pkgRev}, nil)
			},
			filter:        repository.ListPackageRevisionFilter{Label: labels.SelectorFromSet(labels.Set{"test": "true"})},
			expectedError: nil,
			expectedCalls: 1,
			ctx:           context.Background(),
		},
		{
			name: "Timeout error",
			setupMocks: func(c *mockclient.MockClient, pkgRev *mockrepo.MockPackageRevision, cad *mockcad.MockCaDEngine) {
				c.On("List", mock.Anything, &configapi.RepositoryList{}, mock.Anything).
					Run(func(args mock.Arguments) {
						list := args.Get(1).(*configapi.RepositoryList)
						list.Items = []configapi.Repository{{}}
					}).Return(nil)

				cad.On("ListPackageRevisions", mock.Anything, mock.Anything, mock.Anything).
					Return([]repository.PackageRevision{}, context.DeadlineExceeded)
			},
			filter:        repository.ListPackageRevisionFilter{},
			expectedError: nil,
			expectedCalls: 0,
		},
		{
			name: "CaD engine error",
			setupMocks: func(c *mockclient.MockClient, pkgRev *mockrepo.MockPackageRevision, cad *mockcad.MockCaDEngine) {
				c.On("List", mock.Anything, &configapi.RepositoryList{}, mock.Anything).
					Run(func(args mock.Arguments) {
						list := args.Get(1).(*configapi.RepositoryList)
						list.Items = []configapi.Repository{{}}
					}).Return(nil)

				cad.On("ListPackageRevisions", mock.Anything, mock.Anything, mock.Anything).
					Return([]repository.PackageRevision{}, fmt.Errorf("CaD engine error"))
			},
			filter:        repository.ListPackageRevisionFilter{},
			expectedError: nil,
			expectedCalls: 0,
			ctx:           context.Background(),
		},
		{
			name: "Callback error",
			setupMocks: func(c *mockclient.MockClient, pkgRev *mockrepo.MockPackageRevision, cad *mockcad.MockCaDEngine) {
				c.On("List", mock.Anything, &configapi.RepositoryList{}, mock.Anything).
					Run(func(args mock.Arguments) {
						list := args.Get(1).(*configapi.RepositoryList)
						list.Items = []configapi.Repository{{}}
					}).Return(nil)

				cad.On("ListPackageRevisions", mock.Anything, mock.Anything, mock.Anything).
					Return([]repository.PackageRevision{pkgRev}, nil)
			},
			filter:        repository.ListPackageRevisionFilter{},
			expectedError: nil,
			expectedCalls: 1,
			ctx:           context.Background(),
		},
		{
			name: "Concurrent listing",
			setupMocks: func(c *mockclient.MockClient, pkgRev *mockrepo.MockPackageRevision, cad *mockcad.MockCaDEngine) {
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
			filter:        repository.ListPackageRevisionFilter{},
			expectedError: nil,
			expectedCalls: 4,
			ctx:           context.Background(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCoreClient := &mockclient.MockClient{}
			mockPkgRev := &mockrepo.MockPackageRevision{}
			mockCaD := &mockcad.MockCaDEngine{}

			pc := &packageCommon{
				cad:                      mockCaD,
				coreClient:               mockCoreClient,
				ListTimeoutPerRepository: 1 * time.Second,
				MaxConcurrentLists:       3,
			}

			mockCaD.ExpectedCalls = nil
			mockCoreClient.ExpectedCalls = nil
			mockPkgRev.ExpectedCalls = nil
			tt.setupMocks(mockCoreClient, mockPkgRev, mockCaD)

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

			err := pc.listPackageRevisions(ctx, tt.filter, callback)

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
			mockPkgRev.AssertExpectations(t)
		})
	}
}

func TestGetRepoPkgRev(t *testing.T) {
	tests := []struct {
		name                 string
		pkgRevName           string
		ctx                  context.Context
		setupMocks           func(*mockclient.MockClient, *mockcad.MockCaDEngine)
		expectedFailure      bool
		expectedErrorMessage string
	}{
		{
			name:       "Successful package revision retrieval",
			pkgRevName: "repo.pkg.wsn",
			ctx:        genericapirequest.WithNamespace(context.Background(), "test-ns"),
			setupMocks: func(c *mockclient.MockClient, cad *mockcad.MockCaDEngine) {
				repo := &configapi.Repository{}

				pkgRev := &fakeextrepo.FakePackageRevision{
					PrKey: repository.PackageRevisionKey{
						PkgKey: repository.PackageKey{
							RepoKey: repository.RepositoryKey{Name: "repo"},
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

				prKey, _ := repository.PkgRevK8sName2Key("test-ns", "repo.pkg.wsn")
				cad.On("ListPackageRevisions", mock.Anything, repo,
					repository.ListPackageRevisionFilter{Key: prKey}).
					Return([]repository.PackageRevision{pkgRev}, nil)
			},
			expectedFailure: false,
		},
		{
			name:                 "No namespace specified",
			pkgRevName:           "",
			ctx:                  context.TODO(),
			setupMocks:           func(c *mockclient.MockClient, cad *mockcad.MockCaDEngine) {},
			expectedFailure:      true,
			expectedErrorMessage: "namespace must be specified",
		},
		{
			name:                 "No name specified",
			pkgRevName:           "",
			ctx:                  genericapirequest.WithNamespace(context.Background(), "test-ns"),
			setupMocks:           func(c *mockclient.MockClient, cad *mockcad.MockCaDEngine) {},
			expectedFailure:      true,
			expectedErrorMessage: "package revision kubernetes resource name invalid",
		},
		{
			name:                 "Repository name only",
			pkgRevName:           "repo",
			ctx:                  genericapirequest.WithNamespace(context.Background(), "test-ns"),
			setupMocks:           func(c *mockclient.MockClient, cad *mockcad.MockCaDEngine) {},
			expectedFailure:      true,
			expectedErrorMessage: "package revision kubernetes resource name invalid",
		},
		{
			name:                 "Repository name and package only",
			pkgRevName:           "repo.pkg",
			ctx:                  genericapirequest.WithNamespace(context.Background(), "test-ns"),
			setupMocks:           func(c *mockclient.MockClient, cad *mockcad.MockCaDEngine) {},
			expectedFailure:      true,
			expectedErrorMessage: "package revision kubernetes resource name invalid",
		},
		{
			name:       "Repository not found",
			pkgRevName: "repo.pkg.wsn",
			ctx:        genericapirequest.WithNamespace(context.Background(), "test-ns"),
			setupMocks: func(c *mockclient.MockClient, cad *mockcad.MockCaDEngine) {
				c.On("Get", mock.Anything, types.NamespacedName{Name: "repo", Namespace: "test-ns"},
					&configapi.Repository{}, mock.Anything).
					Return(apierrors.NewNotFound(schema.GroupResource{}, "repo"))
			},
			expectedFailure:      true,
			expectedErrorMessage: "not found",
		},
		{
			name:       "Listing package revisions timeout",
			pkgRevName: "repo.pkg.wsn",
			ctx:        genericapirequest.WithNamespace(context.Background(), "test-ns"),
			setupMocks: func(c *mockclient.MockClient, cad *mockcad.MockCaDEngine) {
				repo := &configapi.Repository{}

				c.On("Get", mock.Anything, types.NamespacedName{Name: "repo", Namespace: "test-ns"},
					&configapi.Repository{}, mock.Anything).
					Run(func(args mock.Arguments) {
						arg := args.Get(2).(*configapi.Repository)
						*arg = *repo
					}).Return(nil)

				prKey, _ := repository.PkgRevK8sName2Key("test-ns", "repo.pkg.wsn")
				cad.On("ListPackageRevisions", mock.Anything, repo,
					repository.ListPackageRevisionFilter{Key: prKey}).
					After(2*time.Second).
					Return([]repository.PackageRevision{}, fmt.Errorf("seconds trying to list package revisions"))

			},
			expectedFailure:      true,
			expectedErrorMessage: "seconds trying to list package revisions",
		},
		{
			name:       "Listing package revisions error",
			pkgRevName: "repo.pkg.wsn",
			ctx:        genericapirequest.WithNamespace(context.Background(), "test-ns"),
			setupMocks: func(c *mockclient.MockClient, cad *mockcad.MockCaDEngine) {
				repo := &configapi.Repository{}

				c.On("Get", mock.Anything, types.NamespacedName{Name: "repo", Namespace: "test-ns"},
					&configapi.Repository{}, mock.Anything).
					Run(func(args mock.Arguments) {
						arg := args.Get(2).(*configapi.Repository)
						*arg = *repo
					}).Return(nil)

				prKey, _ := repository.PkgRevK8sName2Key("test-ns", "repo.pkg.wsn")
				cad.On("ListPackageRevisions", mock.Anything, repo,
					repository.ListPackageRevisionFilter{Key: prKey}).
					Return([]repository.PackageRevision{}, fmt.Errorf("list error"))

			},
			expectedFailure:      true,
			expectedErrorMessage: "list error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCoreClient := &mockclient.MockClient{}
			mockCaDEngine := &mockcad.MockCaDEngine{}

			pc := &packageCommon{
				coreClient:               mockCoreClient,
				cad:                      mockCaDEngine,
				ListTimeoutPerRepository: 1 * time.Second,
			}

			tt.setupMocks(mockCoreClient, mockCaDEngine)
			result, err := pc.getRepoPkgRev(tt.ctx, tt.pkgRevName)

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
	tests := []struct {
		name                 string
		pkgName              string
		ctx                  context.Context
		setupMocks           func(c *mockclient.MockClient, cad *mockcad.MockCaDEngine)
		expectedFailure      bool
		expectedErrorMessage string
	}{
		{
			name:    "Successful get package",
			pkgName: "repo.pkg",
			ctx:     genericapirequest.WithNamespace(context.Background(), "test-ns"),
			setupMocks: func(c *mockclient.MockClient, cad *mockcad.MockCaDEngine) {
				repo := &configapi.Repository{}

				pkg := &fakeextrepo.FakePackage{
					PkgKey: repository.PackageKey{
						RepoKey: repository.RepositoryKey{
							Name: "repo",
						},
						Package: "pkg",
					},
				}

				c.On("Get", mock.Anything, types.NamespacedName{Name: "repo", Namespace: "test-ns"},
					&configapi.Repository{}).
					Run(func(args mock.Arguments) {
						arg := args.Get(2).(*configapi.Repository)
						*arg = *repo
					}).Return(nil)

				pkgKey, _ := repository.PkgK8sName2Key("test-ns", "repo.pkg")
				cad.On("ListPackages", mock.Anything, repo,
					repository.ListPackageFilter{Key: pkgKey}).
					Return([]repository.Package{pkg}, nil)
			},
			expectedFailure: false,
		},
		{
			name:                 "No namespace specified",
			pkgName:              "repo.pkg",
			ctx:                  context.TODO(),
			setupMocks:           func(c *mockclient.MockClient, cad *mockcad.MockCaDEngine) {},
			expectedFailure:      true,
			expectedErrorMessage: "namespace must be specified",
		},
		{
			name:                 "No name specified",
			pkgName:              "",
			ctx:                  genericapirequest.WithNamespace(context.Background(), "test-ns"),
			setupMocks:           func(c *mockclient.MockClient, cad *mockcad.MockCaDEngine) {},
			expectedFailure:      true,
			expectedErrorMessage: "package kubernetes resource name invalid",
		},
		{
			name:                 "Repository name only",
			pkgName:              "repo",
			ctx:                  genericapirequest.WithNamespace(context.Background(), "test-ns"),
			setupMocks:           func(c *mockclient.MockClient, cad *mockcad.MockCaDEngine) {},
			expectedFailure:      true,
			expectedErrorMessage: "package kubernetes resource name invalid",
		},
		{
			name:    "Package not found",
			pkgName: "repo.pkg",
			ctx:     genericapirequest.WithNamespace(context.Background(), "test-ns"),
			setupMocks: func(c *mockclient.MockClient, cad *mockcad.MockCaDEngine) {
				c.On("Get", mock.Anything, types.NamespacedName{Name: "repo", Namespace: "test-ns"},
					&configapi.Repository{}, mock.Anything).
					Return(apierrors.NewNotFound(schema.GroupResource{}, "repo"))
			},
			expectedFailure:      true,
			expectedErrorMessage: "not found",
		},
		{
			name:    "Listing package revisions error",
			pkgName: "repo.pkg",
			ctx:     genericapirequest.WithNamespace(context.Background(), "test-ns"),
			setupMocks: func(c *mockclient.MockClient, cad *mockcad.MockCaDEngine) {
				repo := &configapi.Repository{}

				c.On("Get", mock.Anything, types.NamespacedName{Name: "repo", Namespace: "test-ns"},
					&configapi.Repository{}, mock.Anything).
					Run(func(args mock.Arguments) {
						arg := args.Get(2).(*configapi.Repository)
						*arg = *repo
					}).Return(nil)

				pkgKey, _ := repository.PkgK8sName2Key("test-ns", "repo.pkg")
				cad.On("ListPackages", mock.Anything, repo,
					repository.ListPackageFilter{Key: pkgKey}).
					Return(nil, fmt.Errorf("list error"))

			},
			expectedFailure:      true,
			expectedErrorMessage: "list error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCoreClient := &mockclient.MockClient{}
			mockCaDEngine := &mockcad.MockCaDEngine{}
			pc := &packageCommon{
				coreClient: mockCoreClient,
				cad:        mockCaDEngine,
			}

			tt.setupMocks(mockCoreClient, mockCaDEngine)
			result, err := pc.getPackage(tt.ctx, tt.pkgName)

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
