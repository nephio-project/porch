// Copyright 2025 The kpt and Nephio Authors
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

package get

import (
	"context"
	"testing"

	porchapi "github.com/nephio-project/porch/api/porch/v1alpha1"
	// repoGet "github.com/nephio-project/porch/pkg/cli/commands/repo/get" // Remove or comment out unless needed elsewhere

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

func TestGetters(t *testing.T) {
	cells := make([]any, 1)
	cells[0] = int64(1234)

	_, retVal1 := getInt64Cell(cells, -1)
	assert.False(t, retVal1)

	val, retVal2 := getInt64Cell(cells, 0)
	assert.True(t, retVal2)
	assert.Equal(t, int64(1234), val)
}

func TestRunnerPreRunE(t *testing.T) {
	ns := "ns"
	ctx := context.Background()
	gcf := genericclioptions.ConfigFlags{Namespace: &ns}
	r := newRunner(ctx, &gcf)
	cmd := NewCommand(ctx, &gcf)

	err := r.preRunE(cmd, []string{})
	assert.Nil(t, err)

	if r.requestTable != true {
		t.Errorf("Expected requestTable to be true, got %v", r.requestTable)
	}
}

func TestGetCmd(t *testing.T) {
	ctx := context.Background()
	pkgRevName := "repo-testrevision"
	ns := "ns"

	scheme := runtime.NewScheme()
	if err := porchapi.AddToScheme(scheme); err != nil {
		t.Fatalf("Failed to add porch API to scheme: %v", err)
	}

	testCases := map[string]struct {
		output  string
		args    []string
		wantErr bool
	}{
		"Get package": {
			output:  pkgRevName + "\n",
			args:    []string{pkgRevName},
			wantErr: true,
		},
		"Args not provided": {
			wantErr: true,
		},
	}

	for tn := range testCases {
		tc := testCases[tn]
		t.Run(tn, func(t *testing.T) {

			r := newRunner(ctx, &genericclioptions.ConfigFlags{Namespace: &ns})
			cmd := &cobra.Command{}

			go func() {
				err := r.runE(cmd, tc.args)
				if err != nil && !tc.wantErr {
					t.Errorf("unexpected error: %v", err)
				}
			}()

		})
	}
}

type MockRunner struct {
	*runner
}

func (r *MockRunner) packageRevisionMatches(o *unstructured.Unstructured) (bool, error) {
	return r.runner.packageRevisionMatches(o)
}

func TestPackageRevisionMatches(t *testing.T) {
	tests := []struct {
		name       string
		flags      runner
		object     *unstructured.Unstructured
		expected   bool
		shouldFail bool
	}{
		{
			name: "match all filters",
			flags: runner{
				packageName: "pkg1",
				revision:    1,
				workspace:   "ws1",
			},
			object: &unstructured.Unstructured{
				Object: map[string]any{
					"spec": map[string]any{
						"packageName":   "pkg1",
						"revision":      int64(1),
						"workspaceName": "ws1",
					},
				},
			},
			expected: true,
		},
		{
			name: "package name mismatch",
			flags: runner{
				packageName: "pkg1",
				revision:    1,
				workspace:   "ws1",
			},
			object: &unstructured.Unstructured{
				Object: map[string]any{
					"spec": map[string]any{
						"packageName":   "pkg2",
						"revision":      int64(1),
						"workspaceName": "ws1",
					},
				},
			},
			expected: false,
		},
		{
			name: "revision mismatch",
			flags: runner{
				packageName: "pkg1",
				revision:    2,
				workspace:   "ws1",
			},
			object: &unstructured.Unstructured{
				Object: map[string]any{
					"spec": map[string]any{
						"packageName":   "pkg1",
						"revision":      int64(1),
						"workspaceName": "ws1",
					},
				},
			},
			expected: false,
		},
		{
			name: "workspace mismatch",
			flags: runner{
				packageName: "pkg1",
				revision:    1,
				workspace:   "ws2",
			},
			object: &unstructured.Unstructured{
				Object: map[string]any{
					"spec": map[string]any{
						"packageName":   "pkg1",
						"revision":      int64(1),
						"workspaceName": "ws1",
					},
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRunner := &MockRunner{runner: &tt.flags}
			match, err := mockRunner.packageRevisionMatches(tt.object)

			if tt.shouldFail {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, match)
			}
		})
	}
}

func TestNamespaceFlagWithoutValue(t *testing.T) {
	ctx := context.Background()
	ns := ""
	gcf := genericclioptions.ConfigFlags{Namespace: &ns}
	r := newRunner(ctx, &gcf)

	parentCmd := &cobra.Command{Use: "rpkg"}
	gcf.AddFlags(parentCmd.PersistentFlags())
	getCmd := NewCommand(ctx, &gcf)
	parentCmd.AddCommand(getCmd)

	nsFlag := getCmd.Flag("namespace")
	if nsFlag == nil {
		t.Fatal("namespace flag not found")
	}

	nsFlag.Changed = true
	errSet := nsFlag.Value.Set("")
	if errSet != nil {
		t.Fatalf("unexpected error setting namespace flag: %v", errSet)
	}

	err := r.preRunE(getCmd, []string{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "namespace flag specified without a value")
}

func TestNamespaceShortFlagWithoutValue(t *testing.T) {
	ctx := context.Background()
	ns := ""
	gcf := genericclioptions.ConfigFlags{Namespace: &ns}
	r := newRunner(ctx, &gcf)

	parentCmd := &cobra.Command{Use: "rpkg"}
	gcf.AddFlags(parentCmd.PersistentFlags())
	getCmd := NewCommand(ctx, &gcf)
	parentCmd.AddCommand(getCmd)

	nsFlag := getCmd.InheritedFlags().Lookup("namespace")
	if nsFlag == nil {
		t.Fatal("namespace flag not found in inherited flags")
	}
	if nsFlag.Shorthand != "n" {
		t.Fatalf("namespace flag does not have shorthand 'n', got '%s'", nsFlag.Shorthand)
	}
	nsFlag.Changed = true
	errSet := nsFlag.Value.Set("")
	if errSet != nil {
		t.Fatalf("unexpected error setting -n flag: %v", errSet)
	}

	err := r.preRunE(getCmd, []string{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "namespace flag specified without a value")
}
