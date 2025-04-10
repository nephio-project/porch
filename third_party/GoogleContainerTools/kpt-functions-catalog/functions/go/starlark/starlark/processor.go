/*
 Copyright 2025 The Nephio Authors.

 Licensed under the Apache License, Version 2.0 (the "License");
 You may not use this file except in compliance with the License.
 You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

 Unless required by applicable law or agreed to in writing, software
 distributed under the License is distributed on an "AS IS" BASIS,
 WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 See the License for the specific language governing permissions and
 limitations under the License.
*/

package cache

import (
	"context"
	"testing"

	cachetypes "github.com/nephio-project/porch/pkg/cache/types"
	"github.com/stretchr/testify/assert"
)

func TestCreateCacheImpl(t *testing.T) {
	cacheOptions := cachetypes.CacheOptions{
		CacheType: cachetypes.CRCacheType,
	}

	cache, err := CreateCacheImpl(context.TODO(), cacheOptions)
	assert.True(t, err == nil)
	assert.Equal(t, 0, len(cache.GetRepositories(context.TODO())))

	cacheOptions.CacheType = "BAD-CACHE"
	_, err = CreateCacheImpl(context.TODO(), cacheOptions)
	assert.True(t, err != nil)
}
