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
	"maps"
	"time"

	"github.com/nephio-project/porch/api/porch/v1alpha1"
	configapi "github.com/nephio-project/porch/api/porchconfig/v1alpha1"
	"github.com/nephio-project/porch/pkg/externalrepo"
	externalrepotypes "github.com/nephio-project/porch/pkg/externalrepo/types"
	"github.com/nephio-project/porch/pkg/repository"
	"github.com/nephio-project/porch/pkg/util"
	"github.com/nephio-project/porch/third_party/GoogleContainerTools/kpt-functions-sdk/go/fn"
	pkgerrors "github.com/pkg/errors"
	"go.opentelemetry.io/otel/trace"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
)

var _ repository.Repository = &dbRepository{}

type dbRepository struct {
	repoKey        repository.RepositoryKey
	meta           metav1.ObjectMeta
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

	klog.V(5).Infof("dbRepository:OpenRepository: opening repository %+v", r.Key())

	externalRepo, err := externalrepo.CreateRepositoryImpl(ctx, r.spec, externalRepoOptions)
	if err != nil {
		klog.Warningf("dbRepository:OpenRepository: %+v external DB read failed with error %q", r.Key(), err)
		return err
	}

	r.externalRepo = externalRepo

	if _, err = repoReadFromDB(ctx, r.Key()); err == nil {
		return nil
	} else if err != sql.ErrNoRows {
		klog.Warningf("dbRepository:OpenRepository: %+v DB read failed with error %q", r.Key(), err)
		return err
	}

	if err = repoWriteToDB(ctx, r); err != nil {
		klog.Warningf("dbRepository:OpenRepository: %+v DB write failed with error %q", r.Key(), err)
		return err
	}

	klog.V(5).Infof("dbRepository:OpenRepository: opened repository %+v", r.Key())

	return nil
}

func (r *dbRepository) Close(ctx context.Context) error {
	ctx, span := tracer.Start(ctx, "dbRepository::Close", trace.WithAttributes())
	defer span.End()

	klog.V(5).Infof("dbRepository:close: closing repository %+v", r.Key())

	r.repositorySync.stop()

	dbPkgs, err := pkgReadPkgsFromDB(ctx, r.Key())
	if err != nil {
		return err
	}

	for _, pkg := range dbPkgs {
		// Delete cached packages but not packages on external storage
		if err := pkg.Delete(ctx, false); err != nil {
			return err
		}
	}

	err = repoDeleteFromDB(ctx, r.Key())
	if err != nil {
		return err
	}

	err = r.externalRepo.Close(ctx)

	if err == nil {
		klog.V(5).Infof("dbRepository:close: closed repository %+v", r.Key())
	} else {
		klog.Warningf("dbRepository:close: close of repository %+v failed: %q", r.Key(), err)
	}

	return err
}

func (r *dbRepository) ListPackageRevisions(ctx context.Context, filter repository.ListPackageRevisionFilter) ([]repository.PackageRevision, error) {
	ctx, span := tracer.Start(ctx, "dbRepository::ListPackageRevisions", trace.WithAttributes())
	defer span.End()

	klog.V(5).Infof("ListPackageRevisions: listing package revisions in repository %+v with filter %+v", r.Key(), filter)

	filter.Key.PkgKey.RepoKey = r.Key()
	foundPkgRevs, err := pkgRevListPRsFromDB(ctx, filter)
	if err != nil {
		klog.Warningf("ListPackageRevisions: listing package revisions in repository %+v with filter %+v failed: %q", r.Key(), filter, err)

		return nil, err
	}

	genericPkgRevs := make([]repository.PackageRevision, len(foundPkgRevs))
	for i, pkgRev := range foundPkgRevs {
		genericPkgRevs[i] = repository.PackageRevision(pkgRev)
	}

	klog.V(5).Infof("ListPackageRevisions: listed package revisions in repository %+v with filter %+v", r.Key(), filter)

	return genericPkgRevs, nil
}

func (r *dbRepository) CreatePackageRevisionDraft(ctx context.Context, newPR *v1alpha1.PackageRevision) (repository.PackageRevisionDraft, error) {
	ctx, span := tracer.Start(ctx, "dbRepository::CreatePackageRevisionDraft", trace.WithAttributes())
	defer span.End()

	klog.V(5).Infof("dbRepository:CreatePackageRevisionDraft: creating draft for %+v on repo %+v", newPR, r.Key())

	dbPkgRev := &dbPackageRevision{
		repo: r,
		pkgRevKey: repository.PackageRevisionKey{
			PkgKey:        repository.FromFullPathname(r.Key(), newPR.Spec.PackageName),
			Revision:      0,
			WorkspaceName: newPR.Spec.WorkspaceName,
		},
		meta:      newPR.ObjectMeta,
		spec:      &newPR.Spec,
		lifecycle: v1alpha1.PackageRevisionLifecycleDraft,
		updated:   time.Now(),
		updatedBy: getCurrentUser(),
	}

	prDraft, err := r.savePackageRevisionDraft(ctx, dbPkgRev, 0)

	return repository.PackageRevisionDraft(prDraft), err
}

func (r *dbRepository) DeletePackageRevision(ctx context.Context, pr2Delete repository.PackageRevision) error {
	ctx, span := tracer.Start(ctx, "dbRepository::DeletePackageRevision", trace.WithAttributes())
	defer span.End()

	if len(pr2Delete.GetMeta().Finalizers) > 0 {
		klog.V(5).Infof("dbRepository:DeletePackageRevision: deletion ordered on package revision %+v on repo %+v, but finalizers %+v exist", pr2Delete.Key(), r.Key(), pr2Delete.GetMeta().Finalizers)

		deletionTimestamp := metav1.Time{
			Time: time.Now(),
		}

		pr2DeleteMeta := pr2Delete.GetMeta()
		pr2DeleteMeta.DeletionTimestamp = &deletionTimestamp

		if err := pr2Delete.SetMeta(ctx, pr2DeleteMeta); err != nil {
			return pkgerrors.Wrapf(err, "dbRepository:DeletePackageRevision: could not update metadata on package revision %+v on repo %+v", pr2Delete.Key(), r.Key())
		}

		return nil
	}

	klog.V(5).Infof("dbRepository:DeletePackageRevision: deleting package revision %+v on repo %+v", pr2Delete.Key(), r.Key())

	pk := repository.PackageKey{
		RepoKey: r.Key(),
		Package: pr2Delete.Key().PKey().Package,
	}

	foundPkg, err := pkgReadFromDB(ctx, pk)
	if err != nil {
		return err
	}

	// Delete both the cached and external package
	if err := foundPkg.DeletePackageRevision(ctx, pr2Delete, true); err != nil && err != sql.ErrNoRows {
		return err
	}

	foundPRs, err := pkgRevReadPRsFromDB(ctx, foundPkg.Key())
	if err != nil {
		return err
	}

	if len(foundPRs) > 0 {
		return nil
	}

	if err = pkgDeleteFromDB(ctx, pk); err != nil {
		klog.V(5).Infof("dbRepository:DeletePackageRevision: deletion of package revision %+v on repo %+v reported error %q", pr2Delete.Key(), r.Key(), err)
	}

	klog.V(5).Infof("dbRepository:DeletePackageRevision: deleted package revision %+v on repo %+v", pr2Delete.Key(), r.Key())
	return nil
}

func (r *dbRepository) UpdatePackageRevision(ctx context.Context, updatePR repository.PackageRevision) (repository.PackageRevisionDraft, error) {
	ctx, span := tracer.Start(ctx, "dbRepository::UpdatePackageRevision", trace.WithAttributes())
	defer span.End()

	klog.V(5).Infof("dbRepository:UpdatePackageRevision: updating package revision %+v on repo %+v", updatePR.Key(), r.Key())

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

	klog.V(5).Infof("ListPackages: listing packages in repository %+v with filter %+v", r.Key(), filter)

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

func (r *dbRepository) CreatePackage(ctx context.Context, newPkgDef *v1alpha1.PorchPackage) (repository.Package, error) {
	_, span := tracer.Start(ctx, "dbRepository::CreatePackage", trace.WithAttributes())
	defer span.End()

	klog.V(5).Infof("dbRepository:CreatePackage: creating package for %+v on repo %+v", newPkgDef, r.Key())

	dbPkg := &dbPackage{
		repo: r,
		pkgKey: repository.PackageKey{
			RepoKey: r.Key(),
			Package: newPkgDef.Spec.PackageName,
		},
		meta:      newPkgDef.ObjectMeta,
		spec:      &newPkgDef.Spec,
		updated:   time.Now(),
		updatedBy: getCurrentUser(),
	}

	err := pkgWriteToDB(ctx, dbPkg)
	return dbPkg, err
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

	return foundPackage.Delete(ctx, true)
}

func (r *dbRepository) Version(ctx context.Context) (string, error) {
	ctx, span := tracer.Start(ctx, "cachedRepository::Version", trace.WithAttributes())
	defer span.End()

	return r.externalRepo.Version(ctx)
}

func (r *dbRepository) ClosePackageRevisionDraft(ctx context.Context, prd repository.PackageRevisionDraft, version int) (repository.PackageRevision, error) {
	_, span := tracer.Start(ctx, "dbRepository::ClosePackageRevisionDraft", trace.WithAttributes())
	defer span.End()

	pr, err := r.savePackageRevisionDraft(ctx, prd, version)

	return repository.PackageRevision(pr), err
}

func (r *dbRepository) savePackageRevisionDraft(ctx context.Context, prd repository.PackageRevisionDraft, _ int) (*dbPackageRevision, error) {
	_, span := tracer.Start(ctx, "dbRepository::savePackageRevision", trace.WithAttributes())
	defer span.End()

	d := prd.(*dbPackageRevision)

	return r.savePackageRevision(ctx, d)
}

func (r *dbRepository) savePackageRevision(ctx context.Context, d *dbPackageRevision) (*dbPackageRevision, error) {
	_, span := tracer.Start(ctx, "dbRepository::savePackageRevision", trace.WithAttributes())
	defer span.End()

	dbPkg, err := pkgReadFromDB(ctx, d.Key().PKey())
	if err != nil {
		if err != sql.ErrNoRows {
			return nil, err
		}

		dbPkg = &dbPackage{
			repo:      r,
			pkgKey:    d.Key().PKey(),
			updated:   d.updated,
			updatedBy: d.updatedBy,
		}

		if _, err := dbPkg.savePackage(ctx); err != nil {
			return nil, err
		}
	}

	if kptfile, err := fn.NewKptfileFromPackage(d.resources); err == nil {
		kptfileLabels := kptfile.Obj.GetLabels()
		if d.meta.Labels == nil {
			d.meta.Labels = kptfileLabels
		} else {
			maps.Copy(d.meta.Labels, kptfileLabels)
			kptfile.Obj.SetNestedField(d.meta.Labels, "metadata", "labels")
			if err := kptfile.WriteToPackage(d.resources); err != nil {
				klog.Errorf("error writing Kptfile back to resources: %q", err)
			}
		}
	} else {
		klog.Warningf("unable to get Kptfile to copy labels to: %q", err)
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
		klog.Warningf("last sync returned error %q, refreshing . . .", err)
	}

	if err := r.externalRepo.Refresh(ctx); err != nil {
		return err
	}

	if _, err := r.repositorySync.syncOnce(ctx); err != nil {
		klog.Warningf("sync returned error %q", err)
		return err
	}

	return nil
}

func (r *dbRepository) getExternalPr(ctx context.Context, prKey repository.PackageRevisionKey) (repository.PackageRevision, error) {
	_, span := tracer.Start(ctx, "dbRepository::getExternalPr", trace.WithAttributes())
	defer span.End()

	filter := repository.ListPackageRevisionFilter{Key: prKey}

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
		klog.Warning(err)
		return nil, err
	}

	return prList[0], nil
}
