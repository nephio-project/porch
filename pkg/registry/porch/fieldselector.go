// Copyright 2022, 2025 The kpt and Nephio Authors
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

package porch

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	api "github.com/nephio-project/porch/api/porch/v1alpha1"
	"github.com/nephio-project/porch/pkg/repository"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metainternalversion "k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apiserver/pkg/storage"
)

// convertPackageFieldSelector is the schema conversion function for normalizing the FieldSelector for Package
func convertPackageFieldSelector(label, value string) (internalLabel, internalValue string, err error) {
	switch label {
	case "metadata.name":
		return label, value, nil
	case "metadata.namespace":
		return label, value, nil
	case "spec.packageName", "spec.repository":
		return label, value, nil
	default:
		return "", "", fmt.Errorf("%q is not a known field selector", label)
	}
}

// convertPackageRevisionFieldSelector is the schema conversion function for normalizing the FieldSelector for PackageRevision
func convertPackageRevisionFieldSelector(label, value string) (internalLabel, internalValue string, err error) {
	selectableFields := reflect.ValueOf(PkgRevSelectableFields)
	for i := range selectableFields.NumField() {
		if label == selectableFields.Field(i).String() {
			return label, value, nil
		}
	}

	return "", "", fmt.Errorf("%q is not a known field selector", label)
}

// packageFilter filters packages, extending repository.ListPackageFilter
type packageFilter struct {
	repository.ListPackageFilter

	// Namespace filters by the namespace of the objects
	Namespace string

	// Repository restricts to repositories with the given name.
	Repository string
}

// parsePackageFieldSelector parses client-provided fields.Selector into a packageFilter
func parsePackageFieldSelector(fieldSelector fields.Selector) (packageFilter, error) {
	var filter packageFilter

	if fieldSelector == nil {
		return filter, nil
	}

	requirements := fieldSelector.Requirements()
	for _, requirement := range requirements {

		switch requirement.Operator {
		case selection.Equals, selection.DoesNotExist:
			if requirement.Value == "" {
				return filter, apierrors.NewBadRequest(fmt.Sprintf("unsupported fieldSelector value %q for field %q with operator %q", requirement.Value, requirement.Field, requirement.Operator))
			}
		default:
			return filter, apierrors.NewBadRequest(fmt.Sprintf("unsupported fieldSelector operator %q for field %q", requirement.Operator, requirement.Field))
		}

		switch requirement.Field {
		case "metadata.name":
			filter.KubeObjectName = requirement.Value

		case "metadata.namespace":
			filter.Namespace = requirement.Value

		case "spec.packageName":
			filter.Package = requirement.Value
		case "spec.repository":
			filter.Repository = requirement.Value

		default:
			return filter, apierrors.NewBadRequest(fmt.Sprintf("unknown fieldSelector field %q", requirement.Field))
		}
	}

	return filter, nil
}

// packageRevisionFilter filters packages, extending repository.ListPackageRevisionFilter
type packageRevisionFilter struct {
	predicate storage.SelectionPredicate
}

type pkgRevSelectable struct {
	Name          string
	Namespace     string
	Revision      string
	PackageName   string
	Repository    string
	WorkspaceName string
	Lifecycle     string
}

var PkgRevSelectableFields = pkgRevSelectable{
	Name:          "metadata.name",
	Namespace:     "metadata.namespace",
	Revision:      "spec.revision",
	PackageName:   "spec.packageName",
	Repository:    "spec.repository",
	WorkspaceName: "spec.workspaceName",
	Lifecycle:     "spec.lifecycle",
}

func (f *packageRevisionFilter) Matches(ctx context.Context, p api.PackageRevision) (bool, error) {
	if f.predicate.Field == nil {
		return true, nil
	}
	return f.predicate.Matches(&p)
}
func (f *packageRevisionFilter) MatchesRepoRev(p repository.PackageRevision) (bool, error) {
	if f.predicate.Field == nil {
		return true, nil
	}
	return f.predicate.Matches(wrap(&p))
}

func (f *packageRevisionFilter) Namespace(ns string) *packageRevisionFilter {
	if namespaceSelector, err := fields.ParseSelector(PkgRevSelectableFields.Namespace + "=" + ns); err == nil {
		field := f.predicate.Field
		if field == nil {
			f.predicate.Field = namespaceSelector
		} else {
			f.predicate.Field = fields.AndSelectors(f.predicate.Field, namespaceSelector)
		}
	}
	return f
}

func (f *packageRevisionFilter) matchesNamespace(ns string) (bool, string) {
	if f.predicate.Field == nil {
		return true, ""
	}

	filteredNamespace, filteringOnNamespace := f.predicate.MatchesSingleNamespace()
	if !filteringOnNamespace {
		return true, ns
	}
	return (ns != "" && ns != filteredNamespace), filteredNamespace
}

func (f *packageRevisionFilter) filteredRepository() string {
	if f.predicate.Field == nil {
		return ""
	}

	if filteredRepo, filteringOnRepo := f.predicate.Field.RequiresExactMatch(PkgRevSelectableFields.Repository); filteringOnRepo {
		return filteredRepo
	}

	return ""
}

// parsePackageRevisionFieldSelector parses client-provided fields.Selector into a packageRevisionFilter
func parsePackageRevisionFieldSelector(options *metainternalversion.ListOptions) (packageRevisionFilter, error) {
	var filter packageRevisionFilter
	filter.predicate.Label = options.LabelSelector
	fieldSelector := options.FieldSelector
	if fieldSelector == nil {
		return filter, nil
	}

	requirements := fieldSelector.Requirements()
	for _, requirement := range requirements {

		switch requirement.Operator {
		case selection.Equals, selection.DoubleEquals, selection.NotEquals, selection.DoesNotExist:
			if requirement.Value == "" {
				return filter, apierrors.NewBadRequest(fmt.Sprintf("unsupported fieldSelector value %q for field %q with operator %q", requirement.Value, requirement.Field, requirement.Operator))
			}
		default:
			return filter, apierrors.NewBadRequest(fmt.Sprintf("unsupported fieldSelector operator %q for field %q", requirement.Operator, requirement.Field))
		}
	}

	filter.predicate.Field = fieldSelector
	filter.predicate.GetAttrs = pkgRevGetAttrs

	return filter, nil
}

// pkgRevGetAttrs returns labels and fields of a given PackageRevision object for filtering purposes.
func pkgRevGetAttrs(obj runtime.Object) (labels.Set, fields.Set, error) {
	if fieldSet := mapPkgRevFields(obj); fieldSet != nil {
		return nil, fieldSet, nil
	}

	return nil, nil, fmt.Errorf("not a package revision")
}

func mapPkgRevFields(packageRevision any) fields.Set {
	labels := PkgRevSelectableFields
	if p, isApiPkgRev := packageRevision.(*api.PackageRevision); isApiPkgRev {
		return fields.Set{
			labels.Namespace:  p.Namespace,
			labels.Repository: p.Spec.RepositoryName,
		}
	}

	if p, isRepoPkgRev := packageRevision.(*wrappedRepoPkgRev); isRepoPkgRev {
		repoPr := p.unwrap()
		key := repoPr.Key()
		labels := PkgRevSelectableFields
		return fields.Set{
			labels.Name:     repoPr.KubeObjectName(),
			labels.Revision: repository.Revision2Str(key.Revision),
			labels.PackageName: func() string {
				if path := key.PkgKey.Path; path != "" {
					return path + "/"
				}
				return ""
			}() + key.PkgKey.Package,
			labels.WorkspaceName: key.WorkspaceName,
			labels.Lifecycle:     string(repoPr.Lifecycle(context.TODO())),
		}
	}

	return nil
}

// parsePackageRevisionResourcesFieldSelector parses client-provided fields.Selector into a packageRevisionFilter
func parsePackageRevisionResourcesFieldSelector(options *metainternalversion.ListOptions) (packageRevisionFilter, error) {
	// TOOD: This is a little weird, because we don't have the same fields on PackageRevisionResources.
	// But we probably should have the key fields
	return parsePackageRevisionFieldSelector(options)
}

func (f *packageRevisionFilter) toListPackageRevisionFilter() (lowerFilter repository.ListPackageRevisionFilter) {
	labels := PkgRevSelectableFields
	field := f.predicate.Field
	if field == nil {
		return
	}

	if filteredName, filteringOnName := field.RequiresExactMatch(labels.Name); filteringOnName {
		lowerFilter.KubeObjectName = filteredName
	}

	if filteredRevision, filteringOnRevision := field.RequiresExactMatch(labels.Revision); filteringOnRevision {
		lowerFilter.Key.Revision = repository.Revision2Int(filteredRevision)
	}

	if filteredPackage, filteringOnPackage := field.RequiresExactMatch(labels.PackageName); filteringOnPackage {
		if segs := strings.Split(filteredPackage, "/"); len(segs) > 1 {
			lowerFilter.Key.PkgKey.Path = strings.Join(segs[:len(segs)-1], "/")
			filteredPackage = segs[len(segs)]
		}
		lowerFilter.Key.PkgKey.Package = filteredPackage
	}

	if filteredWorkspace, filteringOnWorkspace := field.RequiresExactMatch(labels.WorkspaceName); filteringOnWorkspace {
		lowerFilter.Key.WorkspaceName = filteredWorkspace
	}

	if filteredLifecycle, filteringOnLifecycle := field.RequiresExactMatch(labels.WorkspaceName); filteringOnLifecycle {
		lowerFilter.Lifecycle = api.PackageRevisionLifecycle(filteredLifecycle)
	}

	return
}

type wrappedRepoPkgRev struct {
	metav1.TypeMeta
	repoPr repository.PackageRevision
}

func wrap(p *repository.PackageRevision) *wrappedRepoPkgRev {
	return &wrappedRepoPkgRev{repoPr: *p}
}

func (p *wrappedRepoPkgRev) unwrap() repository.PackageRevision {
	return p.repoPr
}

func (in wrappedRepoPkgRev) DeepCopyObject() runtime.Object {
	return in
}
func (in wrappedRepoPkgRev) GetObjectKind() schema.ObjectKind {
	return schema.EmptyObjectKind
}
