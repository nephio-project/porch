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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func (f *ListPackageRevisionFilter) MatchesNamespace(namespace string) (bool, string) {
	filteredNamespace := f.Key.RKey().Namespace
	return (filteredNamespace == "" || namespace == filteredNamespace), filteredNamespace
}

func (f *ListPackageRevisionFilter) FilteredRepository() string {
	return f.Key.PKey().RKey().Name
}

type wrappedRepoPkgRev struct {
	metav1.TypeMeta
	repoPr PackageRevision
}

func wrap(p *PackageRevision) *wrappedRepoPkgRev {
	return &wrappedRepoPkgRev{repoPr: *p}
}

func (p *wrappedRepoPkgRev) Unwrap() PackageRevision {
	return p.repoPr
}

func (in wrappedRepoPkgRev) DeepCopyObject() runtime.Object {
	return in
}
func (in wrappedRepoPkgRev) GetObjectKind() schema.ObjectKind {
	return schema.EmptyObjectKind
}
