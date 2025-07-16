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

package dbcache

import (
	"sync"
	"testing"
	"time"

	"github.com/nephio-project/porch/pkg/repository"
	"github.com/stretchr/testify/assert"
)

func TestMutexHandlerPkg(t *testing.T) {
	mh := mutexHandler{
		pkgMutexMap: sync.Map{},
		prMutexMap:  sync.Map{},
	}

	pk1 := repository.PackageKey{
		Package: "my-package1",
	}
	pk2 := repository.PackageKey{
		Package: "my-package2",
	}

	// Check for locking on 2 keys in parallel
	startTime := time.Now()

	go holdPkgLock1Second(&mh, pk1)
	go holdPkgLock1Second(&mh, pk2)

	// Give the go routines a chance to start
	time.Sleep(10 * time.Millisecond)

	mh.lockPkg(pk1)
	mh.lockPkg(pk2)
	mh.unlockPkg(pk1)
	mh.unlockPkg(pk2)

	duration := time.Since(startTime).Seconds()
	assert.True(t, duration > 1 && duration < 2)

	// Check for locking on 1 key in series
	startTime = time.Now()

	go holdPkgLock1Second(&mh, pk1)
	go holdPkgLock1Second(&mh, pk1)
	go holdPkgLock1Second(&mh, pk1)
	go holdPkgLock1Second(&mh, pk1)

	// Give the go routines a chance to start
	time.Sleep(10 * time.Millisecond)

	mh.lockPkg(pk1)
	mh.unlockPkg(pk1)

	duration = time.Since(startTime).Seconds()
	assert.True(t, duration > 4 && duration < 5)
}

func TestMutexHandlerPR(t *testing.T) {
	mh := mutexHandler{
		pkgMutexMap: sync.Map{},
		prMutexMap:  sync.Map{},
	}

	prk1 := repository.PackageRevisionKey{
		WorkspaceName: "my-workspace1",
	}
	prk2 := repository.PackageRevisionKey{
		WorkspaceName: "my-workspace2",
	}

	// Check for locking on 2 keys in parallel
	startTime := time.Now()

	go holdPRLock1Second(&mh, prk1)
	go holdPRLock1Second(&mh, prk2)

	// Give the go routines a chance to start
	time.Sleep(10 * time.Millisecond)

	mh.lockPR(prk1)
	mh.lockPR(prk2)
	mh.unlockPR(prk1)
	mh.unlockPR(prk2)

	duration := time.Since(startTime).Seconds()
	assert.True(t, duration > 1 && duration < 2)

	// Check for locking on 1 key in series
	startTime = time.Now()

	go holdPRLock1Second(&mh, prk1)
	go holdPRLock1Second(&mh, prk1)
	go holdPRLock1Second(&mh, prk1)
	go holdPRLock1Second(&mh, prk1)

	// Give the go routines a chance to start
	time.Sleep(10 * time.Millisecond)

	mh.lockPR(prk1)
	mh.unlockPR(prk1)

	duration = time.Since(startTime).Seconds()
	assert.True(t, duration > 4 && duration < 5)
}

func holdPkgLock1Second(mh *mutexHandler, pk repository.PackageKey) {
	mh.lockPkg(pk)

	time.Sleep(1 * time.Second)
	mh.unlockPkg(pk)
}

func holdPRLock1Second(mh *mutexHandler, prk repository.PackageRevisionKey) {
	mh.lockPR(prk)

	time.Sleep(1 * time.Second)
	mh.unlockPR(prk)
}
