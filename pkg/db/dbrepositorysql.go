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

func repoReadFromDB(rk repository.RepositoryKey) (dbRepository, error) {
	sqlStatement := `SELECT namespace, repo_name, updated, updatedby, deployment FROM repositories WHERE namespace=$1 AND repo_name=$2`

	var dbRepo dbRepository

	err := GetDBConnection().db.QueryRow(sqlStatement, rk.Namespace, rk.Repository).Scan(
		&dbRepo.repoKey.Namespace,
		&dbRepo.repoKey.Repository,
		&dbRepo.updated,
		&dbRepo.updatedBy,
		&dbRepo.deployment)

	return dbRepo, err
}

func repoWriteToDB(r dbRepository) error {
	sqlStatement := `
        INSERT INTO repositories (namespace, repo_name, updated, updatedby, deployment)
        VALUES ($1, $2, $3, $4, $5)`

	klog.Infof("DB Connection: running query [%q] on repository (%#v)", sqlStatement, r)

	rk := r.Key()
	if returnedVal := GetDBConnection().db.QueryRow(
		sqlStatement,
		rk.Namespace, rk.Repository, r.updated, r.updatedBy, r.deployment); returnedVal.Err() == nil {
		klog.Infof("DB Connection: query succeeded, row created")
		return nil
	} else {
		klog.Infof("DB Connection: query failed %q", returnedVal.Err())
		return returnedVal.Err()
	}
}

func repoUpdateDB(r dbRepository) error {
	sqlStatement := `
        UPDATE repositories SET updated=$3, updatedby=$4
        WHERE namespace=$1 AND repo_name=$2`

	klog.Infof("repoUpdateDB: running query [%q] on %q)", sqlStatement, r.Key())

	rk := r.Key()
	if returnedVal := GetDBConnection().db.QueryRow(
		sqlStatement,

		rk.Namespace, rk.Repository, r.updated, r.updatedBy); returnedVal.Err() == nil {
		klog.Infof("repoUpdateDB: query succeeded for %q", rk)
		return nil
	} else {
		klog.Infof("repoUpdateDB: query failed for %q: %q", rk, returnedVal.Err())
		return returnedVal.Err()
	}
}

func repoDeleteFromDB(rk repository.RepositoryKey) error {
	sqlStatement := `DELETE FROM repositories WHERE namespace=$1 AND repo_name=$2`

	returnedVal := GetDBConnection().db.QueryRow(sqlStatement, rk.Namespace, rk.Repository)

	return returnedVal.Err()
}