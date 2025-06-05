// Copyright 2024.2025 The Nephio Authors
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
	"github.com/nephio-project/porch/pkg/util"
	"go.opentelemetry.io/otel/trace"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
)

var _ repository.Repository = &dbRepository{}

type dbRepository struct {
	repoKey        repository.RepositoryKey
	meta           *metav1.ObjectMeta
	spec           *configapi.Repository
	externalRepo   repository.Repository
	repositorySync *repositorySync
	updated        time.Time
	updatedBy      string
	deployment     bool
}

func (r *dbRepository) KubeObjectName() string {
	return r.Key().Name
}

func (r *dbRepository) KubeObjectNamespace() string {
	return r.repoKey.Namespace
}

func (r *dbRepository) UID() types.UID {
	return util.GenerateUid("repositories.", r.KubeObjectNamespace(), r.KubeObjectName())
}

func (r *dbRepository) Key() repository.RepositoryKey {
	return r.repoKey
}

func (r *dbRepository) OpenRepository(ctx context.Context, externalRepoOptions externalrepotypes.ExternalRepoOptions) error {
	ctx, span := tracer.Start(ctx, "dbRepository::OpenRepository", trace.WithAttributes())
	defer span.End()

	klog.Infof("DB Repo OpenRepository: %q", r.Key().String())

	externalRepo, err := externalrepo.CreateRepositoryImpl(ctx, r.spec, externalRepoOptions)
	if err != nil {
		return err
	}

	r.externalRepo = externalRepo

	if _, err = repoReadFromDB(ctx, r.Key()); err == nil {
		return nil
	} else if err != sql.ErrNoRows {
		klog.Infof("DB Repo OpenRepository: %q DB read failed with error %q", r.Key().String(), err)
		return err
	}

	if err = repoWriteToDB(ctx, r); err != nil {
		klog.Infof("DB Repo OpenRepository: %q DB write failed with error %q", r.Key().String(), err)
		return err
	}

	return nil
}

func (r *dbRepository) Close(ctx context.Context) error {
	ctx, span := tracer.Start(ctx, "dbRepository::Close", trace.WithAttributes())
	defer span.End()

	klog.Infof("DB Repo close: %q", r.Key().String())

	r.repositorySync.stop()

	dbPkgs, err := pkgReadPkgsFromDB(ctx, r.Key())
	if err != nil {
		return err
	}

	for _, pkg := range dbPkgs {
		if err := pkg.Delete(ctx); err != nil {
			return err
		}
	}

	err = repoDeleteFromDB(ctx, r.Key())
	if err != nil {
		return err
	}

	return r.externalRepo.Close(ctx)
}

func (r *dbRepository) ListPackageRevisions(ctx context.Context, filter repository.ListPackageRevisionFilter) ([]repository.PackageRevision, error) {
	ctx, span := tracer.Start(ctx, "dbRepository::ListPackageRevisions", trace.WithAttributes())
	defer span.End()

	klog.Infof("DB Repo ListPackageRevisions: %q", r.Key().String())

	pkgs, err := pkgReadPkgsFromDB(ctx, r.Key())
	if err != nil {
		return nil, err
	}

	var foundPkgRevs []*dbPackageRevision

	for _, pkg := range pkgs {
		pkgRevs, err := pkgRevReadPRsFromDB(ctx, pkg.Key())
		if err != nil {
			return nil, err
		}

		for _, pkgRev := range pkgRevs {
			if filter.Matches(ctx, pkgRev) {
				foundPkgRevs = append(foundPkgRevs, pkgRev)
			}
		}
	}

	genericPkgRevs := make([]repository.PackageRevision, len(foundPkgRevs))
	for i, pkgRev := range foundPkgRevs {
		genericPkgRevs[i] = repository.PackageRevision(pkgRev)
	}

	return genericPkgRevs, nil
}

func (r *dbRepository) CreatePackageRevisionDraft(ctx context.Context, newPR *v1alpha1.PackageRevision) (repository.PackageRevisionDraft, error) {
	ctx, span := tracer.Start(ctx, "dbRepository::CreatePackageRevisionDraft", trace.WithAttributes())
	defer span.End()

	klog.Infof("DB Repo CreatePackageRevisionDraft: %q", r.Key().String())

	dbPkgRev := &dbPackageRevision{
		repo:       r,
		definition: newPR,
		pkgRevKey: repository.PackageRevisionKey{
			PkgKey: repository.PackageKey{
				RepoKey: r.Key(),
				Package: newPR.Spec.PackageName,
			},
			Revision:      0,
			WorkspaceName: newPR.Spec.WorkspaceName,
		},
		meta:      newPR.ObjectMeta,
		spec:      newPR.Spec,
		lifecycle: v1alpha1.PackageRevisionLifecycleDraft,
		updated:   time.Now(),
		updatedBy: getCurrentUser(),
		tasks:     newPR.Spec.Tasks,
	}

	prDraft, err := r.savePackageRevision(ctx, dbPkgRev, 0)

	return repository.PackageRevisionDraft(prDraft), err
}

func (r *dbRepository) DeletePackageRevision(ctx context.Context, old repository.PackageRevision) error {
	ctx, span := tracer.Start(ctx, "dbRepository::DeletePackageRevision", trace.WithAttributes())
	defer span.End()

	pk := repository.PackageKey{
		RepoKey: r.Key(),
		Package: old.Key().GetPackageKey().Package,
	}

	foundPkg, err := pkgReadFromDB(ctx, pk)
	if err != nil {
		return err
	}
	foundPkg.repo = old.(*dbPackageRevision).repo

	if err := foundPkg.DeletePackageRevision(ctx, old); err != nil && err != sql.ErrNoRows {
		return err
	}

	foundPRs, err := pkgRevReadPRsFromDB(ctx, foundPkg.Key())
	if err != nil {
		return err
	}

	if len(foundPRs) != 0 {
		return nil
	}

	return pkgDeleteFromDB(ctx, pk)
}

func (r *dbRepository) UpdatePackageRevision(ctx context.Context, updatePR repository.PackageRevision) (repository.PackageRevisionDraft, error) {
	ctx, span := tracer.Start(ctx, "dbRepository::UpdatePackageRevision", trace.WithAttributes())
	defer span.End()

	updatePkgRev, ok := updatePR.(*dbPackageRevision)
	if !ok {
		return nil, fmt.Errorf("cannot update DB package revision %T", updatePR)
	}

	if err := updatePkgRev.UpdatePackageRevision(ctx); err != nil {
		return nil, err
	}

	updatePkgRev.updated = time.Now()
	updatePkgRev.updatedBy = getCurrentUser()

	return updatePkgRev, nil
}

func (r *dbRepository) ListPackages(ctx context.Context, filter repository.ListPackageFilter) ([]repository.Package, error) {
	ctx, span := tracer.Start(ctx, "dbRepository::ListPackages", trace.WithAttributes())
	defer span.End()

	dbPkgs, err := pkgReadPkgsFromDB(ctx, r.Key())
	if err != nil {
		return nil, err
	}

	genericPkgs := make([]repository.Package, len(dbPkgs))
	for i, pkg := range dbPkgs {
		genericPkgs[i] = repository.Package(pkg)
	}

	return genericPkgs, nil
}

func (r *dbRepository) CreatePackage(ctx context.Context, obj *v1alpha1.PorchPackage) (repository.Package, error) {
	_, span := tracer.Start(ctx, "dbRepository::CreatePackage", trace.WithAttributes())
	defer span.End()

	return nil, nil
}

func (r *dbRepository) DeletePackage(ctx context.Context, old repository.Package) error {
	ctx, span := tracer.Start(ctx, "dbRepository::DeletePackage", trace.WithAttributes())
	defer span.End()

	foundPackage, err := pkgReadFromDB(ctx, old.Key())
	if err != nil {
		if err == sql.ErrNoRows {
			return nil
		} else {
			return err
		}
	}

	foundPRs, err := pkgRevReadPRsFromDB(ctx, foundPackage.Key())
	if err != nil {
		return err
	}

	if len(foundPRs) != 0 {
		errMsg := fmt.Sprintf("cannot delete package %q, it has %d package revisions", old.Key().String(), len(foundPRs))
		return errors.New(errMsg)
	}

	return pkgDeleteFromDB(ctx, old.Key())
}

func (r *dbRepository) Version(ctx context.Context) (string, error) {
	klog.Infof("DB Repo version: %q", r.Key().String())
	return "Undefined", nil
}

func (r *dbRepository) ClosePackageRevisionDraft(ctx context.Context, prd repository.PackageRevisionDraft, version int) (repository.PackageRevision, error) {
	_, span := tracer.Start(ctx, "dbRepository::ClosePackageRevisionDraft", trace.WithAttributes())
	defer span.End()

	pr, err := r.savePackageRevision(ctx, prd, version)

	return repository.PackageRevision(pr), err
}

func (r *dbRepository) PushPackageRevision(ctx context.Context, pr repository.PackageRevision) error {
	panic("dbRepository:PushPackageRevision: function should not be invoked on caches")
}

func (r *dbRepository) savePackageRevision(ctx context.Context, prd repository.PackageRevisionDraft, _ int) (*dbPackageRevision, error) {
	_, span := tracer.Start(ctx, "dbRepository::savePackageRevision", trace.WithAttributes())
	defer span.End()

	d := prd.(*dbPackageRevision)

	dbPkg, err := pkgReadFromDB(ctx, d.Key().GetPackageKey())
	if err != nil {
		if err != sql.ErrNoRows {
			return nil, err
		}

		dbPkg = &dbPackage{
			repo:      r,
			pkgKey:    d.Key().GetPackageKey(),
			updated:   d.updated,
			updatedBy: d.updatedBy,
		}

		if _, err := dbPkg.savePackage(ctx); err != nil {
			return nil, err
		}
	}

	pkgRev, err := dbPkg.savePackageRevision(ctx, d)
	if err != nil {
		return nil, err
	}

	r.updated = d.updated
	r.updatedBy = d.updatedBy
	if err = repoUpdateDB(ctx, r); err != nil {
		return nil, err
	}

	return pkgRev, nil
}

func (r *dbRepository) Refresh(ctx context.Context) error {
	_, span := tracer.Start(ctx, "dbRepository::Refresh", trace.WithAttributes())
	defer span.End()

	if err := r.repositorySync.getLastSyncError(); err != nil {
		klog.Warningf("last sync returned error %s, refreshing . . .", err)
	}

	if err := r.externalRepo.Refresh(ctx); err != nil {
		return err
	}

	return r.repositorySync.syncOnce(ctx)
}

func (r *dbRepository) UpdateLifecycle(ctx context.Context, pkgRev repository.PackageRevision, newLifecycle v1alpha1.PackageRevisionLifecycle) error {
	_, span := tracer.Start(ctx, "dbRepository::UpdateLifecycle", trace.WithAttributes())
	defer span.End()

	return nil
}

func (r *dbRepository) getExternalPr(ctx context.Context, prKey repository.PackageRevisionKey) (repository.PackageRevision, error) {
	_, span := tracer.Start(ctx, "dbRepository::getExternalPr", trace.WithAttributes())
	defer span.End()

	filter := repository.ListPackageRevisionFilter{
		Key: prKey,
	}

	prList, err := r.externalRepo.ListPackageRevisions(ctx, filter)

	if err != nil {
		klog.Warningf("error retrieving package revision %+v from external repo, %q", prKey, err)
		return nil, err
	}

	if len(prList) != 1 {
		if len(prList) > 1 {
			err = fmt.Errorf("error retrieving package revision %+v from external repo, more than one package revision was returned", prKey)
		} else {
			err = fmt.Errorf("package revision %+v not found on external repo", prKey)
		}
		klog.Warning(err.Error())
		return nil, err
	}

	return prList[0], nil
}
