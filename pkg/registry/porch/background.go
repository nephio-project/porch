// Copyright 2022,2025 The kpt and Nephio Authors
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
	"time"

	configapi "github.com/nephio-project/porch/api/porchconfig/v1alpha1"
	cachetypes "github.com/nephio-project/porch/pkg/cache/types"
	"github.com/nephio-project/porch/pkg/util"
	"golang.org/x/sync/semaphore"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type BackgroundOption interface {
	apply(*background)
}

type backgroundOptionFunc func(*background)

func (f backgroundOptionFunc) apply(background *background) {
	f(background)
}

func WithPeriodicRepoSyncFrequency(period time.Duration) BackgroundOption {
	return backgroundOptionFunc(func(b *background) {
		b.periodicRepoSyncFrequency = period
	})
}

func WithListTimeoutPerRepo(timeout time.Duration) BackgroundOption {
	return backgroundOptionFunc(func(b *background) {
		b.listTimeoutPerRepo = timeout
	})
}

func WithMaxConcurrentLists(maxList int) BackgroundOption {
	return backgroundOptionFunc(func(b *background) {
		b.MaxConcurrentLists = maxList
	})
}

func WithRepoOperationRetryAttempts(count int) BackgroundOption {
	return backgroundOptionFunc(func(b *background) {
		b.repoOperationRetryAttempts = count
	})
}

func RunBackground(ctx context.Context, coreClient client.WithWatch, cache cachetypes.Cache, options ...BackgroundOption) {
	b := &background{
		coreClient:  coreClient,
		cache:       cache,
		repoMutexes: make(map[string]*sync.Mutex),
	}

	for _, o := range options {
		o.apply(b)
	}

	// Initialize Git server semaphore with configured MaxConcurrentLists
	b.workerSemaphore = semaphore.NewWeighted(int64(b.MaxConcurrentLists))

	go b.run(ctx)
}

// background manages background tasks
type background struct {
	coreClient                 client.WithWatch
	cache                      cachetypes.Cache
	periodicRepoSyncFrequency  time.Duration
	listTimeoutPerRepo         time.Duration
	repoOperationRetryAttempts int
	MaxConcurrentLists         int
	repoMutexes                map[string]*sync.Mutex // Per-repository mutexes for event ordering
	repoMutexesLock            sync.RWMutex           // Protects repoMutexes map
	workerSemaphore            *semaphore.Weighted    // Rate limit Git server operations
}

const (
	minReconnectDelay = 1 * time.Second
	maxReconnectDelay = 30 * time.Second
)

// run will run until ctx is done
func (b *background) run(ctx context.Context) {
	klog.Infof("Background routine starting ...")

	// Repository watch.
	var events <-chan watch.Event
	var watcher watch.Interface
	var bookmark string
	var consecutiveFailures int
	defer func() {
		if watcher != nil {
			watcher.Stop()
		}
	}()

	reconnect := newBackoffTimer(minReconnectDelay, maxReconnectDelay)
	defer reconnect.Stop()

	// Start ticker
	ticker := time.NewTicker(b.periodicRepoSyncFrequency)
	defer ticker.Stop()

loop:
	for {
		select {
		case <-reconnect.channel():
			// Reset bookmark if too many consecutive failures (stale resource version)
			if consecutiveFailures >= 3 {
				klog.Warningf("Resetting bookmark after %d consecutive failures, was %q", consecutiveFailures, bookmark)
				bookmark = ""
				consecutiveFailures = 0
			}
			klog.Infof("Starting watch ... ")
			var obj configapi.RepositoryList
			var err error
			watcher, err = b.coreClient.Watch(ctx, &obj, &client.ListOptions{
				Raw: &v1.ListOptions{
					AllowWatchBookmarks: true,
					ResourceVersion:     bookmark,
				},
			})
			if err != nil {
				consecutiveFailures++
				// Check for specific API server errors
				if apierrors.IsResourceExpired(err) || apierrors.IsGone(err) {
					klog.Warningf("Watch start failed: %v (bookmark: %q). Resetting bookmark.", err, bookmark)
					bookmark = ""           // Clear stale bookmark immediately
					consecutiveFailures = 0 // Reset since we're handling the root cause
				} else {
					klog.Errorf("Cannot start watch: %v; will retry", err)
				}
				reconnect.backoff()
			} else {
				klog.Infof("Watch successfully started.")
				events = watcher.ResultChan()
			}

		case event, eventOk := <-events:
			if eventOk {
				b.handleWatchEvent(ctx, event, &bookmark, &consecutiveFailures, reconnect)
			} else {
				klog.Errorf("Watch event stream closed. Will restart watch from bookmark %q", bookmark)
				watcher.Stop()
				events = nil
				watcher = nil
				consecutiveFailures++
				// Use exponential backoff for repeated failures
				reconnect.backoff()
			}

		case t := <-ticker.C:
			klog.V(2).Infof("Background task %s", t)
			if err := b.runOnce(ctx); err != nil {
				klog.Errorf("Periodic repository refresh failed: %v", err)
			}

		case <-ctx.Done():
			if ctx.Err() != nil {
				klog.V(2).Infof("exiting background poller, because context is done: %v", ctx.Err())
			} else {
				klog.Infof("Background routine exiting; context done")
			}
			break loop
		}
	}
}

// handleWatchEvent processes individual watch events
func (b *background) handleWatchEvent(ctx context.Context, event watch.Event, bookmark *string, consecutiveFailures *int, reconnect *backoffTimer) {
	switch event.Type {
	case watch.Bookmark: // BOOKMARK events indicate watch stream health
		if repository, ok := event.Object.(*configapi.Repository); ok {
			*consecutiveFailures = 0
			*bookmark = repository.ResourceVersion
			klog.V(2).Infof("Bookmark: %q", *bookmark)
		}
	case watch.Error: // ERROR events indicate watch stream issues
		// Handle watch error events
		if status, ok := event.Object.(*v1.Status); ok {
			if status.Reason == v1.StatusReasonExpired || status.Reason == v1.StatusReasonGone {
				klog.Warningf("Watch error: %s (code: %d) - %s. Resetting bookmark and restarting watch", status.Reason, status.Code, status.Message)
				*bookmark = ""           // Reset bookmark immediately for these errors
				*consecutiveFailures = 0 // Reset failure count since we're handling the root cause
			} else {
				klog.Errorf("Watch error: %s (code: %d) - %s", status.Reason, status.Code, status.Message)
				(*consecutiveFailures)++
			}
		} else {
			klog.Errorf("Watch error event with unexpected object type: %T", event.Object)
			(*consecutiveFailures)++
		}
		reconnect.reset() // Restart quickly after error events
	default: // ADDED, MODIFIED, DELETED events for repository operations
		if repository, ok := event.Object.(*configapi.Repository); ok {
			*consecutiveFailures = 0
			if err := b.updateCache(ctx, event.Type, repository); err != nil {
				klog.Warningf("error updating cache: %v", err)
			}
		} else {
			klog.V(5).Infof("Received unexpected watch event Object: %T", event.Object)
		}
	}
}

func (b *background) updateCache(ctx context.Context, event watch.EventType, repository *configapi.Repository) error {
	switch event {
	case watch.Added, watch.Modified, watch.Deleted:
		// Process directly without channel bottleneck
		b.processRepositoryEvent(ctx, event, repository)
		return nil // Return immediately since processing is async
	default:
		klog.Warningf("Unhandled watch event type: %s", event)
	}
	return nil
}

func (b *background) getRepositoryMutex(repoKey string) *sync.Mutex {
	b.repoMutexesLock.Lock()
	defer b.repoMutexesLock.Unlock()
	mutex, exists := b.repoMutexes[repoKey]
	if !exists {
		mutex = &sync.Mutex{}
		b.repoMutexes[repoKey] = mutex
	}
	return mutex
}

func (b *background) processRepositoryEvent(ctx context.Context, event watch.EventType, repository *configapi.Repository) {
	// Create unique key for the repository
	repoKey := fmt.Sprintf("%s/%s", repository.Namespace, repository.Name)
	go func() {
		mutex := b.getRepositoryMutex(repoKey)
		mutex.Lock()
		defer mutex.Unlock()
		if err := b.handleRepositoryEvent(ctx, repository, event); err != nil {
			klog.Warningf("Processing error for %s:%s: %v", repository.Namespace, repository.Name, err)
		}
	}()
}

func (b *background) handleRepositoryEvent(ctx context.Context, repo *configapi.Repository, eventType watch.EventType) error {
	msgPreamble := fmt.Sprintf("repository %s event handling: repo %s:%s", eventType, repo.Namespace, repo.Name)
	start := time.Now()
	klog.Infof("%s, handling starting", msgPreamble)

	// Rate limit the goroutine processing
	if err := b.workerSemaphore.Acquire(ctx, 1); err != nil {
		return fmt.Errorf("failed to acquire Git server semaphore: %w", err)
	}
	defer b.workerSemaphore.Release(1)

	if err := util.ValidateRepository(repo.Name, repo.Spec.Git.Directory); err != nil {
		return fmt.Errorf("%s, handling failed, repo specification invalid :%q", msgPreamble, err)
	}

	// Verify repositories can be listed (core client is alive)
	listCtx := ctx
	var cancel context.CancelFunc
	if b.listTimeoutPerRepo != 0 {
		listCtx, cancel = context.WithTimeout(ctx, b.listTimeoutPerRepo)
		defer cancel()
	}
	var repoList configapi.RepositoryList
	if err := b.coreClient.List(listCtx, &repoList); err != nil {
		return fmt.Errorf("%s, handling failed, could not list repos using core client :%q", msgPreamble, err)
	}
	var err error
	switch eventType {
	case watch.Deleted:
		err = b.cache.CloseRepository(listCtx, repo, repoList.Items)
	default:
		// Check connectivity before caching
		if err = b.checkRepositoryConnectivity(listCtx, repo); err != nil {
			klog.Warningf("Repository connectivity check failed for %s: %v", repo.Name, err)
			condition := v1.Condition{
				Type:               configapi.RepositoryReady,
				Status:             v1.ConditionFalse,
				ObservedGeneration: repo.Generation,
				LastTransitionTime: v1.Now(),
				Reason:             configapi.ReasonError,
				Message:            fmt.Sprintf("Repository connectivity check failed: %v", err),
			}
			return b.updateRepositoryStatusCondition(listCtx, repo, condition)
		}
		err = b.cacheRepository(listCtx, repo)
	}
	if err == nil {
		klog.Infof("%s, handling completed in %s", msgPreamble, time.Since(start))
		return nil
	} else {
		return fmt.Errorf("changing repository failed: %s:%s:%q", repo.Namespace, repo.Name, err)
	}
}

func (b *background) runOnce(ctx context.Context) error {
	klog.V(2).Infof("background-refreshing repositories")
	repositories := &configapi.RepositoryList{}
	if err := b.coreClient.List(ctx, repositories); err != nil {
		return fmt.Errorf("error listing repository objects: %w", err)
	}

	for i := range repositories.Items {
		repo := &repositories.Items[i]
		func() {
			repoKey := fmt.Sprintf("%s/%s", repo.Namespace, repo.Name)
			mutex := b.getRepositoryMutex(repoKey)
			mutex.Lock()
			defer mutex.Unlock()

			if err := b.checkRepositoryConnectivity(ctx, repo); err != nil {
				klog.Warningf("Repository connectivity check failed for %s: %v", repo.Name, err)
				condition := v1.Condition{
					Type:               configapi.RepositoryReady,
					Status:             v1.ConditionFalse,
					ObservedGeneration: repo.Generation,
					LastTransitionTime: v1.Now(),
					Reason:             configapi.ReasonError,
					Message:            fmt.Sprintf("Repository connectivity check failed: %v", err),
				}
				if err := b.updateRepositoryStatusCondition(ctx, repo, condition); err != nil {
					klog.Errorf("Failed to update repository status for %s: %v", repo.Name, err)
				}
				return
			}

			if err := b.cacheRepository(ctx, repo); err != nil {
				klog.Errorf("Failed to cache repository: %v", err)
			}
		}()
	}

	return nil
}

func (b *background) cacheRepository(ctx context.Context, repo *configapi.Repository) error {
	start := time.Now()
	defer func() {
		klog.V(2).Infof("background::cacheRepository (%s) took %s", repo.Name, time.Since(start))
	}()

	_, err := b.cache.OpenRepository(ctx, repo)

	// Skip if repository is already ready or reconciling
	if err == nil && len(repo.Status.Conditions) > 0 {
		existingCondition := repo.Status.Conditions[0]
		if existingCondition.Reason == configapi.ReasonReady ||
			existingCondition.Reason == configapi.ReasonReconciling {
			return nil
		}
	}

	var condition v1.Condition
	if err == nil {
		condition = v1.Condition{
			Type:               configapi.RepositoryReady,
			Status:             v1.ConditionTrue,
			ObservedGeneration: repo.Generation,
			LastTransitionTime: v1.Now(),
			Reason:             configapi.ReasonReady,
			Message:            "Repository Ready",
		}
	} else {
		condition = v1.Condition{
			Type:               configapi.RepositoryReady,
			Status:             v1.ConditionFalse,
			ObservedGeneration: repo.Generation,
			LastTransitionTime: v1.Now(),
			Reason:             configapi.ReasonError,
			Message:            err.Error(),
		}
	}

	return b.updateRepositoryStatusCondition(ctx, repo, condition)
}

func (b *background) checkRepositoryConnectivity(ctx context.Context, repo *configapi.Repository) error {
	return b.cache.CheckRepositoryConnectivity(ctx, repo)
}

func (b *background) updateRepositoryStatusCondition(ctx context.Context, repo *configapi.Repository, condition v1.Condition) error {
	for attempt := 1; attempt <= b.repoOperationRetryAttempts; attempt++ {
		latestRepo := &configapi.Repository{}
		err := b.coreClient.Get(ctx, types.NamespacedName{
			Namespace: repo.Namespace,
			Name:      repo.Name,
		}, latestRepo)
		if err != nil {
			return fmt.Errorf("failed to get latest repository object: %w", err)
		}

		meta.SetStatusCondition(&latestRepo.Status.Conditions, condition)
		err = b.coreClient.Status().Update(ctx, latestRepo)
		if err == nil {
			return nil
		}

		if apierrors.IsConflict(err) {
			klog.V(3).Infof("Retrying status update for repository %q in namespace %q due to conflict (attempt %d)", repo.Name, repo.Namespace, attempt)
			time.Sleep(100 * time.Millisecond)
			continue
		}
		return fmt.Errorf("error updating repository status: %w", err)
	}
	return fmt.Errorf("failed to update repository status after retries")
}

type backoffTimer struct {
	min, max, curr time.Duration
	timer          *time.Timer
}

func newBackoffTimer(min, max time.Duration) *backoffTimer {
	return &backoffTimer{
		min:   min,
		max:   max,
		curr:  min,
		timer: time.NewTimer(min),
	}
}

func (t *backoffTimer) Stop() bool {
	return t.timer.Stop()
}

func (t *backoffTimer) channel() <-chan time.Time {
	return t.timer.C
}

func (t *backoffTimer) reset() bool {
	t.curr = t.min
	return t.timer.Reset(t.curr)
}

func (t *backoffTimer) backoff() bool {
	curr := t.curr * 2
	if curr > t.max {
		curr = t.max
	}
	t.curr = curr
	return t.timer.Reset(curr)
}
