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
	"golang.org/x/sync/semaphore"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
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
		expected *background
	}{
		{
			name:     "With periodic repo sync frequency",
			options:  []BackgroundOption{WithPeriodicRepoSyncFrequency(5 * time.Second)},
			expected: &background{periodicRepoSyncFrequency: 5 * time.Second},
		},
		{
			name:     "With list timeout per repo",
			options:  []BackgroundOption{WithListTimeoutPerRepo(10 * time.Second)},
			expected: &background{listTimeoutPerRepo: 10 * time.Second},
		},
		{
			name:     "With repo operation retry attempts",
			options:  []BackgroundOption{WithRepoOperationRetryAttempts(5)},
			expected: &background{repoOperationRetryAttempts: 5},
		},
		{
			name: "With multiple options",
			options: []BackgroundOption{
				WithPeriodicRepoSyncFrequency(5 * time.Second),
				WithListTimeoutPerRepo(10 * time.Second),
				WithRepoOperationRetryAttempts(3),
			},
			expected: &background{
				periodicRepoSyncFrequency:  5 * time.Second,
				listTimeoutPerRepo:         10 * time.Second,
				repoOperationRetryAttempts: 3,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &background{
				workerSemaphore: semaphore.NewWeighted(20),
			}
			for _, o := range tt.options {
				o.apply(b)
			}
			assert.Equal(t, tt.expected.periodicRepoSyncFrequency, b.periodicRepoSyncFrequency)
			assert.Equal(t, tt.expected.listTimeoutPerRepo, b.listTimeoutPerRepo)
			assert.Equal(t, tt.expected.repoOperationRetryAttempts, b.repoOperationRetryAttempts)
		})
	}
}

func TestBackgroundUpdateCache(t *testing.T) {
	mockClient := &mockclient.MockWithWatch{}
	mockCache := &mockcache.MockCache{}

	b := &background{
		coreClient:      mockClient,
		workerSemaphore: semaphore.NewWeighted(20),
		cache:           mockCache,
	}

	// Test invalid repository - should not call OpenRepository
	event := watch.Added
	repository := createRepo(1, 1, false)
	repository.Spec.Git.Directory = "invalid//directory"

	// Expect no calls to cache methods since validation should fail
	mockCache.AssertNotCalled(t, "OpenRepository")
	mockCache.AssertNotCalled(t, "CheckRepositoryConnectivity")

	err := b.updateCache(context.Background(), event, repository)
	assert.Nil(t, err) // updateCache returns nil immediately since processing is async

	// Give goroutine time to complete and fail validation
	time.Sleep(100 * time.Millisecond)

	// Verify cache methods were never called
	mockCache.AssertExpectations(t)
	mockClient.AssertExpectations(t)

	// Test bookmark event
	event = watch.Bookmark
	err = b.updateCache(context.Background(), event, repository)
	assert.Nil(t, err)

	// Verify cache methods expectations for bookmark event
	mockCache.AssertExpectations(t)
	mockClient.AssertExpectations(t)
}

func TestBackgroundHandleWatchEvent(t *testing.T) {
	tests := []struct {
		name                        string
		event                       watch.Event
		initialBookmark             string
		initialConsecutiveFailures  int
		expectedBookmark            string
		expectedConsecutiveFailures int
		expectedReconnectReset      bool
	}{
		{
			name: "Repository bookmark event",
			event: watch.Event{
				Type: watch.Bookmark,
				Object: &configapi.Repository{
					ObjectMeta: v1.ObjectMeta{
						ResourceVersion: "12345",
					},
				},
			},
			initialBookmark:             "old-bookmark",
			initialConsecutiveFailures:  3,
			expectedBookmark:            "12345",
			expectedConsecutiveFailures: 0,
			expectedReconnectReset:      false,
		},
		{
			name: "Watch error - expired status",
			event: watch.Event{
				Type: watch.Error,
				Object: &v1.Status{
					Reason:  v1.StatusReasonExpired,
					Code:    410,
					Message: "Resource version expired",
				},
			},
			initialBookmark:             "old-bookmark",
			initialConsecutiveFailures:  2,
			expectedBookmark:            "",
			expectedConsecutiveFailures: 0,
			expectedReconnectReset:      true,
		},
		{
			name: "Watch error - gone status",
			event: watch.Event{
				Type: watch.Error,
				Object: &v1.Status{
					Reason:  v1.StatusReasonGone,
					Code:    410,
					Message: "Resource gone",
				},
			},
			initialBookmark:             "bookmark",
			initialConsecutiveFailures:  1,
			expectedBookmark:            "",
			expectedConsecutiveFailures: 0,
			expectedReconnectReset:      true,
		},
		{
			name: "Watch error - other status",
			event: watch.Event{
				Type: watch.Error,
				Object: &v1.Status{
					Reason:  "InternalError",
					Code:    500,
					Message: "Internal server error",
				},
			},
			initialBookmark:             "bookmark",
			initialConsecutiveFailures:  1,
			expectedBookmark:            "bookmark",
			expectedConsecutiveFailures: 2,
			expectedReconnectReset:      true,
		},
		{
			name: "Watch error - unexpected object type",
			event: watch.Event{
				Type:   watch.Error,
				Object: &configapi.Repository{}, // Wrong object type for error event
			},
			initialBookmark:             "bookmark",
			initialConsecutiveFailures:  1,
			expectedBookmark:            "bookmark",
			expectedConsecutiveFailures: 2,    // consecutiveFailures incremented because it's an error event with unexpected object type
			expectedReconnectReset:      true, // reconnect.reset() called because it's an error event
		},
		{
			name: "Repository ADDED event",
			event: watch.Event{
				Type: watch.Added,
				Object: &configapi.Repository{
					ObjectMeta: v1.ObjectMeta{
						Name:      "test-repo",
						Namespace: "test-namespace",
					},
					Spec: configapi.RepositorySpec{
						Git: &configapi.GitRepository{
							Repo:      "https://example.com/repo.git",
							Branch:    "main",
							Directory: "/valid/path",
						},
					},
				},
			},
			initialBookmark:             "bookmark",
			initialConsecutiveFailures:  2,
			expectedBookmark:            "bookmark",
			expectedConsecutiveFailures: 0, // Reset to 0 for repository events
			expectedReconnectReset:      false,
		},
		{
			name: "Repository MODIFIED event",
			event: watch.Event{
				Type: watch.Modified,
				Object: &configapi.Repository{
					ObjectMeta: v1.ObjectMeta{
						Name:      "test-repo",
						Namespace: "test-namespace",
					},
					Spec: configapi.RepositorySpec{
						Git: &configapi.GitRepository{
							Repo:      "https://example.com/repo.git",
							Branch:    "main",
							Directory: "/valid/path",
						},
					},
				},
			},
			initialBookmark:             "bookmark",
			initialConsecutiveFailures:  1,
			expectedBookmark:            "bookmark",
			expectedConsecutiveFailures: 0, // Reset to 0 for repository events
			expectedReconnectReset:      false,
		},
		{
			name: "Repository DELETED event",
			event: watch.Event{
				Type: watch.Deleted,
				Object: &configapi.Repository{
					ObjectMeta: v1.ObjectMeta{
						Name:      "test-repo",
						Namespace: "test-namespace",
					},
					Spec: configapi.RepositorySpec{
						Git: &configapi.GitRepository{
							Repo:      "https://example.com/repo.git",
							Branch:    "main",
							Directory: "/valid/path",
						},
					},
				},
			},
			initialBookmark:             "bookmark",
			initialConsecutiveFailures:  3,
			expectedBookmark:            "bookmark",
			expectedConsecutiveFailures: 0, // Reset to 0 for repository events
			expectedReconnectReset:      false,
		},
		{
			name: "Unexpected event object type",
			event: watch.Event{
				Type:   watch.Added,
				Object: &v1.Status{}, // Wrong object type for non-error event
			},
			initialBookmark:             "bookmark",
			initialConsecutiveFailures:  1,
			expectedBookmark:            "bookmark",
			expectedConsecutiveFailures: 1,
			expectedReconnectReset:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := &mockclient.MockWithWatch{}
			mockCache := &mockcache.MockCache{}

			// Add mocks for repository events to verify they get processed
			if tt.name == "Repository ADDED event" || tt.name == "Repository MODIFIED event" {
				// ADDED and MODIFIED events go through the same path (connectivity check + cache)
				mockClient.On("List", mock.Anything, mock.Anything).Return(nil)
				mockCache.On("CheckRepositoryConnectivity", mock.Anything, mock.Anything).Return(nil)
				mockCache.On("OpenRepository", mock.Anything, mock.Anything).Return(nil, nil)
				mockClient.On("Get", mock.Anything, mock.Anything, mock.Anything).Return(nil)
				mockResourceWriter := &mockclient.MockSubResourceWriter{}
				mockResourceWriter.On("Update", mock.Anything, mock.Anything).Return(nil)
				mockClient.On("Status").Return(mockResourceWriter)
			} else if tt.name == "Repository DELETED event" {
				// DELETED events go through a different path (close repository)
				mockClient.On("List", mock.Anything, mock.Anything).Return(nil)
				mockCache.On("CloseRepository", mock.Anything, mock.Anything, mock.Anything).Return(nil)
			}

			b := &background{
				coreClient:                 mockClient,
				workerSemaphore:            semaphore.NewWeighted(20),
				cache:                      mockCache,
				repoOperationRetryAttempts: 3,
			}

			// Initialize test variables
			bookmark := tt.initialBookmark
			consecutiveFailures := tt.initialConsecutiveFailures
			reconnect := newBackoffTimer(minReconnectDelay, maxReconnectDelay)
			defer reconnect.Stop()

			// Track if reconnect.reset() was called
			initialCurr := reconnect.curr
			reconnect.backoff() // Change current value to detect reset

			// Call the function under test
			b.handleWatchEvent(context.Background(), tt.event, &bookmark, &consecutiveFailures, reconnect)

			// For repository events, wait for goroutine to complete and verify cache was called
			if tt.name == "Repository ADDED event" || tt.name == "Repository MODIFIED event" {
				time.Sleep(200 * time.Millisecond) // Give goroutines time to complete
				mockCache.AssertCalled(t, "CheckRepositoryConnectivity", mock.Anything, mock.Anything)
				mockCache.AssertCalled(t, "OpenRepository", mock.Anything, mock.Anything)
			} else if tt.name == "Repository DELETED event" {
				time.Sleep(200 * time.Millisecond) // Give goroutines time to complete
				mockCache.AssertCalled(t, "CloseRepository", mock.Anything, mock.Anything, mock.Anything)
			}

			// Verify results
			assert.Equal(t, tt.expectedBookmark, bookmark)
			assert.Equal(t, tt.expectedConsecutiveFailures, consecutiveFailures)

			if tt.expectedReconnectReset {
				assert.Equal(t, initialCurr, reconnect.curr, "Expected reconnect timer to be reset")
			}

			// Verify cache methods expectations
			mockCache.AssertExpectations(t)
			mockClient.AssertExpectations(t)
		})
	}
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
				mockCache.On("CheckRepositoryConnectivity", mock.Anything, mock.AnythingOfType(v1alpha1Repo)).Return(nil)
				mockCache.On("OpenRepository", mock.Anything, mock.AnythingOfType(v1alpha1Repo), mock.Anything).Return(mockRepo, nil)
				mockClient.On("Status").Return(mockResourceWriter)
				mockResourceWriter.On("Update", mock.Anything, mock.Anything).Return(nil)
				mockClient.On("Get", mock.Anything, mock.AnythingOfType("types.NamespacedName"), mock.AnythingOfType("*v1alpha1.Repository")).
					Run(func(args mock.Arguments) {
						repo := args.Get(2).(*configapi.Repository)
						repo.Status.Conditions = []v1.Condition{}
					}).
					Return(nil)
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
				mockCache.On("CheckRepositoryConnectivity", mock.Anything, mock.AnythingOfType(v1alpha1Repo)).Return(nil)
				mockCache.On("OpenRepository", mock.Anything, mock.AnythingOfType(v1alpha1Repo), mock.Anything).Return(mockRepo, nil)
				mockClient.On("Status").Return(mockResourceWriter)
				mockResourceWriter.On("Update", mock.Anything, mock.Anything).Return(nil)
				mockClient.On("Get", mock.Anything, mock.AnythingOfType("types.NamespacedName"), mock.AnythingOfType("*v1alpha1.Repository")).
					Run(func(args mock.Arguments) {
						repo := args.Get(2).(*configapi.Repository)
						repo.Status.Conditions = []v1.Condition{}
					}).
					Return(nil)
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
		{
			name:  "Repository connectivity check failed",
			event: watch.Added,
			setupMocks: func(mockClient *mockclient.MockWithWatch, mockResourceWriter *mockclient.MockSubResourceWriter,
				mockCache *mockcache.MockCache, mockRepo *mockrepo.MockRepository) {
				mockClient.On("List", mock.Anything, mock.Anything).Return(nil)
				mockCache.On("CheckRepositoryConnectivity", mock.Anything, mock.AnythingOfType(v1alpha1Repo)).Return(fmt.Errorf("remote not found"))
				mockClient.On("Status").Return(mockResourceWriter)
				mockResourceWriter.On("Update", mock.Anything, mock.Anything).
					Run(func(args mock.Arguments) {
						repo := args.Get(1).(*configapi.Repository)
						assert.Len(t, repo.Status.Conditions, 1)
						assert.Equal(t, v1.ConditionFalse, repo.Status.Conditions[0].Status)
						assert.Equal(t, configapi.ReasonError, repo.Status.Conditions[0].Reason)
						assert.Contains(t, repo.Status.Conditions[0].Message, "remote not found")
					}).Return(nil)
				mockClient.On("Get", mock.Anything, mock.AnythingOfType("types.NamespacedName"), mock.AnythingOfType("*v1alpha1.Repository")).
					Run(func(args mock.Arguments) {
						repo := args.Get(2).(*configapi.Repository)
						repo.Status.Conditions = []v1.Condition{}
					}).
					Return(nil)
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
				coreClient:                 mockClient,
				workerSemaphore:            semaphore.NewWeighted(20),
				cache:                      mockCache,
				listTimeoutPerRepo:         1 * time.Second,
				repoOperationRetryAttempts: 3,
			}

			var repository *configapi.Repository
			switch tt.name {
			case "Invalid repository":
				repository = createRepo(2, 1, false)
				repository.Spec.Git.Directory = "invalid//directory"
			case "Successfully add repository event", "Successfully modified repository event":
				repository = createRepo(2, 1, false)
			default:
				repository = createRepo(2, 1, false)
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
				mockCache.On("CheckRepositoryConnectivity", mock.Anything, mock.AnythingOfType(v1alpha1Repo)).Return(nil)
				mockCache.On("OpenRepository", mock.Anything, mock.AnythingOfType(v1alpha1Repo), mock.Anything).Return(mockRepo, nil)
				mockClient.On("Status").Return(mockResourceWriter)
				mockResourceWriter.On("Update", mock.Anything, mock.Anything).
					Run(func(args mock.Arguments) {
						repo := args.Get(1).(*configapi.Repository)
						assert.Len(t, repo.Status.Conditions, 1)
						assert.Equal(t, v1.ConditionTrue, repo.Status.Conditions[0].Status)
						assert.Equal(t, configapi.ReasonReady, repo.Status.Conditions[0].Reason)
					}).Return(nil)
				mockClient.On("Get", mock.Anything, mock.AnythingOfType("types.NamespacedName"), mock.AnythingOfType("*v1alpha1.Repository")).
					Run(func(args mock.Arguments) {
						repo := args.Get(2).(*configapi.Repository)
						repo.Status.Conditions = []v1.Condition{}
					}).
					Return(nil)
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
				mockCache.On("CheckRepositoryConnectivity", mock.Anything, mock.AnythingOfType(v1alpha1Repo)).Return(nil)
				mockCache.On("OpenRepository", mock.Anything, mock.AnythingOfType(v1alpha1Repo), mock.Anything).
					Return(nil, fmt.Errorf("failed to cache"))
				mockClient.On("Status").Return(mockResourceWriter)
				mockResourceWriter.On("Update", mock.Anything, mock.Anything).
					Run(func(args mock.Arguments) {
						repo := args.Get(1).(*configapi.Repository)
						assert.Len(t, repo.Status.Conditions, 1)
						assert.Equal(t, v1.ConditionFalse, repo.Status.Conditions[0].Status)
						assert.Equal(t, configapi.ReasonError, repo.Status.Conditions[0].Reason)
					}).Return(nil)
				mockClient.On("Get", mock.Anything, mock.AnythingOfType("types.NamespacedName"), mock.AnythingOfType("*v1alpha1.Repository")).
					Run(func(args mock.Arguments) {
						repo := args.Get(2).(*configapi.Repository)
						repo.Status.Conditions = []v1.Condition{}
					}).
					Return(nil)
			},
		},
		{
			name: "Repository connectivity check failed",
			setupMocks: func(mockClient *mockclient.MockWithWatch, mockResourceWriter *mockclient.MockSubResourceWriter,
				mockCache *mockcache.MockCache, mockRepo *mockrepo.MockRepository) {
				mockClient.On("List", mock.Anything, mock.AnythingOfType(v1alpha1RepoList)).Run(func(args mock.Arguments) {
					repoList := args.Get(1).(*configapi.RepositoryList)
					*repoList = *repositories
				}).Return(nil)
				mockCache.On("CheckRepositoryConnectivity", mock.Anything, mock.AnythingOfType(v1alpha1Repo)).Return(fmt.Errorf("remote not found"))
				mockClient.On("Status").Return(mockResourceWriter)
				mockResourceWriter.On("Update", mock.Anything, mock.Anything).
					Run(func(args mock.Arguments) {
						repo := args.Get(1).(*configapi.Repository)
						assert.Len(t, repo.Status.Conditions, 1)
						assert.Equal(t, v1.ConditionFalse, repo.Status.Conditions[0].Status)
						assert.Equal(t, configapi.ReasonError, repo.Status.Conditions[0].Reason)
						assert.Contains(t, repo.Status.Conditions[0].Message, "remote not found")
					}).Return(nil)
				mockClient.On("Get", mock.Anything, mock.AnythingOfType("types.NamespacedName"), mock.AnythingOfType("*v1alpha1.Repository")).
					Run(func(args mock.Arguments) {
						repo := args.Get(2).(*configapi.Repository)
						repo.Status.Conditions = []v1.Condition{}
					}).
					Return(nil)
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
				coreClient:                 mockClient,
				workerSemaphore:            semaphore.NewWeighted(20),
				cache:                      mockCache,
				repoOperationRetryAttempts: 3,
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
		setupRepo     func() *configapi.Repository
		expectedError error
		expectedCond  v1.Condition
		skipCondCheck bool
	}{
		{
			name: "Successful repository caching",
			setupMocks: func(mockClient *mockclient.MockWithWatch, mockResourceWriter *mockclient.MockSubResourceWriter,
				mockCache *mockcache.MockCache, mockRepo *mockrepo.MockRepository) {
				mockCache.On("OpenRepository", mock.Anything, mock.AnythingOfType(v1alpha1Repo), mock.Anything).Return(mockRepo, nil)
				mockClient.On("Status").Return(mockResourceWriter)
				mockClient.On("Get", mock.Anything, mock.AnythingOfType("types.NamespacedName"), mock.AnythingOfType("*v1alpha1.Repository")).
					Run(func(args mock.Arguments) {
						repo := args.Get(2).(*configapi.Repository)
						repo.Status.Conditions = []v1.Condition{}
					}).Return(nil)
				mockResourceWriter.On("Update", mock.Anything, mock.AnythingOfType("*v1alpha1.Repository")).Return(nil)
			},
			setupRepo: func() *configapi.Repository { return createRepo(1, 1, false) },
			expectedCond: v1.Condition{
				Type:               configapi.RepositoryReady,
				Status:             v1.ConditionTrue,
				ObservedGeneration: 1,
				Reason:             configapi.ReasonReady,
				Message:            RepositoryReadyMsg,
			},
		},
		{
			name: "Repository already ready - early return",
			setupMocks: func(mockClient *mockclient.MockWithWatch, mockResourceWriter *mockclient.MockSubResourceWriter,
				mockCache *mockcache.MockCache, mockRepo *mockrepo.MockRepository) {
				mockCache.On("OpenRepository", mock.Anything, mock.AnythingOfType(v1alpha1Repo), mock.Anything).Return(mockRepo, nil)
			},
			setupRepo: func() *configapi.Repository {
				repo := createRepo(1, 1, false)
				repo.Status.Conditions = []v1.Condition{{
					Type:   configapi.RepositoryReady,
					Status: v1.ConditionTrue,
					Reason: configapi.ReasonReady,
				}}
				return repo
			},
			skipCondCheck: true,
		},
		{
			name: "Repository reconciling - early return",
			setupMocks: func(mockClient *mockclient.MockWithWatch, mockResourceWriter *mockclient.MockSubResourceWriter,
				mockCache *mockcache.MockCache, mockRepo *mockrepo.MockRepository) {
				mockCache.On("OpenRepository", mock.Anything, mock.AnythingOfType(v1alpha1Repo), mock.Anything).Return(mockRepo, nil)
			},
			setupRepo: func() *configapi.Repository {
				repo := createRepo(1, 1, false)
				repo.Status.Conditions = []v1.Condition{{
					Type:   configapi.RepositoryReady,
					Status: v1.ConditionFalse,
					Reason: configapi.ReasonReconciling,
				}}
				return repo
			},
			skipCondCheck: true,
		},
		{
			name: "Failed to open repository",
			setupMocks: func(mockClient *mockclient.MockWithWatch, mockResourceWriter *mockclient.MockSubResourceWriter,
				mockCache *mockcache.MockCache, mockRepo *mockrepo.MockRepository) {
				mockCache.On("OpenRepository", mock.Anything, mock.AnythingOfType(v1alpha1Repo), mock.Anything).
					Return(nil, fmt.Errorf("failed to open repo"))
				mockClient.On("Status").Return(mockResourceWriter)
				mockClient.On("Get", mock.Anything, mock.AnythingOfType("types.NamespacedName"), mock.AnythingOfType("*v1alpha1.Repository")).
					Run(func(args mock.Arguments) {
						repo := args.Get(2).(*configapi.Repository)
						repo.Status.Conditions = []v1.Condition{}
					}).Return(nil)
				mockResourceWriter.On("Update", mock.Anything, mock.AnythingOfType("*v1alpha1.Repository")).Return(nil)
			},
			setupRepo: func() *configapi.Repository { return createRepo(1, 1, false) },
			expectedCond: v1.Condition{
				Type:               configapi.RepositoryReady,
				Status:             v1.ConditionFalse,
				ObservedGeneration: 1,
				Reason:             configapi.ReasonError,
				Message:            "failed to open repo",
			},
		},
		{
			name: "Get repository failed",
			setupMocks: func(mockClient *mockclient.MockWithWatch, mockResourceWriter *mockclient.MockSubResourceWriter,
				mockCache *mockcache.MockCache, mockRepo *mockrepo.MockRepository) {
				mockCache.On("OpenRepository", mock.Anything, mock.AnythingOfType(v1alpha1Repo), mock.Anything).Return(mockRepo, nil)
				mockClient.On("Get", mock.Anything, mock.AnythingOfType("types.NamespacedName"), mock.AnythingOfType("*v1alpha1.Repository")).
					Return(fmt.Errorf("failed to get repository"))
			},
			setupRepo:     func() *configapi.Repository { return createRepo(1, 1, false) },
			expectedError: fmt.Errorf("failed to get latest repository object: failed to get repository"),
			skipCondCheck: true,
		},
		{
			name: "Status update failed",
			setupMocks: func(mockClient *mockclient.MockWithWatch, mockResourceWriter *mockclient.MockSubResourceWriter,
				mockCache *mockcache.MockCache, mockRepo *mockrepo.MockRepository) {
				mockCache.On("OpenRepository", mock.Anything, mock.AnythingOfType(v1alpha1Repo), mock.Anything).Return(mockRepo, nil)
				mockClient.On("Status").Return(mockResourceWriter)
				mockClient.On("Get", mock.Anything, mock.AnythingOfType("types.NamespacedName"), mock.AnythingOfType("*v1alpha1.Repository")).
					Run(func(args mock.Arguments) {
						repo := args.Get(2).(*configapi.Repository)
						repo.Status.Conditions = []v1.Condition{}
					}).Return(nil)
				mockResourceWriter.On("Update", mock.Anything, mock.AnythingOfType("*v1alpha1.Repository")).Return(fmt.Errorf("status update failed"))
			},
			setupRepo:     func() *configapi.Repository { return createRepo(1, 1, false) },
			expectedError: fmt.Errorf("error updating repository status: status update failed"),
			expectedCond: v1.Condition{
				Type:               configapi.RepositoryReady,
				Status:             v1.ConditionTrue,
				ObservedGeneration: 1,
				Reason:             configapi.ReasonReady,
				Message:            RepositoryReadyMsg,
			},
		},
		{
			name: "API conflict retry success",
			setupMocks: func(mockClient *mockclient.MockWithWatch, mockResourceWriter *mockclient.MockSubResourceWriter,
				mockCache *mockcache.MockCache, mockRepo *mockrepo.MockRepository) {
				mockCache.On("OpenRepository", mock.Anything, mock.AnythingOfType(v1alpha1Repo), mock.Anything).Return(mockRepo, nil)
				mockClient.On("Status").Return(mockResourceWriter)
				// First Get call
				mockClient.On("Get", mock.Anything, mock.AnythingOfType("types.NamespacedName"), mock.AnythingOfType("*v1alpha1.Repository")).
					Run(func(args mock.Arguments) {
						repo := args.Get(2).(*configapi.Repository)
						repo.Status.Conditions = []v1.Condition{}
					}).Return(nil).Once()
				// First Update call - returns conflict
				mockResourceWriter.On("Update", mock.Anything, mock.AnythingOfType("*v1alpha1.Repository")).
					Return(apierrors.NewConflict(schema.GroupResource{Resource: "repositories"}, "test-repo", fmt.Errorf("conflict"))).Once()
				// Second Get call for retry
				mockClient.On("Get", mock.Anything, mock.AnythingOfType("types.NamespacedName"), mock.AnythingOfType("*v1alpha1.Repository")).
					Run(func(args mock.Arguments) {
						repo := args.Get(2).(*configapi.Repository)
						repo.Status.Conditions = []v1.Condition{}
					}).Return(nil).Once()
				// Second Update call - succeeds
				mockResourceWriter.On("Update", mock.Anything, mock.AnythingOfType("*v1alpha1.Repository")).Return(nil).Once()
			},
			setupRepo:     func() *configapi.Repository { return createRepo(1, 1, false) },
			skipCondCheck: true,
		},
		{
			name: "API conflict retry exhausted",
			setupMocks: func(mockClient *mockclient.MockWithWatch, mockResourceWriter *mockclient.MockSubResourceWriter,
				mockCache *mockcache.MockCache, mockRepo *mockrepo.MockRepository) {
				mockCache.On("OpenRepository", mock.Anything, mock.AnythingOfType(v1alpha1Repo), mock.Anything).Return(mockRepo, nil)
				mockClient.On("Status").Return(mockResourceWriter)
				// Three Get calls for three retry attempts
				mockClient.On("Get", mock.Anything, mock.AnythingOfType("types.NamespacedName"), mock.AnythingOfType("*v1alpha1.Repository")).
					Run(func(args mock.Arguments) {
						repo := args.Get(2).(*configapi.Repository)
						repo.Status.Conditions = []v1.Condition{}
					}).Return(nil).Times(3)
				// Three Update calls all return conflict
				mockResourceWriter.On("Update", mock.Anything, mock.AnythingOfType("*v1alpha1.Repository")).
					Return(apierrors.NewConflict(schema.GroupResource{Resource: "repositories"}, "test-repo", fmt.Errorf("conflict"))).Times(3)
			},
			setupRepo:     func() *configapi.Repository { return createRepo(1, 1, false) },
			expectedError: fmt.Errorf("failed to update repository status after retries"),
			skipCondCheck: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := &mockclient.MockWithWatch{}
			mockResourceWriter := &mockclient.MockSubResourceWriter{}
			mockCache := &mockcache.MockCache{}
			mockRepo := &mockrepo.MockRepository{}

			var updatedRepo *configapi.Repository
			if !tt.skipCondCheck && tt.name != "API conflict retry success" && tt.name != "API conflict retry exhausted" {
				mockResourceWriter.On("Update", mock.Anything, mock.AnythingOfType("*v1alpha1.Repository")).
					Run(func(args mock.Arguments) {
						updatedRepo = args.Get(1).(*configapi.Repository)
					}).Return(tt.expectedError)
			}

			b := &background{
				coreClient:                 mockClient,
				workerSemaphore:            semaphore.NewWeighted(20),
				cache:                      mockCache,
				repoOperationRetryAttempts: 3,
			}

			repository := tt.setupRepo()

			tt.setupMocks(mockClient, mockResourceWriter, mockCache, mockRepo)

			err := b.cacheRepository(context.Background(), repository)

			if tt.expectedError != nil {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError.Error())
			} else {
				assert.NoError(t, err)
			}

			if !tt.skipCondCheck {
				assert.NotNil(t, updatedRepo)
				assert.Len(t, updatedRepo.Status.Conditions, 1)
				condition := updatedRepo.Status.Conditions[0]
				assert.Equal(t, tt.expectedCond.Type, condition.Type)
				assert.Equal(t, tt.expectedCond.Status, condition.Status)
				assert.Equal(t, tt.expectedCond.ObservedGeneration, condition.ObservedGeneration)
				assert.Equal(t, tt.expectedCond.Reason, condition.Reason)
				assert.Equal(t, tt.expectedCond.Message, condition.Message)
			}

			mockClient.AssertExpectations(t)
			mockResourceWriter.AssertExpectations(t)
			mockCache.AssertExpectations(t)
			mockRepo.AssertExpectations(t)
		})
	}
}

func createRepo(gen int64, observedGen int64, conditionsNil bool) *configapi.Repository {
	var conditions []v1.Condition
	if !conditionsNil {
		conditions = []v1.Condition{
			{
				Type:               "Ready",
				Status:             v1.ConditionFalse,
				ObservedGeneration: observedGen,
			},
		}
	}

	return &configapi.Repository{
		ObjectMeta: v1.ObjectMeta{
			Name:       "test-repo",
			Namespace:  "test-namespace",
			Generation: gen,
		},
		Spec: configapi.RepositorySpec{
			Type: configapi.RepositoryTypeGit,
			Git: &configapi.GitRepository{
				Repo:      "https://mock-git-server.com/repo.git",
				Branch:    "main",
				Directory: "/valid/path",
			},
		},
		Status: configapi.RepositoryStatus{
			Conditions: conditions,
		},
	}
}
func TestBackoffTimer(t *testing.T) {
	tests := []struct {
		name     string
		min      time.Duration
		max      time.Duration
		expected []time.Duration
	}{
		{
			name:     "Basic backoff progression",
			min:      100 * time.Millisecond,
			max:      800 * time.Millisecond,
			expected: []time.Duration{100 * time.Millisecond, 200 * time.Millisecond, 400 * time.Millisecond, 800 * time.Millisecond, 800 * time.Millisecond},
		},
		{
			name:     "Single step to max",
			min:      1 * time.Second,
			max:      2 * time.Second,
			expected: []time.Duration{1 * time.Second, 2 * time.Second, 2 * time.Second},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			timer := newBackoffTimer(tt.min, tt.max)
			defer timer.Stop()

			// Test initial value
			assert.Equal(t, tt.expected[0], timer.curr)

			// Test backoff progression
			for i := 1; i < len(tt.expected); i++ {
				timer.backoff()
				assert.Equal(t, tt.expected[i], timer.curr)
			}

			// Test reset
			timer.reset()
			assert.Equal(t, tt.min, timer.curr)
		})
	}
}
