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
	"fmt"
	"reflect"

	api "github.com/nephio-project/porch/api/porch/v1alpha1"
	"github.com/nephio-project/porch/pkg/repository"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metainternalversion "k8s.io/apimachinery/pkg/apis/meta/internalversion"
	"k8s.io/apimachinery/pkg/fields"
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
	selectableFields := reflect.ValueOf(api.PackageRevisionSelectableFields)
	for i := range selectableFields.NumField() {
		if label == selectableFields.Field(i).String() {
			return label, value, nil
		}
	}

	return "", "", fmt.Errorf("%q is not a known field selector", label)
}

// parsePackageFieldSelector parses client-provided fields.Selector into a packageFilter
func parsePackageFieldSelector(fieldSelector fields.Selector) (repository.ListPackageFilter, error) {
	var filter repository.ListPackageFilter

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
			if filterKey, err := repository.PkgK8sName2Key("", requirement.Value); err == nil {
				filter.Key = filterKey
			} else {
				return repository.ListPackageFilter{}, err
			}

		case "metadata.namespace":
			filter.Key.RepoKey.Namespace = requirement.Value

		case "spec.packageName":
			filter.Key.Package = requirement.Value

		case "spec.repository":
			filter.Key.RepoKey.Name = requirement.Value

		default:
			return filter, apierrors.NewBadRequest(fmt.Sprintf("unknown fieldSelector field %q", requirement.Field))
		}
	}

	return filter, nil
}

// packageRevisionFilter filters packages, extending repository.ListPackageRevisionFilter
type packageRevisionFilter struct {
	*repository.ListPackageRevisionFilter
}

func newPackageRevisionFilter() *packageRevisionFilter {
	return &packageRevisionFilter{
		ListPackageRevisionFilter: &repository.ListPackageRevisionFilter{
			Predicate: &storage.SelectionPredicate{},
		},
	}
}

func (f *packageRevisionFilter) Matches(p repository.PackageRevision) (bool, error) {
	if f.Predicate.Field == nil {
		return true, nil
	}

	f.ParseAttrFunc(p)

	return f.Predicate.Matches(repository.Wrap(&p))
}

func (f *packageRevisionFilter) Namespace(ns string) *packageRevisionFilter {
	namespaceSelector := fields.OneTermEqualSelector(api.PackageRevisionSelectableFields.Namespace, ns)

	field := f.Predicate.Field
	if field == nil {
		f.Predicate.Field = namespaceSelector
	} else {
		f.Predicate.Field = fields.AndSelectors(field, namespaceSelector)
	}
	return f
}

func (f *packageRevisionFilter) matchesNamespace(ns string) (bool, string) {
	if f.Predicate.Field == nil {
		return true, ""
	}

	filteredNamespace, filteringOnNamespace := f.Predicate.MatchesSingleNamespace()
	if !filteringOnNamespace {
		return true, ""
	}
	return (ns == "" || ns == filteredNamespace), filteredNamespace
}

func (f *packageRevisionFilter) filteredRepository() string {
	if f.Predicate.Field == nil {
		return ""
	}

	if filteredRepo, filteringOnRepo := f.Predicate.Field.RequiresExactMatch(api.PackageRevisionSelectableFields.Repository); filteringOnRepo {
		return filteredRepo
	}

	return ""
}

// parsePackageRevisionFieldSelector parses client-provided fields.Selector into a packageRevisionFilter
func parsePackageRevisionFieldSelector(options *metainternalversion.ListOptions) (*repository.ListPackageRevisionFilter, error) {
	filter := &repository.ListPackageRevisionFilter{
		Predicate: &storage.SelectionPredicate{
			Label: options.LabelSelector,
		},
	}

	fieldSelector := options.FieldSelector
	if fieldSelector == nil {
		return filter, nil
	}

	for _, requirement := range fieldSelector.Requirements() {

		switch requirement.Operator {
		case selection.Equals, selection.DoubleEquals, selection.NotEquals:
			if requirement.Value == "" {
				return filter, apierrors.NewBadRequest(fmt.Sprintf("unsupported fieldSelector value %q for field %q with operator %q", requirement.Value, requirement.Field, requirement.Operator))
			}
		default:
			return filter, apierrors.NewBadRequest(fmt.Sprintf("unsupported fieldSelector operator %q for field %q", requirement.Operator, requirement.Field))
		}
	}

	filter.Predicate.Field = fieldSelector

	return filter, nil
}

// parsePackageRevisionResourcesFieldSelector parses client-provided fields.Selector into a packageRevisionFilter
func parsePackageRevisionResourcesFieldSelector(options *metainternalversion.ListOptions) (*repository.ListPackageRevisionFilter, error) {
	// TOOD: This is a little weird, because we don't have the same fields on PackageRevisionResources.
	// But we probably should have the key fields
	return parsePackageRevisionFieldSelector(options)
}
