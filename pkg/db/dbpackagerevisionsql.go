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

	"github.com/nephio-project/porch/pkg/repository"
	"k8s.io/klog/v2"
)

func pkgRevReadFromDB(prk repository.PackageRevisionKey) (dbPackageRevision, error) {
	klog.Infof("pkgRevReadFromDB: reading package revision %q", prk)

	sqlStatement := `SELECT * FROM package_revisions
     WHERE namespace=$1 AND repo_name=$2 AND package_name=$3 AND package_rev=$4`

	var dbPkgRev dbPackageRevision
	var rawResources []byte

	err := GetDBConnection().db.QueryRow(
		sqlStatement, prk.Namespace, prk.Repository, prk.Package, prk.Revision).Scan(
		&dbPkgRev.pkgRevKey.Namespace,
		&dbPkgRev.pkgRevKey.Repository,
		&dbPkgRev.pkgRevKey.Package,
		&dbPkgRev.pkgRevKey.Revision,
		&dbPkgRev.pkgRevKey.WorkspaceName,
		&dbPkgRev.updated,
		&dbPkgRev.updatedBy,
		&dbPkgRev.lifecycle,
		&rawResources)

	if err != nil {
		if err == sql.ErrNoRows {
			klog.Infof("pkgRevReadFromDB: package not found in db %q", prk)
		} else {
			klog.Infof("pkgRevReadFromDB: reading package revision %q returned err: %q", prk, err)
		}
		return dbPkgRev, err
	}

	klog.Infof("pkgRevReadFromDB: reading package succeeded %q", prk)

	dbPkgRev.resources, err = GetDBConnection().encoder.DecodePackage(rawResources)
	if err != nil {
		klog.Infof("pkgRevReadFromDB: reading package revision resources %q returned err: %q", prk, err)
	}

	return dbPkgRev, err
}

func pkgRevReadPRsFromDB(pk repository.PackageKey) ([]*dbPackageRevision, error) {
	sqlStatement := `SELECT * FROM package_revisions WHERE namespace=$1 AND repo_name=$2 AND package_name=$3`

	var dbPkgRevs []*dbPackageRevision

	klog.Infof("pkgReadPRsFromDB: running query [%q] on %q", sqlStatement, pk)

	rows, err := GetDBConnection().db.Query(sqlStatement, pk.Namespace, pk.Repository, pk.Package)
	if err != nil {
		klog.Infof("pkgReadPRsFromDB: query failed for %q: %q", pk, err)
		return nil, err
	}

	klog.Infof("pkgReadPRsFromDB: query succeeded for %q", pk)

	defer rows.Close()

	for rows.Next() {
		var pkgRev dbPackageRevision
		var rawResources []byte

		rows.Scan(
			&pkgRev.pkgRevKey.Namespace,
			&pkgRev.pkgRevKey.Repository,
			&pkgRev.pkgRevKey.Package,
			&pkgRev.pkgRevKey.Revision,
			&pkgRev.pkgRevKey.WorkspaceName,
			&pkgRev.updated,
			&pkgRev.updatedBy,
			&pkgRev.lifecycle,
			&rawResources)

		pkgRev.resources, err = GetDBConnection().encoder.DecodePackage(rawResources)
		if err != nil {
			klog.Infof("pkgRevReadFromDB: reading package revision resources %q returned err: %q", pkgRev.Key(), err)
		}

		dbPkgRevs = append(dbPkgRevs, &pkgRev)
	}

	return dbPkgRevs, nil
}

func pkgRevWriteToDB(pr dbPackageRevision) error {
	klog.Infof("pkgRevWriteToDB: writing package revision %q", pr.Key())

	rawResources, err := GetDBConnection().encoder.EncodePackage(pr.resources)
	if err != nil {
		return err
	}

	sqlStatement := `
        INSERT INTO package_revisions (namespace, repo_name, package_name, package_rev, workspace_name, updated, updatedby, lifecycle, resources)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`

	klog.Infof("pkgRevWriteToDB: running query [%q] on repository (%#v)", sqlStatement, pr)

	prk := pr.Key()
	if returnedVal := GetDBConnection().db.QueryRow(
		sqlStatement,
		prk.Namespace, prk.Repository, prk.Package, prk.Revision, prk.WorkspaceName,
		pr.updated, pr.updatedBy, pr.lifecycle, rawResources); returnedVal.Err() == nil {
		klog.Infof("pkgRevWriteToDB: query succeeded, row created")
		return nil
	} else {
		klog.Infof("pkgRevWriteToDB: query failed %q", returnedVal.Err())
		return returnedVal.Err()
	}
}

func pkgRevUpdateDB(pr dbPackageRevision) error {

	klog.Infof("pkgRevUpdateDB: updating package revision %q", pr.Key())

	rawResources, err := GetDBConnection().encoder.EncodePackage(pr.resources)
	if err != nil {
		return err
	}

	sqlStatement := `
        UPDATE package_revisions SET updated=$5, updatedby=$6, lifecycle=$7, resources=$8
        WHERE namespace=$1 AND repo_name=$2 AND package_name=$3 AND package_rev=$4`

	klog.Infof("pkgRevUpdateDB: running query [%q] on repository (%#v)", sqlStatement, pr)

	prk := pr.Key()
	if returnedVal := GetDBConnection().db.QueryRow(
		sqlStatement,
		prk.Namespace, prk.Repository, prk.Package, prk.Revision,
		pr.updated, pr.updatedBy, pr.lifecycle, rawResources); returnedVal.Err() == nil {
		klog.Infof("pkgRevUpdateDB:: query succeeded, row created")
		return nil
	} else {
		klog.Infof("pkgRevUpdateDB:: query failed %q", returnedVal.Err())
		return returnedVal.Err()
	}
}

func pkgRevDeleteFromDB(prk repository.PackageRevisionKey) error {
	klog.Infof("pkgRevDeleteFromDB: deleting package revision %q", prk)

	sqlStatement := `DELETE FROM package_revisions WHERE namespace=$1 AND repo_name=$2 AND package_name=$3 AND package_rev=$4`

	returnedVal := GetDBConnection().db.QueryRow(sqlStatement,
		prk.Namespace, prk.Repository, prk.Package, prk.Revision)

	if returnedVal.Err() == nil {
		klog.Infof("pkgRevDeleteFromDB: deleted package revision %q", prk)
	} else {
		klog.Infof("pkgRevDeleteFromDB: deletion of package revision %q failed: %q", prk, returnedVal.Err())
	}

	return returnedVal.Err()
}
