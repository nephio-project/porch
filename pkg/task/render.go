// Copyright 2022, 2024-2025 The kpt and Nephio Authors
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
	"encoding/json"
	"fmt"
	iofs "io/fs"
	"path"
	"strings"

	api "github.com/nephio-project/porch/api/porch/v1alpha1"
	"github.com/nephio-project/porch/internal/kpt/fnruntime"
	"github.com/nephio-project/porch/pkg/kpt"
	fnresult "github.com/nephio-project/porch/pkg/kpt/api/fnresult/v1"
	"github.com/nephio-project/porch/pkg/kpt/fn"
	"github.com/nephio-project/porch/pkg/repository"
	"go.opentelemetry.io/otel/trace"
	"k8s.io/klog/v2"
	"sigs.k8s.io/kustomize/kyaml/filesys"
)

type renderPackageMutation struct {
	draft         repository.PackageRevisionDraft
	runtime       fn.FunctionRuntime
	runnerOptions fnruntime.RunnerOptions
}

var _ mutation = &renderPackageMutation{}

func (m *renderPackageMutation) apply(ctx context.Context, resources repository.PackageResources) (repository.PackageResources, *api.TaskResult, error) {
	ctx, span := tracer.Start(ctx, "renderPackageMutation::apply", trace.WithAttributes())
	defer span.End()

	if isRenderablePackage(resources) {
		resources, err := lockPipelineReadinessGate(ctx, m.draft, resources)
		if err != nil {
			return resources, nil, err
		}
	}

	fs := filesys.MakeFsInMemory()
	taskResult := &api.TaskResult{
		Task: &api.Task{
			Type: "render",
			Eval: &api.FunctionEvalTaskSpec{
				Image:     "render",
				ConfigMap: nil,
			},
		},
		RenderStatus: &api.RenderStatus{},
	}
	pkgPath, err := writeResources(fs, resources)
	if err != nil {
		return repository.PackageResources{}, nil, err
	}

	if pkgPath == "" {
		// We need this for the no-resources case
		// TODO: we should handle this better
		klog.Warningf("skipping render as no package was found")
	} else {
		renderer := kpt.NewRenderer(m.runnerOptions)
		result, renderErr := renderer.Render(ctx, fs, fn.RenderOptions{
			PkgPath: pkgPath,
			Runtime: m.runtime,
		})
		if result != nil {
			var rr api.ResultList
			err := convertResultList(result, &rr)
			if err != nil {
				return repository.PackageResources{}, taskResult, err
			}
			taskResult.RenderStatus.Result = rr
		}
		if renderErr != nil {
			taskResult.RenderStatus.Err = renderErr.Error()
			return repository.PackageResources{}, taskResult, renderErr
		}
	}

	renderedResources, err := readResources(fs)
	if err != nil {
		return repository.PackageResources{}, taskResult, err
	}

	// TODO: There are internal tasks not represented in the API; Update the Apply interface to enable them.
	return renderedResources, taskResult, nil
}

func isRenderablePackage(resources repository.PackageResources) bool {
	kptfile := resources.GetKptfile()
	return !kptfile.Pipeline.IsEmpty()
}

func lockPipelineReadinessGate(ctx context.Context, draft repository.PackageRevisionDraft, resources repository.PackageResources) (repository.PackageResources, error) {
	ctx, span := tracer.Start(ctx, "generictaskhandler.go::pushPipelineReadinessGate", trace.WithAttributes())
	defer span.End()

	repo := draft.GetRepo()

	resources.SetPrStatusCondition(ConditionPipelineNotPassed, true)
	if err := draft.UpdateResources(ctx, &api.PackageRevisionResources{
		Spec: api.PackageRevisionResourcesSpec{
			Resources: resources.Contents,
		},
	}, &api.Task{Type: "lock pipeline readiness gate"}); err != nil {
		return resources, err
	}

	repoPr, err := repo.ClosePackageRevisionDraft(ctx, draft, 0)
	if err != nil {
		return resources, err
	}

	draft, err = repo.UpdatePackageRevision(ctx, repoPr)
	apiResources, err := repoPr.GetResources(ctx)
	if err != nil {
		return resources, fmt.Errorf("cannot get package resources after locking pipeline readiness gate: %w", err)
	}

	resources = repository.PackageResources{
		Contents: apiResources.Spec.Resources,
	}

	return resources, nil
}

// TODO: Implement filesystem abstraction directly rather than on top of PackageResources
func writeResources(fs filesys.FileSystem, resources repository.PackageResources) (string, error) {
	var packageDir string // path to the topmost directory containing Kptfile
	for k, v := range resources.Contents {
		dir := path.Dir(k)
		if dir == "." {
			dir = "/"
		}
		if err := fs.MkdirAll(dir); err != nil {
			return "", err
		}
		base := path.Base(k)
		if err := fs.WriteFile(path.Join(dir, base), []byte(v)); err != nil {
			return "", err
		}
		if base == "Kptfile" {
			// Found Kptfile. Check if the current directory is ancestor of the current
			// topmost package directory. If so, use it instead.
			if packageDir == "" || dir == "/" || strings.HasPrefix(packageDir, dir+"/") {
				packageDir = dir
			}
		}
	}
	// Return topmost directory containing Kptfile
	return packageDir, nil
}

func convertResultList(in *fnresult.ResultList, out *api.ResultList) error {
	if in == nil {
		return nil
	}
	srcBytes, err := json.Marshal(in)
	if err != nil {
		return err
	}

	if err := json.Unmarshal(srcBytes, &out); err != nil {
		return err
	}
	return nil
}

func readResources(fs filesys.FileSystem) (repository.PackageResources, error) {
	contents := map[string]string{}

	if err := fs.Walk("/", func(path string, info iofs.FileInfo, err error) error {
		if info.Mode().IsRegular() {
			data, err := fs.ReadFile(path)
			if err != nil {
				return err
			}
			contents[strings.TrimPrefix(path, "/")] = string(data)
		}
		return nil
	}); err != nil {
		return repository.PackageResources{}, err
	}

	return repository.PackageResources{
		Contents: contents,
	}, nil
}
