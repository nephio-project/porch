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

	"github.com/nephio-project/porch/pkg/repository"
	"go.opentelemetry.io/otel/trace"
	"k8s.io/klog/v2"
)

func pkgReadFromDB(ctx context.Context, pk repository.PackageKey) (dbPackage, error) {
	_, span := tracer.Start(ctx, "dbpackagesql::pkgReadFromDB", trace.WithAttributes())
	defer span.End()

	sqlStatement := `SELECT * FROM packages WHERE k8s_name_space=$1 AND k8s_name=$3`

	var dbPkg dbPackage
	var metaAsJson, specAsJson string

	klog.Infof("pkgReadFromDB: running query [%q] on %q", sqlStatement, pk)
	err := GetDB().db.QueryRow(sqlStatement, pk.K8Sns(), pk.K8Sname()).Scan(
		&dbPkg.pkgKey.RepoKey.Namespace,
		&dbPkg.pkgKey.Package,
		&dbPkg.pkgKey.RepoKey.Name,
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
		return dbPkg, err
	}

	setValueFromJson(metaAsJson, &dbPkg.meta)
	setValueFromJson(specAsJson, &dbPkg.spec)

	return dbPkg, err
}

func pkgReadPkgsFromDB(ctx context.Context, rk repository.RepositoryKey) ([]dbPackage, error) {
	_, span := tracer.Start(ctx, "dbpackagesql::pkgReadPkgsFromDB", trace.WithAttributes())
	defer span.End()

	sqlStatement := `SELECT * FROM packages WHERE k8s_name_space=$1 AND repo_k8s_name=$2`

	var dbPkgs []dbPackage

	rows, err := GetDB().db.Query(
		sqlStatement, rk.Namespace, rk.Name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	klog.Infof("pkgReadPkgsFromDB: query succeeded for %q", rk)

	for rows.Next() {
		var pkg dbPackage
		var metaAsJson, specAsJson string

		if err := rows.Scan(
			&pkg.pkgKey.RepoKey.Namespace,
			&pkg.pkgKey.Package,
			&pkg.pkgKey.RepoKey.Name,
			&pkg.pkgKey.Path,
			&metaAsJson,
			&specAsJson,
			&pkg.updated,
			&pkg.updatedBy); err != nil {
			return nil, err
		}

		setValueFromJson(metaAsJson, &pkg.meta)
		setValueFromJson(specAsJson, &pkg.spec)

		dbPkgs = append(dbPkgs, pkg)
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
		pk.K8Sns(), pk.K8Sname(),
		pk.RepoKey.K8Sname(), pk.Path, valueAsJson(p.meta), valueAsJson(p.spec), p.updated, p.updatedBy); err == nil {
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
	if _, err := GetDB().db.Exec(
		sqlStatement,
		pk.K8Sns(), pk.K8Sname(),
		pk.RepoKey.K8Sname(), pk.Path,
		valueAsJson(p.meta), valueAsJson(p.spec), p.updated, p.updatedBy); err == nil {
		klog.Infof("pkgUpdateDB: query succeeded for %q", pk)
		return nil
	} else {
		klog.Infof("pkgUpdateDB: query failed for %q: %q", pk, err)
		return err
	}
}

func pkgDeleteFromDB(ctx context.Context, pk repository.PackageKey) error {
	_, span := tracer.Start(ctx, "dbpackagesql::pkgDeleteFromDB", trace.WithAttributes())
	defer span.End()

	sqlStatement := `DELETE FROM packages WHERE k8s_name_space=$1 AND k8s_name=$2`

	klog.Infof("DB Connection: running query [%q] on %q", sqlStatement, pk)
	if _, err := GetDB().db.Exec(sqlStatement, pk.K8Sns(), pk.K8Sname()); err == nil {
		klog.Infof("pkgDeleteFromDB: query succeeded for %q", pk)
		return nil
	} else {
		klog.Infof("pkgDeleteFromDB: query failed for %q: %q", pk, err)
		return err
	}
}
