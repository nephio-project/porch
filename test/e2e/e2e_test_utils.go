// Copyright 2024 The Nephio Authors
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
	"time"
	"strings"
	"github.com/google/go-cmp/cmp"
	porchapi "github.com/nephio-project/porch/api/porch/v1alpha1"
	configapi "github.com/nephio-project/porch/api/porchconfig/v1alpha1"
	internalapi "github.com/nephio-project/porch/internal/api/porchinternal/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (t *TestSuite) validateFinalizers(ctx context.Context, name string, finalizers []string) {
	var pr porchapi.PackageRevision
	t.GetF(ctx, client.ObjectKey{
		Namespace: t.namespace,
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

func (t *TestSuite) validateOwnerReferences(ctx context.Context, name string, ownerRefs []metav1.OwnerReference) {
	var pr porchapi.PackageRevision
	t.GetF(ctx, client.ObjectKey{
		Namespace: t.namespace,
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

func (t *TestSuite) validateLabelsAndAnnos(ctx context.Context, name string, labels, annos map[string]string) {
	var pr porchapi.PackageRevision
	t.GetF(ctx, client.ObjectKey{
		Namespace: t.namespace,
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

func (t *TestSuite) registerGitRepositoryF(ctx context.Context, repo, name, directory string) {
	t.CreateF(ctx, &configapi.Repository{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Repository",
			APIVersion: configapi.GroupVersion.Identifier(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: t.namespace,
		},
		Spec: configapi.RepositorySpec{
			Type:    configapi.RepositoryTypeGit,
			Content: configapi.RepositoryContentPackage,
			Git: &configapi.GitRepository{
				Repo:      repo,
				Branch:    "main",
				Directory: directory,
			},
		},
	})

	t.Cleanup(func() {
		t.DeleteL(ctx, &configapi.Repository{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: t.namespace,
			},
		})
	})

	// Make sure the repository is ready before we test to (hopefully)
	// avoid flakiness.
	t.waitUntilRepositoryReady(ctx, name, t.namespace)
}

type repositoryOption func(*configapi.Repository)

func withDeployment() repositoryOption {
	return func(r *configapi.Repository) {
		r.Spec.Deployment = true
	}
}

func withType(t configapi.RepositoryType) repositoryOption {
	return func(r *configapi.Repository) {
		r.Spec.Type = t
	}
}

func withContent(content configapi.RepositoryContent) repositoryOption {
	return func(r *configapi.Repository) {
		r.Spec.Content = content
	}
}

// Creates an empty package draft by initializing an empty package
func (t *TestSuite) createPackageDraftF(ctx context.Context, repository, name, workspace string) *porchapi.PackageRevision {
	pr := &porchapi.PackageRevision{
		TypeMeta: metav1.TypeMeta{
			Kind:       "PackageRevision",
			APIVersion: porchapi.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: t.namespace,
		},
		Spec: porchapi.PackageRevisionSpec{
			PackageName:    name,
			WorkspaceName:  porchapi.WorkspaceName(workspace),
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
	return pr
}

func (t *TestSuite) mustExist(ctx context.Context, key client.ObjectKey, obj client.Object) {
	t.GetF(ctx, key, obj)
	if got, want := obj.GetName(), key.Name; got != want {
		t.Errorf("%T.Name: got %q, want %q", obj, got, want)
	}
	if got, want := obj.GetNamespace(), key.Namespace; got != want {
		t.Errorf("%T.Namespace: got %q, want %q", obj, got, want)
	}
}

func (t *TestSuite) mustNotExist(ctx context.Context, obj client.Object) {
	switch err := t.client.Get(ctx, client.ObjectKeyFromObject(obj), obj); {
	case err == nil:
		t.Errorf("No error returned getting a deleted package; expected error")
	case !apierrors.IsNotFound(err):
		t.Errorf("Expected NotFound error. got %v", err)
	}
}

// waitUntilRepositoryReady waits for up to 10 seconds for the repository with the
// provided name and namespace is ready, i.e. the Ready condition is true.
// It also queries for Functions and PackageRevisions, to ensure these are also
// ready - this is an artifact of the way we've implemented the aggregated apiserver,
// where the first fetch can sometimes be synchronous.
func (t *TestSuite) waitUntilRepositoryReady(ctx context.Context, name, namespace string) {
	nn := types.NamespacedName{
		Name:      name,
		Namespace: namespace,
	}
	var innerErr error
	err := wait.PollImmediateWithContext(ctx, time.Second, 10*time.Second, func(ctx context.Context) (bool, error) {
		var repo configapi.Repository
		if err := t.client.Get(ctx, nn, &repo); err != nil {
			innerErr = err
			return false, nil
		}
		for _, c := range repo.Status.Conditions {
			if c.Type == configapi.RepositoryReady {
				return c.Status == metav1.ConditionTrue, nil
			}
		}
		return false, nil
	})
	if err != nil {
		t.Errorf("Repository not ready after wait: %v", innerErr)
	}

	// While we're using an aggregated apiserver, make sure we can query the generated objects
	if err := wait.PollImmediateWithContext(ctx, time.Second, 10*time.Second, func(ctx context.Context) (bool, error) {
		var revisions porchapi.PackageRevisionList
		if err := t.client.List(ctx, &revisions, client.InNamespace(nn.Namespace)); err != nil {
			innerErr = err
			return false, nil
		}
		return true, nil
	}); err != nil {
		t.Errorf("unable to query PackageRevisions after wait: %v", innerErr)
	}

	// Check for functions also (until we move them to CRDs)
	if err := wait.PollImmediateWithContext(ctx, time.Second, 10*time.Second, func(ctx context.Context) (bool, error) {
		var functions porchapi.FunctionList
		if err := t.client.List(ctx, &functions, client.InNamespace(nn.Namespace)); err != nil {
			innerErr = err
			return false, nil
		}
		return true, nil
	}); err != nil {
		t.Errorf("unable to query Functions after wait: %v", innerErr)
	}

}

func (t *TestSuite) waitUntilRepositoryDeleted(ctx context.Context, name, namespace string) {
	err := wait.PollImmediateWithContext(ctx, time.Second, 20*time.Second, func(ctx context.Context) (done bool, err error) {
		var repo configapi.Repository
		nn := types.NamespacedName{
			Name:      name,
			Namespace: namespace,
		}
		if err := t.client.Get(ctx, nn, &repo); err != nil {
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

func (t *TestSuite) waitUntilAllPackagesDeleted(ctx context.Context, repoName string) {
	err := wait.PollImmediateWithContext(ctx, time.Second, 60*time.Second, func(ctx context.Context) (done bool, err error) {
		var pkgRevList porchapi.PackageRevisionList
		if err := t.client.List(ctx, &pkgRevList); err != nil {
			t.Logf("error listing packages: %v", err)
			return false, nil
		}
		for _, pkgRev := range pkgRevList.Items {
			if strings.HasPrefix(fmt.Sprintf("%s-", pkgRev.Name), repoName) {
				t.Logf("Found package %s from repo %s", pkgRev.Name, repoName)
				return false, nil
			}
		}

		var internalPkgRevList internalapi.PackageRevList
		if err := t.client.List(ctx, &internalPkgRevList); err != nil {
			t.Logf("error list internal packages: %v", err)
			return false, nil
		}
		for _, internalPkgRev := range internalPkgRevList.Items {
			if strings.HasPrefix(fmt.Sprintf("%s-", internalPkgRev.Name), repoName) {
				t.Logf("Found internalPkg %s from repo %s", internalPkgRev.Name, repoName)
				return false, nil
			}
		}
		return true, nil
	})
	if err != nil {
		t.Fatalf("Packages from repo %s still remains", repoName)
	}
}

func (t *TestSuite) waitUntilObjectDeleted(ctx context.Context, gvk schema.GroupVersionKind, namespacedName types.NamespacedName, d time.Duration) {
	var innerErr error
	err := wait.PollImmediateWithContext(ctx, time.Second, d, func(ctx context.Context) (bool, error) {
		var u unstructured.Unstructured
		u.SetGroupVersionKind(gvk)
		if err := t.client.Get(ctx, namespacedName, &u); err != nil {
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

func (t *TestSuite) waitUntilMainBranchPackageRevisionExists(ctx context.Context, pkgName string) {
	err := wait.PollImmediateWithContext(ctx, time.Second, 120*time.Second, func(ctx context.Context) (done bool, err error) {
		var pkgRevList porchapi.PackageRevisionList
		if err := t.client.List(ctx, &pkgRevList); err != nil {
			t.Logf("error listing packages: %v", err)
			return false, nil
		}
		for _, pkgRev := range pkgRevList.Items {
			pkgName := pkgRev.Spec.PackageName
			pkgRevision := pkgRev.Spec.Revision
			if pkgRevision == "main" &&
				pkgName == pkgRev.Spec.PackageName {
				return true, nil
			}
		}
		return false, nil
	})
	if err != nil {
		t.Fatalf("Main branch package revision for %s not found", pkgName)
	}
}
