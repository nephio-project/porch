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
	"fmt"
	"strings"

	porchapi "github.com/nephio-project/porch/api/porch/v1alpha1"
	"github.com/nephio-project/porch/pkg/repository"
	suiteutils "github.com/nephio-project/porch/test/e2e/suiteutils"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

func (t *PorchSuite) TestLargePackageRevision() {
	const (
		testDataSize = 5 * 1024 * 1024
		repoName     = "git-fn-pod-large"
	)

	setAnnotationsImage := t.KrmFunctionsRegistry + "/set-annotations:v0.1.5" // set-annotations:v0.1.5 is an older version that porch maps neither to built-in nor exec.

	t.RegisterGitRepositoryF(t.GetPorchTestRepoURL(), repoName, "", suiteutils.GiteaUser, suiteutils.GiteaPassword)

	// Create Package Revision
	pr := t.CreatePackageDraftF(repoName, "new-package", defaultWorkspace)

	var prr porchapi.PackageRevisionResources
	t.GetF(client.ObjectKeyFromObject(pr), &prr)

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

	t.UpdateF(&prr)

	rs := prr.Status.RenderStatus
	if rs.Err != "" || rs.Result.ExitCode != 0 {
		t.Fatalf("Couldn't render large package! exit code: %v,\n%v", rs.Result.ExitCode, rs.Err)
	}

	// Get package resources
	t.GetF(client.ObjectKeyFromObject(pr), &prr)

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
			longString, err := f.Value.GetString("value")
			if err != nil {
				t.Fatalf("couldn't find large string in test-data: %v", err)
			}
			if len(longString) != testDataSize {
				t.Fatalf("large string size mismatch. want: %v, got: %v", testDataSize, len(longString))
			}
			if !t.TestRunnerIsLocal && (node.GetAnnotations()["test-key"] != "test-val") {
				t.Errorf("Object (%s %q) should contain annotation `test-key:test-val`, but we got: %v", node.GetKind(), node.GetName(), node.GetAnnotations())
			}
		}
	}
}

func (t *PorchSuite) TestPackageRevisionInMultipleNamespaces() {

	registerRepoAndTestRevisions := func(repoName string, ns string, oldPRs []porchapi.PackageRevision) []porchapi.PackageRevision {
		t.RegisterGitRepositoryF(t.GetTestBlueprintsRepoURL(), repoName, "", suiteutils.GiteaUser, suiteutils.GiteaPassword,
			suiteutils.RepositoryOptions{RepOpts: suiteutils.InNamespace(ns), SecOpts: suiteutils.SecretInNamespace(ns)})
		prList := porchapi.PackageRevisionList{}
		t.ListF(&prList, client.InNamespace(ns))
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

	ns3 := &corev1.Namespace{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Namespace",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: t.Namespace + "-3",
		},
	}

	t.CreateF(ns2)
	t.CreateF(ns3)

	t.Cleanup(func() {
		t.DeleteE(ns2)
		t.DeleteE(ns3)
	})

	prs1 := registerRepoAndTestRevisions("test-blueprints", t.Namespace, nil)
	nPRs := len(prs1)
	t.Logf("Number of PRs in repo: %v", nPRs)

	prs2 := registerRepoAndTestRevisions("test-2-blueprints", ns2.Name, nil)
	if len(prs2) != nPRs {
		t.Errorf("number of PackageRevisions in namespace %s: want %v, got %d", ns2.Name, nPRs, len(prs2))
	}

	prs3 := registerRepoAndTestRevisions("test-3-blueprints", ns3.Name, prs1)
	if len(prs3) != nPRs {
		t.Errorf("number of PackageRevisions in repo-3: want %v, got %d", nPRs, len(prs2))
	}
}

func (t *PorchSuite) TestUniquenessOfUIDs() {

	// Register the upstream repository1
	t.RegisterGitRepositoryF(t.GetTestBlueprintsRepoURL(), suiteutils.TestBlueprintsRepoName, "", suiteutils.GiteaUser, suiteutils.GiteaPassword)

	ns2 := &corev1.Namespace{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Namespace",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: t.Namespace + "-2",
		},
	}
	t.CreateF(ns2)
	t.Cleanup(func() {
		t.DeleteE(ns2)
	})

	// Register the upstream repository2
	t.RegisterGitRepositoryF(t.GetTestBlueprintsRepoURL(), "test-2-blueprints", "", suiteutils.GiteaUser, suiteutils.GiteaPassword,
		suiteutils.RepositoryOptions{RepOpts: suiteutils.InNamespace(ns2.Name), SecOpts: suiteutils.SecretInNamespace(ns2.Name)})

	prList := porchapi.PackageRevisionList{}
	t.ListE(&prList, client.InNamespace(t.Namespace))

	uids := make(map[types.UID]*porchapi.PackageRevision)
	for _, pr := range prList.Items {
		otherPr, found := uids[pr.UID]
		if found {
			t.Errorf("PackageRevision %s/%s has the same UID as %s/%s: %v", pr.Namespace, pr.Name, otherPr.Namespace, otherPr.Name, pr.UID)
		}
		uids[pr.UID] = &pr
	}
}

func (t *PorchSuite) TestCreatePackageRevisionRollback() {
	// Create a package revision that will fail during task application
	// This should trigger the rollback functionality
	ctx := context.Background()
	packageName := "test-package-rollback"
	workspaceName := "test-workspace-rollback"
	repositoryName := "test-repo"

	// Register the repository first
	t.RegisterGitRepositoryF(t.GetPorchTestRepoURL(), repositoryName, "", suiteutils.GiteaUser, suiteutils.GiteaPassword)

	// Create a package revision with invalid task configuration
	// that will cause task application to fail
	pr := t.CreatePackageSkeleton(repositoryName, packageName, workspaceName)
	pr.ObjectMeta.Name = fmt.Sprintf("%s.%s.%s", repositoryName, packageName, workspaceName)
	pr.Spec.Tasks = []porchapi.Task{
		{
			Type: porchapi.TaskTypeClone,
			Clone: &porchapi.PackageCloneTaskSpec{
				Upstream: porchapi.UpstreamPackage{
					Type: porchapi.RepositoryTypeGit,
					Git: &porchapi.GitPackage{
						Repo:      "https://github.com/nephio-project/porch.git",
						Ref:       "main",
						Directory: "testdata/test-repo",
					},
				},
			},
		},
	}

	// Attempt to create the package revision
	_, err := t.Clientset.PorchV1alpha1().PackageRevisions(t.Namespace).Create(ctx, pr, metav1.CreateOptions{})
	assert.Error(t, err, "Expected error when creating package revision with invalid task configuration")

	// Verify that the package revision was not created
	_, err = t.Clientset.PorchV1alpha1().PackageRevisions(t.Namespace).Get(ctx, pr.Name, metav1.GetOptions{})
	assert.True(t, errors.IsNotFound(err), "Expected package revision to be deleted after rollback")
}

func (t *PorchSuite) TestMetadataAfterApproveAndBackgroundJob() {
	const (
		repoName    = "repo-background-job"
		packageName = "test-package-approve-and-background-job"
		workspace   = "v1"
	)

	// Register the repository
	t.RegisterGitRepositoryF(t.GetPorchTestRepoURL(), repoName, "", suiteutils.GiteaUser, suiteutils.GiteaPassword)

	// init, propose, and approve a new package
	pr := t.CreatePackageSkeleton(repoName, packageName, workspace)
	pr.Spec.Tasks = []porchapi.Task{{Type: porchapi.TaskTypeInit, Init: &porchapi.PackageInitTaskSpec{}}}
	t.CreateF(pr)
	prKey := client.ObjectKey{
		Namespace: t.Namespace,
		Name:      pr.Name,
	}
	t.GetF(prKey, pr)
	pr.Spec.Lifecycle = porchapi.PackageRevisionLifecycleProposed
	t.UpdateF(pr)
	t.GetF(prKey, pr)
	pr.Spec.Lifecycle = porchapi.PackageRevisionLifecyclePublished
	t.UpdateApprovalF(pr, metav1.UpdateOptions{})
	t.GetF(prKey, pr)

	// List package revisions and check they are as expected
	// (one as the latest with revision 1; one for the "main"/-1 revision)
	var list porchapi.PackageRevisionList
	t.ListE(&list, client.InNamespace(t.Namespace))

	expectedPkgRevKeyv1 := repository.PackageRevisionKey{PkgKey: repository.PackageKey{RepoKey: repository.RepositoryKey{Name: repoName}, Package: packageName}, Revision: 1, WorkspaceName: workspace}
	expectedPkgRevKeyMain := repository.PackageRevisionKey{PkgKey: repository.PackageKey{RepoKey: repository.RepositoryKey{Name: repoName}, Package: packageName}, Revision: -1, WorkspaceName: "main"}

	t.MustFindPackageRevision(&list, expectedPkgRevKeyv1)
	t.MustFindPackageRevision(&list, expectedPkgRevKeyMain)

	t.RetriggerBackgroundJobForRepo(repoName)

	// List package revisions again and check they are still as expected
	t.ListE(&list, client.InNamespace(t.Namespace))
	t.MustFindPackageRevision(&list, expectedPkgRevKeyv1)
	t.MustFindPackageRevision(&list, expectedPkgRevKeyMain)
}

func (t *PorchSuite) TestMetadataAfterDeleteAndBackgroundJob() {
	const (
		repoName    = "repo-background-job"
		packageName = "test-package-delete-and-background-job"
		workspace   = "v1"
	)

	// Register the repository
	t.RegisterGitRepositoryF(t.GetPorchTestRepoURL(), repoName, "", suiteutils.GiteaUser, suiteutils.GiteaPassword)

	// init, propose, and approve a new package
	pr := t.CreatePackageSkeleton(repoName, packageName, workspace)
	pr.Spec.Tasks = []porchapi.Task{{Type: porchapi.TaskTypeInit, Init: &porchapi.PackageInitTaskSpec{}}}
	t.CreateF(pr)
	prKey := client.ObjectKey{
		Namespace: t.Namespace,
		Name:      pr.Name,
	}
	t.GetF(prKey, pr)
	pr.Spec.Lifecycle = porchapi.PackageRevisionLifecycleProposed
	t.UpdateF(pr)
	t.GetF(prKey, pr)
	pr.Spec.Lifecycle = porchapi.PackageRevisionLifecyclePublished
	t.UpdateApprovalF(pr, metav1.UpdateOptions{})
	t.GetF(prKey, pr)

	// List package revisions and check they are as expected
	// (one as the latest with revision 1; one for the "main"/-1 revision)
	var list porchapi.PackageRevisionList
	t.ListE(&list, client.InNamespace(t.Namespace))

	expectedPkgRevKeyv1 := repository.PackageRevisionKey{PkgKey: repository.PackageKey{RepoKey: repository.RepositoryKey{Name: repoName}, Package: packageName}, Revision: 1, WorkspaceName: workspace}
	expectedPkgRevKeyMain := repository.PackageRevisionKey{PkgKey: repository.PackageKey{RepoKey: repository.RepositoryKey{Name: repoName}, Package: packageName}, Revision: -1, WorkspaceName: "main"}

	t.MustFindPackageRevision(&list, expectedPkgRevKeyv1)
	t.MustFindPackageRevision(&list, expectedPkgRevKeyMain)

	// Propose-delete and delete the package revision with "revision" == 1
	//
	// (henceforth called "the v1 package revision")
	pr.Spec.Lifecycle = porchapi.PackageRevisionLifecycleDeletionProposed
	t.UpdateApprovalF(pr, metav1.UpdateOptions{})
	t.DeleteE(&porchapi.PackageRevision{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: t.Namespace,
			Name:      pr.Name,
		},
	})

	// Check that the "main" package revision still exists, while the v1 package
	// revision does not
	mainPrKey := client.ObjectKey{
		Namespace: t.Namespace,
		Name:      strings.Replace(pr.Name, workspace, "main", 1),
	}
	t.MustExist(mainPrKey, &porchapi.PackageRevision{})
	t.MustNotExist(pr)

	t.RetriggerBackgroundJobForRepo(repoName)

	// Check that the existence and non-existence of the package revisions are
	// still as they were before the background job
	t.MustExist(mainPrKey, &porchapi.PackageRevision{})
	t.MustNotExist(pr)
}
