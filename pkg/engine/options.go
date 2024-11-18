// Copyright 2022, 2024 The kpt and Nephio Authors
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

package engine

import (
	"fmt"

	"github.com/nephio-project/porch/internal/kpt/fnruntime"
	"github.com/nephio-project/porch/pkg/cache"
	"github.com/nephio-project/porch/pkg/kpt"
	"github.com/nephio-project/porch/pkg/kpt/fn"
	"github.com/nephio-project/porch/pkg/meta"
	"github.com/nephio-project/porch/pkg/repository"
)

type EngineOption interface {
	apply(engine *cadEngine) error
}

type EngineOptionFunc func(engine *cadEngine) error

var _ EngineOption = EngineOptionFunc(nil)

func (f EngineOptionFunc) apply(engine *cadEngine) error {
	engine.taskHandler.SetRepoOpener(engine)
	return f(engine)
}

func WithCache(cache cache.Cache) EngineOption {
	return EngineOptionFunc(func(engine *cadEngine) error {
		engine.cache = cache
		return nil
	})
}

func WithBuiltinFunctionRuntime() EngineOption {
	return EngineOptionFunc(func(engine *cadEngine) error {
		runtime := newBuiltinRuntime()
		if engine.taskHandler.GetRuntime() == nil {
			engine.taskHandler.SetRuntime(runtime)
		} else if mr, ok := engine.taskHandler.GetRuntime().(*fn.MultiRuntime); ok {
			mr.Add(runtime)
		} else {
			engine.taskHandler.SetRuntime(fn.NewMultiRuntime([]fn.FunctionRuntime{engine.taskHandler.GetRuntime(), runtime}))
		}
		return nil
	})
}

func WithGRPCFunctionRuntime(address string, maxGrpcMessageSize int) EngineOption {
	return EngineOptionFunc(func(engine *cadEngine) error {
		runtime, err := newGRPCFunctionRuntime(address, maxGrpcMessageSize)
		if err != nil {
			return fmt.Errorf("failed to create function runtime: %w", err)
		}
		if engine.taskHandler.GetRuntime() == nil {
			engine.taskHandler.SetRuntime(runtime)
		} else if mr, ok := engine.taskHandler.GetRuntime().(*fn.MultiRuntime); ok {
			mr.Add(runtime)
		} else {
			engine.taskHandler.SetRuntime(fn.NewMultiRuntime([]fn.FunctionRuntime{engine.taskHandler.GetRuntime(), runtime}))
		}
		return nil
	})
}

func WithFunctionRuntime(runtime fn.FunctionRuntime) EngineOption {
	return EngineOptionFunc(func(engine *cadEngine) error {
		engine.taskHandler.SetRuntime(runtime)
		return nil
	})
}

func WithSimpleFunctionRuntime() EngineOption {
	return EngineOptionFunc(func(engine *cadEngine) error {
		engine.taskHandler.SetRuntime(kpt.NewSimpleFunctionRuntime())
		return nil
	})
}

func WithRunnerOptions(options fnruntime.RunnerOptions) EngineOption {
	return WithRunnerOptionsResolver(func(namespace string) fnruntime.RunnerOptions { return options })
}

func WithRunnerOptionsResolver(fn func(namespace string) fnruntime.RunnerOptions) EngineOption {
	return EngineOptionFunc(func(engine *cadEngine) error {
		engine.taskHandler.SetRunnerOptionsResolver(fn)
		return nil
	})
}

func WithCredentialResolver(resolver repository.CredentialResolver) EngineOption {
	return EngineOptionFunc(func(engine *cadEngine) error {
		engine.taskHandler.SetCredentialResolver(resolver)
		return nil
	})
}

func WithReferenceResolver(resolver repository.ReferenceResolver) EngineOption {
	return EngineOptionFunc(func(engine *cadEngine) error {
		engine.taskHandler.SetReferenceResolver(resolver)
		return nil
	})
}

func WithUserInfoProvider(provider repository.UserInfoProvider) EngineOption {
	return EngineOptionFunc(func(engine *cadEngine) error {
		engine.userInfoProvider = provider
		return nil
	})
}

func WithMetadataStore(metadataStore meta.MetadataStore) EngineOption {
	return EngineOptionFunc(func(engine *cadEngine) error {
		engine.metadataStore = metadataStore
		return nil
	})
}

func WithWatcherManager(watcherManager *watcherManager) EngineOption {
	return EngineOptionFunc(func(engine *cadEngine) error {
		engine.watcherManager = watcherManager
		return nil
	})
}
