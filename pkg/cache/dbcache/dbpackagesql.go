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
	"database/sql"
	"fmt"

	"github.com/nephio-project/porch/pkg/repository"
	"go.opentelemetry.io/otel/trace"
	"k8s.io/klog/v2"
)

func pkgReadFromDB(ctx context.Context, pk repository.PackageKey) (*dbPackage, error) {
	_, span := tracer.Start(ctx, "dbpackagesql::pkgReadFromDB", trace.WithAttributes())
	defer span.End()

	sqlStatement := `
		SELECT
			repositories.k8s_name_space, repositories.k8s_name, repositories.directory, repositories.default_ws_name,
			packages.k8s_name, packages.package_path, packages.meta, packages.spec, packages.updated, packages.updatedby
		FROM packages INNER JOIN repositories
			ON packages.k8s_name_space=repositories.k8s_name_space AND packages.repo_k8s_name=repositories.k8s_name
		WHERE packages.k8s_name_space=$1 AND packages.k8s_name=$2`

	var dbPkg dbPackage
	var pkgK8SName, metaAsJson, specAsJson string

	klog.Infof("pkgReadFromDB: running query [%q] on %q", sqlStatement, pk)
	err := GetDB().db.QueryRow(sqlStatement, pk.K8SNS(), pk.K8SName()).Scan(
		&dbPkg.pkgKey.RepoKey.Namespace,
		&dbPkg.pkgKey.RepoKey.Name,
		&dbPkg.pkgKey.RepoKey.Path,
		&dbPkg.pkgKey.RepoKey.PlaceholderWSname,
		&pkgK8SName,
		&dbPkg.pkgKey.Path,
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
		return nil, err
	}

	dbPkg.pkgKey.Package = repository.K8SName2PkgName(pkgK8SName)
	setValueFromJson(metaAsJson, &dbPkg.meta)
	setValueFromJson(specAsJson, &dbPkg.spec)

	return &dbPkg, err
}

func pkgReadPkgsFromDB(ctx context.Context, rk repository.RepositoryKey) ([]*dbPackage, error) {
	_, span := tracer.Start(ctx, "dbpackagesql::pkgReadPkgsFromDB", trace.WithAttributes())
	defer span.End()

	sqlStatement := `
		SELECT
			repositories.k8s_name_space, repositories.k8s_name, repositories.directory, repositories.default_ws_name,
			packages.k8s_name, packages.package_path, packages.meta, packages.spec, packages.updated, packages.updatedby
		FROM packages INNER JOIN repositories
			ON packages.k8s_name_space=repositories.k8s_name_space AND packages.repo_k8s_name=repositories.k8s_name
		WHERE packages.k8s_name_space=$1 AND packages.repo_k8s_name=$2`

	var dbPkgs []*dbPackage

	rows, err := GetDB().db.Query(
		sqlStatement, rk.Namespace, rk.Name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	klog.Infof("pkgReadPkgsFromDB: query succeeded for %q", rk)

	for rows.Next() {
		var dbPkg dbPackage
		var pkgK8SName, metaAsJson, specAsJson string

		err := rows.Scan(
			&dbPkg.pkgKey.RepoKey.Namespace,
			&dbPkg.pkgKey.RepoKey.Name,
			&dbPkg.pkgKey.RepoKey.Path,
			&dbPkg.pkgKey.RepoKey.PlaceholderWSname,
			&pkgK8SName,
			&dbPkg.pkgKey.Path,
			&metaAsJson,
			&specAsJson,
			&dbPkg.updated,
			&dbPkg.updatedBy)

		if err != nil {
			return nil, err
		}

		dbPkg.pkgKey.Package = repository.K8SName2PkgName(pkgK8SName)
		setValueFromJson(metaAsJson, &dbPkg.meta)
		setValueFromJson(specAsJson, &dbPkg.spec)

		dbPkgs = append(dbPkgs, &dbPkg)
	}

	return dbPkgs, nil
}

func pkgWriteToDB(ctx context.Context, p *dbPackage) error {
	_, span := tracer.Start(ctx, "dbpackagesql::pkgWriteToDB", trace.WithAttributes())
	defer span.End()

	sqlStatement := `
        INSERT INTO packages (k8s_name_space, k8s_name, repo_k8s_name, package_path, meta, spec, updated, updatedby)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`

	klog.Infof("pkgWriteToDB: running query [%q] on %q", sqlStatement, p.Key())

	pk := p.Key()
	if _, err := GetDB().db.Exec(sqlStatement,
		pk.K8SNS(), pk.K8SName(),
		pk.RepoKey.K8SName(), pk.Path, valueAsJson(p.meta), valueAsJson(p.spec), p.updated, p.updatedBy); err == nil {
		klog.Infof("pkgWriteToDB: query succeeded for %q", p.Key())
		return nil
	} else {
		klog.Infof("pkgWriteToDB: query failed for %q: %q", p.Key(), err)
		return err
	}
}

func pkgUpdateDB(ctx context.Context, p *dbPackage) error {
	_, span := tracer.Start(ctx, "dbpackagesql::pkgUpdateDB", trace.WithAttributes())
	defer span.End()

	sqlStatement := `
        UPDATE packages SET repo_k8s_name=$3, package_path=$4, meta=$5, spec=$6, updated=$7, updatedby=$8
        WHERE k8s_name_space=$1 AND k8s_name=$2`

	klog.Infof("pkgUpdateDB: running query [%q] on %q)", sqlStatement, p.Key())

	pk := p.Key()
	result, err := GetDB().db.Exec(
		sqlStatement,
		pk.K8SNS(), pk.K8SName(),
		pk.RepoKey.K8SName(), pk.Path,
		valueAsJson(p.meta), valueAsJson(p.spec), p.updated, p.updatedBy)

	if err == nil {
		if rowsAffected, _ := result.RowsAffected(); rowsAffected > 0 {
			klog.Infof("pkgUpdateDB: query succeeded for %q", pk)
			return nil
		}
		err = fmt.Errorf("update failed, no rows found for updating")
	}

	klog.Infof("pkgUpdateDB: query failed for %q: %q", pk, err)
	return err
}

func pkgDeleteFromDB(ctx context.Context, pk repository.PackageKey) error {
	_, span := tracer.Start(ctx, "dbpackagesql::pkgDeleteFromDB", trace.WithAttributes())
	defer span.End()

	sqlStatement := `DELETE FROM packages WHERE k8s_name_space=$1 AND k8s_name=$2`

	klog.Infof("DB Connection: running query [%q] on %q", sqlStatement, pk)
	if _, err := GetDB().db.Exec(sqlStatement, pk.K8SNS(), pk.K8SName()); err == nil {
		klog.Infof("pkgDeleteFromDB: query succeeded for %q", pk)
		return nil
	} else {
		klog.Infof("pkgDeleteFromDB: query failed for %q: %q", pk, err)
		return err
	}
}
