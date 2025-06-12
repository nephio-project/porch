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
	"strings"
	"time"

	"github.com/nephio-project/porch/api/porch/v1alpha1"
	"github.com/nephio-project/porch/internal/kpt/pkg"
	kptfile "github.com/nephio-project/porch/pkg/kpt/api/kptfile/v1"
	"github.com/nephio-project/porch/pkg/repository"
	"github.com/nephio-project/porch/pkg/util"
	"go.opentelemetry.io/otel/trace"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
)

var _ repository.PackageRevision = &dbPackageRevision{}
var _ repository.PackageRevisionDraft = &dbPackageRevision{}

type dbPackageRevision struct {
	repo      *dbRepository
	pkgRevKey repository.PackageRevisionKey
	meta      metav1.ObjectMeta
	spec      *v1alpha1.PackageRevisionSpec
	updated   time.Time
	updatedBy string
	lifecycle v1alpha1.PackageRevisionLifecycle
	tasks     []v1alpha1.Task
	resources map[string]string
}

func (pr *dbPackageRevision) KubeObjectName() string {
	return repository.ComposePkgRevObjName(pr.Key())
}

func (pr *dbPackageRevision) KubeObjectNamespace() string {
	return pr.Key().GetPackageKey().GetRepositoryKey().Namespace
}

func (pr *dbPackageRevision) UID() types.UID {
	return util.GenerateUid("packagerevisions.", pr.KubeObjectNamespace(), pr.KubeObjectName())
}

func (pr *dbPackageRevision) Key() repository.PackageRevisionKey {
	return pr.pkgRevKey
}

func (pr *dbPackageRevision) GetName() string {
	return pr.Key().GetPackageKey().Package
}

func (pr *dbPackageRevision) savePackageRevision(ctx context.Context) (*dbPackageRevision, error) {
	_, span := tracer.Start(ctx, "dbPackageRevision::savePackageRevision", trace.WithAttributes())
	defer span.End()

	_, err := pkgRevReadFromDB(ctx, pr.Key())
	if err == nil {
		return pr, pkgRevUpdateDB(ctx, pr)
	} else if err != sql.ErrNoRows {
		return pr, err
	}

	return pr, pkgRevWriteToDB(ctx, pr)
}

func (pr *dbPackageRevision) UpdatePackageRevision(ctx context.Context) error {
	_, span := tracer.Start(ctx, "dbPackageRevision::UpdatePackageRevision", trace.WithAttributes())
	defer span.End()

	if readPr, err := pkgRevReadFromDB(ctx, pr.Key()); err == nil {
		pr.copyToThis(readPr)
		return nil
	} else {
		return err
	}
}

func (pr *dbPackageRevision) Lifecycle(ctx context.Context) v1alpha1.PackageRevisionLifecycle {
	_, span := tracer.Start(ctx, "dbPackageRevision::Lifecycle", trace.WithAttributes())
	defer span.End()

	if pr, err := pkgRevReadFromDB(ctx, pr.Key()); err != nil {
		klog.Infof("Lifecycle read from DB failed on PackageRevision %q, %q", pr.Key().String(), err)
		return ""
	}

	return pr.lifecycle
}

func (pr *dbPackageRevision) UpdateLifecycle(ctx context.Context, newLifecycle v1alpha1.PackageRevisionLifecycle) error {
	_, span := tracer.Start(ctx, "dbPackageRevision::UpdateLifecycle", trace.WithAttributes())
	defer span.End()

	readPr, err := pkgRevReadFromDB(ctx, pr.Key())
	if err != nil {
		errorMsg := fmt.Sprintf("Lifecycle read on DB failed on PackageRevision %q, %q", pr.Key().String(), err)
		return errors.New(errorMsg)
	}

	readPr.repo = pr.repo
	if err := readPr.updateLifecycle(ctx, newLifecycle); err != nil {
		return err
	}

	pr.copyToThis(readPr)
	return nil
}

func (pr *dbPackageRevision) updateLifecycle(ctx context.Context, newLifecycle v1alpha1.PackageRevisionLifecycle) error {
	_, span := tracer.Start(ctx, "dbPackageRevision::UpdateLifecycle", trace.WithAttributes())
	defer span.End()

	if pr.lifecycle == v1alpha1.PackageRevisionLifecycleProposed && newLifecycle == v1alpha1.PackageRevisionLifecyclePublished {
		latestRev, err := pkgRevGetlatestRevFromDB(ctx, pr.Key().PkgKey)
		if err != nil {
			return err
		}

		pr.pkgRevKey.Revision = latestRev + 1
		if err := pr.publishToExternalRepo(ctx, newLifecycle); err != nil {
			pr.pkgRevKey.Revision = 0
			return err
		}
	} else if v1alpha1.LifecycleIsPublished(pr.lifecycle) {
		if err := pr.updateLifecycleOnExternalRepo(ctx, newLifecycle); err != nil {
			return err
		}
	}

	pr.lifecycle = newLifecycle
	pr.updated = time.Now()
	pr.updatedBy = getCurrentUser()

	return pkgRevUpdateDB(ctx, pr)
}

func (pr *dbPackageRevision) GetPackageRevision(ctx context.Context) (*v1alpha1.PackageRevision, error) {
	_, span := tracer.Start(ctx, "dbPackageRevision::GetPackageRevision", trace.WithAttributes())
	defer span.End()

	lockCopy := &v1alpha1.UpstreamLock{}

	readPr, err := pkgRevReadFromDB(ctx, pr.Key())
	if err != nil {
		return nil, fmt.Errorf("package revision read on DB failed %q, %q", pr.Key().String(), err)
	}

	kf, _ := readPr.GetKptfile(ctx)

	status := v1alpha1.PackageRevisionStatus{
		UpstreamLock: lockCopy,
		Conditions:   repository.ToApiConditions(kf),
	}

	if v1alpha1.LifecycleIsPublished(readPr.Lifecycle(ctx)) {
		if !readPr.updated.IsZero() {
			status.PublishedAt = metav1.Time{Time: readPr.updated}
		}
		if readPr.updatedBy != "" {
			status.PublishedBy = readPr.updatedBy
		}
	}

	return &v1alpha1.PackageRevision{
		TypeMeta: metav1.TypeMeta{
			Kind:       "PackageRevision",
			APIVersion: v1alpha1.SchemeGroupVersion.Identifier(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:            readPr.KubeObjectName(),
			Namespace:       readPr.Key().GetPackageKey().GetRepositoryKey().Namespace,
			UID:             readPr.UID(),
			ResourceVersion: readPr.ResourceVersion(),
			CreationTimestamp: metav1.Time{
				Time: readPr.updated,
			},
		},
		Spec: v1alpha1.PackageRevisionSpec{
			PackageName:    readPr.Key().GetPackageKey().Package,
			RepositoryName: readPr.Key().GetPackageKey().GetRepositoryKey().Name,
			Lifecycle:      readPr.Lifecycle(ctx),
			Tasks:          nil,
			ReadinessGates: repository.ToApiReadinessGates(kf),
			WorkspaceName:  readPr.Key().WorkspaceName,
			Revision:       readPr.Key().Revision,
		},
		Status: status,
	}, nil
}

func (pr *dbPackageRevision) GetResources(ctx context.Context) (*v1alpha1.PackageRevisionResources, error) {
	_, span := tracer.Start(ctx, "dbPackageRevision::GetResources", trace.WithAttributes())
	defer span.End()

	readPr, err := pkgRevReadFromDB(ctx, pr.Key())
	if err != nil {
		return nil, fmt.Errorf("package revision read on DB failed %q, %q", pr.Key().String(), err)
	}

	key := readPr.Key()

	return &v1alpha1.PackageRevisionResources{
		TypeMeta: metav1.TypeMeta{
			Kind:       "PackageRevisionResources",
			APIVersion: v1alpha1.SchemeGroupVersion.Identifier(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:            readPr.KubeObjectName(),
			Namespace:       readPr.Key().GetPackageKey().GetRepositoryKey().Namespace,
			UID:             readPr.UID(),
			ResourceVersion: readPr.ResourceVersion(),
			CreationTimestamp: metav1.Time{
				Time: readPr.updated,
			},
			OwnerReferences: []metav1.OwnerReference{}, // TODO: should point to repository resource
		},
		Spec: v1alpha1.PackageRevisionResourcesSpec{
			PackageName:    key.GetPackageKey().Package,
			WorkspaceName:  key.WorkspaceName,
			Revision:       key.Revision,
			RepositoryName: key.GetPackageKey().GetRepositoryKey().Name,
			Resources:      readPr.resources,
		},
	}, nil
}

func (pr *dbPackageRevision) GetUpstreamLock(context.Context) (kptfile.Upstream, kptfile.UpstreamLock, error) {
	return kptfile.Upstream{}, kptfile.UpstreamLock{}, nil
}

func (pr *dbPackageRevision) ToMainPackageRevision(ctx context.Context) repository.PackageRevision {
	_, span := tracer.Start(ctx, "dbPackageRevision::SetMeta", trace.WithAttributes())
	defer span.End()
	klog.Infof("ToMainPackageRevision: %q, main package revisions are not required when using the DB cache", pr.Key().GetPackageKey().Package)
	return nil
}

func (pr *dbPackageRevision) GetMeta() metav1.ObjectMeta {
	return metav1.ObjectMeta{}
}

func (pr *dbPackageRevision) SetMeta(ctx context.Context, meta metav1.ObjectMeta) error {
	_, span := tracer.Start(ctx, "dbPackageRevision::SetMeta", trace.WithAttributes())
	defer span.End()

	return nil
}

func (pr *dbPackageRevision) GetKptfile(ctx context.Context) (kptfile.KptFile, error) {
	_, span := tracer.Start(ctx, "dbPackageRevision::GetKptfile", trace.WithAttributes())
	defer span.End()

	readPr, err := pkgRevReadFromDB(ctx, pr.Key())
	if err != nil {
		return kptfile.KptFile{}, fmt.Errorf("package revision read on DB failed %q, %q", pr.Key().String(), err)
	}

	kfString, found := readPr.resources[kptfile.KptFileName]
	if !found {
		return kptfile.KptFile{}, fmt.Errorf("packagerevision does not have a Kptfile")
	}
	kf, err := pkg.DecodeKptfile(strings.NewReader(kfString))
	if err != nil {
		return kptfile.KptFile{}, fmt.Errorf("error decoding Kptfile: %w", err)
	}
	return *kf, nil
}

func (pr *dbPackageRevision) GetLock() (kptfile.Upstream, kptfile.UpstreamLock, error) {
	return kptfile.Upstream{}, kptfile.UpstreamLock{}, nil
}

func (pr *dbPackageRevision) ResourceVersion() string {
	return fmt.Sprintf("%s.%d", pr.KubeObjectName(), pr.updated.Unix())
}

func (pr *dbPackageRevision) Delete(ctx context.Context, deleteExternal bool) error {
	_, span := tracer.Start(ctx, "dbPackageRevision::Delete", trace.WithAttributes())
	defer span.End()

	if deleteExternal && (pr.lifecycle == v1alpha1.PackageRevisionLifecyclePublished || pr.lifecycle == v1alpha1.PackageRevisionLifecycleDeletionProposed) {
		externalPr, err := pr.repo.getExternalPr(ctx, pr.Key())
		if err != nil {
			return err
		}

		if err := pr.repo.externalRepo.DeletePackageRevision(ctx, externalPr); err != nil {
			klog.Warningf("dbPackage:DeletePackageRevision: deletion of %+vq failed on external repository %q", pr.Key(), err)
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
}

func (pr *dbPackageRevision) UpdateResources(ctx context.Context, new *v1alpha1.PackageRevisionResources, change *v1alpha1.Task) error {
	_, span := tracer.Start(ctx, "dbPackageRevision::UpdateResources", trace.WithAttributes())
	defer span.End()

	pr.resources = new.Spec.Resources
	return nil
}

func (pr *dbPackageRevision) publishToExternalRepo(ctx context.Context, newLifecycle v1alpha1.PackageRevisionLifecycle) error {
	_, span := tracer.Start(ctx, "dbPackageRevision::publishToExternalRepo", trace.WithAttributes())
	defer span.End()

	pr.lifecycle = newLifecycle
	if err := pr.repo.externalRepo.PushPackageRevision(ctx, pr); err != nil {
		klog.Warningf("push of package revision %+v to external repo failed, %q", pr.Key(), err)
		pr.lifecycle = v1alpha1.PackageRevisionLifecycleProposed
		return err
	}

	return nil
}

func (pr *dbPackageRevision) updateLifecycleOnExternalRepo(ctx context.Context, newLifecycle v1alpha1.PackageRevisionLifecycle) error {
	ctx, span := tracer.Start(ctx, "dbPackageRevision::updateLifecycleOnExternalRepo", trace.WithAttributes())
	defer span.End()

	externalPr, err := pr.repo.getExternalPr(ctx, pr.Key())
	if err != nil {
		return err
	}

	if err := externalPr.UpdateLifecycle(ctx, newLifecycle); err != nil {
		klog.Warningf("error setting lifecycle to %q on package revision %+v fon external repo, %q", newLifecycle, pr.Key(), err)
		return err
	}

	return nil
}
