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

func pkgRevReadFromDB(prk repository.PackageRevisionKey) (dbPackageRevision, error) {
	klog.Infof("pkgRevReadFromDB: reading package revision %q", prk)

	sqlStatement := `SELECT * FROM package_revisions
     WHERE name_space=$1 AND repo_name=$2 AND package_name=$3 AND package_rev=$4 AND workspace_name=$5`

	var pkgRev dbPackageRevision
	var metaAsJson, specAsJson string

	err := GetDB().db.QueryRow(
		sqlStatement, prk.Namespace, prk.Repository, prk.Package, prk.Revision, prk.WorkspaceName).Scan(
		&pkgRev.pkgRevKey.Namespace,
		&pkgRev.pkgRevKey.Repository,
		&pkgRev.pkgRevKey.Package,
		&pkgRev.pkgRevKey.Revision,
		&pkgRev.pkgRevKey.WorkspaceName,
		&metaAsJson,
		&specAsJson,
		&pkgRev.updated,
		&pkgRev.updatedBy,
		&pkgRev.lifecycle)

	if err != nil {
		if err == sql.ErrNoRows {
			klog.Infof("pkgRevReadFromDB: package revision not found in db %q", prk)
		} else {
			klog.Infof("pkgRevReadFromDB: reading package revision %q returned err: %q", prk, err)
		}
		return pkgRev, err
	}

	pkgRev.resources, err = pkgRevResourcesReadFromDB(prk)
	if err != nil {
		klog.Infof("pkgRevReadFromDB: reading package revision %q resources returned err: %q", prk, err)
		return pkgRev, err
	}

	klog.Infof("pkgRevReadFromDB: reading package succeeded %q", prk)

	setValueFromJson(metaAsJson, pkgRev.meta)
	setValueFromJson(specAsJson, pkgRev.spec)

	return pkgRev, err
}

func pkgRevReadPRsFromDB(pk repository.PackageKey) ([]*dbPackageRevision, error) {
	sqlStatement := `SELECT * FROM package_revisions WHERE name_space=$1 AND repo_name=$2 AND package_name=$3`

	var dbPkgRevs []*dbPackageRevision

	klog.Infof("pkgReadPRsFromDB: running query [%q] on %q", sqlStatement, pk)

	rows, err := GetDB().db.Query(sqlStatement, pk.Namespace, pk.Repository, pk.Package)
	if err != nil {
		klog.Infof("pkgReadPRsFromDB: query failed for %q: %q", pk, err)
		return nil, err
	}
	defer rows.Close()

	klog.Infof("pkgReadPRsFromDB: query succeeded for %q", pk)

	for rows.Next() {
		var pkgRev dbPackageRevision
		var metaAsJson, specAsJson string

		if err := rows.Scan(
			&pkgRev.pkgRevKey.Namespace,
			&pkgRev.pkgRevKey.Repository,
			&pkgRev.pkgRevKey.Package,
			&pkgRev.pkgRevKey.Revision,
			&pkgRev.pkgRevKey.WorkspaceName,
			&metaAsJson,
			&specAsJson,
			&pkgRev.updated,
			&pkgRev.updatedBy,
			&pkgRev.lifecycle); err != nil {
			return nil, err
		}

		pkgRev.resources, err = pkgRevResourcesReadFromDB(pkgRev.pkgRevKey)
		if err != nil {
			klog.Infof("pkgRevReadFromDB: reading package revision %q resources returned err: %q", pkgRev.pkgRevKey, err)
			return nil, err
		}

		klog.Infof("pkgRevReadFromDB: reading package succeeded %q", pkgRev.pkgRevKey)

		setValueFromJson(metaAsJson, pkgRev.meta)
		setValueFromJson(specAsJson, pkgRev.spec)

		dbPkgRevs = append(dbPkgRevs, &pkgRev)
	}

	return dbPkgRevs, nil
}

func pkgRevReadLatestPRFromDB(_ repository.PackageKey) *dbPackageRevision {
	return nil
}

func pkgRevWriteToDB(pr *dbPackageRevision) error {
	klog.Infof("pkgRevWriteToDB: writing package revision %q", pr.Key())

	sqlStatement := `
        INSERT INTO package_revisions (name_space, repo_name, package_name, package_rev, workspace_name, meta, spec, updated, updatedby, lifecycle)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`

	klog.Infof("pkgRevWriteToDB: running query [%q] on repository (%#v)", sqlStatement, pr)

	prk := pr.Key()
	if _, err := GetDB().db.Exec(
		sqlStatement,
		prk.Namespace, prk.Repository, prk.Package, prk.Revision, prk.WorkspaceName,
		valueAsJson(pr.meta), valueAsJson(pr.spec), pr.updated, pr.updatedBy, pr.lifecycle); err == nil {
		klog.Infof("pkgRevWriteToDB: query succeeded, row created")
	} else {
		klog.Infof("pkgRevWriteToDB: query failed %q", err)
		return err
	}

	if err := pkgRevResourcesWriteToDB(pr); err == nil {
		klog.Infof("pkgRevWriteToDB: resources written to DB")
		return nil
	} else {
		klog.Infof("pkgRevWriteToDB: resource write to DB failed: %q", err)
		return err
	}
}

func pkgRevUpdateDB(pr *dbPackageRevision) error {
	klog.Infof("pkgRevUpdateDB: updating package revision %q", pr.Key())

	sqlStatement := `
        UPDATE package_revisions SET meta=$6, spec=$7, updated=$8, updatedby=$9, lifecycle=$10
        WHERE name_space=$1 AND repo_name=$2 AND package_name=$3 AND package_rev=$4 AND workspace_name=$5`

	klog.Infof("pkgRevUpdateDB: running query [%q] on repository (%#v)", sqlStatement, pr)

	prk := pr.Key()
	if _, err := GetDB().db.Exec(
		sqlStatement,
		prk.Namespace, prk.Repository, prk.Package, prk.Revision, prk.WorkspaceName,
		valueAsJson(pr.meta), valueAsJson(pr.spec), pr.updated, pr.updatedBy, pr.lifecycle); err == nil {
		klog.Infof("pkgRevUpdateDB:: query succeeded, row created")
	} else {
		klog.Infof("pkgRevUpdateDB:: query failed %q", err)
		return err
	}

	if err := pkgRevResourcesWriteToDB(pr); err == nil {
		klog.Infof("pkgRevWriteToDB: resources written to DB")
		return nil
	} else {
		klog.Infof("pkgRevWriteToDB: resources write to DB failed: %q", err)
		return err
	}
}

func pkgRevDeleteFromDB(prk repository.PackageRevisionKey) error {
	klog.Infof("pkgRevDeleteFromDB: deleting package revision %q", prk)

	if err := pkgRevResourcesDeleteFromDB(prk); err == nil {
		klog.Infof("pkgRevDeleteFromDB: resources deleted from DB")
	} else {
		klog.Infof("pkgRevWriteToDB: resources delete from DB failed: %q", err)
		return err
	}

	sqlStatement := `DELETE FROM package_revisions WHERE name_space=$1 AND repo_name=$2 AND package_name=$3 AND package_rev=$4 AND workspace_name=$5`

	_, err := GetDB().db.Exec(sqlStatement, prk.Namespace, prk.Repository, prk.Package, prk.Revision, prk.WorkspaceName)

	if err == nil {
		klog.Infof("pkgRevDeleteFromDB: deleted package revision %q", prk)
	} else {
		klog.Infof("pkgRevDeleteFromDB: deletion of package revision %q failed: %q", prk, err)
	}

	return err
}
