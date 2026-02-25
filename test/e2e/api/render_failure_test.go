// Copyright 2026 The kpt and Nephio Authors
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

package api

import (
	kptfilev1 "github.com/kptdev/kpt/pkg/api/kptfile/v1"
	porchapi "github.com/nephio-project/porch/api/porch/v1alpha1"
	"github.com/nephio-project/porch/test/e2e/suiteutils"
	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// TestRenderFailureNoPush verifies that when rendering fails and the
// PackageRevision does NOT have the push-on-render-failure annotation,
// the update returns an error and resources are not persisted.
func (t *PorchSuite) TestRenderFailureNoPush() {
	t.skipIfLocalPodEvaluator()

	const (
		repository  = "render-fail-no-push"
		packageName = "test-no-push"
		workspace   = defaultWorkspace
	)

	t.RegisterGitRepositoryF(t.GetPorchTestRepoURL(), repository, "", suiteutils.GiteaUser, suiteutils.GiteaPassword)
	pr := t.CreatePackageDraftF(repository, packageName, workspace)

	// Get resources and add a broken mutator
	var resources porchapi.PackageRevisionResources
	t.GetF(client.ObjectKey{Namespace: t.Namespace, Name: pr.Name}, &resources)

	resources.Spec.Resources["configmap.yaml"] = `apiVersion: v1
kind: ConfigMap
metadata:
  name: test-cm
data:
  key: before-render
`
	t.AddMutator(&resources, "quay.io/invalid/nonexistent-fn:v0.0.1")

	// Update should fail
	err := t.Client.Update(t.GetContext(), &resources)
	assert.Error(t, err, "expected render failure error")
	assert.Contains(t, err.Error(), "error rendering package in kpt function pipeline")

	// Re-fetch resources — the broken mutator and configmap should NOT be persisted
	var refetched porchapi.PackageRevisionResources
	t.GetF(client.ObjectKey{Namespace: t.Namespace, Name: pr.Name}, &refetched)
	_, hasConfigMap := refetched.Spec.Resources["configmap.yaml"]
	assert.False(t, hasConfigMap, "configmap should not be persisted when push-on-render-failure is not set")
}

// TestRenderFailureWithPushAnnotation verifies that when rendering fails but
// the PackageRevision has porch.kpt.dev/push-on-render-failure: "true", the resources
// are persisted to git despite the render error.
func (t *PorchSuite) TestRenderFailureWithPushAnnotation() {
	t.skipIfLocalPodEvaluator()

	const (
		repository  = "render-fail-push"
		packageName = "test-push-on-fail"
		workspace   = defaultWorkspace
	)

	t.RegisterGitRepositoryF(t.GetPorchTestRepoURL(), repository, "", suiteutils.GiteaUser, suiteutils.GiteaPassword)
	pr := t.CreatePackageDraftF(repository, packageName, workspace)

	// Set push-on-render-failure annotation on the PackageRevision
	if pr.Annotations == nil {
		pr.Annotations = make(map[string]string)
	}
	pr.Annotations[porchapi.PushOnRenderFailureAnnotation] = "true"
	t.UpdateF(pr)

	// Get resources and add a broken mutator
	var resources porchapi.PackageRevisionResources
	t.GetF(client.ObjectKey{Namespace: t.Namespace, Name: pr.Name}, &resources)

	resources.Spec.Resources["configmap.yaml"] = `apiVersion: v1
kind: ConfigMap
metadata:
  name: test-cm
data:
  key: before-render
`
	t.AddMutator(&resources, "quay.io/invalid/nonexistent-fn:v0.0.1")

	// Update returns an error (render failed) but resources should be pushed
	err := t.Client.Update(t.GetContext(), &resources)
	assert.Error(t, err, "expected render failure error")

	// Re-fetch resources — they should be persisted (unrendered, since no save-on-render-failure in Kptfile)
	var refetched porchapi.PackageRevisionResources
	t.GetF(client.ObjectKey{Namespace: t.Namespace, Name: pr.Name}, &refetched)
	cm, hasConfigMap := refetched.Spec.Resources["configmap.yaml"]
	assert.True(t, hasConfigMap, "configmap should be persisted when push-on-render-failure annotation is set")

	// Verify the content is the original unrendered resource
	assert.Contains(t, cm, "key: before-render",
		"configmap should contain original unrendered content")
}

// TestRenderFailureWithSaveAndPushAnnotations verifies the combination of both
// Kptfile kpt.dev/save-on-render-failure and PackageRevision porch.kpt.dev/push-on-render-failure
// annotations. The partially-rendered resources should be persisted.
func (t *PorchSuite) TestRenderFailureWithSaveAndPushAnnotations() {
	t.skipIfLocalPodEvaluator()

	const (
		repository  = "render-fail-save-push"
		packageName = "test-save-and-push"
		workspace   = defaultWorkspace
	)

	t.RegisterGitRepositoryF(t.GetPorchTestRepoURL(), repository, "", suiteutils.GiteaUser, suiteutils.GiteaPassword)
	pr := t.CreatePackageDraftF(repository, packageName, workspace)

	// Set push-on-render-failure annotation on the PackageRevision
	if pr.Annotations == nil {
		pr.Annotations = make(map[string]string)
	}
	pr.Annotations[porchapi.PushOnRenderFailureAnnotation] = "true"
	t.UpdateF(pr)

	// Get resources and set save-on-render-failure annotation in Kptfile
	var resources porchapi.PackageRevisionResources
	t.GetF(client.ObjectKey{Namespace: t.Namespace, Name: pr.Name}, &resources)

	kptfile := t.ParseKptfileF(&resources)
	if kptfile.Annotations == nil {
		kptfile.Annotations = make(map[string]string)
	}
	kptfile.Annotations[kptfilev1.SaveOnRenderFailureAnnotation] = "true"

	// Add a working mutator first, then a broken one.
	// The working mutator should produce partial results that get saved.
	if kptfile.Pipeline == nil {
		kptfile.Pipeline = &kptfilev1.Pipeline{}
	}
	kptfile.Pipeline.Mutators = append(kptfile.Pipeline.Mutators,
		kptfilev1.Function{
			Image: t.KrmFunctionsRegistry + "/set-annotations:v0.1.4",
			ConfigMap: map[string]string{
				"render-test": "partial",
			},
		},
		kptfilev1.Function{
			Image: "quay.io/invalid/nonexistent-fn:v0.0.1",
		},
	)
	t.SaveKptfileF(&resources, kptfile)

	resources.Spec.Resources["configmap.yaml"] = `apiVersion: v1
kind: ConfigMap
metadata:
  name: test-cm
data:
  key: before-render
`

	// Update returns an error but partially-rendered resources should be pushed
	err := t.Client.Update(t.GetContext(), &resources)
	assert.Error(t, err, "expected render failure error")

	// Re-fetch resources - they should be persisted with partial rendering
	var refetched porchapi.PackageRevisionResources
	t.GetF(client.ObjectKey{Namespace: t.Namespace, Name: pr.Name}, &refetched)
	cm, hasConfigMap := refetched.Spec.Resources["configmap.yaml"]
	assert.True(t, hasConfigMap, "configmap should be persisted with save-on-render-failure and push-on-render-failure")

	// Verify partial rendering — the working set-annotations mutator should have run
	assert.Contains(t, cm, "render-test: partial",
		"configmap should have annotation from the working mutator (partial render)")

	// Verify the Kptfile still has the save-on-render-failure annotation
	refetchedKptfile := t.ParseKptfileF(&refetched)
	assert.Equal(t, "true", refetchedKptfile.Annotations[kptfilev1.SaveOnRenderFailureAnnotation],
		"save-on-render-failure annotation should be preserved")
}
