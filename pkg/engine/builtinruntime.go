// Copyright 2022 The kpt and Nephio Authors
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
	"io"

	fnsdk "github.com/GoogleContainerTools/kpt-functions-sdk/go/fn"
	"github.com/nephio-project/porch/pkg/kpt"
	v1 "github.com/nephio-project/porch/pkg/kpt/api/kptfile/v1"
	"github.com/nephio-project/porch/pkg/kpt/fn"
)

// When updating the version for the builtin functions, please also update the image version
// in test TestBuiltinFunctionEvaluator in porch/test/e2e/e2e_test.go, if the versions mismatch
// the e2e test will fail in local deployment mode.
var ()

type builtinRuntime struct {
	fnMapping map[string]fnsdk.ResourceListProcessor
}

func newBuiltinRuntime() *builtinRuntime {
	fnMap := map[string]fnsdk.ResourceListProcessor{}
	return &builtinRuntime{
		fnMapping: fnMap,
	}
}

var _ kpt.FunctionRuntime = &builtinRuntime{}

func (br *builtinRuntime) GetRunner(ctx context.Context, funct *v1.Function) (fn.FunctionRunner, error) {
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

func (br *builtinRunner) Run(r io.Reader, w io.Writer) error {
	return fnsdk.Execute(br.processor, r, w)
}
