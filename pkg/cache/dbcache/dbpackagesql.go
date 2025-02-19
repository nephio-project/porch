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
	"database/sql"

	"github.com/nephio-project/porch/pkg/repository"
	"k8s.io/klog/v2"
)

func pkgReadFromDB(pk repository.PackageKey) (dbPackage, error) {
	sqlStatement := `SELECT * FROM packages WHERE name_space=$1 AND repo_name=$2 AND package_name=$3`

	var dbPkg dbPackage
	var metaAsJson, specAsJson string

	klog.Infof("pkgReadFromDB: running query [%q] on %q", sqlStatement, pk)
	err := GetDB().db.QueryRow(sqlStatement, pk.Namespace, pk.Repository, pk.Package).Scan(
		&dbPkg.pkgKey.Namespace,
		&dbPkg.pkgKey.Repository,
		&dbPkg.pkgKey.Package,
		&metaAsJson,
		&specAsJson,
		&dbPkg.updated,
		&dbPkg.updatedBy)

	if err != nil {
		if err == sql.ErrNoRows {
			klog.Infof("pkgReadFromDB: package not found in db %q", pk)
		} else {
			klog.Infof("pkgReadFromDB: reading package %q returned err: %q", pk, err)
		}
		return dbPkg, err
	}

	setValueFromJson(metaAsJson, dbPkg.meta)
	setValueFromJson(specAsJson, dbPkg.spec)

	return dbPkg, err
}

func pkgReadPkgsFromDB(rk repository.RepositoryKey) ([]dbPackage, error) {
	sqlStatement := `SELECT * FROM packages WHERE name_space=$1 AND repo_name=$2`

	var dbPkgs []dbPackage

	rows, err := GetDB().db.Query(
		sqlStatement, rk.Namespace, rk.Repository)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	klog.Infof("pkgReadPkgsFromDB: query succeeded for %q", rk)

	for rows.Next() {
		var pkg dbPackage
		var metaAsJson, specAsJson string

		if err := rows.Scan(
			&pkg.pkgKey.Namespace,
			&pkg.pkgKey.Repository,
			&pkg.pkgKey.Package,
			&metaAsJson,
			&specAsJson,
			&pkg.updated,
			&pkg.updatedBy); err != nil {
			return nil, err
		}

		setValueFromJson(metaAsJson, pkg.meta)
		setValueFromJson(specAsJson, pkg.spec)

		dbPkgs = append(dbPkgs, pkg)
	}

	return dbPkgs, nil
}

func pkgWriteToDB(p *dbPackage) error {
	sqlStatement := `
        INSERT INTO packages (name_space, repo_name, package_name, meta, spec, updated, updatedby)
        VALUES ($1, $2, $3, $4, $5, $6, $7)`

	klog.Infof("pkgWriteToDB: running query [%q] on %q", sqlStatement, p.Key())

	pk := p.Key()
	if _, err := GetDB().db.Exec(
		sqlStatement, pk.Namespace, pk.Repository, pk.Package, valueAsJson(p.meta), valueAsJson(p.spec), p.updated, p.updatedBy); err == nil {
		klog.Infof("pkgWriteToDB: query succeeded for %q", p.Key())
		return nil
	} else {
		klog.Infof("pkgWriteToDB: query failed for %q: %q", p.Key(), err)
		return err
	}
}

func pkgUpdateDB(p *dbPackage) error {
	sqlStatement := `
        UPDATE packages SET meta=$4, spec=$5, updated=$6, updatedby=$7
        WHERE name_space=$1 AND repo_name=$2 AND package_name=$3`

	klog.Infof("pkgUpdateDB: running query [%q] on %q)", sqlStatement, p.Key())

	pk := p.Key()
	if _, err := GetDB().db.Exec(
		sqlStatement,
		pk.Namespace, pk.Repository, pk.Package, valueAsJson(p.meta), valueAsJson(p.spec), p.updated, p.updatedBy); err == nil {
		klog.Infof("pkgUpdateDB: query succeeded for %q", pk)
		return nil
	} else {
		klog.Infof("pkgUpdateDB: query failed for %q: %q", pk, err)
		return err
	}
}

func pkgDeleteFromDB(pk repository.PackageKey) error {
	sqlStatement := `DELETE FROM packages WHERE name_space=$1 AND repo_name=$2 AND package_name=$3`

	klog.Infof("DB Connection: running query [%q] on %q", sqlStatement, pk)
	if _, err := GetDB().db.Exec(sqlStatement, pk.Namespace, pk.Repository, pk.Package); err == nil {
		klog.Infof("pkgDeleteFromDB: query succeeded for %q", pk)
		return nil
	} else {
		klog.Infof("pkgDeleteFromDB: query failed for %q: %q", pk, err)
		return err
	}
}
