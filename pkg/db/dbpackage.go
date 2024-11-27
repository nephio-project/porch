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
	"context"
	"database/sql"
	"strconv"
	"strings"
	"time"

	"github.com/nephio-project/porch/api/porch/v1alpha1"
	"github.com/nephio-project/porch/pkg/repository"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ repository.Package = &dbPackage{}

type dbPackage struct {
	pkgKey    repository.PackageKey
	updated   time.Time
	updatedBy string
}

func (p dbPackage) KubeObjectName() string {
	return p.Key().String()
}

func (p dbPackage) KubeObjectNamespace() string {
	return p.Key().Namespace
}

func (p dbPackage) UID() types.UID {
	return generateUid("packages.", p.KubeObjectNamespace(), p.KubeObjectName())
}

func (p dbPackage) Key() repository.PackageKey {
	return p.pkgKey
}

func (p dbPackage) createPackage() (*dbPackage, error) {
	_, err := pkgReadFromDB(p.Key())
	if err == nil {
		return &p, pkgUpdateDB(p)
	} else if err != sql.ErrNoRows {
		return &p, err
	}

	return &p, pkgWriteToDB(p)
}

func (p dbPackage) GetPackage() *v1alpha1.PorchPackage {
	key := p.Key()

	return &v1alpha1.PorchPackage{
		TypeMeta: metav1.TypeMeta{
			Kind:       "PorchPackage",
			APIVersion: v1alpha1.SchemeGroupVersion.Identifier(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:            p.KubeObjectName(),
			Namespace:       p.Key().Namespace,
			UID:             p.UID(),
			ResourceVersion: "undefined",
			CreationTimestamp: metav1.Time{
				Time: p.updated,
			},
		},
		Spec: v1alpha1.PackageSpec{
			PackageName:    key.Package,
			RepositoryName: key.Repository,
		},
		Status: v1alpha1.PackageStatus{
			LatestRevision: p.GetLatestRevision(),
		},
	}
}

func (p dbPackage) createPackageRevision(d *dbPackageDraft) (*dbPackageRevision, error) {
	dbPkgRev := dbPackageRevision{
		pkgRevKey: repository.PackageRevisionKey{
			Namespace:     p.Key().Namespace,
			Repository:    p.Key().Repository,
			Package:       p.Key().Package,
			Revision:      d.revision,
			WorkspaceName: d.workspaceName,
		},
		lifecycle: d.lifecycle,
		updated:   d.updated,
		updatedBy: d.updatedBy,
	}

	return dbPkgRev.createPackageRevision()
}

func (p dbPackage) DeletePackageRevision(ctx context.Context, old repository.PackageRevision) error {
	if err := pkgRevDeleteFromDB(old.Key()); err != nil && err != sql.ErrNoRows {
		return err
	}

	prSlice, err := pkgRevReadPRsFromDB(p.Key())
	if err != nil {
		return err
	}

	if len(prSlice) == 0 {
		return pkgDeleteFromDB(p.Key())
	}

	return nil
}

func (p dbPackage) GetLatestRevision() string {
	latestRevision := 0

	packageRevisions, err := pkgRevReadPRsFromDB(p.Key())
	if err != nil {
		return ""
	}

	for _, pr := range packageRevisions {
		thisRevision, _ := strconv.Atoi(strings.Replace(pr.Key().Revision, "v", "", 1))
		if thisRevision > latestRevision {
			latestRevision = thisRevision
		}
	}

	return strconv.Itoa(latestRevision)
}

func (p dbPackage) Close() error {
	dbPkgRevs, err := pkgRevReadPRsFromDB(p.Key())
	if err != nil {
		return err
	}

	for _, pkgRev := range dbPkgRevs {
		if err := pkgRev.Close(); err != nil {
			return err
		}
	}

	return pkgDeleteFromDB(p.Key())
}
