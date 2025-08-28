/*
Copyright 2022 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package cel

import (
	"context"
	"strings"
	"testing"
	"time"

	"k8s.io/component-base/metrics/legacyregistry"
	"k8s.io/component-base/metrics/testutil"
)

type metricsObserver func()

func TestNoUtils(t *testing.T) {

	metrics := []string{
		"apiserver_validating_admission_policy_definition_total",
		"apiserver_validating_admission_policy_check_total",
		"apiserver_validating_admission_policy_check_duration_seconds",
	}

	testCases := []struct {
		desc     string
		want     string
		observer metricsObserver
	}{
		{
			desc: "observe policy admission",
			want: `
			# HELP apiserver_validating_admission_policy_check_duration_seconds [ALPHA] Validation admission latency for individual validation expressions in seconds, labeled by policy and further including binding, state and enforcement action taken.
            # TYPE apiserver_validating_admission_policy_check_duration_seconds histogram
			apiserver_validating_admission_policy_check_duration_seconds_bucket{enforcement_action="allow",policy="policy.example.com",policy_binding="binding.example.com",state="active",le="0.0000005"} 0
            apiserver_validating_admission_policy_check_duration_seconds_bucket{enforcement_action="allow",policy="policy.example.com",policy_binding="binding.example.com",state="active",le="0.001"} 0
            apiserver_validating_admission_policy_check_duration_seconds_bucket{enforcement_action="allow",policy="policy.example.com",policy_binding="binding.example.com",state="active",le="0.01"} 0
            apiserver_validating_admission_policy_check_duration_seconds_bucket{enforcement_action="allow",policy="policy.example.com",policy_binding="binding.example.com",state="active",le="0.1"} 0
            apiserver_validating_admission_policy_check_duration_seconds_bucket{enforcement_action="allow",policy="policy.example.com",policy_binding="binding.example.com",state="active",le="1"} 0
            apiserver_validating_admission_policy_check_duration_seconds_bucket{enforcement_action="allow",policy="policy.example.com",policy_binding="binding.example.com",state="active",le="+Inf"} 1
            apiserver_validating_admission_policy_check_duration_seconds_sum{enforcement_action="allow",policy="policy.example.com",policy_binding="binding.example.com",state="active"} 10
            apiserver_validating_admission_policy_check_duration_seconds_count{enforcement_action="allow",policy="policy.example.com",policy_binding="binding.example.com",state="active"} 1
            # HELP apiserver_validating_admission_policy_check_total [ALPHA] Validation admission policy check total, labeled by policy and further identified by binding, enforcement action taken, and state.
            # TYPE apiserver_validating_admission_policy_check_total counter
            apiserver_validating_admission_policy_check_total{enforcement_action="allow",policy="policy.example.com",policy_binding="binding.example.com",state="active"} 1
			`,
			observer: func() {
				Metrics.ObserveAdmissionWithError(context.TODO(), time.Duration(10)*time.Second, "policy.example.com", "binding.example.com", "active")
			},
		},
		{
			desc: "observe policy rejection",
			want: `
			# HELP apiserver_validating_admission_policy_check_duration_seconds [ALPHA] Validation admission latency for individual validation expressions in seconds, labeled by policy and further including binding, state and enforcement action taken.
            # TYPE apiserver_validating_admission_policy_check_duration_seconds histogram
			apiserver_validating_admission_policy_check_duration_seconds_bucket{enforcement_action="deny",policy="policy.example.com",policy_binding="binding.example.com",state="active",le="0.0000005"} 0
            apiserver_validating_admission_policy_check_duration_seconds_bucket{enforcement_action="deny",policy="policy.example.com",policy_binding="binding.example.com",state="active",le="0.001"} 0
            apiserver_validating_admission_policy_check_duration_seconds_bucket{enforcement_action="deny",policy="policy.example.com",policy_binding="binding.example.com",state="active",le="0.01"} 0
            apiserver_validating_admission_policy_check_duration_seconds_bucket{enforcement_action="deny",policy="policy.example.com",policy_binding="binding.example.com",state="active",le="0.1"} 0
            apiserver_validating_admission_policy_check_duration_seconds_bucket{enforcement_action="deny",policy="policy.example.com",policy_binding="binding.example.com",state="active",le="1"} 0
            apiserver_validating_admission_policy_check_duration_seconds_bucket{enforcement_action="deny",policy="policy.example.com",policy_binding="binding.example.com",state="active",le="+Inf"} 1
            apiserver_validating_admission_policy_check_duration_seconds_sum{enforcement_action="deny",policy="policy.example.com",policy_binding="binding.example.com",state="active"} 10
            apiserver_validating_admission_policy_check_duration_seconds_count{enforcement_action="deny",policy="policy.example.com",policy_binding="binding.example.com",state="active"} 1
            # HELP apiserver_validating_admission_policy_check_total [ALPHA] Validation admission policy check total, labeled by policy and further identified by binding, enforcement action taken, and state.
            # TYPE apiserver_validating_admission_policy_check_total counter
            apiserver_validating_admission_policy_check_total{enforcement_action="deny",policy="policy.example.com",policy_binding="binding.example.com",state="active"} 1
			`,
			observer: func() {
				Metrics.ObserveRejection(context.TODO(), time.Duration(10)*time.Second, "policy.example.com", "binding.example.com", "active")
			},
		},
		{
			desc: "observe policy definition",
			want: `# HELP apiserver_validating_admission_policy_definition_total [ALPHA] Validation admission policy count total, labeled by state and enforcement action.
            # TYPE apiserver_validating_admission_policy_definition_total counter
            apiserver_validating_admission_policy_definition_total{enforcement_action="deny",state="active"} 1
			`,
			observer: func() {
				Metrics.ObserveDefinition(context.TODO(), "active", "deny")
			},
		},
	}

	Metrics.Reset()

	for _, tt := range testCases {
		t.Run(tt.desc, func(t *testing.T) {
			defer Metrics.Reset()
			tt.observer()
			if err := testutil.GatherAndCompare(legacyregistry.DefaultGatherer, strings.NewReader(tt.want), metrics...); err != nil {
				t.Fatal(err)
			}
		})
	}
}
