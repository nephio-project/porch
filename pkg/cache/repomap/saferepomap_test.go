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

package repomap

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/nephio-project/porch/pkg/repository"
	mockrepository "github.com/nephio-project/porch/test/mockery/mocks/porch/pkg/repository"
)

func TestLoadOrCreate_Success(t *testing.T) {
	m := &SafeRepoMap{}
	key := repository.RepositoryKey{Name: "test-repo"}

	repo, err := m.LoadOrCreate(key, func() (repository.Repository, error) {
		mock := &mockrepository.MockRepository{}
		mock.On("KubeObjectName").Return("created")
		return mock, nil
	})

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if repo == nil {
		t.Fatal("expected repo, got nil")
	}
}

func TestLoadOrCreate_Reuse(t *testing.T) {
	m := &SafeRepoMap{}
	key := repository.RepositoryKey{Name: "test-repo"}

	var callCount int32
	create := func() (repository.Repository, error) {
		atomic.AddInt32(&callCount, 1)
		return &mockrepository.MockRepository{}, nil
	}

	repo1, err1 := m.LoadOrCreate(key, create)
	repo2, err2 := m.LoadOrCreate(key, create)

	if err1 != nil || err2 != nil {
		t.Fatalf("expected no errors, got %v, %v", err1, err2)
	}
	if repo1 != repo2 {
		t.Error("expected same repo instance")
	}
	if atomic.LoadInt32(&callCount) != 1 {
		t.Errorf("expected create called once, got %d", callCount)
	}
}

func TestLoadOrCreate_ErrorRetry(t *testing.T) {
	m := &SafeRepoMap{}
	key := repository.RepositoryKey{Name: "test-repo"}

	var callCount int32
	create := func() (repository.Repository, error) {
		count := atomic.AddInt32(&callCount, 1)
		if count == 1 {
			return nil, errors.New("first attempt failed")
		}
		return &mockrepository.MockRepository{}, nil
	}

	// First call should fail
	repo1, err1 := m.LoadOrCreate(key, create)
	if err1 == nil {
		t.Fatal("expected error on first call")
	}
	if repo1 != nil {
		t.Error("expected nil repo on error")
	}

	// Second call should succeed (retry)
	repo2, err2 := m.LoadOrCreate(key, create)
	if err2 != nil {
		t.Fatalf("expected success on retry, got %v", err2)
	}
	if repo2 == nil {
		t.Fatal("expected repo on retry")
	}
	if atomic.LoadInt32(&callCount) != 2 {
		t.Errorf("expected create called twice, got %d", callCount)
	}
}

func TestLoadOrCreate_Concurrent(t *testing.T) {
	m := &SafeRepoMap{}
	key := repository.RepositoryKey{Name: "test-repo"}

	var callCount int32
	create := func() (repository.Repository, error) {
		atomic.AddInt32(&callCount, 1)
		return &mockrepository.MockRepository{}, nil
	}

	const goroutines = 10
	var wg sync.WaitGroup
	results := make([]repository.Repository, goroutines)

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			repo, err := m.LoadOrCreate(key, create)
			if err != nil {
				t.Errorf("goroutine %d got error: %v", idx, err)
			}
			results[idx] = repo
		}(i)
	}

	wg.Wait()

	// All goroutines should get the same instance
	for i := 1; i < goroutines; i++ {
		if results[i] != results[0] {
			t.Errorf("goroutine %d got different instance", i)
		}
	}

	// Create should only be called once
	if atomic.LoadInt32(&callCount) != 1 {
		t.Errorf("expected create called once, got %d", callCount)
	}
}

func TestLoadOrCreate_ConcurrentError(t *testing.T) {
	m := &SafeRepoMap{}
	key := repository.RepositoryKey{Name: "test-repo"}

	var callCount int32
	create := func() (repository.Repository, error) {
		atomic.AddInt32(&callCount, 1)
		return nil, errors.New("always fails")
	}

	const goroutines = 5
	var wg sync.WaitGroup

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := m.LoadOrCreate(key, create)
			if err == nil {
				t.Error("expected error")
			}
		}()
	}

	wg.Wait()

	// With concurrent errors and deletion, multiple goroutines may call create
	// This is expected behavior - failed entries are deleted and retried
	count := atomic.LoadInt32(&callCount)
	if count < 1 || count > goroutines {
		t.Errorf("expected create called between 1 and %d times, got %d", goroutines, count)
	}
}

func TestLoad(t *testing.T) {
	m := &SafeRepoMap{}
	key := repository.RepositoryKey{Name: "test-repo"}

	// Load non-existent key
	repo, ok := m.Load(key)
	if ok {
		t.Error("expected Load to return false for non-existent key")
	}
	if repo != nil {
		t.Error("expected nil repo for non-existent key")
	}

	// Create a repo
	m.LoadOrCreate(key, func() (repository.Repository, error) {
		return &mockrepository.MockRepository{}, nil
	})

	// Load existing key
	repo, ok = m.Load(key)
	if !ok {
		t.Error("expected Load to return true for existing key")
	}
	if repo == nil {
		t.Error("expected non-nil repo for existing key")
	}
}

func TestLoadAndDelete(t *testing.T) {
	m := &SafeRepoMap{}
	key := repository.RepositoryKey{Name: "test-repo"}

	// LoadAndDelete non-existent key
	repo, ok := m.LoadAndDelete(key)
	if ok {
		t.Error("expected LoadAndDelete to return false for non-existent key")
	}
	if repo != nil {
		t.Error("expected nil repo for non-existent key")
	}

	// Create a repo
	m.LoadOrCreate(key, func() (repository.Repository, error) {
		return &mockrepository.MockRepository{}, nil
	})

	// LoadAndDelete existing key
	repo, ok = m.LoadAndDelete(key)
	if !ok {
		t.Error("expected LoadAndDelete to return true for existing key")
	}
	if repo == nil {
		t.Error("expected non-nil repo for existing key")
	}

	// Verify it's deleted
	repo, ok = m.Load(key)
	if ok {
		t.Error("expected key to be deleted")
	}
}

func TestRange(t *testing.T) {
	m := &SafeRepoMap{}

	// Create multiple repos
	keys := []repository.RepositoryKey{
		{Name: "repo1"},
		{Name: "repo2"},
		{Name: "repo3"},
	}

	for _, key := range keys {
		m.LoadOrCreate(key, func() (repository.Repository, error) {
			return &mockrepository.MockRepository{}, nil
		})
	}

	// Range over all entries
	var count int
	m.Range(func(key, value any) bool {
		count++
		return true
	})

	if count != len(keys) {
		t.Errorf("expected Range to iterate over %d entries, got %d", len(keys), count)
	}

	// Test early termination
	count = 0
	m.Range(func(key, value any) bool {
		count++
		return count < 2 // Stop after 2 iterations
	})

	if count != 2 {
		t.Errorf("expected Range to stop after 2 iterations, got %d", count)
	}
}
