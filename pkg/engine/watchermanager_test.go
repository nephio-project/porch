package engine

import (
	"context"
	"testing"

	"github.com/nephio-project/porch/pkg/externalrepo/fake"
	"github.com/nephio-project/porch/pkg/repository"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/watch"
)

type fakeObjectWatcher struct {
	onChange bool
}

func (m *fakeObjectWatcher) OnPackageRevisionChange(eventType watch.EventType, obj repository.PackageRevision) bool {
	return m.onChange
}

func TestWatchPackageRevisions(t *testing.T) {
	manager := NewWatcherManager()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for i := 1; i <= 4; i++ {
		if err := manager.WatchPackageRevisions(ctx, repository.ListPackageRevisionFilter{}, &fakeObjectWatcher{onChange: true}); err != nil {
			t.Fatalf("Failed to add watcher %d: %v", i, err)
		}
	}

	t.Logf("Stopping active watchers")
	cancel()

	if err := manager.WatchPackageRevisions(context.Background(), repository.ListPackageRevisionFilter{}, &fakeObjectWatcher{onChange: false}); err != nil {
		t.Fatalf("Failed to add watcher 5: %v", err)
	}

	if err := manager.WatchPackageRevisions(context.Background(), repository.ListPackageRevisionFilter{}, &fakeObjectWatcher{onChange: false}); err != nil {
		t.Fatalf("Failed to add watcher 6: %v", err)
	}

	watchersActive, slots := countActiveWatchers(manager)

	if watchersActive != 2 {
		t.Errorf("Expected 2 active watchers, but got %d", watchersActive)
	}
	if len(manager.watchers) != 4 {
		t.Errorf("Expected 4 slots, but got %d", slots)
	}
}

func TestNotifyPackageRevisionChange(t *testing.T) {
	manager := NewWatcherManager()

	activeCtx := context.Background()
	inactiveCtx, cancel := context.WithCancel(context.Background())

	if err := manager.WatchPackageRevisions(activeCtx, repository.ListPackageRevisionFilter{}, &fakeObjectWatcher{onChange: true}); err != nil {
		t.Fatalf("Failed to add watcher 1: %v", err)
	}

	if err := manager.WatchPackageRevisions(activeCtx, repository.ListPackageRevisionFilter{}, &fakeObjectWatcher{false}); err != nil {
		t.Fatalf("Failed to add watcher 2: %v", err)
	}

	if err := manager.WatchPackageRevisions(inactiveCtx, repository.ListPackageRevisionFilter{}, &fakeObjectWatcher{false}); err != nil {
		t.Fatalf("Failed to add watcher 3: %v", err)
	}

	cancel()

	pkgRev := &fake.FakePackageRevision{}
	sent := manager.NotifyPackageRevisionChange(watch.Added, pkgRev)
	assert.Equal(t, 2, sent)

	watchersActive, _ := countActiveWatchers(manager)
	assert.Equal(t, 1, watchersActive)
}

func countActiveWatchers(manager *watcherManager) (int, int) {
	active := 0
	for _, watcher := range manager.watchers {
		if watcher != nil && watcher.isDoneFunction() == nil {
			active++
		}
	}
	return active, len(manager.watchers)
}
