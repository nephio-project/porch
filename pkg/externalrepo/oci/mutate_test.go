// Copyright 2022, 2025 The kpt and Nephio Authors
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

package oci

import (
	"context"
	"testing"

	"github.com/GoogleContainerTools/kpt/pkg/oci"
	"github.com/nephio-project/porch/api/porch/v1alpha1"
	"github.com/nephio-project/porch/pkg/repository"
	"github.com/stretchr/testify/assert"
)

func TestCreateUpdateDeletePackageRevision(t *testing.T) {
	ociRepo := ociRepository{}
	ociRepo.storage = &oci.Storage{}

	ociRepo.spec.Registry = "my-registry"

	apiPr := &v1alpha1.PackageRevision{
		Spec: v1alpha1.PackageRevisionSpec{
			PackageName:   "my-package-name",
			WorkspaceName: "my-wprkspace",
		},
	}

	_, err := ociRepo.CreatePackageRevisionDraft(context.TODO(), apiPr)
	assert.True(t, err != nil)

	ociRepo.name = "my-repo"
	draftPr, err := ociRepo.CreatePackageRevisionDraft(context.TODO(), apiPr)
	assert.False(t, err != nil)

	meta := draftPr.GetMeta()
	assert.Equal(t, "", meta.Name)

	draftPrKey := repository.PackageRevisionKey{
		PkgKey: repository.PackageKey{
			Package: "my-package-name",
		},
	}
	assert.Equal(t, draftPrKey, draftPr.Key())

	oldPr := ociPackageRevision{}

	_, err = ociRepo.UpdatePackageRevision(context.TODO(), &oldPr)
	assert.True(t, err != nil)

	err = ociRepo.DeletePackageRevision(context.TODO(), &oldPr)
	assert.True(t, err != nil)
}
