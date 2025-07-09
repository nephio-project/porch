// Copyright 2022, 2024-2025 The kpt and Nephio Authors
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

package task

import (
	"context"
	"testing"

	api "github.com/nephio-project/porch/api/porch/v1alpha1"
	"github.com/nephio-project/porch/pkg/externalrepo/fake"
	kptfile "github.com/nephio-project/porch/pkg/kpt/api/kptfile/v1"
	"github.com/nephio-project/porch/pkg/repository"
	"github.com/stretchr/testify/assert"
)

func TestKptfilePatch(t *testing.T) {
	testCases := map[string]struct {
		repoPkgRev   repository.PackageRevision
		newApiPkgRev *api.PackageRevision
		shouldChange bool
		newKptfile   *kptfile.KptFile
	}{
		"no gates or conditions": {
			repoPkgRev: &fake.FakePackageRevision{
				Kptfile: kptfile.KptFile{},
			},
			newApiPkgRev: &api.PackageRevision{
				Spec: api.PackageRevisionSpec{},
			},
			shouldChange: false,
		},
		"first gate and condition added": {
			repoPkgRev: &fake.FakePackageRevision{
				Kptfile: kptfile.KptFile{},
			},
			newApiPkgRev: &api.PackageRevision{
				Spec: api.PackageRevisionSpec{
					ReadinessGates: []api.ReadinessGate{
						{
							ConditionType: "foo",
						},
					},
				},
				Status: api.PackageRevisionStatus{
					Conditions: []api.Condition{
						{
							Type:   "foo",
							Status: api.ConditionTrue,
						},
					},
				},
			},
			shouldChange: true,
			newKptfile: &kptfile.KptFile{
				Info: &kptfile.PackageInfo{
					ReadinessGates: []kptfile.ReadinessGate{
						{
							ConditionType: "foo",
						},
					},
				},
				Status: &kptfile.Status{
					Conditions: []kptfile.Condition{
						{
							Type:   "foo",
							Status: "True",
						},
					},
				},
			},
		},
		"additional readinessGates and conditions added": {
			repoPkgRev: &fake.FakePackageRevision{
				Kptfile: kptfile.KptFile{
					Info: &kptfile.PackageInfo{
						ReadinessGates: []kptfile.ReadinessGate{
							{
								ConditionType: "foo",
							},
						},
					},
					Status: &kptfile.Status{
						Conditions: []kptfile.Condition{
							{
								Type:   "foo",
								Status: kptfile.ConditionTrue,
							},
						},
					},
				},
			},
			newApiPkgRev: &api.PackageRevision{
				Spec: api.PackageRevisionSpec{
					ReadinessGates: []api.ReadinessGate{
						{
							ConditionType: "foo",
						},
						{
							ConditionType: "bar",
						},
					},
				},
				Status: api.PackageRevisionStatus{
					Conditions: []api.Condition{
						{
							Type:    "foo",
							Status:  api.ConditionTrue,
							Reason:  "reason",
							Message: "message",
						},
						{
							Type:    "bar",
							Status:  api.ConditionFalse,
							Reason:  "reason",
							Message: "message",
						},
					},
				},
			},
			shouldChange: true,
			newKptfile: &kptfile.KptFile{
				Info: &kptfile.PackageInfo{
					ReadinessGates: []kptfile.ReadinessGate{
						{
							ConditionType: "foo",
						},
						{
							ConditionType: "bar",
						},
					},
				},
				Status: &kptfile.Status{
					Conditions: []kptfile.Condition{
						{
							Type:    "foo",
							Status:  "True",
							Reason:  "reason",
							Message: "message",
						},
						{
							Type:    "bar",
							Status:  "False",
							Reason:  "reason",
							Message: "message",
						},
					},
				},
			},
		},
		"no changes": {
			repoPkgRev: &fake.FakePackageRevision{
				Kptfile: kptfile.KptFile{
					Info: &kptfile.PackageInfo{
						ReadinessGates: []kptfile.ReadinessGate{
							{
								ConditionType: "foo",
							},
							{
								ConditionType: "bar",
							},
						},
					},
					Status: &kptfile.Status{
						Conditions: []kptfile.Condition{
							{
								Type:    "foo",
								Status:  kptfile.ConditionTrue,
								Reason:  "reason",
								Message: "message",
							},
							{
								Type:    "bar",
								Status:  kptfile.ConditionFalse,
								Reason:  "reason",
								Message: "message",
							},
						},
					},
				},
			},
			newApiPkgRev: &api.PackageRevision{
				Spec: api.PackageRevisionSpec{
					ReadinessGates: []api.ReadinessGate{
						{
							ConditionType: "foo",
						},
						{
							ConditionType: "bar",
						},
					},
				},
				Status: api.PackageRevisionStatus{
					Conditions: []api.Condition{
						{
							Type:    "foo",
							Status:  api.ConditionTrue,
							Reason:  "reason",
							Message: "message",
						},
						{
							Type:    "bar",
							Status:  api.ConditionFalse,
							Reason:  "reason",
							Message: "message",
						},
					},
				},
			},
			shouldChange: false,
		},
		"readinessGates and conditions removed": {
			repoPkgRev: &fake.FakePackageRevision{
				Kptfile: kptfile.KptFile{
					Info: &kptfile.PackageInfo{
						ReadinessGates: []kptfile.ReadinessGate{
							{
								ConditionType: "foo",
							},
							{
								ConditionType: "bar",
							},
						},
					},
					Status: &kptfile.Status{
						Conditions: []kptfile.Condition{
							{
								Type:    "foo",
								Status:  kptfile.ConditionTrue,
								Reason:  "reason",
								Message: "message",
							},
							{
								Type:    "bar",
								Status:  kptfile.ConditionFalse,
								Reason:  "reason",
								Message: "message",
							},
						},
					},
				},
			},
			newApiPkgRev: &api.PackageRevision{
				Spec: api.PackageRevisionSpec{
					ReadinessGates: []api.ReadinessGate{
						{
							ConditionType: "foo",
						},
					},
				},
				Status: api.PackageRevisionStatus{
					Conditions: []api.Condition{
						{
							Type:   "foo",
							Status: api.ConditionTrue,
						},
					},
				},
			},
			shouldChange: true,
			newKptfile: &kptfile.KptFile{
				Info: &kptfile.PackageInfo{
					ReadinessGates: []kptfile.ReadinessGate{
						{
							ConditionType: "foo",
						},
					},
				},
				Status: &kptfile.Status{
					Conditions: []kptfile.Condition{
						{
							Type:   "foo",
							Status: kptfile.ConditionTrue,
						},
					},
				},
			},
		},
	}

	for tn := range testCases {
		tc := testCases[tn]
		t.Run(tn, func(t *testing.T) {
			newKf, err := patchKptfile(context.Background(), tc.repoPkgRev, tc.newApiPkgRev)
			if err != nil {
				t.Fatal(err)
			}

			if tc.shouldChange {
				assert.Equal(t, tc.newKptfile, newKf)
			} else {
				oldKf, _ := tc.repoPkgRev.GetKptfile(context.TODO())
				assert.Equal(t, &oldKf, newKf)
			}
		})
	}
}
