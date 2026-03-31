// Copyright 2025-2026 The kpt and Nephio Authors
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
	"bytes"
	"context"
	"testing"

	"github.com/kptdev/kpt/pkg/lib/util/cmdutil"
	porchapi "github.com/nephio-project/porch/api/porch/v1alpha1"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
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

func TestShowKptfilePreRunE(t *testing.T) {
	ctx := context.Background()
	ns := "default"
	gcf := genericclioptions.ConfigFlags{Namespace: &ns}
	r := newRunner(ctx, &gcf)
	r.showKptfile = true
	cmd := NewCommand(ctx, &gcf)

	// no args should fail
	err := r.preRunE(cmd, []string{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "--show-kptfile requires exactly one package revision name")

	// multiple args should fail
	err = r.preRunE(cmd, []string{"pkg1", "pkg2"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "--show-kptfile requires exactly one package revision name")

	// --show-kptfile with --name should fail
	r2 := newRunner(ctx, &gcf)
	r2.showKptfile = true
	r2.packageName = "some-pkg"
	err = r2.preRunE(cmd, []string{"pkg1"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "--show-kptfile cannot be combined with --name, --revision, or --workspace")

	// --show-kptfile with --revision should fail
	r3 := newRunner(ctx, &gcf)
	r3.showKptfile = true
	r3.revision = 1
	err = r3.preRunE(cmd, []string{"pkg1"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "--show-kptfile cannot be combined with --name, --revision, or --workspace")

	// --show-kptfile with --workspace should fail
	r4 := newRunner(ctx, &gcf)
	r4.showKptfile = true
	r4.workspace = "ws1"
	err = r4.preRunE(cmd, []string{"pkg1"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "--show-kptfile cannot be combined with --name, --revision, or --workspace")

	// --show-kptfile with --all-namespaces should fail
	r5 := newRunner(ctx, &gcf)
	r5.showKptfile = true
	r5.getFlags.AllNamespaces = true
	err = r5.preRunE(cmd, []string{"pkg1"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "--show-kptfile cannot be combined with --all-namespaces")
}

func TestShowKptfileContent(t *testing.T) {
	ns := "ns"
	ctx := context.Background()

	scheme := runtime.NewScheme()
	if err := porchapi.AddToScheme(scheme); err != nil {
		t.Fatalf("Failed to add scheme: %v", err)
	}

	kptfileContent := `apiVersion: kpt.dev/v1
kind: Kptfile
metadata:
  name: test-pkg
info:
  description: test package
`

	testCases := map[string]struct {
		resources map[string]string
		wantOut   string
		wantErr   bool
		errMsg    string
	}{
		"shows root Kptfile": {
			resources: map[string]string{
				"Kptfile":        kptfileContent,
				"nested/Kptfile": "should not appear",
				"cm.yaml":        "should not appear",
			},
			wantOut: kptfileContent,
		},
		"no Kptfile in package": {
			resources: map[string]string{
				"cm.yaml": "apiVersion: v1\nkind: ConfigMap\n",
			},
			wantErr: true,
			errMsg:  "does not contain a root Kptfile",
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			c := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(&porchapi.PackageRevisionResources{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-repo.test-pkg.v1",
						Namespace: ns,
					},
					Spec: porchapi.PackageRevisionResourcesSpec{
						Resources: tc.resources,
					},
				}).
				Build()

			r := &runner{
				ctx:      ctx,
				getFlags: cmdutil.Options{ConfigFlags: &genericclioptions.ConfigFlags{Namespace: &ns}},
				client:   c,
			}

			out := &bytes.Buffer{}
			cmd := &cobra.Command{}
			cmd.SetOut(out)

			err := r.showKptfileContent(cmd, "test-repo.test-pkg.v1")
			if tc.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tc.errMsg)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.wantOut, out.String())
			}
		})
	}

	// Test nil namespace
	t.Run("nil namespace", func(t *testing.T) {
		r := &runner{
			ctx:      ctx,
			getFlags: cmdutil.Options{ConfigFlags: &genericclioptions.ConfigFlags{Namespace: nil}},
			client:   fake.NewClientBuilder().WithScheme(scheme).Build(),
		}
		cmd := &cobra.Command{}
		err := r.showKptfileContent(cmd, "test-repo.test-pkg.v1")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "namespace is not configured")
	})

	// Test empty namespace
	t.Run("empty namespace", func(t *testing.T) {
		emptyNs := ""
		r := &runner{
			ctx:      ctx,
			getFlags: cmdutil.Options{ConfigFlags: &genericclioptions.ConfigFlags{Namespace: &emptyNs}},
			client:   fake.NewClientBuilder().WithScheme(scheme).Build(),
		}
		cmd := &cobra.Command{}
		err := r.showKptfileContent(cmd, "test-repo.test-pkg.v1")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "namespace is not configured")
	})

	// Test package not found
	t.Run("package not found", func(t *testing.T) {
		c := fake.NewClientBuilder().WithScheme(scheme).Build()
		r := &runner{
			ctx:      ctx,
			getFlags: cmdutil.Options{ConfigFlags: &genericclioptions.ConfigFlags{Namespace: &ns}},
			client:   c,
		}
		cmd := &cobra.Command{}
		err := r.showKptfileContent(cmd, "nonexistent-pkg")
		assert.Error(t, err)
	})
}
