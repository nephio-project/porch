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

package api

import (
	"context"
	"strings"
	"time"

	porchapi "github.com/nephio-project/porch/api/porch/v1alpha1"
	suiteutils "github.com/nephio-project/porch/test/e2e/suiteutils"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

const (
	skipPodEvaluatorMsg = "Skipping due to not having pod evaluator in local mode"
)

func (t *PorchSuite) TestBuiltinFunctionEvaluator() {
	const (
		repoName = "git-builtin-fn-eval"
	)
	// Register the repository as 'git-builtin-fn'
	t.RegisterGitRepositoryF(t.GetPorchTestRepoURL(), repoName, "", suiteutils.GiteaUser, suiteutils.GiteaPassword)

	// Create Package Revision
	pr := t.CreatePackageCloneF(repoName, "test-builtin-fn-bucket", "test-workspace", defaultBucketBpRef, "bucket")

	// Get package resources
	resources := &porchapi.PackageRevisionResources{}
	t.GetF(client.ObjectKey{
		Namespace: t.Namespace,
		Name:      pr.Name,
	}, resources)

	t.AddMutator(resources, t.KrmFunctionsRegistry+"/starlark:v0.4.3", suiteutils.WithConfigmap(map[string]string{
		"source": `for resource in ctx.resource_list["items"]:
		  resource["metadata"]["annotations"]["foo"] = "bar"`,
	}))
	t.AddMutator(resources, t.KrmFunctionsRegistry+"/set-namespace:v0.4.1", suiteutils.WithConfigmap(map[string]string{"namespace": "bucket-namespace"}))
	t.UpdateF(resources)

	bucket, ok := resources.Spec.Resources["bucket.yaml"]
	if !ok {
		t.Errorf("'bucket.yaml' not found among package resources")
	}
	node, err := yaml.Parse(bucket)
	if err != nil {
		t.Errorf("yaml.Parse(\"bucket.yaml\") failed: %v", err)
	}
	if got, want := node.GetNamespace(), "bucket-namespace"; got != want {
		t.Errorf("StorageBucket namespace: got %q, want %q", got, want)
	}
	annotations := node.GetAnnotations()
	if val, found := annotations["foo"]; !found || val != "bar" {
		t.Errorf("StorageBucket annotations should contain foo=bar, but got %v", annotations)
	}
}

func (t *PorchSuite) TestExecFunctionEvaluator() {
	const (
		repoName = "git-exec-fn-eval"
	)
	// Register the repository as 'git-fn'
	t.RegisterGitRepositoryF(t.GetPorchTestRepoURL(), repoName, "", suiteutils.GiteaUser, suiteutils.GiteaPassword)

	// Create Package Revision
	pr := t.CreatePackageCloneF(repoName, "test-fn-bucket", "test-workspace", defaultBucketBpRef, "bucket")

	// Get package resources
	resources := &porchapi.PackageRevisionResources{}
	t.GetF(client.ObjectKey{
		Namespace: t.Namespace,
		Name:      pr.Name,
	}, resources)

	t.AddMutator(resources, t.KrmFunctionsRegistry+"/starlark:v0.3.0", suiteutils.WithConfigmap(map[string]string{
		"source": `# set the namespace on all resources

for resource in ctx.resource_list["items"]:

	  # mutate the resource
	  resource["metadata"]["namespace"] = "bucket-namespace"`,
	}))
	t.AddMutator(resources, t.KrmFunctionsRegistry+"/set-annotations:v0.1.4", suiteutils.WithConfigmap(map[string]string{"foo": "bar"}))
	t.UpdateF(resources)

	bucket, ok := resources.Spec.Resources["bucket.yaml"]
	if !ok {
		t.Errorf("'bucket.yaml' not found among package resources")
	}
	node, err := yaml.Parse(bucket)
	if err != nil {
		t.Errorf("yaml.Parse(\"bucket.yaml\") failed: %v", err)
	}
	if got, want := node.GetNamespace(), "bucket-namespace"; got != want {
		t.Errorf("StorageBucket namespace: got %q, want %q", got, want)
	}
	annotations := node.GetAnnotations()
	if val, found := annotations["foo"]; !found || val != "bar" {
		t.Errorf("StorageBucket annotations should contain foo=bar, but got %v", annotations)
	}
}

func (t *PorchSuite) TestPodFunctionEvaluatorWithDistrolessImage() {
	if t.TestRunnerIsLocal {
		t.Skipf(skipPodEvaluatorMsg)
	}
	const (
		repoName = "git-fn-distroless"
	)

	t.RegisterGitRepositoryF(t.GetPorchTestRepoURL(), repoName, "", suiteutils.GiteaUser, suiteutils.GiteaPassword)

	// Create Package Revision
	pr := t.CreatePackageCloneF(repoName, "test-fn-redis-bucket", "test-description", "redis-bucket/v1", "redis-bucket")

	// Get package resources
	resources := &porchapi.PackageRevisionResources{}
	t.GetF(client.ObjectKey{
		Namespace: t.Namespace,
		Name:      pr.Name,
	}, resources)

	resources.Spec.Resources["configmap.yaml"] = `
apiVersion: v1
kind: ConfigMap
metadata:
  name: kptfile.kpt.dev
data:
  name: bucket-namespace
`

	t.AddMutator(resources, t.KrmFunctionsRegistry+"/set-namespace:v0.4.1", suiteutils.WithConfigPath("configmap.yaml"))
	t.UpdateF(resources)

	bucket, ok := resources.Spec.Resources["bucket.yaml"]
	if !ok {
		t.Errorf("'bucket.yaml' not found among package resources")
	}
	node, err := yaml.Parse(bucket)
	if err != nil {
		t.Errorf("yaml.Parse(\"bucket.yaml\") failed: %v", err)
	}
	if got, want := node.GetNamespace(), "bucket-namespace"; got != want {
		t.Errorf("StorageBucket namespace: got %q, want %q", got, want)
	}
}

func (t *PorchSuite) TestPodEvaluator() {
	if t.TestRunnerIsLocal {
		t.Skipf(skipPodEvaluatorMsg)
	}
	const repoName = "git-fn-pod-eval"

	setAnnotationsImage := t.KrmFunctionsRegistry + "/set-annotations:v0.1.4"
	t.RegisterGitRepositoryF(t.GetPorchTestRepoURL(), repoName, "", suiteutils.GiteaUser, suiteutils.GiteaPassword)

	// Create first package revision
	pr := t.CreatePackageCloneF(repoName, "test-fn-pod-bucket", "workspace-1", defaultBucketBpRef, "bucket")
	resources := t.WaitUntilPackageRevisionResourcesExists(types.NamespacedName{Namespace: t.Namespace, Name: pr.Name})
	t.AddMutator(resources, setAnnotationsImage, suiteutils.WithConfigmap(map[string]string{"test-key": "test-val"}))
	t.UpdateF(resources)

	// Wait for the correct pod to be created and capture suffix
	var firstPodSuffix string
	err := wait.PollUntilContextTimeout(t.GetContext(), time.Second, 30*time.Second, false, func(ctx context.Context) (bool, error) {
		podList := &corev1.PodList{}
		if err := t.Client.List(ctx, podList, client.InNamespace("porch-fn-system")); err != nil {
			return false, err
		}
		for _, pod := range podList.Items {
			if pod.Spec.Containers[0].Image == setAnnotationsImage {
				parts := strings.Split(pod.Name, "-")
				if len(parts) > 0 {
					firstPodSuffix = parts[len(parts)-1]
				}
				t.Logf("Found matching pod %s, captured suffix: %s", pod.Name, firstPodSuffix)
				t.DeleteF(&pod)
				return true, nil
			}
		}
		return false, nil
	})
	if err != nil {
		t.Fatalf("Failed to find pod with image %s: %v", setAnnotationsImage, err)
	}

	// Create second package revision
	pr2 := t.CreatePackageCloneF(repoName, "test-fn-pod-bucket-2", "workspace-2", defaultBucketBpRef, "bucket")
	resources = t.WaitUntilPackageRevisionResourcesExists(types.NamespacedName{Namespace: t.Namespace, Name: pr2.Name})

	// Testing pod evaluator with golang function
	t.AddMutator(resources, setAnnotationsImage, suiteutils.WithConfigmap(map[string]string{"new-test-key": "new-test-val"}))
	t.UpdateF(resources)

	// Verify second pod has different suffix
	podList := &corev1.PodList{}
	t.ListF(podList, client.InNamespace("porch-fn-system"))
	var secondPodSuffix string
	for _, pod := range podList.Items {
		if pod.Spec.Containers[0].Image == setAnnotationsImage {
			parts := strings.Split(pod.Name, "-")
			if len(parts) > 0 {
				secondPodSuffix = parts[len(parts)-1]
			}
		}
	}
	if firstPodSuffix == secondPodSuffix {
		t.Errorf("Expected different fn pod suffixes for pr and pr2, but both had: %s", firstPodSuffix)
	} else {
		t.Logf("Successfully verified different fn pods: first=%s, second=%s", firstPodSuffix, secondPodSuffix)
	}
}

func (t *PorchSuite) TestPodEvaluatorWithFailure() {
	if t.TestRunnerIsLocal {
		t.Skipf("Skipping due to not having pod evaluator in local mode")
	}
	const (
		repoName = "git-pod-eval-fn-failure"
	)

	t.RegisterGitRepositoryF(t.GetPorchTestRepoURL(), repoName, "", suiteutils.GiteaUser, suiteutils.GiteaPassword)

	// Create Package Revision
	pr := t.CreatePackageCloneF(repoName, "test-fn-pod-bucket", defaultWorkspace, defaultBucketBpRef, "bucket")

	resources := t.WaitUntilPackageRevisionResourcesExists(types.NamespacedName{Namespace: t.Namespace, Name: pr.Name})

	// Add invalid resource to make kubeconform fail
	resources.Spec.Resources["invalid.yaml"] = `apiVersion: v1
kind: Pod
metadata:
  name: invalid-resource
spec:
  replicas: "invalid"`

	t.AddMutator(resources, t.KrmFunctionsRegistry+"/kubeconform:v0.1.1")

	err := t.Client.Update(t.GetContext(), resources)
	if err == nil {
		t.Fatalf("expected error but got none")
	}
}

func (t *PorchSuite) TestFailedPodEvictionAndRecovery() {
	if t.TestRunnerIsLocal {
		t.Skipf("Skipping test: evaluator pod not available in local mode")
	}
	const (
		repoName = "git-fn-failed-recovery"
	)
	t.RegisterGitRepositoryF(t.GetPorchTestRepoURL(), repoName, "", suiteutils.GiteaUser, suiteutils.GiteaPassword)

	// Define a bogus kpt function image that will fail (image doesn't exist)
	bogusFnImage := "quay.io/invalid/kpt-fn-broken:v0.0.1"

	// Create a PackageRevision
	pr := t.CreatePackageCloneF(repoName, "test-fn-pod-eviction", defaultWorkspace, defaultBucketBpRef, "bucket")

	prr := &porchapi.PackageRevisionResources{}
	t.GetF(client.ObjectKeyFromObject(pr), prr)

	// Add an eval task that will fail
	t.AddMutator(prr, bogusFnImage)

	err := t.Client.Update(t.GetContext(), prr)

	// Assert: creation should fail, and the error should reflect evaluator pod failure
	t.Require().ErrorContains(err, "Error rendering package in kpt function pipeline")

	// Optional: verify no stuck pods exist for the failed image
	pods := &corev1.PodList{}
	t.ListF(pods, client.InNamespace(t.Namespace))

	for _, pod := range pods.Items {
		if strings.Contains(pod.Spec.Containers[0].Image, "kpt-fn-broken") {
			if pod.Status.Phase == corev1.PodFailed {
				t.Logf("Found pod %s in Failed state (expected, cleanup should follow)", pod.Name)
			} else {
				t.Logf("Found pod %s in %s state", pod.Name, pod.Status.Phase)
			}
		}
	}
}