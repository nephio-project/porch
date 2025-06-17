// Copyright 2025 The Nephio Authors
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

package dbcache

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/nephio-project/porch/api/porch/v1alpha1"
	cachetypes "github.com/nephio-project/porch/pkg/cache/types"
	"github.com/nephio-project/porch/pkg/repository"
	mockcachetypes "github.com/nephio-project/porch/test/mockery/mocks/porch/pkg/cache/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestPackageDBWriteRead(t *testing.T) {
	mockCache := mockcachetypes.NewMockCache(t)
	cachetypes.CacheInstance = mockCache
	mockCache.EXPECT().GetRepository(mock.Anything).Return(&dbRepository{})

	dbRepo := dbRepository{
		repoKey: repository.RepositoryKey{
			Namespace:         "my-ns",
			Name:              "my-repo",
			Path:              "my-path",
			PlaceholderWSname: "my-placeholder-ws",
		},
		meta:       metav1.ObjectMeta{},
		spec:       nil,
		updated:    time.Now().UTC(),
		updatedBy:  "porchuser",
		deployment: false,
	}

	dbPkg := dbPackage{
		pkgKey: repository.PackageKey{
			RepoKey: repository.RepositoryKey{
				Namespace: "my-ns",
				Name:      "my-repo",
			},
			Path:    "my-path-to-pkg",
			Package: "network-function",
		},
		meta:      metav1.ObjectMeta{},
		spec:      nil,
		updated:   time.Now().UTC(),
		updatedBy: "porchuser",
	}

	dbPkgUpdate := dbPackage{
		repo: &dbRepo,
		pkgKey: repository.PackageKey{
			RepoKey: dbRepo.Key(),
			Path:    "my-path-to-pkg",
			Package: "network-function",
		},
		meta: metav1.ObjectMeta{
			Name:      "meta-new-name",
			Namespace: "meta-new-namespace",
		},
		spec:      &v1alpha1.PackageSpec{},
		updated:   time.Now().UTC(),
		updatedBy: "porchuser2",
	}

	pkgDBWriteReadTest(t, dbRepo, dbPkg, dbPkgUpdate)

	dbPkg.pkgKey.Path = ""
	dbPkgUpdate.pkgKey.Path = ""
	dbPkgUpdate.updatedBy = "bart"
	pkgDBWriteReadTest(t, dbRepo, dbPkg, dbPkgUpdate)
}

func TestPackageDBSchema(t *testing.T) {
	dbPkg := dbPackage{}
	err := pkgWriteToDB(context.TODO(), &dbPkg)
	assert.NotNil(t, err)
	assert.True(t, strings.Contains(err.Error(), "violates check constraint"))

	dbPkg.pkgKey = repository.PackageKey{
		RepoKey: repository.RepositoryKey{
			Namespace: "my-ns",
		},
	}
	err = pkgWriteToDB(context.TODO(), &dbPkg)
	assert.NotNil(t, err)
	assert.True(t, strings.Contains(err.Error(), "violates check constraint"))

	dbPkg.pkgKey.RepoKey.Name = "my-repo"
	err = pkgWriteToDB(context.TODO(), &dbPkg)
	assert.NotNil(t, err)
	assert.True(t, strings.Contains(err.Error(), "violates check constraint"))

	dbPkg.pkgKey.Package = "my-package"
	err = pkgWriteToDB(context.TODO(), &dbPkg)
	assert.NotNil(t, err)
	assert.True(t, strings.Contains(err.Error(), "violates foreign key constraint"))

	dbRepo := dbRepository{
		repoKey: repository.RepositoryKey{
			Namespace: "my-ns",
			Name:      "my-repo",
		},
	}
	err = repoWriteToDB(context.TODO(), &dbRepo)
	assert.Nil(t, err)

	err = pkgWriteToDB(context.TODO(), &dbPkg)
	assert.Nil(t, err)

	dbPkg.pkgKey.Path = "my-path"
	err = pkgUpdateDB(context.TODO(), &dbPkg)
	assert.NotNil(t, err)
	assert.True(t, strings.Contains(err.Error(), "no rows or multiple rows found for updating"))

	dbPkg.pkgKey.Path = ""
	dbPkg.updatedBy = "Marge"
	err = pkgUpdateDB(context.TODO(), &dbPkg)
	assert.Nil(t, err)

	dbPkg.pkgKey.Path = "my-path"
	err = pkgUpdateDB(context.TODO(), &dbPkg)
	assert.NotNil(t, err)
	assert.True(t, strings.Contains(err.Error(), "no rows or multiple rows found for updating"))

	err = repoDeleteFromDB(context.TODO(), dbRepo.Key())
	assert.Nil(t, err)

	_, err = pkgReadFromDB(context.TODO(), dbPkg.Key())
	assert.NotNil(t, err)
	assert.Equal(t, sql.ErrNoRows, err)
}

func TestMultiPackageRepo(t *testing.T) {
	mockCache := mockcachetypes.NewMockCache(t)
	cachetypes.CacheInstance = mockCache
	mockCache.EXPECT().GetRepository(mock.Anything).Return(&dbRepository{})

	dbRepo11 := createTestRepo(t, "my-ns1", "my-repo1")
	dbRepo12 := createTestRepo(t, "my-ns1", "my-repo2")
	dbRepo21 := createTestRepo(t, "my-ns2", "my-repo1")
	dbRepo22 := createTestRepo(t, "my-ns2", "my-repo2")

	dbRepo11Pkgs := createTestPkgs(t, dbRepo11.Key(), "my-package", 4)
	dbRepo12Pkgs := createTestPkgs(t, dbRepo12.Key(), "my-package", 4)
	dbRepo21Pkgs := createTestPkgs(t, dbRepo21.Key(), "my-package", 4)
	dbRepo22Pkgs := createTestPkgs(t, dbRepo22.Key(), "my-package", 4)

	readRep11Pkgs, err := pkgReadPkgsFromDB(context.TODO(), dbRepo11.Key())
	assert.Nil(t, err)

	readRep12Pkgs, err := pkgReadPkgsFromDB(context.TODO(), dbRepo12.Key())
	assert.Nil(t, err)

	readRep21Pkgs, err := pkgReadPkgsFromDB(context.TODO(), dbRepo21.Key())
	assert.Nil(t, err)

	readRep22Pkgs, err := pkgReadPkgsFromDB(context.TODO(), dbRepo22.Key())
	assert.Nil(t, err)

	assertPackageListsEqual(t, dbRepo11Pkgs, readRep11Pkgs, 4)
	assertPackageListsEqual(t, dbRepo12Pkgs, readRep12Pkgs, 4)
	assertPackageListsEqual(t, dbRepo21Pkgs, readRep21Pkgs, 4)
	assertPackageListsEqual(t, dbRepo22Pkgs, readRep22Pkgs, 4)

	deleteTestRepo(t, dbRepo11.Key())
	deleteTestRepo(t, dbRepo12.Key())
	deleteTestRepo(t, dbRepo21.Key())
	deleteTestRepo(t, dbRepo22.Key())
}

func pkgDBWriteReadTest(t *testing.T, dbRepo dbRepository, dbPkg, dbPkgUpdate dbPackage) {
	err := pkgWriteToDB(context.TODO(), &dbPkg)
	assert.NotNil(t, err)
	assert.True(t, strings.Contains(err.Error(), "violates foreign key constraint"))

	err = repoWriteToDB(context.TODO(), &dbRepo)
	assert.Nil(t, err)

	dbPkg.repo = &dbRepo
	dbPkg.pkgKey.RepoKey = dbRepo.Key()

	err = pkgWriteToDB(context.TODO(), &dbPkg)
	assert.Nil(t, err)

	readPkg, err := pkgReadFromDB(context.TODO(), dbPkg.Key())
	assert.Nil(t, err)
	assertPackagesEqual(t, &dbPkg, readPkg)

	err = pkgWriteToDB(context.TODO(), &dbPkgUpdate)
	assert.NotNil(t, err)
	assert.True(t, strings.Contains(err.Error(), "violates unique constraint"))

	err = pkgUpdateDB(context.TODO(), &dbPkgUpdate)
	assert.Nil(t, err)

	readPkg, err = pkgReadFromDB(context.TODO(), dbPkg.Key())
	assert.Nil(t, err)
	assertPackagesEqual(t, &dbPkgUpdate, readPkg)

	err = pkgDeleteFromDB(context.TODO(), dbPkg.Key())
	assert.Nil(t, err)

	_, err = pkgReadFromDB(context.TODO(), dbPkg.Key())
	assert.NotNil(t, err)
	assert.Equal(t, sql.ErrNoRows, err)

	err = pkgUpdateDB(context.TODO(), &dbPkgUpdate)
	assert.NotNil(t, err)
	assert.True(t, strings.Contains(err.Error(), "no rows or multiple rows found for updating"))

	err = pkgDeleteFromDB(context.TODO(), dbPkg.Key())
	assert.Nil(t, err)

	err = repoDeleteFromDB(context.TODO(), dbRepo.Key())
	assert.Nil(t, err)
}

func assertPackageListsEqual(t *testing.T, left []dbPackage, right []*dbPackage, count int) {
	assert.Equal(t, count, len(left))
	assert.Equal(t, count, len(right))

	leftMap := make(map[repository.PackageKey]*dbPackage)
	for _, leftPkg := range left {
		leftMap[leftPkg.Key()] = &leftPkg
	}

	rightMap := make(map[repository.PackageKey]*dbPackage)
	for _, rightPkg := range right {
		rightMap[rightPkg.Key()] = rightPkg
	}

	for leftKey, leftPkg := range leftMap {
		rightPkg, ok := rightMap[leftKey]
		assert.True(t, ok)

		assertPackagesEqual(t, leftPkg, rightPkg)
	}
}

func assertPackagesEqual(t *testing.T, left, right *dbPackage) {
	assert.Equal(t, left.Key(), right.Key())
	assert.Equal(t, left.meta.Namespace, right.meta.Namespace)
	assert.Equal(t, left.meta.Name, right.meta.Name)
	assert.Equal(t, left.spec, right.spec)
	assert.Equal(t, left.updated, right.updated)
	assert.Equal(t, left.updatedBy, right.updatedBy)
}
