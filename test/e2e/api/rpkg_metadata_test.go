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
	"strconv"
	"time"

	porchapi "github.com/nephio-project/porch/api/porch/v1alpha1"
	"github.com/nephio-project/porch/pkg/repository"
	suiteutils "github.com/nephio-project/porch/test/e2e/suiteutils"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	configMapGVK = corev1.SchemeGroupVersion.WithKind("ConfigMap")
)

func (t *PorchSuite) TestNewPackageRevisionLabels() {
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

	t.RegisterGitRepositoryF(t.GetPorchTestRepoURL(), repository, "", suiteutils.GiteaUser, suiteutils.GiteaPassword)

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
			WorkspaceName:  defaultWorkspace,
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
	t.CreateF(&pr)
	t.ValidateLabelsAndAnnos(pr.Name,
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
	t.UpdateF(&pr)

	// retrieve the updated object
	t.GetF(client.ObjectKey{
		Namespace: pr.Namespace,
		Name:      pr.Name,
	}, &pr)

	t.ValidateLabelsAndAnnos(pr.Name,
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
	_ = t.UpdateApprovalF(&pr, metav1.UpdateOptions{})
	t.ValidateLabelsAndAnnos(pr.Name,
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
	t.GetF(client.ObjectKey{
		Namespace: pr.Namespace,
		Name:      pr.Name,
	}, &pr)

	// Update the labels and annotations on the approved package.
	delete(pr.Labels, labelKey1)
	pr.Labels[labelKey2] = labelVal2
	delete(pr.Annotations, annoKey2)
	pr.Spec.Revision = 1
	t.UpdateF(&pr)
	t.ValidateLabelsAndAnnos(pr.Name,
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
	clonedPr := t.CreatePackageSkeleton(repository, "cloned-package", defaultWorkspace)
	clonedPr.Spec.Tasks = []porchapi.Task{
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
	}
	t.CreateF(clonedPr)
	t.ValidateLabelsAndAnnos(clonedPr.Name,
		map[string]string{},
		map[string]string{},
	)
}

func (t *PorchSuite) TestPackageRevisionLabelSelectors() {
	const (
		repository       = "pkg-rev-label-selectors"
		labelKey         = "kpt.dev/label"
		labelVal1        = "foo"
		labelVal2        = "bar"
		latestLabelKey   = porchapi.LatestPackageRevisionKey
		latestLabelValue = porchapi.LatestPackageRevisionValue
	)

	t.RegisterGitRepositoryF(t.GetPorchTestRepoURL(), repository, "", suiteutils.GiteaUser, suiteutils.GiteaPassword)

	// Create a package with labels and annotations.
	pr := porchapi.PackageRevision{
		TypeMeta: metav1.TypeMeta{
			Kind:       "PackageRevision",
			APIVersion: porchapi.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: t.Namespace,
			Labels: map[string]string{
				labelKey: labelVal1,
			},
		},
		Spec: porchapi.PackageRevisionSpec{
			PackageName:    "new-package",
			WorkspaceName:  defaultWorkspace,
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
	t.CreateF(&pr)

	// Propose and approve to ensure it has the latest-revision label
	pr.Spec.Lifecycle = porchapi.PackageRevisionLifecycleProposed
	t.UpdateF(&pr)
	pr.Spec.Lifecycle = porchapi.PackageRevisionLifecyclePublished
	t.UpdateApprovalF(&pr, metav1.UpdateOptions{})

	prList := porchapi.PackageRevisionList{}
	pkgSelector := client.MatchingLabels(labels.Set{labelKey: labelVal1})
	t.ListE(&prList, client.InNamespace(t.Namespace), pkgSelector)
	require.Equal(t.T(), 1, len(prList.Items))

	pkgSelector = client.MatchingLabels(labels.Set{labelKey: labelVal2})
	t.ListE(&prList, client.InNamespace(t.Namespace), pkgSelector)
	require.Empty(t.T(), prList.Items)

	// Special case for the kpt.dev/latest-revision label,
	// which is managed by Porch and handled separately
	pkgSelector = client.MatchingLabels(labels.Set{latestLabelKey: latestLabelValue})
	t.ListE(&prList, client.InNamespace(t.Namespace), pkgSelector)
	require.Equal(t.T(), 1, len(prList.Items))
}

func (t *PorchSuite) TestRegisteredPackageRevisionLabels() {
	const (
		labelKey = "kpt.dev/label"
		labelVal = "foo"
		annoKey  = "kpt.dev/anno"
		annoVal  = "foo"
	)

	// Register the upstream repository
	t.RegisterGitRepositoryF(t.GetTestBlueprintsRepoURL(), suiteutils.TestBlueprintsRepoName, "", suiteutils.GiteaUser, suiteutils.GiteaPassword)

	var list porchapi.PackageRevisionList
	t.ListE(&list, client.InNamespace(t.Namespace))

	basens := t.MustFindPackageRevision(&list, repository.PackageRevisionKey{PkgKey: repository.PackageKey{RepoKey: repository.RepositoryKey{Name: "test-blueprints"}, Package: "basens"}, Revision: 1})
	if basens.Labels == nil {
		basens.Labels = make(map[string]string)
	}
	basens.Labels[labelKey] = labelVal
	if basens.Annotations == nil {
		basens.Annotations = make(map[string]string)
	}
	basens.Annotations[annoKey] = annoVal
	t.UpdateF(basens)

	t.ValidateLabelsAndAnnos(basens.Name,
		map[string]string{
			labelKey: labelVal,
		},
		map[string]string{
			annoKey: annoVal,
		},
	)
}

func (t *PorchSuite) TestPackageRevisionGCWithOwner() {
	const (
		repository = "pkgrevgcwithowner"
		workspace  = "pkgrevgcwithowner-workspace"
		cmName     = "foo"
	)

	t.RegisterGitRepositoryF(t.GetPorchTestRepoURL(), repository, "", suiteutils.GiteaUser, suiteutils.GiteaPassword)

	// Create a new package (via init)
	pr := t.CreatePackageDraftF(repository, "empty-package", workspace)

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
					Kind:       suiteutils.PackageRevisionGVK.Kind,
					Name:       pr.Name,
					UID:        pr.UID,
				},
			},
		},
		Data: map[string]string{
			"foo": "bar",
		},
	}
	t.CreateF(cm)

	t.DeleteF(pr)
	t.WaitUntilObjectDeleted(
		suiteutils.PackageRevisionGVK,
		types.NamespacedName{
			Name:      pr.Name,
			Namespace: pr.Namespace,
		},
		10*time.Second,
	)
	t.WaitUntilObjectDeleted(
		configMapGVK,
		types.NamespacedName{
			Name:      cm.Name,
			Namespace: cm.Namespace,
		},
		10*time.Second,
	)
}

func (t *PorchSuite) TestPackageRevisionOwnerReferences() {
	const (
		repository = "pkgrevownerrefs"
		workspace  = "pkgrevownerrefs-workspace"
		cmName     = "foo"
	)

	t.RegisterGitRepositoryF(t.GetPorchTestRepoURL(), repository, "", suiteutils.GiteaUser, suiteutils.GiteaPassword)

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
	t.CreateF(cm)

	pr := t.CreatePackageDraftF(repository, "empty-package", workspace)

	t.ValidateOwnerReferences(pr.Name, []metav1.OwnerReference{})

	ownerRef := metav1.OwnerReference{
		APIVersion: "v1",
		Kind:       "ConfigMap",
		Name:       cm.Name,
		UID:        cm.UID,
	}
	pr.OwnerReferences = []metav1.OwnerReference{ownerRef}
	t.UpdateF(pr)
	t.ValidateOwnerReferences(pr.Name, []metav1.OwnerReference{ownerRef})
}

func (t *PorchSuite) TestPackageRevisionFinalizers() {
	const (
		repository  = "pkgrevfinalizers"
		workspace   = "pkgrevfinalizers-workspace"
		description = "empty-package description"
	)

	t.RegisterGitRepositoryF(t.GetPorchTestRepoURL(), repository, "", suiteutils.GiteaUser, suiteutils.GiteaPassword)

	// Create a new package (via init)
	pr := t.CreatePackageDraftF(repository, "empty-package", workspace)

	t.ValidateFinalizers(pr.Name, []string{})

	readPR := t.GetPackageRevision(repository, "empty-package", 0)

	readPR.Finalizers = append(readPR.Finalizers, "foo-finalizer")
	t.UpdateF(readPR)
	t.ValidateFinalizers(readPR.Name, []string{"foo-finalizer"})

	readPR = t.GetPackageRevision(repository, "empty-package", 0)

	t.DeleteF(readPR)
	t.ValidateFinalizers(readPR.Name, []string{"foo-finalizer"})

	readPR = t.GetPackageRevision(repository, "empty-package", 0)

	readPR.Finalizers = []string{}
	t.UpdateF(readPR)
	t.WaitUntilObjectDeleted(suiteutils.PackageRevisionGVK, types.NamespacedName{
		Name:      readPR.Name,
		Namespace: readPR.Namespace,
	}, 10*time.Second)
}

func (t *PorchSuite) TestPackageRevisionFieldSelectors() {
	t.RegisterGitRepositoryF(t.GetTestBlueprintsRepoURL(), suiteutils.TestBlueprintsRepoName, "", suiteutils.GiteaUser, suiteutils.GiteaPassword)

	prList := porchapi.PackageRevisionList{}

	pkgName := "basens"
	pkgSelector := client.MatchingFields(fields.Set{"spec.packageName": pkgName})
	t.ListE(&prList, client.InNamespace(t.Namespace), pkgSelector)
	if len(prList.Items) == 0 {
		t.Errorf("Expected at least one PackageRevision with packageName=%q, but got none", pkgName)
	}
	for _, pr := range prList.Items {
		if pr.Spec.PackageName != pkgName {
			t.Errorf("PackageRevision %s packageName: want %q, but got %q", pr.Name, pkgName, pr.Spec.PackageName)
		}
	}

	revNo := 1
	revSelector := client.MatchingFields(fields.Set{"spec.revision": strconv.Itoa(revNo)})
	t.ListE(&prList, client.InNamespace(t.Namespace), revSelector)
	if len(prList.Items) == 0 {
		t.Errorf("Expected at least one PackageRevision with revision=%q, but got none", revNo)
	}
	for _, pr := range prList.Items {
		if pr.Spec.Revision != revNo {
			t.Errorf("PackageRevision %s revision: want %q, but got %q", pr.Name, pkgName, pr.Spec.PackageName)
		}
	}

	wsName := "v1"
	wsSelector := client.MatchingFields(fields.Set{"spec.workspaceName": wsName})
	t.ListE(&prList, client.InNamespace(t.Namespace), wsSelector)
	if len(prList.Items) == 0 {
		t.Errorf("Expected at least one PackageRevision with workspaceName=%q, but got none", wsName)
	}
	for _, pr := range prList.Items {
		if pr.Spec.WorkspaceName != wsName {
			t.Errorf("PackageRevision %s workspaceName: want %q, but got %q", pr.Name, wsName, pr.Spec.WorkspaceName)
		}
	}

	publishedSelector := client.MatchingFields(fields.Set{"spec.lifecycle": string(porchapi.PackageRevisionLifecyclePublished)})
	t.ListE(&prList, client.InNamespace(t.Namespace), publishedSelector)
	if len(prList.Items) == 0 {
		t.Errorf("Expected at least one PackageRevision with lifecycle=%q, but got none", porchapi.PackageRevisionLifecyclePublished)
	}
	for _, pr := range prList.Items {
		if pr.Spec.Lifecycle != porchapi.PackageRevisionLifecyclePublished {
			t.Errorf("PackageRevision %s lifecycle: want %q, but got %q", pr.Name, porchapi.PackageRevisionLifecyclePublished, pr.Spec.Lifecycle)
		}
	}

	draftSelector := client.MatchingFields(fields.Set{"spec.lifecycle": string(porchapi.PackageRevisionLifecycleDraft)})
	t.ListE(&prList, client.InNamespace(t.Namespace), draftSelector)
	for _, pr := range prList.Items {
		if pr.Spec.Lifecycle != porchapi.PackageRevisionLifecycleDraft {
			t.Errorf("PackageRevision %s lifecycle: want %q, but got %q", pr.Name, porchapi.PackageRevisionLifecycleDraft, pr.Spec.Lifecycle)
		}
	}

	// test combined selectors
	combinedSelector := client.MatchingFields(fields.Set{"spec.revision": strconv.Itoa(revNo), "spec.packageName": pkgName})
	t.ListE(&prList, client.InNamespace(t.Namespace), combinedSelector)
	if len(prList.Items) == 0 {
		t.Errorf("Expected at least one PackageRevision with packageName=%q and revision=%q, but got none", pkgName, revNo)
	}
	for _, pr := range prList.Items {
		if pr.Spec.PackageName != pkgName || pr.Spec.Revision != revNo {
			t.Errorf("PackageRevision %s: want %v/%v, but got %v/%v", pr.Name, pkgName, revNo, pr.Spec.PackageName, pr.Spec.Revision)
		}
	}
}

func (t *PorchSuite) TestPackageRevisionGCAsOwner() {
	// TODO: Garbage collection is not working when a DB cache PackageRevision resource owner is deleted.
	// We need to get this test running in DB cache
	if _, ok := os.LookupEnv("DB_CACHE"); ok {
		return
	}

	const (
		repository  = "pkgrevgcasowner"
		workspace   = "pkgrevgcasowner-workspace"
		description = "empty-package description"
		cmName      = "foo"
	)

	t.RegisterGitRepositoryF(t.GetPorchTestRepoURL(), repository, "", suiteutils.GiteaUser, suiteutils.GiteaPassword)

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
	t.CreateF(cm)

	pr := &porchapi.PackageRevision{
		TypeMeta: metav1.TypeMeta{
			Kind:       suiteutils.PackageRevisionGVK.Kind,
			APIVersion: suiteutils.PackageRevisionGVK.GroupVersion().String(),
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
	t.CreateF(pr)

	t.DeleteF(cm)
	t.WaitUntilObjectDeleted(
		configMapGVK,
		types.NamespacedName{
			Name:      cm.Name,
			Namespace: cm.Namespace,
		},
		20*time.Second,
	)
	t.WaitUntilObjectDeleted(
		suiteutils.PackageRevisionGVK,
		types.NamespacedName{
			Name:      pr.Name,
			Namespace: pr.Namespace,
		},
		20*time.Second,
	)
}
