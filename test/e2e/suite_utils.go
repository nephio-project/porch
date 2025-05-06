// Copyright 2024-2025 The Nephio Authors
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
	"strings"
	"time"

	"github.com/google/go-cmp/cmp"
	porchapi "github.com/nephio-project/porch/api/porch/v1alpha1"
	configapi "github.com/nephio-project/porch/api/porchconfig/v1alpha1"
	internalapi "github.com/nephio-project/porch/internal/api/porchinternal/v1alpha1"
	"github.com/nephio-project/porch/pkg/repository"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type TestSuiteWithGit struct {
	TestSuite
	gitConfig GitConfig
}

func (t *TestSuiteWithGit) SetupSuite() {
	t.TestSuite.SetupSuite()
	t.gitConfig = t.CreateGitRepo()
}

func (t *TestSuiteWithGit) GitConfig(name string) GitConfig {
	repoID := t.Namespace + "-" + name
	config := t.gitConfig
	config.Repo = config.Repo + "/" + repoID
	return config
}

func (t *TestSuiteWithGit) RegisterMainGitRepositoryF(name string, opts ...RepositoryOption) {
	t.T().Helper()
	config := t.GitConfig(name)
	t.registerGitRepositoryFromConfigF(name, config, opts...)
}

func (t *TestSuiteWithGit) RegisterGitRepositoryWithDirectoryF(name string, directory string, opts ...RepositoryOption) {
	t.T().Helper()
	config := t.GitConfig(name)
	config.Directory = directory
	t.registerGitRepositoryFromConfigF(name, config, opts...)
}

func (t *TestSuite) ValidateFinalizers(name string, finalizers []string) {
	t.T().Helper()
	var pr porchapi.PackageRevision
	t.GetF(client.ObjectKey{
		Namespace: t.Namespace,
		Name:      name,
	}, &pr)

	if len(finalizers) != len(pr.Finalizers) {
		diff := cmp.Diff(finalizers, pr.Finalizers)
		t.Errorf("Expected %d finalizers, but got %s", len(finalizers), diff)
	}

	for _, finalizer := range finalizers {
		var found bool
		for _, f := range pr.Finalizers {
			if f == finalizer {
				found = true
			}
		}
		if !found {
			t.Errorf("Expected finalizer %v, but didn't find it", finalizer)
		}
	}
}

func (t *TestSuite) ValidateOwnerReferences(name string, ownerRefs []metav1.OwnerReference) {
	t.T().Helper()
	var pr porchapi.PackageRevision
	t.GetF(client.ObjectKey{
		Namespace: t.Namespace,
		Name:      name,
	}, &pr)

	if len(ownerRefs) != len(pr.OwnerReferences) {
		diff := cmp.Diff(ownerRefs, pr.OwnerReferences)
		t.Errorf("Expected %d ownerReferences, but got %s", len(ownerRefs), diff)
	}

	for _, ownerRef := range ownerRefs {
		var found bool
		for _, or := range pr.OwnerReferences {
			if or == ownerRef {
				found = true
			}
		}
		if !found {
			t.Errorf("Expected ownerRef %v, but didn't find it", ownerRef)
		}
	}
}

func (t *TestSuite) ValidateLabelsAndAnnos(name string, labels, annos map[string]string) {
	t.T().Helper()
	var pr porchapi.PackageRevision
	t.GetF(client.ObjectKey{
		Namespace: t.Namespace,
		Name:      name,
	}, &pr)

	actualLabels := pr.ObjectMeta.Labels
	actualAnnos := pr.ObjectMeta.Annotations

	// Make this check to handle empty vs nil maps
	if !(len(labels) == 0 && len(actualLabels) == 0) {
		if diff := cmp.Diff(actualLabels, labels); diff != "" {
			t.Errorf("Unexpected result (-want, +got): %s", diff)
		}
	}

	if !(len(annos) == 0 && len(actualAnnos) == 0) {
		if diff := cmp.Diff(actualAnnos, annos); diff != "" {
			t.Errorf("Unexpected result (-want, +got): %s", diff)
		}
	}
}

func (t *TestSuite) MustHaveLabels(name string, labels map[string]string) {
	t.T().Helper()
	var pr porchapi.PackageRevision
	t.GetF(client.ObjectKey{
		Namespace: t.Namespace,
		Name:      name,
	}, &pr)

	for labelKey, labelValue := range labels {
		actualValue, ok := pr.Labels[labelKey]
		if !ok {
			t.Errorf("Expected PR %s to have label %s, but didn't find it", pr.Name, labelKey)
		}
		if actualValue != labelValue {
			t.Errorf("Expected PR %s to have label %s value %s but got %s", pr.Name, labelKey, labelValue, actualValue)
		}
	}
}

func (t *TestSuite) MustNotHaveLabels(name string, labels []string) {
	t.T().Helper()
	var pr porchapi.PackageRevision
	t.GetF(client.ObjectKey{
		Namespace: t.Namespace,
		Name:      name,
	}, &pr)

	for _, label := range labels {
		_, ok := pr.Labels[label]
		if ok {
			t.Errorf("Expected PR %s not to have label %s, but found it", pr.Name, label)
		}
	}
}

func (t *TestSuite) RegisterGitRepositoryF(repo, name, directory string, opts ...RepositoryOption) {
	t.T().Helper()
	config := GitConfig{
		Repo:      repo,
		Branch:    "main",
		Directory: directory,
	}
	t.registerGitRepositoryFromConfigF(name, config, opts...)
}

func (t *TestSuite) registerGitRepositoryFromConfigF(name string, config GitConfig, opts ...RepositoryOption) {
	t.T().Helper()
	var secret string
	// Create auth secret if necessary
	if config.Username != "" || config.Password != "" {
		secret = fmt.Sprintf("%s-auth", name)
		immutable := true
		t.CreateF(&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secret,
				Namespace: t.Namespace,
			},
			Immutable: &immutable,
			Data: map[string][]byte{
				"username": []byte(config.Username),
				"password": []byte(config.Password),
			},
			Type: corev1.SecretTypeBasicAuth,
		})
		t.Cleanup(func() {
			t.T().Helper()
			t.DeleteE(&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secret,
					Namespace: t.Namespace,
				},
			})
		})
	}

	repository := &configapi.Repository{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Repository",
			APIVersion: configapi.GroupVersion.Identifier(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: t.Namespace,
		},
		Spec: configapi.RepositorySpec{
			Description: "Porch Test Repository Description",
			Type:        configapi.RepositoryTypeGit,
			Git: &configapi.GitRepository{
				Repo:      config.Repo,
				Branch:    config.Branch,
				Directory: config.Directory,
				SecretRef: configapi.SecretRef{
					Name: secret,
				},
			},
		},
	}

	// Apply options
	for _, o := range opts {
		o(repository)
	}

	// Register repository
	t.CreateF(repository)

	t.Cleanup(func() {
		t.DeleteE(repository)
		t.WaitUntilRepositoryDeleted(name, t.Namespace)
		t.WaitUntilAllPackagesDeleted(name, t.Namespace)
	})

	// Make sure the repository is ready before we test to (hopefully)
	// avoid flakiness.
	t.WaitUntilRepositoryReady(repository.Name, repository.Namespace)
	t.Logf("Repository %s/%s is ready", repository.Namespace, repository.Name)
}

type RepositoryOption func(*configapi.Repository)

func WithDeployment() RepositoryOption {
	return func(r *configapi.Repository) {
		r.Spec.Deployment = true
	}
}

func withType(t configapi.RepositoryType) RepositoryOption {
	return func(r *configapi.Repository) {
		r.Spec.Type = t
	}
}

func InNamespace(ns string) RepositoryOption {
	return func(repo *configapi.Repository) {
		repo.Namespace = ns
	}
}

// Creates an empty package draft by initializing an empty package
func (t *TestSuite) CreatePackageDraftF(repository, packageName, workspace string) *porchapi.PackageRevision {
	t.T().Helper()
	pr := t.CreatePackageSkeleton(repository, packageName, workspace)
	pr.Spec.Tasks = []porchapi.Task{
		{
			Type: porchapi.TaskTypeInit,
			Init: &porchapi.PackageInitTaskSpec{},
		},
	}
	t.CreateF(pr)
	return pr
}

func (t *TestSuite) CreatePackageSkeleton(repoName, packageName, workspace string) *porchapi.PackageRevision {
	return &porchapi.PackageRevision{
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
			RepositoryName: repoName,
			// empty tasks list - set them as needed in the particular usage
			Tasks: []porchapi.Task{},
		},
	}
}

func (t *TestSuite) MustExist(key client.ObjectKey, obj client.Object) {
	t.T().Helper()
	t.Logf("Checking existence of %q...", key)
	t.GetF(key, obj)
	if got, want := obj.GetName(), key.Name; got != want {
		t.Errorf("%T.Name: got %q, want %q", obj, got, want)
	}
	if got, want := obj.GetNamespace(), key.Namespace; got != want {
		t.Errorf("%T.Namespace: got %q, want %q", obj, got, want)
	}
}

func (t *TestSuite) MustNotExist(obj client.Object) {
	t.T().Helper()
	switch err := t.Client.Get(t.GetContext(), client.ObjectKeyFromObject(obj), obj); {
	case err == nil:
		t.Errorf("No error returned getting a deleted package; expected error")
	case !apierrors.IsNotFound(err):
		t.Errorf("Expected NotFound error. got %v", err)
	}
}

// WaitUntilRepositoryReady waits for up to 60 seconds for the repository with the
// provided name and namespace is ready, i.e. the Ready condition is true.
// It also queries for Functions and PackageRevisions, to ensure these are also
// ready - this is an artifact of the way we've implemented the aggregated apiserver,
// where the first fetch can sometimes be synchronous.
func (t *TestSuite) WaitUntilRepositoryReady(name, namespace string) {
	t.T().Helper()
	nn := types.NamespacedName{
		Name:      name,
		Namespace: namespace,
	}
	var innerErr error
	err := wait.PollUntilContextTimeout(t.GetContext(), time.Second, 120*time.Second, true, func(ctx context.Context) (bool, error) {
		var repo configapi.Repository
		if err := t.Client.Get(ctx, nn, &repo); err != nil {
			innerErr = err
			return false, nil
		}
		for _, c := range repo.Status.Conditions {
			if c.Type == configapi.RepositoryReady {
				if c.Status == metav1.ConditionTrue {
					return true, nil
				} else {
					innerErr = fmt.Errorf("error condition is false: %s", c.Message)
					return false, nil
				}
			}
		}
		return false, nil
	})
	if err != nil {
		t.Fatalf("Repository not ready after wait: %v", innerErr)
	}

	// While we're using an aggregated apiserver, make sure we can query the generated objects
	if err := wait.PollUntilContextTimeout(t.GetContext(), time.Second, 10*time.Second, true, func(ctx context.Context) (bool, error) {
		var revisions porchapi.PackageRevisionList
		if err := t.Client.List(ctx, &revisions, client.InNamespace(nn.Namespace)); err != nil {
			innerErr = err
			return false, nil
		}
		return true, nil
	}); err != nil {
		t.Fatalf("unable to query PackageRevisions after wait: %v", innerErr)
	}
}

func (t *TestSuite) WaitUntilRepositoryDeleted(name, namespace string) {
	t.T().Helper()
	err := wait.PollUntilContextTimeout(t.GetContext(), time.Second, 20*time.Second, true, func(ctx context.Context) (done bool, err error) {
		var repo configapi.Repository
		nn := types.NamespacedName{
			Name:      name,
			Namespace: namespace,
		}
		if err := t.Client.Get(ctx, nn, &repo); err != nil {
			if apierrors.IsNotFound(err) {
				return true, nil
			}
			return false, nil
		}
		return false, nil
	})
	if err != nil {
		t.Fatalf("Repository %s/%s not deleted", namespace, name)
	}
}

func (t *TestSuite) WaitUntilAllPackagesDeleted(repoName string, namespace string) {
	t.T().Helper()
	err := wait.PollUntilContextTimeout(t.GetContext(), time.Second, 60*time.Second, true, func(ctx context.Context) (done bool, err error) {
		t.T().Helper()
		var pkgRevList porchapi.PackageRevisionList
		if err := t.Client.List(ctx, &pkgRevList); err != nil {
			t.Logf("error listing packages: %v", err)
			return false, nil
		}
		for _, pkgRev := range pkgRevList.Items {
			if pkgRev.Namespace == namespace && strings.HasPrefix(fmt.Sprintf("%s-", pkgRev.Name), repoName) {
				t.Logf("Found package %s from repo %s", pkgRev.Name, repoName)
				return false, nil
			}
		}
		var internalPkgRevList internalapi.PackageRevList
		if err := t.Client.List(ctx, &internalPkgRevList); err != nil {
			t.Logf("error list internal packages: %v", err)
			return false, nil
		}
		for _, internalPkgRev := range internalPkgRevList.Items {
			if internalPkgRev.Namespace == namespace && strings.HasPrefix(fmt.Sprintf("%s-", internalPkgRev.Name), repoName) {
				t.Logf("Found internalPkg %s/%s from repo %s", internalPkgRev.Namespace, internalPkgRev.Name, repoName)
				return false, nil
			}
		}
		return true, nil
	})
	if err != nil {
		t.Fatalf("Packages from repo %s still remains", repoName)
	}
}

func (t *TestSuite) WaitUntilObjectDeleted(gvk schema.GroupVersionKind, namespacedName types.NamespacedName, d time.Duration) {
	t.T().Helper()
	var innerErr error
	err := wait.PollUntilContextTimeout(t.GetContext(), time.Second, d, true, func(ctx context.Context) (bool, error) {
		var u unstructured.Unstructured
		u.SetGroupVersionKind(gvk)
		if err := t.Client.Get(ctx, namespacedName, &u); err != nil {
			if apierrors.IsNotFound(err) {
				return true, nil
			}
			innerErr = err
			return false, err
		}
		return false, nil
	})
	if err != nil {
		t.Errorf("Object %s not deleted after %s: %v", namespacedName.String(), d.String(), innerErr)
	}
}

func (t *TestSuite) WaitUntilPackageRevisionFulfillingConditionExists(
	timeout time.Duration,
	condition func(porchapi.PackageRevision) bool,
) (*porchapi.PackageRevision, error) {

	t.T().Helper()
	var foundPkgRev *porchapi.PackageRevision
	err := wait.PollUntilContextTimeout(t.GetContext(), time.Second, timeout, true, func(ctx context.Context) (done bool, err error) {
		var pkgRevList porchapi.PackageRevisionList
		if err := t.Client.List(ctx, &pkgRevList); err != nil {
			t.Logf("error listing packages: %v", err)
			return false, nil
		}
		for _, pkgRev := range pkgRevList.Items {
			if condition(pkgRev) {
				foundPkgRev = &pkgRev
				return true, nil
			}
		}
		return false, nil
	})
	return foundPkgRev, err
}

func (t *TestSuite) WaitUntilPackageRevisionExists(repository string, pkgName string, revision int) *porchapi.PackageRevision {
	t.T().Helper()
	t.Logf("Waiting for package revision (%v/%v/%v) to exist", repository, pkgName, revision)
	timeout := 120 * time.Second
	foundPkgRev, err := t.WaitUntilPackageRevisionFulfillingConditionExists(timeout, func(pkgRev porchapi.PackageRevision) bool {
		return pkgRev.Spec.RepositoryName == repository &&
			pkgRev.Spec.PackageName == pkgName &&
			pkgRev.Spec.Revision == revision
	})
	if err != nil {
		t.Fatalf("Package revision (%v/%v/%v) not found in time (%v)", repository, pkgName, revision, timeout)
	}
	return foundPkgRev
}

func (t *TestSuite) WaitUntilDraftPackageRevisionExists(repository string, pkgName string) *porchapi.PackageRevision {
	t.T().Helper()
	t.Logf("Waiting for a draft revision for package %v/%v to exist", repository, pkgName)
	timeout := 120 * time.Second
	foundPkgRev, err := t.WaitUntilPackageRevisionFulfillingConditionExists(timeout, func(pkgRev porchapi.PackageRevision) bool {
		return pkgRev.Spec.RepositoryName == repository &&
			pkgRev.Spec.PackageName == pkgName &&
			pkgRev.Spec.Lifecycle == porchapi.PackageRevisionLifecycleDraft
	})
	if err != nil {
		t.Fatalf("No draft package revision found for package %v/%v in time (%v)", repository, pkgName, timeout)
	}
	return foundPkgRev
}

func (t *TestSuite) WaitUntilPackageRevisionResourcesExists(
	key types.NamespacedName,
) *porchapi.PackageRevisionResources {

	t.T().Helper()
	t.Logf("Waiting for PackageRevisionResources object %v to exist", key)
	timeout := 120 * time.Second
	var foundPrr *porchapi.PackageRevisionResources
	err := wait.PollUntilContextTimeout(t.GetContext(), time.Second, timeout, true, func(ctx context.Context) (done bool, err error) {
		var prrList porchapi.PackageRevisionResourcesList
		if err := t.Client.List(ctx, &prrList); err != nil {
			t.Logf("error listing package revision resources: %v", err)
			return false, nil
		}
		for _, prr := range prrList.Items {
			if client.ObjectKeyFromObject(&prr) == key {
				foundPrr = &prr
				return true, nil
			}
		}
		return false, nil
	})
	if err != nil {
		t.Fatalf("PackageRevisionResources object wasn't found for package %v in time (%v)", key, timeout)
	}
	return foundPrr
}

func (t *TestSuite) GetPackageRevision(repo string, pkgName string, revision int) *porchapi.PackageRevision {
	t.T().Helper()
	var prList porchapi.PackageRevisionList
	selector := client.MatchingFields(fields.Set{
		"spec.repository":  repo,
		"spec.packageName": pkgName,
		"spec.revision":    repository.Revision2Str(revision),
	})
	t.ListF(&prList, selector, client.InNamespace(t.Namespace))

	if len(prList.Items) == 0 {
		t.Fatalf("PackageRevision object wasn't found for package revision %v/%v/%d", repo, pkgName, revision)
	}
	if len(prList.Items) > 1 {
		t.Fatalf("Multiple PackageRevision objects were found for package revision %v/%v/%d", repo, pkgName, revision)
	}
	return &prList.Items[0]
}

func (t *TestSuite) RetriggerBackgroundJobForRepo(repoName string) {
	repoKey := client.ObjectKey{
		Namespace: t.Namespace,
		Name:      repoName,
	}
	var repo configapi.Repository
	t.GetF(repoKey, &repo)
	repo.ResourceVersion = ""

	// Delete and recreate repository to trigger background job on it
	t.DeleteE(&repo)
	t.CreateE(&repo)
	t.WaitUntilRepositoryReady(repo.Name, t.Namespace)
}
