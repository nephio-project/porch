// Copyright 2024 The kpt and Nephio Authors
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

package fn

import (
	"strings"
	"testing"

	kptfileapi "github.com/nephio-project/porch/pkg/kpt/api/kptfile/v1"
	"gotest.tools/assert"
)

func TestAddCondition(t *testing.T) {
	resources := map[string]string{
		"Kptfile": `
apiVersion: kpt.dev/v1
kind: Kptfile
metadata:
  name: example
  annotations:
    config.kubernetes.io/local-config: "true"
pipeline:
  mutators:
    - image: gcr.io/kpt-fn/set-labels:unstable
      configPath: fn-config.yaml`,
		"service.yaml": `
apiVersion: v1
kind: Service
metadata:
 name: whatever
 labels:
   app: myApp`,
	}

	expectedKptfile := strings.TrimSpace(`
apiVersion: kpt.dev/v1
kind: Kptfile
metadata:
  name: example
  annotations:
    config.kubernetes.io/local-config: "true"
pipeline:
  mutators:
    - image: gcr.io/kpt-fn/set-labels:unstable
      configPath: fn-config.yaml
status:
  conditions:
    - type: test
      status: "True"
      message: Everything is awesome!
      reason: Test`)

	kptfile, err := NewKptfileFromPackage(resources)
	assert.NilError(t, err, "failed to parse Kptfile")

	err = kptfile.SetTypedCondition(kptfileapi.Condition{
		Type:    "test",
		Status:  "True",
		Reason:  "Test",
		Message: "Everything is awesome!",
	})
	assert.NilError(t, err, "failed to set condition")

	cond, found := kptfile.GetTypedCondition("test")
	assert.Equal(t, true, found)
	assert.Equal(t, kptfileapi.ConditionTrue, cond.Status)
	assert.Equal(t, expectedKptfile, strings.TrimSpace(kptfile.String()))
}
