// Copyright 2022, 2025 The kpt and Nephio Authors
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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/bluekeyes/go-gitdiff/gitdiff"
	"github.com/google/go-cmp/cmp"
	"github.com/hexops/gotextdiff"
	"github.com/hexops/gotextdiff/myers"
	"github.com/hexops/gotextdiff/span"
	api "github.com/nephio-project/porch/api/porch/v1alpha1"
	"github.com/nephio-project/porch/pkg/repository"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/util/jsonmergepatch"
	"sigs.k8s.io/yaml"
)

func TestGoDiff(t *testing.T) {
	oldYAML := `
apiVersion: v1
kind: ConfigMap
data:
  foo1: bar
  foo2: bar2
  foo3: bar3
`
	newYAML := `
apiVersion: v1
kind: ConfigMap
data:
  foo1: bar11
  foo2: bar22
`

	edits := myers.ComputeEdits(span.URIFromPath("a.txt"), oldYAML, newYAML)
	got := fmt.Sprint(gotextdiff.ToUnified("a.txt", "b.txt", oldYAML, edits))

	want := `
--- a.txt
+++ b.txt
@@ -2,6 +2,5 @@
 apiVersion: v1
 kind: ConfigMap
 data:
-  foo1: bar
-  foo2: bar2
-  foo3: bar3
+  foo1: bar11
+  foo2: bar22
`

	got = strings.TrimSpace(got)
	want = strings.TrimSpace(want)

	t.Logf("patch:\n%v", got)

	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("unexpected result from myers.ComputeEdits: (-want,+got): %s", diff)
	}

	// files is a slice of *gitdiff.File describing the files changed in the patch
	// preamble is a string of the content of the patch before the first file
	files, preamble, err := gitdiff.Parse(strings.NewReader(got))
	if err != nil {
		t.Errorf("unexpected result from gitdiff.Parse: %v", err)
	}

	t.Logf("files=%#v", files)
	t.Logf("preamble=%#v", preamble)

	// apply the changes in the patch to a source file
	var output bytes.Buffer
	if err := gitdiff.Apply(&output, strings.NewReader(oldYAML), files[0]); err != nil {
		t.Errorf("unexpected result from gitdiff.Apply: %v", err)
	}

	patched := output.String()
	t.Logf("patched=%#v", patched)

	if diff := cmp.Diff(strings.TrimSpace(newYAML), strings.TrimSpace(patched)); diff != "" {
		t.Logf("patch result:\n%s", patched)
		t.Errorf("unexpected result from PatchApply: (-want,+got): %s", diff)
	}
}

func TestPatchJSONGen(t *testing.T) {
	oldYAML := `
apiVersion: v1
kind: ConfigMap
data:
  foo1: bar
  foo2: bar2
  foo3: bar3
`
	newYAML := `
apiVersion: v1
kind: ConfigMap
data:
  foo1: bar11
  foo2: bar22
`
	oldObj := make(map[string]interface{})
	if err := yaml.Unmarshal([]byte(oldYAML), &oldObj); err != nil {
		t.Fatalf("error from yaml.Unmarshal: %v", err)
	}

	newObj := make(map[string]interface{})
	if err := yaml.Unmarshal([]byte(newYAML), &newObj); err != nil {
		t.Fatalf("error from yaml.Unmarshal: %v", err)
	}

	oldJSON, err := json.Marshal(oldObj)
	if err != nil {
		t.Fatalf("error from json.Marshal: %v", err)
	}
	newJSON, err := json.Marshal(newObj)
	if err != nil {
		t.Fatalf("error from json.Marshal: %v", err)
	}

	patch, err := jsonmergepatch.CreateThreeWayJSONMergePatch(oldJSON, newJSON, oldJSON)
	if err != nil {
		t.Fatalf("error from CreateThreeWayJSONMergePatch: %v", err)
	}

	patchObject := make(map[string]interface{})
	if err := json.Unmarshal(patch, &patchObject); err != nil {
		t.Errorf("error from json.Unmarshal: %v", err)
	}

	patchYAML, err := yaml.Marshal(patchObject)
	if err != nil {
		t.Errorf("error from yaml.Marshal: %v", err)
	}

	got := string(patchYAML)

	want := `
data:
  foo1: bar11
  foo2: bar22
  foo3: null
`

	got = strings.TrimSpace(got)
	want = strings.TrimSpace(want)

	t.Logf("patch:\n%v", got)

	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("unexpected result from CreateThreeWayJSONMergePatch: (-want,+got): %s", diff)
	}
}

func TestApplyPatchMutation(t *testing.T) {
	patchTask := &api.PackagePatchTaskSpec{
		Patches: []api.PatchSpec{
			{
				File:      "testfile.txt",
				PatchType: api.PatchTypePatchFile,
				Contents: `
--- testfile.txt
+++ testfile.txt
@@ -1 +1 @@
-old line
+new line
`,
			},
		},
	}

	resources := repository.PackageResources{
		Contents: map[string]string{
			"testfile.txt": "old line\n",
		},
	}

	task := &api.Task{
		Type:  api.TaskTypePatch,
		Patch: patchTask,
	}
	m, _ := buildPatchMutation(context.Background(), task, api.ResourceMerge)

	ctx := context.Background()
	_, _, err := m.apply(ctx, resources)
	assert.EqualError(t, err, "patch had unexpected preamble \"\\n\"")
}

func TestApplyPatchMutation_EdgeCases(t *testing.T) {
	tests := []struct {
		name           string
		patchSpec      api.PatchSpec
		resources      repository.PackageResources
		expectedError  string
		expectedResult string
	}{
		{
			name: "Unexpected file name",
			patchSpec: api.PatchSpec{
				File:      "unexpected.txt",
				PatchType: api.PatchTypePatchFile,
				Contents: `
--- unexpected.txt
+++ unexpected.txt
@@ -1 +1 @@
-old line
+new line
`,
			},
			resources: repository.PackageResources{
				Contents: map[string]string{
					"testfile.txt": "old line\n",
				},
			},
			expectedError: "patch specifies file \"unexpected.txt\" which does not exist",
		},
		{
			name: "Binary diff",
			patchSpec: api.PatchSpec{
				File:      "testfile.txt",
				PatchType: api.PatchTypePatchFile,
				Contents: `--- testfile.txt
+++ testfile.txt
@@ -1 +1 @@
-old line
+new line
Binary files differ
`,
			},
			resources: repository.PackageResources{
				Contents: map[string]string{
					"testfile.txt": "old line\n",
				},
			},
			expectedError: "",
		},
		{
			name: "Conflict error",
			patchSpec: api.PatchSpec{
				File:      "testfile.txt",
				PatchType: api.PatchTypePatchFile,
				Contents: `--- testfile.txt
+++ testfile.txt
@@ -1 +1 @@
-old line
+new line
`,
			},
			resources: repository.PackageResources{
				Contents: map[string]string{
					"testfile.txt": "conflicting line\n",
				},
			},
			expectedError: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			patchTask := &api.PackagePatchTaskSpec{
				Patches: []api.PatchSpec{tt.patchSpec},
			}

			m := applyPatchMutation{
				patchTask:     patchTask,
				cloneStrategy: api.CopyMerge,
			}

			ctx := context.Background()
			_, _, err := m.apply(ctx, tt.resources)

			if tt.expectedError != "" {
				if err == nil || !strings.Contains(err.Error(), tt.expectedError) {
					t.Errorf("apply() error = %v, want %v", err, tt.expectedError)
				}
			} else if err != nil {
				t.Errorf("apply() unexpected error: %v", err)
			}
		})
	}
}

func TestSkipPatchMutation(t *testing.T) {
	tests := []struct {
		name          string
		cloneStrategy api.PackageMergeStrategy
		expectedSkip  bool
	}{
		{
			name:          "CopyMerge strategy should skip",
			cloneStrategy: api.CopyMerge,
			expectedSkip:  true,
		},
		{
			name:          "ForceDeleteReplace strategy should skip",
			cloneStrategy: api.ForceDeleteReplace,
			expectedSkip:  true,
		},
		{
			name:          "resource-merge strategy should not skip",
			cloneStrategy: api.PackageMergeStrategy("resource-merge"),
			expectedSkip:  false,
		},
		{
			name:          "Empty strategy should not skip",
			cloneStrategy: api.PackageMergeStrategy(""),
			expectedSkip:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := applyPatchMutation{
				cloneStrategy: tt.cloneStrategy,
			}
			skip := skipPatchMutation(context.Background(), m)
			if skip != tt.expectedSkip {
				t.Errorf("skipPatchMutation() = %v, want %v", skip, tt.expectedSkip)
			}
		})
	}
}
