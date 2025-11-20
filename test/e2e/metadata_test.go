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

package e2e

import (
	porchapi "github.com/nephio-project/porch/api/porch/v1alpha1"
	"github.com/nephio-project/porch/pkg/repository"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (t *PorchSuite) TestPackageMetadataFromKptfile() {
	const (
		repositoryName = "test-package-metadata-repo"
		packageName    = "test-package-metadata"
		workspace      = "test-workspace"
	)

	t.RegisterMainGitRepositoryF(repositoryName)
	t.RegisterTestBlueprintRepository("test-blueprints", "")

	var list porchapi.PackageRevisionList
	t.ListE(&list, client.InNamespace(t.Namespace))

	simplePackage := t.MustFindPackageRevision(&list, repository.PackageRevisionKey{
		PkgKey: repository.PackageKey{
			RepoKey: repository.RepositoryKey{
				Name: "test-blueprints",
			},
			Package: "simple",
		},
		Revision: 1,
	})

	// Create a PackageRevision that clones from the 'simple' package
	clonePr := &porchapi.PackageRevision{
		TypeMeta: metav1.TypeMeta{
			Kind:       "PackageRevision",
			APIVersion: porchapi.SchemeGroupVersion.Identifier(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: t.Namespace,
		},
		Spec: porchapi.PackageRevisionSpec{
			PackageName:    packageName,
			WorkspaceName:  workspace,
			RepositoryName: repositoryName,
			Tasks: []porchapi.Task{
				{
					Type: porchapi.TaskTypeClone,
					Clone: &porchapi.PackageCloneTaskSpec{
						Upstream: porchapi.UpstreamPackage{
							UpstreamRef: &porchapi.PackageRevisionRef{
								Name: simplePackage.Name,
							},
						},
					},
				},
			},
			PackageMetadata: &porchapi.PackageMetadata{
				Labels: map[string]string{
					"porch.dev/new-label": "new-label-value",
				},
				Annotations: map[string]string{
					"porch.dev/new-annotation": "new-annotation-value",
				},
			},
		},
	}

	t.Run("Initial creation", func() {
		t.CreateF(clonePr)
		t.GetF(client.ObjectKeyFromObject(clonePr), clonePr)

		expectedLabels := map[string]string{
			"test-key":            "test-value",      // from upstream
			"porch.dev/new-label": "new-label-value", // from PackageMetadata
		}
		expectedAnnotations := map[string]string{
			"config.kubernetes.io/local-config": "true",                 // from upstream
			"porch.dev/new-annotation":          "new-annotation-value", // from PackageMetadata
		}
		t.Require().Equal(expectedLabels, clonePr.Spec.PackageMetadata.Labels)
		for k, v := range expectedAnnotations {
			actual, ok := clonePr.Spec.PackageMetadata.Annotations[k]
			t.Require().True(ok)
			t.Require().Equal(v, actual)
		}

		var packageResources porchapi.PackageRevisionResources
		t.GetF(client.ObjectKeyFromObject(clonePr), &packageResources)
		kptfile := t.ParseKptfileF(&packageResources)
		t.Require().Equal(expectedLabels, kptfile.Labels)
		for k, v := range expectedAnnotations {
			actual, ok := kptfile.Annotations[k]
			t.Require().True(ok)
			t.Require().Equal(v, actual)
		}
	})

	t.Run("Manual Kptfile metadata update", func() {
		var packageResources porchapi.PackageRevisionResources
		t.GetF(client.ObjectKeyFromObject(clonePr), &packageResources)
		kptfile := t.ParseKptfileF(&packageResources)
		kptfile.Labels["porch.dev/test-label"] = "added-by-e2e-test"
		kptfile.Annotations["porch.dev/test-annotation"] = "e2e-test-annotation-value"
		t.SaveKptfileF(&packageResources, kptfile)
		t.UpdateF(&packageResources)
		t.GetF(client.ObjectKeyFromObject(clonePr), clonePr)

		expectedLabelsAfterManual := map[string]string{
			"test-key":             "test-value",
			"porch.dev/new-label":  "new-label-value",
			"porch.dev/test-label": "added-by-e2e-test",
		}
		expectedAnnotationsAfterManual := map[string]string{
			"config.kubernetes.io/local-config": "true",
			"porch.dev/new-annotation":          "new-annotation-value",
			"porch.dev/test-annotation":         "e2e-test-annotation-value",
		}

		t.Require().Equal(expectedLabelsAfterManual, clonePr.Spec.PackageMetadata.Labels)
		for k, v := range expectedAnnotationsAfterManual {
			actual, ok := clonePr.Spec.PackageMetadata.Annotations[k]
			t.Require().True(ok)
			t.Require().Equal(v, actual)
		}

		t.GetF(client.ObjectKeyFromObject(clonePr), &packageResources)
		kptfile = t.ParseKptfileF(&packageResources)
		t.Require().Equal(expectedLabelsAfterManual, kptfile.Labels)
		for k, v := range expectedAnnotationsAfterManual {
			actual, ok := kptfile.Annotations[k]
			t.Require().True(ok)
			t.Require().Equal(v, actual)
		}
	})

	t.Run("Update PackageMetadata", func() {
		clonePr.Spec.PackageMetadata = &porchapi.PackageMetadata{
			Labels: map[string]string{
				"porch.dev/new-label":   "changed-label-value", // changed
				"porch.dev/added-label": "added-label-value",   // new
			},
			Annotations: map[string]string{
				"porch.dev/new-annotation":   "changed-annotation-value", // changed
				"porch.dev/added-annotation": "added-annotation-value",   // new
			},
		}
		t.UpdateF(clonePr)
		t.GetF(client.ObjectKeyFromObject(clonePr), clonePr)

		expectedLabels := map[string]string{
			"porch.dev/new-label":   "changed-label-value",
			"porch.dev/added-label": "added-label-value",
		}
		expectedAnnotations := map[string]string{
			"porch.dev/new-annotation":   "changed-annotation-value",
			"porch.dev/added-annotation": "added-annotation-value",
		}

		t.Require().Equal(expectedLabels, clonePr.Spec.PackageMetadata.Labels)
		for k, v := range expectedAnnotations {
			actual, ok := clonePr.Spec.PackageMetadata.Annotations[k]
			t.Require().True(ok)
			t.Require().Equal(v, actual)
		}

		var packageResources porchapi.PackageRevisionResources
		t.GetF(client.ObjectKeyFromObject(clonePr), &packageResources)
		kptfile := t.ParseKptfileF(&packageResources)
		t.Require().Equal(expectedLabels, kptfile.Labels)
		for k, v := range expectedAnnotations {
			actual, ok := kptfile.Annotations[k]
			t.Require().True(ok)
			t.Require().Equal(v, actual)
		}
	})
}
