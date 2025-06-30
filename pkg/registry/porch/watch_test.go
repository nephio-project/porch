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

	"github.com/nephio-project/porch/api/porch/v1alpha1"
	"github.com/nephio-project/porch/pkg/engine"
	"github.com/nephio-project/porch/pkg/externalrepo/fake"
	"github.com/nephio-project/porch/pkg/repository"
	metainternalversion "k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
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
	options := &metainternalversion.ListOptions{}

	go w.listAndWatch(ctx, r, filter, options.LabelSelector)

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
					PackageRevision: &v1alpha1.PackageRevision{
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

func (f *fakePackageReader) listPackageRevisions(ctx context.Context, filter repository.ListPackageRevisionFilter, selector labels.Selector, callback func(ctx context.Context, p repository.PackageRevision) error) error {
	return nil
}
