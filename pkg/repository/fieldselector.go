// Copyright 2025 The kpt and Nephio Authors
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

package repository

import (
	"context"

	api "github.com/nephio-project/porch/api/porch/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type prFieldMappingFunc func(p PackageRevision) string

var (
	RepoPrFilterMappings = map[string]prFieldMappingFunc{
		api.PackageRevisionSelectableFields.Name: func(p PackageRevision) string {
			return p.KubeObjectName()
		},
		api.PackageRevisionSelectableFields.Namespace: func(p PackageRevision) string {
			return p.KubeObjectNamespace()
		},
		api.PackageRevisionSelectableFields.Revision: func(p PackageRevision) string {
			return Revision2Str(p.Key().Revision)
		},
		api.PackageRevisionSelectableFields.PackageName: func(p PackageRevision) string {
			key := p.Key()
			return func() string {
				if path := key.PkgKey.Path; path != "" {
					return path + "/"
				}
				return ""
			}() + key.PkgKey.Package
		},
		api.PackageRevisionSelectableFields.Repository: func(p PackageRevision) string {
			return p.Key().PkgKey.RepoKey.Name
		},
		api.PackageRevisionSelectableFields.WorkspaceName: func(p PackageRevision) string {
			return p.Key().WorkspaceName
		},
		api.PackageRevisionSelectableFields.Lifecycle: func(p PackageRevision) string {
			return string(p.Lifecycle(context.TODO()))
		},
	}
)

type WrappedRepoPkgRev struct {
	metav1.TypeMeta
	repoPr PackageRevision
}

func Wrap(p *PackageRevision) *WrappedRepoPkgRev {
	return &WrappedRepoPkgRev{repoPr: *p}
}

func (p *WrappedRepoPkgRev) Unwrap() PackageRevision {
	return p.repoPr
}

func (in WrappedRepoPkgRev) DeepCopyObject() runtime.Object {
	return in
}
func (in WrappedRepoPkgRev) GetObjectKind() schema.ObjectKind {
	return schema.EmptyObjectKind
}
