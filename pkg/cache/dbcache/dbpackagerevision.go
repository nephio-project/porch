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
	"strings"
	"time"

	porchapi "github.com/nephio-project/porch/api/porch/v1alpha1"
	"github.com/nephio-project/porch/internal/kpt/pkg"
	kptfile "github.com/nephio-project/porch/pkg/kpt/api/kptfile/v1"
	"github.com/nephio-project/porch/pkg/repository"
	"github.com/nephio-project/porch/pkg/util"
	pkgerrors "github.com/pkg/errors"
	"go.opentelemetry.io/otel/trace"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
)

var (
	_ repository.PackageRevision      = &dbPackageRevision{}
	_ repository.PackageRevisionDraft = &dbPackageRevision{}
)

type dbPackageRevision struct {
	repo      *dbRepository
	pkgRevKey repository.PackageRevisionKey
	meta      metav1.ObjectMeta
	spec      *porchapi.PackageRevisionSpec
	updated   time.Time
	updatedBy string
	lifecycle porchapi.PackageRevisionLifecycle
	deplState deploymentState
	latest    bool
	tasks     []porchapi.Task
	resources map[string]string
}

func (pr *dbPackageRevision) KubeObjectName() string {
	return repository.ComposePkgRevObjName(pr.Key())
}

func (pr *dbPackageRevision) KubeObjectNamespace() string {
	return pr.Key().RKey().Namespace
}

func (pr *dbPackageRevision) UID() types.UID {
	return util.GenerateUid("packagerevision:", pr.KubeObjectNamespace(), pr.KubeObjectName())
}

func (pr *dbPackageRevision) Key() repository.PackageRevisionKey {
	return pr.pkgRevKey
}

func (pr *dbPackageRevision) savePackageRevision(ctx context.Context, saveResources bool) (*dbPackageRevision, error) {
	_, span := tracer.Start(ctx, "dbPackageRevision::savePackageRevision", trace.WithAttributes())
	defer span.End()

	if saveResources && pr.repo.deployment {
		pr.updated = time.Now()
		pr.updatedBy = getCurrentUser()
	}

	_, err := pkgRevReadFromDB(ctx, pr.Key(), false)
	if err == nil {
		return pr, pkgRevUpdateDB(ctx, pr, saveResources)
	} else if err != sql.ErrNoRows {
		return pr, err
	}

	return pr, pkgRevWriteToDB(ctx, pr)
}

func (pr *dbPackageRevision) UpdatePackageRevision(ctx context.Context) error {
	_, span := tracer.Start(ctx, "dbPackageRevision::UpdatePackageRevision", trace.WithAttributes())
	defer span.End()

	if readPr, err := pkgRevReadFromDB(ctx, pr.Key(), true); err == nil {
		pr.copyToThis(readPr)
		return nil
	} else {
		return err
	}
}

func (pr *dbPackageRevision) Lifecycle(ctx context.Context) porchapi.PackageRevisionLifecycle {
	_, span := tracer.Start(ctx, "dbPackageRevision::Lifecycle", trace.WithAttributes())
	defer span.End()

	return pr.lifecycle
}

func (pr *dbPackageRevision) UpdateLifecycle(ctx context.Context, newLifecycle porchapi.PackageRevisionLifecycle) error {
	_, span := tracer.Start(ctx, "dbPackageRevision::UpdateLifecycle", trace.WithAttributes())
	defer span.End()

	if pr.lifecycle == porchapi.PackageRevisionLifecycleProposed && newLifecycle == porchapi.PackageRevisionLifecyclePublished {
		if err := pr.publishPR(ctx, newLifecycle); err != nil {
			pr.pkgRevKey.Revision = 0
			return pkgerrors.Wrapf(err, "dbPackageRevision:UpdateLifecycle: could not publish package revision %+v", pr.Key())
		}
	} else if porchapi.LifecycleIsPublished(pr.lifecycle) {
		return pr.updateLifecycleOnPublishedPR(ctx, newLifecycle)
	}

	pr.lifecycle = newLifecycle
	return nil
}

func (pr *dbPackageRevision) GetPackageRevision(ctx context.Context) (*porchapi.PackageRevision, error) {
	_, span := tracer.Start(ctx, "dbPackageRevision::GetPackageRevision", trace.WithAttributes())
	defer span.End()

	readPR, err := pkgRevReadFromDB(ctx, pr.Key(), false)
	if err != nil {
		if pr.GetMeta().DeletionTimestamp != nil {
			// The PR is already deleted from the DB so we just return the metadata version of this PR that is just about to be removed from memory
			readPR = pr
		} else {
			return nil, fmt.Errorf("package revision read on DB failed %+v, %q", pr.Key(), err)
		}
	}

	_, lock, _ := pr.GetUpstreamLock(ctx)
	lockCopy := &porchapi.UpstreamLock{}

	// TODO: Comment copied from pkg/externalrepo/git/package.go
	// Use kpt definition of UpstreamLock in the package revision status
	// when https://github.com/GoogleContainerTools/kpt/issues/3297 is complete.
	// Until then, we have to translate from one type to another.
	if lock.Git != nil {
		lockCopy = &porchapi.UpstreamLock{
			Type: porchapi.OriginType(lock.Type),
			Git: &porchapi.GitLock{
				Repo:      lock.Git.Repo,
				Directory: lock.Git.Directory,
				Commit:    lock.Git.Commit,
				Ref:       lock.Git.Ref,
			},
		}
	}

	kf, _ := readPR.GetKptfile(ctx)

	status := porchapi.PackageRevisionStatus{
		UpstreamLock: lockCopy,
		Deployment:   pr.repo.deployment,
		Conditions:   repository.ToAPIConditions(kf),
	}

	if porchapi.LifecycleIsPublished(readPR.Lifecycle(ctx)) {
		if !readPR.updated.IsZero() {
			status.PublishedAt = metav1.Time{Time: readPR.updated}
		}
		if readPR.updatedBy != "" {
			status.PublishedBy = readPR.updatedBy
		}
	}

	// Set the "latest" label
	labels := pr.GetMeta().Labels

	if readPR.latest {
		// copy the labels in case the cached object is being read by another go routine
		newLabels := make(map[string]string, len(labels))
		maps.Copy(newLabels, labels)
		newLabels[porchapi.LatestPackageRevisionKey] = porchapi.LatestPackageRevisionValue
		labels = newLabels
	}

	return &porchapi.PackageRevision{
		TypeMeta: metav1.TypeMeta{
			Kind:       "PackageRevision",
			APIVersion: porchapi.SchemeGroupVersion.Identifier(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:              readPR.KubeObjectName(),
			Namespace:         readPR.Key().RKey().Namespace,
			UID:               readPR.UID(),
			ResourceVersion:   readPR.ResourceVersion(),
			CreationTimestamp: readPR.GetMeta().CreationTimestamp,
			DeletionTimestamp: readPR.GetMeta().DeletionTimestamp,
			Labels:            labels,
			OwnerReferences:   readPR.GetMeta().OwnerReferences,
			Annotations:       readPR.GetMeta().Annotations,
			Finalizers:        readPR.GetMeta().Finalizers,
		},
		Spec: porchapi.PackageRevisionSpec{
			PackageName:    readPR.Key().PKey().ToPkgPathname(),
			RepositoryName: readPR.Key().RKey().Name,
			Lifecycle:      readPR.Lifecycle(ctx),
			Tasks:          readPR.tasks,
			ReadinessGates: repository.ToAPIReadinessGates(kf),
			WorkspaceName:  readPR.Key().WorkspaceName,
			Revision:       readPR.Key().Revision,
		},
		Status: status,
	}, nil
}

func (pr *dbPackageRevision) GetResources(ctx context.Context) (*porchapi.PackageRevisionResources, error) {
	_, span := tracer.Start(ctx, "dbPackageRevision::GetResources", trace.WithAttributes())
	defer span.End()

	resources, err := pkgRevResourcesReadFromDB(ctx, pr.pkgRevKey)
	if err != nil {
		klog.V(5).Infof("pkgRevScanRowsFromDB: reading package revision %+v resources returned err: %q", pr.Key(), err)
		return nil, err
	}

	klog.V(5).Infof("pkgRevScanRowsFromDB: reading package revision resources succeeded %+v", pr.Key())

	return &porchapi.PackageRevisionResources{
		TypeMeta: metav1.TypeMeta{
			Kind:       "PackageRevisionResources",
			APIVersion: porchapi.SchemeGroupVersion.Identifier(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:            pr.KubeObjectName(),
			Namespace:       pr.Key().RKey().Namespace,
			UID:             pr.UID(),
			ResourceVersion: pr.ResourceVersion(),
			CreationTimestamp: metav1.Time{
				Time: pr.updated,
			},
		},
		Spec: porchapi.PackageRevisionResourcesSpec{
			PackageName:    pr.Key().PKey().Package,
			WorkspaceName:  pr.Key().WorkspaceName,
			Revision:       pr.Key().Revision,
			RepositoryName: pr.Key().RKey().Name,
			Resources:      resources,
		},
	}, nil
}

func (pr *dbPackageRevision) GetUpstreamLock(ctx context.Context) (kptfile.Upstream, kptfile.UpstreamLock, error) {
	kf, err := pr.GetKptfile(ctx)
	if err != nil {
		return kptfile.Upstream{}, kptfile.UpstreamLock{}, fmt.Errorf("cannot determine package lock; cannot retrieve resources: %w", err)
	}

	if kf.Upstream == nil || kf.UpstreamLock == nil || kf.Upstream.Git == nil {
		return kptfile.Upstream{}, kptfile.UpstreamLock{}, nil
	}

	return *kf.Upstream, *kf.UpstreamLock, nil
}

func (pr *dbPackageRevision) ToMainPackageRevision(ctx context.Context) repository.PackageRevision {
	_, span := tracer.Start(ctx, "dbPackageRevision::ToMainPackageRevision", trace.WithAttributes())
	defer span.End()

	mainPR := &dbPackageRevision{
		repo: pr.repo,
		pkgRevKey: repository.PackageRevisionKey{
			PkgKey:        pr.Key().PKey(),
			Revision:      -1,
			WorkspaceName: pr.Key().RKey().PlaceholderWSname,
		},
		meta:      metav1.ObjectMeta{},
		spec:      &porchapi.PackageRevisionSpec{},
		updated:   time.Now(),
		updatedBy: getCurrentUser(),
		lifecycle: pr.lifecycle,
		latest:    false,
		tasks:     pr.tasks,
		resources: pr.resources,
	}

	if mainPR.pkgRevKey.WorkspaceName == "" {
		mainPR.pkgRevKey.WorkspaceName = "main"
	}

	klog.V(5).Infof("ToMainPackageRevision: %+v, main package revision generated", mainPR.Key())

	return mainPR
}

func (pr *dbPackageRevision) GetMeta() metav1.ObjectMeta {
	return pr.meta
}

func (pr *dbPackageRevision) SetMeta(ctx context.Context, meta metav1.ObjectMeta) error {
	_, span := tracer.Start(ctx, "dbPackageRevision::SetMeta", trace.WithAttributes())
	defer span.End()

	pr.meta = meta
	return pkgRevUpdateDB(ctx, pr, false)
}

func (pr *dbPackageRevision) GetKptfile(ctx context.Context) (kptfile.KptFile, error) {
	_, span := tracer.Start(ctx, "dbPackageRevision::GetKptfile", trace.WithAttributes())
	defer span.End()

	_, kfString, err := pkgRevResourceReadFromDB(ctx, pr.Key(), kptfile.KptFileName)
	if err != nil {
		return kptfile.KptFile{}, fmt.Errorf("no Kptfile for packagerevision %+v found in DB: %q", pr.Key(), err)
	}

	kf, err := pkg.DecodeKptfile(strings.NewReader(kfString))
	if err != nil {
		return kptfile.KptFile{}, fmt.Errorf("error decoding Kptfile: %w", err)
	}
	return *kf, nil
}

func (pr *dbPackageRevision) GetLock() (kptfile.Upstream, kptfile.UpstreamLock, error) {
	lockRef := ""

	if porchapi.LifecycleIsPublished(pr.lifecycle) {
		if pr.Key().Revision > 0 {
			// Tag for a released revision of a PR
			lockRef = fmt.Sprintf("%s/v%d", pr.Key().PKey().ToPkgPathname(), pr.Key().Revision)
		} else {
			// Reference for the placeholder PR
			lockRef = pr.Key().RKey().PlaceholderWSname
		}
	} else {
		// Where the draft would be if it were in Git
		lockRef = fmt.Sprintf("drafts/%s/%s"+pr.Key().PKey().ToPkgPathname(), pr.Key().WorkspaceName)
	}

	return kptfile.Upstream{
			Type: kptfile.GitOrigin,
			Git: &kptfile.Git{
				Repo:      pr.repo.spec.Spec.Git.Repo,
				Directory: pr.Key().PKey().ToPkgPathname(),
			},
		}, kptfile.UpstreamLock{
			Type: kptfile.GitOrigin,
			Git: &kptfile.GitLock{
				Repo:      pr.repo.spec.Spec.Git.Repo,
				Directory: pr.Key().PKey().ToPkgPathname(),
				Ref:       lockRef,
			},
		}, nil
}

func (pr *dbPackageRevision) ResourceVersion() string {
	return fmt.Sprintf("%s.%d", pr.KubeObjectName(), pr.updated.UnixMicro())
}

func (pr *dbPackageRevision) Delete(ctx context.Context) error {
	_, span := tracer.Start(ctx, "dbPackageRevision::Delete", trace.WithAttributes())
	defer span.End()

	err := pkgRevDeleteFromDB(ctx, pr.Key())
	if err != nil && err != sql.ErrNoRows {
		klog.Warningf("dbPackage:DeletePackageRevision: deletion of %+v failed on database %q", pr.Key(), err)
	}

	return err
}

func (pr *dbPackageRevision) copyToThis(otherPr *dbPackageRevision) {
	otherPr.pkgRevKey.DeepCopy(&pr.pkgRevKey)

	pr.updated = otherPr.updated
	pr.updatedBy = otherPr.updatedBy
	pr.lifecycle = otherPr.lifecycle
	pr.tasks = otherPr.tasks
	pr.resources = otherPr.resources
}

func (pr *dbPackageRevision) UpdateResources(ctx context.Context, new *porchapi.PackageRevisionResources, change *porchapi.Task) error {
	_, span := tracer.Start(ctx, "dbPackageRevision::UpdateResources", trace.WithAttributes())
	defer span.End()

	pr.resources = new.Spec.Resources

	if change != nil && porchapi.IsValidFirstTaskType(change.Type) {
		if len(pr.tasks) > 0 {
			klog.Warningf("Replacing first task of %q", pr.Key())
		}
		pr.tasks = []porchapi.Task{*change}
	}

	return nil
}

func (pr *dbPackageRevision) publishPR(ctx context.Context, newLifecycle porchapi.PackageRevisionLifecycle) error {
	_, span := tracer.Start(ctx, "dbPackageRevision::publishPR", trace.WithAttributes())
	defer span.End()

	latestRev, err := pkgRevGetlatestRevFromDB(ctx, pr.Key().PkgKey)
	if err != nil {
		return pkgerrors.Wrapf(err, "dbPackageRevision:publishPR: could not get latest package revision for package revision %+v from DB", pr.Key())
	}

	pr.pkgRevKey.Revision = latestRev + 1
	pr.lifecycle = newLifecycle

	if pr.pkgRevKey.Revision == 1 {
		if err = pkgRevWriteToDB(ctx, pr.ToMainPackageRevision(ctx).(*dbPackageRevision)); err != nil {
			return pkgerrors.Wrapf(err, "dbPackageRevision:publishPR: could not write placeholder package revision for package revision %+v to DB", pr.Key())
		}
	} else if pr.pkgRevKey.Revision > 1 {
		if err = pkgRevUpdateDB(ctx, pr.ToMainPackageRevision(ctx).(*dbPackageRevision), true); err != nil {
			return pkgerrors.Wrapf(err, "dbPackageRevision:publishPR: could not update placeholder package revision for package revision %+v to DB", pr.Key())
		}
	}

	pr.deplState = Deploying

	if err = pr.repo.Refresh(ctx); err != nil {
		return pkgerrors.Wrapf(err, "dbPackageRevision:publishPR: could not refresh repository for package revision %+v to DB", pr.Key())
	}

	return nil
}

func (pr *dbPackageRevision) updateLifecycleOnPublishedPR(ctx context.Context, newLifecycle porchapi.PackageRevisionLifecycle) error {
	ctx, span := tracer.Start(ctx, "dbPackageRevision::updateLifecycleOnPublishedPR", trace.WithAttributes())
	defer span.End()

	pr.lifecycle = newLifecycle

	_, err := pr.savePackageRevision(ctx, false)
	return err
}
