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

package porch

import (
	"context"
	"testing"

	api "github.com/nephio-project/porch/api/porch/v1alpha1"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestPackageRevisionStrategyValidate(t *testing.T) {
	s := packageRevisionStrategy{}

	testCases := map[string]struct {
		obj   *api.PackageRevision
		valid bool
	}{
		"cannot set latest-revision label during creation": {
			obj: &api.PackageRevision{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						api.LatestPackageRevisionKey: "true",
					},
				},
				Spec: api.PackageRevisionSpec{
					PackageName:    "test-package",
					WorkspaceName:  "test-workspace",
					RepositoryName: "test-repo",
					Lifecycle:      api.PackageRevisionLifecycleDraft,
				},
			},
			valid: false,
		},
		"can create without latest-revision label": {
			obj: &api.PackageRevision{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"other-label": "value",
					},
				},
				Spec: api.PackageRevisionSpec{
					PackageName:    "test-package",
					WorkspaceName:  "test-workspace",
					RepositoryName: "test-repo",
					Lifecycle:      api.PackageRevisionLifecycleDraft,
				},
			},
			valid: true,
		},
	}

	for tn := range testCases {
		tc := testCases[tn]
		t.Run(tn, func(t *testing.T) {
			ctx := context.Background()
			allErrs := s.Validate(ctx, tc.obj)

			if tc.valid {
				assert.Empty(t, allErrs, "Create validation failed unexpectedly")
			} else {
				assert.NotEmpty(t, allErrs, "Create validation should fail but didn't")
			}
		})
	}
}
