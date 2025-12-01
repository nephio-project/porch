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
	"strings"
	"testing"

	porchapi "github.com/nephio-project/porch/api/porch/v1alpha1"
	configapi "github.com/nephio-project/porch/api/porchconfig/v1alpha1"
	fakeextrepo "github.com/nephio-project/porch/pkg/externalrepo/fake"
	"github.com/stretchr/testify/require"

	"github.com/nephio-project/porch/internal/kpt/builtins"
	"github.com/nephio-project/porch/internal/kpt/fnruntime"
	"github.com/nephio-project/porch/pkg/repository"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type mockPackageRevisionDraft struct{}

func (m *mockPackageRevisionDraft) UpdateResources(ctx context.Context, resources *porchapi.PackageRevisionResources, task *porchapi.Task) error {
	return nil
}

func (m *mockPackageRevisionDraft) Key() repository.PackageRevisionKey {
	return repository.PackageRevisionKey{
		WorkspaceName: "test-revision",
		Revision:      1,
	}
}

func (m *mockPackageRevisionDraft) UpdateLifecycle(ctx context.Context, lifecycle porchapi.PackageRevisionLifecycle) error {
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
		tasks         []porchapi.Task
		expectedError string
	}{
		{
			name: "Valid Clone task",
			tasks: []porchapi.Task{
				{
					Type: porchapi.TaskTypeClone,
					Clone: &porchapi.PackageCloneTaskSpec{
						Upstream: porchapi.UpstreamPackage{
							Type: porchapi.RepositoryTypeGit,
							Git: &porchapi.GitPackage{
								Repo:      "https://github.com/example/repo.git",
								Ref:       "main",
								Directory: "/path/to/package",
							},
						},
					},
				},
			},
			expectedError: "cannot fetch repository",
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
				repoOperationRetryAttempts: 3,
			}

			// Mock inputs
			ctx := context.Background()

			draft := &mockPackageRevisionDraft{}
			repositoryObj := &configapi.Repository{}
			obj := &porchapi.PackageRevision{
				Spec: porchapi.PackageRevisionSpec{
					Tasks: tt.tasks,
				},
			}
			packageConfig := &builtins.PackageConfig{}

			// Call ApplyTask
			err := handler.ApplyTask(ctx, draft, repositoryObj, obj, packageConfig)

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

func TestMapTaskToMutationUpgradeTask(t *testing.T) {
	th := &genericTaskHandler{}

	task := &porchapi.Task{
		Type: porchapi.TaskTypeUpgrade,
		Upgrade: &porchapi.PackageUpgradeTaskSpec{
			OldUpstream: porchapi.PackageRevisionRef{
				Name: "old-upstream",
			},
			NewUpstream: porchapi.PackageRevisionRef{
				Name: "new-upstream",
			},
			LocalPackageRevisionRef: porchapi.PackageRevisionRef{
				Name: "local",
			},
		},
	}

	obj := &porchapi.PackageRevision{
		Spec: porchapi.PackageRevisionSpec{
			PackageName: "test-package",
		},
	}

	mut, err := th.mapTaskToMutation(obj, task, false, nil)

	require.NoError(t, err)
	require.NotNil(t, mut)
	assert.IsType(t, &upgradePackageMutation{}, mut)
}

func TestDoPrMutations(t *testing.T) {
	ror := func(namespace string) fnruntime.RunnerOptions {
		return fnruntime.RunnerOptions{
			ImagePullPolicy: fnruntime.IfNotPresentPull,
			ResolveToImage: func(_ context.Context, image string) (string, error) {
				return image, nil
			},
		}
	}

	th := &genericTaskHandler{
		runnerOptionsResolver: ror,
	}

	repoPr := &fakeextrepo.FakePackageRevision{
		Resources: &porchapi.PackageRevisionResources{
			Spec: porchapi.PackageRevisionResourcesSpec{
				Resources: map[string]string{},
			},
		},
	}
	draft := repoPr

	t.Run("No-op when not draft", func(t *testing.T) {
		oldObj := &porchapi.PackageRevision{
			Spec: porchapi.PackageRevisionSpec{
				Lifecycle: porchapi.PackageRevisionLifecyclePublished,
			},
		}
		err := th.DoPRMutations(context.TODO(), repoPr, oldObj, &porchapi.PackageRevision{}, draft)
		require.NoError(t, err)
		assert.Empty(t, draft.Ops)
	})

	// Not exactly helpful, but gets coverage
	t.Run("Success", func(t *testing.T) {
		oldObj := &porchapi.PackageRevision{
			Spec: porchapi.PackageRevisionSpec{
				Lifecycle: porchapi.PackageRevisionLifecycleDraft,
			},
		}
		err := th.DoPRMutations(context.TODO(), repoPr, oldObj, &porchapi.PackageRevision{}, draft)
		require.NoError(t, err)
		require.NotEmpty(t, draft.Ops)
		assert.Equal(t, "UpdateResources", draft.Ops[len(draft.Ops)-1])
	})
}

func TestDoPrResourceMutations(t *testing.T) {
	ror := func(namespace string) fnruntime.RunnerOptions {
		return fnruntime.RunnerOptions{
			ImagePullPolicy: fnruntime.IfNotPresentPull,
			ResolveToImage: func(_ context.Context, image string) (string, error) {
				return image, nil
			},
		}
	}

	th := &genericTaskHandler{
		runnerOptionsResolver: ror,
	}

	repoPr := &fakeextrepo.FakePackageRevision{
		Resources: &porchapi.PackageRevisionResources{
			Spec: porchapi.PackageRevisionResourcesSpec{
				Resources: map[string]string{},
			},
		},
	}
	draft := repoPr

	t.Run("Empty resources", func(t *testing.T) {
		oldRes := &porchapi.PackageRevisionResources{
			Spec: porchapi.PackageRevisionResourcesSpec{
				Resources: map[string]string{},
			},
		}
		newRes := oldRes.DeepCopy()
		renderStatus, err := th.DoPRResourceMutations(context.TODO(), repoPr, draft, oldRes, newRes)
		require.NoError(t, err)
		assert.Nil(t, renderStatus)
		require.NotEmpty(t, draft.Ops)
		assert.Equal(t, "UpdateResources", draft.Ops[len(draft.Ops)-1])
		assert.Empty(t, draft.Resources.Spec.Resources)
	})

	t.Run("Basic resources", func(t *testing.T) {
		oldRes := &porchapi.PackageRevisionResources{
			Spec: porchapi.PackageRevisionResourcesSpec{
				Resources: map[string]string{
					"foo.txt": "bar",
				},
			},
		}
		newRes := oldRes.DeepCopy()
		newRes.Spec.Resources["baz.txt"] = "bar"
		renderStatus, err := th.DoPRResourceMutations(context.TODO(), repoPr, draft, oldRes, newRes)
		require.NoError(t, err)
		assert.Nil(t, renderStatus)
		require.NotEmpty(t, draft.Ops)
		assert.Equal(t, "UpdateResources", draft.Ops[len(draft.Ops)-1])
		assert.Equal(t, map[string]string{
			"foo.txt": "bar",
			"baz.txt": "bar",
		}, draft.Resources.Spec.Resources)
	})

	// TODO: test rendering
}

func TestRenderError(t *testing.T) {
	baseErr := errors.New("some base error")
	wrappedErr := renderError(baseErr)

	if wrappedErr == nil {
		t.Fatal("expected non-nil error")
	}

	// Check that the error message contains both the base error and the wrapper message
	got := wrappedErr.Error()

	if !strings.Contains(got, "some base error") {
		t.Errorf("expected base error message to be included, got: %q", got)
	}

	if !strings.Contains(got, "Error rendering package in kpt function pipeline") {
		t.Errorf("expected wrapper message to be included, got: %q", got)
	}
}
