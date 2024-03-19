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
	"context"
	"testing"

	api "github.com/nephio-project/porch/api/porch/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestUpdateStrategyForLifecycle(t *testing.T) {
	type testCase struct {
		old     api.PorchPkgRevisionLifecycle
		valid   []api.PorchPkgRevisionLifecycle
		invalid []api.PorchPkgRevisionLifecycle
	}

	s := packageRevisionStrategy{}

	for _, tc := range []testCase{
		{
			old:     "",
			valid:   []api.PorchPkgRevisionLifecycle{"", api.PorchPkgRevisionLifecycleDraft, api.PorchPkgRevisionLifecycleProposed},
			invalid: []api.PorchPkgRevisionLifecycle{"Wrong", api.PorchPkgRevisionLifecyclePublished},
		},
		{
			old:     api.PorchPkgRevisionLifecycleDraft,
			valid:   []api.PorchPkgRevisionLifecycle{"", api.PorchPkgRevisionLifecycleDraft, api.PorchPkgRevisionLifecycleProposed},
			invalid: []api.PorchPkgRevisionLifecycle{"Wrong", api.PorchPkgRevisionLifecyclePublished},
		},
		{
			old:     api.PorchPkgRevisionLifecycleProposed,
			valid:   []api.PorchPkgRevisionLifecycle{"", api.PorchPkgRevisionLifecycleDraft, api.PorchPkgRevisionLifecycleProposed},
			invalid: []api.PorchPkgRevisionLifecycle{"Wrong", api.PorchPkgRevisionLifecyclePublished},
		},
		{
			old:     api.PorchPkgRevisionLifecyclePublished,
			valid:   []api.PorchPkgRevisionLifecycle{api.PorchPkgRevisionLifecyclePublished},
			invalid: []api.PorchPkgRevisionLifecycle{"", "Wrong", api.PorchPkgRevisionLifecycleDraft, api.PorchPkgRevisionLifecycleProposed},
		},
		{
			old:     "Wrong",
			valid:   []api.PorchPkgRevisionLifecycle{},
			invalid: []api.PorchPkgRevisionLifecycle{"", "Wrong", api.PorchPkgRevisionLifecycleDraft, api.PorchPkgRevisionLifecycleProposed, api.PorchPkgRevisionLifecyclePublished},
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

func TestUpdateStrategy(t *testing.T) {
	s := packageRevisionStrategy{}

	testCases := map[string]struct {
		old   *api.PorchPkgRevision
		new   *api.PorchPkgRevision
		valid bool
	}{
		"spec can be updated for draft": {
			old: &api.PorchPkgRevision{
				Spec: api.PorchPkgRevisionSpec{
					Lifecycle: api.PorchPkgRevisionLifecycleDraft,
					Tasks: []api.Task{
						{
							Type: api.TaskTypeInit,
							Init: &api.PorchPkgInitTaskSpec{
								Description: "This is a test",
							},
						},
					},
				},
			},
			new: &api.PorchPkgRevision{
				Spec: api.PorchPkgRevisionSpec{
					Lifecycle: api.PorchPkgRevisionLifecycleDraft,
					Tasks: []api.Task{
						{
							Type: api.TaskTypeInit,
							Init: &api.PorchPkgInitTaskSpec{
								Description: "This is a test",
							},
						},
						{
							Type:  api.TaskTypePatch,
							Patch: &api.PorchPkgPatchTaskSpec{},
						},
					},
				},
			},
			valid: true,
		},
		"spec can not be updated for published": {
			old: &api.PorchPkgRevision{
				Spec: api.PorchPkgRevisionSpec{
					Lifecycle: api.PorchPkgRevisionLifecyclePublished,
					Tasks: []api.Task{
						{
							Type: api.TaskTypeInit,
							Init: &api.PorchPkgInitTaskSpec{
								Description: "This is a test",
							},
						},
					},
				},
			},
			new: &api.PorchPkgRevision{
				Spec: api.PorchPkgRevisionSpec{
					Lifecycle: api.PorchPkgRevisionLifecyclePublished,
					Tasks: []api.Task{
						{
							Type: api.TaskTypeInit,
							Init: &api.PorchPkgInitTaskSpec{
								Description: "This is a test",
							},
						},
						{
							Type:  api.TaskTypePatch,
							Patch: &api.PorchPkgPatchTaskSpec{},
						},
					},
				},
			},
			valid: false,
		},
		"labels can be updated for published": {
			old: &api.PorchPkgRevision{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"foo": "bar",
					},
				},
				Spec: api.PorchPkgRevisionSpec{
					Lifecycle: api.PorchPkgRevisionLifecyclePublished,
				},
			},
			new: &api.PorchPkgRevision{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"bar": "foo",
					},
				},
				Spec: api.PorchPkgRevisionSpec{
					Lifecycle: api.PorchPkgRevisionLifecyclePublished,
				},
			},
			valid: true,
		},
		"annotations can be updated for published": {
			old: &api.PorchPkgRevision{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"foo": "bar",
					},
				},
				Spec: api.PorchPkgRevisionSpec{
					Lifecycle: api.PorchPkgRevisionLifecyclePublished,
				},
			},
			new: &api.PorchPkgRevision{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"bar": "foo",
					},
				},
				Spec: api.PorchPkgRevisionSpec{
					Lifecycle: api.PorchPkgRevisionLifecyclePublished,
				},
			},
			valid: true,
		},
	}

	for tn := range testCases {
		tc := testCases[tn]
		t.Run(tn, func(t *testing.T) {
			ctx := context.Background()
			allErrs := s.ValidateUpdate(ctx, tc.new, tc.old)

			if tc.valid {
				if len(allErrs) > 0 {
					t.Errorf("Update failed unexpectedly: %v", allErrs.ToAggregate().Error())
				}
			} else {
				if len(allErrs) == 0 {
					t.Error("Update should fail but didn't")
				}
			}
		})
	}
}
