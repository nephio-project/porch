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
)

// TODO: Add connection pooling

const MAX_MODIFICATION_DURATION = 10

type DBConnection struct {
	driver                  string
	dataSource              string
	db                      *sql.DB
	maxModificationDuration int64
}

var dbConnection *DBConnection = nil

func OpenDBConnection(driver string, dataSource string) error {
	klog.Infof("DBConnection: %q %q", driver, dataSource)

	if dbConnection != nil {
		klog.Infof("DB Connection: connection to database %s using driver %s, already open", dataSource, driver)
		return nil
	}

	db, err := sql.Open(driver, dataSource)
	if err != nil {
		klog.Infof("DB Connection: connection to database %s failed using driver %s, error %q", dataSource, driver, err)
		return err
	}

	klog.Infof("DB Connection: connected to database %s using driver %s", dataSource, driver)

	dbConnection = &DBConnection{
		driver:                  driver,
		dataSource:              dataSource,
		db:                      db,
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
		klog.Infof("DB Connection: connection to database %s using driver %s closed", dbConnection.dataSource, dbConnection.driver)
	} else {
		klog.Infof("DB Connection: close failed on connection to database %s using driver %s: %q", dbConnection.dataSource, dbConnection.driver, err)
	}

	dbConnection = nil
	return err
}
