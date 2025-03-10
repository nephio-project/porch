/*
 Copyright 2025 The Nephio Authors.

 Licensed under the Apache License, Version 2.0 (the "License");
 You may not use this file except in compliance with the License.
 You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

 Unless required by applicable law or agreed to in writing, software
 distributed under the License is distributed on an "AS IS" BASIS,
 WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 See the License for the specific language governing permissions and
 limitations under the License.
*/

package starlark

import (
	"testing"

	"github.com/GoogleContainerTools/kpt-functions-sdk/go/fn"
	"github.com/stretchr/testify/assert"
)

func TestStarlarkConfig(t *testing.T) {
	testcases := []struct {
		name         string
		config       string
		expectErrMsg string
	}{
		{
			name: "valid StarlarkRun",
			config: `apiVersion: fn.kpt.dev/v1alpha1
kind: StarlarkRun
metadata:
  name: my-star-fn
  namespace: foo
source: |
  def run(r, ns_value):
    for resource in r:
      resource["metadata"]["namespace"] = ns_value
  run(ctx.resource_list["items"], "baz")
`,
		},
		{
			name: "StarlarkRun missing Source",
			config: `apiVersion: fn.kpt.dev/v1alpha1
kind: StarlarkRun
metadata:
  name: my-star-fn
`,
			expectErrMsg: "`source` must not be empty",
		},
		{
			name: "valid ConfigMap",
			config: `apiVersion: v1
kind: ConfigMap
metadata:
  name: my-star-fn
data:
  source: |
    def run(r, ns_value):
      for resource in r:
        resource["metadata"]["namespace"] = ns_value
    run(ctx.resource_list["items"], "baz")
`,
		},
		{
			name: "ConfigMap missing source",
			config: `apiVersion: v1
kind: ConfigMap
metadata:
  name: my-star-fn
`,
			expectErrMsg: "`source` must not be empty",
		},
		{
			name: "ConfigMap with parameter but missing source",
			config: `apiVersion: v1
kind: ConfigMap
metadata:
  name: my-star-fn
data:
  param1: foo
`,
			expectErrMsg: "`source` must not be empty",
		},
	}
	for _, tc := range testcases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			sr := &StarlarkRun{}
			ko, err := fn.ParseKubeObject([]byte(tc.config))
			assert.NoError(t, err)
			err = sr.Config(ko)
			if tc.expectErrMsg == "" {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectErrMsg)
			}
		})
	}
}
