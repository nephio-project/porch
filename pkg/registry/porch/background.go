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
	"time"

	configapi "github.com/nephio-project/porch/api/porchconfig/v1alpha1"
	cachetypes "github.com/nephio-project/porch/pkg/cache/types"
	"github.com/nephio-project/porch/pkg/util"
	"k8s.io/apimachinery/pkg/api/meta"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

func WithPeriodicRepoCrSyncFrequency(period time.Duration) BackgroundOption {
	return backgroundOptionFunc(func(b *background) {
		b.periodicRepoCrSyncFrequency = period
	})
}

func WithListTimeoutPerRepo(timeout time.Duration) BackgroundOption {
	return backgroundOptionFunc(func(b *background) {
		b.listTimeoutPerRepo = timeout
	})
}

func RunBackground(ctx context.Context, coreClient client.WithWatch, cache cachetypes.Cache, options ...BackgroundOption) {
	b := &background{
		coreClient: coreClient,
		cache:      cache,
	}

	for _, o := range options {
		o.apply(b)
	}

	go b.run(ctx)
}

// background manages background tasks
type background struct {
	coreClient                  client.WithWatch
	cache                       cachetypes.Cache
	periodicRepoCrSyncFrequency time.Duration
	listTimeoutPerRepo          time.Duration
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
	defer func() {
		if watcher != nil {
			watcher.Stop()
		}
	}()

	reconnect := newBackoffTimer(minReconnectDelay, maxReconnectDelay)
	defer reconnect.Stop()

	// Start ticker
	ticker := time.NewTicker(b.periodicRepoCrSyncFrequency)
	defer ticker.Stop()

loop:
	for {
		select {
		case <-reconnect.channel():
			var err error
			klog.Infof("Starting watch ... ")
			var obj configapi.RepositoryList
			watcher, err = b.coreClient.Watch(ctx, &obj, &client.ListOptions{
				Raw: &v1.ListOptions{
					AllowWatchBookmarks: true,
					ResourceVersion:     bookmark,
				},
			})
			if err != nil {
				klog.Errorf("Cannot start watch: %v; will retry", err)
				reconnect.backoff()
			} else {
				klog.Infof("Watch successfully started.")
				events = watcher.ResultChan()
			}

		case event, eventOk := <-events:
			if !eventOk {
				klog.Errorf("Watch event stream closed. Will restart watch from bookmark %q", bookmark)
				watcher.Stop()
				events = nil
				watcher = nil

				// Initiate reconnect
				reconnect.reset()
			} else if repository, ok := event.Object.(*configapi.Repository); ok {
				if event.Type == watch.Bookmark {
					bookmark = repository.ResourceVersion
					klog.Infof("Bookmark: %q", bookmark)
				} else {
					if err := b.updateCache(ctx, event.Type, repository); err != nil {
						klog.Warningf("error updating cache: %v", err)
					}
				}
			} else {
				klog.V(5).Infof("Received unexpected watch event Object: %T", event.Object)
			}

		case t := <-ticker.C:
			klog.Infof("Background task %s", t)
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

func (b *background) updateCache(ctx context.Context, event watch.EventType, repository *configapi.Repository) error {
	switch event {
	case watch.Added, watch.Modified, watch.Deleted:
		return b.handleRepositoryEvent(ctx, repository, event)
	default:
		klog.Warningf("Unhandled watch event type: %s", event)
	}
	return nil
}

func (b *background) handleRepositoryEvent(ctx context.Context, repo *configapi.Repository, eventType watch.EventType) error {
	msgPreamble := fmt.Sprintf("repository %s event handling: repo %s:%s", eventType, repo.Namespace, repo.Name)
	start := time.Now()
	klog.Infof("%s, handling starting", msgPreamble)

	if err := util.ValidateRepository(repo.Name, repo.Spec.Git.Directory); err != nil {
		return fmt.Errorf("%s, handling failed, repo specification invalid :%q", msgPreamble, err)
	}

	if eventType == watch.Modified && !specChanged(repo) {
		klog.Infof("%s, handling completed in %s", msgPreamble, time.Since(start))
		return nil
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
		err = b.cacheRepository(listCtx, repo, true)
	}
	if err == nil {
		klog.Infof("%s, handling completed in %s", msgPreamble, time.Since(start))
		return nil
	} else {
		return fmt.Errorf("changing repository failed: %s:%s:%q", repo.Namespace, repo.Name, err)
	}
}

func specChanged(repo *configapi.Repository) bool {
	// Compare observedGeneration in status with current generation
	for _, cond := range repo.Status.Conditions {
		if cond.Type == configapi.RepositoryReady {
			return cond.ObservedGeneration < repo.Generation
		}
	}
	// If no condition found, assume spec changed
	return true
}

func (b *background) runOnce(ctx context.Context) error {
	klog.Infof("background-refreshing repositories")
	repositories := &configapi.RepositoryList{}
	if err := b.coreClient.List(ctx, repositories); err != nil {
		return fmt.Errorf("error listing repository objects: %w", err)
	}

	for i := range repositories.Items {
		repo := &repositories.Items[i]

		if err := b.cacheRepository(ctx, repo, false); err != nil {
			klog.Errorf("Failed to cache repository: %v", err)
		}
	}

	return nil
}

func (b *background) cacheRepository(ctx context.Context, repo *configapi.Repository, crModified ...bool) error {
	start := time.Now()
	defer func() {
		klog.V(2).Infof("background::cacheRepository (%s) took %s", repo.Name, time.Since(start))
	}()
	var condition v1.Condition
	modified := false
	if len(crModified) > 0 {
		modified = crModified[0]
	}
	_, err := b.cache.OpenRepository(ctx, repo, modified)
	if err == nil {
		if repo.Status.Conditions == nil {
			// Let's wait for sync job to create condition
			return nil
		} else if repo.Status.Conditions[0].Status == v1.ConditionTrue {
			// Skip if status is already true
			return nil
		}
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
	meta.SetStatusCondition(&repo.Status.Conditions, condition)
	if err := b.coreClient.Status().Update(ctx, repo); err != nil {
		return fmt.Errorf("error updating repository status: %w", err)
	}
	return nil
}

type backoffTimer struct {
	min, max, curr time.Duration
	timer          *time.Timer
}

func newBackoffTimer(min, max time.Duration) *backoffTimer {
	return &backoffTimer{
		min:   min,
		max:   max,
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
