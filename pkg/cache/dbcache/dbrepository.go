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
	"errors"
	"fmt"
	"time"

	"github.com/nephio-project/porch/api/porch/v1alpha1"
	configapi "github.com/nephio-project/porch/api/porchconfig/v1alpha1"
	"github.com/nephio-project/porch/pkg/externalrepo"
	externalrepotypes "github.com/nephio-project/porch/pkg/externalrepo/types"
	"github.com/nephio-project/porch/pkg/repository"
	"go.opentelemetry.io/otel/trace"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
)

var _ repository.Repository = &dbRepository{}

type dbRepository struct {
	repoKey      repository.RepositoryKey
	meta         *metav1.ObjectMeta
	spec         *configapi.Repository
	externalRepo repository.Repository
	updated      time.Time
	updatedBy    string
	deployment   bool
}

func (r *dbRepository) KubeObjectName() string {
	return r.Key().NonNSString()
}

func (r *dbRepository) KubeObjectNamespace() string {
	return r.repoKey.Namespace
}

func (r *dbRepository) UID() types.UID {
	return generateUid("repositories.", r.KubeObjectNamespace(), r.KubeObjectName())
}

func (r *dbRepository) Key() repository.RepositoryKey {
	return r.repoKey
}

func (r *dbRepository) OpenRepository(ctx context.Context, externalRepoOptions externalrepotypes.ExternalRepoOptions) error {
	_, span := tracer.Start(ctx, "dbCache::OpenRepository", trace.WithAttributes())
	defer span.End()

	klog.Infof("DB Repo OpenRepository: %q", r.Key().String())

	externalRepo, err := externalrepo.CreateRepositoryImpl(ctx, r.spec, externalRepoOptions)
	if err != nil {
		return err
	}

	r.externalRepo = externalRepo

	if _, err = repoReadFromDB(r.Key()); err == nil {
		return nil
	} else if err != sql.ErrNoRows {
		klog.Infof("DB Repo OpenRepository: %q DB read failed with error %q", r.Key().String(), err)
		return err
	}

	if err = repoWriteToDB(r); err != nil {
		klog.Infof("DB Repo OpenRepository: %q DB write failed with error %q", r.Key().String(), err)
		return err
	}

	return nil
}

func (r *dbRepository) Close() error {
	klog.Infof("DB Repo close: %q", r.Key().String())

	dbPkgs, err := pkgReadPkgsFromDB(r.Key())
	if err != nil {
		return err
	}

	for _, pkg := range dbPkgs {
		if err := pkg.Delete(); err != nil {
			return err
		}
	}

	err = repoDeleteFromDB(r.Key())
	if err != nil {
		return err
	}

	return r.externalRepo.Close()
}

func (r *dbRepository) ListPackageRevisions(ctx context.Context, filter repository.ListPackageRevisionFilter) ([]repository.PackageRevision, error) {
	klog.Infof("DB Repo ListPackageRevisions: %q", r.Key().String())

	pkgs, err := pkgReadPkgsFromDB(r.Key())
	if err != nil {
		return nil, err
	}

	var foundPkgRevs []*dbPackageRevision

	for _, pkg := range pkgs {
		pkgRevs, err := pkgRevReadPRsFromDB(pkg.Key())
		if err != nil {
			return nil, err
		}

		foundPkgRevs = append(foundPkgRevs, pkgRevs...)
	}

	genericPkgRevs := make([]repository.PackageRevision, len(foundPkgRevs))
	for i, pkgRev := range foundPkgRevs {
		genericPkgRevs[i] = repository.PackageRevision(pkgRev)
	}

	return genericPkgRevs, nil
}

func (r *dbRepository) CreatePackageRevisionDraft(ctx context.Context, newPR *v1alpha1.PackageRevision) (repository.PackageRevisionDraft, error) {
	klog.Infof("DB Repo CreatePackageRevisionDraft: %q", r.Key().String())

	_, span := tracer.Start(ctx, "dbRepository::CreatePackageRevisionDraft", trace.WithAttributes())
	defer span.End()

	dbPkgRev := &dbPackageRevision{
		repo:       r,
		definition: newPR,
		pkgRevKey: repository.PackageRevisionKey{
			Namespace:     r.repoKey.Namespace,
			Repository:    r.repoKey.Repository,
			Package:       newPR.Spec.PackageName,
			Revision:      "",
			WorkspaceName: newPR.Spec.WorkspaceName,
		},
		meta:      newPR.ObjectMeta,
		spec:      newPR.Spec,
		lifecycle: v1alpha1.PackageRevisionLifecycleDraft,
		updated:   time.Now(),
		updatedBy: getCurrentUser(),
		tasks:     newPR.Spec.Tasks,
	}

	prDraft, err := r.savePackageRevision(ctx, dbPkgRev, "")

	return repository.PackageRevisionDraft(prDraft), err
}

func (r *dbRepository) DeletePackageRevision(ctx context.Context, old repository.PackageRevision) error {
	klog.Infof("DB Repo DeletePackageRevision: %q", r.Key().String())

	pk := repository.PackageKey{
		Namespace:  old.Key().Namespace,
		Repository: old.Key().Repository,
		Package:    old.Key().Package,
	}

	foundPkg, err := pkgReadFromDB(pk)
	if err != nil {
		return err
	}

	if err := foundPkg.DeletePackageRevision(ctx, old); err != nil && err != sql.ErrNoRows {
		return nil
	}

	foundPRs, err := pkgRevReadPRsFromDB(foundPkg.Key())
	if err != nil {
		return err
	}

	if len(foundPRs) != 0 {
		return nil
	}

	return pkgDeleteFromDB(pk)
}

func (r *dbRepository) UpdatePackageRevision(ctx context.Context, updatePR repository.PackageRevision) (repository.PackageRevisionDraft, error) {
	klog.Infof("DB Repo UpdatePackageRevision: %q", r.Key().String())

	updatePkgRev, ok := updatePR.(*dbPackageRevision)
	if !ok {
		return nil, fmt.Errorf("cannot update DB package revision %T", updatePR)
	}

	if err := updatePkgRev.UpdatePackageRevision(); err != nil {
		return nil, err
	}

	return &dbPackageRevision{
		repo:      r,
		pkgRevKey: updatePR.Key(),
		lifecycle: updatePkgRev.lifecycle,
		updated:   time.Now(),
		updatedBy: getCurrentUser(),
		tasks:     nil,
		resources: updatePkgRev.resources,
	}, nil
}

func (r *dbRepository) ListPackages(ctx context.Context, filter repository.ListPackageFilter) ([]repository.Package, error) {
	klog.Infof("DB Repo ListPackages: %q", r.Key().String())

	dbPkgs, err := pkgReadPkgsFromDB(r.Key())
	if err != nil {
		return nil, err
	}

	genericPkgs := make([]repository.Package, len(dbPkgs))
	for i, pkg := range dbPkgs {
		genericPkgs[i] = repository.Package(&pkg)
	}

	return genericPkgs, nil
}

func (r *dbRepository) CreatePackage(ctx context.Context, obj *v1alpha1.PorchPackage) (repository.Package, error) {
	klog.Infof("DB Repo CreatePackage: %q", r.Key().String())
	return nil, nil
}

func (r *dbRepository) DeletePackage(ctx context.Context, old repository.Package) error {
	klog.Infof("DB Repo DeletePackage: %q", r.Key().String())

	foundPackage, err := pkgReadFromDB(old.Key())
	if err != nil {
		if err == sql.ErrNoRows {
			return nil
		} else {
			return err
		}
	}

	foundPRs, err := pkgRevReadPRsFromDB(foundPackage.Key())
	if err != nil {
		return err
	}

	if len(foundPRs) != 0 {
		errMsg := fmt.Sprintf("cannot delete package %q, it has %d package revisions", old.Key().String(), len(foundPRs))
		return errors.New(errMsg)
	}

	return pkgDeleteFromDB(old.Key())
}

func (r *dbRepository) Version(ctx context.Context) (string, error) {
	klog.Infof("DB Repo version: %q", r.Key().String())
	return "Undefined", nil
}

func (r *dbRepository) ClosePackageRevisionDraft(ctx context.Context, prd repository.PackageRevisionDraft, version string) (repository.PackageRevision, error) {
	_, span := tracer.Start(ctx, "dbRepository::ClosePackageRevisionDraft", trace.WithAttributes())
	defer span.End()

	pr, err := r.savePackageRevision(ctx, prd, version)

	return repository.PackageRevision(pr), err
}

func (r *dbRepository) PushPackageRevision(ctx context.Context, pr repository.PackageRevision) error {
	return fmt.Errorf("dbRepository:PushPackageRevision: function should not be invoked on caches")
}

func (r *dbRepository) savePackageRevision(ctx context.Context, prd repository.PackageRevisionDraft, _ string) (*dbPackageRevision, error) {
	_, span := tracer.Start(ctx, "dbRepository::savePackageRevision", trace.WithAttributes())
	defer span.End()

	d := prd.(*dbPackageRevision)

	dbPkg, err := pkgReadFromDB(d.Key().PackageKey())
	if err != nil {
		if err != sql.ErrNoRows {
			return nil, err
		}

		dbPkg = dbPackage{
			repo:      r,
			pkgKey:    d.Key().PackageKey(),
			updated:   d.updated,
			updatedBy: d.updatedBy,
		}

		if _, err := dbPkg.savePackage(); err != nil {
			return nil, err
		}
	}

	pkgRev, err := dbPkg.savePackageRevision(d)
	if err != nil {
		return nil, err
	}

	r.updated = d.updated
	r.updatedBy = d.updatedBy
	if err = repoUpdateDB(r); err != nil {
		return nil, err
	}

	return pkgRev, nil
}

func (r *dbRepository) Refresh(ctx context.Context) error {
	return nil
}
