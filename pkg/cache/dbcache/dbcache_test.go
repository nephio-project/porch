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
	"os/user"
	"testing"
	"time"

	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
	configapi "github.com/nephio-project/porch/api/porchconfig/v1alpha1"
	cachetypes "github.com/nephio-project/porch/pkg/cache/types"
	"github.com/nephio-project/porch/pkg/externalrepo"
	"github.com/nephio-project/porch/pkg/repository"
	mockdbcache "github.com/nephio-project/porch/test/mockery/mocks/porch/pkg/cache/dbcache"
	"github.com/stretchr/testify/suite"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/klog/v2"
)

const defaultPorchSQLSchema = "api/sql/porch-db.sql"

type DbTestSuite struct {
	suite.Suite

	ctx            context.Context
	nextPkgRev     int
	savedDBHandler *DBHandler
}

func Test_DbTestSuite(t *testing.T) {
	if u, err := user.Current(); err == nil && u.Username == "root" {
		t.Fatalf("This test cannot run as %q user", u.Username)
	}
	// TODO: replace ctx with t.Context() in go 1.24<
	suite.Run(t, &DbTestSuite{nextPkgRev: 1, ctx: context.Background()})
}

func (t *DbTestSuite) Context() context.Context {
	t.T().Helper()
	return t.ctx
}

func (t *DbTestSuite) SetupSuite() {
	postgres := embeddedpostgres.NewDatabase(embeddedpostgres.DefaultConfig().
		Username("porch").
		Password("porch").
		Database("porch").
		Port(55432))

	err := postgres.Start()
	t.Require().NoError(err, "could not start test instance of postgres")

	t.T().Cleanup(func() {
		if err := postgres.Stop(); err != nil {
			klog.Errorf("stop of test database failed: %q", err)
		}
	})

	dbOpts := &cachetypes.CacheOptions{
		DBCacheOptions: cachetypes.DBCacheOptions{
			Driver:     "pgx",
			DataSource: "postgresql://porch:porch@localhost:55432/porch",
		},
	}

	err = OpenDB(t.Context(), *dbOpts)
	t.Require().NoError(err, "could not connect to test database")

	t.T().Cleanup(func() {
		// TODO: is this sleep necessary?
		time.Sleep(5 * time.Second)

		if err := CloseDB(t.Context()); err != nil {
			t.T().Log(err)
		}
	})

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
	t.Require().NoErrorf(err, "could not read Porch SQL schema file %q", schemaFile)

	_, err = GetDB().db.Exec(string(schemaBytes))
	t.Require().NoErrorf(err, "could not process Porch SQL schema file %q", schemaFile)
}

type mockNotifier struct {
	calls []struct {
		eventType watch.EventType
		obj       repository.PackageRevision
	}
	returnVal int
}

func (n *mockNotifier) NotifyPackageRevisionChange(eventType watch.EventType, obj repository.PackageRevision) int {
	n.calls = append(n.calls, struct {
		eventType watch.EventType
		obj       repository.PackageRevision
	}{eventType: eventType, obj: obj})
	return n.returnVal
}

func (t *DbTestSuite) switchToMockSQL() {
	mockDBCache := mockdbcache.NewMockdbSQLInterface(t.T())

	t.savedDBHandler = GetDB()
	dbHandler = nil

	err := CloseDB(t.Context())
	t.NoError(err)

	dbHandler = &DBHandler{
		dBCacheOptions: t.savedDBHandler.dBCacheOptions,
		dataSource:     t.savedDBHandler.dataSource,
		db:             mockDBCache,
	}
	t.NotNil(dbHandler)
}

func (t *DbTestSuite) revertToPostgreSQL() {
	dbHandler = t.savedDBHandler
}

func (t *DbTestSuite) TestDBRepositoryCrud() {
	externalrepo.ExternalRepoInUnitTestMode = true

	ctx := t.Context()

	options := cachetypes.CacheOptions{
		RepoSyncFrequency: 60 * time.Minute,
	}
	dbCache, err := new(DBCacheFactory).NewCache(ctx, options)
	t.NoError(err)
	t.Empty(dbCache.GetRepositories())

	repositorySpec := configapi.Repository{
		ObjectMeta: v1.ObjectMeta{
			Namespace: "my-ns",
			Name:      "my-repo",
		},
	}
	testRepo, err := dbCache.OpenRepository(ctx, &repositorySpec)
	t.NoError(err)
	t.Equal("my-repo", testRepo.Key().Name)

	gotRepo := dbCache.GetRepository(testRepo.Key())
	t.Equal(testRepo.Key(), gotRepo.Key())

	repositorySpec.Spec.Description = "My lovely Repo"

	err = dbCache.UpdateRepository(ctx, &repositorySpec)
	t.NoError(err)

	err = dbCache.CloseRepository(ctx, &repositorySpec, nil)
	t.NoError(err)
}

func (t *DbTestSuite) createTestRepo(namespace, name string) *dbRepository {
	dbRepo := dbRepository{
		repoKey: repository.RepositoryKey{
			Namespace: namespace,
			Name:      name,
		},
		repoPRChangeNotifier: &mockNotifier{returnVal: 1},
		spec: &configapi.Repository{
			Spec: configapi.RepositorySpec{
				Git: &configapi.GitRepository{
					Repo: "http://www.gitrepo.org/my-repo",
				},
			},
		},
	}
	err := repoWriteToDB(t.Context(), &dbRepo)
	t.NoError(err)

	return &dbRepo
}

func (t *DbTestSuite) deleteTestRepo(key repository.RepositoryKey) {
	err := repoDeleteFromDB(t.Context(), key)
	t.NoError(err)
}

func (t *DbTestSuite) createTestPkg(repoKey repository.RepositoryKey, name string) dbPackage {
	dbPkg := dbPackage{
		repo: cachetypes.CacheInstance.GetRepository(repoKey).(*dbRepository),
		pkgKey: repository.PackageKey{
			RepoKey: repoKey,
			Package: name,
		},
	}

	err := pkgWriteToDB(t.Context(), &dbPkg)
	t.NoError(err)

	return dbPkg
}

func (t *DbTestSuite) createTestPkgs(repoKey repository.RepositoryKey, namePrefix string, count int) []dbPackage {
	var testPkgs []dbPackage

	for i := range count {
		testPkgs = append(testPkgs, t.createTestPkg(repoKey, fmt.Sprintf("%s-%d", namePrefix, i)))
	}

	return testPkgs
}

func (t *DbTestSuite) createTestPR(pkgKey repository.PackageKey, name string) dbPackageRevision {
	dbPkgRev := dbPackageRevision{
		pkgRevKey: repository.PackageRevisionKey{
			PkgKey:        pkgKey,
			WorkspaceName: name,
			Revision:      t.nextPkgRev,
		},
		lifecycle: "Published",
		resources: map[string]string{"Hello.txt": "Hello", "Goodbye.txt": "Goodbye"},
	}

	err := pkgRevWriteToDB(t.Context(), &dbPkgRev)
	t.NoError(err)

	return dbPkgRev
}

func (t *DbTestSuite) createTestPRs(packages []dbPackage, wsNamePrefix string, count int) []dbPackageRevision {
	var testPRs []dbPackageRevision

	for _, pkg := range packages {
		t.nextPkgRev = 1
		for prNo := range count {
			testPRs = append(testPRs, t.createTestPR(pkg.Key(), fmt.Sprintf("%s-%d", wsNamePrefix, prNo)))
			t.nextPkgRev++
		}
	}
	return testPRs
}
