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
	"github.com/nephio-project/porch/pkg/repository"
)

// Implementation of the repository.PackageRevision interface for testing.
type FakePackage struct {
	pkgKey           repository.PackageKey
	PackageRevisions []repository.PackageRevision
	Packages         []repository.Package
}

var _ repository.Package = &FakePackage{}

func (p *FakePackage) KubeObjectName() string {
	return repository.ComposePkgObjName(p.pkgKey)
}

func (p *FakePackage) Key() repository.PackageKey {
	return p.pkgKey
}

func (p *FakePackage) GetPackage(ctx context.Context) *v1alpha1.PorchPackage {
	return &v1alpha1.PorchPackage{}
}

func (p *FakePackage) GetLatestRevision(ctx context.Context) int {
	return 0
}
