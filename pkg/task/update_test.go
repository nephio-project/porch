// Copyright 2025 The Nephio Authors
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
	"github.com/nephio-project/porch/pkg/repository"
	"github.com/stretchr/testify/assert"
)

func createMockResources() repository.PackageResources {
	return repository.PackageResources{
		Contents: map[string]string{
			"apiVersion": "kpt.dev/v1alpha1",
			"kind":       "Kptfile",
		},
	}
}

func createMockCloneTask(upstreamType api.RepositoryType) *api.Task {
	return &api.Task{
		Type: api.TaskTypeClone,
		Clone: &api.PackageCloneTaskSpec{
			Upstream: api.UpstreamPackage{
				Type: upstreamType,
			},
		},
	}
}

func createMockUpdateTask(upstreamType api.RepositoryType) *api.Task {
	return &api.Task{
		Type: api.TaskTypeUpdate,
		Update: &api.PackageUpdateTaskSpec{
			Upstream: api.UpstreamPackage{
				Type: upstreamType,
			},
		},
	}
}

func TestApplyErrorInvalidUpstream(t *testing.T) {
	ctx := context.Background()

	// Mock resources and tasks with invalid upstream
	resources := createMockResources()
	cloneTask := createMockCloneTask(api.RepositoryTypeGit)
	updateTask := createMockUpdateTask(api.RepositoryTypeGit)

	mutation := &updatePackageMutation{
		cloneTask:  cloneTask,
		updateTask: updateTask,
		pkgName:    "test-package",
	}

	_, _, err := mutation.apply(ctx, resources)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "upstream package must be porch native package")
}

func TestCurrUpstreamError(t *testing.T) {
	// Create a mutation without a clone task
	mutation := &updatePackageMutation{
		pkgName: "test-package",
	}

	_, err := mutation.currUpstream()

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "package test-package does not have original upstream info")
}

func TestApplyErrorInsvalidUpstream(t *testing.T) {
	ctx := context.Background()

	// Mock resources and tasks with valid upstream but no resources to fetch
	resources := createMockResources()
	cloneTask := &api.Task{
		Type: api.TaskTypeClone,
		Clone: &api.PackageCloneTaskSpec{
			Upstream: api.UpstreamPackage{
				UpstreamRef: &api.PackageRevisionRef{
					Name: "test-package",
				},
			},
		},
	}
	updateTask := &api.Task{
		Type: api.TaskTypeUpdate,
		Update: &api.PackageUpdateTaskSpec{
			Upstream: api.UpstreamPackage{
				UpstreamRef: &api.PackageRevisionRef{
					Name: "test-package",
				},
			},
		},
	}

	mutation := &updatePackageMutation{
		cloneTask:         cloneTask,
		updateTask:        updateTask,
		pkgName:           "test-package",
		referenceResolver: &fakeReferenceResolver{},
	}

	_, _, err := mutation.apply(ctx, resources)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "error fetching the resources for package")
}
