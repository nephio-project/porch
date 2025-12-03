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
	"path/filepath"
	"sync"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDirectoryPool_GetOrCreateSharedRepository(t *testing.T) {
	pool := &DirectoryPool{
		directories: make(map[string]*SharedDirectory),
	}

	tempDir := t.TempDir()
	repoDir := filepath.Join(tempDir, "test-repo")

	// First call should create new shared directory
	shared1, err := pool.GetOrCreateSharedRepository(repoDir, "test-repo")
	require.NoError(t, err)
	require.NotNil(t, shared1)
	assert.Equal(t, 1, shared1.refCount)

	// Second call should reuse existing directory
	shared2, err := pool.GetOrCreateSharedRepository(repoDir, "test-repo")
	require.NoError(t, err)
	assert.Same(t, shared1, shared2)
	assert.Equal(t, 2, shared2.refCount)
}

func TestDirectoryPool_ReleaseSharedRepository(t *testing.T) {
	pool := &DirectoryPool{
		directories: make(map[string]*SharedDirectory),
	}

	tempDir := t.TempDir()
	repoDir := filepath.Join(tempDir, "test-repo")

	// Create shared repository
	shared, err := pool.GetOrCreateSharedRepository(repoDir, "test-repo")
	require.NoError(t, err)

	// Add another reference
	_, err = pool.GetOrCreateSharedRepository(repoDir, "test-repo")
	require.NoError(t, err)

	// First release should decrement refCount
	pool.ReleaseSharedRepository(repoDir, "test-repo")
	assert.Equal(t, 1, shared.refCount)

	// Second release should remove from pool and cleanup directory
	pool.ReleaseSharedRepository(repoDir, "test-repo")
	_, exists := pool.directories[repoDir]
	assert.False(t, exists)
	_, err = os.Stat(repoDir)
	assert.True(t, os.IsNotExist(err))
}

func TestSharedDirectory_WithLock(t *testing.T) {
	tempDir := t.TempDir()
	repo, err := initEmptyRepository(tempDir)
	require.NoError(t, err)

	shared := &SharedDirectory{
		repo:     repo,
		refCount: 1,
	}

	called := false
	err = shared.WithLock(func(r *git.Repository) error {
		called = true
		assert.Same(t, repo, r)
		return nil
	})

	assert.NoError(t, err)
	assert.True(t, called)
}

func TestSharedDirectory_WithRLock(t *testing.T) {
	tempDir := t.TempDir()
	repo, err := initEmptyRepository(tempDir)
	require.NoError(t, err)

	shared := &SharedDirectory{
		repo:     repo,
		refCount: 1,
	}

	called := false
	err = shared.WithRLock(func(r *git.Repository) error {
		called = true
		assert.Same(t, repo, r)
		return nil
	})

	assert.NoError(t, err)
	assert.True(t, called)
}

func TestDirectoryPool_ConcurrentAccess(t *testing.T) {
	pool := &DirectoryPool{
		directories: make(map[string]*SharedDirectory),
	}

	tempDir := t.TempDir()
	repoDir := filepath.Join(tempDir, "concurrent-repo")

	var wg sync.WaitGroup
	numGoroutines := 10

	// Concurrent creation
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			_, err := pool.GetOrCreateSharedRepository(repoDir, "concurrent-repo")
			assert.NoError(t, err)
		}(i)
	}
	wg.Wait()

	// Check final refCount
	pool.mutex.Lock()
	shared := pool.directories[repoDir]
	pool.mutex.Unlock()

	assert.Equal(t, numGoroutines, shared.refCount)

	// Concurrent release
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			pool.ReleaseSharedRepository(repoDir, "concurrent-repo")
		}()
	}
	wg.Wait()

	// Directory should be cleaned up
	_, exists := pool.directories[repoDir]
	assert.False(t, exists)
}