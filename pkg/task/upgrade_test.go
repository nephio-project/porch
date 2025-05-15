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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func createMockedResources() repository.PackageResources {
	return repository.PackageResources{
		Contents: map[string]string{
			"apiVersion": "kpt.dev/v1alpha1",
			"kind":       "Kptfile",
		},
	}
}

func createMockedPackageRevisionWithClone() api.PackageRevision {
	return api.PackageRevision{
		TypeMeta: metav1.TypeMeta{
			Kind:       "PackageRevision",
			APIVersion: api.SchemeGroupVersion.Identifier(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test-clone",
		},
		Spec: api.PackageRevisionSpec{
			PackageName:    "cloned-package",
			WorkspaceName:  "workspace",
			RepositoryName: "",
			Tasks: []api.Task{
				{
					Type: api.TaskTypeClone,
					Clone: &api.PackageCloneTaskSpec{
						Upstream: api.UpstreamPackage{
							UpstreamRef: &api.PackageRevisionRef{
								Name: "test",
							},
						},
					},
				},
			},
		},
	}
}

func createMockedPackageRevisionWithUpgrade() api.PackageRevision {
	return api.PackageRevision{
		TypeMeta: metav1.TypeMeta{
			Kind:       "PackageRevision",
			APIVersion: api.SchemeGroupVersion.Identifier(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test-upgrade",
		},
		Spec: api.PackageRevisionSpec{
			PackageName:    "upgrade-package",
			WorkspaceName:  "workspace",
			RepositoryName: "",
			Tasks: []api.Task{
				{
					Type: api.TaskTypeUpgrade,
					Upgrade: &api.PackageUpgradeTaskSpec{
						OldUpstream: api.PackageRevisionRef{
							Name: "origin",
						},
						NewUpstream: api.PackageRevisionRef{
							Name: "updated",
						},
						LocalPackageRevisionRef: api.PackageRevisionRef{
							Name: "local",
						},
						Strategy: api.ResourceMerge,
					},
				},
			},
		},
	}
}

func TestApplyErrorInvalidUpstreamUprade(t *testing.T) {
	ctx := context.Background()

	// Mock resources and tasks with valid upstream but no resources to fetch
	resources := createMockedResources()
	updateTask := &api.Task{
		Type: api.TaskTypeUpgrade,
		Upgrade: &api.PackageUpgradeTaskSpec{
			OldUpstream: api.PackageRevisionRef{
				Name: "original",
			},
			NewUpstream: api.PackageRevisionRef{
				Name: "upstream",
			},
			LocalPackageRevisionRef: api.PackageRevisionRef{
				Name: "destination",
			},
		},
	}

	mutation := &upgradePackageMutation{
		upgradeTask: updateTask,
		pkgName:     "test-package",
	}

	_, _, err := mutation.apply(ctx, resources)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "error fetching the resources for package")
}

func TestFindUpgradeTaskFindsUpgrade(t *testing.T) {
	pr := createMockedPackageRevisionWithUpgrade()
	task := findUpgradeTask(&pr)
	assert.Equal(t, task.Type, api.TaskTypeUpgrade)
}

func TestFindUpgradeTaskNotFindsUpgrade(t *testing.T) {
	pr := createMockedPackageRevisionWithClone()
	task := findUpgradeTask(&pr)
	assert.Nil(t, task)
}
