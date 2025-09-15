// Copyright 2025 The Nephio Authors
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
	"fmt"

	"github.com/nephio-project/porch/api/porch/v1alpha1"
	"github.com/nephio-project/porch/pkg/repository"
)

func pkgListFilter2WhereClause(filter repository.ListPackageFilter) string {
	whereStatement := ""

	repoKey := filter.Key.RKey()
	whereStatement, first := filter2SubClauseStr(whereStatement, repoKey.Namespace, "repositories.k8s_name_space", true)
	whereStatement, first = filter2SubClauseStr(whereStatement, repoKey.Name, "repositories.k8s_name", first)
	whereStatement, first = filter2SubClauseStr(whereStatement, repoKey.Path, "repositories.directory", first)
	whereStatement, first = filter2SubClauseStr(whereStatement, repoKey.PlaceholderWSname, "repositories.default_ws_name", first)

	pkgKey := filter.Key
	whereStatement, first = filter2SubClauseStr(whereStatement, pkgKey.K8SName(), "packages.k8s_name", first)
	whereStatement, _ = filter2SubClauseStr(whereStatement, pkgKey.Path, "packages.package_path", first)

	if whereStatement == "" {
		return whereStatement
	} else {
		return "WHERE\n" + whereStatement
	}
}

func prListFilter2WhereClause(filter repository.ListPackageRevisionFilter) string {
	whereStatement := ""

	repoKey := filter.Key.RKey()
	whereStatement, first := filter2SubClauseStr(whereStatement, repoKey.Namespace, "repositories.k8s_name_space", true)
	whereStatement, first = filter2SubClauseStr(whereStatement, repoKey.Name, "repositories.k8s_name", first)
	whereStatement, first = filter2SubClauseStr(whereStatement, repoKey.Path, "repositories.directory", first)
	whereStatement, first = filter2SubClauseStr(whereStatement, repoKey.PlaceholderWSname, "repositories.default_ws_name", first)

	pkgKey := filter.Key.PKey()
	whereStatement, first = filter2SubClauseStr(whereStatement, pkgKey.K8SName(), "packages.k8s_name", first)
	whereStatement, first = filter2SubClauseStr(whereStatement, pkgKey.Path, "packages.package_path", first)

	prKey := filter.Key
	whereStatement, first = filter2SubClauseStr(whereStatement, prKey.K8SName(), "package_revisions.k8s_name", first)
	whereStatement, first = filter2SubClauseInt(whereStatement, prKey.Revision, "package_revisions.revision", first)
	whereStatement, first = filter2SubClauseWorkspace(whereStatement, prKey.WorkspaceName, "package_revisions.k8s_name", first)

	whereStatement, _ = filter2SubClauseLifecycle(whereStatement, filter.Lifecycles, "package_revisions.lifecycle", first)

	if whereStatement == "" {
		return whereStatement
	} else {
		return "WHERE\n" + whereStatement
	}
}

func filter2SubClauseStr(whereStatement, filterField, column string, first bool) (string, bool) {
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

func filter2SubClauseInt(whereStatement string, filterField int, column string, first bool) (string, bool) {
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

func filter2SubClauseWorkspace(whereStatement string, filterField string, column string, first bool) (string, bool) {
	if filterField == "" {
		return whereStatement, first
	}

	subClause := fmt.Sprintf("%s LIKE '%%.%s'\n", column, filterField)

	if first {
		return whereStatement + subClause, false
	} else {
		return whereStatement + "AND " + subClause, false
	}
}

func filter2SubClauseLifecycle(whereStatement string, filterField []v1alpha1.PackageRevisionLifecycle, column string, first bool) (string, bool) {
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
