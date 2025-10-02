package porch

import (
	"context"
	"errors"
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

const (
	v1alpha1Repo       = "*v1alpha1.Repository"
	v1alpha1RepoList   = "*v1alpha1.RepositoryList"
	listErr            = "error listing repository"
	RepositoryReadyMsg = "Repository Ready"
)

func TestBackgroundOptions(t *testing.T) {
	tests := []struct {
		name     string
		options  []BackgroundOption
		expected background
	}{
		{
			name:     "With periodic repo sync frequency",
			options:  []BackgroundOption{WithPeriodicRepoCrSyncFrequency(5 * time.Second)},
			expected: background{periodicRepoCrSyncFrequency: 5 * time.Second},
		},
		{
			name:     "With list timeout per repo",
			options:  []BackgroundOption{WithListTimeoutPerRepo(10 * time.Second)},
			expected: background{listTimeoutPerRepo: 10 * time.Second},
		},
		{
			name: "With multiple options",
			options: []BackgroundOption{
				WithPeriodicRepoCrSyncFrequency(5 * time.Second),
				WithListTimeoutPerRepo(10 * time.Second),
			},
			expected: background{
				periodicRepoCrSyncFrequency: 5 * time.Second,
				listTimeoutPerRepo:          10 * time.Second,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &background{}
			for _, o := range tt.options {
				o.apply(b)
			}
			assert.Equal(t, tt.expected.periodicRepoCrSyncFrequency, b.periodicRepoCrSyncFrequency)
			assert.Equal(t, tt.expected.listTimeoutPerRepo, b.listTimeoutPerRepo)
		})
	}
}

func TestBackgroundUpdateCache(t *testing.T) {
	mockClient := &mockclient.MockWithWatch{}
	mockCache := &mockcache.MockCache{}

	b := &background{
		coreClient: mockClient,
		cache:      mockCache,
	}

	event := watch.Added
	repository := createRepo(1, 1, false)
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

func TestBackgroundHandleRepositoryEvent(t *testing.T) {
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
				mockCache.On("OpenRepository", mock.Anything, mock.AnythingOfType(v1alpha1Repo), mock.Anything).Return(mockRepo, nil)
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
				mockCache.On("OpenRepository", mock.Anything, mock.AnythingOfType(v1alpha1Repo), mock.Anything).Return(mockRepo, nil)
				mockClient.On("Status").Return(mockResourceWriter)
				mockResourceWriter.On("Update", mock.Anything, mock.Anything).Return(nil)
			},
		},
		{
			name:  "Modified event with unchanged spec",
			event: watch.Modified,
			setupMocks: func(mockClient *mockclient.MockWithWatch, mockResourceWriter *mockclient.MockSubResourceWriter,
				mockCache *mockcache.MockCache, mockRepo *mockrepo.MockRepository) {
				// No mocks needed since it is a no-op
			},
			expectedError: nil,
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
				// No mocks needed since it is a no-op
			},
			expectedError: fmt.Errorf("handling failed, repo specification invalid"),
		},
		{
			name:  "Failed to list repository",
			event: watch.Added,
			setupMocks: func(mockClient *mockclient.MockWithWatch, mockResourceWriter *mockclient.MockSubResourceWriter,
				mockCache *mockcache.MockCache, mockRepo *mockrepo.MockRepository) {
				mockClient.On("List", mock.Anything, mock.Anything).Return(errors.New(listErr))
			},
			expectedError: errors.New(listErr),
		},
		{
			name:  "List repository timeout",
			event: watch.Added,
			setupMocks: func(mockClient *mockclient.MockWithWatch, mockResourceWriter *mockclient.MockSubResourceWriter,
				mockCache *mockcache.MockCache, mockRepo *mockrepo.MockRepository) {
				mockClient.On("List", mock.Anything, mock.Anything).After(2 * time.Second).Return(errors.New(listErr))
			},
			expectedError: errors.New(listErr),
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

			var repository *configapi.Repository
			switch tt.name {
			case "Modified event with unchanged spec":
				repository = createRepo(1, 1, false) // specChanged returns false
			case "Invalid repository":
				repository = createRepo(2, 1, false)
				repository.Spec.Git.Directory = "invalid//directory"
			default:
				repository = createRepo(2, 1, false) // specChanged returns true
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

func TestBackgroundRunOnce(t *testing.T) {
	repository := *createRepo(1, 2, false)
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
				mockClient.On("List", mock.Anything, mock.AnythingOfType(v1alpha1RepoList)).Run(func(args mock.Arguments) {
					repoList := args.Get(1).(*configapi.RepositoryList)
					*repoList = *repositories
				}).Return(nil)
				mockCache.On("OpenRepository", mock.Anything, mock.AnythingOfType(v1alpha1Repo), mock.Anything).Return(mockRepo, nil)
				mockClient.On("Status").Return(mockResourceWriter)
				mockResourceWriter.On("Update", mock.Anything, mock.Anything).Return(nil)
			},
		},
		{
			name: "Listing repository failed",
			setupMocks: func(mockClient *mockclient.MockWithWatch, mockResourceWriter *mockclient.MockSubResourceWriter,
				mockCache *mockcache.MockCache, mockRepo *mockrepo.MockRepository) {
				mockClient.On("List", mock.Anything, mock.AnythingOfType(v1alpha1RepoList)).
					Return(fmt.Errorf("error listing repository objects"))
			},
			expectedError: fmt.Errorf("error listing repository objects"),
		},
		{
			name: "Repository caching failed",
			setupMocks: func(mockClient *mockclient.MockWithWatch, mockResourceWriter *mockclient.MockSubResourceWriter,
				mockCache *mockcache.MockCache, mockRepo *mockrepo.MockRepository) {
				mockClient.On("List", mock.Anything, mock.AnythingOfType(v1alpha1RepoList)).Run(func(args mock.Arguments) {
					repoList := args.Get(1).(*configapi.RepositoryList)
					*repoList = *repositories
				}).Return(nil)
				mockCache.On("OpenRepository", mock.Anything, mock.AnythingOfType(v1alpha1Repo), mock.Anything).
					Return(nil, fmt.Errorf("failed to cache"))
				mockClient.On("Status").Return(mockResourceWriter)
				mockResourceWriter.On("Update", mock.Anything, mock.Anything).Return(nil)

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

func TestBackgroundCacheRepository(t *testing.T) {
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
				mockCache.On("OpenRepository", mock.Anything, mock.AnythingOfType(v1alpha1Repo), mock.Anything).Return(mockRepo, nil)
				mockClient.On("Status").Return(mockResourceWriter)
				mockResourceWriter.On("Update", mock.Anything, mock.Anything).Return(nil)
			},
			expectedCond: metav1.Condition{
				Type:               configapi.RepositoryReady,
				Status:             metav1.ConditionTrue,
				ObservedGeneration: 1,
				Reason:             configapi.ReasonReady,
				Message:            RepositoryReadyMsg,
			},
		},
		{
			name: "Failed to open repository",
			setupMocks: func(mockClient *mockclient.MockWithWatch, mockResourceWriter *mockclient.MockSubResourceWriter,
				mockCache *mockcache.MockCache, mockRepo *mockrepo.MockRepository) {
				mockCache.On("OpenRepository", mock.Anything, mock.AnythingOfType(v1alpha1Repo), mock.Anything).
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
				mockCache.On("OpenRepository", mock.Anything, mock.AnythingOfType(v1alpha1Repo), mock.Anything).Return(mockRepo, nil)
				mockClient.On("Status").Return(mockResourceWriter)
				mockResourceWriter.On("Update", mock.Anything, mock.Anything).Return(nil)
			},
			expectedCond: metav1.Condition{
				Type:               configapi.RepositoryReady,
				Status:             metav1.ConditionTrue,
				ObservedGeneration: 1,
				Reason:             configapi.ReasonReady,
				Message:            RepositoryReadyMsg,
			},
		},
		{
			name: "Status update failed",
			setupMocks: func(mockClient *mockclient.MockWithWatch, mockResourceWriter *mockclient.MockSubResourceWriter,
				mockCache *mockcache.MockCache, mockRepo *mockrepo.MockRepository) {
				mockCache.On("OpenRepository", mock.Anything, mock.AnythingOfType(v1alpha1Repo), mock.Anything).Return(mockRepo, nil)
				mockClient.On("Status").Return(mockResourceWriter)
				mockResourceWriter.On("Update", mock.Anything, mock.Anything).Return(fmt.Errorf("status update failed"))
			},
			expectedError: fmt.Errorf("error updating repository status: status update failed"),
			expectedCond: metav1.Condition{
				Type:               configapi.RepositoryReady,
				Status:             metav1.ConditionTrue,
				ObservedGeneration: 1,
				Reason:             configapi.ReasonReady,
				Message:            RepositoryReadyMsg,
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

			repository := createRepo(1, 1, false)

			tt.setupMocks(mockClient, mockResourceWriter, mockCache, mockRepo)

			err := b.cacheRepository(context.Background(), repository)

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

func createRepo(gen int64, observedGen int64, conditionsNil bool) *configapi.Repository {
	var conditions []metav1.Condition
	if !conditionsNil {
		conditions = []metav1.Condition{
			{
				Type:               "Ready",
				Status:             metav1.ConditionFalse,
				ObservedGeneration: observedGen,
			},
		}
	}

	return &configapi.Repository{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-repo",
			Namespace:  "test-namespace",
			Generation: gen,
		},
		Spec: configapi.RepositorySpec{
			Git: &configapi.GitRepository{
				Directory: "/valid/path",
			},
		},
		Status: configapi.RepositoryStatus{
			Conditions: conditions,
		},
	}
}
