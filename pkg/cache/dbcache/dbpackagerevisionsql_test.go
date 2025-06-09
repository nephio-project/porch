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
	"strings"
	"testing"
	"time"

	"github.com/nephio-project/porch/api/porch/v1alpha1"
	"github.com/nephio-project/porch/pkg/repository"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestPackageRevisionDBWriteRead(t *testing.T) {
	dbRepo := dbRepository{
		repoKey: repository.RepositoryKey{
			Namespace:         "my-ns",
			Name:              "my-repo",
			Path:              "my-path",
			PlaceholderWSname: "my-placeholder-ws",
		},
		meta:       nil,
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
		meta:      nil,
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
		meta:      nil,
		spec:      nil,
		updated:   time.Now().UTC(),
		updatedBy: "porchuser",
		lifecycle: "Draft",
		resources: map[string]string{},
	}

	dbPRUpdate := dbPackageRevision{
		repo:      &dbRepo,
		pkgRevKey: dbPR.Key(),
		meta: &metav1.ObjectMeta{
			Name:      "meta-new-name",
			Namespace: "meta-new-namespace",
		},
		spec:      &v1alpha1.PackageRevisionSpec{},
		updated:   time.Now().UTC(),
		updatedBy: "porchuser2",
		lifecycle: "Published",
		resources: map[string]string{"Hello.txt": "Hello"},
	}

	pkgRevDBWriteReadTest(t, dbRepo, dbPkg, dbPR, dbPRUpdate)

	dbPkg.pkgKey.Path = ""
	dbPR.pkgRevKey.PkgKey = dbPkg.Key()
	dbPRUpdate.pkgRevKey.PkgKey = dbPkg.Key()
	dbPRUpdate.updatedBy = "bart"
	pkgRevDBWriteReadTest(t, dbRepo, dbPkg, dbPR, dbPRUpdate)
}

func TestPackageRevisionDBSchema(t *testing.T) {
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
	assert.True(t, strings.Contains(err.Error(), "should return 1 package, it returned 0 packages"))
}

func TestMultiPackageRevisionRepo(t *testing.T) {
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

func pkgRevDBWriteReadTest(t *testing.T, dbRepo dbRepository, dbPkg dbPackage, dbPR, dbPRUpdate dbPackageRevision) {
	err := pkgRevWriteToDB(context.TODO(), &dbPR)
	assert.NotNil(t, err)
	assert.True(t, strings.Contains(err.Error(), "violates foreign key constraint"))

	err = repoWriteToDB(context.TODO(), &dbRepo)
	assert.Nil(t, err)

	dbPkg.repo = &dbRepo
	dbPkg.pkgKey.RepoKey = dbRepo.Key()

	err = pkgWriteToDB(context.TODO(), &dbPkg)
	assert.Nil(t, err)

	dbPR.repo = &dbRepo
	dbPR.pkgRevKey.PkgKey = dbPkg.Key()

	err = pkgRevWriteToDB(context.TODO(), &dbPR)
	assert.Nil(t, err)

	readPR, err := pkgRevReadFromDB(context.TODO(), dbPR.Key())
	assert.Nil(t, err)
	assertPackageRevsEqual(t, &dbPR, readPR)

	err = pkgRevWriteToDB(context.TODO(), &dbPRUpdate)
	assert.NotNil(t, err)
	assert.True(t, strings.Contains(err.Error(), "violates unique constraint"))

	err = pkgRevUpdateDB(context.TODO(), &dbPRUpdate)
	assert.Nil(t, err)

	readPR, err = pkgRevReadFromDB(context.TODO(), dbPR.Key())
	assert.Nil(t, err)
	assertPackageRevsEqual(t, &dbPRUpdate, readPR)

	err = pkgRevDeleteFromDB(context.TODO(), dbPR.Key())
	assert.Nil(t, err)

	_, err = pkgRevReadFromDB(context.TODO(), dbPR.Key())
	assert.NotNil(t, err)
	assert.True(t, strings.Contains(err.Error(), "should return 1 package revision, it returned 0 package revisions"))

	err = pkgRevUpdateDB(context.TODO(), &dbPRUpdate)
	assert.NotNil(t, err)
	assert.True(t, strings.Contains(err.Error(), "no rows or multiple rows found for updating"))

	err = pkgRevDeleteFromDB(context.TODO(), dbPR.Key())
	assert.Nil(t, err)

	err = pkgDeleteFromDB(context.TODO(), dbPkg.Key())
	assert.Nil(t, err)

	err = repoDeleteFromDB(context.TODO(), dbRepo.Key())
	assert.Nil(t, err)
}

func assertPackageRevListsEqual(t *testing.T, left []dbPackageRevision, right []*dbPackageRevision, count int) {
	assert.Equal(t, count, len(left))
	assert.Equal(t, count, len(right))

	for i := range count {
		assertPackageRevsEqual(t, &left[i], right[i])
	}
}

func assertPackageRevsEqual(t *testing.T, left, right *dbPackageRevision) {
	assert.Equal(t, left.Key(), right.Key())
	if left.meta != nil || right.meta != nil {
		assert.Equal(t, left.meta.Namespace, right.meta.Namespace)
		assert.Equal(t, left.meta.Name, right.meta.Name)
	}
	assert.Equal(t, left.spec, right.spec)
	assert.Equal(t, left.updated, right.updated)
	assert.Equal(t, left.updatedBy, right.updatedBy)
	assert.Equal(t, left.lifecycle, right.lifecycle)
	assert.Equal(t, left.tasks, right.tasks)
	assert.Equal(t, left.resources, right.resources)
}
