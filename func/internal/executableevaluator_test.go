// Copyright 2022,2026 The kpt and Nephio Authors
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

package internal

import (
	"context"
	"fmt"
	"testing"

	kptfilev1 "github.com/kptdev/kpt/pkg/api/kptfile/v1"
	"github.com/kptdev/kpt/pkg/fn"
	pb "github.com/nephio-project/porch/func/evaluator"
	"github.com/stretchr/testify/assert"
)

func TestNewExecutableEvaluator(t *testing.T) {
	t.Run("failed to read configuration file", func(t *testing.T) {
		executableEvaluatorOptions := ExecutableEvaluatorOptions{
			ConfigFileName:   "testdata/executable_evaluator_config.yaml",
			FunctionCacheDir: "/tmp/func_cache",
		}

		_, err := NewExecutableEvaluator(executableEvaluatorOptions)
		assert.ErrorContains(t, err, "failed to read configuration file")
	})
	t.Run("failed to parse configuration file", func(t *testing.T) {
		executableEvaluatorOptions := ExecutableEvaluatorOptions{
			ConfigFileName:   "testdata/config_bad_format.yaml",
			FunctionCacheDir: "/tmp/func_cache",
		}

		_, err := NewExecutableEvaluator(executableEvaluatorOptions)
		assert.ErrorContains(t, err, "failed to parse configuration file")
	})
	t.Run("no errors", func(t *testing.T) {
		executableEvaluatorOptions := ExecutableEvaluatorOptions{
			ConfigFileName:   "testdata/config.yaml",
			FunctionCacheDir: "/tmp/func_cache",
		}

		_, err := NewExecutableEvaluator(executableEvaluatorOptions)
		assert.NoError(t, err)
	})
}

func TestEvaluateExecutableFunction(t *testing.T) {
	t.Run("invalid semver constraint will cause function not found error", func(t *testing.T) {
		ctx := context.Background()
		executableEvaluatorOptions := ExecutableEvaluatorOptions{
			ConfigFileName:   "testdata/config.yaml",
			FunctionCacheDir: "/tmp/func_cache",
		}

		req := &pb.EvaluateFunctionRequest{
			ResourceList: []byte("req-rl"),
			Image:        "ghcr.io/kptdev/krm-functions-catalog/test-image:v0.1",
			Tag:          ">> 1.2.3 < 1.3.0", // Invalid semver constraint, '>>' is not a valid operator
		}

		evaluator, _ := NewExecutableEvaluator(executableEvaluatorOptions)
		resp, err := evaluator.EvaluateFunction(ctx, req)

		assert.Nil(t, resp)
		assert.Equal(t, &fn.NotFoundError{
			Function: kptfilev1.Function{Image: req.Image},
		}, err)
	})
	t.Run("function not found", func(t *testing.T) {
		ctx := context.Background()
		executableEvaluatorOptions := ExecutableEvaluatorOptions{
			ConfigFileName:   "testdata/config.yaml",
			FunctionCacheDir: "/tmp/func_cache",
		}

		req := &pb.EvaluateFunctionRequest{
			ResourceList: []byte("req-rl"),
			Image:        "ghcr.io/kptdev/krm-functions-catalog/test-image:v0.1",
			Tag:          "> 1.2.3 < 1.3.0", // This is a valid semver constraint syntax
		}

		evaluator, _ := NewExecutableEvaluator(executableEvaluatorOptions)
		_, err := evaluator.EvaluateFunction(ctx, req)
		assert.Equal(t, fmt.Sprintf("function \"%s\" not found", req.Image), err.Error())
	})
	t.Run("failed to execute function", func(t *testing.T) {
		ctx := context.Background()
		executableEvaluatorOptions := ExecutableEvaluatorOptions{
			ConfigFileName:   "testdata/config.yaml",
			FunctionCacheDir: "/tmp/func_cache",
		}

		req := &pb.EvaluateFunctionRequest{
			ResourceList: []byte("req-rl"),
			Image:        "ghcr.io/kptdev/krm-functions-catalog/set-image:v0.1.2",
			Tag:          "> 1.2.3 < 1.3.0",
		}

		evaluator, _ := NewExecutableEvaluator(executableEvaluatorOptions)
		_, err := evaluator.EvaluateFunction(ctx, req)
		assert.ErrorContains(t, err, fmt.Sprintf("Failed to execute function \"%s\":", req.Image))
	})
}
