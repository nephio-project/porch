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

func pkgRevResourceReadFromDB(prk repository.PackageRevisionKey, resKey string) (string, string, error) {
	klog.Infof("pkgRevResourceReadFromDB: reading package revision resource %q:%s", prk, resKey)

	sqlStatement := `SELECT resource_value FROM resources
     WHERE name_space=$1 AND repo_name=$2 AND package_name=$3 AND package_rev=$4 AND workspace_name=$5 AND key=$6`

	var resVal string

	err := GetDBConnection().db.QueryRow(
		sqlStatement, prk.Namespace, prk.Repository, prk.Package, prk.Revision, prk.WorkspaceName).Scan(
		&resVal)

	if err != nil {
		if err == sql.ErrNoRows {
			klog.Infof("pkgRevResourceReadFromDB: package revision resource not found in db %q:%s", prk, resKey)
		} else {
			klog.Infof("pkgRevResourceReadFromDB: reading package revision resource %q:%s returned err: %q", prk, resKey, err)
		}
		return resKey, "", err
	}

	klog.Infof("pkgRevResourceReadFromDB: reading package succeeded %q:%s", prk, resKey)

	return resKey, resVal, err
}

func pkgRevResourcesReadFromDB(prk repository.PackageRevisionKey) (map[string]string, error) {
	sqlStatement := `SELECT resource_key, resource_value FROM resources WHERE name_space=$1 AND repo_name=$2 AND package_name=$3 AND package_rev=$4 AND workspace_name=$5`

	resources := make(map[string]string)

	klog.Infof("pkgRevResourcesReadFromDB: running query [%q] on %q", sqlStatement, prk)

	rows, err := GetDBConnection().db.Query(sqlStatement, prk.Namespace, prk.Repository, prk.Package, prk.Revision, prk.WorkspaceName)
	if err != nil {
		klog.Infof("pkgRevResourcesReadFromDB: query failed for %q: %q", prk, err)
		return nil, err
	}

	klog.Infof("pkgRevResourcesReadFromDB: query succeeded for %q", prk)

	defer rows.Close()

	for rows.Next() {
		var resKey, resVal string

		rows.Scan(
			&resKey,
			&resVal)

		resources[resKey] = resVal
	}

	return resources, nil
}

func pkgRevResourcesWriteToDB(pr *dbPackageRevision) error {
	klog.Infof("pkgRevResourcesWriteToDB: writing package revision resources for %q", pr.Key())

	for k, v := range pr.resources {
		if err := pkgRevResourceWriteToDB(pr, k, v); err != nil {
			return err
		}
	}

	klog.Infof("pkgRevResourcesWriteToDB: succeeded, row created")
	return nil
}

func pkgRevResourceWriteToDB(pr *dbPackageRevision, resKey string, resVal string) error {
	klog.Infof("pkgRevResourceWriteToDB: writing package revision resource %q:%s", pr.Key(), resKey)

	sqlStatement := `
        INSERT INTO resources (name_space, repo_name, package_name, package_rev, workspace_name, resource_key, resource_value)
        VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (name_space, repo_name, package_name, package_rev, workspace_name, resource_key) 
		DO UPDATE SET resource_value = EXCLUDED.resource_value`

	klog.Infof("pkgRevResourceWriteToDB: running query [%q] on repository (%#v)", sqlStatement, pr)

	prk := pr.Key()
	if returnedVal := GetDBConnection().db.QueryRow(
		sqlStatement,
		prk.Namespace, prk.Repository, prk.Package, prk.Revision, prk.WorkspaceName,
		resKey, resVal); returnedVal.Err() == nil {
		klog.Infof("pkgRevResourceWriteToDB: query succeeded, row created")
		return nil
	} else {
		klog.Infof("pkgRevResourceWriteToDB: query failed %q", returnedVal.Err())
		return returnedVal.Err()
	}
}

func pkgRevResourcesDeleteFromDB(prk repository.PackageRevisionKey) error {
	klog.Infof("pkgRevResourcesDeleteFromDB: deleting package revision %q", prk)

	sqlStatement := `DELETE FROM resources WHERE name_space=$1 AND repo_name=$2 AND package_name=$3 AND package_rev=$4 AND workspace_name=$5`

	returnedVal := GetDBConnection().db.QueryRow(sqlStatement,
		prk.Namespace, prk.Repository, prk.Package, prk.Revision, prk.WorkspaceName)

	if returnedVal.Err() == nil {
		klog.Infof("pkgRevResourcesDeleteFromDB: deleted package revision %q", prk)
	} else {
		klog.Infof("pkgRevResourcesDeleteFromDB: deletion of package revision %q failed: %q", prk, returnedVal.Err())
	}

	return returnedVal.Err()
}

func pkgRevResourceDeleteFromDB(prk repository.PackageRevisionKey, resKey string) error {
	klog.Infof("pkgRevResourceDeleteFromDB: deleting package revision %q", prk)

	sqlStatement := `DELETE FROM resources WHERE name_space=$1 AND repo_name=$2 AND package_name=$3 AND package_rev=$4 AND workspace_name=$5 AND presource_key=$6`

	returnedVal := GetDBConnection().db.QueryRow(sqlStatement,
		prk.Namespace, prk.Repository, prk.Package, prk.Revision, prk.WorkspaceName, resKey)

	if returnedVal.Err() == nil {
		klog.Infof("pkgRevResourceDeleteFromDB: deleted package revision %q", prk)
	} else {
		klog.Infof("pkgRevResourceDeleteFromDB: deletion of package revision %q failed: %q", prk, returnedVal.Err())
	}

	return returnedVal.Err()
}
