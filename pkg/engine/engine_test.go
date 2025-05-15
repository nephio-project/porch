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

package engine

import (
	"context"
	"testing"

	porchapi "github.com/nephio-project/porch/api/porch/v1alpha1"
	"github.com/nephio-project/porch/pkg/externalrepo/fake"
	"github.com/nephio-project/porch/pkg/repository"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreatePRWith2Tasks(t *testing.T) {
	pr := &porchapi.PackageRevision{
		Spec: porchapi.PackageRevisionSpec{
			Tasks: []porchapi.Task{
				{
					Type: porchapi.TaskTypeInit,
					Init: &porchapi.PackageInitTaskSpec{},
				},
				{
					Type: porchapi.TaskTypeEval,
					Eval: &porchapi.FunctionEvalTaskSpec{
						Image: "render",
					},
				},
			},
		},
	}

	engine := &cadEngine{}

	_, err := engine.CreatePackageRevision(context.TODO(), nil, pr, nil)
	assert.ErrorContains(t, err, "must not contain more than one")
}

func TestCreatePRInitIsAdded(t *testing.T) {
	pr := &porchapi.PackageRevision{
		Spec: porchapi.PackageRevisionSpec{
			// short-circuit the method
			Lifecycle: "test",
		},
	}

	engine := &cadEngine{}

	_, err := engine.CreatePackageRevision(context.TODO(), nil, pr, nil)

	require.ErrorContains(t, err, "unsupported lifecycle value")
	require.Len(t, pr.Spec.Tasks, 1)
	require.Equal(t, pr.Spec.Tasks[0].Type, porchapi.TaskTypeInit)
}

func TestValidateUpgradeTask(t *testing.T) {
	oldUpstream := &fake.FakePackageRevision{
		PrKey: repository.PackageRevisionKey{
			PkgKey: repository.PackageKey{
				RepoKey: repository.RepositoryKey{
					Name: "blueprint",
				},
				Package: "test-package",
			},
			WorkspaceName: "v1",
		},
		PackageLifecycle: porchapi.PackageRevisionLifecyclePublished,
	}

	newUpstream := oldUpstream
	newUpstream.PrKey.WorkspaceName = "v2"

	t.Run("Successful", func(t *testing.T) {
		local := &fake.FakePackageRevision{
			PrKey: repository.PackageRevisionKey{
				PkgKey: repository.PackageKey{
					RepoKey: repository.RepositoryKey{
						Name: "deployment",
					},
					Package: "test-package",
				},
				WorkspaceName: "v1",
			},
			PackageLifecycle: porchapi.PackageRevisionLifecyclePublished,
		}

		revs := []repository.PackageRevision{oldUpstream, newUpstream, local}
		spec := &porchapi.PackageUpgradeTaskSpec{
			OldUpstream: porchapi.PackageRevisionRef{
				Name: oldUpstream.KubeObjectName(),
			},
			NewUpstream: porchapi.PackageRevisionRef{
				Name: newUpstream.KubeObjectName(),
			},
			LocalPackageRevisionRef: porchapi.PackageRevisionRef{
				Name: local.KubeObjectName(),
			},
		}

		err := validateUpgradeTask(context.TODO(), revs, spec)
		assert.NoError(t, err)
	})

	t.Run("Failure", func(t *testing.T) {
		local := &fake.FakePackageRevision{
			PrKey: repository.PackageRevisionKey{
				PkgKey: repository.PackageKey{
					RepoKey: repository.RepositoryKey{
						Name: "deployment",
					},
					Package: "test-package",
				},
				WorkspaceName: "v1",
			},
			PackageLifecycle: porchapi.PackageRevisionLifecycleDraft,
		}

		revs := []repository.PackageRevision{oldUpstream, newUpstream, local}
		spec := &porchapi.PackageUpgradeTaskSpec{
			OldUpstream: porchapi.PackageRevisionRef{
				Name: oldUpstream.KubeObjectName(),
			},
			NewUpstream: porchapi.PackageRevisionRef{
				Name: newUpstream.KubeObjectName(),
			},
			LocalPackageRevisionRef: porchapi.PackageRevisionRef{
				Name: local.KubeObjectName(),
			},
		}

		err := validateUpgradeTask(context.TODO(), revs, spec)
		assert.ErrorContains(t, err, "must be published")
		assert.ErrorContains(t, err, local.KubeObjectName())
	})
}
