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

package db

import (
	"database/sql"

	"k8s.io/klog/v2"

	_ "github.com/jackc/pgx/v5/stdlib"
	configapi "github.com/nephio-project/porch/api/porchconfig/v1alpha1"
)

// TODO: Add connection pooling

const MAX_MODIFICATION_DURATION = 10

type DBConnection struct {
	spec                    *configapi.DBRepository
	db                      *sql.DB
	encoder                 encoder
	maxModificationDuration int64
}

var dbConnection *DBConnection = nil

func OpenDBConnection(dbSpec *configapi.DBRepository) error {
	klog.Infof("DBConnection: %q", dbSpec)

	if dbConnection != nil {
		klog.Infof("DB Connection: connection to database %q, already open", dbSpec)
		return nil
	}

	db, err := sql.Open(dbSpec.Driver, dbSpec.DataSource)
	if err != nil {
		klog.Infof("DB Connection: connection to database %q failed: err=%q", dbSpec, err)
		return err
	}

	klog.Infof("DB Connection: connected to database %s", dbSpec)

	dbConnection = &DBConnection{
		spec: dbSpec,
		db:   db,
		encoder: encoder{
			encoding: dbSpec.PackageResourceEncoding,
		},
		maxModificationDuration: int64(MAX_MODIFICATION_DURATION * 1_000_000.0),
	}

	return nil
}

func GetDBConnection() *DBConnection {
	if dbConnection == nil {
		klog.Errorf("DB Connection: the database connection is not open")
		return nil
	}

	return dbConnection
}

func CloseDBConnection() error {
	if dbConnection == nil {
		klog.Infof("DB Connection: connection to database already closed")
		return nil
	}

	var err error
	if err = dbConnection.db.Close(); err == nil {
		klog.Infof("DB Connection: connection to database %s closed", dbConnection.spec)
	} else {
		klog.Infof("DB Connection: close failed on connection to database %s: %q", dbConnection.spec, err)
	}

	dbConnection = nil
	return err
}
