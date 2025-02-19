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
	"github.com/nephio-project/porch/pkg/repository"
	"k8s.io/klog/v2"
)

func repoReadFromDB(rk repository.RepositoryKey) (*dbRepository, error) {
	sqlStatement := `SELECT name_space, repo_name, meta, spec, updated, updatedby, deployment FROM repositories WHERE name_space=$1 AND repo_name=$2`

	var dbRepo dbRepository
	var metaAsJson, specAsJson string

	err := GetDB().db.QueryRow(sqlStatement, rk.Namespace, rk.Repository).Scan(
		&dbRepo.repoKey.Namespace,
		&dbRepo.repoKey.Repository,
		&metaAsJson,
		&specAsJson,
		&dbRepo.updated,
		&dbRepo.updatedBy,
		&dbRepo.deployment)

	if err != nil {
		return nil, err
	}

	setValueFromJson(metaAsJson, dbRepo.meta)
	setValueFromJson(specAsJson, dbRepo.spec)

	return &dbRepo, err
}

func repoWriteToDB(r *dbRepository) error {
	sqlStatement := `
        INSERT INTO repositories (name_space, repo_name, meta, spec, updated, updatedby, deployment)
        VALUES ($1, $2, $3, $4, $5, $6, $7)`

	klog.Infof("DB Connection: running query [%q] on repository (%#v)", sqlStatement, r)

	rk := r.Key()
	if _, err := GetDB().db.Exec(
		sqlStatement,
		rk.Namespace, rk.Repository, valueAsJson(r.meta), valueAsJson(r.spec), r.updated, r.updatedBy, r.deployment); err == nil {
		klog.Infof("DB Connection: query succeeded, row created")
		return nil
	} else {
		klog.Infof("DB Connection: query failed %q", err)
		return err
	}
}

func repoUpdateDB(r *dbRepository) error {
	sqlStatement := `
        UPDATE repositories SET meta=$3, spec=$4, updated=$5, updatedby=$6, deployment=$7
        WHERE name_space=$1 AND repo_name=$2`

	klog.Infof("repoUpdateDB: running query [%q] on %q)", sqlStatement, r.Key())

	rk := r.Key()
	if _, err := GetDB().db.Exec(
		sqlStatement,
		rk.Namespace, rk.Repository, valueAsJson(r.meta), valueAsJson(r.spec), r.updated, r.updatedBy, r.deployment); err == nil {
		klog.Infof("repoUpdateDB: query succeeded for %q", rk)
		return nil
	} else {
		klog.Infof("repoUpdateDB: query failed for %q: %q", rk, err)
		return err
	}
}

func repoDeleteFromDB(rk repository.RepositoryKey) error {
	sqlStatement := `DELETE FROM repositories WHERE name_space=$1 AND repo_name=$2`

	_, err := GetDB().db.Exec(sqlStatement, rk.Namespace, rk.Repository)

	return err
}
