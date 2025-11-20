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

	// Register the main git repository where we'll create the clone
	t.RegisterMainGitRepositoryF(repositoryName)

	// Register the test blueprint repository that contains the 'simple' package
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
		},
	}

	// Create and wait for the package revision to be rendered
	t.CreateF(clonePr)

	// Get the updated package revisions to verify any existing packageMetadata from the upstream
	t.GetF(client.ObjectKeyFromObject(clonePr), clonePr)

	// First validate any existing metadata that came from 'simple' package
	t.Logf("Testing extraction of existing metadata from upstream Kptfile...")

	if clonePr.Spec.PackageMetadata != nil {
		if existingLabels := clonePr.Spec.PackageMetadata.Labels; len(existingLabels) > 0 {
			t.Logf("Found existing labels from upstream: %v", existingLabels)
			if testKey, exists := existingLabels["test-key"]; exists && testKey == "test-value" {
				t.Logf("✓ Successfully extracted existing label 'test-key: test-value' from upstream Kptfile")
			}
		}

		if existingAnnotations := clonePr.Spec.PackageMetadata.Annotations; len(existingAnnotations) > 0 {
			t.Logf("Found existing annotations from upstream: %v", existingAnnotations)
			if localConfig, exists := existingAnnotations["config.kubernetes.io/local-config"]; exists && localConfig == "true" {
				t.Logf("✓ Successfully extracted existing annotation 'config.kubernetes.io/local-config: true' from upstream Kptfile")
			}
		}
	}

	// Adding new metadata by hand
	t.Logf("Testing extraction of manually-added metadata to Kptfile...")

	// Get the package resources to modify the Kptfile
	var packageResources porchapi.PackageRevisionResources
	t.GetF(client.ObjectKeyFromObject(clonePr), &packageResources)

	// Parse and modify the Kptfile to add labels and annotations
	kptfile := t.ParseKptfileF(&packageResources)

	if kptfile.Labels == nil {
		kptfile.Labels = make(map[string]string)
	}
	kptfile.Labels["porch.dev/test-label"] = "added-by-e2e-test"

	if kptfile.Annotations == nil {
		kptfile.Annotations = make(map[string]string)
	}
	kptfile.Annotations["porch.dev/test-annotation"] = "e2e-test-annotation-value"

	// Save the modifies Kptfile back to the package resources
	t.SaveKptfileF(&packageResources, kptfile)

	// Update the package resources and wait for re-render
	t.UpdateF(&packageResources)

	// Get the updated package revision to verify packageMetadata
	t.GetF(client.ObjectKeyFromObject(clonePr), clonePr)

	expectedLabelsFromKptfile := map[string]string{
		"test-key":             "test-value",
		"porch.dev/test-label": "added-by-e2e-test",
	}

	expectedKptfileAnnotations := map[string]string{
		"config.kubernetes.io/local-config": "true",
		"porch.dev/test-annotation":         "e2e-test-annotation-value",
	}

	t.Logf("Validating extracted labels...")
	for expectedKey, expectedValue := range expectedLabelsFromKptfile {
		if actualValue, found := clonePr.Spec.PackageMetadata.Labels[expectedKey]; !found {
			t.Errorf("Expected label %s not found in PackageMetadata", expectedKey)
		} else if actualValue != expectedValue {
			t.Errorf("Label %s: got %v, want %v", expectedKey, actualValue, expectedValue)
		} else {
			t.Logf("✓ Found expected label: %s=%s", expectedKey, expectedValue)
		}
	}

	t.Logf("Validating extracted annotations...")
	for expectedKey, expectedValue := range expectedKptfileAnnotations {
		if actualValue, found := clonePr.Spec.PackageMetadata.Annotations[expectedKey]; !found {
			t.Errorf("Expected annotation %s not found in PackageMetadata", expectedKey)
		} else if actualValue != expectedValue {
			t.Errorf("Annotation %s: got %v, want %v", expectedKey, actualValue, expectedValue)
		} else {
			t.Logf("✓ Found expected annotation: %s=%s", expectedKey, expectedValue)
		}
	}

	t.Logf("Successfully verified PackageMetadata extraction for both existing and newly-added metadata!")
	t.Logf("Final PackageMetadata - Labels: %v", clonePr.Spec.PackageMetadata.Labels)
	t.Logf("Final PackageMetadata - Annotations: %v", clonePr.Spec.PackageMetadata.Annotations)
}
