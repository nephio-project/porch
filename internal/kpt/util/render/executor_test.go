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

package render

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nephio-project/porch/internal/kpt/fnruntime"
	"github.com/nephio-project/porch/internal/kpt/types"
	"github.com/nephio-project/porch/pkg/kpt/printer"
	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/kustomize/kyaml/filesys"
	"sigs.k8s.io/kustomize/kyaml/kio"
)

func TestPathRelToRoot(t *testing.T) {
	tests := []struct {
		name         string
		rootPath     string
		subPkgPath   string
		resourcePath string
		expected     string
		errString    string
	}{
		{
			name:         "root package with non absolute path",
			rootPath:     "tmp",
			subPkgPath:   "/tmp/a",
			resourcePath: "c.yaml",
			expected:     "",
			errString:    fmt.Sprintf("root package path %q must be absolute", "tmp"),
		},
		{
			name:         "subpackage with non absolute path",
			rootPath:     "/tmp",
			subPkgPath:   "tmp/a",
			resourcePath: "c.yaml",
			expected:     "",
			errString:    fmt.Sprintf("subpackage path %q must be absolute", "tmp/a"),
		},
		{
			name:         "resource in a subpackage",
			rootPath:     "/tmp",
			subPkgPath:   "/tmp/a",
			resourcePath: "c.yaml",
			expected:     "a/c.yaml",
		},
		{
			name:         "resource exists in a deeply nested subpackage",
			rootPath:     "/tmp",
			subPkgPath:   "/tmp/a/b/c",
			resourcePath: "c.yaml",
			expected:     "a/b/c/c.yaml",
		},
		{
			name:         "resource exists in a sub dir with same name as sub package",
			rootPath:     "/tmp",
			subPkgPath:   "/tmp/a",
			resourcePath: "a/c.yaml",
			expected:     "a/a/c.yaml",
		},
		{
			name:         "subpackage is not a descendant of root package",
			rootPath:     "/tmp",
			subPkgPath:   "/a",
			resourcePath: "c.yaml",
			expected:     "",
			errString:    fmt.Sprintf("subpackage %q is not a descendant of %q", "/a", "/tmp"),
		},
	}

	for _, test := range tests {
		tc := test
		t.Run(tc.name, func(t *testing.T) {
			newPath, err := pathRelToRoot(tc.rootPath,
				tc.subPkgPath, tc.resourcePath)
			assert.Equal(t, newPath, tc.expected)
			if tc.errString != "" {
				assert.Contains(t, err.Error(), tc.errString)
			}
		})
	}
}

func TestMergeWithInput(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		selectedInput string
		output        string
		expected      string
	}{
		{
			name: "simple input",
			input: `apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx-deployment
  annotations:
    internal.config.k8s.io/kpt-resource-id: "0"
spec:
  replicas: 3`,
			selectedInput: `apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx-deployment
  annotations:
    internal.config.k8s.io/kpt-resource-id: "0"
spec:
  replicas: 3`,
			output: `apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx-deployment
  namespace: staging
  annotations:
    internal.config.k8s.io/kpt-resource-id: "0"
spec:
  replicas: 3`,
			expected: `apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx-deployment
  namespace: staging
  annotations:
    internal.config.k8s.io/kpt-resource-id: "0"
spec:
  replicas: 3
`,
		},
		{
			name: "complex example with generation, transformation and deletion of resource",
			input: `apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx-deployment-0
  annotations:
    internal.config.k8s.io/kpt-resource-id: "0"
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx-deployment-1
  annotations:
    internal.config.k8s.io/kpt-resource-id: "1"
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx-deployment-2
  annotations:
    internal.config.k8s.io/kpt-resource-id: "2"
`,
			selectedInput: `apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx-deployment-0
  annotations:
    internal.config.k8s.io/kpt-resource-id: "0"
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx-deployment-1
  annotations:
    internal.config.k8s.io/kpt-resource-id: "1"
`,
			output: `apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx-deployment-0
  namespace: staging # transformed
  annotations:
    internal.config.k8s.io/kpt-resource-id: "0"
---
apiVersion: apps/v1 # generated resource
kind: Deployment
metadata:
  name: nginx-deployment-3
`,
			expected: `apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx-deployment-0
  namespace: staging # transformed
  annotations:
    internal.config.k8s.io/kpt-resource-id: "0"
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx-deployment-2
  annotations:
    internal.config.k8s.io/kpt-resource-id: "2"
---
apiVersion: apps/v1 # generated resource
kind: Deployment
metadata:
  name: nginx-deployment-3
`,
		},
	}

	for i := range tests {
		tc := tests[i]
		t.Run(tc.name, func(t *testing.T) {
			output, err := kio.ParseAll(tc.output)
			assert.NoError(t, err)
			selectedInput, err := kio.ParseAll(tc.selectedInput)
			assert.NoError(t, err)
			input, err := kio.ParseAll(tc.input)
			assert.NoError(t, err)
			result := fnruntime.MergeWithInput(output, selectedInput, input)
			actual, err := kio.StringAll(result)
			assert.NoError(t, err)
			assert.Equal(t, tc.expected, actual)
		})
	}
}

func setupRendererTest(t *testing.T, renderBFS bool) (*Renderer, *bytes.Buffer, context.Context) {
	var outputBuffer bytes.Buffer
	ctx := context.Background()
	ctx = printer.WithContext(ctx, printer.New(&outputBuffer, &outputBuffer))

	mockFileSystem := filesys.MakeFsInMemory()

	rootPkgPath := "/root"
	err := mockFileSystem.Mkdir(rootPkgPath)
	assert.NoError(t, err)

	subPkgPath := "/root/subpkg"
	err = mockFileSystem.Mkdir(subPkgPath)
	assert.NoError(t, err)

	err = mockFileSystem.WriteFile(filepath.Join(rootPkgPath, "Kptfile"), fmt.Appendf(nil, `
apiVersion: kpt.dev/v1
kind: Kptfile
metadata:
  name: root-package
renderBFS: %t
`, renderBFS))
	assert.NoError(t, err)

	err = mockFileSystem.WriteFile(filepath.Join(subPkgPath, "Kptfile"), []byte(`
apiVersion: kpt.dev/v1
kind: Kptfile
metadata:
  name: sub-package
`))
	assert.NoError(t, err)

	renderer := &Renderer{
		PkgPath:        rootPkgPath,
		ResultsDirPath: "/results",
		FileSystem:     mockFileSystem,
	}

	return renderer, &outputBuffer, ctx
}

func TestRenderer_Execute_RenderOrder(t *testing.T) {
	tests := []struct {
		name           string
		renderBFS      bool
		expectedOrder  func(output string) bool
		expectedErrMsg string
	}{
		{
			name:      "Use BFS",
			renderBFS: true,
			expectedOrder: func(output string) bool {
				rootIndex := strings.Index(output, `Package "root":`)
				subpkgIndex := strings.Index(output, `Package "root/subpkg":`)
				return rootIndex < subpkgIndex // Root should appear before subpkg
			},
		},
		{
			name:      "Use DFS",
			renderBFS: false,
			expectedOrder: func(output string) bool {
				rootIndex := strings.Index(output, `Package "root":`)
				subpkgIndex := strings.Index(output, `Package "root/subpkg":`)
				return rootIndex > subpkgIndex // Subpkg should appear before root
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			renderer, outputBuffer, ctx := setupRendererTest(t, tc.renderBFS)

			fnResults, err := renderer.Execute(ctx)
			assert.NoError(t, err)
			assert.NotNil(t, fnResults)
			assert.Equal(t, 0, len(fnResults.Items))

			output := outputBuffer.String()
			assert.True(t, tc.expectedOrder(output), tc.expectedErrMsg)
		})
	}
}

func TestHydrate_ErrorCases(t *testing.T) {
	mockFileSystem := filesys.MakeFsInMemory()

	// Create a mock root package
	rootPath := "/root"
	err := mockFileSystem.Mkdir(rootPath)
	assert.NoError(t, err)

	// Add a Kptfile to the root package
	err = mockFileSystem.WriteFile(filepath.Join(rootPath, "Kptfile"), []byte(`
apiVersion: kpt.dev/v1
kind: Kptfile
metadata:
  name: root-package
`))
	assert.NoError(t, err)

	// Create a mock hydration context
	root, err := newPkgNode(mockFileSystem, rootPath, nil)
	assert.NoError(t, err)

	hctx := &hydrationContext{
		root:       root,
		pkgs:       map[types.UniquePath]*pkgNode{},
		fileSystem: mockFileSystem,
	}

	t.Run("Cycle Detection in hydrate", func(t *testing.T) {
		// Add the root package to the hydration context in a "Hydrating" state to simulate a cycle
		hctx.pkgs[root.pkg.UniquePath] = &pkgNode{
			pkg:   root.pkg,
			state: Hydrating,
		}

		_, err := hydrate(context.Background(), root, hctx)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cycle detected in pkg dependencies")
	})

	t.Run("Error in LocalResources", func(t *testing.T) {
		// Simulate an error in LocalResources by creating a package with no Kptfile
		invalidPkgPath := "/invalid"
		err := mockFileSystem.Mkdir(invalidPkgPath)
		assert.NoError(t, err)

		invalidPkgNode, err := newPkgNode(mockFileSystem, invalidPkgPath, nil)
		if err != nil {
			// Ensure the error is properly handled
			assert.Contains(t, err.Error(), "error reading Kptfile")
			return
		}

		// If no error, proceed to call hydrate (this should not happen in this case)
		_, err = hydrate(context.Background(), invalidPkgNode, hctx)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read Kptfile")
	})
}

func TestHydrateBFS_ErrorCases(t *testing.T) {
	ctx := printer.WithContext(context.Background(), printer.New(nil, nil))
	mockFileSystem := filesys.MakeFsInMemory()

	rootPkgPath := "/root"
	err := mockFileSystem.Mkdir(rootPkgPath)
	assert.NoError(t, err)

	subPkgPath := "/root/subpkg"
	err = mockFileSystem.Mkdir(subPkgPath)
	assert.NoError(t, err)

	err = mockFileSystem.WriteFile(filepath.Join(rootPkgPath, "Kptfile"), []byte(`
apiVersion: kpt.dev/v1
kind: Kptfile
metadata:
  name: root-package
renderBFS: true
`))
	assert.NoError(t, err)

	err = mockFileSystem.WriteFile(filepath.Join(subPkgPath, "Kptfile"), []byte(`
apiVersion: kpt.dev/v1
kind: Kptfile
metadata:
  name: sub-package
`))
	assert.NoError(t, err)

	// Create a mock hydration context
	root, err := newPkgNode(mockFileSystem, rootPkgPath, nil)
	assert.NoError(t, err)

	hctx := &hydrationContext{
		root:       root,
		pkgs:       map[types.UniquePath]*pkgNode{},
		fileSystem: mockFileSystem,
	}

	t.Run("Cycle Detection in hydrateBFS", func(t *testing.T) {
		// Add the root package to the hydration context in a "Hydrating" state to simulate a cycle
		hctx.pkgs[root.pkg.UniquePath] = &pkgNode{
			pkg:   root.pkg,
			state: Hydrating,
		}

		_, err := hydrateBFS(context.Background(), root, hctx)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cycle detected in pkg dependencies")
	})

	t.Run("Invalid Package State in hydrateBFS", func(t *testing.T) {
		// Add the root package to the hydration context in an invalid state
		hctx.pkgs[root.pkg.UniquePath] = &pkgNode{
			pkg:   root.pkg,
			state: -1, // Invalid state
		}

		_, err := hydrateBFS(context.Background(), root, hctx)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "package found in invalid state")
	})

	t.Run("Wet Package State in hydrateBFS would continue", func(t *testing.T) {
		// Add the root package to the hydration context in an invalid state
		hctx.pkgs[root.pkg.UniquePath] = &pkgNode{
			pkg:   root.pkg,
			state: Wet,
		}

		_, err := hydrateBFS(ctx, root, hctx)
		assert.NoError(t, err)
	})
}
