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

	kptfilev1 "github.com/kptdev/kpt/pkg/api/kptfile/v1"
	kptfn "github.com/kptdev/krm-functions-sdk/go/fn"
	porchapi "github.com/nephio-project/porch/api/porch/v1alpha1"
	configapi "github.com/nephio-project/porch/api/porchconfig/v1alpha1"
	fakeextrepo "github.com/nephio-project/porch/pkg/externalrepo/fake"
	"github.com/stretchr/testify/require"

	"github.com/kptdev/kpt/pkg/lib/pkgcontext/pkgcontexttypes"
	"github.com/kptdev/kpt/pkg/lib/runneroptions"
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
				runnerOptionsResolver: func(namespace string) runneroptions.RunnerOptions {
					// Return a mock RunnerOptions
					return runneroptions.RunnerOptions{
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
			packageConfig := &pkgcontexttypes.PackageConfig{}

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
	ror := func(namespace string) runneroptions.RunnerOptions {
		return runneroptions.RunnerOptions{
			ImagePullPolicy: runneroptions.IfNotPresentPull,
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
	repoPr.Resources.Spec.Resources[kptfilev1.KptFileName] = `apiVersion: kpt.dev/v1
kind: Kptfile
metadata:
  name: test-package
`
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
	ror := func(namespace string) runneroptions.RunnerOptions {
		return runneroptions.RunnerOptions{
			ImagePullPolicy: runneroptions.IfNotPresentPull,
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

func TestKptfilePreservesComments(t *testing.T) {
	// Kptfile with labels/annotations present
	originalKptfile := `
apiVersion: kpt.dev/v1
kind: Kptfile
metadata: # kpt-merge: /my-pkg
  name: my-pkg # this is the package name
  labels:
    foo: bar # upstream label
    toremove: will-be-removed # label to be removed
  annotations:
    foo: bar # upstream annotation
    toremove: will-be-removed # annotation to be removed
# Top-level comment
info:
  readinessGates:
    # This gate is important
    - conditionType: Ready # readiness gate comment
`

	resources := map[string]string{
		"Kptfile": originalKptfile,
	}

	obj := &porchapi.PackageRevision{
		Spec: porchapi.PackageRevisionSpec{
			PackageMetadata: &porchapi.PackageMetadata{
				Labels: map[string]string{
					"new-label": "new-label-value",
					"foo":       "bar-updated",
				},
				Annotations: map[string]string{
					"new-annotation": "new-annotation-value",
					"foo":            "bar-updated",
				},
			},
			ReadinessGates: []porchapi.ReadinessGate{
				{ConditionType: "Ready"},
				{ConditionType: "Healthy"},
			},
		},
	}

	kptf, err := kptfn.NewKptfileFromPackage(resources)
	require.NoError(t, err)

	kptf.SetLabels(obj.Spec.PackageMetadata.Labels)
	labelsAfter := kptf.GetLabels()
	assert.Equal(t, "bar-updated", labelsAfter["foo"])
	assert.Equal(t, "new-label-value", labelsAfter["new-label"])
	assert.NotContains(t, labelsAfter, "toremove")

	kptf.SetAnnotations(obj.Spec.PackageMetadata.Annotations)
	annotationsAfter := kptf.GetAnnotations()
	assert.Equal(t, "bar-updated", annotationsAfter["foo"])
	assert.Equal(t, "new-annotation-value", annotationsAfter["new-annotation"])
	assert.NotContains(t, annotationsAfter, "toremove")

	var gates kptfn.SliceSubObjects
	for _, rg := range obj.Spec.ReadinessGates {
		ko, err := kptfn.NewFromTypedObject(rg)
		require.NoError(t, err)
		gates = append(gates, &ko.SubObject)
	}
	require.NoError(t, kptf.SetReadinessGates(gates))

	require.NoError(t, kptf.WriteToPackage(resources))
	got := resources["Kptfile"]

	assert.Contains(t, got, "# this is the package name")
	assert.Contains(t, got, "# upstream label")
	assert.Contains(t, got, "# upstream annotation")
	assert.Contains(t, got, "# Top-level comment")
	assert.Contains(t, got, "# This gate is important")
	assert.Contains(t, got, "# readiness gate comment")

	// Kptfile with labels/annotations initially empty
	emptyKptfile := `
apiVersion: kpt.dev/v1
kind: Kptfile
metadata:
  name: my-pkg
# Top-level comment
info:
  readinessGates:
    - conditionType: Ready
`

	resources2 := map[string]string{
		"Kptfile": emptyKptfile,
	}

	obj2 := &porchapi.PackageRevision{
		Spec: porchapi.PackageRevisionSpec{
			PackageMetadata: &porchapi.PackageMetadata{
				Labels: map[string]string{
					"foo": "bar",
				},
				Annotations: map[string]string{
					"foo": "bar",
				},
			},
		},
	}
	kptf2, err := kptfn.NewKptfileFromPackage(resources2)
	require.NoError(t, err)

	labels2 := kptf2.GetLabels()
	for k, v := range obj2.Spec.PackageMetadata.Labels {
		labels2[k] = v
	}
	kptf2.SetLabels(labels2)

	annotations2 := kptf2.GetAnnotations()
	for k, v := range obj2.Spec.PackageMetadata.Annotations {
		annotations2[k] = v
	}
	kptf2.SetAnnotations(annotations2)
	require.NoError(t, kptf2.WriteToPackage(resources2))
	got2 := resources2["Kptfile"]
	assert.Contains(t, got2, "foo: bar")
	assert.Contains(t, got2, "# Top-level comment")
}
