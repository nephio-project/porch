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

	"github.com/nephio-project/porch/api/porch/v1alpha1"
	"github.com/nephio-project/porch/pkg/repository"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/selection"
)

// convertPackageRevisionFieldSelector is the schema conversion function for normalizing the the FieldSelector for PackageRevision
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

// convertPackageRevisionFieldSelector is the schema conversion function for normalizing the the FieldSelector for PackageRevision
func convertPackageRevisionFieldSelector(label, value string) (internalLabel, internalValue string, err error) {
	switch label {
	case "metadata.name":
		return label, value, nil
	case "metadata.namespace":
		return label, value, nil
	case "spec.revision", "spec.packageName", "spec.repository":
		return label, value, nil
	case "spec.workspaceName", "spec.lifecycle":
		return label, value, nil
	default:
		return "", "", fmt.Errorf("%q is not a known field selector", label)
	}
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

// parsePackageRevisionFieldSelector parses client-provided fields.Selector into a packageRevisionFilter
func parsePackageRevisionFieldSelector(fieldSelector fields.Selector) (repository.ListPackageRevisionFilter, error) {
	var filter repository.ListPackageRevisionFilter

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
			if filterKey, err := repository.PkgRevK8sName2Key("", requirement.Value); err == nil {
				filter.Key = filterKey
			} else {
				return repository.ListPackageRevisionFilter{}, err
			}

		case "metadata.namespace":
			filter.Key.PkgKey.RepoKey.Namespace = requirement.Value

		case "spec.revision":
			filter.Key.Revision = repository.Revision2Int(requirement.Value)

		case "spec.packageName":
			filter.Key.PkgKey.Package = requirement.Value

		case "spec.repository":
			filter.Key.PkgKey.RepoKey.Name = requirement.Value

		case "spec.workspaceName":
			filter.Key.WorkspaceName = requirement.Value

		case "spec.lifecycle":
			v := v1alpha1.PackageRevisionLifecycle(requirement.Value)
			switch v {
			case v1alpha1.PackageRevisionLifecycleDraft,
				v1alpha1.PackageRevisionLifecycleProposed,
				v1alpha1.PackageRevisionLifecyclePublished,
				v1alpha1.PackageRevisionLifecycleDeletionProposed:
				filter.Lifecycles = []v1alpha1.PackageRevisionLifecycle{v}
			default:
				return filter, apierrors.NewBadRequest(fmt.Sprintf("unsupported fieldSelector value %q for field %q", requirement.Value, requirement.Field))
			}

		default:
			return filter, apierrors.NewBadRequest(fmt.Sprintf("unknown fieldSelector field %q", requirement.Field))
		}
	}

	return filter, nil
}

// parsePackageRevisionResourcesFieldSelector parses client-provided fields.Selector into a packageRevisionFilter
func parsePackageRevisionResourcesFieldSelector(fieldSelector fields.Selector) (repository.ListPackageRevisionFilter, error) {
	// TOOD: This is a little weird, because we don't have the same fields on PackageRevisionResources.
	// But we probably should have the key fields
	return parsePackageRevisionFieldSelector(fieldSelector)
}
