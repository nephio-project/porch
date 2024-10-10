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

type dbPackageRevision struct {
	pkgRevKey repository.PackageRevisionKey
	updated   time.Time
	updatedBy string
	lifecycle v1alpha1.PackageRevisionLifecycle
	resources map[string]string
}

func (pr dbPackageRevision) KubeObjectName() string {
	return pr.Key().String()
}

func (pr dbPackageRevision) KubeObjectNamespace() string {
	return pr.pkgRevKey.Namespace
}

func (pr dbPackageRevision) UID() types.UID {
	return generateUid("packagerevisions.", pr.KubeObjectNamespace(), pr.KubeObjectName())
}

func (pr dbPackageRevision) Key() repository.PackageRevisionKey {
	return pr.pkgRevKey
}

func (pr dbPackageRevision) createPackageRevision() (*dbPackageRevision, error) {
	_, err := pkgRevReadFromDB(pr.Key())
	if err == nil {
		return &pr, pkgRevUpdateDB(pr)
	} else if err != sql.ErrNoRows {
		return &pr, err
	}

	return &pr, pkgRevWriteToDB(pr)
}

func (pr dbPackageRevision) Lifecycle() v1alpha1.PackageRevisionLifecycle {
	if pr, err := pkgRevReadFromDB(pr.Key()); err != nil {
		klog.Infof("Lifecycle read from DB failed on PackageRevision %q, %q", pr.Key().String(), err)
		return ""
	}

	return pr.lifecycle
}

func (pr dbPackageRevision) UpdateLifecycle(ctx context.Context, newLifecycle v1alpha1.PackageRevisionLifecycle) error {
	if pr, err := pkgRevReadFromDB(pr.Key()); err != nil {
		errorMsg := fmt.Sprintf("Lifecycle update on DB failed on PackageRevision %q, %q", pr.Key().String(), err)
		return errors.New(errorMsg)
	}

	pr.lifecycle = newLifecycle
	pr.updated = time.Now()
	pr.updatedBy = getCurrentUser()

	return pkgRevUpdateDB(pr)
}

func (pr dbPackageRevision) GetPackageRevision(ctx context.Context) (*v1alpha1.PackageRevision, error) {
	key := pr.Key()

	lockCopy := &v1alpha1.UpstreamLock{}

	kf, _ := pr.GetKptfile(ctx)

	status := v1alpha1.PackageRevisionStatus{
		UpstreamLock: lockCopy,
		Conditions:   repository.ToApiConditions(kf),
	}

	if v1alpha1.LifecycleIsPublished(pr.Lifecycle()) {
		if !pr.updated.IsZero() {
			status.PublishedAt = metav1.Time{Time: pr.updated}
		}
		if pr.updatedBy != "" {
			status.PublishedBy = pr.updatedBy
		}
	}

	return &v1alpha1.PackageRevision{
		TypeMeta: metav1.TypeMeta{
			Kind:       "PackageRevision",
			APIVersion: v1alpha1.SchemeGroupVersion.Identifier(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:            pr.KubeObjectName(),
			Namespace:       pr.pkgRevKey.Namespace,
			UID:             pr.UID(),
			ResourceVersion: pr.ResourceVersion(),
			CreationTimestamp: metav1.Time{
				Time: pr.updated,
			},
		},
		Spec: v1alpha1.PackageRevisionSpec{
			PackageName:    key.Package,
			RepositoryName: key.Repository,
			Lifecycle:      pr.Lifecycle(),
			Tasks:          nil,
			ReadinessGates: repository.ToApiReadinessGates(kf),
			WorkspaceName:  key.WorkspaceName,
			Revision:       key.Revision,
		},
		Status: status,
	}, nil
}

func (pr dbPackageRevision) GetResources(context.Context) (*v1alpha1.PackageRevisionResources, error) {

	key := pr.Key()

	return &v1alpha1.PackageRevisionResources{
		TypeMeta: metav1.TypeMeta{
			Kind:       "PackageRevisionResources",
			APIVersion: v1alpha1.SchemeGroupVersion.Identifier(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:            pr.KubeObjectName(),
			Namespace:       pr.pkgRevKey.Namespace,
			UID:             pr.UID(),
			ResourceVersion: pr.ResourceVersion(),
			CreationTimestamp: metav1.Time{
				Time: pr.updated,
			},
			OwnerReferences: []metav1.OwnerReference{}, // TODO: should point to repository resource
		},
		Spec: v1alpha1.PackageRevisionResourcesSpec{
			PackageName:    key.Package,
			WorkspaceName:  key.WorkspaceName,
			Revision:       key.Revision,
			RepositoryName: key.Repository,
			Resources:      pr.resources,
		},
	}, nil
}

func (pr dbPackageRevision) GetUpstreamLock(context.Context) (kptfile.Upstream, kptfile.UpstreamLock, error) {
	return kptfile.Upstream{}, kptfile.UpstreamLock{}, nil
}

func (pr dbPackageRevision) GetKptfile(context.Context) (kptfile.KptFile, error) {
	kfString, found := pr.resources[kptfile.KptFileName]
	if !found {
		return kptfile.KptFile{}, fmt.Errorf("packagerevision does not have a Kptfile")
	}
	kf, err := pkg.DecodeKptfile(strings.NewReader(kfString))
	if err != nil {
		return kptfile.KptFile{}, fmt.Errorf("error decoding Kptfile: %w", err)
	}
	return *kf, nil
}

func (pr dbPackageRevision) GetLock() (kptfile.Upstream, kptfile.UpstreamLock, error) {
	return kptfile.Upstream{}, kptfile.UpstreamLock{}, nil
}

func (pr dbPackageRevision) ResourceVersion() string {
	return fmt.Sprintf("%s.%d", pr.KubeObjectName(), pr.updated.Unix())
}

func (pr dbPackageRevision) Close() error {
	return pkgRevDeleteFromDB(pr.Key())
}
