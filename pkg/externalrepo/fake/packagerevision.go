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

func (c *FakePackageRevision) KubeObjectName() string {
	return repository.ComposePkgRevObjName(c.Key())
}

func (c *FakePackageRevision) KubeObjectNamespace() string {
	return c.Key().RKey().Namespace
}

func (c *FakePackageRevision) UID() types.UID {
	return util.GenerateUid("packagerevision:", c.KubeObjectNamespace(), c.KubeObjectName())
}

var _ repository.PackageRevision = &FakePackageRevision{}

// ToMainPackageRevision implements repository.PackageRevision.
func (f *FakePackageRevision) ToMainPackageRevision(context.Context) repository.PackageRevision {
	panic("unimplemented")
}

func (pr *FakePackageRevision) ResourceVersion() string {
	return pr.PackageRevision.ResourceVersion
}

func (pr *FakePackageRevision) Key() repository.PackageRevisionKey {
	return pr.PrKey
}

func (pr *FakePackageRevision) Lifecycle(ctx context.Context) v1alpha1.PackageRevisionLifecycle {
	return pr.PackageLifecycle
}

func (pr *FakePackageRevision) GetPackageRevision(context.Context) (*v1alpha1.PackageRevision, error) {
	return pr.PackageRevision, pr.Err
}

func (pr *FakePackageRevision) GetResources(context.Context) (*v1alpha1.PackageRevisionResources, error) {
	return pr.Resources, pr.Err
}

func (pr *FakePackageRevision) GetKptfile(ctx context.Context) (kptfile.KptFile, error) {
	pr.Ops = append(pr.Ops, "GetKptfile")
	return pr.Kptfile, pr.Err
}

func (pr *FakePackageRevision) GetUpstreamLock(context.Context) (kptfile.Upstream, kptfile.UpstreamLock, error) {
	pr.Ops = append(pr.Ops, "GetUptreamLock")
	return *pr.Kptfile.Upstream, *pr.Kptfile.UpstreamLock, pr.Err
}

func (pr *FakePackageRevision) GetLock() (kptfile.Upstream, kptfile.UpstreamLock, error) {
	pr.Ops = append(pr.Ops, "GetLock")
	return *pr.Kptfile.Upstream, *pr.Kptfile.UpstreamLock, pr.Err
}

func (pr *FakePackageRevision) UpdateLifecycle(_ context.Context, lifecycle v1alpha1.PackageRevisionLifecycle) error {
	pr.Ops = append(pr.Ops, "UpdateLifecycle")
	pr.PackageLifecycle = lifecycle
	if pr.PackageRevision != nil {
		pr.PackageRevision.Spec.Lifecycle = lifecycle
	}
	return pr.Err
}

func (pr *FakePackageRevision) GetMeta() metav1.ObjectMeta {
	return metav1.ObjectMeta{}
}

func (pr *FakePackageRevision) SetMeta(context.Context, metav1.ObjectMeta) error {
	pr.Ops = append(pr.Ops, "SetMeta")
	return pr.Err
}

func (pr *FakePackageRevision) UpdateResources(ctx context.Context, new *v1alpha1.PackageRevisionResources, change *v1alpha1.Task) error {
	pr.Ops = append(pr.Ops, "UpdateResources")
	pr.Resources = new
	return pr.Err
}
