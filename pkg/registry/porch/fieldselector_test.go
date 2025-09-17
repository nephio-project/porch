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

	"github.com/nephio-project/porch/api/porch/v1alpha1"
	fakeextrepo "github.com/nephio-project/porch/pkg/externalrepo/fake"
	"github.com/nephio-project/porch/pkg/repository"
	"github.com/stretchr/testify/require"
	metainternalversion "k8s.io/apimachinery/pkg/apis/meta/internalversion"
	"k8s.io/apimachinery/pkg/fields"
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
		name        string
		filter      repository.ListPackageRevisionFilter
		p           repository.PackageRevision
		wantMatches bool
	}{
		{
			name:        "nil selector",
			filter:      repository.ListPackageRevisionFilter{},
			p:           &fakeextrepo.FakePackageRevision{},
			wantMatches: true,
		},
		{
			name:   "matching selector",
			filter: repository.ListPackageRevisionFilter{Key: repository.PackageRevisionKey{PkgKey: repository.PackageKey{RepoKey: repository.RepositoryKey{Namespace: "foo"}}}},
			p: &fakeextrepo.FakePackageRevision{
				PrKey: repository.PackageRevisionKey{
					PkgKey: repository.PackageKey{
						RepoKey: repository.RepositoryKey{
							Namespace: "foo",
						},
					},
				},
			},
			wantMatches: true,
		},
		{
			name:        "different selector",
			filter:      repository.ListPackageRevisionFilter{Lifecycles: []v1alpha1.PackageRevisionLifecycle{"Published"}},
			p:           &fakeextrepo.FakePackageRevision{PackageLifecycle: "Published"},
			wantMatches: true,
		},
		{
			name:   "non-matching selector",
			filter: repository.ListPackageRevisionFilter{Key: repository.PackageRevisionKey{PkgKey: repository.PackageKey{RepoKey: repository.RepositoryKey{Namespace: "bar"}}}},
			p: &fakeextrepo.FakePackageRevision{
				PrKey: repository.PackageRevisionKey{
					PkgKey: repository.PackageKey{
						RepoKey: repository.RepositoryKey{
							Namespace: "foo",
						},
					},
				},
			},
			wantMatches: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			gotMatches := tt.filter.Matches(context.TODO(), tt.p)
			require.Equal(t, tt.wantMatches, gotMatches)
		})
	}
}

func Test_parsePackageRevisionFieldSelector(t *testing.T) {
	// SETUP test cases with varying selectors for a packageRevisionFilter
	//********************************************************************
	positiveTests := []struct {
		name       string
		selector   string
		wantFilter repository.ListPackageRevisionFilter
	}{
		{
			name:       "empty selector",
			selector:   "",
			wantFilter: repository.ListPackageRevisionFilter{},
		},
		{
			name:       "revision selector",
			selector:   "spec.revision=1",
			wantFilter: repository.ListPackageRevisionFilter{Key: repository.PackageRevisionKey{Revision: 1}},
		},
		{
			name:       "namespace selector",
			selector:   "metadata.namespace=foo",
			wantFilter: repository.ListPackageRevisionFilter{Key: repository.PackageRevisionKey{PkgKey: repository.PackageKey{RepoKey: repository.RepositoryKey{Namespace: "foo"}}}},
		},
		{
			name:       "namespace selector with == operator",
			selector:   "metadata.namespace==foo",
			wantFilter: repository.ListPackageRevisionFilter{Key: repository.PackageRevisionKey{PkgKey: repository.PackageKey{RepoKey: repository.RepositoryKey{Namespace: "foo"}}}},
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

			require.EqualValues(t, &tt.wantFilter, gotFilter)
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

			wantFilter := &repository.ListPackageRevisionFilter{}
			require.EqualValues(t, wantFilter, gotFilter)
			if tt.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tt.wantErr)
			}
		})
	}
}

func Test_parsePackageRevisionResourcesFieldSelector(t *testing.T) {
	tests := []struct {
		name       string
		selector   string
		wantFilter repository.ListPackageRevisionFilter
	}{
		{
			name:       "empty selector",
			selector:   "",
			wantFilter: repository.ListPackageRevisionFilter{},
		},
		{
			name:       "revision selector",
			selector:   "spec.revision=1",
			wantFilter: repository.ListPackageRevisionFilter{Key: repository.PackageRevisionKey{Revision: 1}},
		},
		{
			name:       "namespace selector",
			selector:   "metadata.namespace=foo",
			wantFilter: repository.ListPackageRevisionFilter{Key: repository.PackageRevisionKey{PkgKey: repository.PackageKey{RepoKey: repository.RepositoryKey{Namespace: "foo"}}}},
		},
		{
			name:       "namespace selector with == operator",
			selector:   "metadata.namespace==foo",
			wantFilter: repository.ListPackageRevisionFilter{Key: repository.PackageRevisionKey{PkgKey: repository.PackageKey{RepoKey: repository.RepositoryKey{Namespace: "foo"}}}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// GIVEN a packageRevisionFilter selecting on the specified fieldSelector
			//***********************************************************************
			fieldSelector, _ := fields.ParseSelector(tt.selector)
			options := &metainternalversion.ListOptions{
				FieldSelector: fieldSelector,
			}

			gotFilter, err := parsePackageRevisionResourcesFieldSelector(options)

			require.EqualValues(t, &tt.wantFilter, gotFilter)
			require.NoError(t, err)
		})
	}
}
