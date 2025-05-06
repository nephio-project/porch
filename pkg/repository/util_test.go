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

package repository

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRevision2Int(t *testing.T) {
	assert.Equal(t, 123, Revision2Int("123"))
	assert.Equal(t, 123, Revision2Int("v123"))
	assert.Equal(t, -1, Revision2Int("V123"))
	assert.Equal(t, -1, Revision2Int("v123v"))
	assert.Equal(t, -1, Revision2Int("-1"))
	assert.Equal(t, 0, Revision2Int("v0"))
	assert.Equal(t, 0, Revision2Int("0"))
}

func TestRevision2Str(t *testing.T) {
	assert.Equal(t, "123", Revision2Str(123))
	assert.Equal(t, "-1", Revision2Str(-1))
}

func TestComposePkgObjName(t *testing.T) {
	pkgKey := PackageKey{
		RepoKey: RepositoryKey{
			Namespace:         "the-ns",
			Name:              "the-repo",
			Path:              "the/dir/path",
			PlaceholderWSname: "the-placeholder-ws-name",
		},
		Path:    "the/pkg/path",
		Package: "the-package-name",
	}

	assert.Equal(t, "the-repo.the.pkg.path.the-package-name", ComposePkgObjName(pkgKey))

	pkgKey.Path = ""
	assert.Equal(t, "the-repo.the-package-name", ComposePkgObjName(pkgKey))
}

func TestComposePkgRevObjName(t *testing.T) {
	pkgRevKey := PackageRevisionKey{
		PkgKey: PackageKey{
			RepoKey: RepositoryKey{
				Namespace:         "the-ns",
				Name:              "the-repo",
				Path:              "the/dir/path",
				PlaceholderWSname: "the-placeholder-ws-name",
			},
			Path:    "the/pkg/path",
			Package: "the-package-name",
		},
		Revision:      123,
		WorkspaceName: "the-ws-name",
	}

	assert.Equal(t, "the-repo.the.pkg.path.the-package-name.the-ws-name", ComposePkgRevObjName(pkgRevKey))

	pkgRevKey.Revision = -1
	assert.Equal(t, "the-repo.the.pkg.path.the-package-name.the-ws-name", ComposePkgRevObjName(pkgRevKey))
}
