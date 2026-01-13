// Copyright 2022, 2025-2026 The kpt and Nephio Authors
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
	"context"
	"fmt"
	"io"

	kptfilev1 "github.com/kptdev/kpt/pkg/api/kptfile/v1"
	fnsdk "github.com/kptdev/krm-functions-sdk/go/fn"
	"github.com/nephio-project/porch/internal/kpt/fnruntime"
	"github.com/nephio-project/porch/pkg/kpt"
	"github.com/nephio-project/porch/pkg/kpt/fn"
	"github.com/nephio-project/porch/third_party/kptdev/krm-functions-catalog/functions/go/apply_replacements"
	"github.com/nephio-project/porch/third_party/kptdev/krm-functions-catalog/functions/go/set_namespace"
	"github.com/nephio-project/porch/third_party/kptdev/krm-functions-catalog/functions/go/starlark/starlark"
)

// When updating the version for the builtin functions, please also update the image version
// in test TestBuiltinFunctionEvaluator in porch/test/e2e/api/fn_runner_test.go, if the versions mismatch
// the e2e test will fail in local deployment mode.
var (
	applyReplacementsImageAliases = []string{
		"apply-replacements:v0.1.1",
		"apply-replacements:v0.1",
		"apply-replacements@sha256:85913d4ec8db62053eb060ff1b7e26d13ff8853b75cae4d0461b8a1c7ddd4947",
	}
	setNamespaceImageAliases = []string{
		"set-namespace:v0.4.1",
		"set-namespace:v0.4",
		"set-namespace@sha256:f930d9248001fa763799cc81cf2d89bbf83954fc65de0db20ab038a21784f323",
	}
	starlarkImageAliases = []string{
		"starlark:v0.4.3",
		"starlark:v0.4",
		"starlark@sha256:6ba3971c64abcd6c3d93039d45721bb5ab496c7fbbc9ac1e685b11577f368ce0",
	}
)

type builtinRuntime struct {
	fnMapping map[string]fnsdk.ResourceListProcessor
}

func newBuiltinRuntime(imagePrefix string) *builtinRuntime {
	fnMap := map[string]fnsdk.ResourceListProcessor{}

	applyMappings := func(aliases []string, fn fnsdk.ResourceListProcessorFunc) {
		for _, img := range aliases {
			fnMap[img] = fn
			fnMap[fnruntime.GHCRImagePrefix+img] = fn
			if imagePrefix != "" && imagePrefix != fnruntime.GHCRImagePrefix {
				fnMap[imagePrefix+"/"+img] = fn
			}
		}
	}

	applyMappings(applyReplacementsImageAliases, apply_replacements.ApplyReplacements)
	applyMappings(setNamespaceImageAliases, set_namespace.Run)
	applyMappings(starlarkImageAliases, starlark.Process)

	return &builtinRuntime{
		fnMapping: fnMap,
	}
}

var _ kpt.FunctionRuntime = &builtinRuntime{}

func (br *builtinRuntime) GetRunner(ctx context.Context, funct *kptfilev1.Function) (fn.FunctionRunner, error) {
	processor, found := br.fnMapping[funct.Image]
	if !found {
		return nil, &fn.NotFoundError{Function: *funct}
	}

	return &builtinRunner{
		ctx:       ctx,
		processor: processor,
	}, nil
}

func (br *builtinRuntime) Close() error {
	return nil
}

type builtinRunner struct {
	ctx       context.Context
	processor fnsdk.ResourceListProcessor
}

var _ fn.FunctionRunner = &builtinRunner{}

func (br *builtinRunner) Run(r io.Reader, w io.Writer) (err error) {
	// KRM functions often panic on input validation errors, so we need to convert panics to errors
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("KRM function panicked with: %v", p)
		}
	}()
	return fnsdk.Execute(br.processor, r, w)
}
