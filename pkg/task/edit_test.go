// Copyright 2022, 2024-2025 The kpt and Nephio Authors
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

package task

import (
	"context"
	"errors"
	"strings"
	"testing"

	mockrepo "github.com/nephio-project/porch/test/mockery/mocks/porch/pkg/repository"
	"github.com/stretchr/testify/mock"
	"gotest.tools/assert"

	"github.com/google/go-cmp/cmp"
	"github.com/nephio-project/porch/api/porch/v1alpha1"
	api "github.com/nephio-project/porch/api/porch/v1alpha1"
	configapi "github.com/nephio-project/porch/api/porchconfig/v1alpha1"
	"github.com/nephio-project/porch/pkg/externalrepo/fake"
	kptfile "github.com/nephio-project/porch/pkg/kpt/api/kptfile/v1"
	"github.com/nephio-project/porch/pkg/repository"
)

func TestEdit(t *testing.T) {
	epm := getBasicValidEditMutation()

	res, _, err := epm.apply(context.Background(), repository.PackageResources{})
	if err != nil {
		t.Errorf("task apply failed: %v", err)
	}

	want := strings.TrimSpace(`
apiVersion: kpt.dev/v1
kind: Kptfile
metadata:
  name: example
  annotations:
    config.kubernetes.io/local-config: "true"
info:
  description: sample description
	`)
	got := strings.TrimSpace(res.Contents[kptfile.KptFileName])
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("result mismatch (-want +got):\n%s", diff)
	}
}

func TestEditWithErroredFetchRevisions(t *testing.T) {
	mockOpener := mockrepo.NewMockRepositoryOpener(t)
	mockOpener.EXPECT().OpenRepository(mock.Anything, mock.Anything).Return(nil, errors.New("network error fetching resources"))
	epm := getBasicValidEditMutation()
	epm.repoOpener = mockOpener

	_, _, err := epm.apply(context.Background(), repository.PackageResources{})
	assert.ErrorContains(t, err, "failed to fetch package")
	assert.ErrorContains(t, err, "network error")
}

func TestEditWithWrongPackageDetails(t *testing.T) {
	epm := getBasicValidEditMutation()
	epm.pkgRev.Spec.PackageName = "wrongPackage"

	_, _, err := epm.apply(context.Background(), repository.PackageResources{})
	assert.ErrorContains(t, err, "source revision must be from same package")
}

func TestEditWithWrongLifecycle(t *testing.T) {
	epm := getBasicValidEditMutation()

	epm.repoOpener.(*fakeRepositoryOpener).
		repository.(*fake.Repository).
		PackageRevisions[0].(*fake.FakePackageRevision).
		PackageLifecycle = api.PackageRevisionLifecycleDraft

	_, _, err := epm.apply(context.Background(), repository.PackageResources{})
	assert.ErrorContains(t, err, "source revision must be published")
}

func TestEditWithErroredGetResources(t *testing.T) {
	epm := getBasicValidEditMutation()

	mockPackage := mockrepo.NewMockPackageRevision(t)

	mockPackage.On("Key").Return(
		epm.repoOpener.(*fakeRepositoryOpener).
			repository.(*fake.Repository).
			PackageRevisions[0].Key()).Maybe()
	mockPackage.On("KubeObjectName").Return(
		epm.repoOpener.(*fakeRepositoryOpener).
			repository.(*fake.Repository).
			PackageRevisions[0].KubeObjectName()).Maybe()
	mockPackage.On("Lifecycle", mock.Anything).Return(api.PackageRevisionLifecyclePublished).Maybe()

	epm.repoOpener.(*fakeRepositoryOpener).
		repository.(*fake.Repository).
		PackageRevisions[0] = mockPackage

	mockPackage.EXPECT().GetResources(mock.Anything).Return(nil, errors.New("error getting resources from existing package revision"))

	_, _, err := epm.apply(context.Background(), repository.PackageResources{})
	assert.ErrorContains(t, err, "cannot read contents")
	assert.ErrorContains(t, err, "error getting resources")
}

func TestEditWithInputConditions(t *testing.T) {
	epm := getBasicValidEditMutation()

	epm.repoOpener.(*fakeRepositoryOpener).
		repository.(*fake.Repository).
		PackageRevisions[0].(*fake.FakePackageRevision).Resources.Spec.Resources[kptfile.KptFileName] = strings.TrimSpace(`
apiVersion: kpt.dev/v1
kind: Kptfile
metadata:
  name: example
  annotations:
    config.kubernetes.io/local-config: "true"
info:
  description: sample description
status:
  conditions:
  - type: PackagePipelinePassed
    status: "False"
    message: waiting for package pipeline to pass
    reason: WaitingOnPipeline
	`)
	epm.pkgRev.Status.Conditions = append(epm.pkgRev.Status.Conditions, api.Condition{
		Type:   "SomeUsefulCondition",
		Status: api.ConditionFalse,
		Reason: "WaitingForSomething",
	})

	want := strings.TrimSpace(`
apiVersion: kpt.dev/v1
kind: Kptfile
metadata:
  name: example
  annotations:
    config.kubernetes.io/local-config: "true"
info:
  readinessGates:
  - conditionType: SomeUsefulCondition
  description: sample description
status:
  conditions:
  - type: SomeUsefulCondition
    status: "False"
    reason: WaitingForSomething
		`)

	result, _, err := epm.apply(context.Background(), repository.PackageResources{})
	if err != nil {
		t.Errorf("task apply failed: %v", err)
	}

	got := strings.TrimSpace(result.Contents[kptfile.KptFileName])
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("result mismatch (-want +got):\n%s", diff)
	}
}

func getBasicValidEditMutation() editPackageMutation {
	repositoryName := "repo"
	pkg := "1234567890"
	revision := 1
	workspace := "ws"
	packageName := "repo.1234567890.ws"
	packageRevision := &fake.FakePackageRevision{
		PrKey: repository.PackageRevisionKey{
			PkgKey: repository.PackageKey{
				RepoKey: repository.RepositoryKey{
					Name: repositoryName,
				},
				Package: pkg,
			},
			Revision:      revision,
			WorkspaceName: workspace,
		},
		PackageLifecycle: v1alpha1.PackageRevisionLifecyclePublished,
		Resources: &v1alpha1.PackageRevisionResources{
			Spec: v1alpha1.PackageRevisionResourcesSpec{
				PackageName:    pkg,
				Revision:       revision,
				RepositoryName: repositoryName,
				Resources: map[string]string{
					kptfile.KptFileName: strings.TrimSpace(`
apiVersion: kpt.dev/v1
kind: Kptfile
metadata:
  name: example
  annotations:
    config.kubernetes.io/local-config: "true"
info:
  description: sample description
					`),
				},
			},
		},
	}
	repo := &fake.Repository{
		PackageRevisions: []repository.PackageRevision{
			packageRevision,
		},
	}
	repoOpener := &fakeRepositoryOpener{
		repository: repo,
	}

	testpkg := &api.PackageRevision{
		Spec: api.PackageRevisionSpec{
			PackageName:    pkg,
			RepositoryName: repositoryName,
			ReadinessGates: []api.ReadinessGate{},
		},
		Status: api.PackageRevisionStatus{
			Conditions: []api.Condition{},
		},
	}

	epm := editPackageMutation{
		pkgRev: testpkg,
		task: &v1alpha1.Task{
			Type: "edit",
			Edit: &v1alpha1.PackageEditTaskSpec{
				Source: &v1alpha1.PackageRevisionRef{
					Name: packageName,
				},
			},
		},

		referenceResolver: &fakeReferenceResolver{},
		repoOpener:        repoOpener,
	}
	return epm
}

// Implementation of the ReferenceResolver interface for testing.
type fakeReferenceResolver struct{}

func (f *fakeReferenceResolver) ResolveReference(ctx context.Context, namespace, name string, result repository.Object) error {
	return nil
}

type fakeRepositoryOpener struct {
	repository repository.Repository
}

func (f *fakeRepositoryOpener) OpenRepository(ctx context.Context, repositorySpec *configapi.Repository) (repository.Repository, error) {
	return f.repository, nil
}
