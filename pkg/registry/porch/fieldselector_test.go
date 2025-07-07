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
	"testing"

	"github.com/nephio-project/porch/pkg/repository"
	"github.com/stretchr/testify/require"
	metainternalversion "k8s.io/apimachinery/pkg/apis/meta/internalversion"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apiserver/pkg/storage"
)

func Test_convertPackageRevisionFieldSelector(t *testing.T) {
	type args struct {
		label string
		value string
	}
	positiveTests := []args{
		{label: "metadata.name", value: "foo"},
		{label: "metadata.namespace", value: "foo"},
		{label: "spec.revision", value: "foo"},
		{label: "spec.packageName", value: "foo"},
		{label: "spec.repository", value: "foo"},
		{label: "spec.workspaceName", value: "foo"},
		{label: "spec.lifecycle", value: "foo"},
	}
	for _, tt := range positiveTests {
		t.Run(tt.label, func(t *testing.T) {
			gotInternalLabel, gotInternalValue, err := convertPackageRevisionFieldSelector(tt.label, tt.value)
			require.NoError(t, err)
			require.Equal(t, tt.label, gotInternalLabel)
			require.Equal(t, tt.value, gotInternalValue)
		})
	}

	negativeTests := []args{
		{label: "metadata.nonesuch", value: "foo"},
	}
	for _, tt := range negativeTests {
		t.Run(tt.label, func(t *testing.T) {
			gotInternalLabel, gotInternalValue, err := convertPackageRevisionFieldSelector(tt.label, tt.value)
			require.EqualError(t, err, fmt.Sprintf("%q is not a known field selector", tt.label))
			require.Equal(t, "", gotInternalLabel)
			require.Equal(t, "", gotInternalValue)
		})
	}
}

func Test_packageRevisionFilter_Matches(t *testing.T) {
	tests := []struct {
		name          string
		fieldSelector fields.Selector
		p             repository.PackageRevision
		wantMatches   bool
	}{
		{
			name:          "nil selector",
			fieldSelector: nil,
			p:             &fakePackageRevision{},
			wantMatches:   true,
		},
		{
			name:          "matching selector",
			fieldSelector: fields.OneTermEqualSelector("metadata.namespace", "foo"),
			p:             &fakePackageRevision{namespace: "foo"},
			wantMatches:   true,
		},
		{
			name:          "different selector",
			fieldSelector: fields.OneTermEqualSelector("spec.lifecycle", "Published"),
			p:             &fakePackageRevision{lifecycle: "Published"},
			wantMatches:   true,
		},
		{
			name:          "non-matching selector",
			fieldSelector: fields.OneTermEqualSelector("metadata.namespace", "bar"),
			p:             &fakePackageRevision{namespace: "foo"},
			wantMatches:   false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter := &repository.ListPackageRevisionFilter{
				Predicate: &storage.SelectionPredicate{
					Field: tt.fieldSelector,
					Label: labels.Everything(),
				},
			}

			gotMatches := filter.Matches(context.TODO(), tt.p)
			require.Equal(t, tt.wantMatches, gotMatches)
		})
	}
}

func Test_packageRevisionFilter_Namespace(t *testing.T) {
	// SETUP test cases with varying selector strings for a packageRevisionFilter
	//***************************************************************************
	tests := []struct {
		existingSelectors string
	}{
		{existingSelectors: "spec.revision=bar,spec.repository=baz"},
		{existingSelectors: ""},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("selectors %q", tt.existingSelectors), func(t *testing.T) {
			// GIVEN a packageRevisionFilter selecting on the specified fieldSelector
			//***********************************************************************
			f := newPackageRevisionFilter()
			f.Predicate.Field, _ = fields.ParseSelector(tt.existingSelectors)

			// WHEN we set the filter to match a particular namespace
			//*******************************************************
			ns := "foo"
			got := f.Namespace(ns)

			// THEN the filter is updated to add the 'metadata.namespace' selector
			//      to any existing selectors
			//********************************************************************
			want := newPackageRevisionFilter()
			existing, _ := fields.ParseSelector(tt.existingSelectors)
			want.Predicate.Field = fields.AndSelectors(existing, fields.OneTermEqualSelector("metadata.namespace", ns))
			require.EqualValues(t, want, got)
		})
	}
}

func Test_packageRevisionFilter_matchesNamespace(t *testing.T) {
	// SETUP test cases with varying selectors for a packageRevisionFilter
	//********************************************************************
	tests := []struct {
		name                  string
		fieldSelector         fields.Selector
		wantMatches           bool
		wantFilteredNamespace string
	}{
		{
			name:                  "nil selector",
			fieldSelector:         nil,
			wantMatches:           true,
			wantFilteredNamespace: "",
		},
		{
			name:                  "matching selector",
			fieldSelector:         fields.Set{"metadata.namespace": "foo"}.AsSelector(),
			wantMatches:           true,
			wantFilteredNamespace: "foo",
		},
		{
			name:                  "non-matching selector",
			fieldSelector:         fields.Set{"metadata.namespace": "bar"}.AsSelector(),
			wantMatches:           false,
			wantFilteredNamespace: "bar",
		},
		{
			name:                  "different selector",
			fieldSelector:         fields.Set{"metadata.name": "something"}.AsSelector(),
			wantMatches:           true,
			wantFilteredNamespace: "",
		},
	}

	for _, tt := range tests {
		// GIVEN a packageRevisionFilter selecting on the specified fieldSelector
		//***********************************************************************
		t.Run(tt.name, func(t *testing.T) {
			filter := &repository.ListPackageRevisionFilter{
				Predicate: &storage.SelectionPredicate{
					Field: tt.fieldSelector,
				},
			}

			// WHEN we check if the filter matches a particular namespace
			//***********************************************************
			gotMatches, gotFilteredNamespace := filter.MatchesNamespace("foo")

			// THEN the filter returns expected values for whether the namespace
			//      matches, and what namespace is being matched against
			//******************************************************************
			require.Equal(t, tt.wantMatches, gotMatches)
			require.Equal(t, tt.wantFilteredNamespace, gotFilteredNamespace)
		})
	}
}

func Test_packageRevisionFilter_filteredRepository(t *testing.T) {
	// SETUP test cases with varying selectors for a packageRevisionFilter
	//********************************************************************
	tests := []struct {
		name             string
		fieldSelector    fields.Selector
		wantFilteredRepo string
	}{
		{
			name:             "nil selector",
			fieldSelector:    nil,
			wantFilteredRepo: "",
		},
		{
			name:             "matching selector",
			fieldSelector:    fields.Set{"spec.repository": "foo"}.AsSelector(),
			wantFilteredRepo: "foo",
		},
		{
			name:             "different selector",
			fieldSelector:    fields.Set{"metadata.name": "something"}.AsSelector(),
			wantFilteredRepo: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// GIVEN a packageRevisionFilter selecting on the specified fieldSelector
			//***********************************************************************
			filter := &repository.ListPackageRevisionFilter{
				Predicate: &storage.SelectionPredicate{
					Field: tt.fieldSelector,
				},
			}
			// WHEN we get the filter's repository selector
			//***********************************************************
			gotFilteredRepo := filter.FilteredRepository()

			// THEN the filter returns expected values for the repo being matched against
			//***************************************************************************
			require.Equal(t, tt.wantFilteredRepo, gotFilteredRepo)
		})
	}
}

func Test_parsePackageRevisionFieldSelector(t *testing.T) {
	// SETUP test cases with varying selectors for a packageRevisionFilter
	//********************************************************************
	positiveTests := []struct {
		name         string
		selector     string
		wantSelector fields.Selector
		wantErr      bool
	}{
		{
			name:         "empty selector",
			selector:     "",
			wantSelector: fields.Set{}.AsSelector(),
		},
		{
			name:         "revision selector",
			selector:     "spec.revision=1",
			wantSelector: fields.Set{"spec.revision": "1"}.AsSelector(),
		},
		{
			name:         "namespace selector",
			selector:     "metadata.namespace=foo",
			wantSelector: fields.Set{"metadata.namespace": "foo"}.AsSelector(),
		},
		{
			name:         "namespace selector with == operator",
			selector:     "metadata.namespace==foo",
			wantSelector: fields.Set{"metadata.namespace": "foo"}.AsSelector(),
		},
		{
			name:     "namespace selector with != operator",
			selector: "metadata.namespace!=foo",
			wantSelector: func() fields.Selector {
				selector, _ := fields.ParseSelector("metadata.namespace!=foo")
				return selector
			}(),
		},
	}
	for _, tt := range positiveTests {
		t.Run(tt.name, func(t *testing.T) {
			// GIVEN a packageRevisionFilter selecting on the specified fieldSelector
			//***********************************************************************
			fieldSelector, _ := fields.ParseSelector(tt.selector)
			options := &metainternalversion.ListOptions{
				FieldSelector: fieldSelector,
			}

			gotFilter, err := parsePackageRevisionFieldSelector(options)

			wantFilter := &repository.ListPackageRevisionFilter{
				Predicate: &storage.SelectionPredicate{
					Field: tt.wantSelector,
				},
			}
			require.EqualValues(t, wantFilter, gotFilter)
			require.NoError(t, err)
		})
	}

	// SETUP test cases with varying invalid/errored selectors for a packageRevisionFilter
	//************************************************************************************
	negativeTests := []struct {
		name     string
		selector fields.Selector
		wantErr  string
	}{
		{
			name:     "nil selector",
			selector: nil,
			wantErr:  "",
		},
		{
			name:     "empty selector value",
			selector: fields.Set{"metadata.namespace": ""}.AsSelector(),
			wantErr:  "unsupported fieldSelector value",
		},
	}
	for _, tt := range negativeTests {
		t.Run(tt.name, func(t *testing.T) {
			// GIVEN a packageRevisionFilter selecting on the specified fieldSelector
			//***********************************************************************
			options := &metainternalversion.ListOptions{
				FieldSelector: tt.selector,
			}

			gotFilter, err := parsePackageRevisionFieldSelector(options)

			wantFilter := &repository.ListPackageRevisionFilter{
				Predicate: &storage.SelectionPredicate{
					Field: nil,
				},
			}
			require.EqualValues(t, wantFilter, gotFilter)
			if tt.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tt.wantErr)
			}
		})
	}
}
