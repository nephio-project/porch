// Copyright 2024 The kpt and Nephio Authors
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
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	porchapi "github.com/nephio-project/porch/api/porch/v1alpha1"
	configapi "github.com/nephio-project/porch/api/porchconfig/v1alpha1"
	kptfilev1 "github.com/nephio-project/porch/pkg/kpt/api/kptfile/v1"
	"github.com/nephio-project/porch/pkg/repository"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/types"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

const (
	testBlueprintsRepo = "https://github.com/platkrm/test-blueprints.git"
	kptRepo            = "https://github.com/kptdev/kpt.git"
)

var (
	packageRevisionGVK = porchapi.SchemeGroupVersion.WithKind("PackageRevision")
	configMapGVK       = corev1.SchemeGroupVersion.WithKind("ConfigMap")
)

type PorchSuite struct {
	TestSuiteWithGit
}

var _ TSetter = &PorchSuite{}
var _ Initializer = &PorchSuite{}

func TestE2E(t *testing.T) {
	e2e := os.Getenv("E2E")
	if e2e == "" {
		t.Skip("set E2E to run this test")
	}

	RunSuite(&PorchSuite{}, t)
}

func (t *PorchSuite) TestGitRepository(ctx context.Context) {
	// Register the repository as 'git'
	t.RegisterMainGitRepositoryF(ctx, "git")

	// Create Package Revision
	pr := &porchapi.PackageRevision{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: t.Namespace,
		},
		Spec: porchapi.PackageRevisionSpec{
			PackageName:    "test-bucket",
			WorkspaceName:  "workspace",
			RepositoryName: "git",
			Tasks: []porchapi.Task{
				{
					Type: "clone",
					Clone: &porchapi.PackageCloneTaskSpec{
						Upstream: porchapi.UpstreamPackage{
							Type: "git",
							Git: &porchapi.GitPackage{
								Repo:      "https://github.com/GoogleCloudPlatform/blueprints.git",
								Ref:       "bucket-blueprint-v0.4.3",
								Directory: "catalog/bucket",
							},
						},
					},
				},
				{
					Type: "eval",
					Eval: &porchapi.FunctionEvalTaskSpec{
						Image: "gcr.io/kpt-fn/set-namespace:v0.4.1",
						ConfigMap: map[string]string{
							"namespace": "bucket-namespace",
						},
					},
				},
			},
		},
	}
	t.CreateF(ctx, pr)

	// Get package resources
	var resources porchapi.PackageRevisionResources
	t.GetF(ctx, client.ObjectKey{
		Namespace: t.Namespace,
		Name:      pr.Name,
	}, &resources)

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

func (t *PorchSuite) TestGitRepositoryWithReleaseTagsAndDirectory(ctx context.Context) {
	t.RegisterGitRepositoryF(ctx, kptRepo, "kpt-repo", "package-examples")

	t.Log("Listing PackageRevisions in " + t.Namespace)
	var list porchapi.PackageRevisionList
	t.ListF(ctx, &list, client.InNamespace(t.Namespace))

	for _, pr := range list.Items {
		if strings.HasPrefix(pr.Spec.PackageName, "package-examples") {
			t.Errorf("package name %q should not include repo directory %q as prefix", pr.Spec.PackageName, "package-examples")
		}
	}
}

func (t *PorchSuite) TestCloneFromUpstream(ctx context.Context) {
	// Register Upstream Repository
	t.RegisterGitRepositoryF(ctx, testBlueprintsRepo, "test-blueprints", "")

	var list porchapi.PackageRevisionList
	t.ListE(ctx, &list, client.InNamespace(t.Namespace))

	upstreamPr := MustFindPackageRevision(t.T, &list, repository.PackageRevisionKey{Repository: "test-blueprints", Package: "basens", Revision: "v1"})

	// Register the repository as 'downstream'
	t.RegisterMainGitRepositoryF(ctx, "downstream")

	// Create PackageRevision from upstream repo
	clonedPr := &porchapi.PackageRevision{
		TypeMeta: metav1.TypeMeta{
			Kind:       "PackageRevision",
			APIVersion: porchapi.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: t.Namespace,
		},
		Spec: porchapi.PackageRevisionSpec{
			PackageName:    "istions",
			WorkspaceName:  "test-workspace",
			RepositoryName: "downstream",
			Tasks: []porchapi.Task{
				{
					Type: porchapi.TaskTypeClone,
					Clone: &porchapi.PackageCloneTaskSpec{
						Upstream: porchapi.UpstreamPackage{
							UpstreamRef: &porchapi.PackageRevisionRef{
								Name: upstreamPr.Name,
							},
						},
					},
				},
			},
		},
	}

	t.CreateF(ctx, clonedPr)

	// Get istions resources
	var istions porchapi.PackageRevisionResources
	t.GetF(ctx, client.ObjectKey{
		Namespace: t.Namespace,
		Name:      clonedPr.Name,
	}, &istions)

	kptfile := t.ParseKptfileF(&istions)

	if got, want := kptfile.Name, "istions"; got != want {
		t.Errorf("istions package Kptfile.metadata.name: got %q, want %q", got, want)
	}
	if kptfile.UpstreamLock == nil {
		t.Fatalf("istions package upstreamLock is missing")
	}
	if kptfile.UpstreamLock.Git == nil {
		t.Errorf("istions package upstreamLock.git is missing")
	}
	if kptfile.UpstreamLock.Git.Commit == "" {
		t.Errorf("istions package upstreamLock.git.commit is missing")
	}

	// Remove commit from comparison
	got := kptfile.UpstreamLock
	got.Git.Commit = ""

	want := &kptfilev1.UpstreamLock{
		Type: kptfilev1.GitOrigin,
		Git: &kptfilev1.GitLock{
			Repo:      testBlueprintsRepo,
			Directory: "basens",
			Ref:       "basens/v1",
		},
	}
	if !cmp.Equal(want, got) {
		t.Errorf("unexpected upstreamlock returned (-want, +got) %s", cmp.Diff(want, got))
	}

	// Check Upstream
	if got, want := kptfile.Upstream, (&kptfilev1.Upstream{
		Type: kptfilev1.GitOrigin,
		Git: &kptfilev1.Git{
			Repo:      testBlueprintsRepo,
			Directory: "basens",
			Ref:       "basens/v1",
		},
	}); !cmp.Equal(want, got) {
		t.Errorf("unexpected upstream returned (-want, +got) %s", cmp.Diff(want, got))
	}
}

func (t *PorchSuite) TestConcurrentClones(ctx context.Context) {
	const (
		upstreamRepository   = "upstream"
		upstreamPackage      = "basens"
		downstreamRepository = "downstream"
		downstreamPackage    = "istions-concurrent"
		workspace            = "test-workspace"
	)

	// Register Upstream and Downstream Repositories
	t.RegisterGitRepositoryF(ctx, testBlueprintsRepo, upstreamRepository, "")
	t.RegisterMainGitRepositoryF(ctx, downstreamRepository)

	var list porchapi.PackageRevisionList
	t.ListE(ctx, &list, client.InNamespace(t.Namespace))
	upstreamPr := MustFindPackageRevision(t.T,
		&list,
		repository.PackageRevisionKey{
			Repository: upstreamRepository,
			Package:    upstreamPackage,
			Revision:   "v1"})

	// Create PackageRevision from upstream repo
	clonedPr := t.CreatePackageSkeleton(downstreamRepository, downstreamPackage, workspace)
	clonedPr.Spec.Tasks = []porchapi.Task{
		{
			Type: porchapi.TaskTypeClone,
			Clone: &porchapi.PackageCloneTaskSpec{
				Upstream: porchapi.UpstreamPackage{
					UpstreamRef: &porchapi.PackageRevisionRef{
						Name: upstreamPr.Name,
					},
				},
			},
		},
	}

	// Two clients at the same time try to run the Create operation for the clone
	cloneFunction := func() any {
		return t.Client.Create(ctx, clonedPr)
	}
	results := RunInParallel(cloneFunction, cloneFunction)

	assert.Contains(t, results, nil, "expected one request to succeed, but did not happen - results: %v", results)

	conflictFailurePresent := slices.ContainsFunc(results, func(eachResult any) bool {
		return eachResult != nil && strings.Contains(eachResult.(error).Error(), "another request is already in progress")
	})
	assert.True(t, conflictFailurePresent, "expected one request to fail with a conflict, but did not happen - results: %v", results)
}

func (t *PorchSuite) TestInitEmptyPackage(ctx context.Context) {
	// Create a new package via init, no task specified
	const (
		repository  = "git"
		packageName = "empty-package"
		revision    = "v1"
		workspace   = "test-workspace"
		description = "empty-package description"
	)

	// Register the repository
	t.RegisterMainGitRepositoryF(ctx, repository)

	// Create a new package (via init)
	pr := &porchapi.PackageRevision{
		TypeMeta: metav1.TypeMeta{
			Kind:       "PackageRevision",
			APIVersion: porchapi.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: t.Namespace,
		},
		Spec: porchapi.PackageRevisionSpec{
			PackageName:    packageName,
			WorkspaceName:  workspace,
			RepositoryName: repository,
		},
	}
	t.CreateF(ctx, pr)

	// Get the package
	var newPackage porchapi.PackageRevisionResources
	t.GetF(ctx, client.ObjectKey{
		Namespace: t.Namespace,
		Name:      pr.Name,
	}, &newPackage)

	kptfile := t.ParseKptfileF(&newPackage)
	if got, want := kptfile.Name, "empty-package"; got != want {
		t.Fatalf("New package name: got %q, want %q", got, want)
	}
	if got, want := kptfile.Info, (&kptfilev1.PackageInfo{
		Description: description,
	}); !cmp.Equal(want, got) {
		t.Fatalf("unexpected %s/%s package info (-want, +got) %s", newPackage.Namespace, newPackage.Name, cmp.Diff(want, got))
	}
}

func (t *PorchSuite) TestConcurrentInits(ctx context.Context) {
	// Create a new package via init, no task specified
	const (
		repository  = "git-concurrent"
		packageName = "empty-package-concurrent"
		revision    = "v1"
		workspace   = "test-workspace"
		description = "empty-package description"
	)

	// Register the repository
	t.RegisterMainGitRepositoryF(ctx, repository)

	// Two clients try to create the same new draft package
	pr := t.CreatePackageSkeleton(repository, packageName, workspace)
	creationFunction := func() any {
		return t.Client.Create(ctx, pr)
	}
	results := RunInParallel(creationFunction, creationFunction)

	// one client succeeds; one receives a conflict error
	expectedResultCount := 2
	actualResultCount := len(results)
	assert.Equal(t, expectedResultCount, actualResultCount, "expected %d results but was %d", expectedResultCount, actualResultCount)

	assert.Contains(t, results, nil, "expected one request to succeed, but did not happen - results: %v", results)

	conflictFailurePresent := slices.ContainsFunc(results, func(eachResult any) bool {
		return eachResult != nil && strings.Contains(eachResult.(error).Error(), "another request is already in progress")
	})
	assert.True(t, conflictFailurePresent, "expected one request to fail with a conflict, but did not happen - results: %v", results)
}

func (t *PorchSuite) TestInitTaskPackage(ctx context.Context) {
	const (
		repository  = "git"
		packageName = "new-package"
		revision    = "v1"
		workspace   = "test-workspace"
		description = "New Package"
		site        = "https://kpt.dev/new-package"
	)
	keywords := []string{"test"}

	// Register the repository
	t.RegisterMainGitRepositoryF(ctx, repository)

	// Create a new package (via init)
	pr := &porchapi.PackageRevision{
		TypeMeta: metav1.TypeMeta{
			Kind:       "PackageRevision",
			APIVersion: porchapi.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: t.Namespace,
		},
		Spec: porchapi.PackageRevisionSpec{
			PackageName:    "new-package",
			WorkspaceName:  workspace,
			RepositoryName: repository,
			Tasks: []porchapi.Task{
				{
					Type: porchapi.TaskTypeInit,
					Init: &porchapi.PackageInitTaskSpec{
						Description: description,
						Keywords:    keywords,
						Site:        site,
					},
				},
			},
		},
	}
	t.CreateF(ctx, pr)

	// Get the package
	var newPackage porchapi.PackageRevisionResources
	t.GetF(ctx, client.ObjectKey{
		Namespace: t.Namespace,
		Name:      pr.Name,
	}, &newPackage)

	kptfile := t.ParseKptfileF(&newPackage)
	if got, want := kptfile.Name, "new-package"; got != want {
		t.Fatalf("New package name: got %q, want %q", got, want)
	}
	if got, want := kptfile.Info, (&kptfilev1.PackageInfo{
		Site:        site,
		Description: description,
		Keywords:    keywords,
	}); !cmp.Equal(want, got) {
		t.Fatalf("unexpected %s/%s package info (-want, +got) %s", newPackage.Namespace, newPackage.Name, cmp.Diff(want, got))
	}
}

func (t *PorchSuite) TestCloneIntoDeploymentRepository(ctx context.Context) {
	const downstreamRepository = "deployment"
	const downstreamPackage = "istions"
	const downstreamWorkspace = "test-workspace"

	// Register the deployment repository
	t.RegisterMainGitRepositoryF(ctx, downstreamRepository, WithDeployment())

	// Register the upstream repository
	t.RegisterGitRepositoryF(ctx, testBlueprintsRepo, "test-blueprints", "")

	var upstreamPackages porchapi.PackageRevisionList
	t.ListE(ctx, &upstreamPackages, client.InNamespace(t.Namespace))
	upstreamPackage := MustFindPackageRevision(t.T, &upstreamPackages, repository.PackageRevisionKey{
		Repository:    "test-blueprints",
		Package:       "basens",
		Revision:      "v1",
		WorkspaceName: "v1",
	})

	// Create PackageRevision from upstream repo
	pr := &porchapi.PackageRevision{
		TypeMeta: metav1.TypeMeta{
			Kind:       "PackageRevision",
			APIVersion: porchapi.SchemeGroupVersion.Identifier(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: t.Namespace,
		},
		Spec: porchapi.PackageRevisionSpec{
			PackageName:    downstreamPackage,
			WorkspaceName:  downstreamWorkspace,
			RepositoryName: downstreamRepository,
			Tasks: []porchapi.Task{
				{
					Type: porchapi.TaskTypeClone,
					Clone: &porchapi.PackageCloneTaskSpec{
						Upstream: porchapi.UpstreamPackage{
							UpstreamRef: &porchapi.PackageRevisionRef{
								Name: upstreamPackage.Name, // Package to be cloned
							},
						},
					},
				},
			},
		},
	}
	t.CreateF(ctx, pr)

	// Get istions resources
	var istions porchapi.PackageRevisionResources
	t.GetF(ctx, client.ObjectKey{
		Namespace: t.Namespace,
		Name:      pr.Name,
	}, &istions)

	kptfile := t.ParseKptfileF(&istions)

	if got, want := kptfile.Name, "istions"; got != want {
		t.Errorf("istions package Kptfile.metadata.name: got %q, want %q", got, want)
	}
	if kptfile.UpstreamLock == nil {
		t.Fatalf("istions package upstreamLock is missing")
	}
	if kptfile.UpstreamLock.Git == nil {
		t.Errorf("istions package upstreamLock.git is missing")
	}
	if kptfile.UpstreamLock.Git.Commit == "" {
		t.Errorf("istions package upstreamLock.git.commit is missing")
	}

	// Remove commit from comparison
	got := kptfile.UpstreamLock
	got.Git.Commit = ""

	want := &kptfilev1.UpstreamLock{
		Type: kptfilev1.GitOrigin,
		Git: &kptfilev1.GitLock{
			Repo:      testBlueprintsRepo,
			Directory: "basens",
			Ref:       "basens/v1",
		},
	}
	if !cmp.Equal(want, got) {
		t.Errorf("unexpected upstreamlock returned (-want, +got) %s", cmp.Diff(want, got))
	}

	// Check Upstream
	if got, want := kptfile.Upstream, (&kptfilev1.Upstream{
		Type: kptfilev1.GitOrigin,
		Git: &kptfilev1.Git{
			Repo:      testBlueprintsRepo,
			Directory: "basens",
			Ref:       "basens/v1",
		},
	}); !cmp.Equal(want, got) {
		t.Errorf("unexpected upstream returned (-want, +got) %s", cmp.Diff(want, got))
	}

	// Check generated context
	var configmap corev1.ConfigMap
	t.FindAndDecodeF(&istions, "package-context.yaml", &configmap)
	if got, want := configmap.Name, "kptfile.kpt.dev"; got != want {
		t.Errorf("package context name: got %s, want %s", got, want)
	}
	if got, want := configmap.Data["name"], "istions"; got != want {
		t.Errorf("package context 'data.name': got %s, want %s", got, want)
	}
}

func (t *PorchSuite) TestEditPackageRevision(ctx context.Context) {
	const (
		repository       = "edit-test"
		packageName      = "simple-package"
		otherPackageName = "other-package"
		workspace        = "workspace"
		workspace2       = "workspace2"
	)

	t.RegisterMainGitRepositoryF(ctx, repository)

	// Create a new package (via init)
	pr := &porchapi.PackageRevision{
		TypeMeta: metav1.TypeMeta{
			Kind:       "PackageRevision",
			APIVersion: porchapi.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: t.Namespace,
		},
		Spec: porchapi.PackageRevisionSpec{
			PackageName:    packageName,
			WorkspaceName:  workspace,
			RepositoryName: repository,
		},
	}
	t.CreateF(ctx, pr)

	// Create a new revision, but with a different package as the source.
	// This is not allowed.
	invalidEditPR := &porchapi.PackageRevision{
		TypeMeta: metav1.TypeMeta{
			Kind:       "PackageRevision",
			APIVersion: porchapi.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: t.Namespace,
		},
		Spec: porchapi.PackageRevisionSpec{
			PackageName:    otherPackageName,
			WorkspaceName:  workspace2,
			RepositoryName: repository,
			Tasks: []porchapi.Task{
				{
					Type: porchapi.TaskTypeEdit,
					Edit: &porchapi.PackageEditTaskSpec{
						Source: &porchapi.PackageRevisionRef{
							Name: pr.Name,
						},
					},
				},
			},
		},
	}
	if err := t.Client.Create(ctx, invalidEditPR); err == nil {
		t.Fatalf("Expected error for source revision being from different package")
	}

	// Create a new revision of the package with a source that is a revision
	// of the same package.
	editPR := &porchapi.PackageRevision{
		TypeMeta: metav1.TypeMeta{
			Kind:       "PackageRevision",
			APIVersion: porchapi.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: t.Namespace,
		},
		Spec: porchapi.PackageRevisionSpec{
			PackageName:    packageName,
			WorkspaceName:  workspace2,
			RepositoryName: repository,
			Tasks: []porchapi.Task{
				{
					Type: porchapi.TaskTypeEdit,
					Edit: &porchapi.PackageEditTaskSpec{
						Source: &porchapi.PackageRevisionRef{
							Name: pr.Name,
						},
					},
				},
			},
		},
	}
	if err := t.Client.Create(ctx, editPR); err == nil {
		t.Fatalf("Expected error for source revision not being published")
	}

	// Publish the source package to make it a valid source for edit.
	pr.Spec.Lifecycle = porchapi.PackageRevisionLifecycleProposed
	t.UpdateF(ctx, pr)

	// Approve the package
	pr.Spec.Lifecycle = porchapi.PackageRevisionLifecyclePublished
	t.UpdateApprovalF(ctx, pr, metav1.UpdateOptions{})

	// Create a new revision with the edit task.
	t.CreateF(ctx, editPR)

	// Check its task list
	var pkgRev porchapi.PackageRevision
	t.GetF(ctx, client.ObjectKey{
		Namespace: t.Namespace,
		Name:      editPR.Name,
	}, &pkgRev)
	tasks := pkgRev.Spec.Tasks
	for _, tsk := range tasks {
		t.Logf("Task: %s", tsk.Type)
	}
	assert.Equal(t, 2, len(tasks))
}

func (t *PorchSuite) TestConcurrentEdits(ctx context.Context) {
	const (
		repository  = "edit-test"
		packageName = "simple-package-concurrent"
		workspace   = "workspace"
		workspace2  = "workspace2"
	)

	t.RegisterMainGitRepositoryF(ctx, repository)

	// Create a new package (via init)
	pr := t.CreatePackageSkeleton(repository, packageName, workspace)
	t.CreateF(ctx, pr)

	// Publish and approve the source package to make it a valid source for edit.
	pr.Spec.Lifecycle = porchapi.PackageRevisionLifecycleProposed
	t.UpdateF(ctx, pr)
	pr.Spec.Lifecycle = porchapi.PackageRevisionLifecyclePublished
	t.UpdateApprovalF(ctx, pr, metav1.UpdateOptions{})

	// Create a new revision of the package with a source that is a revision
	// of the same package.
	editPR := t.CreatePackageSkeleton(repository, packageName, workspace2)
	editPR.Spec.Tasks = []porchapi.Task{
		{
			Type: porchapi.TaskTypeEdit,
			Edit: &porchapi.PackageEditTaskSpec{
				Source: &porchapi.PackageRevisionRef{
					Name: pr.Name,
				},
			},
		},
	}

	// Two clients try to create the new package at the same time
	editFunction := func() any {
		return t.Client.Create(ctx, editPR)
	}
	results := RunInParallel(
		editFunction,
		editFunction)

	assert.Contains(t, results, nil, "expected one request to succeed, but did not happen - results: %v", results)

	conflictFailurePresent := slices.ContainsFunc(results, func(eachResult any) bool {
		return eachResult != nil && strings.Contains(eachResult.(error).Error(), "another request is already in progress")
	})
	assert.True(t, conflictFailurePresent, "expected one request to fail with a conflict, but did not happen - results: %v", results)

	// Check its task list
	var pkgRev porchapi.PackageRevision
	t.GetF(ctx, client.ObjectKey{
		Namespace: t.Namespace,
		Name:      editPR.Name,
	}, &pkgRev)
	tasks := pkgRev.Spec.Tasks
	assert.Equal(t, 2, len(tasks))
}

// Test will initialize an empty package, update its resources, adding a function
// to the Kptfile's pipeline, and then check that the package was re-rendered.
func (t *PorchSuite) TestUpdateResources(ctx context.Context) {
	const (
		repository  = "re-render-test"
		packageName = "simple-package"
		workspace   = "workspace"
	)

	t.RegisterMainGitRepositoryF(ctx, repository)

	// Create a new package (via init)
	pr := &porchapi.PackageRevision{
		TypeMeta: metav1.TypeMeta{
			Kind:       "PackageRevision",
			APIVersion: porchapi.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: t.Namespace,
		},
		Spec: porchapi.PackageRevisionSpec{
			PackageName:    packageName,
			WorkspaceName:  workspace,
			RepositoryName: repository,
		},
	}
	t.CreateF(ctx, pr)

	// Get the package resources
	var newPackage porchapi.PackageRevisionResources
	t.GetF(ctx, client.ObjectKey{
		Namespace: t.Namespace,
		Name:      pr.Name,
	}, &newPackage)

	// Add function into a pipeline
	kptfile := t.ParseKptfileF(&newPackage)
	if kptfile.Pipeline == nil {
		kptfile.Pipeline = &kptfilev1.Pipeline{}
	}
	kptfile.Pipeline.Mutators = append(kptfile.Pipeline.Mutators, kptfilev1.Function{
		Image: "gcr.io/kpt-fn/set-annotations:v0.1.4",
		ConfigMap: map[string]string{
			"color": "red",
			"fruit": "apple",
		},
		Name: "set-annotations",
	})
	t.SaveKptfileF(&newPackage, kptfile)

	// Add a new resource
	filename := filepath.Join("testdata", "update-resources", "add-config-map.yaml")
	cm, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("Failed to read ConfigMap from %q: %v", filename, err)
	}
	newPackage.Spec.Resources["config-map.yaml"] = string(cm)
	t.UpdateF(ctx, &newPackage)

	updated, ok := newPackage.Spec.Resources["config-map.yaml"]
	if !ok {
		t.Fatalf("Updated config map config-map.yaml not found")
	}

	renderStatus := newPackage.Status.RenderStatus
	assert.Empty(t, renderStatus.Err, "render error must be empty for successful render operation.")
	assert.Zero(t, renderStatus.Result.ExitCode, "exit code must be zero for successful render operation.")
	assert.True(t, len(renderStatus.Result.Items) > 0)

	golden := filepath.Join("testdata", "update-resources", "want-config-map.yaml")
	if diff := t.CompareGoldenFileYAML(golden, updated); diff != "" {
		t.Errorf("Unexpected updated config map contents: (-want,+got): %s", diff)
	}
}

// Test will initialize an empty package, and then make a call to update the resources
// without actually making any changes. This test is ensuring that no additional
// tasks get added.
func (t *PorchSuite) TestUpdateResourcesEmptyPatch(ctx context.Context) {
	const (
		repository  = "empty-patch-test"
		packageName = "simple-package"
		workspace   = "workspace"
	)

	t.RegisterMainGitRepositoryF(ctx, repository)

	// Create a new package (via init)
	pr := &porchapi.PackageRevision{
		TypeMeta: metav1.TypeMeta{
			Kind:       "PackageRevision",
			APIVersion: porchapi.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: t.Namespace,
		},
		Spec: porchapi.PackageRevisionSpec{
			PackageName:    packageName,
			WorkspaceName:  workspace,
			RepositoryName: repository,
		},
	}
	t.CreateF(ctx, pr)

	// Check its task list
	var pkgBeforeUpdate porchapi.PackageRevision
	t.GetF(ctx, client.ObjectKey{
		Namespace: t.Namespace,
		Name:      pr.Name,
	}, &pkgBeforeUpdate)
	tasksBeforeUpdate := pkgBeforeUpdate.Spec.Tasks
	assert.Equal(t, 2, len(tasksBeforeUpdate))

	// Get the package resources
	var resourcesBeforeUpdate porchapi.PackageRevisionResources
	t.GetF(ctx, client.ObjectKey{
		Namespace: t.Namespace,
		Name:      pr.Name,
	}, &resourcesBeforeUpdate)

	// "Update" the package resources, without changing anything
	t.UpdateF(ctx, &resourcesBeforeUpdate)

	// Check the task list
	var pkgAfterUpdate porchapi.PackageRevision
	t.GetF(ctx, client.ObjectKey{
		Namespace: t.Namespace,
		Name:      pr.Name,
	}, &pkgAfterUpdate)
	tasksAfterUpdate := pkgAfterUpdate.Spec.Tasks
	assert.Equal(t, 2, len(tasksAfterUpdate))

	assert.True(t, reflect.DeepEqual(tasksBeforeUpdate, tasksAfterUpdate))

	// Get the package resources
	var resourcesAfterUpdate porchapi.PackageRevisionResources
	t.GetF(ctx, client.ObjectKey{
		Namespace: t.Namespace,
		Name:      pr.Name,
	}, &resourcesAfterUpdate)

	assert.Equal(t, 3, len(resourcesAfterUpdate.Spec.Resources))
	assert.True(t, reflect.DeepEqual(resourcesBeforeUpdate, resourcesAfterUpdate))
}

func (t *PorchSuite) TestConcurrentResourceUpdates(ctx context.Context) {
	const (
		repository  = "concurrent-update-test"
		packageName = "simple-package"
		workspace   = "workspace"
	)

	t.RegisterMainGitRepositoryF(ctx, repository)

	// Create a new package (via init)
	pr := t.CreatePackageSkeleton(repository, packageName, workspace)
	t.CreateF(ctx, pr)

	// Get the package resources
	var newPackageResources porchapi.PackageRevisionResources
	t.GetF(ctx, client.ObjectKey{
		Namespace: t.Namespace,
		Name:      pr.Name,
	}, &newPackageResources)

	// "Update" the package resources with two clients at the same time
	updateFunction := func() any {
		return t.Client.Update(ctx, &newPackageResources)
	}
	results := RunInParallel(updateFunction, updateFunction)

	assert.Contains(t, results, nil, "expected one request to succeed, but did not happen - results: %v", results)

	conflictFailurePresent := slices.ContainsFunc(results, func(eachResult any) bool {
		return eachResult != nil && strings.Contains(eachResult.(error).Error(), "another request is already in progress")
	})
	assert.True(t, conflictFailurePresent, "expected one request to fail with a conflict, but did not happen - results: %v", results)
}

func (t *PorchSuite) NewClientWithTimeout(timeout time.Duration) client.Client {
	cfg := *t.Kubeconfig
	cfg.Timeout = timeout
	c, err := client.New(&cfg, client.Options{
		Scheme: t.Client.Scheme(),
	})
	if err != nil {
		t.Fatalf("Failed to create client with timeout: %v", err)
	}
	return c
}

func (t *PorchSuite) TestPublicGitRepository(ctx context.Context) {
	t.RegisterGitRepositoryF(ctx, testBlueprintsRepo, "demo-blueprints", "")

	var list porchapi.PackageRevisionList
	t.ListE(ctx, &list, client.InNamespace(t.Namespace))

	if got := len(list.Items); got == 0 {
		t.Errorf("Found no package revisions in %s; expected at least one", testBlueprintsRepo)
	}
}

func (t *PorchSuite) TestProposeApprove(ctx context.Context) {
	const (
		repository  = "lifecycle"
		packageName = "test-package"
		workspace   = "workspace"
	)

	// Register the repository
	t.RegisterMainGitRepositoryF(ctx, repository)

	// Create a new package (via init)
	pr := &porchapi.PackageRevision{
		TypeMeta: metav1.TypeMeta{
			Kind:       "PackageRevision",
			APIVersion: porchapi.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: t.Namespace,
		},
		Spec: porchapi.PackageRevisionSpec{
			PackageName:    packageName,
			WorkspaceName:  workspace,
			RepositoryName: repository,
			Tasks: []porchapi.Task{
				{
					Type: porchapi.TaskTypeInit,
					Init: &porchapi.PackageInitTaskSpec{},
				},
			},
		},
	}
	t.CreateF(ctx, pr)

	var pkg porchapi.PackageRevision
	t.GetF(ctx, client.ObjectKey{
		Namespace: t.Namespace,
		Name:      pr.Name,
	}, &pkg)

	// Propose the package revision to be finalized
	pkg.Spec.Lifecycle = porchapi.PackageRevisionLifecycleProposed
	t.UpdateF(ctx, &pkg)

	var proposed porchapi.PackageRevision
	t.GetF(ctx, client.ObjectKey{
		Namespace: t.Namespace,
		Name:      pr.Name,
	}, &proposed)

	if got, want := proposed.Spec.Lifecycle, porchapi.PackageRevisionLifecycleProposed; got != want {
		t.Fatalf("Proposed package lifecycle value: got %s, want %s", got, want)
	}

	// Approve using Update should fail.
	proposed.Spec.Lifecycle = porchapi.PackageRevisionLifecyclePublished
	if err := t.Client.Update(ctx, &proposed); err == nil {
		t.Fatalf("Finalization of a package via Update unexpectedly succeeded")
	}

	// Approve the package
	proposed.Spec.Lifecycle = porchapi.PackageRevisionLifecyclePublished
	approved := t.UpdateApprovalF(ctx, &proposed, metav1.UpdateOptions{})
	if got, want := approved.Spec.Lifecycle, porchapi.PackageRevisionLifecyclePublished; got != want {
		t.Fatalf("Approved package lifecycle value: got %s, want %s", got, want)
	}

	// Check its revision number
	if got, want := approved.Spec.Revision, "v1"; got != want {
		t.Fatalf("Approved package revision value: got %s, want %s", got, want)
	}
}

func (t *PorchSuite) TestConcurrentProposeApprove(ctx context.Context) {
	const (
		repository  = "lifecycle"
		packageName = "test-package-concurrent"
		workspace   = "workspace"
	)

	// Register the repository
	t.RegisterMainGitRepositoryF(ctx, repository)

	// Create a new package (via init)
	pr := t.CreatePackageSkeleton(repository, packageName, workspace)
	pr.Spec.Tasks = []porchapi.Task{
		{
			Type: porchapi.TaskTypeInit,
			Init: &porchapi.PackageInitTaskSpec{},
		},
	}
	t.CreateF(ctx, pr)

	var pkg porchapi.PackageRevision
	t.GetF(ctx, client.ObjectKey{
		Namespace: t.Namespace,
		Name:      pr.Name,
	}, &pkg)

	// Propose the package revision to be finalized
	pkg.Spec.Lifecycle = porchapi.PackageRevisionLifecycleProposed
	proposeFunction := func() any {
		return t.Client.Update(ctx, &pkg)
	}
	proposeResults := RunInParallel(proposeFunction, proposeFunction)

	assert.Contains(t, proposeResults, nil, "expected one 'propose' request to succeed, but did not happen - results: %v", proposeResults)

	conflictFailurePresent := slices.ContainsFunc(proposeResults, func(eachResult any) bool {
		return eachResult != nil && strings.Contains(eachResult.(error).Error(), "another request is already in progress")
	})
	assert.True(t, conflictFailurePresent, "expected one 'propose' request to fail with a conflict, but did not happen - results: %v", proposeResults)

	var proposed porchapi.PackageRevision
	t.GetF(ctx, client.ObjectKey{
		Namespace: t.Namespace,
		Name:      pr.Name,
	}, &proposed)

	// Approve the package
	proposed.Spec.Lifecycle = porchapi.PackageRevisionLifecyclePublished
	approveFunction := func() any {
		_, err := t.Clientset.PorchV1alpha1().PackageRevisions(proposed.Namespace).UpdateApproval(ctx, proposed.Name, &proposed, metav1.UpdateOptions{})
		return err
	}
	approveResults := RunInParallel(approveFunction, approveFunction)

	assert.Contains(t, approveResults, nil, "expected one 'approve' request to succeed, but did not happen - results: %v", approveResults)

	conflictFailurePresent = slices.ContainsFunc(approveResults, func(eachResult any) bool {
		return eachResult != nil && strings.Contains(eachResult.(error).Error(), "another request is already in progress")
	})
	assert.True(t, conflictFailurePresent, "expected one 'approve' request to fail with a conflict, but did not happen - results: %v", approveResults)
}

func (t *PorchSuite) TestSubfolderPackageRevisionIncrementation(ctx context.Context) {
	const (
		repository           = "lifecycle"
		subfolderRepository  = "repo-in-subfolder"
		subfolderDirectory   = "randomRepoFolder"
		normalPackageName    = "test-package"
		subfolderPackageName = "randomPackageFolder/test-package"
		workspace            = "workspace"
		workspace2           = "workspace2"
	)

	// Register the repositories
	t.RegisterMainGitRepositoryF(ctx, repository)
	t.RegisterGitRepositoryWithDirectoryF(ctx, subfolderRepository, subfolderDirectory)

	// Create a new package (via init)
	subfolderPr := t.CreatePackageDraftF(ctx, repository, subfolderPackageName, workspace)
	prInSubfolder := t.CreatePackageDraftF(ctx, subfolderRepository, normalPackageName, workspace)

	// Propose and approve the package revisions
	subfolderPr.Spec.Lifecycle = porchapi.PackageRevisionLifecycleProposed
	prInSubfolder.Spec.Lifecycle = porchapi.PackageRevisionLifecycleProposed
	t.UpdateF(ctx, subfolderPr)
	t.UpdateF(ctx, prInSubfolder)

	subfolderPr.Spec.Lifecycle = porchapi.PackageRevisionLifecyclePublished
	prInSubfolder.Spec.Lifecycle = porchapi.PackageRevisionLifecyclePublished
	subfolderPr = t.UpdateApprovalF(ctx, subfolderPr, metav1.UpdateOptions{})
	prInSubfolder = t.UpdateApprovalF(ctx, prInSubfolder, metav1.UpdateOptions{})

	assert.Equal(t, porchapi.PackageRevisionLifecyclePublished, subfolderPr.Spec.Lifecycle)
	assert.Equal(t, "v1", subfolderPr.Spec.Revision)
	assert.Equal(t, porchapi.PackageRevisionLifecyclePublished, prInSubfolder.Spec.Lifecycle)
	assert.Equal(t, "v1", prInSubfolder.Spec.Revision)

	// Create new package revisions via edit/copy
	editedSubfolderPr := t.CreatePackageSkeleton(repository, subfolderPackageName, workspace2)
	editedSubfolderPr.Spec.Tasks = []porchapi.Task{
		{
			Type: porchapi.TaskTypeEdit,
			Edit: &porchapi.PackageEditTaskSpec{
				Source: &porchapi.PackageRevisionRef{
					Name: subfolderPr.Name,
				},
			},
		},
	}
	t.CreateF(ctx, editedSubfolderPr)
	editedPrInSubfolder := t.CreatePackageSkeleton(subfolderRepository, normalPackageName, workspace2)
	editedPrInSubfolder.Spec.Tasks = []porchapi.Task{
		{
			Type: porchapi.TaskTypeEdit,
			Edit: &porchapi.PackageEditTaskSpec{
				Source: &porchapi.PackageRevisionRef{
					Name: prInSubfolder.Name,
				},
			},
		},
	}
	t.CreateF(ctx, editedPrInSubfolder)

	// Propose and approve these package revisions as well
	editedSubfolderPr.Spec.Lifecycle = porchapi.PackageRevisionLifecycleProposed
	editedPrInSubfolder.Spec.Lifecycle = porchapi.PackageRevisionLifecycleProposed
	t.UpdateF(ctx, editedSubfolderPr)
	t.UpdateF(ctx, editedPrInSubfolder)

	editedSubfolderPr.Spec.Lifecycle = porchapi.PackageRevisionLifecyclePublished
	editedPrInSubfolder.Spec.Lifecycle = porchapi.PackageRevisionLifecyclePublished
	editedSubfolderPr = t.UpdateApprovalF(ctx, editedSubfolderPr, metav1.UpdateOptions{})
	editedPrInSubfolder = t.UpdateApprovalF(ctx, editedPrInSubfolder, metav1.UpdateOptions{})

	assert.Equal(t, porchapi.PackageRevisionLifecyclePublished, editedSubfolderPr.Spec.Lifecycle)
	assert.Equal(t, "v2", editedSubfolderPr.Spec.Revision)
	assert.Equal(t, porchapi.PackageRevisionLifecyclePublished, editedPrInSubfolder.Spec.Lifecycle)
	assert.Equal(t, "v2", editedPrInSubfolder.Spec.Revision)
}

func (t *PorchSuite) TestDeleteDraft(ctx context.Context) {
	const (
		repository  = "delete-draft"
		packageName = "test-delete-draft"
		revision    = "v1"
		workspace   = "test-workspace"
	)

	// Register the repository
	t.RegisterMainGitRepositoryF(ctx, repository)

	// Create a draft package
	created := t.CreatePackageDraftF(ctx, repository, packageName, workspace)

	// Check the package exists
	var draft porchapi.PackageRevision
	t.MustExist(ctx, client.ObjectKey{Namespace: t.Namespace, Name: created.Name}, &draft)

	// Delete the package
	t.DeleteE(ctx, &porchapi.PackageRevision{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: t.Namespace,
			Name:      created.Name,
		},
	})

	t.MustNotExist(ctx, &draft)
}

func (t *PorchSuite) TestConcurrentDeletes(ctx context.Context) {
	const (
		repository  = "delete-draft"
		packageName = "test-delete-draft-concurrent"
		revision    = "v1"
		workspace   = "test-workspace"
	)

	// Register the repository and create a draft package
	t.RegisterMainGitRepositoryF(ctx, repository)
	created := t.CreatePackageDraftF(ctx, repository, packageName, workspace)

	// Check the package exists
	var draft porchapi.PackageRevision
	t.MustExist(ctx, client.ObjectKey{Namespace: t.Namespace, Name: created.Name}, &draft)

	// Delete the same package with two clients at the same time
	deleteFunction := func() any {
		return t.Client.Delete(ctx, &porchapi.PackageRevision{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: t.Namespace,
				Name:      created.Name,
			},
		})
	}
	results := RunInParallel(
		deleteFunction,
		deleteFunction)

	expectedResultCount := 2
	actualResultCount := len(results)
	assert.Equal(t, expectedResultCount, actualResultCount, "expected %d results but was %d", expectedResultCount, actualResultCount)

	assert.Contains(t, results, nil, "expected one request to succeed, but did not happen - results: %v", results)

	conflictFailurePresent := slices.ContainsFunc(results, func(eachResult any) bool {
		return eachResult != nil && strings.Contains(eachResult.(error).Error(), "another request is already in progress")
	})
	assert.True(t, conflictFailurePresent, "expected one request to fail with a conflict, but did not happen - results: %v", results)
	t.MustNotExist(ctx, &draft)
}

func (t *PorchSuite) TestDeleteProposed(ctx context.Context) {
	const (
		repository  = "delete-proposed"
		packageName = "test-delete-proposed"
		revision    = "v1"
		workspace   = "workspace"
	)

	// Register the repository
	t.RegisterMainGitRepositoryF(ctx, repository)

	// Create a draft package
	created := t.CreatePackageDraftF(ctx, repository, packageName, workspace)

	// Check the package exists
	var pkg porchapi.PackageRevision
	t.MustExist(ctx, client.ObjectKey{Namespace: t.Namespace, Name: created.Name}, &pkg)

	// Propose the package revision to be finalized
	pkg.Spec.Lifecycle = porchapi.PackageRevisionLifecycleProposed
	t.UpdateF(ctx, &pkg)

	// Delete the package
	t.DeleteE(ctx, &porchapi.PackageRevision{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: t.Namespace,
			Name:      created.Name,
		},
	})

	t.MustNotExist(ctx, &pkg)
}

func (t *PorchSuite) TestDeleteFinal(ctx context.Context) {
	const (
		repository  = "delete-final"
		packageName = "test-delete-final"
		workspace   = "workspace"
	)

	// Register the repository
	t.RegisterMainGitRepositoryF(ctx, repository)

	// Create a draft package
	created := t.CreatePackageDraftF(ctx, repository, packageName, workspace)

	// Check the package exists
	var pkg porchapi.PackageRevision
	t.MustExist(ctx, client.ObjectKey{Namespace: t.Namespace, Name: created.Name}, &pkg)

	// Propose the package revision to be finalized
	t.Log("Proposing package")
	pkg.Spec.Lifecycle = porchapi.PackageRevisionLifecycleProposed
	t.UpdateF(ctx, &pkg)

	t.Log("Approving package")
	pkg.Spec.Lifecycle = porchapi.PackageRevisionLifecyclePublished
	t.UpdateApprovalF(ctx, &pkg, metav1.UpdateOptions{})

	t.MustExist(ctx, client.ObjectKey{Namespace: t.Namespace, Name: created.Name}, &pkg)

	// Try to delete the package. This should fail because it hasn't been proposed for deletion.
	t.Log("Trying to delete package (should fail)")
	t.DeleteL(ctx, &porchapi.PackageRevision{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: t.Namespace,
			Name:      created.Name,
		},
	})
	t.MustExist(ctx, client.ObjectKey{Namespace: t.Namespace, Name: created.Name}, &pkg)

	// Propose deletion and then delete the package
	t.Log("Proposing deletion of  package")
	pkg.Spec.Lifecycle = porchapi.PackageRevisionLifecycleDeletionProposed
	t.UpdateApprovalF(ctx, &pkg, metav1.UpdateOptions{})

	t.Log("Deleting package")
	t.DeleteE(ctx, &porchapi.PackageRevision{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: t.Namespace,
			Name:      created.Name,
		},
	})

	t.MustNotExist(ctx, &pkg)
}

func (t *PorchSuite) TestConcurrentProposeDeletes(ctx context.Context) {
	const (
		repository  = "delete-final"
		packageName = "test-delete-final-concurrent"
		workspace   = "workspace"
	)

	// Register the repository and create a draft package
	t.RegisterMainGitRepositoryF(ctx, repository)
	created := t.CreatePackageDraftF(ctx, repository, packageName, workspace)
	// Check the package exists
	var pkg porchapi.PackageRevision
	t.MustExist(ctx, client.ObjectKey{Namespace: t.Namespace, Name: created.Name}, &pkg)

	// Propose and approve the package revision to be finalized
	pkg.Spec.Lifecycle = porchapi.PackageRevisionLifecycleProposed
	t.UpdateF(ctx, &pkg)
	pkg.Spec.Lifecycle = porchapi.PackageRevisionLifecyclePublished
	t.UpdateApprovalF(ctx, &pkg, metav1.UpdateOptions{})

	t.MustExist(ctx, client.ObjectKey{Namespace: t.Namespace, Name: created.Name}, &pkg)

	// Propose deletion with two clients at once
	pkg.Spec.Lifecycle = porchapi.PackageRevisionLifecycleDeletionProposed
	proposeDeleteFunction := func() any {
		_, err := t.Clientset.PorchV1alpha1().PackageRevisions(pkg.Namespace).UpdateApproval(ctx, pkg.Name, &pkg, metav1.UpdateOptions{})
		return err
	}
	proposeDeleteResults := RunInParallel(proposeDeleteFunction, proposeDeleteFunction)

	assert.Contains(t, proposeDeleteResults, nil, "expected one 'propose-delete' request to succeed, but did not happen - results: %v", proposeDeleteResults)

	conflictFailurePresent := slices.ContainsFunc(proposeDeleteResults, func(eachResult any) bool {
		return eachResult != nil && strings.Contains(eachResult.(error).Error(), "another request is already in progress")
	})
	assert.True(t, conflictFailurePresent, "expected one 'propose-delete' request to fail with a conflict, but did not happen")
}

func (t *PorchSuite) TestProposeDeleteAndUndo(ctx context.Context) {
	const (
		repository  = "test-propose-delete-and-undo"
		packageName = "test-propose-delete-and-undo"
		workspace   = "workspace"
	)

	// Register the repository
	t.RegisterMainGitRepositoryF(ctx, repository)

	// Create a draft package
	created := t.CreatePackageDraftF(ctx, repository, packageName, workspace)

	// Check the package exists
	var pkg porchapi.PackageRevision
	t.MustExist(ctx, client.ObjectKey{Namespace: t.Namespace, Name: created.Name}, &pkg)

	// Propose the package revision to be finalized
	pkg.Spec.Lifecycle = porchapi.PackageRevisionLifecycleProposed
	t.UpdateF(ctx, &pkg)

	pkg.Spec.Lifecycle = porchapi.PackageRevisionLifecyclePublished
	t.UpdateApprovalF(ctx, &pkg, metav1.UpdateOptions{})
	t.MustExist(ctx, client.ObjectKey{Namespace: t.Namespace, Name: created.Name}, &pkg)

	_ = t.WaitUntilPackageRevisionExists(ctx, repository, packageName, "main")

	var list porchapi.PackageRevisionList
	t.ListF(ctx, &list, client.InNamespace(t.Namespace))

	for i := range list.Items {
		pkgRev := list.Items[i]
		t.Run(fmt.Sprintf("revision %s", pkgRev.Spec.Revision), func(newT *testing.T) {
			// This is a bit awkward, we should find a better way to allow subtests
			// with our custom implementation of t.
			oldT := t.T
			t.T = newT
			defer func() {
				t.T = oldT
			}()

			// Propose deletion
			pkgRev.Spec.Lifecycle = porchapi.PackageRevisionLifecycleDeletionProposed
			t.UpdateApprovalF(ctx, &pkgRev, metav1.UpdateOptions{})

			// Undo proposal of deletion
			pkgRev.Spec.Lifecycle = porchapi.PackageRevisionLifecyclePublished
			t.UpdateApprovalF(ctx, &pkgRev, metav1.UpdateOptions{})

			// Try to delete the package. This should fail because the lifecycle should be changed back to Published.
			t.DeleteL(ctx, &porchapi.PackageRevision{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: t.Namespace,
					Name:      pkgRev.Name,
				},
			})
			t.MustExist(ctx, client.ObjectKey{Namespace: t.Namespace, Name: pkgRev.Name}, &pkgRev)

			// Propose deletion and then delete the package
			pkgRev.Spec.Lifecycle = porchapi.PackageRevisionLifecycleDeletionProposed
			t.UpdateApprovalF(ctx, &pkgRev, metav1.UpdateOptions{})

			t.DeleteE(ctx, &porchapi.PackageRevision{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: t.Namespace,
					Name:      pkgRev.Name,
				},
			})

			t.MustNotExist(ctx, &pkgRev)
		})
	}
}

func (t *PorchSuite) TestDeleteAndRecreate(ctx context.Context) {
	const (
		repository  = "delete-and-recreate"
		packageName = "test-delete-and-recreate"
		revision    = "v1"
		workspace   = "work"
	)

	// Register the repository
	t.RegisterMainGitRepositoryF(ctx, repository)

	// Create a draft package
	created := t.CreatePackageDraftF(ctx, repository, packageName, workspace)

	// Check the package exists
	var pkg porchapi.PackageRevision
	t.MustExist(ctx, client.ObjectKey{Namespace: t.Namespace, Name: created.Name}, &pkg)

	t.Log("Propose the package revision to be finalized")
	pkg.Spec.Lifecycle = porchapi.PackageRevisionLifecycleProposed
	t.UpdateF(ctx, &pkg)

	t.Log("Approve the package revision to be finalized")
	pkg.Spec.Lifecycle = porchapi.PackageRevisionLifecyclePublished
	t.UpdateApprovalF(ctx, &pkg, metav1.UpdateOptions{})

	t.MustExist(ctx, client.ObjectKey{Namespace: t.Namespace, Name: created.Name}, &pkg)

	mainPkg := t.WaitUntilPackageRevisionExists(ctx, repository, packageName, "main")

	t.Log("Propose deletion and then delete the package with revision v1")
	pkg.Spec.Lifecycle = porchapi.PackageRevisionLifecycleDeletionProposed
	t.UpdateApprovalF(ctx, &pkg, metav1.UpdateOptions{})

	t.DeleteE(ctx, &porchapi.PackageRevision{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: t.Namespace,
			Name:      created.Name,
		},
	})
	t.MustNotExist(ctx, &pkg)

	t.Log("Propose deletion and then delete the package with revision main")
	mainPkg.Spec.Lifecycle = porchapi.PackageRevisionLifecycleDeletionProposed
	t.UpdateApprovalF(ctx, mainPkg, metav1.UpdateOptions{})

	t.DeleteE(ctx, &porchapi.PackageRevision{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: t.Namespace,
			Name:      mainPkg.Name,
		},
	})
	t.MustNotExist(ctx, mainPkg)

	// Recreate the package with the same name and workspace
	created = t.CreatePackageDraftF(ctx, repository, packageName, workspace)

	// Check the package exists
	t.MustExist(ctx, client.ObjectKey{Namespace: t.Namespace, Name: created.Name}, &pkg)

	// Ensure that there is only one init task in the package revision history
	foundInitTask := false
	for _, task := range pkg.Spec.Tasks {
		if task.Type == porchapi.TaskTypeInit {
			if foundInitTask {
				t.Fatalf("found two init tasks in recreated package revision")
			}
			foundInitTask = true
		}
	}
	t.Logf("successfully recreated package revision %q", packageName)
}

func (t *PorchSuite) TestDeleteFromMain(ctx context.Context) {
	const (
		repository        = "delete-main"
		packageNameFirst  = "test-delete-main-first"
		packageNameSecond = "test-delete-main-second"
		workspace         = "workspace"
	)

	// Register the repository
	t.RegisterMainGitRepositoryF(ctx, repository)

	t.Logf("Create and approve package: %s", packageNameFirst)
	createdFirst := t.CreatePackageDraftF(ctx, repository, packageNameFirst, workspace)

	// Check the package exists
	var pkgFirst porchapi.PackageRevision
	t.MustExist(ctx, client.ObjectKey{Namespace: t.Namespace, Name: createdFirst.Name}, &pkgFirst)

	// Propose the package revision to be finalized
	pkgFirst.Spec.Lifecycle = porchapi.PackageRevisionLifecycleProposed
	t.UpdateF(ctx, &pkgFirst)

	pkgFirst.Spec.Lifecycle = porchapi.PackageRevisionLifecyclePublished
	t.UpdateApprovalF(ctx, &pkgFirst, metav1.UpdateOptions{})

	t.MustExist(ctx, client.ObjectKey{Namespace: t.Namespace, Name: createdFirst.Name}, &pkgFirst)

	t.Logf("Create and approve package: %s", packageNameSecond)
	createdSecond := t.CreatePackageDraftF(ctx, repository, packageNameSecond, workspace)

	// Check the package exists
	var pkgSecond porchapi.PackageRevision
	t.MustExist(ctx, client.ObjectKey{Namespace: t.Namespace, Name: createdSecond.Name}, &pkgSecond)

	// Propose the package revision to be finalized
	pkgSecond.Spec.Lifecycle = porchapi.PackageRevisionLifecycleProposed
	t.UpdateF(ctx, &pkgSecond)

	pkgSecond.Spec.Lifecycle = porchapi.PackageRevisionLifecyclePublished
	t.UpdateApprovalF(ctx, &pkgSecond, metav1.UpdateOptions{})

	t.MustExist(ctx, client.ObjectKey{Namespace: t.Namespace, Name: createdSecond.Name}, &pkgSecond)

	t.Log("Wait for the 'main' revisions to get created")
	firstPkgRevFromMain := t.WaitUntilPackageRevisionExists(ctx, repository, packageNameFirst, "main")
	secondPkgRevFromMain := t.WaitUntilPackageRevisionExists(ctx, repository, packageNameSecond, "main")

	t.Log("Propose deletion of both main packages")
	firstPkgRevFromMain.Spec.Lifecycle = porchapi.PackageRevisionLifecycleDeletionProposed
	t.UpdateApprovalF(ctx, firstPkgRevFromMain, metav1.UpdateOptions{})
	secondPkgRevFromMain.Spec.Lifecycle = porchapi.PackageRevisionLifecycleDeletionProposed
	t.UpdateApprovalF(ctx, secondPkgRevFromMain, metav1.UpdateOptions{})

	t.Log("Delete the first package revision from main")
	t.DeleteE(ctx, &porchapi.PackageRevision{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: t.Namespace,
			Name:      firstPkgRevFromMain.Name,
		},
	})

	t.Log("Delete the second package revision from main")
	t.DeleteE(ctx, &porchapi.PackageRevision{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: t.Namespace,
			Name:      secondPkgRevFromMain.Name,
		},
	})

	// Propose and delete the original package revisions (cleanup)
	var list porchapi.PackageRevisionList
	t.ListE(ctx, &list, client.InNamespace(t.Namespace))
	for _, pkgrev := range list.Items {
		t.Logf("Propose deletion and delete package revision: %s", pkgrev.Name)
		pkgrev.Spec.Lifecycle = porchapi.PackageRevisionLifecycleDeletionProposed
		t.UpdateApprovalF(ctx, &pkgrev, metav1.UpdateOptions{})
		t.DeleteE(ctx, &porchapi.PackageRevision{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: t.Namespace,
				Name:      pkgrev.Name,
			},
		})
	}
}

func (t *PorchSuite) TestCloneLeadingSlash(ctx context.Context) {
	const (
		repository  = "clone-ls"
		packageName = "test-clone-ls"
		revision    = "v1"
		workspace   = "workspace"
	)

	t.RegisterMainGitRepositoryF(ctx, repository)

	// Clone the package. Use leading slash in the directory (regression test)
	new := &porchapi.PackageRevision{
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
			RepositoryName: repository,
			Tasks: []porchapi.Task{
				{
					Type: porchapi.TaskTypeClone,
					Clone: &porchapi.PackageCloneTaskSpec{
						Upstream: porchapi.UpstreamPackage{
							Type: porchapi.RepositoryTypeGit,
							Git: &porchapi.GitPackage{
								Repo:      "https://github.com/platkrm/test-blueprints",
								Ref:       "basens/v1",
								Directory: "/basens",
							},
						},
						Strategy: porchapi.ResourceMerge,
					},
				},
			},
		},
	}

	t.CreateF(ctx, new)

	var pr porchapi.PackageRevision
	t.MustExist(ctx, client.ObjectKey{Namespace: t.Namespace, Name: new.Name}, &pr)
}

func (t *PorchSuite) TestPackageUpdate(ctx context.Context) {
	const (
		gitRepository = "package-update"
	)

	t.RegisterGitRepositoryF(ctx, testBlueprintsRepo, "test-blueprints", "")

	var list porchapi.PackageRevisionList
	t.ListE(ctx, &list, client.InNamespace(t.Namespace))

	basensV1 := MustFindPackageRevision(t.T, &list, repository.PackageRevisionKey{Repository: "test-blueprints", Package: "basens", Revision: "v1"})
	basensV2 := MustFindPackageRevision(t.T, &list, repository.PackageRevisionKey{Repository: "test-blueprints", Package: "basens", Revision: "v2"})

	// Register the repository as 'downstream'
	t.RegisterMainGitRepositoryF(ctx, gitRepository)

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
			PackageName:    "testns",
			WorkspaceName:  "test-workspace",
			RepositoryName: gitRepository,
			Tasks: []porchapi.Task{
				{
					Type: porchapi.TaskTypeClone,
					Clone: &porchapi.PackageCloneTaskSpec{
						Upstream: porchapi.UpstreamPackage{
							UpstreamRef: &porchapi.PackageRevisionRef{
								Name: basensV1.Name,
							},
						},
					},
				},
			},
		},
	}

	t.CreateF(ctx, pr)

	var revisionResources porchapi.PackageRevisionResources
	t.GetF(ctx, client.ObjectKey{
		Namespace: t.Namespace,
		Name:      pr.Name,
	}, &revisionResources)

	filename := filepath.Join("testdata", "update-resources", "add-config-map.yaml")
	cm, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("Failed to read ConfigMap from %q: %v", filename, err)
	}
	revisionResources.Spec.Resources["config-map.yaml"] = string(cm)
	t.UpdateF(ctx, &revisionResources)

	var newrr porchapi.PackageRevisionResources
	t.GetF(ctx, client.ObjectKey{
		Namespace: t.Namespace,
		Name:      pr.Name,
	}, &newrr)

	by, _ := yaml.Marshal(&newrr)
	t.Logf("PRR: %s", string(by))

	t.GetF(ctx, client.ObjectKey{
		Namespace: t.Namespace,
		Name:      pr.Name,
	}, pr)

	upstream := pr.Spec.Tasks[0].Clone.Upstream.DeepCopy()
	upstream.UpstreamRef.Name = basensV2.Name
	pr.Spec.Tasks = append(pr.Spec.Tasks, porchapi.Task{
		Type: porchapi.TaskTypeUpdate,
		Update: &porchapi.PackageUpdateTaskSpec{
			Upstream: *upstream,
		},
	})

	t.UpdateE(ctx, pr, &client.UpdateOptions{})

	t.GetF(ctx, client.ObjectKey{
		Namespace: t.Namespace,
		Name:      pr.Name,
	}, &revisionResources)

	if _, found := revisionResources.Spec.Resources["resourcequota.yaml"]; !found {
		t.Errorf("Updated package should contain 'resourcequota.yaml` file")
	}
}

func (t *PorchSuite) TestConcurrentPackageUpdates(ctx context.Context) {
	const (
		gitRepository = "package-update"
		packageName   = "testns-concurrent"
		workspace     = "test-workspace"
	)

	t.RegisterGitRepositoryF(ctx, testBlueprintsRepo, "test-blueprints", "")

	var list porchapi.PackageRevisionList
	t.ListE(ctx, &list, client.InNamespace(t.Namespace))

	basensV1 := MustFindPackageRevision(t.T, &list, repository.PackageRevisionKey{Repository: "test-blueprints", Package: "basens", Revision: "v1"})
	basensV2 := MustFindPackageRevision(t.T, &list, repository.PackageRevisionKey{Repository: "test-blueprints", Package: "basens", Revision: "v2"})

	// Register the repository as 'downstream'
	t.RegisterMainGitRepositoryF(ctx, gitRepository)

	// Create PackageRevision from upstream repo
	pr := t.CreatePackageSkeleton(gitRepository, packageName, workspace)
	pr.Spec.Tasks = []porchapi.Task{
		{
			Type: porchapi.TaskTypeClone,
			Clone: &porchapi.PackageCloneTaskSpec{
				Upstream: porchapi.UpstreamPackage{
					UpstreamRef: &porchapi.PackageRevisionRef{
						Name: basensV1.Name,
					},
				},
			},
		},
	}
	t.CreateF(ctx, pr)

	upstream := pr.Spec.Tasks[0].Clone.Upstream.DeepCopy()
	upstream.UpstreamRef.Name = basensV2.Name
	pr.Spec.Tasks = append(pr.Spec.Tasks, porchapi.Task{
		Type: porchapi.TaskTypeUpdate,
		Update: &porchapi.PackageUpdateTaskSpec{
			Upstream: *upstream,
		},
	})

	// Two clients at the same time try to update the downstream package
	updateFunction := func() any {
		return t.Client.Update(ctx, pr, &client.UpdateOptions{})
	}
	results := RunInParallel(updateFunction, updateFunction)

	assert.Contains(t, results, nil, "expected one request to succeed, but did not happen - results: %v", results)

	conflictFailurePresent := slices.ContainsFunc(results, func(eachResult any) bool {
		return eachResult != nil && strings.Contains(eachResult.(error).Error(), "another request is already in progress")
	})
	assert.True(t, conflictFailurePresent, "expected one request to fail with a conflict, but did not happen - results: %v", results)
}

func (t *PorchSuite) TestRegisterRepository(ctx context.Context) {
	const (
		repository = "register"
	)
	t.RegisterMainGitRepositoryF(ctx, repository,
		withType(configapi.RepositoryTypeGit),
		WithDeployment())

	var repo configapi.Repository
	t.GetF(ctx, client.ObjectKey{
		Namespace: t.Namespace,
		Name:      repository,
	}, &repo)

	if got, want := repo.Spec.Type, configapi.RepositoryTypeGit; got != want {
		t.Errorf("Repo Type: got %q, want %q", got, want)
	}
	if got, want := repo.Spec.Deployment, true; got != want {
		t.Errorf("Repo Deployment: got %t, want %t", got, want)
	}
}

func (t *PorchSuite) TestBuiltinFunctionEvaluator(ctx context.Context) {
	// Register the repository as 'git-fn'
	t.RegisterMainGitRepositoryF(ctx, "git-builtin-fn")

	// Create Package Revision
	pr := &porchapi.PackageRevision{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: t.Namespace,
		},
		Spec: porchapi.PackageRevisionSpec{
			PackageName:    "test-builtin-fn-bucket",
			WorkspaceName:  "test-workspace",
			RepositoryName: "git-builtin-fn",
			Tasks: []porchapi.Task{
				{
					Type: "clone",
					Clone: &porchapi.PackageCloneTaskSpec{
						Upstream: porchapi.UpstreamPackage{
							Type: "git",
							Git: &porchapi.GitPackage{
								Repo:      "https://github.com/GoogleCloudPlatform/blueprints.git",
								Ref:       "bucket-blueprint-v0.4.3",
								Directory: "catalog/bucket",
							},
						},
					},
				},
				{
					Type: "eval",
					Eval: &porchapi.FunctionEvalTaskSpec{
						//
						Image: "gcr.io/kpt-fn/starlark:v0.4.3",
						ConfigMap: map[string]string{
							"source": `for resource in ctx.resource_list["items"]:
  resource["metadata"]["annotations"]["foo"] = "bar"`,
						},
					},
				},
				{
					Type: "eval",
					Eval: &porchapi.FunctionEvalTaskSpec{
						Image: "gcr.io/kpt-fn/set-namespace:v0.4.1",
						ConfigMap: map[string]string{
							"namespace": "bucket-namespace",
						},
					},
				},
				// TODO: add test for apply-replacements, we can't do it now because FunctionEvalTaskSpec doesn't allow
				// non-ConfigMap functionConfig due to a code generator issue when dealing with unstructured.
			},
		},
	}
	t.CreateF(ctx, pr)

	// Get package resources
	var resources porchapi.PackageRevisionResources
	t.GetF(ctx, client.ObjectKey{
		Namespace: t.Namespace,
		Name:      pr.Name,
	}, &resources)

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

func (t *PorchSuite) TestExecFunctionEvaluator(ctx context.Context) {
	// Register the repository as 'git-fn'
	t.RegisterMainGitRepositoryF(ctx, "git-fn")

	// Create Package Revision
	pr := &porchapi.PackageRevision{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: t.Namespace,
		},
		Spec: porchapi.PackageRevisionSpec{
			PackageName:    "test-fn-bucket",
			WorkspaceName:  "test-workspace",
			RepositoryName: "git-fn",
			Tasks: []porchapi.Task{
				{
					Type: "clone",
					Clone: &porchapi.PackageCloneTaskSpec{
						Upstream: porchapi.UpstreamPackage{
							Type: "git",
							Git: &porchapi.GitPackage{
								Repo:      "https://github.com/GoogleCloudPlatform/blueprints.git",
								Ref:       "bucket-blueprint-v0.4.3",
								Directory: "catalog/bucket",
							},
						},
					},
				},
				{
					Type: "eval",
					Eval: &porchapi.FunctionEvalTaskSpec{
						Image: "gcr.io/kpt-fn/starlark:v0.3.0",
						ConfigMap: map[string]string{
							"source": `# set the namespace on all resources
for resource in ctx.resource_list["items"]:
  # mutate the resource
  resource["metadata"]["namespace"] = "bucket-namespace"`,
						},
					},
				},
				{
					Type: "eval",
					Eval: &porchapi.FunctionEvalTaskSpec{
						Image: "gcr.io/kpt-fn/set-annotations:v0.1.4",
						ConfigMap: map[string]string{
							"foo": "bar",
						},
					},
				},
			},
		},
	}
	t.CreateF(ctx, pr)

	// Get package resources
	var resources porchapi.PackageRevisionResources
	t.GetF(ctx, client.ObjectKey{
		Namespace: t.Namespace,
		Name:      pr.Name,
	}, &resources)

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

func (t *PorchSuite) TestPodFunctionEvaluatorWithDistrolessImage(ctx context.Context) {
	if t.TestRunnerIsLocal {
		t.Skipf("Skipping due to not having pod evaluator in local mode")
	}

	t.RegisterMainGitRepositoryF(ctx, "git-fn-distroless")

	// Create Package Revision
	pr := &porchapi.PackageRevision{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: t.Namespace,
		},
		Spec: porchapi.PackageRevisionSpec{
			PackageName:    "test-fn-redis-bucket",
			WorkspaceName:  "test-description",
			RepositoryName: "git-fn-distroless",
			Tasks: []porchapi.Task{
				{
					Type: "clone",
					Clone: &porchapi.PackageCloneTaskSpec{
						Upstream: porchapi.UpstreamPackage{
							Type: "git",
							Git: &porchapi.GitPackage{
								Repo:      "https://github.com/GoogleCloudPlatform/blueprints.git",
								Ref:       "redis-bucket-blueprint-v0.3.2",
								Directory: "catalog/redis-bucket",
							},
						},
					},
				},
				{
					Type: "patch",
					Patch: &porchapi.PackagePatchTaskSpec{
						Patches: []porchapi.PatchSpec{
							{
								File: "configmap.yaml",
								Contents: `apiVersion: v1
kind: ConfigMap
metadata:
  name: kptfile.kpt.dev
data:
  name: bucket-namespace
`,
								PatchType: porchapi.PatchTypeCreateFile,
							},
						},
					},
				},
				{
					Type: "eval",
					Eval: &porchapi.FunctionEvalTaskSpec{
						// This image is a mirror of gcr.io/cad-demo-sdk/set-namespace@sha256:462e44020221e72e3eb337ee59bc4bc3e5cb50b5ed69d377f55e05bec3a93d11
						// which uses gcr.io/distroless/base-debian11:latest as the base image.
						Image: "gcr.io/kpt-fn-demo/set-namespace:v0.1.0",
					},
				},
			},
		},
	}
	t.CreateF(ctx, pr)

	// Get package resources
	var resources porchapi.PackageRevisionResources
	t.GetF(ctx, client.ObjectKey{
		Namespace: t.Namespace,
		Name:      pr.Name,
	}, &resources)

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

func (t *PorchSuite) TestPodEvaluator(ctx context.Context) {
	if t.TestRunnerIsLocal {
		t.Skipf("Skipping due to not having pod evaluator in local mode")
	}

	const (
		generateFolderImage = "gcr.io/kpt-fn/generate-folders:v0.1.1" // This function is a TS based function.
		setAnnotationsImage = "gcr.io/kpt-fn/set-annotations:v0.1.3"  // set-annotations:v0.1.3 is an older version that porch maps neither to built-in nor exec.
	)

	// Register the repository as 'git-fn'
	t.RegisterMainGitRepositoryF(ctx, "git-fn-pod")

	// Create Package Revision
	pr := &porchapi.PackageRevision{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: t.Namespace,
		},
		Spec: porchapi.PackageRevisionSpec{
			PackageName:    "test-fn-pod-hierarchy",
			WorkspaceName:  "workspace-1",
			RepositoryName: "git-fn-pod",
			Tasks: []porchapi.Task{
				{
					Type: "clone",
					Clone: &porchapi.PackageCloneTaskSpec{
						Upstream: porchapi.UpstreamPackage{
							Type: "git",
							Git: &porchapi.GitPackage{
								Repo:      "https://github.com/GoogleCloudPlatform/blueprints.git",
								Ref:       "783380ce4e6c3f21e9e90055b3a88bada0410154",
								Directory: "catalog/hierarchy/simple",
							},
						},
					},
				},
				// Testing pod evaluator with TS function
				{
					Type: "eval",
					Eval: &porchapi.FunctionEvalTaskSpec{
						Image: generateFolderImage,
					},
				},
				// Testing pod evaluator with golang function
				{
					Type: "eval",
					Eval: &porchapi.FunctionEvalTaskSpec{
						Image: setAnnotationsImage,
						ConfigMap: map[string]string{
							"test-key": "test-val",
						},
					},
				},
			},
		},
	}
	t.CreateF(ctx, pr)

	// Get package resources
	var resources porchapi.PackageRevisionResources
	t.GetF(ctx, client.ObjectKey{
		Namespace: t.Namespace,
		Name:      pr.Name,
	}, &resources)

	counter := 0
	for name, obj := range resources.Spec.Resources {
		if strings.HasPrefix(name, "hierarchy/") {
			counter++
			node, err := yaml.Parse(obj)
			if err != nil {
				t.Errorf("failed to parse Folder object: %v", err)
			}
			if node.GetAnnotations()["test-key"] != "test-val" {
				t.Errorf("Folder should contain annotation `test-key:test-val`, the annotations we got: %v", node.GetAnnotations())
			}
		}
	}
	if counter != 4 {
		t.Errorf("expected 4 Folder objects, but got %v", counter)
	}

	// Get the fn runner pods and delete them.
	podList := &corev1.PodList{}
	t.ListF(ctx, podList, client.InNamespace("porch-fn-system"))
	for _, pod := range podList.Items {
		img := pod.Spec.Containers[0].Image
		if img == generateFolderImage || img == setAnnotationsImage {
			t.DeleteF(ctx, &pod)
		}
	}

	// Create another Package Revision
	pr2 := &porchapi.PackageRevision{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: t.Namespace,
		},
		Spec: porchapi.PackageRevisionSpec{
			PackageName:    "test-fn-pod-hierarchy",
			WorkspaceName:  "workspace-2",
			RepositoryName: "git-fn-pod",
			Tasks: []porchapi.Task{
				{
					Type: "clone",
					Clone: &porchapi.PackageCloneTaskSpec{
						Upstream: porchapi.UpstreamPackage{
							Type: "git",
							Git: &porchapi.GitPackage{
								Repo:      "https://github.com/GoogleCloudPlatform/blueprints.git",
								Ref:       "783380ce4e6c3f21e9e90055b3a88bada0410154",
								Directory: "catalog/hierarchy/simple",
							},
						},
					},
				},
				// Testing pod evaluator with TS function
				{
					Type: "eval",
					Eval: &porchapi.FunctionEvalTaskSpec{
						Image: generateFolderImage,
					},
				},
				// Testing pod evaluator with golang function
				{
					Type: "eval",
					Eval: &porchapi.FunctionEvalTaskSpec{
						Image: setAnnotationsImage,
						ConfigMap: map[string]string{
							"new-test-key": "new-test-val",
						},
					},
				},
			},
		},
	}
	t.CreateF(ctx, pr2)

	// Get package resources
	t.GetF(ctx, client.ObjectKey{
		Namespace: t.Namespace,
		Name:      pr2.Name,
	}, &resources)

	counter = 0
	for name, obj := range resources.Spec.Resources {
		if strings.HasPrefix(name, "hierarchy/") {
			counter++
			node, err := yaml.Parse(obj)
			if err != nil {
				t.Errorf("failed to parse Folder object: %v", err)
			}
			if node.GetAnnotations()["new-test-key"] != "new-test-val" {
				t.Errorf("Folder should contain annotation `test-key:test-val`, the annotations we got: %v", node.GetAnnotations())
			}
		}
	}
	if counter != 4 {
		t.Errorf("expected 4 Folder objects, but got %v", counter)
	}
}

func (t *PorchSuite) TestPodEvaluatorWithFailure(ctx context.Context) {
	if t.TestRunnerIsLocal {
		t.Skipf("Skipping due to not having pod evaluator in local mode")
	}

	t.RegisterMainGitRepositoryF(ctx, "git-fn-pod-failure")

	// Create Package Revision
	pr := &porchapi.PackageRevision{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: t.Namespace,
		},
		Spec: porchapi.PackageRevisionSpec{
			PackageName:    "test-fn-pod-bucket",
			WorkspaceName:  "workspace",
			RepositoryName: "git-fn-pod-failure",
			Tasks: []porchapi.Task{
				{
					Type: "clone",
					Clone: &porchapi.PackageCloneTaskSpec{
						Upstream: porchapi.UpstreamPackage{
							Type: "git",
							Git: &porchapi.GitPackage{
								Repo:      "https://github.com/GoogleCloudPlatform/blueprints.git",
								Ref:       "bucket-blueprint-v0.4.3",
								Directory: "catalog/bucket",
							},
						},
					},
				},
				{
					Type: "eval",
					Eval: &porchapi.FunctionEvalTaskSpec{
						// This function is expect to fail due to not knowing schema for some CRDs.
						Image: "gcr.io/kpt-fn/kubeval:v0.2.0",
					},
				},
			},
		},
	}
	err := t.Client.Create(ctx, pr)
	expectedErrMsg := "Validating arbitrary CRDs is not supported"
	if err == nil || !strings.Contains(err.Error(), expectedErrMsg) {
		t.Fatalf("expected the error to contain %q, but got %v", expectedErrMsg, err)
	}
}

func (t *PorchSuite) TestLargePackageRevision(ctx context.Context) {
	const (
		setAnnotationsImage = "gcr.io/kpt-fn/set-annotations:v0.1.3" // set-annotations:v0.1.3 is an older version that porch maps neither to built-in nor exec.
		testDataSize        = 5 * 1024 * 1024
	)

	t.RegisterMainGitRepositoryF(ctx, "git-fn-pod-large")

	// Create Package Revision
	pr := &porchapi.PackageRevision{
		TypeMeta: metav1.TypeMeta{
			Kind:       porchapi.PackageRevisionGVR.Resource,
			APIVersion: porchapi.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: t.Namespace,
		},
		Spec: porchapi.PackageRevisionSpec{
			PackageName:    "new-package",
			WorkspaceName:  "workspace",
			RepositoryName: "git-fn-pod-large",
			Tasks: []porchapi.Task{
				{
					Type: porchapi.TaskTypeInit,
					Init: &porchapi.PackageInitTaskSpec{
						Description: "this is a test",
					},
				},
			},
		},
	}

	t.CreateF(ctx, pr)

	var prr porchapi.PackageRevisionResources
	t.GetF(ctx, client.ObjectKeyFromObject(pr), &prr)

	if !t.TestRunnerIsLocal {
		// pod evaluator is not available in local test mode, skip testing it
		prr.Spec.Resources["Kptfile"] += `
pipeline:
  mutators:
  - image: ` + setAnnotationsImage + `
    configMap:
      test-key: test-val
    selectors:
       - name: test-data
`
	}

	prr.Spec.Resources["largefile.yaml"] = `
apiVersion: v1
kind: ConfigMap
metadata:
  name: test-data
  labels:
    something: somewhere
data:
  value: "` + strings.Repeat("a", testDataSize) + `"
`

	t.UpdateF(ctx, &prr)

	rs := prr.Status.RenderStatus
	if rs.Err != "" || rs.Result.ExitCode != 0 {
		t.Fatalf("Couldn't render large package! exit code: %v,\n%v", rs.Result.ExitCode, rs.Err)
	}

	// Get package resources
	t.GetF(ctx, client.ObjectKeyFromObject(pr), &prr)

	for name, obj := range prr.Spec.Resources {
		if !strings.HasSuffix(name, ".yaml") {
			continue
		}
		node, err := yaml.Parse(obj)
		if err != nil {
			t.Errorf("failed to parse object: %v", err)
		}
		if node.GetName() == "test-data" {
			f := node.Field("data")
			if f.IsNilOrEmpty() {
				t.Fatalf("couldn't find data field in test-data")
			}
			long_string, err := f.Value.GetString("value")
			if err != nil {
				t.Fatalf("couldn't find large string in test-data: %v", err)
			}
			if len(long_string) != testDataSize {
				t.Fatalf("large string size mismatch. want: %v, got: %v", testDataSize, len(long_string))
			}
			if !t.TestRunnerIsLocal && (node.GetAnnotations()["test-key"] != "test-val") {
				t.Errorf("Object (%s %q) should contain annotation `test-key:test-val`, but we got: %v", node.GetKind(), node.GetName(), node.GetAnnotations())
			}
		}
	}
}

func (t *PorchSuite) TestRepositoryError(ctx context.Context) {
	const (
		repositoryName = "repo-with-error"
	)
	t.CreateF(ctx, &configapi.Repository{
		TypeMeta: metav1.TypeMeta{
			Kind:       configapi.TypeRepository.Kind,
			APIVersion: configapi.TypeRepository.APIVersion(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      repositoryName,
			Namespace: t.Namespace,
		},
		Spec: configapi.RepositorySpec{
			Description: "Repository With Error",
			Type:        configapi.RepositoryTypeGit,
			Git: &configapi.GitRepository{
				// Use `invalid` domain: https://www.rfc-editor.org/rfc/rfc6761#section-6.4
				Repo: "https://repo.invalid/repository.git",
			},
		},
	})
	t.Cleanup(func() {
		t.DeleteL(ctx, &configapi.Repository{
			ObjectMeta: metav1.ObjectMeta{
				Name:      repositoryName,
				Namespace: t.Namespace,
			},
		})
	})

	giveUp := time.Now().Add(60 * time.Second)

	for {
		if time.Now().After(giveUp) {
			t.Errorf("Timed out waiting for Repository Condition")
			break
		}

		time.Sleep(5 * time.Second)

		var repository configapi.Repository
		t.GetF(ctx, client.ObjectKey{
			Namespace: t.Namespace,
			Name:      repositoryName,
		}, &repository)

		available := meta.FindStatusCondition(repository.Status.Conditions, configapi.RepositoryReady)
		if available == nil {
			// Condition not yet set
			t.Logf("Repository condition not yet available")
			continue
		}

		if got, want := available.Status, metav1.ConditionFalse; got != want {
			t.Errorf("Repository Available Condition Status; got %q, want %q", got, want)
		}
		if got, want := available.Reason, configapi.ReasonError; got != want {
			t.Errorf("Repository Available Condition Reason: got %q, want %q", got, want)
		}
		break
	}
}

func (t *PorchSuite) TestNewPackageRevisionLabels(ctx context.Context) {
	const (
		repository = "pkg-rev-labels"
		labelKey1  = "kpt.dev/label"
		labelVal1  = "foo"
		labelKey2  = "kpt.dev/other-label"
		labelVal2  = "bar"
		annoKey1   = "kpt.dev/anno"
		annoVal1   = "foo"
		annoKey2   = "kpt.dev/other-anno"
		annoVal2   = "bar"
	)

	t.RegisterMainGitRepositoryF(ctx, repository)

	// Create a package with labels and annotations.
	pr := porchapi.PackageRevision{
		TypeMeta: metav1.TypeMeta{
			Kind:       "PackageRevision",
			APIVersion: porchapi.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: t.Namespace,
			Labels: map[string]string{
				labelKey1: labelVal1,
			},
			Annotations: map[string]string{
				annoKey1: annoVal1,
				annoKey2: annoVal2,
			},
		},
		Spec: porchapi.PackageRevisionSpec{
			PackageName:    "new-package",
			WorkspaceName:  "workspace",
			RepositoryName: repository,
			Tasks: []porchapi.Task{
				{
					Type: porchapi.TaskTypeInit,
					Init: &porchapi.PackageInitTaskSpec{
						Description: "this is a test",
					},
				},
			},
		},
	}
	t.CreateF(ctx, &pr)
	t.ValidateLabelsAndAnnos(ctx, pr.Name,
		map[string]string{
			labelKey1: labelVal1,
		},
		map[string]string{
			annoKey1: annoVal1,
			annoKey2: annoVal2,
		},
	)

	// Propose the package.
	pr.Spec.Lifecycle = porchapi.PackageRevisionLifecycleProposed
	t.UpdateF(ctx, &pr)

	// retrieve the updated object
	t.GetF(ctx, client.ObjectKey{
		Namespace: pr.Namespace,
		Name:      pr.Name,
	}, &pr)

	t.ValidateLabelsAndAnnos(ctx, pr.Name,
		map[string]string{
			labelKey1: labelVal1,
		},
		map[string]string{
			annoKey1: annoVal1,
			annoKey2: annoVal2,
		},
	)

	// Approve the package
	pr.Spec.Lifecycle = porchapi.PackageRevisionLifecyclePublished
	_ = t.UpdateApprovalF(ctx, &pr, metav1.UpdateOptions{})
	t.ValidateLabelsAndAnnos(ctx, pr.Name,
		map[string]string{
			labelKey1:                         labelVal1,
			porchapi.LatestPackageRevisionKey: porchapi.LatestPackageRevisionValue,
		},
		map[string]string{
			annoKey1: annoVal1,
			annoKey2: annoVal2,
		},
	)

	// retrieve the updated object
	t.GetF(ctx, client.ObjectKey{
		Namespace: pr.Namespace,
		Name:      pr.Name,
	}, &pr)

	// Update the labels and annotations on the approved package.
	delete(pr.ObjectMeta.Labels, labelKey1)
	pr.ObjectMeta.Labels[labelKey2] = labelVal2
	delete(pr.ObjectMeta.Annotations, annoKey2)
	pr.Spec.Revision = "v1"
	t.UpdateF(ctx, &pr)
	t.ValidateLabelsAndAnnos(ctx, pr.Name,
		map[string]string{
			labelKey2:                         labelVal2,
			porchapi.LatestPackageRevisionKey: porchapi.LatestPackageRevisionValue,
		},
		map[string]string{
			annoKey1: annoVal1,
		},
	)

	// Create PackageRevision from upstream repo. Labels and annotations should
	// not be retained from upstream.
	clonedPr := porchapi.PackageRevision{
		TypeMeta: metav1.TypeMeta{
			Kind:       "PackageRevision",
			APIVersion: porchapi.SchemeGroupVersion.Identifier(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: t.Namespace,
		},
		Spec: porchapi.PackageRevisionSpec{
			PackageName:    "cloned-package",
			WorkspaceName:  "workspace",
			RepositoryName: repository,
			Tasks: []porchapi.Task{
				{
					Type: porchapi.TaskTypeClone,
					Clone: &porchapi.PackageCloneTaskSpec{
						Upstream: porchapi.UpstreamPackage{
							UpstreamRef: &porchapi.PackageRevisionRef{
								Name: pr.Name, // Package to be cloned
							},
						},
					},
				},
			},
		},
	}
	t.CreateF(ctx, &clonedPr)
	t.ValidateLabelsAndAnnos(ctx, clonedPr.Name,
		map[string]string{},
		map[string]string{},
	)
}

func (t *PorchSuite) TestRegisteredPackageRevisionLabels(ctx context.Context) {
	const (
		labelKey = "kpt.dev/label"
		labelVal = "foo"
		annoKey  = "kpt.dev/anno"
		annoVal  = "foo"
	)

	t.RegisterGitRepositoryF(ctx, testBlueprintsRepo, "test-blueprints", "")

	var list porchapi.PackageRevisionList
	t.ListE(ctx, &list, client.InNamespace(t.Namespace))

	basens := MustFindPackageRevision(t.T, &list, repository.PackageRevisionKey{Repository: "test-blueprints", Package: "basens", Revision: "v1"})
	if basens.ObjectMeta.Labels == nil {
		basens.ObjectMeta.Labels = make(map[string]string)
	}
	basens.ObjectMeta.Labels[labelKey] = labelVal
	if basens.ObjectMeta.Annotations == nil {
		basens.ObjectMeta.Annotations = make(map[string]string)
	}
	basens.ObjectMeta.Annotations[annoKey] = annoVal
	t.UpdateF(ctx, basens)

	t.ValidateLabelsAndAnnos(ctx, basens.Name,
		map[string]string{
			labelKey: labelVal,
		},
		map[string]string{
			annoKey: annoVal,
		},
	)
}

func (t *PorchSuite) TestPackageRevisionGCWithOwner(ctx context.Context) {
	const (
		repository  = "pkgrevgcwithowner"
		workspace   = "pkgrevgcwithowner-workspace"
		description = "empty-package description"
		cmName      = "foo"
	)

	t.RegisterMainGitRepositoryF(ctx, repository)

	// Create a new package (via init)
	pr := &porchapi.PackageRevision{
		TypeMeta: metav1.TypeMeta{
			Kind:       packageRevisionGVK.Kind,
			APIVersion: packageRevisionGVK.GroupVersion().String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: t.Namespace,
		},
		Spec: porchapi.PackageRevisionSpec{
			PackageName:    "empty-package",
			WorkspaceName:  workspace,
			RepositoryName: repository,
		},
	}
	t.CreateF(ctx, pr)

	cm := &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			Kind:       configMapGVK.Kind,
			APIVersion: configMapGVK.GroupVersion().String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      cmName,
			Namespace: t.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: porchapi.SchemeGroupVersion.String(),
					Kind:       packageRevisionGVK.Kind,
					Name:       pr.Name,
					UID:        pr.UID,
				},
			},
		},
		Data: map[string]string{
			"foo": "bar",
		},
	}
	t.CreateF(ctx, cm)

	t.DeleteF(ctx, pr)
	t.WaitUntilObjectDeleted(
		ctx,
		packageRevisionGVK,
		types.NamespacedName{
			Name:      pr.Name,
			Namespace: pr.Namespace,
		},
		10*time.Second,
	)
	t.WaitUntilObjectDeleted(
		ctx,
		configMapGVK,
		types.NamespacedName{
			Name:      cm.Name,
			Namespace: cm.Namespace,
		},
		10*time.Second,
	)
}

func (t *PorchSuite) TestPackageRevisionGCAsOwner(ctx context.Context) {
	const (
		repository  = "pkgrevgcasowner"
		workspace   = "pkgrevgcasowner-workspace"
		description = "empty-package description"
		cmName      = "foo"
	)

	t.RegisterMainGitRepositoryF(ctx, repository)

	cm := &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			Kind:       configMapGVK.Kind,
			APIVersion: configMapGVK.GroupVersion().String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      cmName,
			Namespace: t.Namespace,
		},
		Data: map[string]string{
			"foo": "bar",
		},
	}
	t.CreateF(ctx, cm)

	pr := &porchapi.PackageRevision{
		TypeMeta: metav1.TypeMeta{
			Kind:       packageRevisionGVK.Kind,
			APIVersion: packageRevisionGVK.GroupVersion().String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: t.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "v1",
					Kind:       "ConfigMap",
					Name:       cm.Name,
					UID:        cm.UID,
				},
			},
		},
		Spec: porchapi.PackageRevisionSpec{
			PackageName:    "empty-package",
			WorkspaceName:  workspace,
			RepositoryName: repository,
		},
	}
	t.CreateF(ctx, pr)

	t.DeleteF(ctx, cm)
	t.WaitUntilObjectDeleted(
		ctx,
		configMapGVK,
		types.NamespacedName{
			Name:      cm.Name,
			Namespace: cm.Namespace,
		},
		10*time.Second,
	)
	t.WaitUntilObjectDeleted(
		ctx,
		packageRevisionGVK,
		types.NamespacedName{
			Name:      pr.Name,
			Namespace: pr.Namespace,
		},
		10*time.Second,
	)
}

func (t *PorchSuite) TestPackageRevisionOwnerReferences(ctx context.Context) {
	const (
		repository  = "pkgrevownerrefs"
		workspace   = "pkgrevownerrefs-workspace"
		description = "empty-package description"
		cmName      = "foo"
	)

	t.RegisterMainGitRepositoryF(ctx, repository)

	cm := &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			Kind:       configMapGVK.Kind,
			APIVersion: configMapGVK.GroupVersion().String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      cmName,
			Namespace: t.Namespace,
		},
		Data: map[string]string{
			"foo": "bar",
		},
	}
	t.CreateF(ctx, cm)

	pr := &porchapi.PackageRevision{
		TypeMeta: metav1.TypeMeta{
			Kind:       packageRevisionGVK.Kind,
			APIVersion: packageRevisionGVK.GroupVersion().String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: t.Namespace,
		},
		Spec: porchapi.PackageRevisionSpec{
			PackageName:    "empty-package",
			WorkspaceName:  workspace,
			RepositoryName: repository,
		},
	}
	t.CreateF(ctx, pr)
	t.ValidateOwnerReferences(ctx, pr.Name, []metav1.OwnerReference{})

	ownerRef := metav1.OwnerReference{
		APIVersion: "v1",
		Kind:       "ConfigMap",
		Name:       cm.Name,
		UID:        cm.UID,
	}
	pr.ObjectMeta.OwnerReferences = []metav1.OwnerReference{ownerRef}
	t.UpdateF(ctx, pr)
	t.ValidateOwnerReferences(ctx, pr.Name, []metav1.OwnerReference{ownerRef})

	pr.ObjectMeta.OwnerReferences = []metav1.OwnerReference{}
	t.UpdateF(ctx, pr)
	t.ValidateOwnerReferences(ctx, pr.Name, []metav1.OwnerReference{})
}

func (t *PorchSuite) TestPackageRevisionFinalizers(ctx context.Context) {
	const (
		repository  = "pkgrevfinalizers"
		workspace   = "pkgrevfinalizers-workspace"
		description = "empty-package description"
	)

	t.RegisterMainGitRepositoryF(ctx, repository)

	pr := &porchapi.PackageRevision{
		TypeMeta: metav1.TypeMeta{
			Kind:       packageRevisionGVK.Kind,
			APIVersion: packageRevisionGVK.GroupVersion().String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: t.Namespace,
		},
		Spec: porchapi.PackageRevisionSpec{
			PackageName:    "empty-package",
			WorkspaceName:  workspace,
			RepositoryName: repository,
		},
	}
	t.CreateF(ctx, pr)
	t.ValidateFinalizers(ctx, pr.Name, []string{})

	pr.Finalizers = append(pr.Finalizers, "foo-finalizer")
	t.UpdateF(ctx, pr)
	t.ValidateFinalizers(ctx, pr.Name, []string{"foo-finalizer"})

	t.DeleteF(ctx, pr)
	t.ValidateFinalizers(ctx, pr.Name, []string{"foo-finalizer"})

	pr.Finalizers = []string{}
	t.UpdateF(ctx, pr)
	t.WaitUntilObjectDeleted(ctx, packageRevisionGVK, types.NamespacedName{
		Name:      pr.Name,
		Namespace: pr.Namespace,
	}, 10*time.Second)
}

func (t *PorchSuite) TestPackageRevisionInMultipleNamespaces(ctx context.Context) {

	registerRepoAndTestRevisions := func(repoName string, ns string, oldPRs []porchapi.PackageRevision) []porchapi.PackageRevision {

		t.RegisterGitRepositoryF(ctx, testBlueprintsRepo, repoName, "", InNamespace(ns))
		prList := porchapi.PackageRevisionList{}
		t.ListF(ctx, &prList, client.InNamespace(ns))
		newPRs := prList.Items
		if len(newPRs) == 0 {
			t.Errorf("no PackageRevisions found in repo: %s", repoName)
		}

		// remove PRs that were in the namespace before repo registration
		for _, oldPR := range oldPRs {
			for i, pr := range newPRs {
				if pr.Name == oldPR.Name &&
					pr.Namespace == oldPR.Namespace &&
					pr.Spec.RepositoryName == oldPR.Spec.RepositoryName &&
					pr.Spec.PackageName == oldPR.Spec.PackageName &&
					pr.Spec.WorkspaceName == oldPR.Spec.WorkspaceName {

					newPRs[i] = newPRs[len(newPRs)-1]
					newPRs = newPRs[:len(newPRs)-1]
					break // only remove oldPR once
				}
			}
		}

		// test PRs that came from the new repo
		for _, pr := range newPRs {
			if !strings.HasPrefix(pr.Name, repoName) {
				t.Errorf("PR name: want prefix: %s, got %s", repoName, pr.Name)
			}
			if pr.Spec.RepositoryName != repoName {
				t.Errorf("PR repository: want: %s, got %s", repoName, pr.Spec.RepositoryName)
			}

			if pr.Namespace != ns {
				t.Errorf("PR %s namespace: want %s, got %s", pr.Name, ns, pr.Namespace)
			}
		}

		return newPRs
	}

	ns2 := &corev1.Namespace{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Namespace",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: t.Namespace + "-2",
		},
	}
	t.CreateF(ctx, ns2)
	t.Cleanup(func() {
		t.DeleteE(ctx, ns2)
	})

	prs1 := registerRepoAndTestRevisions("test-blueprints", t.Namespace, nil)
	nPRs := len(prs1)
	t.Logf("Number of PRs in repo: %v", nPRs)

	prs2 := registerRepoAndTestRevisions("test-2-blueprints", ns2.Name, nil)
	if len(prs2) != nPRs {
		t.Errorf("number of PackageRevisions in namespace %s: want %v, got %d", ns2.Name, nPRs, len(prs2))
	}

	prs3 := registerRepoAndTestRevisions("test-3-blueprints", t.Namespace, prs1)
	if len(prs3) != nPRs {
		t.Errorf("number of PackageRevisions in repo-3: want %v, got %d", nPRs, len(prs2))
	}
}

func (t *PorchSuite) TestUniquenessOfUIDs(ctx context.Context) {

	t.RegisterGitRepositoryF(ctx, testBlueprintsRepo, "test-blueprints", "")
	t.RegisterGitRepositoryF(ctx, testBlueprintsRepo, "test-2-blueprints", "")

	prList := porchapi.PackageRevisionList{}
	t.ListE(ctx, &prList, client.InNamespace(t.Namespace))

	uids := make(map[types.UID]*porchapi.PackageRevision)
	for _, pr := range prList.Items {
		otherPr, found := uids[pr.UID]
		if found {
			t.Errorf("PackageRevision %s/%s has the same UID as %s/%s: %v", pr.Namespace, pr.Name, otherPr.Namespace, otherPr.Name, pr.UID)
		}
		uids[pr.UID] = &pr
	}
}

func (t *PorchSuite) TestPackageRevisionFieldSelectors(ctx context.Context) {
	repo := "test-blueprints"
	t.RegisterGitRepositoryF(ctx, testBlueprintsRepo, repo, "")

	prList := porchapi.PackageRevisionList{}

	pkgName := "basens"
	pkgSelector := client.MatchingFields(fields.Set{"spec.packageName": pkgName})
	t.ListE(ctx, &prList, client.InNamespace(t.Namespace), pkgSelector)
	if len(prList.Items) == 0 {
		t.Errorf("Expected at least one PackageRevision with packageName=%q, but got none", pkgName)
	}
	for _, pr := range prList.Items {
		if pr.Spec.PackageName != pkgName {
			t.Errorf("PackageRevision %s packageName: want %q, but got %q", pr.Name, pkgName, pr.Spec.PackageName)
		}
	}

	revName := "v1"
	revSelector := client.MatchingFields(fields.Set{"spec.revision": revName})
	t.ListE(ctx, &prList, client.InNamespace(t.Namespace), revSelector)
	if len(prList.Items) == 0 {
		t.Errorf("Expected at least one PackageRevision with revision=%q, but got none", revName)
	}
	for _, pr := range prList.Items {
		if pr.Spec.Revision != revName {
			t.Errorf("PackageRevision %s revision: want %q, but got %q", pr.Name, pkgName, pr.Spec.PackageName)
		}
	}

	wsName := "v1"
	wsSelector := client.MatchingFields(fields.Set{"spec.workspaceName": wsName})
	t.ListE(ctx, &prList, client.InNamespace(t.Namespace), wsSelector)
	if len(prList.Items) == 0 {
		t.Errorf("Expected at least one PackageRevision with workspaceName=%q, but got none", wsName)
	}
	for _, pr := range prList.Items {
		if pr.Spec.WorkspaceName != porchapi.WorkspaceName(wsName) {
			t.Errorf("PackageRevision %s workspaceName: want %q, but got %q", pr.Name, wsName, pr.Spec.WorkspaceName)
		}
	}

	publishedSelector := client.MatchingFields(fields.Set{"spec.lifecycle": string(porchapi.PackageRevisionLifecyclePublished)})
	t.ListE(ctx, &prList, client.InNamespace(t.Namespace), publishedSelector)
	if len(prList.Items) == 0 {
		t.Errorf("Expected at least one PackageRevision with lifecycle=%q, but got none", porchapi.PackageRevisionLifecyclePublished)
	}
	for _, pr := range prList.Items {
		if pr.Spec.Lifecycle != porchapi.PackageRevisionLifecyclePublished {
			t.Errorf("PackageRevision %s lifecycle: want %q, but got %q", pr.Name, porchapi.PackageRevisionLifecyclePublished, pr.Spec.Lifecycle)
		}
	}

	draftSelector := client.MatchingFields(fields.Set{"spec.lifecycle": string(porchapi.PackageRevisionLifecycleDraft)})
	t.ListE(ctx, &prList, client.InNamespace(t.Namespace), draftSelector)
	// TODO: add draft packages to the test repo
	// if len(prList.Items) == 0 {
	// 	t.Errorf("Expected at least one PackageRevision with lifecycle=%q, but got none", porchapi.PackageRevisionLifecycleDraft)
	// }
	for _, pr := range prList.Items {
		if pr.Spec.Lifecycle != porchapi.PackageRevisionLifecycleDraft {
			t.Errorf("PackageRevision %s lifecycle: want %q, but got %q", pr.Name, porchapi.PackageRevisionLifecycleDraft, pr.Spec.Lifecycle)
		}
	}

	// test combined selectors
	combinedSelector := client.MatchingFields(fields.Set{"spec.revision": revName, "spec.packageName": pkgName})
	t.ListE(ctx, &prList, client.InNamespace(t.Namespace), combinedSelector)
	if len(prList.Items) == 0 {
		t.Errorf("Expected at least one PackageRevision with packageName=%q and revision=%q, but got none", pkgName, revName)
	}
	for _, pr := range prList.Items {
		if pr.Spec.PackageName != pkgName || pr.Spec.Revision != revName {
			t.Errorf("PackageRevision %s: want %v/%v, but got %v/%v", pr.Name, pkgName, revName, pr.Spec.PackageName, pr.Spec.Revision)
		}
	}
}
