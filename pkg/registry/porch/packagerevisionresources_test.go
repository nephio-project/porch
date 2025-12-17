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
	"testing"

	porchapi "github.com/nephio-project/porch/api/porch"
	configapi "github.com/nephio-project/porch/api/porchconfig/v1alpha1"
	"github.com/nephio-project/porch/pkg/repository"
	mockclient "github.com/nephio-project/porch/test/mockery/mocks/external/sigs.k8s.io/controller-runtime/pkg/client"
	mockengine "github.com/nephio-project/porch/test/mockery/mocks/porch/pkg/engine"
	mockrepo "github.com/nephio-project/porch/test/mockery/mocks/porch/pkg/repository"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"k8s.io/apimachinery/pkg/apis/meta/internalversion"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	packagerevisionresources = &packageRevisionResources{
		TableConvertor: packageRevisionResourcesTableConvertor,
		packageCommon: packageCommon{
			scheme:         runtime.NewScheme(),
			gr:             porchapi.Resource("packagerevisions"),
			coreClient:     nil,
			updateStrategy: packageRevisionStrategy{},
			createStrategy: packageRevisionStrategy{},
		},
	}
)

func setupResourcesTest(t *testing.T) (mockClient *mockclient.MockClient, mockEngine *mockengine.MockCaDEngine) {
	mockClient = mockclient.NewMockClient(t)
	packagerevisionresources.coreClient = mockClient
	mockEngine = mockengine.NewMockCaDEngine(t)
	packagerevisionresources.cad = mockEngine
	mockClient.On("List", mock.Anything, mock.Anything, mock.Anything).Return(func(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
		list.(*configapi.RepositoryList).Items = []configapi.Repository{
			dummyRepoObject,
		}
		return nil
	}).Maybe()

	return
}

func TestListResources(t *testing.T) {
	_, mockEngine := setupResourcesTest(t)
	mockEngine.On("ListPackageRevisions", mock.Anything, mock.Anything, mock.Anything).Return([]repository.PackageRevision{
		packageRevision,
	}, nil).Once()

	result, err := packagerevisionresources.List(context.TODO(), &internalversion.ListOptions{})
	assert.NoError(t, err)
	assert.Equal(t, 1, len(result.(*porchapi.PackageRevisionResourcesList).Items))

	//=========================================================================================

	result, err = packagerevisionresources.List(context.TODO(), &internalversion.ListOptions{
		FieldSelector: fields.OneTermEqualSelector("something.crazy", "somethingOffTheWall"),
	})
	assert.Equal(t, nil, result)
	assert.ErrorContains(t, err, "unknown fieldSelector field")

	//=========================================================================================

	mockPkgRev := mockrepo.NewMockPackageRevision(t)
	mockEngine.On("ListPackageRevisions", mock.Anything, mock.Anything, mock.Anything).Return([]repository.PackageRevision{
		mockPkgRev,
	}, nil)
	mockPkgRev.On("GetResources", mock.Anything).Return(nil, errors.New("error getting API package revision")).Once()
	result, err = packagerevisionresources.List(context.TODO(), &internalversion.ListOptions{})
	assert.NoError(t, err)
	resultList, isList := result.(*porchapi.PackageRevisionResourcesList)
	assert.True(t, isList)
	assert.Equal(t, 0, len(resultList.Items))
}
