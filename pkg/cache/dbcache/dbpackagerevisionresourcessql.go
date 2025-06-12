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

func pkgRevResourceReadFromDB(ctx context.Context, prk repository.PackageRevisionKey, resKey string) (string, string, error) {
	_, span := tracer.Start(ctx, "dbpackagerevisionresourcessql::pkgRevResourceReadFromDB", trace.WithAttributes())
	defer span.End()

	klog.Infof("pkgRevResourceReadFromDB: reading package revision resource %q:%s", prk, resKey)

	sqlStatement := `SELECT resource_value FROM resources WHERE k8s_name_space=$1 AND k8s_name=$2 AND resource_key=$3`

	var resVal string

	err := GetDB().db.QueryRow(sqlStatement, prk.K8SNS(), prk.K8SName(), resKey).Scan(&resVal)

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

func pkgRevResourcesReadFromDB(ctx context.Context, prk repository.PackageRevisionKey) (map[string]string, error) {
	_, span := tracer.Start(ctx, "dbpackagerevisionresourcessql::pkgRevResourcesReadFromDB", trace.WithAttributes())
	defer span.End()

	sqlStatement := `SELECT resource_key, resource_value FROM resources WHERE k8s_name_space=$1 AND k8s_name=$2`

	resources := make(map[string]string)

	klog.Infof("pkgRevResourcesReadFromDB: running query [%q] on %q", sqlStatement, prk)

	rows, err := GetDB().db.Query(sqlStatement, prk.K8SNS(), prk.K8SName())
	if err != nil {
		klog.Infof("pkgRevResourcesReadFromDB: query failed for %q: %q", prk, err)
		return nil, err
	}
	defer rows.Close()

	klog.Infof("pkgRevResourcesReadFromDB: query succeeded for %q", prk)

	for rows.Next() {
		var resKey, resVal string

		if err := rows.Scan(&resKey, &resVal); err != nil {
			return nil, err
		}
		resources[resKey] = resVal
	}

	return resources, nil
}

func pkgRevResourceWriteToDB(ctx context.Context, prk repository.PackageRevisionKey, resKey, resVal string) error {
	_, span := tracer.Start(ctx, "dbpackagerevisionresourcessql::pkgRevResourceWriteToDB", trace.WithAttributes())
	defer span.End()

	klog.Infof("pkgRevResourceWriteToDB: writing package revision resource %s=%s for %q", resKey, resVal, prk)

	sqlStatement := `
		INSERT INTO resources (k8s_name_space, k8s_name, revision, resource_key, resource_value)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (k8s_name_space, k8s_name, resource_key) 
			DO UPDATE SET resource_value = EXCLUDED.resource_value`

	klog.Infof("pkgRevResourceWriteToDB: running query [%q] on repository (%#v)", sqlStatement, prk)

	if _, err := GetDB().db.Exec(sqlStatement, prk.K8SNS(), prk.K8SName(), prk.Revision, resKey, resVal); err == nil {
		klog.Infof("pkgRevResourceWriteToDB: query succeeded, row created/updated")
		return nil
	} else {
		klog.Infof("pkgRevResourceWriteToDB: query failed %q", err)
		return err
	}
}

func pkgRevResourcesWriteToDB(ctx context.Context, pr *dbPackageRevision) error {
	_, span := tracer.Start(ctx, "dbpackagerevisionresourcessql::pkgRevResourcesWriteToDB", trace.WithAttributes())
	defer span.End()

	if err := pkgRevResourcesDeleteFromDB(ctx, pr.Key()); err != nil {
		return err
	}

	if len(pr.resources) == 0 {
		klog.Infof("pkgRevResourcesWriteToDB: pr %+v has no resources", pr.Key())
		return nil
	}

	klog.Infof("pkgRevResourcesWriteToDB: writing package revision resources for %q", pr.Key())

	for resourceKey, resourceValue := range pr.resources {
		if err := pkgRevResourceWriteToDB(ctx, pr.Key(), resourceKey, resourceValue); err != nil {
			return err
		}
	}

	klog.Infof("pkgRevResourceWriteToDB: query succeeded, row created/updated")
	return nil
}

func pkgRevResourcesDeleteFromDB(ctx context.Context, prk repository.PackageRevisionKey) error {
	_, span := tracer.Start(ctx, "dbpackagerevisionresourcessql::pkgRevResourcesDeleteFromDB", trace.WithAttributes())
	defer span.End()

	klog.Infof("pkgRevResourcesDeleteFromDB: deleting package revision %q", prk)

	sqlStatement := `DELETE FROM resources WHERE k8s_name_space=$1 AND k8s_name=$2`

	_, err := GetDB().db.Exec(sqlStatement, prk.K8SNS(), prk.K8SName())

	if err == nil {
		klog.Infof("pkgRevResourcesDeleteFromDB: deleted package revision %q", prk)
	} else {
		klog.Infof("pkgRevResourcesDeleteFromDB: deletion of package revision %q failed: %q", prk, err)
	}

	return err
}

func pkgRevResourceDeleteFromDB(ctx context.Context, prk repository.PackageRevisionKey, resKey string) error {
	_, span := tracer.Start(ctx, "dbpackagerevisionresourcessql::pkgRevResourceDeleteFromDB", trace.WithAttributes())
	defer span.End()

	klog.Infof("pkgRevResourceDeleteFromDB: deleting package revision %q", prk)

	sqlStatement := `DELETE FROM resources WHERE k8s_name_space=$1 AND k8s_name=$2 AND resource_key=$3`

	_, err := GetDB().db.Exec(sqlStatement, prk.K8SNS(), prk.K8SName(), resKey)

	if err == nil {
		klog.Infof("pkgRevResourceDeleteFromDB: deleted package revision %q", prk)
	} else {
		klog.Infof("pkgRevResourceDeleteFromDB: deletion of package revision %q failed: %q", prk, err)
	}

	return err
}
