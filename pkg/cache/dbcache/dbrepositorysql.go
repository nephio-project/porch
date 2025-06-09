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
	"fmt"

	"github.com/nephio-project/porch/pkg/repository"
	"go.opentelemetry.io/otel/trace"
	"k8s.io/klog/v2"
)

func repoReadFromDB(ctx context.Context, rk repository.RepositoryKey) (*dbRepository, error) {
	_, span := tracer.Start(ctx, "dbrepositorysql::repoReadFromDB", trace.WithAttributes())
	defer span.End()

	sqlStatement := `
		SELECT k8s_name_space, k8s_name, directory, default_ws_name, meta, spec, updated, updatedby, deployment
		FROM repositories
		WHERE k8s_name_space=$1 AND k8s_name=$2`

	var dbRepo dbRepository
	var metaAsJson, specAsJson string

	err := GetDB().db.QueryRow(sqlStatement, rk.K8SNS(), rk.K8SName()).Scan(
		&dbRepo.repoKey.Namespace,
		&dbRepo.repoKey.Name,
		&dbRepo.repoKey.Path,
		&dbRepo.repoKey.PlaceholderWSname,
		&metaAsJson,
		&specAsJson,
		&dbRepo.updated,
		&dbRepo.updatedBy,
		&dbRepo.deployment)

	if err != nil {
		return nil, err
	}

	setValueFromJson(metaAsJson, &dbRepo.meta)
	setValueFromJson(specAsJson, &dbRepo.spec)

	return &dbRepo, err
}

func repoWriteToDB(ctx context.Context, r *dbRepository) error {
	_, span := tracer.Start(ctx, "dbrepositorysql::repoWriteToDB", trace.WithAttributes())
	defer span.End()

	sqlStatement := `
        INSERT INTO repositories (k8s_name_space, k8s_name, directory, default_ws_name, meta, spec, updated, updatedby, deployment)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`

	klog.Infof("DB Connection: running query [%q] on repository (%#v)", sqlStatement, r)

	rk := r.Key()
	if _, err := GetDB().db.Exec(
		sqlStatement,
		rk.K8SNS(), rk.K8SName(), rk.Path, rk.PlaceholderWSname, valueAsJson(r.meta), valueAsJson(r.spec), r.updated, r.updatedBy, r.deployment); err == nil {
		klog.Infof("DB Connection: query succeeded, row created")
		return nil
	} else {
		klog.Infof("DB Connection: query failed %q", err)
		return err
	}
}

func repoUpdateDB(ctx context.Context, r *dbRepository) error {
	_, span := tracer.Start(ctx, "dbrepositorysql::repoUpdateDB", trace.WithAttributes())
	defer span.End()

	sqlStatement := `
        UPDATE repositories SET directory=$3, default_ws_name=$4 ,meta=$5, spec=$6, updated=$7, updatedby=$8, deployment=$9
        WHERE k8s_name_space=$1 AND k8s_name=$2`

	klog.Infof("repoUpdateDB: running query [%q] on %q)", sqlStatement, r.Key())

	rk := r.Key()
	result, err := GetDB().db.Exec(
		sqlStatement,
		rk.K8SNS(), rk.K8SName(), rk.Path, rk.PlaceholderWSname, valueAsJson(r.meta), valueAsJson(r.spec), r.updated, r.updatedBy, r.deployment)

	if err == nil {
		if rowsAffected, _ := result.RowsAffected(); rowsAffected == 1 {
			klog.Infof("repoUpdateDB: query succeeded for %q", rk)
			return nil
		}
		err = fmt.Errorf("update failed, no rows or multiple rows found for updating")
	}

	klog.Infof("repoUpdateDB: query failed for %q: %q", rk, err)
	return err
}

func repoDeleteFromDB(ctx context.Context, rk repository.RepositoryKey) error {
	_, span := tracer.Start(ctx, "dbrepositorysql::repoDeleteFromDB", trace.WithAttributes())
	defer span.End()

	sqlStatement := `DELETE FROM repositories WHERE k8s_name_space=$1 AND k8s_name=$2`

	_, err := GetDB().db.Exec(sqlStatement, rk.K8SNS(), rk.K8SName())

	return err
}
