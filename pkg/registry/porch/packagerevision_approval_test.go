// Copyright 2022 The kpt and Nephio Authors
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
	"testing"

	api "github.com/nephio-project/porch/api/porch/v1alpha1"
)

func TestApprovalUpdateStrategy(t *testing.T) {
	s := packageRevisionApprovalStrategy{}

	type testCase struct {
		old     api.PorchPkgRevisionLifecycle
		valid   []api.PorchPkgRevisionLifecycle
		invalid []api.PorchPkgRevisionLifecycle
	}

	for _, tc := range []testCase{
		{
			old:     "",
			valid:   []api.PorchPkgRevisionLifecycle{},
			invalid: []api.PorchPkgRevisionLifecycle{"", "Wrong", api.PorchPkgRevisionLifecycleDraft, api.PorchPkgRevisionLifecycleProposed, api.PorchPkgRevisionLifecyclePublished, api.PorchPkgRevisionLifecycleDeletionProposed},
		},
		{
			old:     "Wrong",
			valid:   []api.PorchPkgRevisionLifecycle{},
			invalid: []api.PorchPkgRevisionLifecycle{"", "Wrong", api.PorchPkgRevisionLifecycleDraft, api.PorchPkgRevisionLifecycleProposed, api.PorchPkgRevisionLifecyclePublished, api.PorchPkgRevisionLifecycleDeletionProposed},
		},
		{
			old:     api.PorchPkgRevisionLifecycleDraft,
			valid:   []api.PorchPkgRevisionLifecycle{},
			invalid: []api.PorchPkgRevisionLifecycle{"", "Wrong", api.PorchPkgRevisionLifecycleDraft, api.PorchPkgRevisionLifecycleProposed, api.PorchPkgRevisionLifecyclePublished, api.PorchPkgRevisionLifecycleDeletionProposed},
		},
		{
			old:     api.PorchPkgRevisionLifecyclePublished,
			valid:   []api.PorchPkgRevisionLifecycle{api.PorchPkgRevisionLifecycleDeletionProposed},
			invalid: []api.PorchPkgRevisionLifecycle{"", "Wrong", api.PorchPkgRevisionLifecycleDraft, api.PorchPkgRevisionLifecycleProposed, api.PorchPkgRevisionLifecyclePublished},
		},
		{
			old:     api.PorchPkgRevisionLifecycleDeletionProposed,
			valid:   []api.PorchPkgRevisionLifecycle{api.PorchPkgRevisionLifecyclePublished},
			invalid: []api.PorchPkgRevisionLifecycle{"", "Wrong", api.PorchPkgRevisionLifecycleDraft, api.PorchPkgRevisionLifecycleProposed, api.PorchPkgRevisionLifecycleDeletionProposed},
		},
		{
			old:     api.PorchPkgRevisionLifecycleProposed,
			valid:   []api.PorchPkgRevisionLifecycle{api.PorchPkgRevisionLifecycleDraft, api.PorchPkgRevisionLifecyclePublished},
			invalid: []api.PorchPkgRevisionLifecycle{"", "Wrong", api.PorchPkgRevisionLifecycleProposed, api.PorchPkgRevisionLifecycleDeletionProposed},
		},
	} {
		for _, new := range tc.valid {
			testValidateUpdate(t, s, tc.old, new, true)
		}
		for _, new := range tc.invalid {
			testValidateUpdate(t, s, tc.old, new, false)
		}
	}
}
