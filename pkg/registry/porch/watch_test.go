// Copyright 2022, 2024-2025 The kpt and Nephio Authors
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
	"sync"
	"testing"
	"time"

	porchapi "github.com/nephio-project/porch/api/porch/v1alpha1"
	"github.com/nephio-project/porch/pkg/engine"
	"github.com/nephio-project/porch/pkg/externalrepo/fake"
	"github.com/nephio-project/porch/pkg/repository"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
)

func TestWatcherClose(t *testing.T) {
	ctx := context.Background()
	ctx, cancelFunc := context.WithCancel(ctx)

	w := &watcher{
		cancel:     cancelFunc,
		resultChan: make(chan watch.Event, 64),
	}

	r := &fakePackageReader{}
	r.Add(1)
	var filter repository.ListPackageRevisionFilter

	go w.listAndWatch(ctx, r, filter)

	// Just make sure someone is pulling events of the result channel.
	go func() {
		for range w.resultChan {
			// do nothing
		}
	}()

	// Wait until the callback has been set in the fakePackageReader
	r.Wait()

	// Create lots of watch events for the next 2 seconds.
	timer := time.NewTimer(2 * time.Second)
	go func() {
		ch := make(chan struct{})
		close(ch)
		for {
			select {
			case <-ch:
				pkgRev := &fake.FakePackageRevision{
					PackageRevision: &porchapi.PackageRevision{
						ObjectMeta: metav1.ObjectMeta{
							Labels: make(map[string]string),
						},
					},
				}
				if cont := r.callback.OnPackageRevisionChange(watch.Modified, pkgRev); !cont {
					return
				}
			case <-timer.C:
				return
			}
		}
	}()

	// Close the watcher while watch events are being sent.
	<-time.NewTimer(1 * time.Second).C
	cancelFunc()
	<-timer.C
}

type fakePackageReader struct {
	sync.WaitGroup
	callback engine.ObjectWatcher
}

func (f *fakePackageReader) watchPackages(ctx context.Context, filter repository.ListPackageRevisionFilter, callback engine.ObjectWatcher) error {
	f.callback = callback
	f.Done()
	return nil
}

func (f *fakePackageReader) listPackageRevisions(ctx context.Context, filter repository.ListPackageRevisionFilter, callback func(ctx context.Context, p repository.PackageRevision) error) error {
	return nil
}
func TestWatcherNilObject(t *testing.T) {
	tests := []struct {
		name             string
		packages         []repository.PackageRevision
		waitForStreaming bool
		sendEvent        bool
	}{
		{
			name:      "backlog phase",
			packages:  nil,
			sendEvent: false,
		},
		{
			name:             "streaming phase",
			packages:         nil,
			waitForStreaming: true,
			sendEvent:        true,
		},
		{
			name: "list phase",
			packages: []repository.PackageRevision{
				&fake.FakePackageRevision{
					PackageRevision: &porchapi.PackageRevision{
						ObjectMeta: metav1.ObjectMeta{
							Labels: make(map[string]string),
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancelFunc := context.WithCancel(context.Background())
			defer cancelFunc()

			w := &watcher{
				cancel:     cancelFunc,
				resultChan: make(chan watch.Event, 64),
				extractor: func(ctx context.Context, pr repository.PackageRevision) (runtime.Object, error) {
					return nil, nil
				},
			}

			r := &nilCheckFakeReader{
				packages:           tt.packages,
				sendEventInBacklog: tt.name == "backlog phase",
			}
			r.Add(1)
			var filter repository.ListPackageRevisionFilter
			go w.listAndWatch(ctx, r, filter)
			r.Wait()

			if tt.waitForStreaming {
				time.Sleep(100 * time.Millisecond)
			}

			if tt.sendEvent {
				pkgRev := &fake.FakePackageRevision{
					PackageRevision: &porchapi.PackageRevision{
						ObjectMeta: metav1.ObjectMeta{
							Labels: make(map[string]string),
						},
					},
				}
				cont := r.callback.OnPackageRevisionChange(watch.Modified, pkgRev)
				if !cont {
					t.Error("Expected callback to return true for nil object")
				}
			} else {
				time.Sleep(10 * time.Millisecond)
			}
		})
	}
}

type nilCheckFakeReader struct {
	sync.WaitGroup
	callback           engine.ObjectWatcher
	packages           []repository.PackageRevision
	sendEventInBacklog bool
}

func (f *nilCheckFakeReader) watchPackages(ctx context.Context, filter repository.ListPackageRevisionFilter, callback engine.ObjectWatcher) error {
	f.callback = callback
	if f.sendEventInBacklog {
		// Send event synchronously in backlog phase
		pkgRev := &fake.FakePackageRevision{
			PackageRevision: &porchapi.PackageRevision{
				ObjectMeta: metav1.ObjectMeta{
					Labels: make(map[string]string),
				},
			},
		}
		callback.OnPackageRevisionChange(watch.Modified, pkgRev)
	}
	f.Done()
	return nil
}

func (f *nilCheckFakeReader) listPackageRevisions(ctx context.Context, filter repository.ListPackageRevisionFilter, callback func(ctx context.Context, p repository.PackageRevision) error) error {
	for _, pkg := range f.packages {
		if err := callback(ctx, pkg); err != nil {
			return err
		}
	}
	return nil
}
