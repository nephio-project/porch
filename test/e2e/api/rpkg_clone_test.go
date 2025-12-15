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
	"os"
	"path/filepath"

	"github.com/google/go-cmp/cmp"
	porchapi "github.com/nephio-project/porch/api/porch/v1alpha1"
	kptfilev1 "github.com/nephio-project/porch/pkg/kpt/api/kptfile/v1"
	"github.com/nephio-project/porch/pkg/repository"
	suiteutils "github.com/nephio-project/porch/test/e2e/suiteutils"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)



func (t *PorchSuite) TestCloneFromUpstream() {
	testBlueprintsRepo := t.GetTestBlueprintsRepoURL()
	// Register Upstream Repository
	t.RegisterGitRepositoryF(testBlueprintsRepo, suiteutils.TestBlueprintsRepoName, "", suiteutils.GiteaUser, suiteutils.GiteaPassword)

	var list porchapi.PackageRevisionList
	t.ListE(&list, client.InNamespace(t.Namespace))

	upstreamPr := t.MustFindPackageRevision(&list, repository.PackageRevisionKey{
		PkgKey: repository.PackageKey{
			RepoKey: repository.RepositoryKey{
				Name: suiteutils.TestBlueprintsRepoName,
			},
			Package: "basens"},
		Revision: 1})

	// Register the repository as 'downstream'
	t.RegisterGitRepositoryF(t.GetPorchTestRepoURL(), suiteutils.PorchTestRepoName, "", suiteutils.GiteaUser, suiteutils.GiteaPassword)

	// Create PackageRevision from upstream repo
	clonedPr := t.CreatePackageSkeleton(suiteutils.PorchTestRepoName, "istions", "test-workspace")
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

	t.CreateF(clonedPr)

	// Get istions resources
	var istions porchapi.PackageRevisionResources
	t.GetF(client.ObjectKey{
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

func (t *PorchSuite) TestCloneIntoDeploymentRepository() {
	const downstreamPackage = "istions"
	const downstreamWorkspace = "test-workspace"

	testBlueprintsRepo := t.GetTestBlueprintsRepoURL()

	// Register the deployment repository
	t.RegisterGitRepositoryF(t.GetPorchTestRepoURL(), suiteutils.PorchTestRepoName, "", suiteutils.GiteaUser, suiteutils.GiteaPassword, suiteutils.RepositoryOptions{RepOpts: suiteutils.WithDeployment()})

	// Register the upstream repository
	t.RegisterGitRepositoryF(testBlueprintsRepo, suiteutils.TestBlueprintsRepoName, "", suiteutils.GiteaUser, suiteutils.GiteaPassword)

	var upstreamPackages porchapi.PackageRevisionList
	t.ListE(&upstreamPackages, client.InNamespace(t.Namespace))
	upstreamPackage := t.MustFindPackageRevision(&upstreamPackages, repository.PackageRevisionKey{
		PkgKey: repository.PackageKey{
			RepoKey: repository.RepositoryKey{
				Name: suiteutils.TestBlueprintsRepoName,
			},
			Package: "basens",
		},
		Revision:      1,
		WorkspaceName: "v1",
	})

	// Create PackageRevision from upstream repo
	pr := t.CreatePackageSkeleton(suiteutils.PorchTestRepoName, downstreamPackage, downstreamWorkspace)
	pr.Spec.Tasks = []porchapi.Task{
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
	}

	t.CreateF(pr)

	// Get istions resources
	var istions porchapi.PackageRevisionResources
	t.GetF(client.ObjectKey{
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

func (t *PorchSuite) TestCloneLeadingSlash() {
	const (
		repository = "clone-ls"
	)
	t.RegisterGitRepositoryF(t.GetPorchTestRepoURL(), repository, "", suiteutils.GiteaUser, suiteutils.GiteaPassword)

	// Clone the package. Use leading slash in the directory (regression test)
	new := t.CreatePackageCloneF(repository, "test-clone-ls", defaultWorkspace, "basens/v1", "/basens")

	var pr porchapi.PackageRevision
	t.MustExist(client.ObjectKey{Namespace: t.Namespace, Name: new.Name}, &pr)
}

func (t *PorchSuite) TestPackageUpgrade() {
	const (
		gitRepository = "package-upgrade"
	)

	t.RegisterGitRepositoryF(t.GetTestBlueprintsRepoURL(), suiteutils.TestBlueprintsRepoName, "", suiteutils.GiteaUser, suiteutils.GiteaPassword)

	// Register the repository as 'downstream'
	t.RegisterGitRepositoryF(t.GetPorchTestRepoURL(), gitRepository, "", suiteutils.GiteaUser, suiteutils.GiteaPassword)

	var list porchapi.PackageRevisionList
	t.ListE(&list, client.InNamespace(t.Namespace))

	basensV1 := t.MustFindPackageRevision(&list, repository.PackageRevisionKey{PkgKey: repository.PackageKey{RepoKey: repository.RepositoryKey{Name: "test-blueprints"}, Package: "basens"}, Revision: 1})
	basensV2 := t.MustFindPackageRevision(&list, repository.PackageRevisionKey{PkgKey: repository.PackageKey{RepoKey: repository.RepositoryKey{Name: "test-blueprints"}, Package: "basens"}, Revision: 2})

	// Create PackageRevision from upstream repo
	pr := t.CreatePackageSkeleton(gitRepository, "testns", "test-workspace")
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
	t.CreateF(pr)

	var revisionResources porchapi.PackageRevisionResources
	t.GetF(client.ObjectKey{
		Namespace: t.Namespace,
		Name:      pr.Name,
	}, &revisionResources)

	filename := filepath.Join("testdata", "update-resources", "add-config-map.yaml")
	cm, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("Failed to read ConfigMap from %q: %v", filename, err)
	}
	revisionResources.Spec.Resources["config-map.yaml"] = string(cm)
	t.UpdateF(&revisionResources)

	// publish PackageRevision
	t.GetF(client.ObjectKeyFromObject(pr), pr)
	pr.Spec.Lifecycle = porchapi.PackageRevisionLifecycleProposed
	t.UpdateF(pr)
	pr.Spec.Lifecycle = porchapi.PackageRevisionLifecyclePublished
	t.UpdateApprovalF(pr, metav1.UpdateOptions{})

	// upgrade "test-workspace" to basensV2
	pr.Spec.Lifecycle = porchapi.PackageRevisionLifecycleDraft
	pr.Spec.WorkspaceName = "test-workspace-upgrade"
	pr.Spec.Tasks = []porchapi.Task{{
		Type: porchapi.TaskTypeUpgrade,
		Upgrade: &porchapi.PackageUpgradeTaskSpec{
			OldUpstream: porchapi.PackageRevisionRef{
				Name: basensV1.Name,
			},
			NewUpstream: porchapi.PackageRevisionRef{
				Name: basensV2.Name,
			},
			LocalPackageRevisionRef: porchapi.PackageRevisionRef{
				Name: pr.Name, // this is still the name of the "test-workspace" PR
			},
		},
	}}

	t.CreateF(pr)

	t.GetF(client.ObjectKey{
		Namespace: t.Namespace,
		Name:      pr.Name,
	}, &revisionResources)

	if _, found := revisionResources.Spec.Resources["resourcequota.yaml"]; !found {
		t.Errorf("Updated package should contain 'resourcequota.yaml` file")
	}
}