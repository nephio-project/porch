package engine

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	api "github.com/nephio-project/porch/api/porch/v1alpha1"
	configapi "github.com/nephio-project/porch/api/porchconfig/v1alpha1"
	"github.com/nephio-project/porch/internal/kpt/builtins"
	"github.com/nephio-project/porch/internal/kpt/fnruntime"
	v1 "github.com/nephio-project/porch/pkg/kpt/api/kptfile/v1"
	"github.com/nephio-project/porch/pkg/kpt/fn"
	"github.com/nephio-project/porch/pkg/repository"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// Test fixtures for common test objects
type testFixture struct {
	mockRepo        *mockRepository
	mockCache       *mockCache
	mockTaskHandler *mockTaskHandler
	repositoryObj   *configapi.Repository
	packageRevision *api.PackageRevision
	engine          *cadEngine
}

func newTestFixture(t *testing.T) *testFixture {
	mockRepo := &mockRepository{}
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

	packageRevision := &api.PackageRevision{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-pkg",
		},
		Spec: api.PackageRevisionSpec{
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

func setupMockPackageRevision(t *testing.T) *mockPackageRevision {
	mockPkgRev := &mockPackageRevision{}
	// Setup common mock package revision expectations
	mockPkgRev.On("Key").Return(repository.PackageRevisionKey{})
	mockPkgRev.On("GetMeta").Return(metav1.ObjectMeta{})
	mockPkgRev.On("GetKptfile", mock.Anything).Return(v1.KptFile{}, nil)
	mockPkgRev.On("KubeObjectName").Return("test-pkg")
	mockPkgRev.On("KubeObjectNamespace").Return("default")
	mockPkgRev.On("UID").Return(types.UID("test-uid"))
	mockPkgRev.On("Lifecycle", mock.Anything).Return(api.PackageRevisionLifecycleDraft)
	mockPkgRev.On("GetPackageRevision", mock.Anything).Return(&api.PackageRevision{}, nil)
	mockPkgRev.On("GetResources", mock.Anything).Return(&api.PackageRevisionResources{}, nil)
	mockPkgRev.On("GetUpstreamLock", mock.Anything).Return(v1.Upstream{}, v1.UpstreamLock{}, nil)
	mockPkgRev.On("GetLock").Return(v1.Upstream{}, v1.UpstreamLock{}, nil)
	mockPkgRev.On("ResourceVersion").Return("1")
	mockPkgRev.On("ToMainPackageRevision").Return(mockPkgRev)
	mockPkgRev.On("SetMeta", mock.Anything, mock.Anything).Return(nil)
	return mockPkgRev
}

type mockPackageRevisionDraft struct {
	mock.Mock
}

func (m *mockPackageRevisionDraft) Key() repository.PackageRevisionKey {
	args := m.Called()
	return args.Get(0).(repository.PackageRevisionKey)
}

func (m *mockPackageRevisionDraft) GetMeta() metav1.ObjectMeta {
	args := m.Called()
	return args.Get(0).(metav1.ObjectMeta)
}

func (m *mockPackageRevisionDraft) UpdateResources(ctx context.Context, new *api.PackageRevisionResources, task *api.Task) error {
	args := m.Called(ctx, new, task)
	return args.Error(0)
}

func (m *mockPackageRevisionDraft) UpdateLifecycle(ctx context.Context, new api.PackageRevisionLifecycle) error {
	args := m.Called(ctx, new)
	return args.Error(0)
}

func TestCreatePackageRevisionRollback(t *testing.T) {
	tests := []struct {
		name          string
		setupTest     func(*testFixture, *mockPackageRevision, *mockPackageRevisionDraft)
		expectedError bool
		errorContains string
	}{
		{
			name: "rollback on task application failure",
			setupTest: func(f *testFixture, mockPkgRev *mockPackageRevision, mockDraft *mockPackageRevisionDraft) {
				mockDraft.On("UpdateResources", mock.Anything, mock.Anything, mock.Anything).Return(nil)
				mockDraft.On("UpdateLifecycle", mock.Anything, mock.Anything).Return(nil)

				f.mockRepo.On("ListPackageRevisions", mock.Anything, mock.Anything).Return([]repository.PackageRevision{}, nil)
				f.mockRepo.On("CreatePackageRevisionDraft", mock.Anything, mock.Anything).Return(mockDraft, nil)
				f.mockRepo.On("ClosePackageRevisionDraft", mock.Anything, mock.Anything, mock.Anything).Return(mockPkgRev, nil)
				f.mockRepo.On("DeletePackageRevision", mock.Anything, mock.Anything).Return(nil)
				f.mockRepo.On("Close").Return(nil)

				f.mockTaskHandler.On("ApplyTasks", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(fmt.Errorf("task application failed"))
			},
			expectedError: true,
			errorContains: "task application failed",
		},
		{
			name: "rollback on lifecycle update failure",
			setupTest: func(f *testFixture, mockPkgRev *mockPackageRevision, mockDraft *mockPackageRevisionDraft) {
				mockDraft.On("UpdateResources", mock.Anything, mock.Anything, mock.Anything).Return(nil)
				mockDraft.On("UpdateLifecycle", mock.Anything, mock.Anything).Return(fmt.Errorf("lifecycle update failed"))

				f.mockRepo.On("ListPackageRevisions", mock.Anything, mock.Anything).Return([]repository.PackageRevision{}, nil)
				f.mockRepo.On("CreatePackageRevisionDraft", mock.Anything, mock.Anything).Return(mockDraft, nil)
				f.mockRepo.On("ClosePackageRevisionDraft", mock.Anything, mock.Anything, mock.Anything).Return(mockPkgRev, nil)
				f.mockRepo.On("DeletePackageRevision", mock.Anything, mock.Anything).Return(nil)
				f.mockRepo.On("Close").Return(nil)

				f.mockTaskHandler.On("ApplyTasks", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
			},
			expectedError: true,
			errorContains: "lifecycle update failed",
		},
		{
			name: "rollback on close draft failure",
			setupTest: func(f *testFixture, mockPkgRev *mockPackageRevision, mockDraft *mockPackageRevisionDraft) {
				mockDraft.On("UpdateResources", mock.Anything, mock.Anything, mock.Anything).Return(nil)
				mockDraft.On("UpdateLifecycle", mock.Anything, mock.Anything).Return(nil)

				f.mockRepo.On("ListPackageRevisions", mock.Anything, mock.Anything).Return([]repository.PackageRevision{}, nil)
				f.mockRepo.On("CreatePackageRevisionDraft", mock.Anything, mock.Anything).Return(mockDraft, nil)
				f.mockRepo.On("ClosePackageRevisionDraft", mock.Anything, mock.Anything, mock.Anything).Return(mockPkgRev, fmt.Errorf("close failed"))
				f.mockRepo.On("Close").Return(nil)

				f.mockTaskHandler.On("ApplyTasks", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
			},
			expectedError: true,
			errorContains: "close failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newTestFixture(t)
			mockPkgRev := setupMockPackageRevision(t)
			mockDraft := &mockPackageRevisionDraft{}
			tt.setupTest(f, mockPkgRev, mockDraft)

			_, err := f.engine.CreatePackageRevision(context.Background(), f.repositoryObj, f.packageRevision, nil)
			if tt.expectedError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorContains)
			} else {
				assert.NoError(t, err)
			}

			f.mockRepo.Close()

			f.mockRepo.AssertExpectations(t)
			f.mockTaskHandler.AssertExpectations(t)
			f.mockCache.AssertExpectations(t)
		})
	}
}

// Mock implementations
type mockRepository struct {
	mock.Mock
}

func (m *mockRepository) ListPackageRevisions(ctx context.Context, filter repository.ListPackageRevisionFilter) ([]repository.PackageRevision, error) {
	args := m.Called(ctx, filter)
	return args.Get(0).([]repository.PackageRevision), args.Error(1)
}

func (m *mockRepository) CreatePackageRevisionDraft(ctx context.Context, obj *api.PackageRevision) (repository.PackageRevisionDraft, error) {
	args := m.Called(ctx, obj)
	return args.Get(0).(repository.PackageRevisionDraft), args.Error(1)
}

func (m *mockRepository) ClosePackageRevisionDraft(ctx context.Context, draft repository.PackageRevisionDraft, version int) (repository.PackageRevision, error) {
	args := m.Called(ctx, draft, version)
	return args.Get(0).(repository.PackageRevision), args.Error(1)
}

func (m *mockRepository) DeletePackageRevision(ctx context.Context, obj repository.PackageRevision) error {
	args := m.Called(ctx, obj)
	return args.Error(0)
}

func (m *mockRepository) UpdatePackageRevision(ctx context.Context, old repository.PackageRevision) (repository.PackageRevisionDraft, error) {
	args := m.Called(ctx, old)
	return args.Get(0).(repository.PackageRevisionDraft), args.Error(1)
}

func (m *mockRepository) ListPackages(ctx context.Context, filter repository.ListPackageFilter) ([]repository.Package, error) {
	args := m.Called(ctx, filter)
	return args.Get(0).([]repository.Package), args.Error(1)
}

func (m *mockRepository) CreatePackage(ctx context.Context, obj *api.PorchPackage) (repository.Package, error) {
	args := m.Called(ctx, obj)
	return args.Get(0).(repository.Package), args.Error(1)
}

func (m *mockRepository) DeletePackage(ctx context.Context, obj repository.Package) error {
	args := m.Called(ctx, obj)
	return args.Error(0)
}

func (m *mockRepository) Version(ctx context.Context) (string, error) {
	args := m.Called(ctx)
	return args.String(0), args.Error(1)
}

func (m *mockRepository) Close() error {
	args := m.Called()
	return args.Error(0)
}

func (m *mockRepository) Refresh(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

type mockPackageRevision struct {
	mock.Mock
}

func (m *mockPackageRevision) Key() repository.PackageRevisionKey {
	args := m.Called()
	return args.Get(0).(repository.PackageRevisionKey)
}

func (m *mockPackageRevision) GetMeta() metav1.ObjectMeta {
	args := m.Called()
	return args.Get(0).(metav1.ObjectMeta)
}

func (m *mockPackageRevision) UpdateResources(ctx context.Context, new *api.PackageRevisionResources, task *api.Task) error {
	args := m.Called(ctx, new, task)
	return args.Error(0)
}

func (m *mockPackageRevision) UpdateLifecycle(ctx context.Context, new api.PackageRevisionLifecycle) error {
	args := m.Called(ctx, new)
	return args.Error(0)
}

func (m *mockPackageRevision) GetKptfile(ctx context.Context) (v1.KptFile, error) {
	args := m.Called()
	return args.Get(0).(v1.KptFile), args.Error(1)
}

func (m *mockPackageRevision) KubeObjectName() string {
	args := m.Called()
	return args.String(0)
}

func (m *mockPackageRevision) KubeObjectNamespace() string {
	args := m.Called()
	return args.String(0)
}

func (m *mockPackageRevision) UID() types.UID {
	args := m.Called()
	return args.Get(0).(types.UID)
}

func (m *mockPackageRevision) Lifecycle(ctx context.Context) api.PackageRevisionLifecycle {
	args := m.Called(ctx)
	return args.Get(0).(api.PackageRevisionLifecycle)
}

func (m *mockPackageRevision) GetPackageRevision(ctx context.Context) (*api.PackageRevision, error) {
	args := m.Called(ctx)
	return args.Get(0).(*api.PackageRevision), args.Error(1)
}

func (m *mockPackageRevision) GetResources(ctx context.Context) (*api.PackageRevisionResources, error) {
	args := m.Called(ctx)
	return args.Get(0).(*api.PackageRevisionResources), args.Error(1)
}

func (m *mockPackageRevision) GetUpstreamLock(ctx context.Context) (v1.Upstream, v1.UpstreamLock, error) {
	args := m.Called(ctx)
	return args.Get(0).(v1.Upstream), args.Get(1).(v1.UpstreamLock), args.Error(2)
}

func (m *mockPackageRevision) GetLock() (v1.Upstream, v1.UpstreamLock, error) {
	args := m.Called()
	return args.Get(0).(v1.Upstream), args.Get(1).(v1.UpstreamLock), args.Error(2)
}

func (m *mockPackageRevision) ResourceVersion() string {
	args := m.Called()
	return args.String(0)
}

func (m *mockPackageRevision) ToMainPackageRevision() repository.PackageRevision {
	args := m.Called()
	return args.Get(0).(repository.PackageRevision)
}

func (m *mockPackageRevision) SetMeta(ctx context.Context, meta metav1.ObjectMeta) error {
	args := m.Called(ctx, meta)
	return args.Error(0)
}

type mockTaskHandler struct {
	mock.Mock
}

func (m *mockTaskHandler) ApplyTasks(ctx context.Context, draft repository.PackageRevisionDraft, repositoryObj *configapi.Repository, obj *api.PackageRevision, packageConfig *builtins.PackageConfig) error {
	args := m.Called(ctx, draft, repositoryObj, obj, packageConfig)
	return args.Error(0)
}

func (m *mockTaskHandler) DoPRMutations(ctx context.Context, namespace string, repoPr repository.PackageRevision, oldObj *api.PackageRevision, newObj *api.PackageRevision, draft repository.PackageRevisionDraft) error {
	args := m.Called(ctx, namespace, repoPr, oldObj, newObj, draft)
	return args.Error(0)
}

func (m *mockTaskHandler) DoPRResourceMutations(ctx context.Context, pr2Update repository.PackageRevision, draft repository.PackageRevisionDraft, oldRes *api.PackageRevisionResources, newRes *api.PackageRevisionResources) (*api.RenderStatus, error) {
	args := m.Called(ctx, pr2Update, draft, oldRes, newRes)
	return args.Get(0).(*api.RenderStatus), args.Error(1)
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

func (m *mockCache) OpenRepository(ctx context.Context, repositoryObj *configapi.Repository) (repository.Repository, error) {
	args := m.Called(ctx, repositoryObj)
	return args.Get(0).(repository.Repository), args.Error(1)
}

func (m *mockCache) CloseRepository(ctx context.Context, repositoryObj *configapi.Repository, repositories []configapi.Repository) error {
	args := m.Called(ctx, repositoryObj, repositories)
	return args.Error(0)
}

func (m *mockCache) GetRepositories(ctx context.Context) []*configapi.Repository {
	args := m.Called(ctx)
	return args.Get(0).([]*configapi.Repository)
}

func (m *mockCache) UpdateRepository(ctx context.Context, repositoryObj *configapi.Repository) error {
	args := m.Called(ctx, repositoryObj)
	return args.Error(0)
}
