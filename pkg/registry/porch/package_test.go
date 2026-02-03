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

package porch

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPackageCUD(t *testing.T) {

	testPackages := packages{}

	pkgObj, err := testPackages.Create(context.TODO(), nil, nil, nil)
	assert.Nil(t, pkgObj)
	assert.Equal(t, "package creation is not supported directly over the API, create or clone a PackageRevision resource to create a package", err.Error())

	pkgObj, flag, err := testPackages.Update(context.TODO(), "", nil, nil, nil, false, nil)
	assert.Nil(t, pkgObj)
	assert.False(t, flag)
	assert.Equal(t, "package update is not supported directly over the API, update a PackageRevision resource to update a package", err.Error())

	pkgObj, flag, err = testPackages.Delete(context.TODO(), "", nil, nil)
	assert.Nil(t, pkgObj)
	assert.False(t, flag)
	assert.Equal(t, "package deletion is not supported directly over the API, delete all PackageRevisions resources of a package to delete a package", err.Error())
}
