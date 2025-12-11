// Copyright 2022-2025 The kpt and Nephio Authors
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
	"testing"

	porchapi "github.com/nephio-project/porch/api/porch/v1alpha1"
	configapi "github.com/nephio-project/porch/api/porchconfig/v1alpha1"
	"github.com/nephio-project/porch/pkg/externalrepo/fake"
	kptfile "github.com/nephio-project/porch/pkg/kpt/api/kptfile/v1"
	"github.com/nephio-project/porch/pkg/repository"
	mockclient "github.com/nephio-project/porch/test/mockery/mocks/external/sigs.k8s.io/controller-runtime/pkg/client"
	mockengine "github.com/nephio-project/porch/test/mockery/mocks/porch/pkg/engine"
	mockrepo "github.com/nephio-project/porch/test/mockery/mocks/porch/pkg/repository"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/endpoints/request"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	packagerevisions = &packageRevisions{
		TableConvertor: packageRevisionTableConvertor,
		packageCommon: packageCommon{
			scheme:         runtime.NewScheme(),
			gr:             porchapi.Resource("packagerevisions"),
			coreClient:     nil,
			updateStrategy: packageRevisionStrategy{},
			createStrategy: packageRevisionStrategy{},
		},
	}
	dummyRepoObject = configapi.Repository{
		TypeMeta: metav1.TypeMeta{
			Kind:       configapi.TypeRepository.Kind,
			APIVersion: configapi.TypeRepository.APIVersion(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "someDummyRepo",
			Namespace: "someDummyNamespace",
		},
		Spec: configapi.RepositorySpec{
			Description: "Repository With Error",
			Type:        configapi.RepositoryTypeGit,
			Git: &configapi.GitRepository{
				// Use `invalid` domain: https://www.rfc-editor.org/rfc/rfc6761#section-6.4
				Repo: "https://repo.invalid/repository.git",
			},
		},
	}
	repositoryName  = "repo"
	pkg             = "1234567890"
	revision        = 1
	workspace       = "ws"
	packageRevision = &fake.FakePackageRevision{
		PrKey: repository.PackageRevisionKey{
			PkgKey: repository.PackageKey{
				RepoKey: repository.RepositoryKey{
					Name: repositoryName,
				},
				Package: pkg,
			},
			Revision:      revision,
			WorkspaceName: workspace,
		},
		PackageLifecycle: porchapi.PackageRevisionLifecyclePublished,
		PackageRevision: &porchapi.PackageRevision{
			ObjectMeta: metav1.ObjectMeta{
				Labels: make(map[string]string),
			},
		},
		Resources: &porchapi.PackageRevisionResources{
			Spec: porchapi.PackageRevisionResourcesSpec{
				PackageName:    pkg,
				Revision:       revision,
				RepositoryName: repositoryName,
				Resources: map[string]string{
					kptfile.KptFileName: strings.TrimSpace(`
apiVersion: kpt.dev/v1
kind: Kptfile
metadata:
  name: example
  annotations:
    config.kubernetes.io/local-config: "true"
info:
  description: sample description
					`),
				},
			},
		},
	}
)

func setup(t *testing.T) (mockClient *mockclient.MockClient, mockEngine *mockengine.MockCaDEngine) {
	mockClient = mockclient.NewMockClient(t)
	packagerevisions.coreClient = mockClient
	mockEngine = mockengine.NewMockCaDEngine(t)
	packagerevisions.cad = mockEngine
	mockClient.On("List", mock.Anything, mock.Anything, mock.Anything).Return(func(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
		list.(*configapi.RepositoryList).Items = []configapi.Repository{
			dummyRepoObject,
		}
		return nil
	}).Maybe()

	return
}

func TestList(t *testing.T) {
	_, mockEngine := setup(t)
	mockEngine.On("ListPackageRevisions", mock.Anything, mock.Anything, mock.Anything).Return([]repository.PackageRevision{
		packageRevision,
	}, nil).Once()

	result, err := packagerevisions.List(context.TODO(), &internalversion.ListOptions{})
	assert.NoError(t, err)
	assert.Equal(t, 1, len(result.(*porchapi.PackageRevisionList).Items))

	//=========================================================================================

	result, err = packagerevisions.List(context.TODO(), &internalversion.ListOptions{
		FieldSelector: fields.OneTermEqualSelector("something.crazy", "somethingOffTheWall"),
	})
	assert.Equal(t, nil, result)
	assert.ErrorContains(t, err, "unknown fieldSelector field")

	//=========================================================================================

	mockPkgRev := mockrepo.NewMockPackageRevision(t)
	mockEngine.On("ListPackageRevisions", mock.Anything, mock.Anything, mock.Anything).Return([]repository.PackageRevision{
		mockPkgRev,
	}, nil)
	mockPkgRev.On("GetPackageRevision", mock.Anything).Return(nil, errors.New("error getting API package revision")).Once()
	result, err = packagerevisions.List(context.TODO(), &internalversion.ListOptions{})
	assert.NoError(t, err)
	resultList, isList := result.(*porchapi.PackageRevisionList)
	assert.True(t, isList)
	assert.Equal(t, 0, len(resultList.Items))
}

func TestWatch(t *testing.T) {
	_, mockEngine := setup(t)
	mockWatcherManager := mockengine.NewMockWatcherManager(t)
	mockEngine.On("ObjectCache").Return(mockWatcherManager).Maybe()

	mockWatcherManager.On("WatchPackageRevisions", mock.Anything, mock.Anything, mock.Anything).Return(fmt.Errorf("error starting watch")).Maybe()

	_, err := packagerevisions.Watch(context.TODO(), &internalversion.ListOptions{})
	assert.NoError(t, err)

	//=========================================================================================

	result, err := packagerevisions.Watch(context.TODO(), &internalversion.ListOptions{
		FieldSelector: fields.OneTermEqualSelector("something.crazy", "somethingOffTheWall"),
	})
	assert.Equal(t, nil, result)
	assert.ErrorContains(t, err, "unknown fieldSelector field")

	//=========================================================================================

	result, err = packagerevisions.Watch(request.WithNamespace(context.TODO(), "foo"), &internalversion.ListOptions{
		FieldSelector: fields.OneTermEqualSelector("metadata.namespace", "somethingOffTheWall"),
	})
	assert.Equal(t, nil, result)
	assert.ErrorContains(t, err, "conflicting namespaces specified:")
}

func TestUpdateStrategyForLifecycle(t *testing.T) {
	type testCase struct {
		old     porchapi.PackageRevisionLifecycle
		valid   []porchapi.PackageRevisionLifecycle
		invalid []porchapi.PackageRevisionLifecycle
	}

	s := packageRevisionStrategy{}

	for _, tc := range []testCase{
		{
			old:     "",
			valid:   []porchapi.PackageRevisionLifecycle{"", porchapi.PackageRevisionLifecycleDraft, porchapi.PackageRevisionLifecycleProposed},
			invalid: []porchapi.PackageRevisionLifecycle{"Wrong", porchapi.PackageRevisionLifecyclePublished},
		},
		{
			old:     porchapi.PackageRevisionLifecycleDraft,
			valid:   []porchapi.PackageRevisionLifecycle{"", porchapi.PackageRevisionLifecycleDraft, porchapi.PackageRevisionLifecycleProposed},
			invalid: []porchapi.PackageRevisionLifecycle{"Wrong", porchapi.PackageRevisionLifecyclePublished},
		},
		{
			old:     porchapi.PackageRevisionLifecycleProposed,
			valid:   []porchapi.PackageRevisionLifecycle{"", porchapi.PackageRevisionLifecycleDraft, porchapi.PackageRevisionLifecycleProposed},
			invalid: []porchapi.PackageRevisionLifecycle{"Wrong", porchapi.PackageRevisionLifecyclePublished},
		},
		{
			old:     porchapi.PackageRevisionLifecyclePublished,
			valid:   []porchapi.PackageRevisionLifecycle{porchapi.PackageRevisionLifecyclePublished},
			invalid: []porchapi.PackageRevisionLifecycle{"", "Wrong", porchapi.PackageRevisionLifecycleDraft, porchapi.PackageRevisionLifecycleProposed},
		},
		{
			old:     "Wrong",
			valid:   []porchapi.PackageRevisionLifecycle{},
			invalid: []porchapi.PackageRevisionLifecycle{"", "Wrong", porchapi.PackageRevisionLifecycleDraft, porchapi.PackageRevisionLifecycleProposed, porchapi.PackageRevisionLifecyclePublished},
		},
	} {
		for _, new := range tc.valid {
			testValidateUpdate(t, s, tc.old, new, true)
		}
		for _, new := range tc.invalid {
			testValidateUpdate(t, s, tc.old, new, false)
		}
	}
}

func TestUpdateStrategy(t *testing.T) {
	s := packageRevisionStrategy{}

	testCases := map[string]struct {
		old   *porchapi.PackageRevision
		new   *porchapi.PackageRevision
		valid bool
	}{
		"spec can be updated for draft": {
			old: &porchapi.PackageRevision{
				Spec: porchapi.PackageRevisionSpec{
					Lifecycle: porchapi.PackageRevisionLifecycleDraft,
					Tasks: []porchapi.Task{
						{
							Type: porchapi.TaskTypeInit,
							Init: &porchapi.PackageInitTaskSpec{
								Description: "This is a test",
							},
						},
					},
				},
			},
			new: &porchapi.PackageRevision{
				Spec: porchapi.PackageRevisionSpec{
					Lifecycle: porchapi.PackageRevisionLifecycleDraft,
					Tasks: []porchapi.Task{
						{
							Type: porchapi.TaskTypeInit,
							Init: &porchapi.PackageInitTaskSpec{
								Description: "This is a test",
							},
						},
					},
				},
			},
			valid: true,
		},
		"spec can not be updated for published": {
			old: &porchapi.PackageRevision{
				Spec: porchapi.PackageRevisionSpec{
					Lifecycle: porchapi.PackageRevisionLifecyclePublished,
					Tasks: []porchapi.Task{
						{
							Type: porchapi.TaskTypeInit,
							Init: &porchapi.PackageInitTaskSpec{
								Description: "This is a test",
							},
						},
					},
				},
			},
			new: &porchapi.PackageRevision{
				Spec: porchapi.PackageRevisionSpec{
					Lifecycle: porchapi.PackageRevisionLifecyclePublished,
					Tasks: []porchapi.Task{
						{
							Type: porchapi.TaskTypeClone,
							Clone: &porchapi.PackageCloneTaskSpec{
								Upstream: porchapi.UpstreamPackage{
									Type: porchapi.RepositoryTypeGit,
									Git: &porchapi.GitPackage{
										Repo:      "https://github.com/example/repo.git",
										Ref:       "main",
										Directory: "/path/to/package",
									},
								},
							},
						},
					},
				},
			},
			valid: false,
		},
		"labels can be updated for published": {
			old: &porchapi.PackageRevision{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"foo": "bar",
					},
				},
				Spec: porchapi.PackageRevisionSpec{
					Lifecycle: porchapi.PackageRevisionLifecyclePublished,
				},
			},
			new: &porchapi.PackageRevision{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"bar": "foo",
					},
				},
				Spec: porchapi.PackageRevisionSpec{
					Lifecycle: porchapi.PackageRevisionLifecyclePublished,
				},
			},
			valid: true,
		},
		"annotations can be updated for published": {
			old: &porchapi.PackageRevision{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"foo": "bar",
					},
				},
				Spec: porchapi.PackageRevisionSpec{
					Lifecycle: porchapi.PackageRevisionLifecyclePublished,
				},
			},
			new: &porchapi.PackageRevision{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"bar": "foo",
					},
				},
				Spec: porchapi.PackageRevisionSpec{
					Lifecycle: porchapi.PackageRevisionLifecyclePublished,
				},
			},
			valid: true,
		},
		"cannot modify latest-revision label": {
			old: &porchapi.PackageRevision{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						porchapi.LatestPackageRevisionKey: "true",
					},
				},
				Spec: porchapi.PackageRevisionSpec{
					Lifecycle: porchapi.PackageRevisionLifecycleDraft,
				},
			},
			new: &porchapi.PackageRevision{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						porchapi.LatestPackageRevisionKey: "false",
					},
				},
				Spec: porchapi.PackageRevisionSpec{
					Lifecycle: porchapi.PackageRevisionLifecycleDraft,
				},
			},
			valid: false,
		},
	}

	for tn := range testCases {
		tc := testCases[tn]
		t.Run(tn, func(t *testing.T) {
			ctx := context.Background()
			allErrs := s.ValidateUpdate(ctx, tc.new, tc.old)

			if tc.valid {
				if len(allErrs) > 0 {
					t.Errorf("Update failed unexpectedly: %v", allErrs.ToAggregate().Error())
				}
			} else {
				if len(allErrs) == 0 {
					t.Error("Update should fail but didn't")
				}
			}
		})
	}
}
func TestCreateAction(t *testing.T) {
	testCases := map[string]struct {
		pkgRev   *porchapi.PackageRevision
		expected string
	}{
		"nil package revision": {
			pkgRev:   nil,
			expected: "Create",
		},
		"nil tasks": {
			pkgRev:   &porchapi.PackageRevision{},
			expected: "Create",
		},
		"init task": {
			pkgRev: &porchapi.PackageRevision{
				Spec: porchapi.PackageRevisionSpec{
					Tasks: []porchapi.Task{
						{Type: porchapi.TaskTypeInit},
					},
				},
			},
			expected: "Init",
		},
		"clone task": {
			pkgRev: &porchapi.PackageRevision{
				Spec: porchapi.PackageRevisionSpec{
					Tasks: []porchapi.Task{
						{Type: porchapi.TaskTypeClone},
					},
				},
			},
			expected: "Clone",
		},
		"upgrade task": {
			pkgRev: &porchapi.PackageRevision{
				Spec: porchapi.PackageRevisionSpec{
					Tasks: []porchapi.Task{
						{Type: porchapi.TaskTypeUpgrade},
					},
				},
			},
			expected: "Upgrade",
		},
		"edit task with source": {
			pkgRev: &porchapi.PackageRevision{
				Spec: porchapi.PackageRevisionSpec{
					Tasks: []porchapi.Task{
						{
							Type: porchapi.TaskTypeEdit,
							Edit: &porchapi.PackageEditTaskSpec{
								Source: &porchapi.PackageRevisionRef{},
							},
						},
					},
				},
			},
			expected: "Copy",
		},
		"multiple different task types": {
			pkgRev: &porchapi.PackageRevision{
				Spec: porchapi.PackageRevisionSpec{
					Tasks: []porchapi.Task{
						{Type: porchapi.TaskTypeInit},
						{Type: porchapi.TaskTypeClone},
					},
				},
			},
			expected: "Create",
		},
		"empty tasks slice": {
			pkgRev: &porchapi.PackageRevision{
				Spec: porchapi.PackageRevisionSpec{
					Tasks: []porchapi.Task{},
				},
			},
			expected: "Create",
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			result := createAction(tc.pkgRev)
			assert.Equal(t, tc.expected, result)
		})
	}
}





func TestApprovalUpdateStrategy(t *testing.T) {
	s := packageRevisionApprovalStrategy{}

	type testCase struct {
		old     porchapi.PackageRevisionLifecycle
		valid   []porchapi.PackageRevisionLifecycle
		invalid []porchapi.PackageRevisionLifecycle
	}

	for _, tc := range []testCase{
		{
			old:     "",
			valid:   []porchapi.PackageRevisionLifecycle{},
			invalid: []porchapi.PackageRevisionLifecycle{"", "Wrong", porchapi.PackageRevisionLifecycleDraft, porchapi.PackageRevisionLifecycleProposed, porchapi.PackageRevisionLifecyclePublished, porchapi.PackageRevisionLifecycleDeletionProposed},
		},
		{
			old:     "Wrong",
			valid:   []porchapi.PackageRevisionLifecycle{},
			invalid: []porchapi.PackageRevisionLifecycle{"", "Wrong", porchapi.PackageRevisionLifecycleDraft, porchapi.PackageRevisionLifecycleProposed, porchapi.PackageRevisionLifecyclePublished, porchapi.PackageRevisionLifecycleDeletionProposed},
		},
		{
			old:     porchapi.PackageRevisionLifecycleDraft,
			valid:   []porchapi.PackageRevisionLifecycle{},
			invalid: []porchapi.PackageRevisionLifecycle{"", "Wrong", porchapi.PackageRevisionLifecycleDraft, porchapi.PackageRevisionLifecycleProposed, porchapi.PackageRevisionLifecyclePublished, porchapi.PackageRevisionLifecycleDeletionProposed},
		},
		{
			old:     porchapi.PackageRevisionLifecyclePublished,
			valid:   []porchapi.PackageRevisionLifecycle{porchapi.PackageRevisionLifecycleDeletionProposed},
			invalid: []porchapi.PackageRevisionLifecycle{"", "Wrong", porchapi.PackageRevisionLifecycleDraft, porchapi.PackageRevisionLifecycleProposed, porchapi.PackageRevisionLifecyclePublished},
		},
		{
			old:     porchapi.PackageRevisionLifecycleDeletionProposed,
			valid:   []porchapi.PackageRevisionLifecycle{porchapi.PackageRevisionLifecyclePublished},
			invalid: []porchapi.PackageRevisionLifecycle{"", "Wrong", porchapi.PackageRevisionLifecycleDraft, porchapi.PackageRevisionLifecycleProposed, porchapi.PackageRevisionLifecycleDeletionProposed},
		},
		{
			old:     porchapi.PackageRevisionLifecycleProposed,
			valid:   []porchapi.PackageRevisionLifecycle{porchapi.PackageRevisionLifecycleDraft, porchapi.PackageRevisionLifecyclePublished},
			invalid: []porchapi.PackageRevisionLifecycle{"", "Wrong", porchapi.PackageRevisionLifecycleProposed, porchapi.PackageRevisionLifecycleDeletionProposed},
		},
	} {
		for _, new := range tc.valid {
			testValidateUpdate(t, s, tc.old, new, true)
		}
		for _, new := range tc.invalid {
			testValidateUpdate(t, s, tc.old, new, false)
		}
	}
}





func TestCreationConflictError(t *testing.T) {
	pkgRev := &porchapi.PackageRevision{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test-ns",
		},
		Spec: porchapi.PackageRevisionSpec{
			RepositoryName: "test-repo",
			PackageName:    "test-pkg",
			WorkspaceName:  "test-ws",
		},
	}

	err := creationConflictError(pkgRev)
	assert.Error(t, err)
	assert.ErrorContains(t, err, "test-ns")
	assert.ErrorContains(t, err, "test-repo")
	assert.ErrorContains(t, err, "test-pkg")
	assert.ErrorContains(t, err, "test-ws")
}

func TestUncreatedPackageMutexKey(t *testing.T) {
	pkgRev := &porchapi.PackageRevision{
		ObjectMeta: metav1.ObjectMeta{Namespace: "test-ns"},
		Spec: porchapi.PackageRevisionSpec{
			RepositoryName: "test-repo",
			PackageName:    "test-pkg",
			WorkspaceName:  "test-ws",
		},
	}

	key := uncreatedPackageMutexKey(pkgRev)
	assert.Equal(t, "test-ns-test-repo-test-pkg-test-ws", key)
}

func TestCreate(t *testing.T) {
	tests := []struct {
		name          string
		pkgRev        *porchapi.PackageRevision
		setupMocks    func(*mockclient.MockClient, *mockengine.MockCaDEngine, *mockrepo.MockPackageRevision)
		expectError   bool
		errorContains string
	}{
		{
			name: "success - init task",
			pkgRev: &porchapi.PackageRevision{
				ObjectMeta: metav1.ObjectMeta{Namespace: "default"},
				Spec: porchapi.PackageRevisionSpec{
					RepositoryName: "test-repo",
					PackageName:    "test-pkg",
					WorkspaceName:  "test-ws",
					Tasks:          []porchapi.Task{{Type: porchapi.TaskTypeInit}},
				},
			},
			setupMocks: func(mc *mockclient.MockClient, me *mockengine.MockCaDEngine, mpr *mockrepo.MockPackageRevision) {
				mc.On("Get", mock.Anything, mock.Anything, mock.Anything).Return(nil)
				mc.On("List", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(func(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
					list.(*configapi.RepositoryList).Items = []configapi.Repository{
						{ObjectMeta: metav1.ObjectMeta{Name: "test-repo", Namespace: "default"}},
					}
					return nil
				})
				me.On("ListPackageRevisions", mock.Anything, mock.Anything, mock.Anything).Return([]repository.PackageRevision{}, nil)
				me.On("CreatePackageRevision", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(mpr, nil)
				mpr.On("GetPackageRevision", mock.Anything).Return(&porchapi.PackageRevision{
					ObjectMeta: metav1.ObjectMeta{Name: "test-pkg-test-ws", Namespace: "default"},
					Spec:       porchapi.PackageRevisionSpec{PackageName: "test-pkg"},
				}, nil)
			},
			expectError: false,
		},
		{
			name: "error - no namespace",
			pkgRev: &porchapi.PackageRevision{
				Spec: porchapi.PackageRevisionSpec{RepositoryName: "test-repo"},
			},
			setupMocks:    func(mc *mockclient.MockClient, me *mockengine.MockCaDEngine, mpr *mockrepo.MockPackageRevision) {},
			expectError:   true,
			errorContains: "namespace must be specified",
		},
		{
			name: "error - missing repository name",
			pkgRev: &porchapi.PackageRevision{
				ObjectMeta: metav1.ObjectMeta{Namespace: "default"},
				Spec:       porchapi.PackageRevisionSpec{},
			},
			setupMocks:    func(mc *mockclient.MockClient, me *mockengine.MockCaDEngine, mpr *mockrepo.MockPackageRevision) {},
			expectError:   true,
			errorContains: "spec.repositoryName is required",
		},
		{
			name: "error - repository not found",
			pkgRev: &porchapi.PackageRevision{
				ObjectMeta: metav1.ObjectMeta{Namespace: "default"},
				Spec:       porchapi.PackageRevisionSpec{RepositoryName: "missing-repo"},
			},
			setupMocks: func(mc *mockclient.MockClient, me *mockengine.MockCaDEngine, mpr *mockrepo.MockPackageRevision) {
				mc.On("Get", mock.Anything, mock.Anything, mock.Anything).Return(errors.New("not found"))
			},
			expectError:   true,
			errorContains: "error getting repository",
		},


		{
			name: "error - CreatePackageRevision fails",
			pkgRev: &porchapi.PackageRevision{
				ObjectMeta: metav1.ObjectMeta{Namespace: "default"},
				Spec: porchapi.PackageRevisionSpec{
					RepositoryName: "test-repo",
					PackageName:    "test-pkg",
					WorkspaceName:  "test-ws",
					Tasks:          []porchapi.Task{{Type: porchapi.TaskTypeInit}},
				},
			},
			setupMocks: func(mc *mockclient.MockClient, me *mockengine.MockCaDEngine, mpr *mockrepo.MockPackageRevision) {
				mc.On("Get", mock.Anything, mock.Anything, mock.Anything).Return(nil)
				mc.On("List", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(func(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
					list.(*configapi.RepositoryList).Items = []configapi.Repository{
						{ObjectMeta: metav1.ObjectMeta{Name: "test-repo", Namespace: "default"}},
					}
					return nil
				})
				me.On("ListPackageRevisions", mock.Anything, mock.Anything, mock.Anything).Return([]repository.PackageRevision{}, nil)
				me.On("CreatePackageRevision", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, errors.New("creation failed"))
			},
			expectError:   true,
			errorContains: "Internal error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := mockclient.NewMockClient(t)
			mockEngine := mockengine.NewMockCaDEngine(t)
			mockPkgRev := mockrepo.NewMockPackageRevision(t)

			tt.setupMocks(mockClient, mockEngine, mockPkgRev)

			pr := &packageRevisions{
				TableConvertor: packageRevisionTableConvertor,
				packageCommon: packageCommon{
					scheme:         runtime.NewScheme(),
					gr:             porchapi.Resource("packagerevisions"),
					coreClient:     mockClient,
					cad:            mockEngine,
					updateStrategy: packageRevisionStrategy{},
					createStrategy: packageRevisionStrategy{},
				},
			}

			ctx := context.Background()
			if tt.pkgRev.Namespace != "" {
				ctx = request.WithNamespace(ctx, tt.pkgRev.Namespace)
			}

			result, err := pr.Create(ctx, tt.pkgRev, nil, nil)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.ErrorContains(t, err, tt.errorContains)
				}
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
			}
		})
	}
}
