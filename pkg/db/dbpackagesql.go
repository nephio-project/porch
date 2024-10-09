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
	"github.com/nephio-project/porch/pkg/repository"
	"k8s.io/klog/v2"
)

func pkgReadFromDB(pk repository.PackageKey) (dbPackage, error) {
	sqlStatement := `SELECT * FROM packages WHERE namespace=$1 AND repo_name=$2 AND package_name=$3`

	var dbPkg dbPackage

	klog.Infof("pkgReadFromDB: running query [%q] on %q", sqlStatement, pk)
	err := GetDBConnection().db.QueryRow(sqlStatement, pk.Namespace, pk.Repository, pk.Package).Scan(
		&dbPkg.pkgKey.Namespace,
		&dbPkg.pkgKey.Repository,
		&dbPkg.pkgKey.Package,
		&dbPkg.updated,
		&dbPkg.updatedBy)

	klog.Infof("pkgReadFromDB: query result on %q: err=%q", pk, err)

	return dbPkg, err
}

func pkgReadPkgsFromDB(rk repository.RepositoryKey) ([]dbPackage, error) {
	sqlStatement := `SELECT * FROM packages WHERE namespace=$1 AND repo_name=$2`

	var dbPkgs []dbPackage

	rows, err := GetDBConnection().db.Query(
		sqlStatement, rk.Namespace, rk.Repository)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	for rows.Next() {
		var pkg dbPackage

		rows.Scan(
			&pkg.pkgKey.Namespace,
			&pkg.pkgKey.Repository,
			&pkg.pkgKey.Package,
			&pkg.updated,
			&pkg.updatedBy)

		dbPkgs = append(dbPkgs, pkg)
	}

	return dbPkgs, nil
}

func pkgWriteToDB(p dbPackage) error {
	sqlStatement := `
        INSERT INTO packages (namespace, repo_name, package_name, updated, updatedby)
        VALUES ($1, $2, $3, $4, $5)`

	klog.Infof("pkgWriteToDB: running query [%q] on %q", sqlStatement, p.Key())

	pk := p.Key()
	if returnedVal := GetDBConnection().db.QueryRow(
		sqlStatement, pk.Namespace, pk.Repository, pk.Package, p.updated, p.updatedBy); returnedVal.Err() == nil {
		klog.Infof("pkgWriteToDB: query succeeded for %q", p.Key())
		return nil
	} else {
		klog.Infof("pkgWriteToDB: query failed for %q: %q", p.Key(), returnedVal.Err())
		return returnedVal.Err()
	}
}

func pkgUpdateDB(p dbPackage) error {
	sqlStatement := `
        UPDATE packages SET updated=$4, updatedby=$5
        WHERE namespace=$1 AND repo_name=$2 AND package_name=$3`

	klog.Infof("pkgUpdateDB: running query [%q] on %q)", sqlStatement, p.Key())

	pk := p.Key()
	if returnedVal := GetDBConnection().db.QueryRow(
		sqlStatement,
		pk.Namespace, pk.Repository, pk.Package, p.updated, p.updatedBy); returnedVal.Err() == nil {
		klog.Infof("pkgUpdateDB: query succeeded for %q", pk)
		return nil
	} else {
		klog.Infof("pkgUpdateDB: query failed for %q: %q", pk, returnedVal.Err())
		return returnedVal.Err()
	}
}

func pkgDeleteFromDB(pk repository.PackageKey) error {
	sqlStatement := `DELETE FROM packages WHERE namespace=$1 AND repo_name=$2 AND package_name=$3`

	klog.Infof("DB Connection: running query [%q] on %q", sqlStatement, pk)
	if returnedVal := GetDBConnection().db.QueryRow(sqlStatement, pk.Namespace, pk.Repository, pk.Package); returnedVal.Err() == nil {
		klog.Infof("pkgDeleteFromDB: query succeeded for %q", pk)
		return nil
	} else {
		klog.Infof("pkgDeleteFromDB: query failed for %q: %q", pk, returnedVal.Err())
		return returnedVal.Err()
	}
}
