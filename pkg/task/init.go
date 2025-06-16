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

package task

import (
	"context"
	"fmt"

	api "github.com/nephio-project/porch/api/porch/v1alpha1"
	"github.com/nephio-project/porch/pkg/kpt/kptpkg"
	"github.com/nephio-project/porch/pkg/kpt/printer"
	"github.com/nephio-project/porch/pkg/kpt/printer/fake"
	"github.com/nephio-project/porch/pkg/repository"
	"github.com/nephio-project/porch/pkg/util"
	"go.opentelemetry.io/otel/trace"
	"sigs.k8s.io/kustomize/kyaml/filesys"
)

type initPackageMutation struct {
	kptpkg.DefaultInitializer
	pkgRev *api.PackageRevision
	task   *api.Task
}

var _ mutation = &initPackageMutation{}

func (m *initPackageMutation) apply(ctx context.Context, resources repository.PackageResources) (repository.PackageResources, *api.TaskResult, error) {
	ctx, span := tracer.Start(ctx, "initPackageMutation::apply", trace.WithAttributes())
	defer span.End()

	fs := filesys.MakeFsInMemory()
	// virtual fs expected a rooted filesystem
	pkgPath := "/"

	if m.task.Init.Subpackage != "" {
		pkgPath = "/" + m.task.Init.Subpackage
	}
	if err := fs.Mkdir(pkgPath); err != nil {
		return repository.PackageResources{}, nil, err
	}

	readinessConditions := util.MergeFunc(DefaultReadinessConditions, m.pkgRev.Status.Conditions, func(aDefault, anInput api.Condition) bool {
		return aDefault.Type == anInput.Type
	})

	name := m.pkgRev.Spec.PackageName
	initSpec := m.task.Init

	err := m.Initialize(printer.WithContext(ctx, &fake.Printer{}), fs, kptpkg.InitOptions{
		PkgPath:             pkgPath,
		PkgName:             name,
		Desc:                initSpec.Description,
		Keywords:            initSpec.Keywords,
		Site:                initSpec.Site,
		ReadinessConditions: readinessConditions,
	})
	if err != nil {
		return repository.PackageResources{}, nil, fmt.Errorf("failed to initialize pkg %q: %w", name, err)
	}

	initializedResources, err := readResources(fs)
	if err != nil {
		return repository.PackageResources{}, nil, err
	}

	return initializedResources, &api.TaskResult{Task: m.task}, nil
}
