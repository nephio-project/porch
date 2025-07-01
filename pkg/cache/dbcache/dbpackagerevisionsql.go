// Copyright 2024-2025 The Nephio Authors
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

	"github.com/nephio-project/porch/api/porch/v1alpha1"
	cachetypes "github.com/nephio-project/porch/pkg/cache/types"
	"github.com/nephio-project/porch/pkg/repository"
	"go.opentelemetry.io/otel/trace"
	"k8s.io/klog/v2"
)

func pkgRevReadFromDB(ctx context.Context, prk repository.PackageRevisionKey, readResources bool) (*dbPackageRevision, error) {
	_, span := tracer.Start(ctx, "dbpackagerevisionsql::pkgRevReadFromDB", trace.WithAttributes())
	defer span.End()

	klog.V(5).Infof("pkgRevReadFromDB: reading package revision %+v", prk)

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
			package_revisions.lifecycle,
			package_revisions.tasks
		FROM package_revisions INNER JOIN packages
			ON package_revisions.k8s_name_space=packages.k8s_name_space AND package_revisions.package_k8s_name=packages.k8s_name
		 INNER JOIN repositories
			ON packages.k8s_name_space=repositories.k8s_name_space AND packages.repo_k8s_name=repositories.k8s_name
		WHERE package_revisions.k8s_name_space=$1 AND package_revisions.k8s_name=$2`

	klog.V(6).Infof("pkgRevReadFromDB: running query %q on package revision %+v", sqlStatement, prk)
	rows, err := GetDB().db.Query(sqlStatement, prk.K8SNS(), prk.K8SName())
	if err != nil {
		klog.Warningf("pkgRevReadFromDB: reading package revision %+v returned err: %q", prk, err)
		return nil, err
	}

	prs, err := pkgRevScanRowsFromDB(ctx, rows)
	if err != nil {
		klog.Warningf("pkgRevReadFromDB: reading package revision %+v returned err: %q", prk, err)
		return nil, err
	}

	if len(prs) == 0 {
		return nil, sql.ErrNoRows
	}

	if len(prs) > 1 {
		err := fmt.Errorf("pkgRevReadFromDB: reading package revision %+v should return 1 package revision, it returned %d package revisions", prk, len(prs))
		klog.Warning(err)
		return nil, err
	}

	readPr := prs[0]

	if !readResources {
		return readPr, nil
	}

	resources, err := pkgRevResourcesReadFromDB(ctx, readPr.Key())
	if err != nil {
		return nil, err
	}

	readPr.resources = resources
	return readPr, err
}

func pkgRevListPRsFromDB(ctx context.Context, filter repository.ListPackageRevisionFilter) ([]*dbPackageRevision, error) {
	_, span := tracer.Start(ctx, "dbrepositorysql::repoDeleteFromDB", trace.WithAttributes())
	defer span.End()

	klog.V(5).Infof("pkgRevListPRsFromDB: listing package revisions for filter %+v", filter)

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
			package_revisions.lifecycle,
			package_revisions.tasks
		FROM package_revisions
		INNER JOIN packages
			ON package_revisions.k8s_name_space=packages.k8s_name_space AND package_revisions.package_k8s_name=packages.k8s_name
		INNER JOIN repositories
			ON packages.k8s_name_space=repositories.k8s_name_space AND packages.repo_k8s_name=repositories.k8s_name
		`

	sqlStatement += prListFilter2WhereClause(filter)

	klog.V(6).Infof("pkgRevListPRsFromDB: running query %q on package revisions with filter %+v", sqlStatement, filter)
	rows, err := GetDB().db.Query(sqlStatement)
	if err != nil {
		klog.Warningf("pkgRevListPRsFromDB: reading package revision list for filter %+v returned err: %q", filter, err)
		return nil, err
	}

	return pkgRevScanRowsFromDB(ctx, rows)
}

func pkgRevReadPRsFromDB(ctx context.Context, pk repository.PackageKey) ([]*dbPackageRevision, error) {
	_, span := tracer.Start(ctx, "dbpackagerevisionsql::pkgRevReadPRsFromDB", trace.WithAttributes())
	defer span.End()

	klog.V(5).Infof("pkgRevReadPRsFromDB: reading package revisions for package %+v", pk)

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
			package_revisions.lifecycle,
			package_revisions.tasks
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

	klog.V(5).Infof("pkgRevReadLatestPRFromDB: reading latest package revision for package %+v", pk)

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
			package_revisions.lifecycle,
			package_revisions.tasks
		FROM package_revisions INNER JOIN packages
			ON package_revisions.k8s_name_space=packages.k8s_name_space AND package_revisions.package_k8s_name=packages.k8s_name
		 INNER JOIN repositories
			ON packages.k8s_name_space=repositories.k8s_name_space AND packages.repo_k8s_name=repositories.k8s_name
		WHERE packages.k8s_name_space=$1 AND packages.k8s_name=$2 AND package_revisions.revision=(
			SELECT MAX(revision) FROM package_revisions
				WHERE package_revisions.k8s_name_space=$1 AND package_revisions.package_k8S_name=$2
			)`

	latestPRList, err := pkgRevReadPRListFromDB(ctx, pk, sqlStatement)
	if err != nil {
		return nil, err
	}

	switch len(latestPRList) {
	case 1:
		return latestPRList[0], nil
	case 0:
		err := fmt.Errorf("latest package revision for package %+v not found in DB", pk)
		klog.Warning(err)
		return nil, err
	default:
		// Multiple drafts with a revision value of 0 are allowed, just return nil with no error
		if latestPRList[0].pkgRevKey.Revision == 0 {
			return nil, nil
		} else {
			err := fmt.Errorf("multiple latest package revisions with revision %d for package %+v found in DB", latestPRList[0].pkgRevKey.Revision, pk)
			klog.Warning(err)
			return nil, err
		}
	}
}

func pkgRevReadPRListFromDB(ctx context.Context, pk repository.PackageKey, sqlStatement string) ([]*dbPackageRevision, error) {
	_, span := tracer.Start(ctx, "dbpackagerevisionsql::pkgRevReadPRsFromDB", trace.WithAttributes())
	defer span.End()

	klog.V(5).Infof("pkgRevReadPRListFromDB: reading package revisions for package %+v", pk)

	klog.V(6).Infof("pkgRevReadPRListFromDB: running query %q on package revisions for package %+v", sqlStatement, pk)
	rows, err := GetDB().db.Query(sqlStatement, pk.K8SNS(), pk.K8SName())
	if err != nil {
		klog.Warningf("pkgRevReadPRListFromDB: query failed for %+v: %q", pk, err)
		return nil, err
	}

	klog.V(5).Infof("pkgRevReadPRListFromDB: query succeeded for %+v", pk)

	prs, err := pkgRevScanRowsFromDB(ctx, rows)

	if err != nil {
		if err == sql.ErrNoRows {
			klog.V(5).Infof("pkgRevReadPRListFromDB: package revisions for package %+v not found in db", pk)
		} else {
			klog.Warningf("pkgRevReadPRListFromDB: reading package revisions for package %+v returned err: %q", pk, err)
		}
		return nil, err
	}

	return prs, nil
}

func pkgRevScanRowsFromDB(ctx context.Context, rows *sql.Rows) ([]*dbPackageRevision, error) {
	_, span := tracer.Start(ctx, "dbpackagesql::pkgScanRowsFromDB", trace.WithAttributes())
	defer span.End()

	defer rows.Close()

	klog.V(5).Info("pkgRevScanRowsFromDB: scanning rows returned from query")

	var dbPkgRevs []*dbPackageRevision

	for rows.Next() {
		var pkgRev dbPackageRevision
		var pkgK8SName, prK8SName, metaAsJSON, specAsJSON, tasks string

		err := rows.Scan(
			&pkgRev.pkgRevKey.PkgKey.RepoKey.Namespace,
			&pkgRev.pkgRevKey.PkgKey.RepoKey.Name,
			&pkgRev.pkgRevKey.PkgKey.RepoKey.Path,
			&pkgRev.pkgRevKey.PkgKey.RepoKey.PlaceholderWSname,
			&pkgK8SName,
			&pkgRev.pkgRevKey.PkgKey.Path,
			&prK8SName,
			&pkgRev.pkgRevKey.Revision,
			&metaAsJSON,
			&specAsJSON,
			&pkgRev.updated,
			&pkgRev.updatedBy,
			&pkgRev.lifecycle,
			&tasks)

		if err != nil {
			klog.Warningf("pkgRevScanRowsFromDB: scanning rows failed: %q", err)
			return nil, err
		}

		pkgRev.repo = cachetypes.CacheInstance.GetRepository(pkgRev.pkgRevKey.PkgKey.RepoKey).(*dbRepository)
		pkgRev.pkgRevKey.PkgKey.Package = repository.K8SName2PkgName(pkgK8SName)
		pkgRev.pkgRevKey.WorkspaceName = repository.K8SName2PkgRevWSName(pkgK8SName, prK8SName)
		setValueFromJSON(metaAsJSON, &pkgRev.meta)
		setValueFromJSON(specAsJSON, &pkgRev.spec)
		setValueFromJSON(tasks, &pkgRev.tasks)

		dbPkgRevs = append(dbPkgRevs, &pkgRev)
	}

	return dbPkgRevs, nil
}

func pkgRevWriteToDB(ctx context.Context, pr *dbPackageRevision) error {
	_, span := tracer.Start(ctx, "dbpackagerevisionsql::pkgRevWriteToDB", trace.WithAttributes())
	defer span.End()

	klog.V(5).Infof("pkgRevWriteToDB: writing package revision %+v", pr.Key())

	sqlStatement := `
        INSERT INTO package_revisions (k8s_name_space, k8s_name, package_k8s_name, revision, meta, spec, updated, updatedby, lifecycle, tasks)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`

	klog.V(6).Infof("pkgRevWriteToDB: running query %q on package revision %+v", sqlStatement, pr)
	prk := pr.Key()
	if _, err := GetDB().db.Exec(
		sqlStatement,
		prk.K8SNS(), prk.K8SName(),
		prk.PKey().K8SName(), prk.Revision, valueAsJSON(pr.meta), valueAsJSON(pr.spec), pr.updated, pr.updatedBy, pr.lifecycle, valueAsJSON(pr.tasks)); err == nil {
		klog.V(5).Infof("pkgRevWriteToDB: query succeeded, row created")
	} else {
		klog.Warningf("pkgRevWriteToDB: query failed for %+v %q", pr.Key(), err)
		return err
	}

	if err := pkgRevResourcesWriteToDB(ctx, pr); err == nil {
		klog.V(5).Infof("pkgRevWriteToDB: resources written to DB")
		return nil
	} else {
		klog.Warningf("pkgRevWriteToDB: resource write to DB failed: %q", err)
		return err
	}
}

func pkgRevUpdateDB(ctx context.Context, pr *dbPackageRevision, updateResources bool) error {
	_, span := tracer.Start(ctx, "dbpackagerevisionsql::pkgRevUpdateDB", trace.WithAttributes())
	defer span.End()

	klog.V(5).Infof("pkgRevUpdateDB: updating package revision %+v", pr.Key())

	sqlStatement := `
        UPDATE package_revisions SET package_k8s_name=$3, revision=$4, meta=$5, spec=$6, updated=$7, updatedby=$8, lifecycle=$9, tasks=$10
        WHERE k8s_name_space=$1 AND k8s_name=$2`

	klog.V(6).Infof("pkgRevUpdateDB: running query %q on package revision %+v", sqlStatement, pr)
	prk := pr.Key()
	result, err := GetDB().db.Exec(
		sqlStatement,
		prk.K8SNS(), prk.K8SName(),
		prk.PKey().K8SName(), prk.Revision, valueAsJSON(pr.meta), valueAsJSON(pr.spec), pr.updated, pr.updatedBy, pr.lifecycle, valueAsJSON(pr.tasks))

	if err == nil {
		if rowsAffected, _ := result.RowsAffected(); rowsAffected == 1 {
			klog.V(5).Infof("pkgRevUpdateDB: query succeeded, for %+v, row updated", pr.Key())
		} else {
			err = fmt.Errorf("pkgRevUpdateDB: update failed for %+v, no rows or multiple rows found for updating", pr.Key())
		}
	}

	if err != nil {
		klog.Warningf("pkgRevUpdateDB:: query failed for %+v: %q", pr.Key(), err)
		return err
	}

	if !updateResources {
		return nil
	}

	if err := pkgRevResourcesWriteToDB(ctx, pr); err == nil {
		klog.V(5).Infof("pkgRevUpdateDB: resources written to DB for %+v", pr.Key())
		return nil
	} else {
		klog.Warningf("pkgRevUpdateDB: resources write to DB failed for %+v: %q", pr.Key(), err)
		return err
	}
}

func pkgRevDeleteFromDB(ctx context.Context, prk repository.PackageRevisionKey) error {
	_, span := tracer.Start(ctx, "dbpackagerevisionsql::pkgRevDeleteFromDB", trace.WithAttributes())
	defer span.End()

	klog.V(5).Infof("pkgRevDeleteFromDB: deleting package revision %+v", prk)

	if err := pkgRevResourcesDeleteFromDB(ctx, prk); err == nil {
		klog.V(5).Infof("pkgRevDeleteFromDB: resources deleted from DB for %+v", prk)
	} else {
		klog.Warningf("pkgRevDeleteFromDB: resources delete from DB failed: %+v", err)
		return err
	}

	sqlStatement := `DELETE FROM package_revisions WHERE k8s_name_space=$1 AND k8s_name=$2`

	klog.V(6).Infof("pkgRevDeleteFromDB: running query %q on package revision %+v", sqlStatement, prk)
	_, err := GetDB().db.Exec(sqlStatement, prk.K8SNS(), prk.K8SName())

	if err == nil {
		klog.V(5).Infof("pkgRevDeleteFromDB: deleted package revision %+v", prk)
	} else {
		klog.Warningf("pkgRevDeleteFromDB: deletion of package revision %+v failed: %q", prk, err)
	}

	return err
}

func prListFilter2WhereClause(filter repository.ListPackageRevisionFilter) string {
	whereStatement := ""

	repoKey := filter.Key.RKey()
	whereStatement, first := prListFilter2SubClauseStr(whereStatement, repoKey.Namespace, "repositories.k8s_name_space", true)
	whereStatement, first = prListFilter2SubClauseStr(whereStatement, repoKey.Name, "repositories.k8s_name", first)
	whereStatement, first = prListFilter2SubClauseStr(whereStatement, repoKey.Path, "repositories.directory", first)
	whereStatement, first = prListFilter2SubClauseStr(whereStatement, repoKey.PlaceholderWSname, "repositories.default_ws_name", first)

	pkgKey := filter.Key.PKey()
	whereStatement, first = prListFilter2SubClauseStr(whereStatement, pkgKey.K8SName(), "packages.k8s_name", first)
	whereStatement, first = prListFilter2SubClauseStr(whereStatement, pkgKey.Path, "packages.package_path", first)

	prKey := filter.Key
	whereStatement, first = prListFilter2SubClauseStr(whereStatement, prKey.K8SName(), "package_revisions.k8s_name", first)
	whereStatement, first = prListFilter2SubClauseInt(whereStatement, prKey.Revision, "package_revisions.revision", first)

	whereStatement, _ = prListFilter2SubClauseLifecycle(whereStatement, filter.Lifecycles, "package_revisions.lifecycle", first)

	if whereStatement == "" {
		return whereStatement
	} else {
		return "WHERE\n" + whereStatement
	}
}

func prListFilter2SubClauseStr(whereStatement, filterField, column string, first bool) (string, bool) {
	if filterField == "" {
		return whereStatement, first
	}

	subClause := fmt.Sprintf("%s='%s'\n", column, filterField)

	if first {
		return whereStatement + subClause, false
	} else {
		return whereStatement + "AND " + subClause, false
	}
}

func prListFilter2SubClauseInt(whereStatement string, filterField int, column string, first bool) (string, bool) {
	if filterField == 0 {
		return whereStatement, first
	}

	subClause := fmt.Sprintf("%s=%d\n", column, filterField)

	if first {
		return whereStatement + subClause, false
	} else {
		return whereStatement + "AND " + subClause, false
	}
}

func prListFilter2SubClauseLifecycle(whereStatement string, filterField []v1alpha1.PackageRevisionLifecycle, column string, first bool) (string, bool) {
	if len(filterField) == 0 {
		return whereStatement, first
	}

	subClause := "("
	for i, lifecycle := range filterField {
		if i == 0 {
			subClause = subClause + fmt.Sprintf("%s='%s'", column, lifecycle)
		} else {
			subClause = subClause + fmt.Sprintf(" OR %s='%s'", column, lifecycle)
		}
	}
	subClause += ")"

	if first {
		return whereStatement + subClause, false
	} else {
		return whereStatement + "AND " + subClause, false
	}
}
