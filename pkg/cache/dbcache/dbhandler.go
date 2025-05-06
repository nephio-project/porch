// Copyright 2024 The Nephio Authors
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

	"go.opentelemetry.io/otel/trace"
	"k8s.io/klog/v2"

	cachetypes "github.com/nephio-project/porch/pkg/cache/types"
)

type DBHandler struct {
	dBCacheOptions cachetypes.DBCacheOptions
	dataSource     string
	db             *sql.DB
}

var dbHandler *DBHandler = nil

func OpenDB(ctx context.Context, opts cachetypes.CacheOptions) error {
	_, span := tracer.Start(ctx, "DBHandler::OpenDB", trace.WithAttributes())
	defer span.End()

	klog.Infof("OpenDB: %s %s", opts.DBCacheOptions.Driver, opts.DBCacheOptions.DataSource)

	if dbHandler != nil {
		klog.Infof("OpenDB: database %s, already open", opts.DBCacheOptions.DataSource)
		return nil
	}

	db, err := sql.Open(opts.DBCacheOptions.Driver, opts.DBCacheOptions.DataSource)
	if err != nil {
		klog.Infof("OpenDB: database %s open failed: err=%s", opts.DBCacheOptions.DataSource, err)
		return err
	}

	if err := db.Ping(); err == nil {
		klog.Infof("OpenDB: database %s opened", opts.DBCacheOptions.DataSource)
	} else {
		db.Close()
		klog.Infof("OpenDB: database %s open failed", opts.DBCacheOptions.DataSource)
		return err
	}

	dbHandler = &DBHandler{
		dBCacheOptions: opts.DBCacheOptions,
		db:             db,
	}

	return nil
}

func GetDB() *DBHandler {
	if dbHandler == nil {
		klog.Errorf("GetDB: the database is not open")
		return nil
	}

	return dbHandler
}

func CloseDB(ctx context.Context) error {
	_, span := tracer.Start(ctx, "DBHandler::CloseDB", trace.WithAttributes())
	defer span.End()

	if dbHandler == nil {
		klog.Infof("CloseDB: the databse is already closed")
		return nil
	}

	var err error
	if err = dbHandler.db.Close(); err == nil {
		klog.Infof("CloseDB: database %s closed", dbHandler.dataSource)
	} else {
		klog.Infof("CloseDB: close failed to database %s: %q", dbHandler.dataSource, err)
	}

	dbHandler = nil
	return err
}
