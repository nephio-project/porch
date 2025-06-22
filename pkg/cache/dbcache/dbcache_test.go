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

package dbcache

import (
	"context"
	"fmt"
	"os"
	"testing"

	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
	cachetypes "github.com/nephio-project/porch/pkg/cache/types"
	"github.com/nephio-project/porch/pkg/repository"
	"github.com/stretchr/testify/assert"
	"k8s.io/klog/v2"
)

var (
	defaultPorchSQLSchema string = "api/sql/porch-db.sql"
	nextPkgRev            int    = 1
)

func TestMain(m *testing.M) {
	code, err := run(m)
	if err != nil {
		klog.Errorf("tests failed: %q", err)
	}
	os.Exit(code)
}

func run(m *testing.M) (code int, err error) {
	postgres := embeddedpostgres.NewDatabase(embeddedpostgres.DefaultConfig().
		Username("porch").
		Password("porch").
		Database("porch").
		Port(55432))

	if err := postgres.Start(); err != nil {
		return -1, fmt.Errorf("could not start test instance of postgres: %w", err)
	}

	defer func() {
		if err := postgres.Stop(); err != nil {
			klog.Errorf("stop of test database failed: %q", err)
		}
	}()

	dbOpts := &cachetypes.CacheOptions{
		DBCacheOptions: cachetypes.DBCacheOptions{
			Driver:     "pgx",
			DataSource: "postgresql://porch:porch@localhost:55432/porch",
		},
	}

	if err := OpenDB(context.TODO(), *dbOpts); err != nil {
		return -1, fmt.Errorf("could not connect to test database: %w", err)
	}

	schemaFile, ok := os.LookupEnv("PORCH_SQL_SCHEMA")
	if !ok {
		klog.Infof("environment variable PORCH_SQL_SCHEMA not set, trying to find default schema in PORCHDIR/%q", defaultPorchSQLSchema)

		gitRoot, ok := os.LookupEnv("PORCHDIR")
		if ok {
			schemaFile = gitRoot + "/" + defaultPorchSQLSchema
		} else {
			klog.Infof("environment variable PORCHDIR not set, setting schema file as %q", defaultPorchSQLSchema)
			schemaFile = defaultPorchSQLSchema
		}
	}

	schemaBytes, err := os.ReadFile(schemaFile)
	if err != nil {
		return -1, fmt.Errorf("could not read Porch SQL schema file %q: %w", schemaFile, err)
	}

	_, err = GetDB().db.Exec(string(schemaBytes))
	if err != nil {
		return -1, fmt.Errorf("could not process Porch SQL schema file %q: %w", schemaFile, err)
	}

	// truncates all test data after the tests are run
	defer func() {
		if err := CloseDB(context.TODO()); err != nil {
			klog.Errorf("close of test database failed: %q", err)
		}
	}()

	return m.Run(), nil
}

func createTestRepo(t *testing.T, namespace, name string) dbRepository {
	dbRepo := dbRepository{
		repoKey: repository.RepositoryKey{
			Namespace: namespace,
			Name:      name,
		},
	}
	err := repoWriteToDB(context.TODO(), &dbRepo)
	assert.Nil(t, err)

	return dbRepo
}

func deleteTestRepo(t *testing.T, key repository.RepositoryKey) {
	err := repoDeleteFromDB(context.TODO(), key)
	assert.Nil(t, err)
}

func createTestPkg(t *testing.T, repoKey repository.RepositoryKey, name string) dbPackage {
	dbPkg := dbPackage{
		repo: cachetypes.CacheInstance.GetRepository(repoKey).(*dbRepository),
		pkgKey: repository.PackageKey{
			RepoKey: repoKey,
			Package: name,
		},
	}

	err := pkgWriteToDB(context.TODO(), &dbPkg)
	assert.Nil(t, err)

	return dbPkg
}

func createTestPkgs(t *testing.T, repoKey repository.RepositoryKey, namePrefix string, count int) []dbPackage {
	var testPkgs []dbPackage

	for i := range count {
		testPkgs = append(testPkgs, createTestPkg(t, repoKey, fmt.Sprintf("%s-%d", namePrefix, i)))
	}

	return testPkgs
}

func createTestPR(t *testing.T, pkgKey repository.PackageKey, name string) dbPackageRevision {
	dbPkgRev := dbPackageRevision{
		pkgRevKey: repository.PackageRevisionKey{
			PkgKey:        pkgKey,
			WorkspaceName: name,
			Revision:      nextPkgRev,
		},
		lifecycle: "Published",
		resources: map[string]string{"Hello.txt": "Hello", "Goodbye.txt": "Goodbye"},
	}

	err := pkgRevWriteToDB(context.TODO(), &dbPkgRev)
	assert.Nil(t, err)

	return dbPkgRev
}

func createTestPRs(t *testing.T, packages []dbPackage, wsNamePrefix string, count int) []dbPackageRevision {
	var testPRs []dbPackageRevision

	for _, pkg := range packages {
		nextPkgRev = 1
		for prNo := range count {
			testPRs = append(testPRs, createTestPR(t, pkg.Key(), fmt.Sprintf("%s-%d", wsNamePrefix, prNo)))
			nextPkgRev++
		}
	}
	return testPRs
}
