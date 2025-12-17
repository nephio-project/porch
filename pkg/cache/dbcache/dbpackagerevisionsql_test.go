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

func (t *DbTestSuite) TestPackageRevisionDBWriteRead() {
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
			RepoKey: dbRepo.Key(),
			Path:    "my-path-to-pkg",
			Package: "network-function",
		},
		meta:      metav1.ObjectMeta{},
		spec:      nil,
		updated:   time.Now().UTC(),
		updatedBy: "porchuser",
	}

	dbPR := dbPackageRevision{
		pkgRevKey: repository.PackageRevisionKey{
			PkgKey:        dbPkg.Key(),
			WorkspaceName: "my-lovely-pr",
			Revision:      0,
		},
		meta:      metav1.ObjectMeta{},
		spec:      nil,
		updated:   time.Now().UTC(),
		updatedBy: "porchuser",
		lifecycle: "Draft",
		resources: map[string]string{},
	}

	dbPRUpdate := dbPackageRevision{
		repo:      &dbRepo,
		pkgRevKey: dbPR.Key(),
		meta: metav1.ObjectMeta{
			Name:      "meta-new-name",
			Namespace: "meta-new-namespace",
		},
		spec:      &porchapi.PackageRevisionSpec{},
		updated:   time.Now().UTC(),
		updatedBy: "porchuser2",
		lifecycle: "Proposed",
		resources: map[string]string{"Hello.txt": "Hello", "Goodbye.txt": "Goodbye"},
	}

	t.pkgRevDBWriteReadTest(&dbRepo, dbPkg, dbPR, dbPRUpdate)

	dbPkg.pkgKey.Path = ""
	dbPR.pkgRevKey.PkgKey = dbPkg.Key()
	dbPRUpdate.pkgRevKey.PkgKey = dbPkg.Key()
	dbPRUpdate.updatedBy = "bart"
	dbPRUpdate.lifecycle = "Proposed"
	dbPR.resources = map[string]string{"Hello.txt": "Hello", "Goodbye.txt": "Goodbye"}
	dbPRUpdate.resources = map[string]string{"Hello.txt": "Hello"}
	t.pkgRevDBWriteReadTest(&dbRepo, dbPkg, dbPR, dbPRUpdate)

	dbPRUpdate.lifecycle = "Draft"
	dbPR.resources = map[string]string{"Hello.txt": "Hello", "Goodbye.txt": "Goodbye"}
	dbPRUpdate.resources = map[string]string{"AAA": "ZZZ", "BBB": "YYY"}
	t.pkgRevDBWriteReadTest(&dbRepo, dbPkg, dbPR, dbPRUpdate)

	dbPRUpdate.lifecycle = "Draft"
	dbPR.resources = map[string]string{"Hello.txt": "Hello", "Goodbye.txt": "Goodbye"}
	dbPRUpdate.resources = map[string]string{}
	t.pkgRevDBWriteReadTest(&dbRepo, dbPkg, dbPR, dbPRUpdate)
}

func (t *DbTestSuite) TestPackageRevisionLatest() {
	mockCache := mockcachetypes.NewMockCache(t.T())
	cachetypes.CacheInstance = mockCache
	mockCache.EXPECT().GetRepository(mock.Anything).Return(&dbRepository{})

	dbRepo := t.createTestRepo("my-ns", "my-repo")
	dbPkg := t.createTestPkg(dbRepo.Key(), "my-package")
	dbPkg.repo = dbRepo

	dbPR1 := dbPackageRevision{
		pkgRevKey: repository.PackageRevisionKey{
			PkgKey:        dbPkg.Key(),
			WorkspaceName: "my-lovely-pr-1",
			Revision:      0,
		},
		meta:      metav1.ObjectMeta{},
		spec:      nil,
		updated:   time.Now().UTC(),
		updatedBy: "porchuser",
		lifecycle: "Draft",
		resources: map[string]string{},
	}

	latestPR, err := pkgRevReadLatestPRFromDB(t.Context(), dbPR1.pkgRevKey.PkgKey)
	t.Require().NoError(err)
	t.Require().Nil(latestPR)

	err = pkgRevWriteToDB(t.Context(), &dbPR1)
	t.Require().NoError(err)

	// Latest PR is only set on published PRs
	latestPR, err = pkgRevReadLatestPRFromDB(t.Context(), dbPR1.pkgRevKey.PkgKey)
	t.Require().NoError(err)
	t.Require().Nil(latestPR)

	dbPR1.lifecycle = "Proposed"
	err = pkgRevUpdateDB(t.Context(), &dbPR1, true)
	t.Require().NoError(err)

	dbPR1.pkgRevKey.Revision = 1
	dbPR1.lifecycle = "Published"
	err = pkgRevUpdateDB(t.Context(), &dbPR1, true)
	t.Require().NoError(err)

	latestPR, err = pkgRevReadLatestPRFromDB(t.Context(), dbPR1.pkgRevKey.PkgKey)
	t.Require().NoError(err)
	t.Equal(1, latestPR.pkgRevKey.Revision)

	resources, err := pkgRevResourcesReadFromDB(t.Context(), latestPR.Key())
	t.Require().NoError(err)

	latestPR.resources = resources
	t.assertPackageRevsEqual(&dbPR1, latestPR)

	dbPR2 := dbPackageRevision{
		pkgRevKey: repository.PackageRevisionKey{
			PkgKey:        dbPkg.Key(),
			WorkspaceName: "my-lovely-pr-2",
			Revision:      0,
		},
		meta:      metav1.ObjectMeta{},
		spec:      nil,
		updated:   time.Now().UTC(),
		updatedBy: "porchuser",
		lifecycle: "Draft",
		resources: map[string]string{},
	}

	err = pkgRevWriteToDB(t.Context(), &dbPR2)
	t.Require().NoError(err)

	latestPR, err = pkgRevReadLatestPRFromDB(t.Context(), dbPR1.pkgRevKey.PkgKey)
	t.Require().NoError(err)
	t.Equal(1, latestPR.pkgRevKey.Revision)

	dbPR3 := dbPackageRevision{
		pkgRevKey: repository.PackageRevisionKey{
			PkgKey:        dbPkg.Key(),
			WorkspaceName: "my-lovely-pr-3",
			Revision:      10,
		},
		meta:      metav1.ObjectMeta{},
		spec:      nil,
		updated:   time.Now().UTC(),
		updatedBy: "porchuser",
		lifecycle: "Published",
		resources: map[string]string{},
	}

	err = pkgRevWriteToDB(t.Context(), &dbPR3)
	t.Require().NoError(err)

	latestPR, err = pkgRevReadLatestPRFromDB(t.Context(), dbPR1.pkgRevKey.PkgKey)
	t.Require().NoError(err)
	t.Equal(10, latestPR.pkgRevKey.Revision)

	dbPR4 := dbPackageRevision{
		pkgRevKey: repository.PackageRevisionKey{
			PkgKey:        dbPkg.Key(),
			WorkspaceName: "my-lovely-pr-4",
			Revision:      10,
		},
		meta:      metav1.ObjectMeta{},
		spec:      nil,
		updated:   time.Now().UTC(),
		updatedBy: "porchuser",
		lifecycle: "Published",
		resources: map[string]string{},
	}

	err = pkgRevWriteToDB(t.Context(), &dbPR4)
	t.Require().ErrorContains(err, "revision 10 already exists")

	latestPR, err = pkgRevReadLatestPRFromDB(t.Context(), dbPR1.pkgRevKey.PkgKey)
	t.Require().NoError(err)
	t.Equal(10, latestPR.pkgRevKey.Revision)

	dbPR4.pkgRevKey.Revision = 11
	err = pkgRevWriteToDB(t.Context(), &dbPR4)
	t.Require().NoError(err)

	latestPR, err = pkgRevReadLatestPRFromDB(t.Context(), dbPR1.pkgRevKey.PkgKey)
	t.Require().NoError(err)
	t.Equal(11, latestPR.pkgRevKey.Revision)

	err = repoDeleteFromDB(t.Context(), dbRepo.Key())
	t.Require().NoError(err)
}

func (t *DbTestSuite) TestPackageRevisionResources() {
	dbRepo := t.createTestRepo("my-ns", "my-repo")
	dbPkg := t.createTestPkg(dbRepo.Key(), "my-package")
	dbPkg.repo = dbRepo

	_, _, err := pkgRevResourceReadFromDB(t.Context(), repository.PackageRevisionKey{}, "")
	t.Require().ErrorContains(err, "no rows in result set")

	dbPR := t.createTestPR(dbPkg.Key(), "my_pr")
	dbPR.repo = dbRepo

	_, _, err = pkgRevResourceReadFromDB(t.Context(), dbPR.Key(), "resource.txt")
	t.Require().ErrorContains(err, "no rows in result set")

	resKey, resVal, err := pkgRevResourceReadFromDB(t.Context(), dbPR.Key(), "Hello.txt")
	t.Require().NoError(err)
	t.Equal("Hello.txt", resKey)
	t.Equal("Hello", resVal)

	resKey, resVal, err = pkgRevResourceReadFromDB(t.Context(), dbPR.Key(), "Goodbye.txt")
	t.Require().NoError(err)
	t.Equal("Goodbye.txt", resKey)
	t.Equal("Goodbye", resVal)

	err = pkgRevResourceDeleteFromDB(t.Context(), dbPR.Key(), "Goodbye.txt")
	t.Require().NoError(err)

	err = pkgRevResourceDeleteFromDB(t.Context(), dbPR.Key(), "Goodbye.txt")
	t.Require().NoError(err)

	_, _, err = pkgRevResourceReadFromDB(t.Context(), dbPR.Key(), "Goodbye.txt")
	t.Require().ErrorContains(err, "no rows in result set")

	err = pkgRevResourceWriteToDB(t.Context(), dbPR.Key(), "Goodbye.txt", "So long")
	t.Require().NoError(err)

	resKey, resVal, err = pkgRevResourceReadFromDB(t.Context(), dbPR.Key(), "Goodbye.txt")
	t.Require().NoError(err)
	t.Equal("Goodbye.txt", resKey)
	t.Equal("So long", resVal)

	err = pkgRevResourceWriteToDB(t.Context(), dbPR.Key(), "Goodbye.txt", "See ya later")
	t.Require().NoError(err)

	resKey, resVal, err = pkgRevResourceReadFromDB(t.Context(), dbPR.Key(), "Goodbye.txt")
	t.Require().NoError(err)
	t.Equal("Goodbye.txt", resKey)
	t.Equal("See ya later", resVal)

	err = pkgRevResourceWriteToDB(t.Context(), dbPR.Key(), "Grand.txt", "Grand")
	t.Require().NoError(err)

	resKey, resVal, err = pkgRevResourceReadFromDB(t.Context(), dbPR.Key(), "Grand.txt")
	t.Require().NoError(err)
	t.Equal("Grand.txt", resKey)
	t.Equal("Grand", resVal)

	dbPR.pkgRevKey.WorkspaceName = "bad"
	err = pkgRevResourceWriteToDB(t.Context(), dbPR.Key(), "Grand.txt", "Grand")
	t.Require().ErrorContains(err, "violates foreign key constraint")

	err = repoDeleteFromDB(t.Context(), dbRepo.Key())
	t.Require().NoError(err)
}

func (t *DbTestSuite) TestPackageRevisionDBSchema() {
	dbPR := dbPackageRevision{}
	err := pkgRevWriteToDB(t.Context(), &dbPR)
	t.Require().ErrorContains(err, "revision value of 0 is only allowed on when lifecycle is Draft or Proposed")

	dbPR.pkgRevKey = repository.PackageRevisionKey{
		PkgKey: repository.PackageKey{
			RepoKey: repository.RepositoryKey{
				Namespace: "my-ns",
			},
		},
	}

	err = pkgRevWriteToDB(t.Context(), &dbPR)
	t.Require().ErrorContains(err, "revision value of 0 is only allowed on when lifecycle is Draft or Proposed")

	dbPR.lifecycle = porchapi.PackageRevisionLifecycleDraft
	dbPR.pkgRevKey.PkgKey.RepoKey.Name = "my-repo"
	err = pkgRevWriteToDB(t.Context(), &dbPR)
	t.Require().ErrorContains(err, "violates check constraint")

	dbPR.pkgRevKey.PkgKey.Package = "my-package"
	err = pkgRevWriteToDB(t.Context(), &dbPR)
	t.Require().ErrorContains(err, "violates check constraint")

	dbPR.pkgRevKey.WorkspaceName = "my-ws"
	err = pkgRevWriteToDB(t.Context(), &dbPR)
	t.Require().ErrorContains(err, "violates foreign key constraint")

	dbPR.lifecycle = ""
	err = pkgRevWriteToDB(t.Context(), &dbPR)
	t.Require().ErrorContains(err, "revision value of 0 is only allowed on when lifecycle is Draft or Proposed")

	dbPR.lifecycle = "StoneDead"
	err = pkgRevWriteToDB(t.Context(), &dbPR)
	t.Require().ErrorContains(err, "revision value of 0 is only allowed on when lifecycle is Draft or Proposed")

	dbPR.lifecycle = "Draft"
	err = pkgRevWriteToDB(t.Context(), &dbPR)
	t.Require().ErrorContains(err, "violates foreign key constraint")

	dbRepo := dbRepository{
		repoKey: repository.RepositoryKey{
			Namespace: "my-ns",
			Name:      "my-repo",
		},
	}

	err = repoWriteToDB(t.Context(), &dbRepo)
	t.Require().NoError(err)

	dbPkg := dbPackage{
		pkgKey: repository.PackageKey{
			RepoKey: dbRepo.Key(),
			Package: "my-package",
		},
	}

	err = pkgWriteToDB(t.Context(), &dbPkg)
	t.Require().NoError(err)

	err = pkgRevWriteToDB(t.Context(), &dbPR)
	t.Require().NoError(err)

	dbPR.pkgRevKey.WorkspaceName = "my-other-ws"
	err = pkgRevUpdateDB(t.Context(), &dbPR, true)
	t.Require().ErrorContains(err, "no rows or multiple rows found for updating")

	dbPR.pkgRevKey.WorkspaceName = "my-ws"
	dbPR.updatedBy = "Marge"
	err = pkgRevUpdateDB(t.Context(), &dbPR, true)
	t.Require().NoError(err)

	err = repoDeleteFromDB(t.Context(), dbRepo.Key())
	t.Require().NoError(err)

	_, err = pkgRevReadFromDB(t.Context(), dbPR.Key(), false)
	t.Require().NotNil(err)
	t.Equal(sql.ErrNoRows, err)
}

func (t *DbTestSuite) TestMultiPackageRevisionRepo() {
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

	dbRepo11PkgsPRs := t.createTestPRs(dbRepo11Pkgs, "my-ws", 4)
	dbRepo12PkgsPRs := t.createTestPRs(dbRepo12Pkgs, "my-ws", 4)
	dbRepo21PkgsPRs := t.createTestPRs(dbRepo21Pkgs, "my-ws", 4)
	dbRepo22PkgsPRs := t.createTestPRs(dbRepo22Pkgs, "my-ws", 4)

	readRepo11PkgsPRs := t.readRepoPkgPRs(dbRepo11Pkgs)
	readRepo12PkgsPRs := t.readRepoPkgPRs(dbRepo12Pkgs)
	readRepo21PkgsPRs := t.readRepoPkgPRs(dbRepo21Pkgs)
	readRepo22PkgsPRs := t.readRepoPkgPRs(dbRepo22Pkgs)

	t.assertPackageRevListsEqual(dbRepo11PkgsPRs, readRepo11PkgsPRs, 16)
	t.assertPackageRevListsEqual(dbRepo12PkgsPRs, readRepo12PkgsPRs, 16)
	t.assertPackageRevListsEqual(dbRepo21PkgsPRs, readRepo21PkgsPRs, 16)
	t.assertPackageRevListsEqual(dbRepo22PkgsPRs, readRepo22PkgsPRs, 16)

	t.assertPackageRevLatestIs(4, readRepo11PkgsPRs)
	t.assertPackageRevLatestIs(4, readRepo12PkgsPRs)
	t.assertPackageRevLatestIs(4, readRepo21PkgsPRs)
	t.assertPackageRevLatestIs(4, readRepo22PkgsPRs)

	t.deleteTestRepo(dbRepo11.Key())
	t.deleteTestRepo(dbRepo12.Key())
	t.deleteTestRepo(dbRepo21.Key())
	t.deleteTestRepo(dbRepo22.Key())

	t.Empty(t.readRepoPkgPRs(dbRepo11Pkgs))
	t.Empty(t.readRepoPkgPRs(dbRepo12Pkgs))
	t.Empty(t.readRepoPkgPRs(dbRepo21Pkgs))
	t.Empty(t.readRepoPkgPRs(dbRepo22Pkgs))
}

func (t *DbTestSuite) TestPackageRevisionFilter() {
	mockCache := mockcachetypes.NewMockCache(t.T())
	cachetypes.CacheInstance = mockCache
	mockCache.EXPECT().GetRepository(mock.Anything).Return(&dbRepository{})

	dbRepo := t.createTestRepo("my-ns", "my-repo")

	dbRepoPkgs := t.createTestPkgs(dbRepo.Key(), "my-package", 4)

	_ = t.createTestPRs(dbRepoPkgs, "my-ws", 4)

	prFilter := repository.ListPackageRevisionFilter{}
	listPRs, err := pkgRevListPRsFromDB(t.Context(), prFilter)
	t.Require().NoError(err)
	t.Equal(16, len(listPRs))

	prFilter.Key.WorkspaceName = "my-ws-2"
	listPRs, err = pkgRevListPRsFromDB(t.Context(), prFilter)
	t.Require().NoError(err)
	t.Equal(4, len(listPRs))

	prFilter.Key.Revision = 3
	listPRs, err = pkgRevListPRsFromDB(t.Context(), prFilter)
	t.Require().NoError(err)
	t.Equal(4, len(listPRs))

	prFilter.Key.Revision = 1
	listPRs, err = pkgRevListPRsFromDB(t.Context(), prFilter)
	t.Require().NoError(err)
	t.Equal(0, len(listPRs))

	t.deleteTestRepo(dbRepo.Key())

	t.Empty(t.readRepoPkgPRs(dbRepoPkgs))
}

func (t *DbTestSuite) TestMultiPackageRevisionList() {
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

	dbRepo11PkgsPRs := t.createTestPRs(dbRepo11Pkgs, "my-ws", 4)
	dbRepo12PkgsPRs := t.createTestPRs(dbRepo12Pkgs, "my-ws", 4)
	dbRepo21PkgsPRs := t.createTestPRs(dbRepo21Pkgs, "my-ws", 4)
	dbRepo22PkgsPRs := t.createTestPRs(dbRepo22Pkgs, "my-ws", 4)

	listRepo11PkgsPRs := t.listRepoPkgPRs(dbRepo11Pkgs)
	listRepo12PkgsPRs := t.listRepoPkgPRs(dbRepo12Pkgs)
	listRepo21PkgsPRs := t.listRepoPkgPRs(dbRepo21Pkgs)
	listRepo22PkgsPRs := t.listRepoPkgPRs(dbRepo22Pkgs)

	t.assertPackageRevListsEqual(dbRepo11PkgsPRs, listRepo11PkgsPRs, 16)
	t.assertPackageRevListsEqual(dbRepo12PkgsPRs, listRepo12PkgsPRs, 16)
	t.assertPackageRevListsEqual(dbRepo21PkgsPRs, listRepo21PkgsPRs, 16)
	t.assertPackageRevListsEqual(dbRepo22PkgsPRs, listRepo22PkgsPRs, 16)

	t.assertPackageRevLatestIs(4, listRepo11PkgsPRs)
	t.assertPackageRevLatestIs(4, listRepo12PkgsPRs)
	t.assertPackageRevLatestIs(4, listRepo21PkgsPRs)
	t.assertPackageRevLatestIs(4, listRepo22PkgsPRs)

	t.deleteTestRepo(dbRepo11.Key())
	t.deleteTestRepo(dbRepo12.Key())
	t.deleteTestRepo(dbRepo21.Key())
	t.deleteTestRepo(dbRepo22.Key())

	t.Empty(t.readRepoPkgPRs(dbRepo11Pkgs))
	t.Empty(t.readRepoPkgPRs(dbRepo12Pkgs))
	t.Empty(t.readRepoPkgPRs(dbRepo21Pkgs))
	t.Empty(t.readRepoPkgPRs(dbRepo22Pkgs))
}

func (t *DbTestSuite) pkgRevDBWriteReadTest(dbRepo *dbRepository, dbPkg dbPackage, dbPR, dbPRUpdate dbPackageRevision) {
	err := pkgRevWriteToDB(t.Context(), &dbPR)
	t.Require().ErrorContains(err, "violates foreign key constraint")

	err = repoWriteToDB(t.Context(), dbRepo)
	t.Require().NoError(err)

	dbPkg.repo = dbRepo
	dbPkg.pkgKey.RepoKey = dbRepo.Key()

	err = pkgWriteToDB(t.Context(), &dbPkg)
	t.Require().NoError(err)

	dbPR.repo = dbRepo
	dbPR.pkgRevKey.PkgKey = dbPkg.Key()

	err = pkgRevWriteToDB(t.Context(), &dbPR)
	t.Require().NoError(err)

	readPR, err := pkgRevReadFromDB(t.Context(), dbPR.Key(), false)
	t.Require().NoError(err)

	resources, err := pkgRevResourcesReadFromDB(t.Context(), readPR.Key())
	t.Require().NoError(err)

	readPR.resources = resources

	t.assertPackageRevsEqual(&dbPR, readPR)

	err = pkgRevWriteToDB(t.Context(), &dbPRUpdate)
	t.Require().ErrorContains(err, "violates unique constraint")

	err = pkgRevUpdateDB(t.Context(), &dbPRUpdate, true)
	t.Require().NoError(err)

	readPR, err = pkgRevReadFromDB(t.Context(), dbPR.Key(), false)
	t.Require().NoError(err)

	resources, err = pkgRevResourcesReadFromDB(t.Context(), readPR.Key())
	t.Require().NoError(err)

	readPR.resources = resources

	t.assertPackageRevsEqual(&dbPRUpdate, readPR)

	err = pkgRevDeleteFromDB(t.Context(), dbPR.Key())
	t.Require().NoError(err)

	_, err = pkgRevReadFromDB(t.Context(), dbPR.Key(), false)
	t.Require().NotNil(err)
	t.Equal(sql.ErrNoRows, err)

	err = pkgRevUpdateDB(t.Context(), &dbPRUpdate, false)
	t.Require().ErrorContains(err, "no rows or multiple rows found for updating")

	err = pkgRevDeleteFromDB(t.Context(), dbPR.Key())
	t.Require().NoError(err)

	err = pkgDeleteFromDB(t.Context(), dbPkg.Key())
	t.Require().NoError(err)

	err = repoDeleteFromDB(t.Context(), dbRepo.Key())
	t.Require().NoError(err)
}

func (t *DbTestSuite) readRepoPkgPRs(pkgs []dbPackage) []*dbPackageRevision {
	var readRepoPkgsPRs []*dbPackageRevision

	for _, pkg := range pkgs {
		readPRs, err := pkgRevReadPRsFromDB(t.Context(), pkg.Key())
		t.Require().NoError(err)

		readRepoPkgsPRs = append(readRepoPkgsPRs, readPRs...)
	}

	return readRepoPkgsPRs
}

func (t *DbTestSuite) listRepoPkgPRs(pkgs []dbPackage) []*dbPackageRevision {
	var listRepoPkgsPRs []*dbPackageRevision

	for _, pkg := range pkgs {
		prFilter := repository.ListPackageRevisionFilter{
			Key: repository.PackageRevisionKey{
				PkgKey: pkg.Key(),
			},
		}

		listPRs, err := pkgRevListPRsFromDB(t.Context(), prFilter)
		t.Require().NoError(err)

		listRepoPkgsPRs = append(listRepoPkgsPRs, listPRs...)
	}

	return listRepoPkgsPRs
}

func (t *DbTestSuite) assertPackageRevListsEqual(left []dbPackageRevision, right []*dbPackageRevision, count int) {
	t.Len(left, count)
	t.Len(right, count)

	leftMap := make(map[repository.PackageRevisionKey]*dbPackageRevision)
	for _, leftPr := range left {
		resources, err := pkgRevResourcesReadFromDB(t.Context(), leftPr.Key())
		t.Require().NoError(err)

		leftPr.resources = resources
		leftMap[leftPr.Key()] = &leftPr
	}

	rightMap := make(map[repository.PackageRevisionKey]*dbPackageRevision)
	for _, rightPr := range right {
		rightMap[rightPr.Key()] = rightPr

		resources, err := pkgRevResourcesReadFromDB(t.Context(), rightPr.Key())
		t.Require().NoError(err)

		rightPr.resources = resources
	}

	for leftKey, leftPr := range leftMap {
		rightPr, ok := rightMap[leftKey]
		t.True(ok)

		t.assertPackageRevsEqual(leftPr, rightPr)
	}
}

func (t *DbTestSuite) assertPackageRevsEqual(left, right *dbPackageRevision) {
	t.Equal(left.Key(), right.Key())
	t.Equal(left.meta.Namespace, right.meta.Namespace)
	t.Equal(left.meta.Name, right.meta.Name)
	t.Equal(left.spec, right.spec)
	t.Equal(left.updatedBy, right.updatedBy)
	t.Equal(left.lifecycle, right.lifecycle)
	t.Equal(left.tasks, right.tasks)
	t.Equal(left.resources, right.resources)
}

func (t *DbTestSuite) assertPackageRevLatestIs(expectedLatest int, prList []*dbPackageRevision) {
	for _, pr := range prList {
		latestPrRev, err := pkgRevGetlatestRevFromDB(t.Context(), pr.Key().PkgKey)
		t.Require().NoError(err)
		t.Equal(expectedLatest, latestPrRev)
	}
}
