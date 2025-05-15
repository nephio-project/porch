// Copyright 2022 The kpt and Nephio Authors
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

package porch

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/nephio-project/porch/api/porch"
	api "github.com/nephio-project/porch/api/porch/v1alpha1"
	configapi "github.com/nephio-project/porch/api/porchconfig/v1alpha1"
	"github.com/nephio-project/porch/pkg/externalrepo/fake"
	kptfile "github.com/nephio-project/porch/pkg/kpt/api/kptfile/v1"
	"github.com/nephio-project/porch/pkg/repository"
	mockclient "github.com/nephio-project/porch/test/mockery/mocks/external/sigs.k8s.io/controller-runtime/pkg/client"
	mockengine "github.com/nephio-project/porch/test/mockery/mocks/porch/pkg/engine"
	mockrepo "github.com/nephio-project/porch/test/mockery/mocks/porch/pkg/repository"
	"github.com/stretchr/testify/mock"
	"gotest.tools/assert"
	"k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	packagerevisions = &packageRevisions{
		TableConvertor: packageRevisionTableConvertor,
		packageCommon: packageCommon{
			scheme:         runtime.NewScheme(),
			gr:             porch.Resource("packagerevisions"),
			coreClient:     nil,
			updateStrategy: packageRevisionStrategy{},
			createStrategy: packageRevisionStrategy{},
		},
	}
	dummyRepoObject = configapi.Repository{
		TypeMeta: metav1.TypeMeta{
			Kind:       configapi.TypeRepository.Kind,
			APIVersion: configapi.TypeRepository.APIVersion(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "someDummyRepo",
			Namespace: "someDummyNamespace",
		},
		Spec: configapi.RepositorySpec{
			Description: "Repository With Error",
			Type:        configapi.RepositoryTypeGit,
			Git: &configapi.GitRepository{
				// Use `invalid` domain: https://www.rfc-editor.org/rfc/rfc6761#section-6.4
				Repo: "https://repo.invalid/repository.git",
			},
		},
	}
	repositoryName  = "repo"
	pkg             = "1234567890"
	revision        = 1
	workspace       = "ws"
	packageRevision = &fake.FakePackageRevision{
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
		PackageLifecycle: api.PackageRevisionLifecyclePublished,
		PackageRevision: &api.PackageRevision{
			ObjectMeta: metav1.ObjectMeta{
				Labels: make(map[string]string),
			},
		},
		Resources: &api.PackageRevisionResources{
			Spec: api.PackageRevisionResourcesSpec{
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
)

func setup(t *testing.T) (mockClient *mockclient.MockClient, mockEngine *mockengine.MockCaDEngine) {
	mockClient = mockclient.NewMockClient(t)
	packagerevisions.coreClient = mockClient
	mockEngine = mockengine.NewMockCaDEngine(t)
	packagerevisions.cad = mockEngine
	mockClient.On("List", mock.Anything, mock.Anything, mock.Anything).Return(func(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
		list.(*configapi.RepositoryList).Items = []configapi.Repository{
			dummyRepoObject,
		}
		return nil
	}).Maybe()

	return
}

func TestListPackageRevisions(t *testing.T) {
	_, mockEngine := setup(t)
	mockEngine.On("ListPackageRevisions", mock.Anything, mock.Anything, mock.Anything).Return([]repository.PackageRevision{
		packageRevision,
	}, nil).Once()

	result, err := packagerevisions.List(context.TODO(), &internalversion.ListOptions{})
	assert.NilError(t, err)
	assert.Equal(t, 1, len(result.(*api.PackageRevisionList).Items))

	//=========================================================================================

	result, err = packagerevisions.List(context.TODO(), &internalversion.ListOptions{
		FieldSelector: fields.OneTermEqualSelector("something.crazy", "somethingOffTheWall"),
	})
	assert.Equal(t, nil, result)
	assert.ErrorContains(t, err, "unknown fieldSelector field")

	//=========================================================================================

	mockPkgRev := mockrepo.NewMockPackageRevision(t)
	mockEngine.On("ListPackageRevisions", mock.Anything, mock.Anything, mock.Anything).Return([]repository.PackageRevision{
		mockPkgRev,
	}, nil)
	mockPkgRev.On("GetPackageRevision", mock.Anything).Return(nil, errors.New("error getting API package revision")).Once()
	result, err = packagerevisions.List(context.TODO(), &internalversion.ListOptions{})
	assert.Equal(t, nil, result)
	assert.ErrorContains(t, err, "error getting API package revision")

	//=========================================================================================

	mockPkgRev.On("GetPackageRevision", mock.Anything).Return(packageRevision.PackageRevision, nil).Once()
	mockPkgRev.On("GetPackageRevision", mock.Anything).Return(nil, errors.New("error getting API package revision")).Once()
	result, err = packagerevisions.List(context.TODO(), &internalversion.ListOptions{})
	assert.Equal(t, nil, result)
	assert.ErrorContains(t, err, "error getting API package revision")
}

func TestUpdateStrategyForLifecycle(t *testing.T) {
	type testCase struct {
		old     api.PackageRevisionLifecycle
		valid   []api.PackageRevisionLifecycle
		invalid []api.PackageRevisionLifecycle
	}

	s := packageRevisionStrategy{}

	for _, tc := range []testCase{
		{
			old:     "",
			valid:   []api.PackageRevisionLifecycle{"", api.PackageRevisionLifecycleDraft, api.PackageRevisionLifecycleProposed},
			invalid: []api.PackageRevisionLifecycle{"Wrong", api.PackageRevisionLifecyclePublished},
		},
		{
			old:     api.PackageRevisionLifecycleDraft,
			valid:   []api.PackageRevisionLifecycle{"", api.PackageRevisionLifecycleDraft, api.PackageRevisionLifecycleProposed},
			invalid: []api.PackageRevisionLifecycle{"Wrong", api.PackageRevisionLifecyclePublished},
		},
		{
			old:     api.PackageRevisionLifecycleProposed,
			valid:   []api.PackageRevisionLifecycle{"", api.PackageRevisionLifecycleDraft, api.PackageRevisionLifecycleProposed},
			invalid: []api.PackageRevisionLifecycle{"Wrong", api.PackageRevisionLifecyclePublished},
		},
		{
			old:     api.PackageRevisionLifecyclePublished,
			valid:   []api.PackageRevisionLifecycle{api.PackageRevisionLifecyclePublished},
			invalid: []api.PackageRevisionLifecycle{"", "Wrong", api.PackageRevisionLifecycleDraft, api.PackageRevisionLifecycleProposed},
		},
		{
			old:     "Wrong",
			valid:   []api.PackageRevisionLifecycle{},
			invalid: []api.PackageRevisionLifecycle{"", "Wrong", api.PackageRevisionLifecycleDraft, api.PackageRevisionLifecycleProposed, api.PackageRevisionLifecyclePublished},
		},
	} {
		for _, new := range tc.valid {
			testValidateUpdate(t, s, tc.old, new, true)
		}
		for _, new := range tc.invalid {
			testValidateUpdate(t, s, tc.old, new, false)
		}
	}
}

func TestUpdateStrategy(t *testing.T) {
	s := packageRevisionStrategy{}

	testCases := map[string]struct {
		old   *api.PackageRevision
		new   *api.PackageRevision
		valid bool
	}{
		"spec can be updated for draft": {
			old: &api.PackageRevision{
				Spec: api.PackageRevisionSpec{
					Lifecycle: api.PackageRevisionLifecycleDraft,
					Tasks: []api.Task{
						{
							Type: api.TaskTypeInit,
							Init: &api.PackageInitTaskSpec{
								Description: "This is a test",
							},
						},
					},
				},
			},
			new: &api.PackageRevision{
				Spec: api.PackageRevisionSpec{
					Lifecycle: api.PackageRevisionLifecycleDraft,
					Tasks: []api.Task{
						{
							Type: api.TaskTypeInit,
							Init: &api.PackageInitTaskSpec{
								Description: "This is a test",
							},
						},
						{
							Type:  api.TaskTypePatch,
							Patch: &api.PackagePatchTaskSpec{},
						},
					},
				},
			},
			valid: true,
		},
		"spec can not be updated for published": {
			old: &api.PackageRevision{
				Spec: api.PackageRevisionSpec{
					Lifecycle: api.PackageRevisionLifecyclePublished,
					Tasks: []api.Task{
						{
							Type: api.TaskTypeInit,
							Init: &api.PackageInitTaskSpec{
								Description: "This is a test",
							},
						},
					},
				},
			},
			new: &api.PackageRevision{
				Spec: api.PackageRevisionSpec{
					Lifecycle: api.PackageRevisionLifecyclePublished,
					Tasks: []api.Task{
						{
							Type: api.TaskTypeInit,
							Init: &api.PackageInitTaskSpec{
								Description: "This is a test",
							},
						},
						{
							Type:  api.TaskTypePatch,
							Patch: &api.PackagePatchTaskSpec{},
						},
					},
				},
			},
			valid: false,
		},
		"labels can be updated for published": {
			old: &api.PackageRevision{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"foo": "bar",
					},
				},
				Spec: api.PackageRevisionSpec{
					Lifecycle: api.PackageRevisionLifecyclePublished,
				},
			},
			new: &api.PackageRevision{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"bar": "foo",
					},
				},
				Spec: api.PackageRevisionSpec{
					Lifecycle: api.PackageRevisionLifecyclePublished,
				},
			},
			valid: true,
		},
		"annotations can be updated for published": {
			old: &api.PackageRevision{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"foo": "bar",
					},
				},
				Spec: api.PackageRevisionSpec{
					Lifecycle: api.PackageRevisionLifecyclePublished,
				},
			},
			new: &api.PackageRevision{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"bar": "foo",
					},
				},
				Spec: api.PackageRevisionSpec{
					Lifecycle: api.PackageRevisionLifecyclePublished,
				},
			},
			valid: true,
		},
	}

	for tn := range testCases {
		tc := testCases[tn]
		t.Run(tn, func(t *testing.T) {
			ctx := context.Background()
			allErrs := s.ValidateUpdate(ctx, tc.new, tc.old)

			if tc.valid {
				if len(allErrs) > 0 {
					t.Errorf("Update failed unexpectedly: %v", allErrs.ToAggregate().Error())
				}
			} else {
				if len(allErrs) == 0 {
					t.Error("Update should fail but didn't")
				}
			}
		})
	}
}

func TestOtherInterfaceMethods(t *testing.T) {

	assert.Equal(t, "packagerevision", packagerevisions.GetSingularName())
	_, ok := packagerevisions.New().(*api.PackageRevision)
	assert.Assert(t, ok)
	packagerevisions.Destroy()
	_, ok = packagerevisions.NewList().(*api.PackageRevisionList)
	assert.Assert(t, ok)
	assert.Assert(t, packagerevisions.NamespaceScoped())

}
