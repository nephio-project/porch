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
	"strings"
	"time"

	porchapi "github.com/nephio-project/porch/api/porch/v1alpha1"
	"github.com/nephio-project/porch/internal/kpt/pkg"
	"github.com/nephio-project/porch/pkg/engine"
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

	repositoryGVK      = porchapi.SchemeGroupVersion.WithKind("Repository")
	packageRevisionGVK = porchapi.SchemeGroupVersion.WithKind("PackageRevision")
)

type dbPackageRevision struct {
	repo      *dbRepository
	pkgRevKey repository.PackageRevisionKey
	meta      metav1.ObjectMeta
	spec      *porchapi.PackageRevisionSpec
	updated   time.Time
	updatedBy string
	lifecycle porchapi.PackageRevisionLifecycle
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

	pr.updated = time.Now()
	pr.updatedBy = getCurrentUser()

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
	pr.updated = time.Now()
	pr.updatedBy = getCurrentUser()

	return nil
}

func (pr *dbPackageRevision) GetPackageRevision(ctx context.Context) (*porchapi.PackageRevision, error) {
	_, span := tracer.Start(ctx, "dbPackageRevision::GetPackageRevision", trace.WithAttributes())
	defer span.End()

	readPr, err := pkgRevReadFromDB(ctx, pr.Key(), false)
	if err != nil {
		return nil, fmt.Errorf("package revision read on DB failed %+v, %q", pr.Key(), err)
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

	kf, _ := readPr.GetKptfile(ctx)

	status := porchapi.PackageRevisionStatus{
		UpstreamLock: lockCopy,
		Deployment:   pr.repo.deployment,
		Conditions:   repository.ToApiConditions(kf),
	}

	if porchapi.LifecycleIsPublished(readPr.Lifecycle(ctx)) {
		if !readPr.updated.IsZero() {
			status.PublishedAt = metav1.Time{Time: readPr.updated}
		}
		if readPr.updatedBy != "" {
			status.PublishedBy = readPr.updatedBy
		}
	}

	// Set the "latest" label
	labels := pr.GetMeta().Labels

	if pr.latest {
		// copy the labels in case the cached object is being read by another go routine
		newLabels := make(map[string]string, len(labels))
		for k, v := range labels {
			newLabels[k] = v
		}
		newLabels[porchapi.LatestPackageRevisionKey] = porchapi.LatestPackageRevisionValue
		labels = newLabels
	}

	return &porchapi.PackageRevision{
		TypeMeta: metav1.TypeMeta{
			Kind:       "PackageRevision",
			APIVersion: porchapi.SchemeGroupVersion.Identifier(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:            readPr.KubeObjectName(),
			Namespace:       readPr.Key().RKey().Namespace,
			UID:             readPr.UID(),
			ResourceVersion: readPr.ResourceVersion(),
			CreationTimestamp: metav1.Time{
				Time: readPr.updated,
			},
			Labels: labels,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: repositoryGVK.GroupVersion().String(),
					Kind:       repositoryGVK.Kind,
					Name:       pr.repo.repoKey.K8SName(),
					UID:        pr.repo.UID(),
				},
			},
		},
		Spec: porchapi.PackageRevisionSpec{
			PackageName:    readPr.Key().PKey().Package,
			RepositoryName: readPr.Key().RKey().Name,
			Lifecycle:      readPr.Lifecycle(ctx),
			Tasks:          readPr.tasks,
			ReadinessGates: repository.ToApiReadinessGates(kf),
			WorkspaceName:  readPr.Key().WorkspaceName,
			Revision:       readPr.Key().Revision,
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
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: packageRevisionGVK.GroupVersion().String(),
					Kind:       packageRevisionGVK.Kind,
					Name:       pr.Key().K8SName(),
					UID:        pr.UID(),
				},
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
	_, span := tracer.Start(ctx, "dbPackageRevision::SetMeta", trace.WithAttributes())
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
	return metav1.ObjectMeta{}
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
	if porchapi.LifecycleIsPublished(pr.lifecycle) {
		externalPr, err := pr.repo.getExternalPr(context.Background(), pr.Key())
		if err != nil {
			return kptfile.Upstream{}, kptfile.UpstreamLock{},
				pkgerrors.Wrapf(err, "dbPackageRevision:GetLock: getting loc of %+v failed, could not find package revision on external repository", pr.Key())
		}

		return externalPr.GetLock()
	} else {
		return kptfile.Upstream{}, kptfile.UpstreamLock{}, nil
	}
}

func (pr *dbPackageRevision) ResourceVersion() string {
	return fmt.Sprintf("%s.%d", pr.KubeObjectName(), pr.updated.Unix())
}

func (pr *dbPackageRevision) Delete(ctx context.Context, deleteExternal bool) error {
	_, span := tracer.Start(ctx, "dbPackageRevision::Delete", trace.WithAttributes())
	defer span.End()

	if deleteExternal && porchapi.LifecycleIsPublished(pr.lifecycle) {
		externalPr, err := pr.repo.getExternalPr(ctx, pr.Key())
		if err != nil {
			return pkgerrors.Wrapf(err, "dbPackageRevision:Delete: deletion of %+v failed, could not find package revision on external repository", pr.Key())
		}

		if err := pr.repo.externalRepo.DeletePackageRevision(ctx, externalPr); err != nil {
			klog.Warningf("dbPackageRevision:Delete: deletion of %+v failed on external repository %q", pr.Key(), err)
			return err
		}
	}

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

	if change != nil {
		pr.tasks = append(pr.tasks, *change)
	}

	return nil
}

func (pr *dbPackageRevision) publishPR(ctx context.Context, newLifecycle porchapi.PackageRevisionLifecycle) error {
	_, span := tracer.Start(ctx, "dbPackageRevision::publishToExternalRepo", trace.WithAttributes())
	defer span.End()

	latestRev, err := pkgRevGetlatestRevFromDB(ctx, pr.Key().PkgKey)
	if err != nil {
		return pkgerrors.Wrapf(err, "dbPackageRevision:publishPR: could not get latest package revision for package revision %+v from DB", pr.Key())
	}

	pr.pkgRevKey.Revision = latestRev + 1
	pr.lifecycle = newLifecycle

	if err := engine.PushPackageRevision(ctx, pr.repo.externalRepo, pr); err != nil {
		klog.Warningf("push of package revision %+v to external repo failed, %q", pr.Key(), err)
		pr.pkgRevKey.Revision = 0
		pr.lifecycle = porchapi.PackageRevisionLifecycleProposed
		return pkgerrors.Wrapf(err, "dbPackageRevision:publishPR: push of package revision %+v to external repo failed", pr.Key())
	}

	if pr.pkgRevKey.Revision == 1 {
		if err = pkgRevWriteToDB(ctx, pr.ToMainPackageRevision(ctx).(*dbPackageRevision)); err != nil {
			return pkgerrors.Wrapf(err, "dbPackageRevision:UpdateLifecycle: could not write placeholder package revision for package revision %+v to DB", pr.Key())
		}
	} else if pr.pkgRevKey.Revision > 1 {
		if err = pkgRevUpdateDB(ctx, pr.ToMainPackageRevision(ctx).(*dbPackageRevision), true); err != nil {
			return pkgerrors.Wrapf(err, "dbPackageRevision:UpdateLifecycle: could not update placeholder package revision for package revision %+v to DB", pr.Key())
		}
	}

	return nil
}

func (pr *dbPackageRevision) updateLifecycleOnPublishedPR(ctx context.Context, newLifecycle porchapi.PackageRevisionLifecycle) error {
	ctx, span := tracer.Start(ctx, "dbPackageRevision::updateLifecycleOnPublishedPR", trace.WithAttributes())
	defer span.End()

	externalPr, err := pr.repo.getExternalPr(ctx, pr.Key())
	if err != nil {
		return err
	}

	if err := externalPr.UpdateLifecycle(ctx, newLifecycle); err != nil {
		klog.Warningf("error setting lifecycle to %q on package revision %+v for external repo, %q", newLifecycle, pr.Key(), err)
		return err
	}

	pr.lifecycle = newLifecycle
	pr.updated = time.Now()
	pr.updatedBy = getCurrentUser()

	_, err = pr.savePackageRevision(ctx, false)
	return err
}
