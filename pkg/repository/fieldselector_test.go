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
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/storage"
)

func TestListPackageRevisionFilter_Namespace(t *testing.T) {
	// SETUP test cases with varying selector strings for a packageRevisionFilter
	//***************************************************************************
	tests := []struct {
		existingSelectors fields.Selector
	}{
		{existingSelectors: fields.Set{"spec.revision": "bar", "spec.repository": "baz"}.AsSelector()},
		{existingSelectors: nil},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("selectors %q", tt.existingSelectors), func(t *testing.T) {
			// GIVEN a packageRevisionFilter selecting on the specified fieldSelector
			//***********************************************************************
			f := &ListPackageRevisionFilter{Predicate: &storage.SelectionPredicate{Field: tt.existingSelectors}}

			// WHEN we set the filter to match a particular namespace
			//*******************************************************
			ns := "foo"
			got := f.Namespace(ns)

			// THEN the filter is updated to add the 'metadata.namespace' selector
			//      to any existing selectors
			//********************************************************************
			want := &ListPackageRevisionFilter{
				Predicate: &storage.SelectionPredicate{
					Field: func() fields.Selector {
						if tt.existingSelectors == nil {
							return fields.OneTermEqualSelector("metadata.namespace", ns)
						}
						return fields.AndSelectors(tt.existingSelectors, fields.OneTermEqualSelector("metadata.namespace", ns))
					}(),
				},
			}
			require.EqualValues(t, want, got)
		})
	}
	t.Run("nil predicate", func(t *testing.T) {
		// GIVEN a packageRevisionFilter selecting on the specified fieldSelector
		//***********************************************************************
		ns := "foo"
		f := &ListPackageRevisionFilter{Key: PackageRevisionKey{PkgKey: PackageKey{RepoKey: RepositoryKey{Namespace: ns}}}}

		// WHEN we set the filter to match a particular namespace
		//*******************************************************
		got := f.Namespace(ns)

		// THEN the filter is updated to add the 'metadata.namespace' selector
		//      to any existing selectors
		//********************************************************************
		want := &ListPackageRevisionFilter{
			Predicate: nil,
			Key:       PackageRevisionKey{PkgKey: PackageKey{RepoKey: RepositoryKey{Namespace: ns}}},
		}
		require.EqualValues(t, want, got)
	})
}

func TestListPackageRevisionFilter_MatchesNamespace(t *testing.T) {
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
			filter := &ListPackageRevisionFilter{
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

	t.Run("nil predicate", func(t *testing.T) {
		// GIVEN a packageRevisionFilter selecting on the specified namespace
		//*******************************************************************
		ns := "foo"
		f := &ListPackageRevisionFilter{Key: PackageRevisionKey{PkgKey: PackageKey{RepoKey: RepositoryKey{Namespace: ns}}}}

		// WHEN we check if the filter matches a particular namespace
		//***********************************************************
		gotMatches, gotFilteredNamespace := f.MatchesNamespace("foo")

		// THEN the filter returns expected values for whether the namespace
		//      matches, and what namespace is being matched against
		//******************************************************************
		require.True(t, gotMatches)
		require.Equal(t, "foo", gotFilteredNamespace)
	})
}

func TestListPackageRevisionFilter_FilteredRepository(t *testing.T) {
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
			filter := &ListPackageRevisionFilter{
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

func Test_wrap(t *testing.T) {
	tests := []struct {
		name string
		p    PackageRevision
	}{
		{
			name: "basic wrap",
			p:    &fakePackageRevision{name: "wrapping subject"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			got := wrap(&tt.p)

			want := &wrappedRepoPkgRev{repoPr: &fakePackageRevision{name: "wrapping subject"}}
			require.EqualValues(t, want, got)
		})
	}
}

func TestWrappedRepoPkgRev_Unwrap(t *testing.T) {
	tests := []struct {
		name string
		p    wrappedRepoPkgRev
	}{
		{
			name: "basic wrap",
			p:    wrappedRepoPkgRev{repoPr: &fakePackageRevision{name: "wrapped up"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			got := tt.p.Unwrap()

			want := &fakePackageRevision{name: "wrapped up"}
			require.EqualValues(t, want, got)
		})
	}
}

func TestWrappedRepoPkgRev_DeepCopyObject(t *testing.T) {
	in := wrappedRepoPkgRev{repoPr: &fakePackageRevision{name: "wrapped up"}}
	got := in.DeepCopyObject()
	require.Equal(t, in, got)
}

func TestWrappedRepoPkgRev_GetObjectKind(t *testing.T) {
	type fields struct {
		TypeMeta metav1.TypeMeta
		repoPr   PackageRevision
	}
	tests := []struct {
		name   string
		fields fields
		want   schema.ObjectKind
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := wrappedRepoPkgRev{repoPr: &fakePackageRevision{}}
			got := in.GetObjectKind()
			require.Equal(t, schema.EmptyObjectKind, got)
		})
	}
}
