// Copyright 2022, 2024-2025 The kpt and Nephio Authors
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

package fake

import (
	"context"

	"github.com/nephio-project/porch/api/porch/v1alpha1"
	kptfile "github.com/nephio-project/porch/pkg/kpt/api/kptfile/v1"
	"github.com/nephio-project/porch/pkg/repository"
	"github.com/nephio-project/porch/pkg/util"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// Implementation of the repository.PackageRevision interface for testing.
type FakePackageRevision struct {
	PrKey            repository.PackageRevisionKey
	Uid              types.UID
	PackageLifecycle v1alpha1.PackageRevisionLifecycle
	PackageRevision  *v1alpha1.PackageRevision
	Resources        *v1alpha1.PackageRevisionResources
	Kptfile          kptfile.KptFile
	Ops              []string
	Err              error
}

var _ repository.PackageRevision = &FakePackageRevision{}
var _ repository.PackageRevisionDraft = &FakePackageRevision{}

func (fpr *FakePackageRevision) KubeObjectName() string {
	fpr.Ops = append(fpr.Ops, "KubeObjectName")
	return repository.ComposePkgRevObjName(fpr.Key())
}

func (fpr *FakePackageRevision) KubeObjectNamespace() string {
	fpr.Ops = append(fpr.Ops, "KubeObjectNamespace")
	return fpr.Key().RKey().Namespace
}

func (fpr *FakePackageRevision) UID() types.UID {
	fpr.Ops = append(fpr.Ops, "UID")
	return util.GenerateUid("packagerevision:", fpr.KubeObjectNamespace(), fpr.KubeObjectName())
}

var _ repository.PackageRevision = &FakePackageRevision{}

// ToMainPackageRevision implements repository.PackageRevision.
func (fpr *FakePackageRevision) ToMainPackageRevision(context.Context) repository.PackageRevision {
	panic("unimplemented")
}

func (fpr *FakePackageRevision) ResourceVersion() string {
	fpr.Ops = append(fpr.Ops, "ResourceVersion")
	return fpr.PackageRevision.ResourceVersion
}

func (fpr *FakePackageRevision) Key() repository.PackageRevisionKey {
	fpr.Ops = append(fpr.Ops, "Key")
	return fpr.PrKey
}

func (fpr *FakePackageRevision) Lifecycle(ctx context.Context) v1alpha1.PackageRevisionLifecycle {
	fpr.Ops = append(fpr.Ops, "Lifecycle")
	return fpr.PackageLifecycle
}

func (fpr *FakePackageRevision) GetPackageRevision(context.Context) (*v1alpha1.PackageRevision, error) {
	fpr.Ops = append(fpr.Ops, "GetPackageRevision")
	return fpr.PackageRevision, pr.Err
}

func (fpr *FakePackageRevision) GetResources(context.Context) (*v1alpha1.PackageRevisionResources, error) {
	fpr.Ops = append(fpr.Ops, "GetResources")
	return fpr.Resources, pr.Err
}

func (fpr *FakePackageRevision) GetKptfile(ctx context.Context) (kptfile.KptFile, error) {
	fpr.Ops = append(fpr.Ops, "GetKptfile")
	return fpr.Kptfile, pr.Err
}

func (fpr *FakePackageRevision) GetUpstreamLock(context.Context) (kptfile.Upstream, kptfile.UpstreamLock, error) {
	fpr.Ops = append(fpr.Ops, "GetUpstreamLock")
	return *fpr.Kptfile.Upstream, *fpr.Kptfile.UpstreamLock, pr.Err
}

func (fpr *FakePackageRevision) GetLock() (kptfile.Upstream, kptfile.UpstreamLock, error) {
	fpr.Ops = append(fpr.Ops, "GetLock")
	return *fpr.Kptfile.Upstream, *fpr.Kptfile.UpstreamLock, pr.Err
}

func (fpr *FakePackageRevision) UpdateLifecycle(_ context.Context, lifecycle v1alpha1.PackageRevisionLifecycle) error {
	fpr.Ops = append(fpr.Ops, "UpdateLifecycle")
	fpr.PackageLifecycle = lifecycle
	if fpr.PackageRevision != nil {
		fpr.PackageRevision.Spec.Lifecycle = lifecycle
	}
	return pr.Err
}

func (fpr *FakePackageRevision) GetMeta() metav1.ObjectMeta {
	fpr.Ops = append(fpr.Ops, "GetMeta")
	return metav1.ObjectMeta{}
}

func (fpr *FakePackageRevision) SetMeta(context.Context, metav1.ObjectMeta) error {
	fpr.Ops = append(fpr.Ops, "SetMeta")
	return pr.Err
}

func (fpr *FakePackageRevision) UpdateResources(ctx context.Context, new *v1alpha1.PackageRevisionResources, change *v1alpha1.Task) error {
	fpr.Ops = append(fpr.Ops, "UpdateResources")
	fpr.Resources = new
	return pr.Err
}
