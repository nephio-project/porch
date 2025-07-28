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

package engine

import (
	"context"
	"testing"

	"github.com/nephio-project/porch/api/porch/v1alpha1"
	v1 "github.com/nephio-project/porch/pkg/kpt/api/kptfile/v1"
	"github.com/nephio-project/porch/pkg/repository"
	mockrepo "github.com/nephio-project/porch/test/mockery/mocks/porch/pkg/repository"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestPushPR(t *testing.T) {
	mockRepo := mockrepo.NewMockRepository(t)
	mockPR := mockrepo.NewMockPackageRevision(t)
	mockPRD := mockrepo.NewMockPackageRevisionDraft(t)

	ctx := context.TODO()

	mockRepo.EXPECT().Key().Return(repository.RepositoryKey{}).Maybe()
	mockPR.EXPECT().Key().Return(repository.PackageRevisionKey{}).Maybe()

	mockPR.EXPECT().Lifecycle(mock.Anything).Return(v1alpha1.PackageRevisionLifecycleDraft).Once()
	_, err := PushPackageRevision(ctx, mockRepo, mockPR)
	assert.NotNil(t, err)

	mockPR.EXPECT().Lifecycle(mock.Anything).Return(v1alpha1.PackageRevisionLifecyclePublished).Maybe()
	mockPR.EXPECT().GetPackageRevision(mock.Anything).Return(nil, err).Once()
	_, err = PushPackageRevision(ctx, mockRepo, mockPR)
	assert.NotNil(t, err)

	mockPR.EXPECT().GetPackageRevision(mock.Anything).Return(&v1alpha1.PackageRevision{}, nil).Maybe()
	mockPR.EXPECT().GetResources(mock.Anything).Return(nil, err).Once()
	_, err = PushPackageRevision(ctx, mockRepo, mockPR)
	assert.NotNil(t, err)

	mockPR.EXPECT().GetResources(mock.Anything).Return(&v1alpha1.PackageRevisionResources{}, nil).Maybe()
	mockRepo.EXPECT().CreatePackageRevisionDraft(mock.Anything, mock.Anything).Return(nil, err).Once()
	_, err = PushPackageRevision(ctx, mockRepo, mockPR)
	assert.NotNil(t, err)

	mockRepo.EXPECT().CreatePackageRevisionDraft(mock.Anything, mock.Anything).Return(mockPRD, nil).Maybe()
	mockPRD.EXPECT().UpdateResources(mock.Anything, mock.Anything, mock.Anything).Return(err).Once()
	_, err = PushPackageRevision(ctx, mockRepo, mockPR)
	assert.NotNil(t, err)

	mockPRD.EXPECT().UpdateResources(mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
	mockPRD.EXPECT().UpdateLifecycle(mock.Anything, mock.Anything).Return(err).Once()
	_, err = PushPackageRevision(ctx, mockRepo, mockPR)
	assert.NotNil(t, err)

	mockPRD.EXPECT().UpdateLifecycle(mock.Anything, mock.Anything).Return(nil).Maybe()
	mockRepo.EXPECT().ClosePackageRevisionDraft(mock.Anything, mock.Anything, mock.Anything).Return(nil, err).Once()
	_, err = PushPackageRevision(ctx, mockRepo, mockPR)
	assert.NotNil(t, err)

	mockRepo.EXPECT().ClosePackageRevisionDraft(mock.Anything, mock.Anything, mock.Anything).Return(mockPR, nil).Once()
	_, err = PushPackageRevision(ctx, mockRepo, mockPR)
	assert.Nil(t, err)
}
