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

	cachetypes "github.com/nephio-project/porch/pkg/cache/types"
	"github.com/nephio-project/porch/pkg/repository"
	"go.opentelemetry.io/otel/trace"
	"k8s.io/klog/v2"
)

func pkgRevReadFromDB(ctx context.Context, prk repository.PackageRevisionKey) (*dbPackageRevision, error) {
	_, span := tracer.Start(ctx, "dbpackagerevisionsql::pkgRevReadFromDB", trace.WithAttributes())
	defer span.End()

	klog.Infof("pkgRevReadFromDB: reading package revision %q", prk)

	sqlStatement := `
		SELECT
			repositories.k8s_name_space,
			repositories.k8s_name,
			repositories.directory,
			repositories.default_ws_name,
			packages.k8s_name,
			packages.package_path,
			package_revisions.k8s_name,
			package_revisions.revision,
			package_revisions.meta,
			package_revisions.spec,
			package_revisions.updated,
			package_revisions.updatedby,
			package_revisions.lifecycle
		FROM package_revisions INNER JOIN packages
			ON package_revisions.k8s_name_space=packages.k8s_name_space AND package_revisions.package_k8s_name=packages.k8s_name
		 INNER JOIN repositories
			ON packages.k8s_name_space=repositories.k8s_name_space AND packages.repo_k8s_name=repositories.k8s_name
		WHERE package_revisions.k8s_name_space=$1 AND package_revisions.k8s_name=$2`

	rows, err := GetDB().db.Query(sqlStatement, prk.K8SNS(), prk.K8SName())
	if err != nil {
		klog.Infof("pkgRevReadFromDB: reading package revision %q returned err: %q", prk, err)
		return nil, err
	}

	prs, err := pkgRevScanRowsFromDB(ctx, rows)
	if err != nil {
		klog.Infof("pkgRevReadFromDB: reading package revision %q returned err: %q", prk, err)
		return nil, err
	}

	if len(prs) == 0 {
		return nil, sql.ErrNoRows
	}

	if len(prs) > 1 {
		return nil, fmt.Errorf("pkgRevReadFromDB: reading package revision %q should return 1 package revision, it returned %d package revisions", prk, len(prs))
	}

	return prs[0], err
}

func pkgRevReadPRsFromDB(ctx context.Context, pk repository.PackageKey) ([]*dbPackageRevision, error) {
	_, span := tracer.Start(ctx, "dbpackagerevisionsql::pkgRevReadPRsFromDB", trace.WithAttributes())
	defer span.End()

	sqlStatement := `
		SELECT
			repositories.k8s_name_space,
			repositories.k8s_name,
			repositories.directory,
			repositories.default_ws_name,
			packages.k8s_name,
			packages.package_path,
			package_revisions.k8s_name,
			package_revisions.revision,
			package_revisions.meta,
			package_revisions.spec,
			package_revisions.updated,
			package_revisions.updatedby,
			package_revisions.lifecycle
		FROM package_revisions INNER JOIN packages
			ON package_revisions.k8s_name_space=packages.k8s_name_space AND package_revisions.package_k8s_name=packages.k8s_name
		 INNER JOIN repositories
			ON packages.k8s_name_space=repositories.k8s_name_space AND packages.repo_k8s_name=repositories.k8s_name
		WHERE packages.k8s_name_space=$1 AND packages.k8s_name=$2`

	return pkgRevReadPRListFromDB(ctx, pk, sqlStatement)
}

func pkgRevGetlatestRevFromDB(ctx context.Context, pk repository.PackageKey) (int, error) {

	latestPR, err := pkgRevReadLatestPRFromDB(ctx, pk)
	if err != nil {
		return 0, err
	}

	if latestPR != nil {
		return latestPR.Key().Revision, nil
	} else {
		return 0, nil
	}
}

func pkgRevReadLatestPRFromDB(ctx context.Context, pk repository.PackageKey) (*dbPackageRevision, error) {
	_, span := tracer.Start(ctx, "dbpackagerevisionsql::pkgRevReadLatestPRFromDB", trace.WithAttributes())
	defer span.End()

	sqlStatement := `
		SELECT
			repositories.k8s_name_space,
			repositories.k8s_name,
			repositories.directory,
			repositories.default_ws_name,
			packages.k8s_name,
			packages.package_path,
			package_revisions.k8s_name,
			package_revisions.revision,
			package_revisions.meta,
			package_revisions.spec,
			package_revisions.updated,
			package_revisions.updatedby,
			package_revisions.lifecycle
		FROM package_revisions INNER JOIN packages
			ON package_revisions.k8s_name_space=packages.k8s_name_space AND package_revisions.package_k8s_name=packages.k8s_name
		 INNER JOIN repositories
			ON packages.k8s_name_space=repositories.k8s_name_space AND packages.repo_k8s_name=repositories.k8s_name
		WHERE packages.k8s_name_space=$1 AND packages.k8s_name=$2 AND package_revisions.revision=(
			SELECT MAX(revision) FROM package_revisions
				WHERE packages.k8s_name_space=$1 AND packages.k8S_name=$2
			)`

	latestPRList, err := pkgRevReadPRListFromDB(ctx, pk, sqlStatement)
	if err != nil {
		return nil, err
	}

	switch len(latestPRList) {
	case 1:
		return latestPRList[0], nil
	case 0:
		return nil, fmt.Errorf("latest package revision for package %+v not found in DB", pk)
	default:
		// Multiple drafts with a revision value of 0 are allowed, just return nil with no error
		if latestPRList[0].pkgRevKey.Revision == 0 {
			return nil, nil
		} else {
			return nil, fmt.Errorf("multiple latest package revisions with revision %d for package %+v found in DB", latestPRList[0].pkgRevKey.Revision, pk)
		}
	}
}

func pkgRevReadPRListFromDB(ctx context.Context, pk repository.PackageKey, sqlStatement string) ([]*dbPackageRevision, error) {
	_, span := tracer.Start(ctx, "dbpackagerevisionsql::pkgRevReadPRsFromDB", trace.WithAttributes())
	defer span.End()

	klog.Infof("pkgReadPRsFromDB: running query [%q] on %q", sqlStatement, pk)

	rows, err := GetDB().db.Query(sqlStatement, pk.K8SNS(), pk.K8SName())
	if err != nil {
		klog.Infof("pkgReadPRsFromDB: query failed for %q: %q", pk, err)
		return nil, err
	}

	klog.Infof("pkgReadPRsFromDB: query succeeded for %q", pk)

	prs, err := pkgRevScanRowsFromDB(ctx, rows)

	if err != nil {
		if err == sql.ErrNoRows {
			klog.Infof("pkgRevReadPRListFromDB: package revisions for package %q not found in db", pk)
		} else {
			klog.Infof("pkgRevReadPRListFromDB: reading package revisions for package %q returned err: %q", pk, err)
		}
		return nil, err
	}

	return prs, nil
}

func pkgRevScanRowsFromDB(ctx context.Context, rows *sql.Rows) ([]*dbPackageRevision, error) {
	_, span := tracer.Start(ctx, "dbpackagesql::pkgScanRowsFromDB", trace.WithAttributes())
	defer span.End()

	defer rows.Close()

	var dbPkgRevs []*dbPackageRevision

	for rows.Next() {
		var pkgRev dbPackageRevision
		var pkgK8SName, prK8SName, metaAsJson, specAsJson string

		err := rows.Scan(
			&pkgRev.pkgRevKey.PkgKey.RepoKey.Namespace,
			&pkgRev.pkgRevKey.PkgKey.RepoKey.Name,
			&pkgRev.pkgRevKey.PkgKey.RepoKey.Path,
			&pkgRev.pkgRevKey.PkgKey.RepoKey.PlaceholderWSname,
			&pkgK8SName,
			&pkgRev.pkgRevKey.PkgKey.Path,
			&prK8SName,
			&pkgRev.pkgRevKey.Revision,
			&metaAsJson,
			&specAsJson,
			&pkgRev.updated,
			&pkgRev.updatedBy,
			&pkgRev.lifecycle)

		if err != nil {
			return nil, err
		}

		pkgRev.repo = cachetypes.CacheInstance.GetRepository(pkgRev.pkgRevKey.PkgKey.RepoKey).(*dbRepository)
		pkgRev.pkgRevKey.PkgKey.Package = repository.K8SName2PkgName(pkgK8SName)
		pkgRev.pkgRevKey.WorkspaceName = repository.K8SName2PkgRevWSName(pkgK8SName, prK8SName)
		setValueFromJson(metaAsJson, &pkgRev.meta)
		setValueFromJson(specAsJson, &pkgRev.spec)

		dbPkgRevs = append(dbPkgRevs, &pkgRev)

		pkgRev.resources, err = pkgRevResourcesReadFromDB(ctx, pkgRev.pkgRevKey)
		if err != nil {
			klog.Infof("pkgRevScanRowsFromDB: reading package revision %q resources returned err: %q", pkgRev.pkgRevKey, err)
			return nil, err
		}

		klog.Infof("pkgRevScanRowsFromDB: reading package revision resources succeeded %q", pkgRev.pkgRevKey)

	}

	return dbPkgRevs, nil
}

func pkgRevWriteToDB(ctx context.Context, pr *dbPackageRevision) error {
	_, span := tracer.Start(ctx, "dbpackagerevisionsql::pkgRevWriteToDB", trace.WithAttributes())
	defer span.End()

	klog.Infof("pkgRevWriteToDB: writing package revision %q", pr.Key())

	sqlStatement := `
        INSERT INTO package_revisions (k8s_name_space, k8s_name, package_k8s_name, revision, meta, spec, updated, updatedby, lifecycle)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`

	klog.Infof("pkgRevWriteToDB: running query [%q] on repository (%#v)", sqlStatement, pr)

	prk := pr.Key()
	if _, err := GetDB().db.Exec(
		sqlStatement,
		prk.K8SNS(), prk.K8SName(),
		prk.GetPackageKey().K8SName(), prk.Revision, valueAsJson(pr.meta), valueAsJson(pr.spec), pr.updated, pr.updatedBy, pr.lifecycle); err == nil {
		klog.Infof("pkgRevWriteToDB: query succeeded, row created")
	} else {
		klog.Infof("pkgRevWriteToDB: query failed %q", err)
		return err
	}

	if err := pkgRevResourcesWriteToDB(ctx, pr); err == nil {
		klog.Infof("pkgRevWriteToDB: resources written to DB")
		return nil
	} else {
		klog.Infof("pkgRevWriteToDB: resource write to DB failed: %q", err)
		return err
	}
}

func pkgRevUpdateDB(ctx context.Context, pr *dbPackageRevision) error {
	_, span := tracer.Start(ctx, "dbpackagerevisionsql::pkgRevUpdateDB", trace.WithAttributes())
	defer span.End()

	klog.Infof("pkgRevUpdateDB: updating package revision %q", pr.Key())

	sqlStatement := `
        UPDATE package_revisions SET package_k8s_name=$3, revision=$4, meta=$5, spec=$6, updated=$7, updatedby=$8, lifecycle=$9
        WHERE k8s_name_space=$1 AND k8s_name=$2`

	klog.Infof("pkgRevUpdateDB: running query [%q] on repository (%#v)", sqlStatement, pr)

	prk := pr.Key()
	result, err := GetDB().db.Exec(
		sqlStatement,
		prk.K8SNS(), prk.K8SName(),
		prk.GetPackageKey().K8SName(), prk.Revision, valueAsJson(pr.meta), valueAsJson(pr.spec), pr.updated, pr.updatedBy, pr.lifecycle)

	if err == nil {
		if rowsAffected, _ := result.RowsAffected(); rowsAffected == 1 {
			klog.Infof("pkgRevUpdateDB:: query succeeded, row updated")
		} else {
			err = fmt.Errorf("update failed, no rows or multiple rows found for updating")
		}
	}

	if err != nil {
		klog.Infof("pkgRevUpdateDB:: query failed %q", err)
		return err
	}

	if err := pkgRevResourcesWriteToDB(ctx, pr); err == nil {
		klog.Infof("pkgRevUpdateDB: resources written to DB")
		return nil
	} else {
		klog.Infof("pkgRevUpdateDB: resources write to DB failed: %q", err)
		return err
	}
}

func pkgRevDeleteFromDB(ctx context.Context, prk repository.PackageRevisionKey) error {
	_, span := tracer.Start(ctx, "dbpackagerevisionsql::pkgRevDeleteFromDB", trace.WithAttributes())
	defer span.End()

	klog.Infof("pkgRevDeleteFromDB: deleting package revision %q", prk)

	if err := pkgRevResourcesDeleteFromDB(ctx, prk); err == nil {
		klog.Infof("pkgRevDeleteFromDB: resources deleted from DB")
	} else {
		klog.Infof("pkgRevDeleteFromDB: resources delete from DB failed: %q", err)
		return err
	}

	sqlStatement := `DELETE FROM package_revisions WHERE k8s_name_space=$1 AND k8s_name=$2`

	_, err := GetDB().db.Exec(sqlStatement, prk.K8SNS(), prk.K8SName())

	if err == nil {
		klog.Infof("pkgRevDeleteFromDB: deleted package revision %q", prk)
	} else {
		klog.Infof("pkgRevDeleteFromDB: deletion of package revision %q failed: %q", prk, err)
	}

	return err
}
