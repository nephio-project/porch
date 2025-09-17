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
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestListPackageRevisionFilter_MatchesNamespace(t *testing.T) {
	// SETUP test cases with varying selectors for a packageRevisionFilter
	//********************************************************************
	tests := []struct {
		name                  string
		filter                *ListPackageRevisionFilter
		wantMatches           bool
		wantFilteredNamespace string
	}{
		{
			name:                  "nil key",
			filter:                &ListPackageRevisionFilter{},
			wantMatches:           true,
			wantFilteredNamespace: "",
		},
		{
			name:                  "matching key",
			filter:                &ListPackageRevisionFilter{Key: PackageRevisionKey{PkgKey: PackageKey{RepoKey: RepositoryKey{Namespace: "foo"}}}},
			wantMatches:           true,
			wantFilteredNamespace: "foo",
		},
		{
			name:                  "non-matching key",
			filter:                &ListPackageRevisionFilter{Key: PackageRevisionKey{PkgKey: PackageKey{RepoKey: RepositoryKey{Namespace: "bar"}}}},
			wantMatches:           false,
			wantFilteredNamespace: "bar",
		},
		{
			name:                  "different key",
			filter:                &ListPackageRevisionFilter{Key: PackageRevisionKey{PkgKey: PackageKey{Package: "something"}}},
			wantMatches:           true,
			wantFilteredNamespace: "",
		},
	}

	for _, tt := range tests {
		// GIVEN a packageRevisionFilter selecting on the specified PackageRevisionKey
		//****************************************************************************
		t.Run(tt.name, func(t *testing.T) {
			filter := tt.filter

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

func TestListPackageRevisionFilter_FilteredRepository(t *testing.T) {
	// SETUP test cases with varying selectors for a packageRevisionFilter
	//********************************************************************
	tests := []struct {
		name             string
		filter           *ListPackageRevisionFilter
		wantFilteredRepo string
	}{
		{
			name:             "nil key",
			filter:           &ListPackageRevisionFilter{},
			wantFilteredRepo: "",
		},
		{
			name:             "matching key",
			filter:           &ListPackageRevisionFilter{Key: PackageRevisionKey{PkgKey: PackageKey{RepoKey: RepositoryKey{Name: "foo"}}}},
			wantFilteredRepo: "foo",
		},
		{
			name:             "different key",
			filter:           &ListPackageRevisionFilter{Key: PackageRevisionKey{PkgKey: PackageKey{Package: "something"}}},
			wantFilteredRepo: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// GIVEN a packageRevisionFilter selecting on the specified filter
			//****************************************************************
			// WHEN we get the filter's repository selector
			//*********************************************
			gotFilteredRepo := tt.filter.FilteredRepository()

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
	in := wrappedRepoPkgRev{repoPr: &fakePackageRevision{}}
	got := in.GetObjectKind()
	require.Equal(t, schema.EmptyObjectKind, got)

}
