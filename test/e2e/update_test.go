// Copyright 2022, 2025 The kpt and Nephio Authors
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
	"os"

	porchapi "github.com/nephio-project/porch/api/porch/v1alpha1"
	"github.com/nephio-project/porch/pkg/repository"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (t *PorchSuite) TestPackageUpdateRecloneAndReplay() {
	const (
		gitRepository = "package-update"
	)

	secret := t.CreateRepositorySecret("test-blueprints", os.Getenv(testBlueprintsRepoUserEnv), Password(os.Getenv(testBlueprintsRepoPasswordEnv)))
	t.RegisterGitRepositoryF(t.testBlueprintsRepo, "test-blueprints", "", secret)

	var list porchapi.PackageRevisionList
	t.ListE(&list, client.InNamespace(t.Namespace))

	basensV2 := t.MustFindPackageRevision(&list, repository.PackageRevisionKey{PkgKey: repository.PackageKey{RepoKey: repository.RepositoryKey{Name: "test-blueprints"}, Package: "basens"}, Revision: 2})
	t.Logf("basensV2 = %v", basensV2)

	// Register the repository as 'downstream'
	t.RegisterMainGitRepositoryF(gitRepository)

	// Create PackageRevision from upstream repo
	pr := &porchapi.PackageRevision{
		TypeMeta: metav1.TypeMeta{
			Kind:       "PackageRevision",
			APIVersion: porchapi.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: t.Namespace,
		},
		Spec: porchapi.PackageRevisionSpec{
			PackageName:    "testrecloneandreplay",
			WorkspaceName:  "testdescr",
			RepositoryName: gitRepository,
			Tasks: []porchapi.Task{
				{
					Type: porchapi.TaskTypeClone,
					Clone: &porchapi.PackageCloneTaskSpec{
						Upstream: porchapi.UpstreamPackage{
							Git: &porchapi.GitPackage{
								Repo:      t.testBlueprintsRepo,
								Ref:       "v1",
								Directory: "basens",
								SecretRef: porchapi.SecretRef{
									Name: t.CreateRepositorySecret("testrecloneandreplay-v1", os.Getenv(testBlueprintsRepoUserEnv), Password(os.Getenv(testBlueprintsRepoPasswordEnv))),
								},
							},
						},
					},
				},
			},
		},
	}

	t.CreateF(pr)

	t.GetF(client.ObjectKey{
		Namespace: t.Namespace,
		Name:      pr.Name,
	}, pr)

	// Update the clone task
	pr.Spec.Tasks[0].Clone = &porchapi.PackageCloneTaskSpec{
		Upstream: porchapi.UpstreamPackage{
			Git: &porchapi.GitPackage{
				Repo:      t.testBlueprintsRepo,
				Ref:       "v2",
				Directory: "basens",
				SecretRef: porchapi.SecretRef{
					Name: t.CreateRepositorySecret("testrecloneandreplay-v2", os.Getenv(testBlueprintsRepoUserEnv), Password(os.Getenv(testBlueprintsRepoPasswordEnv))),
				},
			},
		},
	}

	t.UpdateF(pr)

	t.GetF(client.ObjectKey{
		Namespace: t.Namespace,
		Name:      pr.Name,
	}, pr)

	var revisionResources porchapi.PackageRevisionResources
	t.GetF(client.ObjectKey{
		Namespace: t.Namespace,
		Name:      pr.Name,
	}, &revisionResources)

	if _, found := revisionResources.Spec.Resources["resourcequota.yaml"]; !found {
		t.Errorf("Updated package should contain 'resourcequota.yaml` file")
	}
}
