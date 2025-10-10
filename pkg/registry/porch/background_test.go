package porch

import (
	"context"
	"fmt"
	"testing"
	"time"

	configapi "github.com/nephio-project/porch/api/porchconfig/v1alpha1"
	mockclient "github.com/nephio-project/porch/test/mockery/mocks/external/sigs.k8s.io/controller-runtime/pkg/client"
	mockcache "github.com/nephio-project/porch/test/mockery/mocks/porch/pkg/cache/types"
	mockrepo "github.com/nephio-project/porch/test/mockery/mocks/porch/pkg/repository"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
)

func TestBackground_Options(t *testing.T) {
	tests := []struct {
		name     string
		options  []BackgroundOption
		expected background
	}{
		{
			name:     "With periodic repo sync frequency",
			options:  []BackgroundOption{WithPeriodicRepoSyncFrequency(5 * time.Second)},
			expected: background{periodicRepoSyncFrequency: 5 * time.Second},
		},
		{
			name:     "With list timeout per repo",
			options:  []BackgroundOption{WithListTimeoutPerRepo(10 * time.Second)},
			expected: background{listTimeoutPerRepo: 10 * time.Second},
		},
		{
			name: "With multiple options",
			options: []BackgroundOption{
				WithPeriodicRepoSyncFrequency(5 * time.Second),
				WithListTimeoutPerRepo(10 * time.Second),
			},
			expected: background{
				periodicRepoSyncFrequency: 5 * time.Second,
				listTimeoutPerRepo:        10 * time.Second,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &background{}
			for _, o := range tt.options {
				o.apply(b)
			}
			assert.Equal(t, tt.expected.periodicRepoSyncFrequency, b.periodicRepoSyncFrequency)
			assert.Equal(t, tt.expected.listTimeoutPerRepo, b.listTimeoutPerRepo)
		})
	}
}

func TestBackground_UpdateCache(t *testing.T) {
	mockClient := &mockclient.MockWithWatch{}
	mockCache := &mockcache.MockCache{}

	b := &background{
		coreClient: mockClient,
		cache:      mockCache,
	}

	event := watch.Added
	repository := createRepo()
	repository.Spec.Git.Directory = "invalid//directory"

	err := b.updateCache(context.Background(), event, repository)

	if err != nil {
		assert.Error(t, err)
		assert.Contains(t, err.Error(), fmt.Errorf("handling failed, repo specification invalid").Error())
	}

	event = watch.Bookmark
	err = b.updateCache(context.Background(), event, repository)
	assert.Nil(t, err)
}

func TestBackground_HandleRepositoryEvent(t *testing.T) {
	tests := []struct {
		name          string
		event         watch.EventType
		setupMocks    func(*mockclient.MockWithWatch, *mockclient.MockSubResourceWriter, *mockcache.MockCache, *mockrepo.MockRepository)
		expectedError error
	}{
		{
			name:  "Successfully add repository event",
			event: watch.Added,
			setupMocks: func(mockClient *mockclient.MockWithWatch, mockResourceWriter *mockclient.MockSubResourceWriter,
				mockCache *mockcache.MockCache, mockRepo *mockrepo.MockRepository) {
				mockClient.On("List", mock.Anything, mock.Anything).Return(nil)
				mockCache.On("OpenRepository", mock.Anything, mock.AnythingOfType("*v1alpha1.Repository")).Return(mockRepo, nil)
				mockClient.On("Status").Return(mockResourceWriter)
				mockResourceWriter.On("Update", mock.Anything, mock.Anything).Return(nil)
			},
		},
		{
			name:  "Successfully delete repository event",
			event: watch.Deleted,
			setupMocks: func(mockClient *mockclient.MockWithWatch, mockResourceWriter *mockclient.MockSubResourceWriter,
				mockCache *mockcache.MockCache, mockRepo *mockrepo.MockRepository) {
				mockClient.On("List", mock.Anything, mock.Anything).Return(nil)
				mockCache.On("CloseRepository", mock.Anything, mock.Anything, mock.Anything).Return(nil)
			},
		},
		{
			name:  "Successfully modified repository event",
			event: watch.Modified,
			setupMocks: func(mockClient *mockclient.MockWithWatch, mockResourceWriter *mockclient.MockSubResourceWriter,
				mockCache *mockcache.MockCache, mockRepo *mockrepo.MockRepository) {
				mockClient.On("List", mock.Anything, mock.Anything).Return(nil)
				mockCache.On("OpenRepository", mock.Anything, mock.AnythingOfType("*v1alpha1.Repository")).Return(mockRepo, nil)
				mockRepo.On("Refresh", mock.Anything).Return(nil)
				mockClient.On("Status").Return(mockResourceWriter)
				mockResourceWriter.On("Update", mock.Anything, mock.Anything).Return(nil)
			},
		},
		{
			name:  "Modified event with nil cachedRepo and no error",
			event: watch.Modified,
			setupMocks: func(mockClient *mockclient.MockWithWatch, mockResourceWriter *mockclient.MockSubResourceWriter,
				mockCache *mockcache.MockCache, mockRepo *mockrepo.MockRepository) {
				mockClient.On("List", mock.Anything, mock.Anything).Return(nil)
				mockCache.On("OpenRepository", mock.Anything, mock.AnythingOfType("*v1alpha1.Repository")).Return(nil, nil)
				mockClient.On("Status").Return(mockResourceWriter)
				mockResourceWriter.On("Update", mock.Anything, mock.Anything).Return(nil)
			},
		},
		{
			name:  "Unsuccessful repository deletion event",
			event: watch.Deleted,
			setupMocks: func(mockClient *mockclient.MockWithWatch, mockResourceWriter *mockclient.MockSubResourceWriter,
				mockCache *mockcache.MockCache, mockRepo *mockrepo.MockRepository) {
				mockClient.On("List", mock.Anything, mock.Anything).Return(nil)
				mockCache.On("CloseRepository", mock.Anything, mock.Anything, mock.Anything).Return(fmt.Errorf("error deleting repository"))
			},
			expectedError: fmt.Errorf("error deleting repository"),
		},
		{
			name:  "Invalid repository",
			event: watch.Added,
			setupMocks: func(mockClient *mockclient.MockWithWatch, mockResourceWriter *mockclient.MockSubResourceWriter,
				mockCache *mockcache.MockCache, mockRepo *mockrepo.MockRepository) {
			},
			expectedError: fmt.Errorf("handling failed, repo specification invalid"),
		},
		{
			name:  "Failed to list repository",
			event: watch.Added,
			setupMocks: func(mockClient *mockclient.MockWithWatch, mockResourceWriter *mockclient.MockSubResourceWriter,
				mockCache *mockcache.MockCache, mockRepo *mockrepo.MockRepository) {
				mockClient.On("List", mock.Anything, mock.Anything).Return(fmt.Errorf("error listing repository"))
			},
			expectedError: fmt.Errorf("error listing repository"),
		},
		{
			name:  "List repository timeout",
			event: watch.Added,
			setupMocks: func(mockClient *mockclient.MockWithWatch, mockResourceWriter *mockclient.MockSubResourceWriter,
				mockCache *mockcache.MockCache, mockRepo *mockrepo.MockRepository) {
				mockClient.On("List", mock.Anything, mock.Anything).After(2 * time.Second).Return(fmt.Errorf("error listing repository"))
			},
			expectedError: fmt.Errorf("error listing repository"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := &mockclient.MockWithWatch{}
			mockResourceWriter := &mockclient.MockSubResourceWriter{}
			mockCache := &mockcache.MockCache{}
			mockRepo := &mockrepo.MockRepository{}

			b := &background{
				coreClient:         mockClient,
				cache:              mockCache,
				listTimeoutPerRepo: 1 * time.Second,
			}

			repository := createRepo()
			if tt.name == "Invalid repository" {
				repository.Spec.Git.Directory = "invalid//directory"
			}

			tt.setupMocks(mockClient, mockResourceWriter, mockCache, mockRepo)

			err := b.handleRepositoryEvent(context.Background(), repository, tt.event)

			if tt.expectedError != nil {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError.Error())
			} else {
				assert.NoError(t, err)
			}

			mockClient.AssertExpectations(t)
			mockResourceWriter.AssertExpectations(t)
			mockCache.AssertExpectations(t)
			mockRepo.AssertExpectations(t)
		})
	}
}

func TestBackground_RunOnce(t *testing.T) {
	repository := *createRepo()
	repositories := &configapi.RepositoryList{Items: []configapi.Repository{repository}}

	tests := []struct {
		name          string
		setupMocks    func(*mockclient.MockWithWatch, *mockclient.MockSubResourceWriter, *mockcache.MockCache, *mockrepo.MockRepository)
		expectedError error
	}{
		{
			name: "Successfully list and cache",
			setupMocks: func(mockClient *mockclient.MockWithWatch, mockResourceWriter *mockclient.MockSubResourceWriter,
				mockCache *mockcache.MockCache, mockRepo *mockrepo.MockRepository) {
				mockClient.On("List", mock.Anything, mock.AnythingOfType("*v1alpha1.RepositoryList")).Run(func(args mock.Arguments) {
					repoList := args.Get(1).(*configapi.RepositoryList)
					*repoList = *repositories
				}).Return(nil)
				mockCache.On("OpenRepository", mock.Anything, mock.AnythingOfType("*v1alpha1.Repository")).Return(mockRepo, nil)
				mockClient.On("Status").Return(mockResourceWriter)
				mockResourceWriter.On("Update", mock.Anything, mock.Anything).Return(nil)
			},
		},
		{
			name: "Listing repository failed",
			setupMocks: func(mockClient *mockclient.MockWithWatch, mockResourceWriter *mockclient.MockSubResourceWriter,
				mockCache *mockcache.MockCache, mockRepo *mockrepo.MockRepository) {
				mockClient.On("List", mock.Anything, mock.AnythingOfType("*v1alpha1.RepositoryList")).
					Return(fmt.Errorf("error listing repository objects"))
			},
			expectedError: fmt.Errorf("error listing repository objects"),
		},
		{
			name: "Repository caching failed",
			setupMocks: func(mockClient *mockclient.MockWithWatch, mockResourceWriter *mockclient.MockSubResourceWriter,
				mockCache *mockcache.MockCache, mockRepo *mockrepo.MockRepository) {
				mockClient.On("List", mock.Anything, mock.AnythingOfType("*v1alpha1.RepositoryList")).Run(func(args mock.Arguments) {
					repoList := args.Get(1).(*configapi.RepositoryList)
					*repoList = *repositories
				}).Return(nil)
				mockCache.On("OpenRepository", mock.Anything, mock.AnythingOfType("*v1alpha1.Repository")).Return(mockRepo, nil)
				mockClient.On("Status").Return(mockResourceWriter)
				mockResourceWriter.On("Update", mock.Anything, mock.Anything).Return(fmt.Errorf("status update failed"))
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := &mockclient.MockWithWatch{}
			mockResourceWriter := &mockclient.MockSubResourceWriter{}
			mockCache := &mockcache.MockCache{}
			mockRepo := &mockrepo.MockRepository{}

			b := &background{
				coreClient: mockClient,
				cache:      mockCache,
			}

			tt.setupMocks(mockClient, mockResourceWriter, mockCache, mockRepo)

			err := b.runOnce(context.Background())

			if tt.expectedError != nil {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError.Error())
			} else {
				assert.NoError(t, err)
			}

			mockClient.AssertExpectations(t)
			mockResourceWriter.AssertExpectations(t)
			mockCache.AssertExpectations(t)
			mockRepo.AssertExpectations(t)
		})
	}
}

func TestBackground_CacheRepository(t *testing.T) {
	tests := []struct {
		name          string
		setupMocks    func(*mockclient.MockWithWatch, *mockclient.MockSubResourceWriter, *mockcache.MockCache, *mockrepo.MockRepository)
		expectedError error
		expectedCond  metav1.Condition
	}{
		{
			name: "Successful repository caching",
			setupMocks: func(mockClient *mockclient.MockWithWatch, mockResourceWriter *mockclient.MockSubResourceWriter,
				mockCache *mockcache.MockCache, mockRepo *mockrepo.MockRepository) {
				mockCache.On("OpenRepository", mock.Anything, mock.AnythingOfType("*v1alpha1.Repository")).Return(mockRepo, nil)
				mockClient.On("Status").Return(mockResourceWriter)
				mockResourceWriter.On("Update", mock.Anything, mock.Anything).Return(nil)
			},
			expectedCond: metav1.Condition{
				Type:               configapi.RepositoryReady,
				Status:             metav1.ConditionTrue,
				ObservedGeneration: 1,
				Reason:             configapi.ReasonReady,
				Message:            "Repository Ready",
			},
		},
		{
			name: "Failed to open repository",
			setupMocks: func(mockClient *mockclient.MockWithWatch, mockResourceWriter *mockclient.MockSubResourceWriter,
				mockCache *mockcache.MockCache, mockRepo *mockrepo.MockRepository) {
				mockCache.On("OpenRepository", mock.Anything, mock.AnythingOfType("*v1alpha1.Repository")).
					Return(nil, fmt.Errorf("failed to open repo"))
				mockClient.On("Status").Return(mockResourceWriter)
				mockResourceWriter.On("Update", mock.Anything, mock.Anything).Return(nil)
			},
			expectedCond: metav1.Condition{
				Type:               configapi.RepositoryReady,
				Status:             metav1.ConditionFalse,
				ObservedGeneration: 1,
				Reason:             configapi.ReasonError,
				Message:            "failed to open repo",
			},
		},
		{
			name: "Repository refresh failed",
			setupMocks: func(mockClient *mockclient.MockWithWatch, mockResourceWriter *mockclient.MockSubResourceWriter,
				mockCache *mockcache.MockCache, mockRepo *mockrepo.MockRepository) {
				mockCache.On("OpenRepository", mock.Anything, mock.AnythingOfType("*v1alpha1.Repository")).Return(mockRepo, nil)
				mockClient.On("Status").Return(mockResourceWriter)
				mockResourceWriter.On("Update", mock.Anything, mock.Anything).Return(nil)
			},
			expectedCond: metav1.Condition{
				Type:               configapi.RepositoryReady,
				Status:             metav1.ConditionTrue,
				ObservedGeneration: 1,
				Reason:             configapi.ReasonReady,
				Message:            "Repository Ready",
			},
		},
		{
			name: "Status update failed",
			setupMocks: func(mockClient *mockclient.MockWithWatch, mockResourceWriter *mockclient.MockSubResourceWriter,
				mockCache *mockcache.MockCache, mockRepo *mockrepo.MockRepository) {
				mockCache.On("OpenRepository", mock.Anything, mock.AnythingOfType("*v1alpha1.Repository")).Return(mockRepo, nil)
				mockClient.On("Status").Return(mockResourceWriter)
				mockResourceWriter.On("Update", mock.Anything, mock.Anything).Return(fmt.Errorf("status update failed"))
			},
			expectedError: fmt.Errorf("error updating repository status: status update failed"),
			expectedCond: metav1.Condition{
				Type:               configapi.RepositoryReady,
				Status:             metav1.ConditionTrue,
				ObservedGeneration: 1,
				Reason:             configapi.ReasonReady,
				Message:            "Repository Ready",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := &mockclient.MockWithWatch{}
			mockResourceWriter := &mockclient.MockSubResourceWriter{}
			mockCache := &mockcache.MockCache{}
			mockRepo := &mockrepo.MockRepository{}

			b := &background{
				coreClient: mockClient,
				cache:      mockCache,
			}

			repository := createRepo()

			tt.setupMocks(mockClient, mockResourceWriter, mockCache, mockRepo)

			_, err := b.cacheRepository(context.Background(), repository)

			if tt.expectedError != nil {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError.Error())
			} else {
				assert.NoError(t, err)
			}

			assert.Len(t, repository.Status.Conditions, 1)
			condition := repository.Status.Conditions[0]
			assert.Equal(t, tt.expectedCond.Type, condition.Type)
			assert.Equal(t, tt.expectedCond.Status, condition.Status)
			assert.Equal(t, tt.expectedCond.ObservedGeneration, condition.ObservedGeneration)
			assert.Equal(t, tt.expectedCond.Reason, condition.Reason)
			assert.Equal(t, tt.expectedCond.Message, condition.Message)

			mockClient.AssertExpectations(t)
			mockResourceWriter.AssertExpectations(t)
			mockCache.AssertExpectations(t)
			mockRepo.AssertExpectations(t)
		})
	}
}

func createRepo() *configapi.Repository {
	return &configapi.Repository{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-repo",
			Namespace:  "test-namespace",
			Generation: 1,
		},
		Spec: configapi.RepositorySpec{
			Git: &configapi.GitRepository{
				Directory: "test-dir",
			},
		},
		Status: configapi.RepositoryStatus{
			Conditions: []metav1.Condition{},
		},
	}
}
