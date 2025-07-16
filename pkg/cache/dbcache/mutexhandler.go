// Copyright 2025 The kpt and Nephio Authors
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

package dbcache

import (
	"sync"

	"github.com/nephio-project/porch/pkg/repository"
)

type MutexHandler interface {
	lockPkg(repository.PackageKey)
	unlockPkg(repository.PackageKey)
	lockPR(repository.PackageRevisionKey)
	unlockPR(repository.PackageRevisionKey)
}

var _ MutexHandler = &mutexHandler{}

type mutexHandler struct {
	pkgMutexMap sync.Map
	prMutexMap  sync.Map
}

type mutexEntry struct {
	mutex   sync.Mutex
	waiting uint
}

func (h *mutexHandler) lockPkg(pkgKey repository.PackageKey) {
	syncMapLockAny(&h.pkgMutexMap, pkgKey)
}

func (h *mutexHandler) unlockPkg(pkgKey repository.PackageKey) {
	syncMapUnlockAny(&h.pkgMutexMap, pkgKey)
}

func (h *mutexHandler) lockPR(prKey repository.PackageRevisionKey) {
	syncMapLockAny(&h.prMutexMap, prKey)
}

func (h *mutexHandler) unlockPR(prKey repository.PackageRevisionKey) {
	syncMapUnlockAny(&h.prMutexMap, prKey)
}

func syncMapLockAny(syncMap *sync.Map, anyKey any) {
	mutexAny, _ := syncMap.LoadOrStore(anyKey, &mutexEntry{
		mutex:   sync.Mutex{},
		waiting: 0,
	})
	mutexEntry := mutexAny.(*mutexEntry)
	mutexEntry.waiting++

	mutexEntry.mutex.Lock()
	mutexEntry.waiting--
}

func syncMapUnlockAny(syncMap *sync.Map, anyKey any) {
	mutexAny, exists := syncMap.Load(anyKey)
	if !exists {
		return
	}
	mutexEntry := mutexAny.(*mutexEntry)

	if mutexEntry.waiting == 0 {
		syncMap.Delete(anyKey)
	}
	mutexEntry.mutex.Unlock()
}
