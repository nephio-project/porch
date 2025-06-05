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

package task

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	api "github.com/nephio-project/porch/api/porch/v1alpha1"
	configapi "github.com/nephio-project/porch/api/porchconfig/v1alpha1"
	"sigs.k8s.io/kustomize/kyaml/yaml"

	"github.com/nephio-project/porch/internal/kpt/builtins"
	"github.com/nephio-project/porch/internal/kpt/fnruntime"
	"github.com/nephio-project/porch/pkg/repository"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// type mockPackageRevisionDraft struct{}

type mockPackageRevisionDraft struct {
	updateResourcesFn func(ctx context.Context, r *api.PackageRevisionResources, task *api.Task) error
}

// func (m *mockPackageRevisionDraft) UpdateResources(ctx context.Context, resources *api.PackageRevisionResources, task *api.Task) error {
// 	return nil
// }

func (m *mockPackageRevisionDraft) UpdateResources(ctx context.Context, r *api.PackageRevisionResources, task *api.Task) error {
	return m.updateResourcesFn(ctx, r, task)
}

func (m *mockPackageRevisionDraft) Key() repository.PackageRevisionKey {
	return repository.PackageRevisionKey{
		WorkspaceName: "test-revision",
		Revision:      1,
	}
}

func (m *mockPackageRevisionDraft) UpdateLifecycle(ctx context.Context, lifecycle api.PackageRevisionLifecycle) error {
	return nil
}

func (m *mockPackageRevisionDraft) GetMeta() metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:      "mock-package-revision",
		Namespace: "default",
	}
}

func TestApplyTasks(t *testing.T) {
	tests := []struct {
		name          string
		tasks         []api.Task
		expectedError string
	}{
		{
			name: "Valid Clone task",
			tasks: []api.Task{
				{
					Type: api.TaskTypeClone,
					Clone: &api.PackageCloneTaskSpec{
						Upstream: api.UpstreamPackage{
							Type: api.RepositoryTypeGit,
							Git: &api.GitPackage{
								Repo:      "https://github.com/example/repo.git",
								Ref:       "main",
								Directory: "/path/to/package",
							},
						},
						Strategy: "copy-merge",
					},
				},
			},
			expectedError: "cannot fetch repository /",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := &genericTaskHandler{
				runnerOptionsResolver: func(namespace string) fnruntime.RunnerOptions {
					// Return a mock RunnerOptions
					return fnruntime.RunnerOptions{
						DisplayResourceCount: false,
					}
				},
			}

			// Mock inputs
			ctx := context.Background()

			draft := &mockPackageRevisionDraft{}
			repositoryObj := &configapi.Repository{}
			obj := &api.PackageRevision{
				Spec: api.PackageRevisionSpec{
					Tasks: tt.tasks,
				},
			}
			packageConfig := &builtins.PackageConfig{}

			// Call ApplyTasks
			err := handler.ApplyTasks(ctx, draft, repositoryObj, obj, packageConfig)

			// Verify results
			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestMapTaskToMutationPatchTask(t *testing.T) {
	ctx := context.Background()

	// Mock genericTaskHandler
	handler := &genericTaskHandler{
		cloneStrategy: api.CopyMerge,
	}

	// Mock task of type Patch
	patchTask := &api.Task{
		Type: api.TaskTypePatch,
		Patch: &api.PackagePatchTaskSpec{
			Patches: []api.PatchSpec{
				{
					File:      "test.yaml",
					Contents:  "patch contents",
					PatchType: api.PatchTypePatchFile,
				},
			},
		},
	}

	// Mock PackageRevision
	obj := &api.PackageRevision{
		Spec: api.PackageRevisionSpec{
			PackageName: "test-package",
		},
	}

	// Call mapTaskToMutation
	mutation, err := handler.mapTaskToMutation(ctx, obj, patchTask, false, nil)

	// Verify results
	assert.NoError(t, err)
	assert.NotNil(t, mutation)
	assert.IsType(t, &applyPatchMutation{}, mutation)
}

var sampleRenderStatus = &api.RenderStatus{
	Result: api.ResultList{
		Items: []*api.Result{
			{
				Image:    "gcr.io/kpt-fn/set-namespace:v0.4.0",
				ExitCode: 0,
				Results: []api.ResultItem{
					{
						Message:  `namespace "" updated to "example-namespace", 1 value(s) changed`,
						Severity: "info",
					},
				},
			},
			{
				Image:    "gcr.io/kpt-fn/apply-replacements:v0.1.0",
				ExitCode: 1,
			},
		},
	},
	Err: "fn.render: pkg /:\n\tpkg.render:\n\tpipeline.run: func eval \"gcr.io/kpt-fn/apply-replacements:v0.1.0\" failed: rpc error: code = Internal desc = unable to evaluate gcr.io/kpt-fn/apply-replacements:v0.1.0 with pod evaluator: rpc error: code = Internal desc = failed to evaluate function \"gcr.io/kpt-fn/apply-replacements:v0.1.0\" with structured results: [error]: target must specify resources to select and stderr: ",
}

var sampleResourcesOutput = repository.PackageResources{
	Contents: map[string]string{
		"Kptfile":                 "Kptfile content string",
		"README.md":               "readme file content string",
		"deployment.yaml":         "deployment file content string",
		"apply-replacements.yaml": "apply-replacements file content string",
		"package-context.yaml":    "package-context file content string",
	},
}

var sampleExpectedYamlOutputString = `status:
  result:
    kind: ""
    apiversion: ""
    exitCode: 0
    items:
      - image: gcr.io/kpt-fn/set-namespace:v0.4.0
        exitCode: 0
        results:
          - message: namespace "" updated to "example-namespace", 1 value(s) changed
            severity: info
      - image: gcr.io/kpt-fn/apply-replacements:v0.1.0
        exitCode: 1
  err: "fn.render: pkg /:\n\tpkg.render:\n\tpipeline.run: func eval \"gcr.io/kpt-fn/apply-replacements:v0.1.0\" failed: rpc error: code = Internal desc = unable to evaluate gcr.io/kpt-fn/apply-replacements:v0.1.0 with pod evaluator: rpc error: code = Internal desc = failed to evaluate function \"gcr.io/kpt-fn/apply-replacements:v0.1.0\" with structured results: [error]: target must specify resources to select and stderr: "
`

func copyPackageResources(original repository.PackageResources) repository.PackageResources {
	newContents := make(map[string]string, len(original.Contents))
	for key, value := range original.Contents {
		newContents[key] = value
	}
	return repository.PackageResources{
		Contents: newContents,
	}
}

func TestAppendRenderResult(t *testing.T) {
	type args struct {
		renderStatus *api.RenderStatus
		resources    repository.PackageResources
	}

	tests := []struct {
		name            string
		args            args
		expectedYAML    string
		overrideMarshal bool
	}{
		{
			name: "Main Use Case",
			args: args{
				renderStatus: sampleRenderStatus,
				resources:    copyPackageResources(sampleResourcesOutput),
			},
			expectedYAML: sampleExpectedYamlOutputString,
		},
		{
			name: "Marshal Error Use Case",
			args: args{
				renderStatus: sampleRenderStatus,
				resources:    copyPackageResources(sampleResourcesOutput),
			},
			overrideMarshal: true,
			expectedYAML:    "", // We expect Porchfile to be an empty string
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Optionally override marshal function to simulate error
			if tt.overrideMarshal {
				original := marshalYAML
				defer func() { marshalYAML = original }()
				marshalYAML = func(v interface{}) ([]byte, error) {
					return nil, fmt.Errorf("simulated marshal error")
				}
			}

			appendRenderResult(tt.args.renderStatus, tt.args.resources)
			got := tt.args.resources.Contents["Porchfile"]

			if tt.overrideMarshal {
				if got != "" {
					t.Errorf("Expected Porchfile to be empty string on marshal error, got: %q", got)
				}
				return
			}

			var gotYAML, expectedYAML map[string]interface{}
			if err := yaml.Unmarshal([]byte(got), &gotYAML); err != nil {
				t.Fatalf("Failed to unmarshal generated YAML: %v\nYAML:\n%s", err, got)
			}
			if err := yaml.Unmarshal([]byte(tt.expectedYAML), &expectedYAML); err != nil {
				t.Fatalf("Failed to unmarshal expected YAML: %v", err)
			}

			if !reflect.DeepEqual(gotYAML, expectedYAML) {
				t.Errorf("YAML mismatch.\nGot:\n%v\nWant:\n%v", gotYAML, expectedYAML)
			}
		})
	}
}

type mutationFunc func(context.Context, repository.PackageResources) (repository.PackageResources, *api.TaskResult, error)

func (f mutationFunc) apply(ctx context.Context, res repository.PackageResources) (repository.PackageResources, *api.TaskResult, error) {
	return f(ctx, res)
}

func TestApplyResourceMutations_EvalRenderTask(t *testing.T) {
	ctx := context.Background()

	// Create a fake mutation that returns a render taskResult
	mockMutation := mutationFunc(func(ctx context.Context, res repository.PackageResources) (repository.PackageResources, *api.TaskResult, error) {
		return copyPackageResources(sampleResourcesOutput), &api.TaskResult{
			Task: &api.Task{
				Type: api.TaskTypeEval,
			},
			RenderStatus: sampleRenderStatus,
		}, nil
	})

	// Track UpdateResources call
	var updated bool
	mockDraft := &mockPackageRevisionDraft{
		updateResourcesFn: func(ctx context.Context, r *api.PackageRevisionResources, task *api.Task) error {
			updated = true
			if r == nil || r.Spec.Resources["Porchfile"] == "" {
				t.Errorf("Expected Porchfile to be populated in resources, got: %v", r.Spec.Resources)
			}
			return nil
		},
	}

	applied, renderStatus, err := applyResourceMutations(ctx, mockDraft, copyPackageResources(sampleResourcesOutput), []mutation{mockMutation})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if !updated {
		t.Errorf("Expected UpdateResources to be called")
	}
	if renderStatus == nil {
		t.Errorf("Expected renderStatus to be returned")
	}
	if applied.Contents["Porchfile"] == "" {
		t.Errorf("Expected Porchfile to be added to resources")
	}
}

func TestApplyResourceMutations_EvalRenderTaskWithError(t *testing.T) {
	ctx := context.Background()

	// This mutation returns a render taskResult but with a failure
	mockMutation := mutationFunc(func(ctx context.Context, res repository.PackageResources) (repository.PackageResources, *api.TaskResult, error) {
		return repository.PackageResources{}, &api.TaskResult{
			Task: &api.Task{
				Type: api.TaskTypeEval,
			},
			RenderStatus: sampleRenderStatus,
		}, errors.New("render failed")
	})

	var updateCalled bool
	mockDraft := &mockPackageRevisionDraft{
		updateResourcesFn: func(ctx context.Context, r *api.PackageRevisionResources, task *api.Task) error {
			updateCalled = true
			if r.Spec.Resources["Porchfile"] == "" {
				t.Errorf("Expected Porchfile to be included in resources")
			}
			return nil
		},
	}

	applied, _, err := applyResourceMutations(ctx, mockDraft, copyPackageResources(sampleResourcesOutput), []mutation{mockMutation})
	if err == nil {
		t.Fatalf("Expected error to be returned")
	}
	if !strings.Contains(err.Error(), "Porch Package Pipeline Failed Render") {
		t.Errorf("Expected wrapped render error, got: %v", err)
	}
	if !updateCalled {
		t.Errorf("Expected UpdateResources to be called even after render error")
	}
	if applied.Contents["Porchfile"] == "" {
		t.Errorf("Expected Porchfile to be written even after render error")
	}
}

func TestApplyResourceMutations_UpdateResourcesFails(t *testing.T) {
	ctx := context.Background()

	mockMutation := mutationFunc(func(ctx context.Context, res repository.PackageResources) (repository.PackageResources, *api.TaskResult, error) {
		// Successful mutation (non-render)
		return copyPackageResources(sampleResourcesOutput), &api.TaskResult{
			Task: &api.Task{
				Type: api.TaskTypeInit, // Not Eval
			},
		}, nil
	})

	mockDraft := &mockPackageRevisionDraft{
		updateResourcesFn: func(ctx context.Context, r *api.PackageRevisionResources, task *api.Task) error {
			return errors.New("update failed")
		},
	}

	applied, renderStatus, err := applyResourceMutations(ctx, mockDraft, copyPackageResources(sampleResourcesOutput), []mutation{mockMutation})
	if err == nil {
		t.Fatalf("Expected error due to failed UpdateResources")
	}
	if !strings.Contains(err.Error(), "update failed") {
		t.Errorf("Unexpected error returned: %v", err)
	}
	// Don't expect applied.Contents to be nil — it was set before the update failed.
	if applied.Contents == nil || len(applied.Contents) == 0 {
		t.Errorf("Expected applied to be populated before update failure")
	}
	if renderStatus != nil {
		t.Errorf("Expected no renderStatus returned")
	}
}
