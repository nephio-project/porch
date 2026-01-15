// Copyright 2025-2026 The kpt and Nephio Authors
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

package crcache

import (
	"context"
	"testing"

	porchapi "github.com/nephio-project/porch/api/porch/v1alpha1"
	"github.com/nephio-project/porch/pkg/cache/crcache/meta/fake"
	"github.com/nephio-project/porch/pkg/repository"
	mockrepo "github.com/nephio-project/porch/test/mockery/mocks/porch/pkg/repository"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestCachedPackageRevisionRenderStatus(t *testing.T) {
	store := &fake.MemoryMetadataStore{
		RenderStatuses: make(map[string]*porchapi.RenderStatus),
	}

	mockPR := mockrepo.NewMockPackageRevision(t)
	mockPR.On("Key").Return(repository.PackageRevisionKey{
		PkgKey: repository.PackageKey{
			RepoKey: repository.RepositoryKey{
				Namespace: "test-ns",
				Name:      "test-repo",
			},
			Package: "test-pkg",
		},
		Revision:      1,
		WorkspaceName: "main",
	})

	cached := &cachedPackageRevision{
		PackageRevision: mockPR,
		metadataStore:   store,
	}

	t.Run("SetRenderStatus stores in memory", func(t *testing.T) {
		rs := &porchapi.RenderStatus{Err: "test error"}
		cached.SetRenderStatus(rs)

		assert.Equal(t, "test error", cached.renderStatus.Err)
	})

	t.Run("SaveRenderStatus persists to MetadataStore", func(t *testing.T) {
		err := cached.SaveRenderStatus(context.TODO())
		assert.NoError(t, err)

		nsn := types.NamespacedName{
			Namespace: "test-ns",
			Name:      "test-repo.test-pkg.main",
		}
		assert.Equal(t, "test error", store.RenderStatuses[nsn.String()].Err)
	})

	t.Run("GetPackageRevision includes RenderStatus", func(t *testing.T) {
		mockPR.On("GetMeta").Return(metav1.ObjectMeta{
			Name:      "test-repo.test-pkg.main",
			Namespace: "test-ns",
		})
		mockPR.On("GetPackageRevision", mock.Anything).Return(&porchapi.PackageRevision{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-repo.test-pkg.main",
				Namespace: "test-ns",
			},
			Status: porchapi.PackageRevisionStatus{},
		}, nil).Once()

		pr, err := cached.GetPackageRevision(context.Background())
		assert.NoError(t, err)
		assert.NotNil(t, pr.Status.RenderStatus)
		assert.Equal(t, "test error", pr.Status.RenderStatus.Err)
	})
}
