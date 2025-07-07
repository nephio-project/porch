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
	"context"

	cachetypes "github.com/nephio-project/porch/pkg/cache/types"
	"github.com/nephio-project/porch/pkg/repository"
	"go.opentelemetry.io/otel/trace"
)

var _ cachetypes.CacheFactory = &DBCacheFactory{}

type DBCacheFactory struct {
}

func (f *DBCacheFactory) NewCache(ctx context.Context, options cachetypes.CacheOptions) (cachetypes.Cache, error) {
	_, span := tracer.Start(ctx, "DbCacheFactory::NewCache", trace.WithAttributes())
	defer span.End()

	if err := OpenDB(ctx, options); err != nil {
		return nil, err
	}

	return &dbCache{
		repositories: make(map[repository.RepositoryKey]*dbRepository),
		options:      options,
	}, nil
}
