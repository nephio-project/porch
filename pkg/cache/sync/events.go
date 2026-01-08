// Copyright 2026 The kpt and Nephio Authors
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

package sync

import (
	"context"
	"fmt"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	api "github.com/nephio-project/porch/controllers/repositories/api/v1alpha1"
	"github.com/nephio-project/porch/pkg/repository"
)

const (
	// PorchSyncComponent is the component name for sync events
	PorchSyncComponent = "porch-server"
)

// GlobalDeletionWatcher manages a single watch for all repository deletions
type GlobalDeletionWatcher struct {
	client       client.WithWatch
	syncManagers map[string]*SyncManager // repo key -> sync manager
	mu           sync.RWMutex
	watcher      watch.Interface
	started      bool
}

var globalDeletionWatcher = &GlobalDeletionWatcher{
	syncManagers: make(map[string]*SyncManager),
}

// RegisterSyncManager registers a sync manager for deletion notifications
func RegisterSyncManager(key repository.RepositoryKey, manager *SyncManager) {
	globalDeletionWatcher.mu.Lock()
	defer globalDeletionWatcher.mu.Unlock()
	
	globalDeletionWatcher.syncManagers[key.String()] = manager
	
	// Start global watcher if this is the first sync manager
	if !globalDeletionWatcher.started && manager.coreClient != nil {
		globalDeletionWatcher.client = manager.coreClient
		go globalDeletionWatcher.start()
		globalDeletionWatcher.started = true
	}
	
	klog.V(2).Infof("Registered sync manager for repository %s (total: %d)", 
		key.String(), len(globalDeletionWatcher.syncManagers))
}

// UnregisterSyncManager removes a sync manager from deletion notifications
func UnregisterSyncManager(key repository.RepositoryKey) {
	globalDeletionWatcher.mu.Lock()
	defer globalDeletionWatcher.mu.Unlock()
	
	delete(globalDeletionWatcher.syncManagers, key.String())
	klog.V(2).Infof("Unregistered sync manager for repository %s (remaining: %d)", 
		key.String(), len(globalDeletionWatcher.syncManagers))
}

// start begins watching for all repository deletions
func (g *GlobalDeletionWatcher) start() {
	klog.Info("Starting global repository deletion watcher")
	
	// Start periodic cleanup
	go g.periodicCleanup()
	
	for {
		if err := g.watchRepositories(); err != nil {
			klog.Errorf("Global deletion watcher error: %v, restarting in 5s", err)
			time.Sleep(5 * time.Second)
		}
	}
}

// watchRepositories sets up and maintains the repository watch
func (g *GlobalDeletionWatcher) watchRepositories() error {
	ctx := context.Background()
	
	// Watch all namespaces for Repository deletions
	watcher, err := g.client.Watch(ctx, &api.RepositoryList{}, client.InNamespace(""))
	if err != nil {
		klog.Errorf("Failed to create repository watch: %v", err)
		return err
	}
	defer watcher.Stop()
	
	klog.Info("Global repository deletion watch established")
	
	for event := range watcher.ResultChan() {
		if repo, ok := event.Object.(*api.Repository); ok {
			klog.V(2).Infof("Watch event %s for repository %s/%s", event.Type, repo.Namespace, repo.Name)
		}
		switch event.Type {
		case watch.Deleted:
			if repo, ok := event.Object.(*api.Repository); ok {
				g.handleRepositoryDeletion(repo)
			}
		case watch.Modified:
			// Check for repositories marked for deletion
			if repo, ok := event.Object.(*api.Repository); ok {
				if repo.DeletionTimestamp != nil {
					g.handleRepositoryDeletion(repo)
				}
			}
		}
	}
	
	klog.Warning("Global repository deletion watch channel closed, restarting")
	return nil
}

// handleRepositoryDeletion notifies the appropriate sync manager of deletion
func (g *GlobalDeletionWatcher) handleRepositoryDeletion(repo *api.Repository) {
	repoKey := repository.RepositoryKey{
		Name:      repo.Name,
		Namespace: repo.Namespace,
	}
	
	g.mu.RLock()
	manager, exists := g.syncManagers[repoKey.String()]
	g.mu.RUnlock()
	
	if exists {
		klog.V(1).Infof("Stopping sync manager for deleted repository %s", repoKey.String())
		manager.Stop()
	}
}

// periodicCleanup runs periodic reconciliation to clean up orphaned sync managers
func (g *GlobalDeletionWatcher) periodicCleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	
	for range ticker.C {
		g.reconcileSyncManagers()
	}
}

// reconcileSyncManagers compares registered sync managers against existing repositories
func (g *GlobalDeletionWatcher) reconcileSyncManagers() {
	ctx := context.Background()
	repos := &api.RepositoryList{}
	
	if err := g.client.List(ctx, repos); err != nil {
		klog.Errorf("Failed to list repositories for periodic cleanup: %v", err)
		return
	}
	
	// Build set of existing repositories
	existingRepos := make(map[string]bool)
	for _, repo := range repos.Items {
		repoKey := repository.RepositoryKey{
			Name:      repo.Name,
			Namespace: repo.Namespace,
		}
		existingRepos[repoKey.String()] = true
	}
	
	// Find orphaned sync managers
	g.mu.RLock()
	var orphaned []string
	for repoKey := range g.syncManagers {
		if !existingRepos[repoKey] {
			orphaned = append(orphaned, repoKey)
		}
	}
	g.mu.RUnlock()
	
	// Stop orphaned sync managers
	for _, repoKey := range orphaned {
		g.mu.RLock()
		manager, exists := g.syncManagers[repoKey]
		g.mu.RUnlock()
		
		if exists {
			klog.Infof("Periodic cleanup: stopped sync manager for %s", repoKey)
			manager.Stop()
		}
	}
	
	if len(orphaned) > 0 {
		klog.Infof("Periodic cleanup: stopped %d orphaned sync managers", len(orphaned))
	} else {
		klog.V(3).Infof("Periodic cleanup: no orphaned sync managers found")
	}
}

// NewRepositoryEventRecorder creates a new event recorder for repository sync operations
func NewRepositoryEventRecorder(coreClient client.WithWatch) record.EventRecorder {
	return &SimpleEventRecorder{
		coreClient: coreClient,
		component:  PorchSyncComponent,
	}
}

type SimpleEventRecorder struct {
	coreClient client.WithWatch
	component  string
}

func (r *SimpleEventRecorder) Event(object runtime.Object, eventtype, reason, message string) {
	r.AnnotatedEventf(object, nil, eventtype, reason, "%s", message)
}

func (r *SimpleEventRecorder) Eventf(object runtime.Object, eventtype, reason, messageFmt string, args ...any) {
	r.AnnotatedEventf(object, nil, eventtype, reason, messageFmt, args...)
}

func (r *SimpleEventRecorder) AnnotatedEventf(object runtime.Object, annotations map[string]string, eventtype, reason, messageFmt string, args ...any) {
	r.createEvent(object, annotations, eventtype, reason, messageFmt, args...)
}

func (r *SimpleEventRecorder) createEvent(object runtime.Object, annotations map[string]string, eventtype, reason, messageFmt string, args ...any) {
	ctx := context.Background()
	obj := object.(client.Object)
	now := metav1.Now()
	
	// For SyncStarted events, offset by 1s to ensure correct ordering due to rapid sync cycles
	if reason == "SyncStarted" {
		now.Time = now.Time.Add(-1 * time.Second)
	}
	
	event := &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Name:        fmt.Sprintf("%s.%s.%x", obj.GetName(), reason, now.UnixNano()),
			Namespace:   obj.GetNamespace(),
			Annotations: annotations,
		},
		InvolvedObject: corev1.ObjectReference{
			Kind:       api.TypeRepository.Kind,
			Name:       obj.GetName(),
			Namespace:  obj.GetNamespace(),
			UID:        obj.GetUID(),
			APIVersion: api.TypeRepository.APIVersion(),
		},
		Reason:  reason,
		Message: fmt.Sprintf(messageFmt, args...),
		Type:    eventtype,
		Source: corev1.EventSource{
			Component: r.component,
		},
		FirstTimestamp: now,
		LastTimestamp:  now,
		Count:          1,
	}
	
	if err := r.coreClient.Create(ctx, event); err != nil {
		klog.Errorf("Failed to create event %s: %v", event.Name, err)
	} else {
		klog.V(2).Infof("Created event %s for %s/%s", event.Name, event.Namespace, event.InvolvedObject.Name)
	}
}

// cleanupOldEvents removes old Porch events for a repository, keeping only the most recent ones
func cleanupOldEvents(ctx context.Context, coreClient client.WithWatch, repoName, repoNamespace string, keepCount int) {
	klog.V(2).Infof("Starting cleanup of old events for %s/%s (keepCount=%d)", repoNamespace, repoName, keepCount)
	
	events := &corev1.EventList{}
	listOpts := []client.ListOption{
		client.InNamespace(repoNamespace),
		client.MatchingFields{"involvedObject.name": repoName},
	}
	
	if err := coreClient.List(ctx, events, listOpts...); err != nil {
		klog.Errorf("Failed to list events for cleanup: %v", err)
		return
	}
	
	klog.V(2).Infof("Found %d total events for %s/%s", len(events.Items), repoNamespace, repoName)
	
	// Filter to only Porch events and sort by timestamp (newest first)
	var porchEvents []corev1.Event
	for _, event := range events.Items {
		klog.V(2).Infof("Checking event: %s (component: %s, reason: %s)", event.Name, event.Source.Component, event.Reason)
		if event.Source.Component == PorchSyncComponent {
			porchEvents = append(porchEvents, event)
			klog.V(3).Infof("Found Porch event: %s (timestamp: %v)", event.Name, event.FirstTimestamp)
		} else {
			klog.V(3).Infof("Skipping non-Porch event: %s (component: %s)", event.Name, event.Source.Component)
		}
	}
	
	klog.V(2).Infof("Found %d Porch events for %s/%s", len(porchEvents), repoNamespace, repoName)
	
	if len(porchEvents) <= keepCount {
		klog.V(2).Infof("No cleanup needed: %d events <= %d keepCount", len(porchEvents), keepCount)
		return // Nothing to clean up
	}
	
	// Sort by FirstTimestamp (newest first)
	for i := 0; i < len(porchEvents); i++ {
		for j := i + 1; j < len(porchEvents); j++ {
			if porchEvents[i].FirstTimestamp.Time.Before(porchEvents[j].FirstTimestamp.Time) {
				porchEvents[i], porchEvents[j] = porchEvents[j], porchEvents[i]
			}
		}
	}
	
	// Delete events beyond keepCount
	var toDelete []client.Object
	for i := keepCount; i < len(porchEvents); i++ {
		event := porchEvents[i]
		toDelete = append(toDelete, &event)
		klog.V(2).Infof("Marking event for deletion: %s (timestamp: %v)", event.Name, event.FirstTimestamp)
	}
	
	deleted := 0
	for _, event := range toDelete {
		if err := coreClient.Delete(ctx, event); err != nil {
			klog.Errorf("Failed to delete old event %s: %v", event.GetName(), err)
		} else {
			klog.V(2).Infof("Successfully deleted event: %s", event.GetName())
			deleted++
		}
	}
	
	if deleted > 0 {
		klog.V(1).Infof("Cleaned up %d old events for repository %s/%s (kept %d most recent)", deleted, repoNamespace, repoName, keepCount)
	}
}