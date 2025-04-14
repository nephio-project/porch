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
	"testing"

	api "github.com/nephio-project/porch/api/porch/v1alpha1"
	configapi "github.com/nephio-project/porch/api/porchconfig/v1alpha1"

	"github.com/nephio-project/porch/internal/kpt/builtins"
	"github.com/nephio-project/porch/internal/kpt/fnruntime"
	"github.com/nephio-project/porch/pkg/repository"
	"github.com/stretchr/testify/assert"
)

type mockPackageRevisionDraft struct{}

func (m *mockPackageRevisionDraft) UpdateResources(ctx context.Context, resources *api.PackageRevisionResources, task *api.Task) error {
	return nil
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
