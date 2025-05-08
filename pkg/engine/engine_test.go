package engine

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	api "github.com/nephio-project/porch/api/porch/v1alpha1"
	configapi "github.com/nephio-project/porch/api/porchconfig/v1alpha1"
	"github.com/nephio-project/porch/internal/kpt/builtins"
	"github.com/nephio-project/porch/internal/kpt/fnruntime"
	"github.com/nephio-project/porch/pkg/kpt/fn"
	"github.com/nephio-project/porch/pkg/repository"
)

func TestCreatePackageRevisionRollback(t *testing.T) {
	tests := []struct {
		name          string
		mockSetup     func(*mockRepository)
		expectedError bool
		errorContains string
	}{
		{
			name: "rollback on task application failure",
			mockSetup: func(m *mockRepository) {
				m.On("CreatePackageRevisionDraft", mock.Anything, mock.Anything).Return(&mockPackageRevision{}, nil)
				m.On("ClosePackageRevisionDraft", mock.Anything, mock.Anything, mock.Anything).Return(&mockPackageRevision{}, nil)
				m.On("DeletePackageRevision", mock.Anything, mock.Anything).Return(nil)
			},
			expectedError: true,
			errorContains: "task application failed",
		},
		{
			name: "rollback on lifecycle update failure",
			mockSetup: func(m *mockRepository) {
				m.On("CreatePackageRevisionDraft", mock.Anything, mock.Anything).Return(&mockPackageRevision{}, nil)
				m.On("ClosePackageRevisionDraft", mock.Anything, mock.Anything, mock.Anything).Return(&mockPackageRevision{}, nil)
				m.On("DeletePackageRevision", mock.Anything, mock.Anything).Return(nil)
			},
			expectedError: true,
			errorContains: "lifecycle update failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := &mockRepository{}
			tt.mockSetup(mockRepo)

			engine := &cadEngine{
				taskHandler: &mockTaskHandler{},
			}

			_, err := engine.CreatePackageRevision(context.Background(), &configapi.Repository{}, &api.PackageRevision{}, nil)
			if tt.expectedError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorContains)
			} else {
				assert.NoError(t, err)
			}

			mockRepo.AssertExpectations(t)
		})
	}
}

// Mock implementations
type mockRepository struct {
	mock.Mock
}

type mockPackageRevision struct {
	mock.Mock
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
