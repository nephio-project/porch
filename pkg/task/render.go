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
	"encoding/json"
	"fmt"
	iofs "io/fs"
	"path"
	"strings"

	api "github.com/nephio-project/porch/api/porch/v1alpha1"
	fnsdk "github.com/nephio-project/porch/third_party/GoogleContainerTools/kpt-functions-sdk/go/fn"

	"github.com/nephio-project/porch/internal/kpt/fnruntime"
	"github.com/nephio-project/porch/pkg/kpt"
	fnresult "github.com/nephio-project/porch/pkg/kpt/api/fnresult/v1"
	kptfileapi "github.com/nephio-project/porch/pkg/kpt/api/kptfile/v1"
	"github.com/nephio-project/porch/pkg/kpt/fn"
	"github.com/nephio-project/porch/pkg/repository"
	"go.opentelemetry.io/otel/trace"
	"k8s.io/klog/v2"
	"sigs.k8s.io/kustomize/kyaml/filesys"
)

type renderPackageMutation struct {
	runtime       fn.FunctionRuntime
	runnerOptions fnruntime.RunnerOptions
}

var _ mutation = &renderPackageMutation{}

func (m *renderPackageMutation) apply(ctx context.Context, resources repository.PackageResources) (repository.PackageResources, *api.TaskResult, error) {
	ctx, span := tracer.Start(ctx, "renderPackageMutation::apply", trace.WithAttributes())
	defer span.End()

	fs := filesys.MakeFsInMemory()
	taskResult := &api.TaskResult{
		Task: &api.Task{
			Type: api.TaskTypeEval,
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
		return resources, taskResult, nil
	}

	renderer := kpt.NewRenderer(m.runnerOptions)
	result, renderErr := renderer.Render(ctx, fs, fn.RenderOptions{
		PkgPath: pkgPath,
		Runtime: m.runtime,
	})
	if result != nil {
		var rr api.ResultList
		err := convertResultList(result, &rr)
		if err != nil {
			taskResult.RenderStatus.Err = err.Error()
			return repository.PackageResources{}, taskResult, err
		}
		taskResult.RenderStatus.Result = rr
	}
	if renderErr != nil {
		taskResult.RenderStatus.Err = renderErr.Error()
		// a render failure is considered normal operation, so do not return an error here
	}

	renderedResources, err := readResources(fs)
	if err != nil {
		return repository.PackageResources{}, taskResult, err
	}

	err = addRenderStatusConditionToKptfile(&renderedResources, taskResult.RenderStatus)
	if err != nil {
		return repository.PackageResources{}, taskResult, err
	}
	return renderedResources, taskResult, nil
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
		if base == kptfileapi.KptFileName {
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

// addRenderStatusConditionToKptfile sets the render condition in the Kptfile's status
func addRenderStatusConditionToKptfile(resources *repository.PackageResources, status *api.RenderStatus) error {
	kptfile, err := fnsdk.NewKptfileFromPackage(resources.Contents)
	if err != nil {
		return fmt.Errorf("failed to read Kptfile, skipping render condition setup: %w", err)
	}

	success := status.Err == ""
	if success {
		for _, result := range status.Result.Items {
			if result.ExitCode != 0 {
				success = false
				break
			}
			for _, fnResult := range result.Results {
				if fnResult.Severity == string(fnsdk.Error) {
					success = false
					break
				}
			}
		}
	}

	renderCondition := kptfileapi.Condition{
		Type: kptfileapi.RenderConditionType,
	}
	if success {
		renderCondition.Status = kptfileapi.ConditionTrue
		renderCondition.Reason = "Rendered"
		renderCondition.Message = "Rendered successfully"
	} else {
		renderCondition.Status = kptfileapi.ConditionFalse
		renderCondition.Reason = "RenderFailed"
		renderStatusJson, _ := json.Marshal(status)
		renderCondition.Message = status.Err + string(renderStatusJson)
	}
	err = kptfile.SetTypedCondition(renderCondition)
	if err != nil {
		return fmt.Errorf("failed to set render condition in Kptfile: %w", err)
	}

	return kptfile.WriteToPackage(resources.Contents)
}
