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

package task

import (
	"context"
	"fmt"

	api "github.com/nephio-project/porch/api/porch/v1alpha1"
	"github.com/nephio-project/porch/internal/kpt/builtins"
	"github.com/nephio-project/porch/internal/kpt/fnruntime"
	"github.com/nephio-project/porch/pkg/kpt/fn"
	"github.com/nephio-project/porch/pkg/repository"
	"sigs.k8s.io/kustomize/kyaml/fn/runtime/runtimeutil"
	"sigs.k8s.io/kustomize/kyaml/kio"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

type builtinEvalMutation struct {
	function string
	runner   fn.FunctionRunner
}

func newPackageContextGeneratorMutation(packageConfig *builtins.PackageConfig) (mutation, error) {
	runner := &builtins.PackageContextGenerator{
		PackageConfig: packageConfig,
	}

	return &builtinEvalMutation{
		function: fnruntime.FuncGenPkgContext,
		runner:   runner,
	}, nil
}

var _ mutation = &builtinEvalMutation{}

func (m *builtinEvalMutation) apply(ctx context.Context, resources repository.PackageResources) (repository.PackageResources, *api.TaskResult, error) {
	ff := &runtimeutil.FunctionFilter{
		Run:     m.runner.Run,
		Results: &yaml.RNode{},
	}

	pr := &packageReader{
		input: resources,
		extra: map[string]string{},
	}

	result := repository.PackageResources{
		Contents: map[string]string{},
	}

	pipeline := kio.Pipeline{
		Inputs:  []kio.Reader{pr},
		Filters: []kio.Filter{ff},
		Outputs: []kio.Writer{&packageWriter{
			output: result,
		}},
	}

	if err := pipeline.Execute(); err != nil {
		return repository.PackageResources{}, nil, fmt.Errorf("failed to evaluate function %q: %w", m.function, err)
	}

	for k, v := range pr.extra {
		result.Contents[k] = v
	}

	return result, &api.TaskResult{}, nil
}
