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

package engine

import (
	"context"
	"fmt"
	"testing"

	porchapi "github.com/nephio-project/porch/api/porch/v1alpha1"
	"github.com/nephio-project/porch/pkg/externalrepo/fake"
	"github.com/nephio-project/porch/pkg/repository"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	configapi "github.com/nephio-project/porch/api/porchconfig/v1alpha1"
	"github.com/nephio-project/porch/internal/kpt/builtins"
	"github.com/nephio-project/porch/internal/kpt/fnruntime"
	cachetypes "github.com/nephio-project/porch/pkg/cache/types"
	v1 "github.com/nephio-project/porch/pkg/kpt/api/kptfile/v1"
	"github.com/nephio-project/porch/pkg/kpt/fn"
	mockrepo "github.com/nephio-project/porch/test/mockery/mocks/porch/pkg/repository"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// Test fixtures for common test objects
type testFixture struct {
	mockRepo        *mockrepo.MockRepository
	mockCache       *mockCache
	mockTaskHandler *mockTaskHandler
	repositoryObj   *configapi.Repository
	packageRevision *porchapi.PackageRevision
	engine          *cadEngine
}

func newTestFixture(t *testing.T) *testFixture {
	mockRepo := &mockrepo.MockRepository{}
	mockCache := &mockCache{}
	mockTaskHandler := &mockTaskHandler{}

	repositoryObj := &configapi.Repository{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-repo",
			Namespace: "default",
		},
		Spec: configapi.RepositorySpec{
			Git: &configapi.GitRepository{
				Directory: "test-dir",
			},
		},
	}

	packageRevision := &porchapi.PackageRevision{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-pkg",
		},
		Spec: porchapi.PackageRevisionSpec{
			PackageName:    "test-package",
			WorkspaceName:  "test-workspace",
			RepositoryName: "test-repo",
		},
	}

	// Setup common mock expectations
	mockCache.On("OpenRepository", mock.Anything, mock.MatchedBy(func(repo *configapi.Repository) bool {
		return repo.Name == "test-repo" && repo.Namespace == "default"
	})).Return(mockRepo, nil).Once()

	engine := &cadEngine{
		cache:       mockCache,
		taskHandler: mockTaskHandler,
	}

	return &testFixture{
		mockRepo:        mockRepo,
		mockCache:       mockCache,
		mockTaskHandler: mockTaskHandler,
		repositoryObj:   repositoryObj,
		packageRevision: packageRevision,
		engine:          engine,
	}
}

func setupMockPackageRevision(t *testing.T) *mockrepo.MockPackageRevision {
	mockPkgRev := &mockrepo.MockPackageRevision{}
	// Setup common mock package revision expectations
	mockPkgRev.On("Key").Return(repository.PackageRevisionKey{})
	mockPkgRev.On("GetMeta").Return(metav1.ObjectMeta{})
	mockPkgRev.On("GetKptfile", mock.Anything).Return(v1.KptFile{}, nil)
	mockPkgRev.On("KubeObjectName").Return("test-pkg")
	mockPkgRev.On("KubeObjectNamespace").Return("default")
	mockPkgRev.On("UID").Return(types.UID("test-uid"))
	mockPkgRev.On("Lifecycle", mock.Anything).Return(porchapi.PackageRevisionLifecycleDraft)
	mockPkgRev.On("GetPackageRevision", mock.Anything).Return(&porchapi.PackageRevision{}, nil)
	mockPkgRev.On("GetResources", mock.Anything).Return(&porchapi.PackageRevisionResources{}, nil)
	mockPkgRev.On("GetUpstreamLock", mock.Anything).Return(v1.Upstream{}, v1.UpstreamLock{}, nil)
	mockPkgRev.On("GetLock").Return(v1.Upstream{}, v1.UpstreamLock{}, nil)
	mockPkgRev.On("ResourceVersion").Return("1")
	mockPkgRev.On("ToMainPackageRevision", mock.Anything).Return(mockPkgRev)
	mockPkgRev.On("SetMeta", mock.Anything, mock.Anything).Return(nil)
	return mockPkgRev
}

func TestCreatePackageRevisionRollback(t *testing.T) {
	tests := []struct {
		name          string
		setupTest     func(*testFixture, *mockrepo.MockPackageRevision, *mockrepo.MockPackageRevisionDraft)
		expectedError bool
		errorContains string
	}{
		{
			name: "rollback on task application failure",
			setupTest: func(f *testFixture, mockPkgRev *mockrepo.MockPackageRevision, mockDraft *mockrepo.MockPackageRevisionDraft) {
				mockDraft.On("UpdateResources", mock.Anything, mock.Anything, mock.Anything).Return(nil)
				mockDraft.On("UpdateLifecycle", mock.Anything, mock.Anything).Return(nil)

				f.mockRepo.On("ListPackageRevisions", mock.Anything, mock.Anything).Return([]repository.PackageRevision{}, nil)
				f.mockRepo.On("CreatePackageRevisionDraft", mock.Anything, mock.Anything).Return(mockDraft, nil)
				f.mockRepo.On("ClosePackageRevisionDraft", mock.Anything, mock.Anything, mock.Anything).Return(mockPkgRev, nil)
				f.mockRepo.On("DeletePackageRevision", mock.Anything, mock.Anything).Return(nil)
				f.mockRepo.On("Close", mock.Anything).Return(nil)
				f.mockRepo.On("Key", mock.Anything).Return(repository.RepositoryKey{})

				f.mockTaskHandler.On("ApplyTask", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(fmt.Errorf("task application failed"))
			},
			expectedError: true,
			errorContains: "task application failed",
		},
		{
			name: "rollback on lifecycle update failure",
			setupTest: func(f *testFixture, mockPkgRev *mockrepo.MockPackageRevision, mockDraft *mockrepo.MockPackageRevisionDraft) {
				mockDraft.On("UpdateResources", mock.Anything, mock.Anything, mock.Anything).Return(nil)
				mockDraft.On("UpdateLifecycle", mock.Anything, mock.Anything).Return(fmt.Errorf("lifecycle update failed"))

				f.mockRepo.On("ListPackageRevisions", mock.Anything, mock.Anything).Return([]repository.PackageRevision{}, nil)
				f.mockRepo.On("CreatePackageRevisionDraft", mock.Anything, mock.Anything).Return(mockDraft, nil)
				f.mockRepo.On("ClosePackageRevisionDraft", mock.Anything, mock.Anything, mock.Anything).Return(mockPkgRev, nil)
				f.mockRepo.On("DeletePackageRevision", mock.Anything, mock.Anything).Return(nil)
				f.mockRepo.On("Close", mock.Anything).Return(nil)
				f.mockRepo.On("Key", mock.Anything).Return(repository.RepositoryKey{})

				f.mockTaskHandler.On("ApplyTask", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
			},
			expectedError: true,
			errorContains: "lifecycle update failed",
		},
		{
			name: "rollback on close draft failure",
			setupTest: func(f *testFixture, mockPkgRev *mockrepo.MockPackageRevision, mockDraft *mockrepo.MockPackageRevisionDraft) {
				mockDraft.On("UpdateResources", mock.Anything, mock.Anything, mock.Anything).Return(nil)
				mockDraft.On("UpdateLifecycle", mock.Anything, mock.Anything).Return(nil)

				f.mockRepo.On("ListPackageRevisions", mock.Anything, mock.Anything).Return([]repository.PackageRevision{}, nil)
				f.mockRepo.On("CreatePackageRevisionDraft", mock.Anything, mock.Anything).Return(mockDraft, nil)
				f.mockRepo.On("ClosePackageRevisionDraft", mock.Anything, mock.Anything, mock.Anything).Return(mockPkgRev, fmt.Errorf("close failed"))
				f.mockRepo.On("Close", mock.Anything).Return(nil)
				f.mockRepo.On("Key", mock.Anything).Return(repository.RepositoryKey{})

				f.mockTaskHandler.On("ApplyTask", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
			},
			expectedError: true,
			errorContains: "close failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newTestFixture(t)
			mockPkgRev := setupMockPackageRevision(t)
			mockDraft := &mockrepo.MockPackageRevisionDraft{}
			tt.setupTest(f, mockPkgRev, mockDraft)

			_, err := f.engine.CreatePackageRevision(context.Background(), f.repositoryObj, f.packageRevision, nil)
			if tt.expectedError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorContains)
			} else {
				assert.NoError(t, err)
			}

			f.mockRepo.Close(context.Background())

			f.mockRepo.AssertExpectations(t)
			f.mockTaskHandler.AssertExpectations(t)
			f.mockCache.AssertExpectations(t)
		})
	}
}

// Mock implementations
type mockTaskHandler struct {
	mock.Mock
}

func (m *mockTaskHandler) ApplyTask(ctx context.Context, draft repository.PackageRevisionDraft, repositoryObj *configapi.Repository, obj *porchapi.PackageRevision, packageConfig *builtins.PackageConfig) error {
	args := m.Called(ctx, draft, repositoryObj, obj, packageConfig)
	return args.Error(0)
}

func (m *mockTaskHandler) DoPRMutations(ctx context.Context, repoPr repository.PackageRevision, oldObj *porchapi.PackageRevision, newObj *porchapi.PackageRevision, draft repository.PackageRevisionDraft) error {
	args := m.Called(ctx, repoPr, oldObj, newObj, draft)
	return args.Error(0)
}

func (m *mockTaskHandler) DoPRResourceMutations(ctx context.Context, pr2Update repository.PackageRevision, draft repository.PackageRevisionDraft, oldRes *porchapi.PackageRevisionResources, newRes *porchapi.PackageRevisionResources) (*porchapi.RenderStatus, error) {
	args := m.Called(ctx, pr2Update, draft, oldRes, newRes)
	return args.Get(0).(*porchapi.RenderStatus), args.Error(1)
}

func (m *mockTaskHandler) GetRuntime() fn.FunctionRuntime {
	args := m.Called()
	return args.Get(0).(fn.FunctionRuntime)
}

func (m *mockTaskHandler) SetCredentialResolver(resolver repository.CredentialResolver) {
	m.Called(resolver)
}

func (m *mockTaskHandler) SetReferenceResolver(resolver repository.ReferenceResolver) {
	m.Called(resolver)
}

func (m *mockTaskHandler) SetRepoOpener(opener repository.RepositoryOpener) {
	m.Called(opener)
}

func (m *mockTaskHandler) SetRunnerOptionsResolver(resolver func(namespace string) fnruntime.RunnerOptions) {
	m.Called(resolver)
}

func (m *mockTaskHandler) SetRuntime(runtime fn.FunctionRuntime) {
	m.Called(runtime)
}

type mockCache struct {
	mock.Mock
}

var _ cachetypes.Cache = &mockCache{}

func (m *mockCache) OpenRepository(ctx context.Context, repositoryObj *configapi.Repository) (repository.Repository, error) {
	args := m.Called(ctx, repositoryObj)
	return args.Get(0).(repository.Repository), args.Error(1)
}

func (m *mockCache) CloseRepository(ctx context.Context, repositoryObj *configapi.Repository, repositories []configapi.Repository) error {
	args := m.Called(ctx, repositoryObj, repositories)
	return args.Error(0)
}

func (m *mockCache) GetRepositories() []*configapi.Repository {
	args := m.Called()
	return args.Get(0).([]*configapi.Repository)
}

func (m *mockCache) GetRepository(repoKey repository.RepositoryKey) repository.Repository {
	args := m.Called()
	return args.Get(0).(repository.Repository)
}

func (m *mockCache) UpdateRepository(ctx context.Context, repositoryObj *configapi.Repository) error {
	args := m.Called(ctx, repositoryObj)
	return args.Error(0)
}

func TestCreatePRWith2Tasks(t *testing.T) {
	pr := &porchapi.PackageRevision{
		Spec: porchapi.PackageRevisionSpec{
			Tasks: []porchapi.Task{
				{
					Type: porchapi.TaskTypeInit,
					Init: &porchapi.PackageInitTaskSpec{},
				},
				{
					Type: porchapi.TaskTypeEdit,
					Edit: &porchapi.PackageEditTaskSpec{
						Source: &porchapi.PackageRevisionRef{
							Name: "test",
						},
					},
				},
			},
		},
	}

	engine := &cadEngine{}

	_, err := engine.CreatePackageRevision(context.TODO(), nil, pr, nil)
	assert.ErrorContains(t, err, "must not contain more than one")
}

func TestCreatePRInitIsAdded(t *testing.T) {
	pr := &porchapi.PackageRevision{
		Spec: porchapi.PackageRevisionSpec{
			// short-circuit the method
			Lifecycle: "test",
		},
	}

	engine := &cadEngine{}

	_, err := engine.CreatePackageRevision(context.TODO(), nil, pr, nil)

	require.ErrorContains(t, err, "unsupported lifecycle value")
	require.Len(t, pr.Spec.Tasks, 1)
	require.Equal(t, pr.Spec.Tasks[0].Type, porchapi.TaskTypeInit)
}

func TestValidateUpgradeTask(t *testing.T) {
	oldUpstream := &fake.FakePackageRevision{
		PrKey: repository.PackageRevisionKey{
			PkgKey: repository.PackageKey{
				RepoKey: repository.RepositoryKey{
					Name: "blueprint",
				},
				Package: "test-package",
			},
			WorkspaceName: "v1",
		},
		PackageLifecycle: porchapi.PackageRevisionLifecyclePublished,
	}

	newUpstream := oldUpstream
	newUpstream.PrKey.WorkspaceName = "v2"

	t.Run("Successful", func(t *testing.T) {
		local := &fake.FakePackageRevision{
			PrKey: repository.PackageRevisionKey{
				PkgKey: repository.PackageKey{
					RepoKey: repository.RepositoryKey{
						Name: "deployment",
					},
					Package: "test-package",
				},
				WorkspaceName: "v1",
			},
			PackageLifecycle: porchapi.PackageRevisionLifecyclePublished,
		}

		revs := []repository.PackageRevision{oldUpstream, newUpstream, local}
		spec := &porchapi.PackageUpgradeTaskSpec{
			OldUpstream: porchapi.PackageRevisionRef{
				Name: oldUpstream.KubeObjectName(),
			},
			NewUpstream: porchapi.PackageRevisionRef{
				Name: newUpstream.KubeObjectName(),
			},
			LocalPackageRevisionRef: porchapi.PackageRevisionRef{
				Name: local.KubeObjectName(),
			},
		}

		err := validateUpgradeTask(context.TODO(), revs, spec)
		assert.NoError(t, err)
	})

	t.Run("Failure", func(t *testing.T) {
		local := &fake.FakePackageRevision{
			PrKey: repository.PackageRevisionKey{
				PkgKey: repository.PackageKey{
					RepoKey: repository.RepositoryKey{
						Name: "deployment",
					},
					Package: "test-package",
				},
				WorkspaceName: "v1",
			},
			PackageLifecycle: porchapi.PackageRevisionLifecycleDraft,
		}

		revs := []repository.PackageRevision{oldUpstream, newUpstream, local}
		spec := &porchapi.PackageUpgradeTaskSpec{
			OldUpstream: porchapi.PackageRevisionRef{
				Name: oldUpstream.KubeObjectName(),
			},
			NewUpstream: porchapi.PackageRevisionRef{
				Name: newUpstream.KubeObjectName(),
			},
			LocalPackageRevisionRef: porchapi.PackageRevisionRef{
				Name: local.KubeObjectName(),
			},
		}

		err := validateUpgradeTask(context.TODO(), revs, spec)
		assert.ErrorContains(t, err, "must be published")
		assert.ErrorContains(t, err, local.KubeObjectName())
	})
}
