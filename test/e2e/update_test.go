// Copyright 2022 The kpt and Nephio Authors
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

package e2e

import (
	"context"

	porchapi "github.com/nephio-project/porch/api/porch/v1alpha1"
	"github.com/nephio-project/porch/pkg/repository"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (t *PorchSuite) TestPackageUpdateRecloneAndReplay(ctx context.Context) {
	const (
		gitRepository = "package-update"
	)

	t.registerGitRepositoryF(ctx, testBlueprintsRepo, "test-blueprints", "")

	var list porchapi.PorchPkgRevisionList
	t.ListE(ctx, &list, client.InNamespace(t.namespace))

	basensV2 := MustFindPorchPkgRevision(t.T, &list, repository.PackageRevisionKey{Repository: "test-blueprints", Package: "basens", Revision: "v2"})
	t.Logf("basensV2 = %v", basensV2)

	// Register the repository as 'downstream'
	t.registerMainGitRepositoryF(ctx, gitRepository)

	// Create PorchPkgRevision from upstream repo
	pr := &porchapi.PorchPkgRevision{
		TypeMeta: metav1.TypeMeta{
			Kind:       "PorchPkgRevision",
			APIVersion: porchapi.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: t.namespace,
		},
		Spec: porchapi.PorchPkgRevisionSpec{
			PorchPkgName:   "testRecloneAndReplay",
			WorkspaceName:  "testdescr",
			RepositoryName: gitRepository,
			Tasks: []porchapi.Task{
				{
					Type: porchapi.TaskTypeClone,
					Clone: &porchapi.PorchPkgCloneTaskSpec{
						Upstream: porchapi.UpstreamPorchPkg{
							Git: &porchapi.GitPorchPkg{
								Repo:      testBlueprintsRepo,
								Ref:       "v1",
								Directory: "basens",
							},
						},
					},
				},
			},
		},
	}

	t.CreateF(ctx, pr)

	t.GetF(ctx, client.ObjectKey{
		Namespace: t.namespace,
		Name:      pr.Name,
	}, pr)

	// Update the clone task
	pr.Spec.Tasks[0].Clone = &porchapi.PorchPkgCloneTaskSpec{
		Upstream: porchapi.UpstreamPorchPkg{
			Git: &porchapi.GitPorchPkg{
				Repo:      testBlueprintsRepo,
				Ref:       "v2",
				Directory: "basens",
			},
		},
	}

	t.UpdateF(ctx, pr)

	t.GetF(ctx, client.ObjectKey{
		Namespace: t.namespace,
		Name:      pr.Name,
	}, pr)

	var revisionResources porchapi.PorchPkgRevisionResources
	t.GetF(ctx, client.ObjectKey{
		Namespace: t.namespace,
		Name:      pr.Name,
	}, &revisionResources)

	if _, found := revisionResources.Spec.Resources["resourcequota.yaml"]; !found {
		t.Errorf("Updated package should contain 'resourcequota.yaml` file")
	}
}
