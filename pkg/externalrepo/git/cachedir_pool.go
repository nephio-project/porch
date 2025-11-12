// Copyright 2025 The Nephio Authors
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

package git

import (
	"os"
	"sync"

	"github.com/go-git/go-git/v5"
	"k8s.io/klog/v2"
)

// DirectoryPool manages shared access to git directories
type DirectoryPool struct {
	directories map[string]*SharedDirectory
	mutex       sync.Mutex
}

type SharedDirectory struct {
	repo     *git.Repository
	mutex    sync.RWMutex
	refCount int
}

var globalDirectoryPool = &DirectoryPool{
	directories: make(map[string]*SharedDirectory),
}

// GetSharedRepository returns a thread-safe shared repository for the given directory
func (p *DirectoryPool) GetSharedRepository(dir string, repo *git.Repository) *SharedDirectory {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if shared, exists := p.directories[dir]; exists {
		shared.refCount++
		klog.Infof("Reusing shared directory %s, refCount now: %d", dir, shared.refCount)
		return shared
	}

	shared := &SharedDirectory{
		repo:     repo,
		refCount: 1,
	}
	p.directories[dir] = shared
	klog.Infof("Created new shared directory %s, refCount: %d", dir, shared.refCount)
	return shared
}

// ReleaseSharedRepository decrements reference count and cleans up if needed
func (p *DirectoryPool) ReleaseSharedRepository(dir string) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if shared, exists := p.directories[dir]; exists {
		shared.refCount--
		klog.Infof("Released reference to %s, refCount now: %d", dir, shared.refCount)
		if shared.refCount <= 0 {
			delete(p.directories, dir)
			klog.Infof("Cleaning up cached directory %s (refCount reached 0)", dir)
			if err := os.RemoveAll(dir); err != nil {
				klog.Errorf("Failed to remove cached directory %s: %v", dir, err)
			} else {
				klog.V(2).Infof("Successfully removed cached directory %s", dir)
			}
		}
	} else {
		// Directory not in pool, but try to clean up anyway in case of orphaned directories
		if _, err := os.Stat(dir); err == nil {
			if err := os.RemoveAll(dir); err == nil {
				klog.V(2).Infof("Cleaned up orphaned directory %s", dir)
			}
		}
	}
}

// WithLock executes function with exclusive access to the repository
func (s *SharedDirectory) WithLock(fn func(*git.Repository) error) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return fn(s.repo)
}
