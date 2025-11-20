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

package fn_runner

import (
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	porchapi "github.com/nephio-project/porch/api/porch/v1alpha1"
	"github.com/nephio-project/porch/test/e2e"
	"github.com/nephio-project/porch/third_party/kptdev/krm-functions-sdk/go/fn"
	"github.com/stretchr/testify/suite"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

type FunctionRunnerSuite struct {
	e2e.TestSuiteWithGit
}

func TestE2E(t *testing.T) {
	// https://github.com/nephio-project/porch/pull/256
	// Updating 3rd party dependencies may break existing kpt functions because of api incompatibility. 
	// This suite goes though all the used base kpt functions.
	if os.Getenv("TPP") == "" {
		t.Skip("set TPP to run this test")
	}

	suite.Run(t, &FunctionRunnerSuite{})
}

func (t *FunctionRunnerSuite) TestApplySetters() {
	testCases := map[string]struct {
		image string
	}{
		"apply-setter:v0.1.1": {
			image: t.TestSuiteWithGit.GcrPrefix + "/apply-setters:v0.1.1",
		},
		"apply-setter:v0.2": {
			image: t.TestSuiteWithGit.GcrPrefix + "/apply-setters:v0.2",
		},
		"apply-setter:v0.2.2": {
			image: t.TestSuiteWithGit.GcrPrefix + "/apply-setters:v0.2.2",
		},
	}

	t.RegisterMainGitRepositoryF("test-apply-setters")

	for tn, tc := range testCases {

		t.Run(tn, func() {
			pr := t.createEmptyPackageRevision("test-apply-setters")
			t.CreateF(&pr)
			t.Cleanup(func() {
				t.doCleanup(&pr, tc.image)
			})

			// Get package resources
			var resources = t.WaitUntilPackageRevisionResourcesExists(types.NamespacedName{Namespace: t.Namespace, Name: pr.Name})

			t.AddResourceToPackage(resources, "../testdata/resources-for-krm-functions/project.yaml", "project.yaml")

			t.AddMutator(resources, tc.image, e2e.WithConfigmap(map[string]string{
				"projects-namespace": "updated-projects",
			}))

			t.UpdateF(resources)
			t.failOnRenderError(resources)

			for name, obj := range resources.Spec.Resources {
				if strings.HasPrefix(name, "project") {
					node, err := yaml.Parse(obj)
					if err != nil {
						t.Errorf("failed to parse Folder object: %v", err)
					}
					namespace := node.GetNamespace()
					if namespace != "updated-projects" {
						t.Errorf("Project should contain namespace `updated-projects`, the namespace we got: %v", namespace)
					}
				}
			}
		})

	}
}

func (t *FunctionRunnerSuite) TestSetNamespace() {
	testCases := map[string]struct {
		image string
	}{
		"set-namespace:v0.2.0": {
			image: t.TestSuiteWithGit.GcrPrefix + "/set-namespace:v0.2.0",
		},
		"set-namespace:v0.3.4": {
			image: t.TestSuiteWithGit.GcrPrefix + "/set-namespace:v0.3.4",
		},
		"set-namespace:v0.4.1": {
			image: t.TestSuiteWithGit.GcrPrefix + "/set-namespace:v0.4.1",
		},
	}

	t.RegisterMainGitRepositoryF("test-set-namespace")
	for tn, tc := range testCases {

		t.Run(tn, func() {
			pr := t.createEmptyPackageRevision("test-set-namespace")
			t.CreateF(&pr)
			t.Cleanup(func() {
				t.doCleanup(&pr, tc.image)
			})

			// Get package resources
			var resources = t.WaitUntilPackageRevisionResourcesExists(types.NamespacedName{Namespace: t.Namespace, Name: pr.Name})

			t.AddResourceToPackage(resources, "../testdata/resources-for-krm-functions/bucket.yaml", "bucket.yaml")

			t.AddMutator(resources, tc.image, e2e.WithConfigmap(map[string]string{
				"namespace": "updated-namespace",
			}))

			t.UpdateF(resources)
			t.failOnRenderError(resources)

			bucket, ok := resources.Spec.Resources["bucket.yaml"]
			if !ok {
				t.Errorf("'bucket.yaml' not found among package resources")
			}

			node, err := yaml.Parse(bucket)
			if err != nil {
				t.Errorf("yaml.Parse(\"bucket.yaml\") failed: %v", err)
			}
			namespace := node.GetNamespace()
			if namespace != "updated-namespace" {
				t.Errorf("Project should contain namespace `updated-namespace`, the namespace we got: %v", namespace)
			}
		})

	}
}

func (t *FunctionRunnerSuite) TestSetLabels() {
	testCases := map[string]struct {
		image string
	}{
		"set-labels:v0.1.5": {
			image: t.TestSuiteWithGit.GcrPrefix + "/set-labels:v0.1.5",
		},
		"set-labels:v0.2.0": {
			image: t.TestSuiteWithGit.GcrPrefix + "/set-labels:v0.2.0",
		},
	}

	t.RegisterMainGitRepositoryF("test-set-labels")
	for tn, tc := range testCases {

		t.Run(tn, func() {
			pr := t.createEmptyPackageRevision("test-set-labels")
			t.CreateF(&pr)
			t.Cleanup(func() {
				t.doCleanup(&pr, tc.image)
			})

			// Get package resources
			var resources = t.WaitUntilPackageRevisionResourcesExists(types.NamespacedName{Namespace: t.Namespace, Name: pr.Name})

			t.AddResourceToPackage(resources, "../testdata/resources-for-krm-functions/daemonset.yaml", "daemonset.yaml")

			t.AddMutator(resources, tc.image, e2e.WithConfigmap(map[string]string{
				"app": "updated-cloud-sql-auth-proxy",
			}))

			t.UpdateF(resources)
			t.failOnRenderError(resources)

			// Get package resources again
			resources = t.WaitUntilPackageRevisionResourcesExists(types.NamespacedName{Namespace: t.Namespace, Name: pr.Name})

			daemonset, ok := resources.Spec.Resources["daemonset.yaml"]
			if !ok {
				t.Errorf("'daemonset.yaml' not found among package resources")
			}

			node, err := yaml.Parse(daemonset)
			if err != nil {
				t.Errorf("yaml.Parse(\"daemonset.yaml\") failed: %v", err)
			}
			labels := node.GetLabels()
			if labels["app"] != "updated-cloud-sql-auth-proxy" {
				t.Errorf("Project should contain label `app: updated-cloud-sql-auth-proxy`, the labels we got: %v", labels)
			}
		})

	}
}

func (t *FunctionRunnerSuite) TestSetAnnotations() {
	testCases := map[string]struct {
		image string
	}{
		"set-annotations:v0.1.4": {
			image: t.TestSuiteWithGit.GcrPrefix + "/set-annotations:v0.1.4",
		},
	}

	t.RegisterMainGitRepositoryF("test-set-annotations")
	for tn, tc := range testCases {

		t.Run(tn, func() {
			pr := t.createEmptyPackageRevision("test-set-annotations")
			t.CreateF(&pr)
			t.Cleanup(func() {
				t.doCleanup(&pr, tc.image)
			})

			// Get package resources
			var resources = t.WaitUntilPackageRevisionResourcesExists(types.NamespacedName{Namespace: t.Namespace, Name: pr.Name})

			t.AddResourceToPackage(resources, "../testdata/resources-for-krm-functions/daemonset.yaml", "daemonset.yaml")

			t.AddMutator(resources, tc.image, e2e.WithConfigmap(map[string]string{
				"cnrm.cloud.google.com/blueprint": "updated-cnrm/sql/auth-proxy/v0.2.0",
			}))

			t.UpdateF(resources)
			t.failOnRenderError(resources)

			daemonset, ok := resources.Spec.Resources["daemonset.yaml"]
			if !ok {
				t.Errorf("'daemonset.yaml' not found among package resources")
			}

			node, err := yaml.Parse(daemonset)
			if err != nil {
				t.Errorf("yaml.Parse(\"daemonset.yaml\") failed: %v", err)
			}
			annotations := node.GetAnnotations()
			if val, found := annotations["cnrm.cloud.google.com/blueprint"]; !found || val != "updated-cnrm/sql/auth-proxy/v0.2.0" {
				t.Errorf("Project should contain annotation `cnrm.cloud.google.com/blueprint: updated-cloud-sql-auth-proxy`, the annotations we got: %v", annotations)
			}
		})

	}
}

func (t *FunctionRunnerSuite) TestSearchReplace() {
	testCases := map[string]struct {
		image string
	}{
		"search-replace:v0.2.0": {
			image: t.TestSuiteWithGit.GcrPrefix + "/search-replace:v0.2.0",
		},
	}

	t.RegisterMainGitRepositoryF("test-search-replace")
	for tn, tc := range testCases {

		t.Run(tn, func() {
			pr := t.createEmptyPackageRevision("test-search-replace")
			t.CreateF(&pr)
			t.Cleanup(func() {
				t.doCleanup(&pr, tc.image)
			})

			// Get package resources
			var resources = t.WaitUntilPackageRevisionResourcesExists(types.NamespacedName{Namespace: t.Namespace, Name: pr.Name})

			t.AddResourceToPackage(resources, "../testdata/resources-for-krm-functions/service.yaml", "service.yaml")

			t.AddMutator(resources, tc.image, e2e.WithConfigmap(map[string]string{
				"by-value":  "cloud-sql-auth-proxy",
				"put-value": "updated-cloud-sql-auth-proxy",
			}))

			t.UpdateF(resources)
			t.failOnRenderError(resources)

			service, ok := resources.Spec.Resources["service.yaml"]
			if !ok {
				t.Errorf("'service.yaml' not found among package resources")
			}

			node, err := yaml.Parse(service)
			if err != nil {
				t.Errorf("yaml.Parse(\"service.yaml\") failed: %v", err)
			}
			selector_value, err := node.GetFieldValue("spec.selector.app")
			if err != nil {
				t.Errorf("failed to parse external object: %v", err)
			}
			if selector_value != "updated-cloud-sql-auth-proxy" {
				t.Errorf("Project should contain selector.app with value `updated-cloud-sql-auth-proxy`, the value we got: %v", selector_value)
			}
		})

	}
}

func (t *FunctionRunnerSuite) TestStarlark() {
	testCases := map[string]struct {
		image string
	}{
		"starlark:v0.3.0": {
			image: t.TestSuiteWithGit.GcrPrefix + "/starlark:v0.3.0",
		},
		"starlark:v0.4.3": {
			image: t.TestSuiteWithGit.GcrPrefix + "/starlark:v0.4.3",
		},
		"starlark:v0.5.0": {
			image: t.TestSuiteWithGit.GcrPrefix + "/starlark:v0.5.0",
		},
	}

	t.RegisterMainGitRepositoryF("test-starlark")
	for tn, tc := range testCases {

		t.Run(tn, func() {
			pr := t.createEmptyPackageRevision("test-starlark")
			t.CreateF(&pr)
			t.Cleanup(func() {
				t.doCleanup(&pr, tc.image)
			})

			// Get package resources
			var resources = t.WaitUntilPackageRevisionResourcesExists(types.NamespacedName{Namespace: t.Namespace, Name: pr.Name})

			t.AddResourceToPackage(resources, "../testdata/resources-for-krm-functions/bucket.yaml", "bucket.yaml")

			t.AddMutator(resources, tc.image, e2e.WithConfigmap(map[string]string{
				"source": `for resource in ctx.resource_list["items"]:
  resource["metadata"]["annotations"]["foo"] = "bar"`,
			}))

			t.UpdateF(resources)
			t.failOnRenderError(resources)

			bucket, ok := resources.Spec.Resources["bucket.yaml"]
			if !ok {
				t.Errorf("'bucket.yaml' not found among package resources")
			}

			node, err := yaml.Parse(bucket)
			if err != nil {
				t.Errorf("yaml.Parse(\"bucket.yaml\") failed: %v", err)
			}
			annotations := node.GetAnnotations()
			if val, found := annotations["foo"]; !found || val != "bar" {
				t.Errorf("StorageBucket annotations should contain foo=bar, but got %v", annotations)
			}
		})

	}
}

func (t *FunctionRunnerSuite) TestEnsureNameSubstring() {
	testCases := map[string]struct {
		image string
	}{
		"ensure-name-substring:v0.1.1": {
			image: t.TestSuiteWithGit.GcrPrefix + "/ensure-name-substring:v0.1.1",
		},
		"ensure-name-substring:v0.2.0": {
			image: t.TestSuiteWithGit.GcrPrefix + "/ensure-name-substring:v0.2.0",
		},
	}

	t.RegisterMainGitRepositoryF("test-ensure-name-substring")
	for tn, tc := range testCases {

		t.Run(tn, func() {
			pr := t.createEmptyPackageRevision("test-ensure-name-substring")
			t.CreateF(&pr)
			t.Cleanup(func() {
				t.doCleanup(&pr, tc.image)
			})

			// Get package resources
			var resources = t.WaitUntilPackageRevisionResourcesExists(types.NamespacedName{Namespace: t.Namespace, Name: pr.Name})

			t.AddResourceToPackage(resources, "../testdata/resources-for-krm-functions/service.yaml", "service.yaml")

			t.AddMutator(resources, tc.image, e2e.WithConfigmap(map[string]string{
				"append": "-test",
			}))

			t.UpdateF(resources)
			t.failOnRenderError(resources)

			service := resources.Spec.Resources["service.yaml"]

			node, err := yaml.Parse(service)
			if err != nil {
				t.Errorf("yaml.Parse(\"service.yaml\") failed: %v", err)
			}
			resourceName := node.GetName()
			expectedResourceName := "cloud-sql-auth-proxy-test"
			if resourceName != expectedResourceName {
				t.Errorf("Project should contain selector.app with value `%s`, the value we got: %s", expectedResourceName, resourceName)
			}
		})

	}
}

func (t *FunctionRunnerSuite) TestGenerateFolders() {
	testCases := map[string]struct {
		image string
	}{
		"generate-folders:v0.1.1": {
			image: t.TestSuiteWithGit.GcrPrefix + "/generate-folders:v0.1.1",
		},
	}

	t.RegisterMainGitRepositoryF("test-generate-folders")
	for tn, tc := range testCases {

		t.Run(tn, func() {
			pr := t.createEmptyPackageRevision("test-generate-folders")
			t.CreateF(&pr)
			t.Cleanup(func() {
				t.doCleanup(&pr, tc.image)
			})

			// Get package resources
			var resources = t.WaitUntilPackageRevisionResourcesExists(types.NamespacedName{Namespace: t.Namespace, Name: pr.Name})

			t.AddResourceToPackage(resources, "../testdata/resources-for-krm-functions/folder-hiearchy.yaml", "folder-hiearchy.yaml")

			t.AddMutator(resources, tc.image)

			t.UpdateF(resources)
			t.failOnRenderError(resources)

			counter := 0
			for name := range resources.Spec.Resources {
				if strings.HasPrefix(name, "hierarchy/") {
					counter++
				}
			}
			if counter != 4 {
				t.Errorf("expected 4 Folder objects, but got %v", counter)
			}
		})

	}
}

func (t *FunctionRunnerSuite) TestSetImage() {
	testCases := map[string]struct {
		image string
	}{
		"set-image:v0.1.1": {
			image: t.TestSuiteWithGit.GcrPrefix + "/set-image:v0.1.1",
		},
	}

	t.RegisterMainGitRepositoryF("test-set-image")
	for tn, tc := range testCases {

		t.Run(tn, func() {
			pr := t.createEmptyPackageRevision("test-set-image")
			t.CreateF(&pr)
			t.Cleanup(func() {
				t.doCleanup(&pr, tc.image)
			})

			// Get package resources
			var resources = t.WaitUntilPackageRevisionResourcesExists(types.NamespacedName{Namespace: t.Namespace, Name: pr.Name})

			t.AddResourceToPackage(resources, "../testdata/resources-for-krm-functions/daemonset.yaml", "daemonset.yaml")

			t.AddMutator(resources, tc.image, e2e.WithConfigmap(map[string]string{
				"name":    "gcr.io/cloud-sql-connectors/cloud-sql-proxy",
				"newName": "bitnami/nginx-updated",
				"newTag":  "1.22.0",
			}))

			t.UpdateF(resources)
			t.failOnRenderError(resources)

			daemonset, ok := resources.Spec.Resources["daemonset.yaml"]
			if !ok {
				t.Errorf("'daemonset.yaml' not found among package resources")
			}

			node, err := yaml.Parse(daemonset)
			if err != nil {
				t.Errorf("yaml.Parse(\"daemonset.yaml\") failed: %v", err)
			}
			containerNode, err := node.Pipe(yaml.Lookup("spec", "template", "spec", "containers"))
			if err != nil {
				t.Errorf("failed to parse containers: %v", err)
			}
			containers, err := containerNode.Elements()
			if err != nil {
				t.Errorf("failed to get elements: %v", err)
			}
			imageNode, err := containers[0].Pipe(yaml.Lookup("image"))
			if err != nil {
				t.Errorf("failed to parse image node: %v", err)
			}
			imageName, err := imageNode.String()
			if err != nil {
				t.Errorf("failed to parse image name: %v", err)
			}
			expectedName := "bitnami/nginx-updated:1.22.0"
			if strings.TrimSpace(imageName) != expectedName {
				t.Errorf("Daemonset should contain image with value `%s`, the value we got: %s", expectedName, imageName)
			}
		})

	}
}

func (t *FunctionRunnerSuite) TestApplyReplacements() {
	testCases := map[string]struct {
		image string
	}{
		"apply-replacements:v0.1.1": {
			image: t.TestSuiteWithGit.GcrPrefix + "/apply-replacements:v0.1.1",
		},
	}

	t.RegisterMainGitRepositoryF("test-apply-replacements")
	for tn, tc := range testCases {

		t.Run(tn, func() {
			pr := t.createEmptyPackageRevision("test-apply-replacements")
			t.CreateF(&pr)
			t.Cleanup(func() {
				t.doCleanup(&pr, tc.image)
			})

			// Get package resources
			var resources = t.WaitUntilPackageRevisionResourcesExists(types.NamespacedName{Namespace: t.Namespace, Name: pr.Name})

			t.AddResourceToPackage(resources, "../testdata/resources-for-krm-functions/applyreplacement/job.yaml", "job.yaml")
			t.AddResourceToPackage(resources, "../testdata/resources-for-krm-functions/applyreplacement/resources.yaml", "resources.yaml")
			t.AddResourceToPackage(resources, "../testdata/resources-for-krm-functions/applyreplacement/applyreplacement.yaml", "applyreplacement.yaml")

			t.AddMutator(resources, tc.image, e2e.WithConfigPath("applyreplacement.yaml"))

			t.UpdateF(resources)
			t.failOnRenderError(resources)

			job, ok := resources.Spec.Resources["job.yaml"]
			if !ok {
				t.Errorf("'job.yaml' not found among package resources")
			}

			node, err := yaml.Parse(job)
			if err != nil {
				t.Errorf("yaml.Parse(\"job.yaml\") failed: %v", err)
			}
			restartPolicy, err := node.GetFieldValue("spec.template.spec.restartPolicy")
			if err != nil {
				t.Errorf("Cannot get the restartPolicy field: %v", err)
			}
			if restartPolicy == nil {
				t.Errorf("Job should contain the restartPolicy field!")
			}
		})

	}
}

func (t *FunctionRunnerSuite) TestCreateSetters() {
	testCases := map[string]struct {
		image string
	}{
		"create-setters:v0.1.0": {
			image: t.TestSuiteWithGit.GcrPrefix + "/create-setters:v0.1.0",
		},
	}

	t.RegisterMainGitRepositoryF("test-create-setters")
	for tn, tc := range testCases {

		t.Run(tn, func() {
			pr := t.createEmptyPackageRevision("test-create-setters")
			t.CreateF(&pr)
			t.Cleanup(func() {
				t.doCleanup(&pr, tc.image)
			})

			// Get package resources
			var resources = t.WaitUntilPackageRevisionResourcesExists(types.NamespacedName{Namespace: t.Namespace, Name: pr.Name})

			t.AddResourceToPackage(resources, "../testdata/resources-for-krm-functions/createsetters/setters.yaml", "setters.yaml")
			t.AddResourceToPackage(resources, "../testdata/resources-for-krm-functions/createsetters/resources.yaml", "resources.yaml")

			t.AddMutator(resources, tc.image, e2e.WithConfigPath("setters.yaml"))

			t.AddMutator(resources, t.TestSuiteWithGit.GcrPrefix+"/apply-setters:v0.2.0", e2e.WithConfigmap(map[string]string{
				"nginx-replicas": "5",
			}))

			t.UpdateF(resources)
			t.failOnRenderError(resources)

			packageResources, ok := resources.Spec.Resources["resources.yaml"]
			if !ok {
				t.Errorf("'resources.yaml' not found among package resources")
			}

			node, err := yaml.Parse(packageResources)
			if err != nil {
				t.Errorf("yaml.Parse(\"resources.yaml\") failed: %v", err)
			}

			replicas, err := node.GetFieldValue("spec.replicas")
			if err != nil {
				t.Errorf("Cannot get replicas field: %v", err)
			}
			expectedReplicas := 5
			if replicas != expectedReplicas {
				t.Errorf("Deployment should contain replicas with value `%s`, the value we got: %s", expectedReplicas, replicas)
			}
		})

	}
}

func (t *FunctionRunnerSuite) TestEnableGcpServices() {
	testCases := map[string]struct {
		image string
	}{
		"enable-gcp-services:v0.1.0": {
			image: t.TestSuiteWithGit.GcrPrefix + "/enable-gcp-services:v0.1.0",
		},
	}

	t.RegisterMainGitRepositoryF("test-enable-gcp-services")
	for tn, tc := range testCases {

		t.Run(tn, func() {
			pr := t.createEmptyPackageRevision("test-enable-gcp-services")
			t.CreateF(&pr)
			t.Cleanup(func() {
				t.doCleanup(&pr, tc.image)
			})

			// Get package resources
			var resources = t.WaitUntilPackageRevisionResourcesExists(types.NamespacedName{Namespace: t.Namespace, Name: pr.Name})

			t.AddResourceToPackage(resources, "../testdata/resources-for-krm-functions/gcp-services.yaml", "gcp-services.yaml")

			t.AddMutator(resources, tc.image)

			t.UpdateF(resources)
			t.failOnRenderError(resources)

			keys := make([]string, 0, len(resources.Spec.Resources))
			for k := range resources.Spec.Resources {
				keys = append(keys, k)
			}
			expectedResource := "service_proj1-service-compute.yaml"
			if !slices.Contains(keys, expectedResource) {
				t.Errorf("Package should contain `%s`, but not found.", expectedResource)
			}
		})

	}
}

// Utility functions

func (t *FunctionRunnerSuite) createEmptyPackageRevision(repo string) porchapi.PackageRevision {
	workspace, err := uuid.NewRandom()
	if err != nil {
		t.Fatalf("failed to create workspace UUID: %v", err)
	}
	pr := porchapi.PackageRevision{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: t.Namespace,
		},
		Spec: porchapi.PackageRevisionSpec{
			PackageName:    "test-fn-pod",
			WorkspaceName:  workspace.String(),
			RepositoryName: repo,
			Tasks: []porchapi.Task{
				{
					Type: "clone",
					Clone: &porchapi.PackageCloneTaskSpec{
						Upstream: porchapi.UpstreamPackage{
							Type: "git",
							Git: &porchapi.GitPackage{
								Repo:      t.TestSuiteWithGit.GcpBlueprintsRepo,
								Ref:       t.TestSuiteWithGit.KptFunctionRef,
								Directory: "catalog/empty",
								SecretRef: porchapi.SecretRef{
									Name: t.CreateGcpPackageRevisionSecret("test-fn-pod"),
								},
							},
						},
					},
				},
			},
		},
	}
	return pr
}

func (t *FunctionRunnerSuite) failOnRenderError(resources *porchapi.PackageRevisionResources) {
	if resources.Status.RenderStatus.Err != "" {
		t.Fatalf("failed to render package: %v", resources.Status.RenderStatus.Err)
	}
	for _, result := range resources.Status.RenderStatus.Result.Items {
		if result.ExitCode != 0 {
			t.Fatalf("failed to render package: non-zero exit code for %v", result.Image)
		}
		for _, resultItem := range result.Results {
			if resultItem.Severity == string(fn.Error) {
				t.Fatalf("failed to render package: error in %v: %v", result.Image, resultItem.Message)
			}
		}
	}
}

func (t *FunctionRunnerSuite) doCleanup(pr *porchapi.PackageRevision, mutatorImage string) {
	t.DeleteF(pr)
	t.WaitUntilObjectDeleted(
		porchapi.SchemeGroupVersion.WithKind("PackageRevision"),
		types.NamespacedName{
			Name:      pr.Name,
			Namespace: pr.Namespace,
		},
		10*time.Second,
	)

	podList := &corev1.PodList{}
	t.ListF(podList, client.InNamespace("porch-fn-system"))
	for _, pod := range podList.Items {
		img := pod.Spec.Containers[0].Image
		if img == mutatorImage {
			t.DeleteF(&pod)
			time.Sleep(1 * time.Second)
		}
	}
}
