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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
)

var _ repository.PackageRevision = &dbPackageRevision{}
var _ repository.PackageRevisionDraft = &dbPackageRevision{}

type dbPackageRevision struct {
	repo       *dbRepository
	definition *v1alpha1.PackageRevision
	pkgRevKey  repository.PackageRevisionKey
	meta       metav1.ObjectMeta
	spec       v1alpha1.PackageRevisionSpec
	updated    time.Time
	updatedBy  string
	lifecycle  v1alpha1.PackageRevisionLifecycle
	tasks      []v1alpha1.Task
	resources  map[string]string
}

func (pr *dbPackageRevision) KubeObjectName() string {
	return pr.Key().NonNSString()
}

func (pr *dbPackageRevision) KubeObjectNamespace() string {
	return pr.pkgRevKey.Namespace
}

func (pr *dbPackageRevision) UID() types.UID {
	return generateUid("packagerevisions.", pr.KubeObjectNamespace(), pr.KubeObjectName())
}

func (pr *dbPackageRevision) Key() repository.PackageRevisionKey {
	return pr.pkgRevKey
}

func (pr *dbPackageRevision) savePackageRevision() (*dbPackageRevision, error) {
	_, err := pkgRevReadFromDB(pr.Key())
	if err == nil {
		return pr, pkgRevUpdateDB(pr)
	} else if err != sql.ErrNoRows {
		return pr, err
	}

	return pr, pkgRevWriteToDB(pr)
}

func (pr *dbPackageRevision) UpdatePackageRevision() error {
	if readPr, err := pkgRevReadFromDB(pr.Key()); err == nil {
		pr.copyToThis(&readPr)
		return nil
	} else {
		return err
	}
}

func (pr *dbPackageRevision) Lifecycle(_ context.Context) v1alpha1.PackageRevisionLifecycle {
	if pr, err := pkgRevReadFromDB(pr.Key()); err != nil {
		klog.Infof("Lifecycle read from DB failed on PackageRevision %q, %q", pr.Key().String(), err)
		return ""
	}

	return pr.lifecycle
}

func (pr *dbPackageRevision) UpdateLifecycle(ctx context.Context, newLifecycle v1alpha1.PackageRevisionLifecycle) error {
	if pr, err := pkgRevReadFromDB(pr.Key()); err != nil {
		errorMsg := fmt.Sprintf("Lifecycle read on DB failed on PackageRevision %q, %q", pr.Key().String(), err)
		return errors.New(errorMsg)
	}

	pr.lifecycle = newLifecycle

	if newLifecycle == v1alpha1.PackageRevisionLifecyclePublished {
		if err := pr.repo.externalRepo.PushPackageRevision(ctx, pr); err != nil {
			klog.Warningf("Push of PackageRevision %q to external repo failed, %q", pr.Key().String(), err)
			pr.lifecycle = v1alpha1.PackageRevisionLifecycleProposed
			return err
		}
	}

	pr.updated = time.Now()
	pr.updatedBy = getCurrentUser()

	return pkgRevUpdateDB(pr)
}

func (pr *dbPackageRevision) GetPackageRevision(ctx context.Context) (*v1alpha1.PackageRevision, error) {
	lockCopy := &v1alpha1.UpstreamLock{}

	readPr, err := pkgRevReadFromDB(pr.Key())
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
			Namespace:       readPr.pkgRevKey.Namespace,
			UID:             readPr.UID(),
			ResourceVersion: readPr.ResourceVersion(),
			CreationTimestamp: metav1.Time{
				Time: readPr.updated,
			},
		},
		Spec: v1alpha1.PackageRevisionSpec{
			PackageName:    readPr.Key().Package,
			RepositoryName: readPr.Key().Repository,
			Lifecycle:      readPr.Lifecycle(ctx),
			Tasks:          nil,
			ReadinessGates: repository.ToApiReadinessGates(kf),
			WorkspaceName:  readPr.Key().WorkspaceName,
			Revision:       readPr.Key().Revision,
		},
		Status: status,
	}, nil
}

func (pr *dbPackageRevision) GetResources(context.Context) (*v1alpha1.PackageRevisionResources, error) {
	readPr, err := pkgRevReadFromDB(pr.Key())
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
			Namespace:       readPr.pkgRevKey.Namespace,
			UID:             readPr.UID(),
			ResourceVersion: readPr.ResourceVersion(),
			CreationTimestamp: metav1.Time{
				Time: readPr.updated,
			},
			OwnerReferences: []metav1.OwnerReference{}, // TODO: should point to repository resource
		},
		Spec: v1alpha1.PackageRevisionResourcesSpec{
			PackageName:    key.Package,
			WorkspaceName:  key.WorkspaceName,
			Revision:       key.Revision,
			RepositoryName: key.Repository,
			Resources:      readPr.resources,
		},
	}, nil
}

func (pr *dbPackageRevision) GetUpstreamLock(context.Context) (kptfile.Upstream, kptfile.UpstreamLock, error) {
	return kptfile.Upstream{}, kptfile.UpstreamLock{}, nil
}

func (pr *dbPackageRevision) ToMainPackageRevision() repository.PackageRevision {
	klog.Infof("ToMainPackageRevision: %q, main package revisions are not required when using the DB cache", pr.pkgRevKey.Package)
	return nil
}

func (pr *dbPackageRevision) GetMeta() metav1.ObjectMeta {
	return metav1.ObjectMeta{}
}

func (pr *dbPackageRevision) SetMeta(context.Context, metav1.ObjectMeta) error {
	return nil
}

func (pr *dbPackageRevision) GetKptfile(context.Context) (kptfile.KptFile, error) {
	readPr, err := pkgRevReadFromDB(pr.Key())
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

func (pr *dbPackageRevision) Delete() error {
	return pkgRevDeleteFromDB(pr.Key())
}

func (pr *dbPackageRevision) copyToThis(otherPr *dbPackageRevision) {
	pr.pkgRevKey.Namespace = otherPr.pkgRevKey.Namespace
	pr.pkgRevKey.Repository = otherPr.pkgRevKey.Repository
	pr.pkgRevKey.Package = otherPr.pkgRevKey.Package
	pr.pkgRevKey.Revision = otherPr.pkgRevKey.Revision
	pr.pkgRevKey.WorkspaceName = otherPr.pkgRevKey.WorkspaceName
	pr.updated = otherPr.updated
	pr.updatedBy = otherPr.updatedBy
	pr.lifecycle = otherPr.lifecycle
	pr.resources = otherPr.resources
}

func (pr *dbPackageRevision) UpdateResources(ctx context.Context, new *v1alpha1.PackageRevisionResources, change *v1alpha1.Task) error {
	pr.resources = new.Spec.Resources
	return nil
}

func (pr *dbPackageRevision) GetName() string {
	return pr.pkgRevKey.Package
}
