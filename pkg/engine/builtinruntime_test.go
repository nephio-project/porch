// Copyright 2022,2025-2026 The kpt and Nephio Authors
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

package engine

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	kptfilev1 "github.com/kptdev/kpt/pkg/api/kptfile/v1"
	"github.com/kptdev/kpt/pkg/fn"
	fnsdk "github.com/kptdev/krm-functions-sdk/go/fn"
	"github.com/stretchr/testify/assert"
	"k8s.io/klog/v2"
)

const (
	gcrImagePrefix        = ""
	defaultKRMImagePrefix = "ghcr.io/kptdev/krm-functions-catalog/"
	testImageName         = "test-image"
	setNamespaceImageName = "set-namespace"
)

func TestNewBuiltinRuntime(t *testing.T) {
	t.Run("custom image prefix specified", func(t *testing.T) {
		customPrefix := "test.io/kptdev/krm-functions-catalog/"
		br := newBuiltinRuntime(customPrefix)

		assert.NotNil(t, br)
		assert.NotNil(t, br.fnMapping)

		// Verify that functions are registered with the custom image prefix
		applyReplacementCustomKey := customPrefix + "/" + "apply-replacements:v0.1.1"
		applyReplacementCustomProcessor, applyReplacementCustomExists := br.fnMapping[applyReplacementCustomKey]
		assert.True(t, applyReplacementCustomExists,
			"Expected function to be registered with custom image prefix: %s", applyReplacementCustomKey)
		assert.NotNil(t, applyReplacementCustomProcessor)

		// Verify that functions are also registered with default GHCR prefix
		applyReplacementGHCRKey := defaultKRMImagePrefix + "apply-replacements:v0.1.1"
		applyReplacementGHCRProcessor, applyReplacementGHCRExists := br.fnMapping[applyReplacementGHCRKey]
		assert.True(t, applyReplacementGHCRExists,
			"Expected function to be registered with default GHCR prefix")
		assert.NotNil(t, applyReplacementGHCRProcessor)
	})
	t.Run("custom image prefix is not specified", func(t *testing.T) {
		customPrefix := "test.io/kptdev/krm-functions-catalog/"
		br := newBuiltinRuntime(defaultKRMImagePrefix)

		assert.NotNil(t, br)
		assert.NotNil(t, br.fnMapping)

		// Verify that functions are not registered with the custom image prefix
		applyReplacementCustomKey := customPrefix + "/" + "apply-replacements:v0.1.1"
		applyReplacementCustomProcessor, applyReplacementCustomExists := br.fnMapping[applyReplacementCustomKey]
		assert.False(t, applyReplacementCustomExists,
			"Expected function to not be registered with custom image prefix: %s", applyReplacementCustomKey)
		assert.Nil(t, applyReplacementCustomProcessor)

		// Verify that functions are registered with default GHCR prefix
		applyReplacementGHCRKey := defaultKRMImagePrefix + "apply-replacements:v0.1.1"
		applyReplacementGHCRProcessor, applyReplacementGHCRExists := br.fnMapping[applyReplacementGHCRKey]
		assert.True(t, applyReplacementGHCRExists,
			"Expected function to be registered with default GHCR prefix")
		assert.NotNil(t, applyReplacementGHCRProcessor)
	})
}

func TestBuiltinRuntime(t *testing.T) {
	t.Run("invalid semver constraint syntax", func(t *testing.T) {
		br := newBuiltinRuntime(gcrImagePrefix)
		funct := &kptfilev1.Function{
			Image: defaultKRMImagePrefix + testImageName,
			// Invalid semver constraint, '>>' is not a valid operator
			// -> will cause a function not found error
			Tag: ">> 0.4.0 < 0.5.0",
		}
		_, err := br.GetRunner(context.Background(), funct)
		assert.Equal(t, &fn.NotFoundError{
			Function: kptfilev1.Function{Image: funct.Image},
		}, err)
	})
	t.Run("builtinrutime not found", func(t *testing.T) {
		br := newBuiltinRuntime(gcrImagePrefix)
		funct := &kptfilev1.Function{
			Image: defaultKRMImagePrefix + testImageName,
			// The csemver constraint is valid, however, there is no function with
			// the name 'test-image' is found
			// -> will cause a function not found error
			Tag: ">= 0.4.0 < 0.5.0",
		}
		_, err := br.GetRunner(context.Background(), funct)
		assert.Equal(t, &fn.NotFoundError{
			Function: kptfilev1.Function{Image: funct.Image},
		}, err)
	})
	t.Run("function does not match the semantic version constraints", func(t *testing.T) {
		br := newBuiltinRuntime(gcrImagePrefix)
		funct := &kptfilev1.Function{
			Image: defaultKRMImagePrefix + setNamespaceImageName,
			// The csemver constraint is valid, however, there is no function with
			// the name 'apply-replacements' is found with the specified semver constraint
			// -> will cause a function not found error
			Tag: "> 0.2.0 < 0.3.0",
		}
		_, err := br.GetRunner(context.Background(), funct)
		assert.Equal(t, &fn.NotFoundError{
			Function: kptfilev1.Function{Image: funct.Image},
		}, err)
	})
	t.Run("function execution error", func(t *testing.T) {
		br := newBuiltinRuntime(gcrImagePrefix)
		fn := &kptfilev1.Function{
			// Wrong function is specified for namespace setting,
			// which will cause an execution error when the function tries to run
			Image: defaultKRMImagePrefix + "apply-replacements",
			Tag:   ">= 0.1.0 < 0.2.0",
		}
		fr, err := br.GetRunner(context.Background(), fn)
		assert.Nil(t, err)
		reader := bytes.NewReader([]byte(`apiVersion: config.kubernetes.io/v1alpha1
kind: ResourceList
items:
  - apiVersion: v1
    kind: ConfigMap
    metadata:
      name: my-cm
      namespace: old
    data:
      foo: bar
functionConfig:
  apiVersion: v1
  kind: ConfigMap
  data:
    namespace: test-ns
`))
		var buf bytes.Buffer
		err = fr.Run(reader, &buf)
		assert.Equal(t, "error: function failure", err.Error())
	})
	t.Run("successful execution", func(t *testing.T) {
		br := newBuiltinRuntime(gcrImagePrefix)
		fn := &kptfilev1.Function{
			Image: defaultKRMImagePrefix + setNamespaceImageName,
			// This semver constraint matches the version of the apply-replacements function in builtin runtime,
			// so it should successfully find the function and run it
			Tag: ">= 0.4.0 < 0.5.0",
		}

		// Capture klog output by redirecting stderr
		oldStderr := os.Stderr
		r, w, _ := os.Pipe()
		os.Stderr = w

		fr, err := br.GetRunner(context.Background(), fn)
		assert.Nil(t, err)

		// Flush klog and restore stderr
		klog.Flush()
		w.Close()
		os.Stderr = oldStderr

		// Read captured output
		var logBuffer bytes.Buffer
		logBuffer.ReadFrom(r)
		logOutput := logBuffer.String()

		// Verify the klog message contains the expected version selection
		assert.Contains(t, logOutput, `Selected image "ghcr.io/kptdev/krm-functions-catalog/set-namespace:v0.4.1"`)
		assert.Contains(t, logOutput, `(version 0.4.1)`)
		assert.Contains(t, logOutput, `for request "ghcr.io/kptdev/krm-functions-catalog/set-namespace"`)

		// Ensure v0.4 is not in the selection message (only v0.4.1 should be selected)
		// We check that if "Selected image" appears, it's followed by v0.4.1, not v0.4
		if strings.Contains(logOutput, "Selected image") {
			selectedLine := ""
			for _, line := range strings.Split(logOutput, "\n") {
				if strings.Contains(line, "Selected image") {
					selectedLine = line
					break
				}
			}
			assert.Contains(t, selectedLine, "0.4.1", "Expected v0.4.1 to be selected as it's the greatest version")
		}

		reader := bytes.NewReader([]byte(`apiVersion: config.kubernetes.io/v1alpha1
kind: ResourceList
items:
  - apiVersion: v1
    kind: ConfigMap
    metadata:
      name: my-cm
      namespace: old
    data:
      foo: bar
functionConfig:
  apiVersion: v1
  kind: ConfigMap
  data:
    namespace: test-ns
`))

		var buf bytes.Buffer
		err = fr.Run(reader, &buf)
		assert.Nil(t, err)
		rl, err := fnsdk.ParseResourceList(buf.Bytes())
		assert.Nil(t, err)
		assert.Equal(t, 1, len(rl.Items))
		ns := rl.Items[0].GetNamespace()
		assert.Equal(t, "test-ns", ns)
	})
}
