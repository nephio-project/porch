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
	"database/sql"
	"time"

	porchapi "github.com/nephio-project/porch/api/porch"
	cachetypes "github.com/nephio-project/porch/pkg/cache/types"
	"github.com/nephio-project/porch/pkg/repository"
	mockcachetypes "github.com/nephio-project/porch/test/mockery/mocks/porch/pkg/cache/types"
	"github.com/stretchr/testify/mock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (t *DbTestSuite) TestPackageDBWriteRead() {
	mockCache := mockcachetypes.NewMockCache(t.T())
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
		spec:      &porchapi.PackageSpec{},
		updated:   time.Now().UTC(),
		updatedBy: "porchuser2",
	}

	t.pkgDBWriteReadTest(&dbRepo, dbPkg, dbPkgUpdate)

	dbPkg.pkgKey.Path = ""
	dbPkgUpdate.pkgKey.Path = ""
	dbPkgUpdate.updatedBy = "bart"
	t.pkgDBWriteReadTest(&dbRepo, dbPkg, dbPkgUpdate)
}

func (t *DbTestSuite) TestPackageDBSchema() {
	dbPkg := dbPackage{}
	err := pkgWriteToDB(t.Context(), &dbPkg)
	t.Require().ErrorContains(err, "violates check constraint")

	dbPkg.pkgKey = repository.PackageKey{
		RepoKey: repository.RepositoryKey{
			Namespace: "my-ns",
		},
	}
	err = pkgWriteToDB(t.Context(), &dbPkg)
	t.Require().ErrorContains(err, "violates check constraint")

	dbPkg.pkgKey.RepoKey.Name = "my-repo"
	err = pkgWriteToDB(t.Context(), &dbPkg)
	t.Require().ErrorContains(err, "violates check constraint")

	dbPkg.pkgKey.Package = "my-package"
	err = pkgWriteToDB(t.Context(), &dbPkg)
	t.Require().ErrorContains(err, "violates foreign key constraint")

	dbRepo := dbRepository{
		repoKey: repository.RepositoryKey{
			Namespace: "my-ns",
			Name:      "my-repo",
		},
	}
	err = repoWriteToDB(t.Context(), &dbRepo)
	t.Require().NoError(err)

	err = pkgWriteToDB(t.Context(), &dbPkg)
	t.Require().NoError(err)

	dbPkg.pkgKey.Path = "my-path"
	err = pkgUpdateDB(t.Context(), &dbPkg)
	t.Require().ErrorContains(err, "no rows or multiple rows found for updating")

	dbPkg.pkgKey.Path = ""
	dbPkg.updatedBy = "Marge"
	err = pkgUpdateDB(t.Context(), &dbPkg)
	t.Require().NoError(err)

	dbPkg.pkgKey.Path = "my-path"
	err = pkgUpdateDB(t.Context(), &dbPkg)
	t.Require().ErrorContains(err, "no rows or multiple rows found for updating")

	err = repoDeleteFromDB(t.Context(), dbRepo.Key())
	t.Require().NoError(err)

	_, err = pkgReadFromDB(t.Context(), dbPkg.Key())
	t.Require().NotNil(err)
	t.Equal(sql.ErrNoRows, err)
}

func (t *DbTestSuite) TestMultiPackageRepo() {
	mockCache := mockcachetypes.NewMockCache(t.T())
	cachetypes.CacheInstance = mockCache
	mockCache.EXPECT().GetRepository(mock.Anything).Return(&dbRepository{})

	dbRepo11 := t.createTestRepo("my-ns1", "my-repo1")
	dbRepo12 := t.createTestRepo("my-ns1", "my-repo2")
	dbRepo21 := t.createTestRepo("my-ns2", "my-repo1")
	dbRepo22 := t.createTestRepo("my-ns2", "my-repo2")

	dbRepo11Pkgs := t.createTestPkgs(dbRepo11.Key(), "my-package", 4)
	dbRepo12Pkgs := t.createTestPkgs(dbRepo12.Key(), "my-package", 4)
	dbRepo21Pkgs := t.createTestPkgs(dbRepo21.Key(), "my-package", 4)
	dbRepo22Pkgs := t.createTestPkgs(dbRepo22.Key(), "my-package", 4)

	readRep11Pkgs, err := pkgReadPkgsFromDB(t.Context(), dbRepo11.Key())
	t.Require().NoError(err)

	readRep12Pkgs, err := pkgReadPkgsFromDB(t.Context(), dbRepo12.Key())
	t.Require().NoError(err)

	readRep21Pkgs, err := pkgReadPkgsFromDB(t.Context(), dbRepo21.Key())
	t.Require().NoError(err)

	readRep22Pkgs, err := pkgReadPkgsFromDB(t.Context(), dbRepo22.Key())
	t.Require().NoError(err)

	t.assertPackageListsEqual(dbRepo11Pkgs, readRep11Pkgs, 4)
	t.assertPackageListsEqual(dbRepo12Pkgs, readRep12Pkgs, 4)
	t.assertPackageListsEqual(dbRepo21Pkgs, readRep21Pkgs, 4)
	t.assertPackageListsEqual(dbRepo22Pkgs, readRep22Pkgs, 4)

	t.deleteTestRepo(dbRepo11.Key())
	t.deleteTestRepo(dbRepo12.Key())
	t.deleteTestRepo(dbRepo21.Key())
	t.deleteTestRepo(dbRepo22.Key())
}

func (t *DbTestSuite) TestPackageFilter() {
	mockCache := mockcachetypes.NewMockCache(t.T())
	cachetypes.CacheInstance = mockCache
	mockCache.EXPECT().GetRepository(mock.Anything).Return(&dbRepository{})

	dbRepo := t.createTestRepo("my-ns", "my-repo")

	dbRepoPkgs := t.createTestPkgs(dbRepo.Key(), "my-package", 4)

	pkgFilter := repository.ListPackageFilter{}
	listPkgs, err := pkgListPkgsFromDB(t.Context(), pkgFilter)
	t.Require().NoError(err)
	t.Len(listPkgs, 4)

	pkgFilter.Key.RepoKey = dbRepo.repoKey
	pkgFilter.Key.Package = "my-package-2"
	listPkgs, err = pkgListPkgsFromDB(t.Context(), pkgFilter)
	t.Require().NoError(err)
	t.Len(listPkgs, 1)

	pkgFilter.Key.Package = "my-package-5"
	listPkgs, err = pkgListPkgsFromDB(t.Context(), pkgFilter)
	t.Require().NoError(err)
	t.Empty(listPkgs)

	pkgFilter.Key.Package = "my-package-2"
	pkgFilter.Key.Path = "a/path"
	listPkgs, err = pkgListPkgsFromDB(t.Context(), pkgFilter)
	t.Require().NoError(err)
	t.Empty(listPkgs)

	pkgFilter.Key.Path = ""
	listPkgs, err = pkgListPkgsFromDB(t.Context(), pkgFilter)
	t.Require().NoError(err)
	t.Len(listPkgs, 1)

	t.deleteTestRepo(dbRepo.Key())

	t.Empty(t.readRepoPkgPRs(dbRepoPkgs))
}

func (t *DbTestSuite) pkgDBWriteReadTest(dbRepo *dbRepository, dbPkg, dbPkgUpdate dbPackage) {
	err := pkgWriteToDB(t.Context(), &dbPkg)
	t.Require().ErrorContains(err, "violates foreign key constraint")

	err = repoWriteToDB(t.Context(), dbRepo)
	t.Require().NoError(err)

	dbPkg.repo = dbRepo
	dbPkg.pkgKey.RepoKey = dbRepo.Key()

	err = pkgWriteToDB(t.Context(), &dbPkg)
	t.Require().NoError(err)

	readPkg, err := pkgReadFromDB(t.Context(), dbPkg.Key())
	t.Require().NoError(err)
	t.assertPackagesEqual(&dbPkg, readPkg)

	err = pkgWriteToDB(t.Context(), &dbPkgUpdate)
	t.Require().ErrorContains(err, "violates unique constraint")

	err = pkgUpdateDB(t.Context(), &dbPkgUpdate)
	t.Require().NoError(err)

	readPkg, err = pkgReadFromDB(t.Context(), dbPkg.Key())
	t.Require().NoError(err)
	t.assertPackagesEqual(&dbPkgUpdate, readPkg)

	err = pkgDeleteFromDB(t.Context(), dbPkg.Key())
	t.Require().NoError(err)

	_, err = pkgReadFromDB(t.Context(), dbPkg.Key())
	t.Require().NotNil(err)
	t.Equal(sql.ErrNoRows, err)

	err = pkgUpdateDB(t.Context(), &dbPkgUpdate)
	t.Require().ErrorContains(err, "no rows or multiple rows found for updating")

	err = pkgDeleteFromDB(t.Context(), dbPkg.Key())
	t.Require().NoError(err)

	err = repoDeleteFromDB(t.Context(), dbRepo.Key())
	t.Require().NoError(err)
}

func (t *DbTestSuite) assertPackageListsEqual(left []dbPackage, right []*dbPackage, count int) {
	t.Len(left, count)
	t.Len(right, count)

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
		t.True(ok)

		t.assertPackagesEqual(leftPkg, rightPkg)
	}
}

func (t *DbTestSuite) assertPackagesEqual(left, right *dbPackage) {
	t.Equal(left.Key(), right.Key())
	t.Equal(left.meta.Namespace, right.meta.Namespace)
	t.Equal(left.meta.Name, right.meta.Name)
	t.Equal(left.spec, right.spec)
	t.Equal(left.updatedBy, right.updatedBy)
}
