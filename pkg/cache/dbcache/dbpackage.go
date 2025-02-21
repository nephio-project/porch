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
	"time"

	"github.com/nephio-project/porch/api/porch/v1alpha1"
	"github.com/nephio-project/porch/pkg/repository"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ repository.Package = &dbPackage{}

type dbPackage struct {
	pkgKey    repository.PackageKey
	meta      metav1.ObjectMeta
	spec      v1alpha1.PackageSpec
	updated   time.Time
	updatedBy string
}

func (p *dbPackage) KubeObjectName() string {
	return p.Key().NonNSString()
}

func (p *dbPackage) KubeObjectNamespace() string {
	return p.Key().Namespace
}

func (p *dbPackage) UID() types.UID {
	return generateUid("packages.", p.KubeObjectNamespace(), p.KubeObjectName())
}

func (p *dbPackage) Key() repository.PackageKey {
	return p.pkgKey
}

func (p *dbPackage) savePackage() (*dbPackage, error) {
	_, err := pkgReadFromDB(p.Key())
	if err == nil {
		return p, pkgUpdateDB(p)
	} else if err != sql.ErrNoRows {
		return p, err
	}

	p.meta = metav1.ObjectMeta{
		Name:      p.KubeObjectName(),
		Namespace: p.KubeObjectNamespace(),
	}

	p.spec = v1alpha1.PackageSpec{
		PackageName:    p.pkgKey.Package,
		RepositoryName: p.Key().Repository,
	}

	return p, pkgWriteToDB(p)
}

func (p *dbPackage) GetPackage() *v1alpha1.PorchPackage {
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

func (p *dbPackage) savePackageRevision(d *dbPackageRevision) (*dbPackageRevision, error) {
	return d.savePackageRevision()
}

func (p *dbPackage) DeletePackageRevision(ctx context.Context, old repository.PackageRevision) error {
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

func (p *dbPackage) GetLatestRevision() string {
	return pkgRevReadLatestPRFromDB(p.Key()).ResourceVersion()
}

func (p *dbPackage) Delete() error {
	dbPkgRevs, err := pkgRevReadPRsFromDB(p.Key())
	if err != nil {
		return err
	}

	for _, pkgRev := range dbPkgRevs {
		if err := pkgRev.Delete(); err != nil {
			return err
		}
	}

	return pkgDeleteFromDB(p.Key())
}
