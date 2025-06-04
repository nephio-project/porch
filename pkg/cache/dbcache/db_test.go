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
	"k8s.io/klog/v2"
)

func TestMain(m *testing.M) {
	code, err := run(m)
	if err != nil {
		klog.Errorf("tests failed: %s", err.Error())
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
			klog.Errorf("stop of test database failed: %s", err.Error())
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
		return -1, fmt.Errorf("environment variable PORCH_SQL_SCHEMA not set")
	}

	schemaBytes, err := os.ReadFile(schemaFile)
	if err != nil {
		return -1, fmt.Errorf("could not read Porch SQL schema file %s: %w", schemaFile, err)
	}

	_, err = GetDB().db.Exec(string(schemaBytes))
	if err != nil {
		return -1, fmt.Errorf("could not process Porch SQL schema file %s: %w", schemaFile, err)
	}

	// truncates all test data after the tests are run
	defer func() {
		if err := CloseDB(context.TODO()); err != nil {
			klog.Errorf("close of test database failed: %s", err.Error())
		}
	}()

	return m.Run(), nil
}
