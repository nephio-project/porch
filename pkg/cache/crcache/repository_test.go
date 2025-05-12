/*
 Copyright 2025 The Nephio Authors.

 Licensed under the Apache License, Version 2.0 (the "License");
 You may not use this file except in compliance with the License.
 You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

 Unless required by applicable law or agreed to in writing, software
 distributed under the License is distributed on an "AS IS" BASIS,
 WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 See the License for the specific language governing permissions and
 limitations under the License.
*/

package crcache

import (
	"context"
	"errors"
	"testing"
	"time"

	porchtypes "github.com/nephio-project/porch/v4/api/porch/v1alpha1"
	configapi "github.com/nephio-project/porch/v4/api/porchconfig/v1alpha1"
	cachetypes "github.com/nephio-project/porch/v4/pkg/cache/types"
	"github.com/nephio-project/porch/v4/pkg/externalrepo/fake"
	"github.com/nephio-project/porch/v4/pkg/repository"
	mockmeta "github.com/nephio-project/porch/v4/test/mockery/mocks/porch/pkg/cache/crcache/meta"
	mockcachetypes "github.com/nephio-project/porch/v4/test/mockery/mocks/porch/pkg/cache/types"
	mockrepo "github.com/nephio-project/porch/v4/test/mockery/mocks/porch/pkg/repository"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestCachedRepoRefresh(t *testing.T) {
	mockRepo := mockrepo.NewMockRepository(t)
	mockMeta := mockmeta.NewMockMetadataStore(t)
	mockNotifier := mockcachetypes.NewMockRepoPRChangeNotifier(t)

	repoSpec := configapi.Repository{}
	options := cachetypes.CacheOptions{
		RepoPRChangeNotifier: mockNotifier,
		RepoSyncFrequency:    time.Minute,
	}

	metaMap := []metav1.ObjectMeta{}

	mockRepo.EXPECT().Refresh(mock.Anything).Return(nil).Maybe()
	repoVersionCall := mockRepo.EXPECT().Version(mock.Anything).Return("v1.0", nil).Maybe()
	repoListPRCall := mockRepo.EXPECT().ListPackageRevisions(mock.Anything, mock.Anything).Return(nil, nil).Maybe()
	repoClosePRDCall := mockRepo.EXPECT().ClosePackageRevisionDraft(mock.Anything, mock.Anything, 1).Return(nil, errors.New("create draft error")).Maybe()

	metaListCall := mockMeta.EXPECT().List(mock.Anything, mock.Anything).Return(metaMap, nil).Maybe()
	mockNotifier.EXPECT().NotifyPackageRevisionChange(mock.Anything, mock.Anything).Return(0).Maybe()

	cr := newRepository("my-cached-repo", &repoSpec, mockRepo, mockMeta, options)
	assert.Equal(t, cr.id, "my-cached-repo")

	prKey := repository.PackageRevisionKey{
		PkgKey: repository.PackageKey{
			RepoKey: repository.RepositoryKey{
				Namespace: "the-ns",
				Name:      "the-name",
			},
			Path:    "",
			Package: "the-package",
		},
		WorkspaceName: "the-ws",
		Revision:      0,
	}

	fpr := fake.FakePackageRevision{
		PrKey: prKey,
	}

	cr.cachedPackageRevisions = make(map[repository.PackageRevisionKey]*cachedPackageRevision)
	cr.cachedPackageRevisions[prKey] = &cachedPackageRevision{
		PackageRevision:  &fpr,
		metadataStore:    mockMeta,
		isLatestRevision: false,
	}

	err := cr.Refresh(context.TODO())
	assert.True(t, err == nil)

	repoVersionCall.Return("", errors.New("version error")).Maybe()
	err = cr.Refresh(context.TODO())
	assert.False(t, err == nil)
	repoVersionCall.Return("v2.0", nil).Maybe()

	metaListCall.Return(nil, errors.New("meta list error")).Maybe()
	err = cr.Refresh(context.TODO())
	assert.False(t, err == nil)
	metaListCall.Return(metaMap, nil).Maybe()

	repoListPRCall.Return(nil, errors.New("list error")).Maybe()
	err = cr.Refresh(context.TODO())
	assert.False(t, err == nil)
	repoListPRCall.Return(nil, nil).Maybe()

	repoVersionCall.Return("v3.0", nil).Maybe()
	metaDeleteCall := mockMeta.EXPECT().Delete(mock.Anything, mock.Anything, mock.Anything).Return(metav1.ObjectMeta{}, errors.New("delete error")).Maybe()
	err = cr.Refresh(context.TODO())
	assert.True(t, err == nil)
	metaDeleteCall.Return(metav1.ObjectMeta{}, nil).Maybe()

	cr.flush()
	assert.True(t, cr.cachedPackageRevisions == nil)

	prMeta := porchtypes.PackageRevision{}

	repoCreatePRDCall := mockRepo.EXPECT().CreatePackageRevisionDraft(mock.Anything, mock.Anything).Return(nil, errors.New("create draft error")).Maybe()
	_, err = cr.CreatePackageRevisionDraft(context.TODO(), &prMeta)
	assert.True(t, err != nil)

	repoCreatePRDCall.Return(&fpr, nil).Maybe()
	prd, err := cr.CreatePackageRevisionDraft(context.TODO(), &prMeta)
	assert.True(t, err == nil)
	assert.Equal(t, 0, prd.Key().Revision)

	repoVersionCall.Return("", errors.New("version 2 error")).Maybe()
	_, err = cr.ClosePackageRevisionDraft(context.TODO(), prd, 1)
	assert.True(t, err != nil)

	repoVersionCall.Return("v3.2.4", nil).Maybe()
	_, err = cr.ClosePackageRevisionDraft(context.TODO(), prd, 1)
	assert.True(t, err != nil)

	_, err = cr.ClosePackageRevisionDraft(context.TODO(), prd, 1)
	assert.True(t, err != nil)
	repoClosePRDCall.Return(&fpr, nil).Maybe()

	repoClosePRDCall.Return(prd, nil).Maybe()
	metaCreateCall := mockMeta.EXPECT().Create(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(metav1.ObjectMeta{}, errors.New("meta create error")).Maybe()

	_, err = cr.ClosePackageRevisionDraft(context.TODO(), prd, 1)
	assert.True(t, err != nil)
	repoClosePRDCall.Return(&fpr, nil).Maybe()
	metaCreateCall.Return(metav1.ObjectMeta{}, nil)

	mockGet := mockMeta.EXPECT().Get(mock.Anything, mock.Anything).Return(metav1.ObjectMeta{}, nil).Maybe()
	mockUpdate := mockMeta.EXPECT().Update(mock.Anything, mock.Anything).Return(metav1.ObjectMeta{}, nil).Maybe()
	pr, err := cr.ClosePackageRevisionDraft(context.TODO(), prd, 1)
	assert.True(t, err == nil)
	assert.True(t, pr != nil)

	mockUpdate.Return(metav1.ObjectMeta{}, errors.New("meta update error")).Maybe()
	err = cr.cachedPackageRevisions[prKey].SetMeta(context.TODO(), metav1.ObjectMeta{Name: "Hello"})
	assert.True(t, err != nil)
	mockUpdate.Return(metav1.ObjectMeta{}, nil).Maybe()

	mockGet.Return(metav1.ObjectMeta{}, errors.New("meta get error")).Maybe()
	err = cr.cachedPackageRevisions[prKey].SetMeta(context.TODO(), metav1.ObjectMeta{Name: "Hello"})
	assert.True(t, err != nil)
	mockGet.Return(metav1.ObjectMeta{}, nil).Maybe()

	returnedMeta := metav1.ObjectMeta{
		Finalizers: []string{
			"finalizer",
		},
	}
	metaDeleteCall.Return(returnedMeta, errors.New("Delete on meta error")).Maybe()
	err = cr.DeletePackageRevision(context.TODO(), cr.cachedPackageRevisions[prKey])
	assert.True(t, err == nil)

}
