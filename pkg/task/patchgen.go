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
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/bluekeyes/go-gitdiff/gitdiff"
	api "github.com/nephio-project/porch/api/porch/v1alpha1"

	"github.com/nephio-project/porch/pkg/repository"
	"go.opentelemetry.io/otel/trace"
	"k8s.io/klog/v2"

	"github.com/hexops/gotextdiff"
	"github.com/hexops/gotextdiff/myers"
	"github.com/hexops/gotextdiff/span"
)

// GeneratePatch returns patch operations for transforming from oldV to newV.
func GeneratePatch(fileName string, oldV, newV string) (api.PatchSpec, error) {
	edits := myers.ComputeEdits(span.URIFromPath(fileName), oldV, newV)
	diff := fmt.Sprint(gotextdiff.ToUnified(fileName, fileName, oldV, edits))

	patchSpec := api.PatchSpec{
		File:      fileName,
		PatchType: api.PatchTypePatchFile,
		Contents:  diff,
	}

	return patchSpec, nil
}

type applyPatchMutation struct {
	patchTask     *api.PackagePatchTaskSpec
	task          *api.Task
	cloneStrategy api.PackageMergeStrategy
}

var _ mutation = &applyPatchMutation{}

func (m *applyPatchMutation) apply(ctx context.Context, resources repository.PackageResources) (repository.PackageResources, *api.TaskResult, error) {
	_, span := tracer.Start(ctx, "applyPatchMutation::apply", trace.WithAttributes())
	defer span.End()

	result := repository.PackageResources{
		Contents: map[string]string{},
	}

	for k, v := range resources.Contents {
		result.Contents[k] = v
	}

	for _, patchSpec := range m.patchTask.Patches {
		switch patchSpec.PatchType {

		case api.PatchTypeCreateFile:
			if _, found := result.Contents[patchSpec.File]; found {
				// TODO: We should be able to tolerate this.  Either do a merge or create as a different filename "-2"
				return result, nil, fmt.Errorf("patch wants to create file %q but already exists", patchSpec.File)
			}
			result.Contents[patchSpec.File] = patchSpec.Contents

		case api.PatchTypeDeleteFile:
			if _, found := result.Contents[patchSpec.File]; !found {
				// TODO: I don't think this should be an error, but maybe we should use object manipulation more than file manipulation.
				// TODO: Support object based patches where we can.
				klog.Warningf("patch wants to delete file %q, but already deleted", patchSpec.File)
			}
			delete(result.Contents, patchSpec.File)

		case api.PatchTypePatchFile:
			oldContents, found := result.Contents[patchSpec.File]
			if !found {
				return result, nil, fmt.Errorf("patch specifies file %q which does not exist", patchSpec.File)
			}
			if skipPatchMutation(ctx, *m) {
				klog.Infof("Skipping patch for file %q, returning original content", patchSpec.File)
				result.Contents[patchSpec.File] = oldContents
				continue
			}
			files, preamble, err := gitdiff.Parse(strings.NewReader(patchSpec.Contents))
			if err != nil {
				return result, nil, fmt.Errorf("error parsing patch: %w", err)
			}

			if len(files) == 0 {
				return result, nil, fmt.Errorf("patch did not specify any files")
			}
			if len(files) > 1 {
				return result, nil, fmt.Errorf("patch specified multiple files")
			}
			if preamble != "" {
				return result, nil, fmt.Errorf("patch had unexpected preamble %q", preamble)
			}

			file := files[0]
			if file.OldName != patchSpec.File {
				return result, nil, fmt.Errorf("patch contained unexpected name; got %q, want %q", file.OldName, patchSpec.File)
			}

			if file.IsBinary {
				return result, nil, fmt.Errorf("patch was a binary diff; expected text diff")
			}
			if file.IsCopy || file.IsDelete || file.IsNew || file.IsRename {
				return result, nil, fmt.Errorf("patch was of an unexpected type (copy/delete/new/rename)")
			}
			if file.OldMode != file.NewMode {
				return result, nil, fmt.Errorf("patch contained file mode change")
			}

			var output bytes.Buffer
			if err = gitdiff.Apply(&output, strings.NewReader(oldContents), file); err != nil {
				return result, nil, fmt.Errorf("error applying patch %q to file %q: %w", patchSpec.Contents, oldContents, err)
			}
			patched := output.String()
			result.Contents[patchSpec.File] = patched

		default:
			return result, nil, fmt.Errorf("unhandled patch type %q", patchSpec.PatchType)
		}
	}

	return result, &api.TaskResult{Task: m.task}, nil
}

func buildPatchMutation(_ context.Context, task *api.Task, cloneStrategy api.PackageMergeStrategy) (mutation, error) {
	if task.Patch == nil {
		return nil, fmt.Errorf("patch not set for task of type %q", task.Type)
	}

	m := &applyPatchMutation{
		patchTask:     task.Patch,
		task:          task,
		cloneStrategy: cloneStrategy,
	}
	return m, nil
}

func skipPatchMutation(_ context.Context, m applyPatchMutation) bool {
	if m.cloneStrategy == api.CopyMerge || m.cloneStrategy == api.ForceDeleteReplace {
		klog.Infof("clone strategy is %v, patching skipped on this type of clone strategy", m.cloneStrategy)
		return true
	}
	return false
}
