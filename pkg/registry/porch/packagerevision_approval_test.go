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

	porchapi "github.com/nephio-project/porch/api/porch"
)

func TestApprovalUpdateStrategy(t *testing.T) {
	s := packageRevisionApprovalStrategy{}

	type testCase struct {
		old     porchapi.PackageRevisionLifecycle
		valid   []porchapi.PackageRevisionLifecycle
		invalid []porchapi.PackageRevisionLifecycle
	}

	for _, tc := range []testCase{
		{
			old:     "",
			valid:   []porchapi.PackageRevisionLifecycle{},
			invalid: []porchapi.PackageRevisionLifecycle{"", "Wrong", porchapi.PackageRevisionLifecycleDraft, porchapi.PackageRevisionLifecycleProposed, porchapi.PackageRevisionLifecyclePublished, porchapi.PackageRevisionLifecycleDeletionProposed},
		},
		{
			old:     "Wrong",
			valid:   []porchapi.PackageRevisionLifecycle{},
			invalid: []porchapi.PackageRevisionLifecycle{"", "Wrong", porchapi.PackageRevisionLifecycleDraft, porchapi.PackageRevisionLifecycleProposed, porchapi.PackageRevisionLifecyclePublished, porchapi.PackageRevisionLifecycleDeletionProposed},
		},
		{
			old:     porchapi.PackageRevisionLifecycleDraft,
			valid:   []porchapi.PackageRevisionLifecycle{},
			invalid: []porchapi.PackageRevisionLifecycle{"", "Wrong", porchapi.PackageRevisionLifecycleDraft, porchapi.PackageRevisionLifecycleProposed, porchapi.PackageRevisionLifecyclePublished, porchapi.PackageRevisionLifecycleDeletionProposed},
		},
		{
			old:     porchapi.PackageRevisionLifecyclePublished,
			valid:   []porchapi.PackageRevisionLifecycle{porchapi.PackageRevisionLifecycleDeletionProposed},
			invalid: []porchapi.PackageRevisionLifecycle{"", "Wrong", porchapi.PackageRevisionLifecycleDraft, porchapi.PackageRevisionLifecycleProposed, porchapi.PackageRevisionLifecyclePublished},
		},
		{
			old:     porchapi.PackageRevisionLifecycleDeletionProposed,
			valid:   []porchapi.PackageRevisionLifecycle{porchapi.PackageRevisionLifecyclePublished},
			invalid: []porchapi.PackageRevisionLifecycle{"", "Wrong", porchapi.PackageRevisionLifecycleDraft, porchapi.PackageRevisionLifecycleProposed, porchapi.PackageRevisionLifecycleDeletionProposed},
		},
		{
			old:     porchapi.PackageRevisionLifecycleProposed,
			valid:   []porchapi.PackageRevisionLifecycle{porchapi.PackageRevisionLifecycleDraft, porchapi.PackageRevisionLifecyclePublished},
			invalid: []porchapi.PackageRevisionLifecycle{"", "Wrong", porchapi.PackageRevisionLifecycleProposed, porchapi.PackageRevisionLifecycleDeletionProposed},
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
