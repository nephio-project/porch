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

func TestPackageRevisionDBWriteRead(t *testing.T) {
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
		spec:      &v1alpha1.PackageRevisionSpec{},
		updated:   time.Now().UTC(),
		updatedBy: "porchuser2",
		lifecycle: "Proposed",
		resources: map[string]string{"Hello.txt": "Hello", "Goodbye.txt": "Goodbye"},
	}

	pkgRevDBWriteReadTest(t, &dbRepo, dbPkg, dbPR, dbPRUpdate)

	dbPkg.pkgKey.Path = ""
	dbPR.pkgRevKey.PkgKey = dbPkg.Key()
	dbPRUpdate.pkgRevKey.PkgKey = dbPkg.Key()
	dbPRUpdate.updatedBy = "bart"
	dbPRUpdate.lifecycle = "Proposed"
	dbPR.resources = map[string]string{"Hello.txt": "Hello", "Goodbye.txt": "Goodbye"}
	dbPRUpdate.resources = map[string]string{"Hello.txt": "Hello"}
	pkgRevDBWriteReadTest(t, &dbRepo, dbPkg, dbPR, dbPRUpdate)

	dbPRUpdate.lifecycle = "Draft"
	dbPR.resources = map[string]string{"Hello.txt": "Hello", "Goodbye.txt": "Goodbye"}
	dbPRUpdate.resources = map[string]string{"AAA": "ZZZ", "BBB": "YYY"}
	pkgRevDBWriteReadTest(t, &dbRepo, dbPkg, dbPR, dbPRUpdate)

	dbPRUpdate.lifecycle = "Draft"
	dbPR.resources = map[string]string{"Hello.txt": "Hello", "Goodbye.txt": "Goodbye"}
	dbPRUpdate.resources = map[string]string{}
	pkgRevDBWriteReadTest(t, &dbRepo, dbPkg, dbPR, dbPRUpdate)
}

func TestPackageRevisionLatest(t *testing.T) {
	mockCache := mockcachetypes.NewMockCache(t)
	cachetypes.CacheInstance = mockCache
	mockCache.EXPECT().GetRepository(mock.Anything).Return(&dbRepository{})

	dbRepo := createTestRepo(t, "my-ns", "my-repo")
	dbPkg := createTestPkg(t, dbRepo.Key(), "my-package")
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

	latestPR, err := pkgRevReadLatestPRFromDB(context.TODO(), dbPR1.pkgRevKey.PkgKey)
	assert.Nil(t, err)
	assert.Nil(t, latestPR)

	err = pkgRevWriteToDB(context.TODO(), &dbPR1)
	assert.Nil(t, err)

	// Latest PR is only set on published PRs
	latestPR, err = pkgRevReadLatestPRFromDB(context.TODO(), dbPR1.pkgRevKey.PkgKey)
	assert.Nil(t, err)
	assert.Nil(t, latestPR)

	dbPR1.lifecycle = "Proposed"
	err = pkgRevUpdateDB(context.TODO(), &dbPR1, true)
	assert.Nil(t, err)

	dbPR1.pkgRevKey.Revision = 1
	dbPR1.lifecycle = "Published"
	err = pkgRevUpdateDB(context.TODO(), &dbPR1, true)
	assert.Nil(t, err)

	latestPR, err = pkgRevReadLatestPRFromDB(context.TODO(), dbPR1.pkgRevKey.PkgKey)
	assert.Nil(t, err)
	assert.Equal(t, 1, latestPR.pkgRevKey.Revision)

	resources, err := pkgRevResourcesReadFromDB(context.TODO(), latestPR.Key())
	assert.Nil(t, err)

	latestPR.resources = resources
	assertPackageRevsEqual(t, &dbPR1, latestPR)

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

	err = pkgRevWriteToDB(context.TODO(), &dbPR2)
	assert.Nil(t, err)

	latestPR, err = pkgRevReadLatestPRFromDB(context.TODO(), dbPR1.pkgRevKey.PkgKey)
	assert.Nil(t, err)
	assert.Equal(t, 1, latestPR.pkgRevKey.Revision)

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

	err = pkgRevWriteToDB(context.TODO(), &dbPR3)
	assert.Nil(t, err)

	latestPR, err = pkgRevReadLatestPRFromDB(context.TODO(), dbPR1.pkgRevKey.PkgKey)
	assert.Nil(t, err)
	assert.Equal(t, 10, latestPR.pkgRevKey.Revision)

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

	err = pkgRevWriteToDB(context.TODO(), &dbPR4)
	assert.NotNil(t, err)
	assert.True(t, strings.Contains(err.Error(), "revision 10 already exists"))

	latestPR, err = pkgRevReadLatestPRFromDB(context.TODO(), dbPR1.pkgRevKey.PkgKey)
	assert.Nil(t, err)
	assert.Equal(t, 10, latestPR.pkgRevKey.Revision)

	dbPR4.pkgRevKey.Revision = 11
	err = pkgRevWriteToDB(context.TODO(), &dbPR4)
	assert.Nil(t, err)

	latestPR, err = pkgRevReadLatestPRFromDB(context.TODO(), dbPR1.pkgRevKey.PkgKey)
	assert.Nil(t, err)
	assert.Equal(t, 11, latestPR.pkgRevKey.Revision)

	err = repoDeleteFromDB(context.TODO(), dbRepo.Key())
	assert.Nil(t, err)
}

func TestPackageRevisionResources(t *testing.T) {
	dbRepo := createTestRepo(t, "my-ns", "my-repo")
	dbPkg := createTestPkg(t, dbRepo.Key(), "my-package")
	dbPkg.repo = dbRepo

	_, _, err := pkgRevResourceReadFromDB(context.TODO(), repository.PackageRevisionKey{}, "")
	assert.NotNil(t, err)
	assert.True(t, strings.Contains(err.Error(), "no rows in result set"))

	dbPR := createTestPR(t, dbPkg.Key(), "my_pr")
	dbPR.repo = dbRepo

	_, _, err = pkgRevResourceReadFromDB(context.TODO(), dbPR.Key(), "resource.txt")
	assert.NotNil(t, err)
	assert.True(t, strings.Contains(err.Error(), "no rows in result set"))

	resKey, resVal, err := pkgRevResourceReadFromDB(context.TODO(), dbPR.Key(), "Hello.txt")
	assert.Nil(t, err)
	assert.Equal(t, "Hello.txt", resKey)
	assert.Equal(t, "Hello", resVal)

	resKey, resVal, err = pkgRevResourceReadFromDB(context.TODO(), dbPR.Key(), "Goodbye.txt")
	assert.Nil(t, err)
	assert.Equal(t, "Goodbye.txt", resKey)
	assert.Equal(t, "Goodbye", resVal)

	err = pkgRevResourceDeleteFromDB(context.TODO(), dbPR.Key(), "Goodbye.txt")
	assert.Nil(t, err)

	err = pkgRevResourceDeleteFromDB(context.TODO(), dbPR.Key(), "Goodbye.txt")
	assert.Nil(t, err)

	_, _, err = pkgRevResourceReadFromDB(context.TODO(), dbPR.Key(), "Goodbye.txt")
	assert.NotNil(t, err)
	assert.True(t, strings.Contains(err.Error(), "no rows in result set"))

	err = pkgRevResourceWriteToDB(context.TODO(), dbPR.Key(), "Goodbye.txt", "So long")
	assert.Nil(t, err)

	resKey, resVal, err = pkgRevResourceReadFromDB(context.TODO(), dbPR.Key(), "Goodbye.txt")
	assert.Nil(t, err)
	assert.Equal(t, "Goodbye.txt", resKey)
	assert.Equal(t, "So long", resVal)

	err = pkgRevResourceWriteToDB(context.TODO(), dbPR.Key(), "Goodbye.txt", "See ya later")
	assert.Nil(t, err)

	resKey, resVal, err = pkgRevResourceReadFromDB(context.TODO(), dbPR.Key(), "Goodbye.txt")
	assert.Nil(t, err)
	assert.Equal(t, "Goodbye.txt", resKey)
	assert.Equal(t, "See ya later", resVal)

	err = pkgRevResourceWriteToDB(context.TODO(), dbPR.Key(), "Grand.txt", "Grand")
	assert.Nil(t, err)

	resKey, resVal, err = pkgRevResourceReadFromDB(context.TODO(), dbPR.Key(), "Grand.txt")
	assert.Nil(t, err)
	assert.Equal(t, "Grand.txt", resKey)
	assert.Equal(t, "Grand", resVal)

	dbPR.pkgRevKey.WorkspaceName = "bad"
	err = pkgRevResourceWriteToDB(context.TODO(), dbPR.Key(), "Grand.txt", "Grand")
	assert.NotNil(t, err)
	assert.True(t, strings.Contains(err.Error(), "violates foreign key constraint"))

	err = repoDeleteFromDB(context.TODO(), dbRepo.Key())
	assert.Nil(t, err)
}

func TestPackageRevisionDBSchema(t *testing.T) {
	dbPR := dbPackageRevision{}
	err := pkgRevWriteToDB(context.TODO(), &dbPR)
	assert.NotNil(t, err)
	assert.True(t, strings.Contains(err.Error(), "revision value of 0 is only allowed on when lifecycle is Draft or Proposed"))

	dbPR.pkgRevKey = repository.PackageRevisionKey{
		PkgKey: repository.PackageKey{
			RepoKey: repository.RepositoryKey{
				Namespace: "my-ns",
			},
		},
	}

	err = pkgRevWriteToDB(context.TODO(), &dbPR)
	assert.NotNil(t, err)
	assert.True(t, strings.Contains(err.Error(), "revision value of 0 is only allowed on when lifecycle is Draft or Proposed"))

	dbPR.lifecycle = v1alpha1.PackageRevisionLifecycleDraft
	dbPR.pkgRevKey.PkgKey.RepoKey.Name = "my-repo"
	err = pkgRevWriteToDB(context.TODO(), &dbPR)
	assert.NotNil(t, err)
	assert.True(t, strings.Contains(err.Error(), "violates check constraint"))

	dbPR.pkgRevKey.PkgKey.Package = "my-package"
	err = pkgRevWriteToDB(context.TODO(), &dbPR)
	assert.NotNil(t, err)
	assert.True(t, strings.Contains(err.Error(), "violates check constraint"))

	dbPR.pkgRevKey.WorkspaceName = "my-ws"
	err = pkgRevWriteToDB(context.TODO(), &dbPR)
	assert.NotNil(t, err)
	assert.True(t, strings.Contains(err.Error(), "violates foreign key constraint"))

	dbPR.lifecycle = ""
	err = pkgRevWriteToDB(context.TODO(), &dbPR)
	assert.NotNil(t, err)
	assert.True(t, strings.Contains(err.Error(), "revision value of 0 is only allowed on when lifecycle is Draft or Proposed"))

	dbPR.lifecycle = "StoneDead"
	err = pkgRevWriteToDB(context.TODO(), &dbPR)
	assert.NotNil(t, err)
	assert.True(t, strings.Contains(err.Error(), "revision value of 0 is only allowed on when lifecycle is Draft or Proposed"))

	dbPR.lifecycle = "Draft"
	err = pkgRevWriteToDB(context.TODO(), &dbPR)
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

	dbPkg := dbPackage{
		pkgKey: repository.PackageKey{
			RepoKey: dbRepo.Key(),
			Package: "my-package",
		},
	}

	err = pkgWriteToDB(context.TODO(), &dbPkg)
	assert.Nil(t, err)

	err = pkgRevWriteToDB(context.TODO(), &dbPR)
	assert.Nil(t, err)

	dbPR.pkgRevKey.WorkspaceName = "my-other-ws"
	err = pkgRevUpdateDB(context.TODO(), &dbPR, true)
	assert.NotNil(t, err)
	assert.True(t, strings.Contains(err.Error(), "no rows or multiple rows found for updating"))

	dbPR.pkgRevKey.WorkspaceName = "my-ws"
	dbPR.updatedBy = "Marge"
	err = pkgRevUpdateDB(context.TODO(), &dbPR, true)
	assert.Nil(t, err)

	err = repoDeleteFromDB(context.TODO(), dbRepo.Key())
	assert.Nil(t, err)

	_, err = pkgRevReadFromDB(context.TODO(), dbPR.Key(), false)
	assert.NotNil(t, err)
	assert.Equal(t, sql.ErrNoRows, err)
}

func TestMultiPackageRevisionRepo(t *testing.T) {
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

	dbRepo11PkgsPRs := createTestPRs(t, dbRepo11Pkgs, "my-ws", 4)
	dbRepo12PkgsPRs := createTestPRs(t, dbRepo12Pkgs, "my-ws", 4)
	dbRepo21PkgsPRs := createTestPRs(t, dbRepo21Pkgs, "my-ws", 4)
	dbRepo22PkgsPRs := createTestPRs(t, dbRepo22Pkgs, "my-ws", 4)

	readRepo11PkgsPRs := readRepoPkgPRs(t, dbRepo11Pkgs)
	readRepo12PkgsPRs := readRepoPkgPRs(t, dbRepo12Pkgs)
	readRepo21PkgsPRs := readRepoPkgPRs(t, dbRepo21Pkgs)
	readRepo22PkgsPRs := readRepoPkgPRs(t, dbRepo22Pkgs)

	assertPackageRevListsEqual(t, dbRepo11PkgsPRs, readRepo11PkgsPRs, 16)
	assertPackageRevListsEqual(t, dbRepo12PkgsPRs, readRepo12PkgsPRs, 16)
	assertPackageRevListsEqual(t, dbRepo21PkgsPRs, readRepo21PkgsPRs, 16)
	assertPackageRevListsEqual(t, dbRepo22PkgsPRs, readRepo22PkgsPRs, 16)

	assertPackageRevLatestIs(t, 4, readRepo11PkgsPRs)
	assertPackageRevLatestIs(t, 4, readRepo12PkgsPRs)
	assertPackageRevLatestIs(t, 4, readRepo21PkgsPRs)
	assertPackageRevLatestIs(t, 4, readRepo22PkgsPRs)

	deleteTestRepo(t, dbRepo11.Key())
	deleteTestRepo(t, dbRepo12.Key())
	deleteTestRepo(t, dbRepo21.Key())
	deleteTestRepo(t, dbRepo22.Key())

	assert.Equal(t, 0, len(readRepoPkgPRs(t, dbRepo11Pkgs)))
	assert.Equal(t, 0, len(readRepoPkgPRs(t, dbRepo12Pkgs)))
	assert.Equal(t, 0, len(readRepoPkgPRs(t, dbRepo21Pkgs)))
	assert.Equal(t, 0, len(readRepoPkgPRs(t, dbRepo22Pkgs)))
}

func TestPackageRevisionFilter(t *testing.T) {
	mockCache := mockcachetypes.NewMockCache(t)
	cachetypes.CacheInstance = mockCache
	mockCache.EXPECT().GetRepository(mock.Anything).Return(&dbRepository{})

	dbRepo := createTestRepo(t, "my-ns", "my-repo")

	dbRepoPkgs := createTestPkgs(t, dbRepo.Key(), "my-package", 4)

	_ = createTestPRs(t, dbRepoPkgs, "my-ws", 4)

	prFilter := repository.ListPackageRevisionFilter{}
	listPRs, err := pkgRevListPRsFromDB(context.TODO(), prFilter)
	assert.Nil(t, err)
	assert.Equal(t, 16, len(listPRs))

	prFilter.Key.WorkspaceName = "my-ws-2"
	listPRs, err = pkgRevListPRsFromDB(context.TODO(), prFilter)
	assert.Nil(t, err)
	assert.Equal(t, 4, len(listPRs))

	prFilter.Key.Revision = 3
	listPRs, err = pkgRevListPRsFromDB(context.TODO(), prFilter)
	assert.Nil(t, err)
	assert.Equal(t, 4, len(listPRs))

	prFilter.Key.Revision = 1
	listPRs, err = pkgRevListPRsFromDB(context.TODO(), prFilter)
	assert.Nil(t, err)
	assert.Equal(t, 0, len(listPRs))

	deleteTestRepo(t, dbRepo.Key())

	assert.Equal(t, 0, len(readRepoPkgPRs(t, dbRepoPkgs)))
}

func TestMultiPackageRevisionList(t *testing.T) {
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

	dbRepo11PkgsPRs := createTestPRs(t, dbRepo11Pkgs, "my-ws", 4)
	dbRepo12PkgsPRs := createTestPRs(t, dbRepo12Pkgs, "my-ws", 4)
	dbRepo21PkgsPRs := createTestPRs(t, dbRepo21Pkgs, "my-ws", 4)
	dbRepo22PkgsPRs := createTestPRs(t, dbRepo22Pkgs, "my-ws", 4)

	listRepo11PkgsPRs := listRepoPkgPRs(t, dbRepo11Pkgs)
	listRepo12PkgsPRs := listRepoPkgPRs(t, dbRepo12Pkgs)
	listRepo21PkgsPRs := listRepoPkgPRs(t, dbRepo21Pkgs)
	listRepo22PkgsPRs := listRepoPkgPRs(t, dbRepo22Pkgs)

	assertPackageRevListsEqual(t, dbRepo11PkgsPRs, listRepo11PkgsPRs, 16)
	assertPackageRevListsEqual(t, dbRepo12PkgsPRs, listRepo12PkgsPRs, 16)
	assertPackageRevListsEqual(t, dbRepo21PkgsPRs, listRepo21PkgsPRs, 16)
	assertPackageRevListsEqual(t, dbRepo22PkgsPRs, listRepo22PkgsPRs, 16)

	assertPackageRevLatestIs(t, 4, listRepo11PkgsPRs)
	assertPackageRevLatestIs(t, 4, listRepo12PkgsPRs)
	assertPackageRevLatestIs(t, 4, listRepo21PkgsPRs)
	assertPackageRevLatestIs(t, 4, listRepo22PkgsPRs)

	deleteTestRepo(t, dbRepo11.Key())
	deleteTestRepo(t, dbRepo12.Key())
	deleteTestRepo(t, dbRepo21.Key())
	deleteTestRepo(t, dbRepo22.Key())

	assert.Equal(t, 0, len(readRepoPkgPRs(t, dbRepo11Pkgs)))
	assert.Equal(t, 0, len(readRepoPkgPRs(t, dbRepo12Pkgs)))
	assert.Equal(t, 0, len(readRepoPkgPRs(t, dbRepo21Pkgs)))
	assert.Equal(t, 0, len(readRepoPkgPRs(t, dbRepo22Pkgs)))
}

func pkgRevDBWriteReadTest(t *testing.T, dbRepo *dbRepository, dbPkg dbPackage, dbPR, dbPRUpdate dbPackageRevision) {
	err := pkgRevWriteToDB(context.TODO(), &dbPR)
	assert.NotNil(t, err)
	assert.True(t, strings.Contains(err.Error(), "violates foreign key constraint"))

	err = repoWriteToDB(context.TODO(), dbRepo)
	assert.Nil(t, err)

	dbPkg.repo = dbRepo
	dbPkg.pkgKey.RepoKey = dbRepo.Key()

	err = pkgWriteToDB(context.TODO(), &dbPkg)
	assert.Nil(t, err)

	dbPR.repo = dbRepo
	dbPR.pkgRevKey.PkgKey = dbPkg.Key()

	err = pkgRevWriteToDB(context.TODO(), &dbPR)
	assert.Nil(t, err)

	readPR, err := pkgRevReadFromDB(context.TODO(), dbPR.Key(), false)
	assert.Nil(t, err)

	resources, err := pkgRevResourcesReadFromDB(context.TODO(), readPR.Key())
	assert.Nil(t, err)

	readPR.resources = resources

	assertPackageRevsEqual(t, &dbPR, readPR)

	err = pkgRevWriteToDB(context.TODO(), &dbPRUpdate)
	assert.NotNil(t, err)
	assert.True(t, strings.Contains(err.Error(), "violates unique constraint"))

	err = pkgRevUpdateDB(context.TODO(), &dbPRUpdate, true)
	assert.Nil(t, err)

	readPR, err = pkgRevReadFromDB(context.TODO(), dbPR.Key(), false)
	assert.Nil(t, err)

	resources, err = pkgRevResourcesReadFromDB(context.TODO(), readPR.Key())
	assert.Nil(t, err)

	readPR.resources = resources

	assertPackageRevsEqual(t, &dbPRUpdate, readPR)

	err = pkgRevDeleteFromDB(context.TODO(), dbPR.Key())
	assert.Nil(t, err)

	_, err = pkgRevReadFromDB(context.TODO(), dbPR.Key(), false)
	assert.NotNil(t, err)
	assert.Equal(t, sql.ErrNoRows, err)

	err = pkgRevUpdateDB(context.TODO(), &dbPRUpdate, false)
	assert.NotNil(t, err)
	assert.True(t, strings.Contains(err.Error(), "no rows or multiple rows found for updating"))

	err = pkgRevDeleteFromDB(context.TODO(), dbPR.Key())
	assert.Nil(t, err)

	err = pkgDeleteFromDB(context.TODO(), dbPkg.Key())
	assert.Nil(t, err)

	err = repoDeleteFromDB(context.TODO(), dbRepo.Key())
	assert.Nil(t, err)
}

func readRepoPkgPRs(t *testing.T, pkgs []dbPackage) []*dbPackageRevision {
	var readRepoPkgsPRs []*dbPackageRevision

	for _, pkg := range pkgs {
		readPRs, err := pkgRevReadPRsFromDB(context.TODO(), pkg.Key())
		assert.Nil(t, err)

		readRepoPkgsPRs = append(readRepoPkgsPRs, readPRs...)
	}

	return readRepoPkgsPRs
}

func listRepoPkgPRs(t *testing.T, pkgs []dbPackage) []*dbPackageRevision {
	var listRepoPkgsPRs []*dbPackageRevision

	for _, pkg := range pkgs {
		prFilter := repository.ListPackageRevisionFilter{
			Key: repository.PackageRevisionKey{
				PkgKey: pkg.Key(),
			},
		}

		listPRs, err := pkgRevListPRsFromDB(context.TODO(), prFilter)
		assert.Nil(t, err)

		listRepoPkgsPRs = append(listRepoPkgsPRs, listPRs...)
	}

	return listRepoPkgsPRs
}

func assertPackageRevListsEqual(t *testing.T, left []dbPackageRevision, right []*dbPackageRevision, count int) {
	assert.Equal(t, count, len(left))
	assert.Equal(t, count, len(right))

	leftMap := make(map[repository.PackageRevisionKey]*dbPackageRevision)
	for _, leftPr := range left {
		resources, err := pkgRevResourcesReadFromDB(context.TODO(), leftPr.Key())
		assert.Nil(t, err)

		leftPr.resources = resources
		leftMap[leftPr.Key()] = &leftPr
	}

	rightMap := make(map[repository.PackageRevisionKey]*dbPackageRevision)
	for _, rightPr := range right {
		rightMap[rightPr.Key()] = rightPr

		resources, err := pkgRevResourcesReadFromDB(context.TODO(), rightPr.Key())
		assert.Nil(t, err)

		rightPr.resources = resources
	}

	for leftKey, leftPr := range leftMap {
		rightPr, ok := rightMap[leftKey]
		assert.True(t, ok)

		assertPackageRevsEqual(t, leftPr, rightPr)
	}
}

func assertPackageRevsEqual(t *testing.T, left, right *dbPackageRevision) {
	assert.Equal(t, left.Key(), right.Key())
	assert.Equal(t, left.meta.Namespace, right.meta.Namespace)
	assert.Equal(t, left.meta.Name, right.meta.Name)
	assert.Equal(t, left.spec, right.spec)
	assert.Equal(t, left.updatedBy, right.updatedBy)
	assert.Equal(t, left.lifecycle, right.lifecycle)
	assert.Equal(t, left.tasks, right.tasks)
	assert.Equal(t, left.resources, right.resources)
}

func assertPackageRevLatestIs(t *testing.T, expectedLatest int, prList []*dbPackageRevision) {
	for _, pr := range prList {
		latestPrRev, err := pkgRevGetlatestRevFromDB(context.TODO(), pr.Key().PkgKey)
		assert.True(t, err == nil)
		assert.Equal(t, expectedLatest, latestPrRev)
	}
}
