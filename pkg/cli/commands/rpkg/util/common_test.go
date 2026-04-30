// Copyright 2026 The kpt and Nephio Authors
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

package util

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"testing"

	porchapi "github.com/nephio-project/porch/api/porch/v1alpha1"
	configapi "github.com/nephio-project/porch/api/porchconfig/v1alpha1"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// -- InitClient tests --

func TestInitClient_EmptyNamespaceReturnsError(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().String("namespace", "", "")
	_ = cmd.Flags().Set("namespace", "")

	_, err := InitClient(cmd, nil)
	assert.ErrorContains(t, err, "namespace flag specified without a value")
}

func TestInitClient_NamespaceNotSetPassesValidation(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().String("namespace", "default", "")

	assert.Panics(t, func() {
		_, _ = InitClient(cmd, nil)
	}, "should panic in CreateClientWithFlags, not return a namespace error")
}

func TestInitClient_ValidConfigReturnsClient(t *testing.T) {
	cmd := &cobra.Command{}
	kubeconfig := createTempKubeconfig(t)
	cfg := genericclioptions.NewConfigFlags(false)
	cfg.KubeConfig = &kubeconfig

	c, err := InitClient(cmd, cfg)
	assert.NoError(t, err)
	assert.NotNil(t, c)
}

func TestInitClient_ShortFlagNAlsoValidated(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().StringP("namespace", "n", "", "")
	_ = cmd.Flags().Set("namespace", "")

	_, err := InitClient(cmd, nil)
	assert.ErrorContains(t, err, "namespace flag specified without a value")
}

// -- MakePreRunE tests --

func TestMakePreRunE_SetsClient(t *testing.T) {
	kubeconfig := createTempKubeconfig(t)
	cfg := genericclioptions.NewConfigFlags(false)
	cfg.KubeConfig = &kubeconfig

	var c client.Client
	preRunE := MakePreRunE("test.preRunE", cfg, &c)

	err := preRunE(&cobra.Command{}, nil)
	assert.NoError(t, err)
	assert.NotNil(t, c)
}

func TestMakePreRunE_EmptyNamespaceReturnsError(t *testing.T) {
	var c client.Client
	preRunE := MakePreRunE("test.preRunE", &genericclioptions.ConfigFlags{}, &c)

	cmd := &cobra.Command{}
	cmd.Flags().String("namespace", "", "")
	_ = cmd.Flags().Set("namespace", "")

	err := preRunE(cmd, nil)
	assert.ErrorContains(t, err, "namespace flag specified without a value")
}

// -- CreateScheme tests --

func TestCreateScheme_RegistersPorchTypes(t *testing.T) {
	scheme, err := CreateScheme()
	require.NoError(t, err)
	assert.NotNil(t, scheme)

	gvk := porchapi.SchemeGroupVersion.WithKind("PackageRevision")
	assert.True(t, scheme.Recognizes(gvk), "scheme should recognize PackageRevision")
}

func TestCreateScheme_RegistersConfigAndCoreTypes(t *testing.T) {
	scheme, err := CreateScheme()
	require.NoError(t, err)

	configGVK := configapi.GroupVersion.WithKind("Repository")
	assert.True(t, scheme.Recognizes(configGVK), "scheme should recognize porchconfig Repository")

	coreGVK := corev1.SchemeGroupVersion.WithKind("ConfigMap")
	assert.True(t, scheme.Recognizes(coreGVK), "scheme should recognize core/v1 ConfigMap")
}

func TestBuildScheme_ReturnsErrorWhenAdderFails(t *testing.T) {
	wantErr := fmt.Errorf("boom")
	scheme, err := buildScheme([]func(*runtime.Scheme) error{
		func(*runtime.Scheme) error { return wantErr },
	})
	assert.Nil(t, scheme)
	assert.ErrorIs(t, err, wantErr)
}

func TestBuildScheme_StopsAtFirstError(t *testing.T) {
	called := 0
	first := func(*runtime.Scheme) error { called++; return fmt.Errorf("first") }
	second := func(*runtime.Scheme) error { called++; return nil }
	_, err := buildScheme([]func(*runtime.Scheme) error{first, second})
	assert.ErrorContains(t, err, "first")
	assert.Equal(t, 1, called, "should not call subsequent adders after error")
}

// -- RunForEachPackage tests --

func TestRunForEachPackage_AllSucceed(t *testing.T) {
	cmd, stdout, stderr := setupCmdBuffers()

	fc := setupFakeClient(t,
		newPR("pkg-a", "ns", porchapi.PackageRevisionLifecycleDraft),
		newPR("pkg-b", "ns", porchapi.PackageRevisionLifecycleDraft),
	)
	err := RunForEachPackage(context.Background(), "cmdrpkgtest", fc, cmd, "ns",
		[]string{"pkg-a", "pkg-b"}, true, false,
		func(_ context.Context, _ client.Client, pr *porchapi.PackageRevision) (string, error) {
			return fmt.Sprintf("%s done", pr.Name), nil
		})
	assert.NoError(t, err)
	assert.Contains(t, stdout.String(), "pkg-a done")
	assert.Contains(t, stdout.String(), "pkg-b done")
	assert.Empty(t, stderr.String())
}

func TestRunForEachPackage_PartialFailure(t *testing.T) {
	cmd, stdout, stderr := setupCmdBuffers()

	fc := setupFakeClient(t,
		newPR("pkg-a", "ns", porchapi.PackageRevisionLifecycleDraft),
		newPR("pkg-b", "ns", porchapi.PackageRevisionLifecycleDraft),
	)
	err := RunForEachPackage(context.Background(), "cmdrpkgtest", fc, cmd, "ns",
		[]string{"pkg-a", "pkg-b"}, true, false,
		func(_ context.Context, _ client.Client, pr *porchapi.PackageRevision) (string, error) {
			if pr.Name == "pkg-b" {
				return "", fmt.Errorf("something went wrong")
			}
			return fmt.Sprintf("%s done", pr.Name), nil
		})
	assert.Error(t, err)
	assert.Contains(t, stdout.String(), "pkg-a done")
	assert.Contains(t, stderr.String(), "pkg-b failed")
}

func TestRunForEachPackage_NotFoundReportsError(t *testing.T) {
	cmd, _, stderr := setupCmdBuffers()

	fc := setupFakeClient(t)
	err := RunForEachPackage(context.Background(), "cmdrpkgtest", fc, cmd, "ns",
		[]string{"missing-pkg"}, true, false,
		func(_ context.Context, _ client.Client, pr *porchapi.PackageRevision) (string, error) {
			return fmt.Sprintf("%s done", pr.Name), nil
		})
	assert.Error(t, err)
	assert.Contains(t, stderr.String(), "missing-pkg failed")
}

func TestRunForEachPackage_WithoutRetry(t *testing.T) {
	cmd, stdout, _ := setupCmdBuffers()

	fc := setupFakeClient(t,
		newPR("pkg-a", "ns", porchapi.PackageRevisionLifecycleDraft),
	)
	err := RunForEachPackage(context.Background(), "cmdrpkgtest", fc, cmd, "ns",
		[]string{"pkg-a"}, false, false,
		func(_ context.Context, _ client.Client, pr *porchapi.PackageRevision) (string, error) {
			return fmt.Sprintf("%s deleted", pr.Name), nil
		})
	assert.NoError(t, err)
	assert.Contains(t, stdout.String(), "pkg-a deleted")
}

func TestRunForEachPackage_EmptySuccessMessageSkipsPrint(t *testing.T) {
	cmd, stdout, stderr := setupCmdBuffers()

	fc := setupFakeClient(t,
		newPR("pkg-a", "ns", porchapi.PackageRevisionLifecycleDraft),
	)
	err := RunForEachPackage(context.Background(), "cmdrpkgtest", fc, cmd, "ns",
		[]string{"pkg-a"}, true, false,
		func(_ context.Context, _ client.Client, _ *porchapi.PackageRevision) (string, error) {
			return "", nil
		})
	assert.NoError(t, err)
	assert.Empty(t, stdout.String(), "empty success message should not print anything")
	assert.Empty(t, stderr.String())
}

func TestRunForEachPackage_WithoutRetryGetFails(t *testing.T) {
	cmd, _, stderr := setupCmdBuffers()

	fc := setupFakeClient(t)
	err := RunForEachPackage(context.Background(), "cmdrpkgtest", fc, cmd, "ns",
		[]string{"missing"}, false, false,
		func(_ context.Context, _ client.Client, pr *porchapi.PackageRevision) (string, error) {
			return fmt.Sprintf("%s done", pr.Name), nil
		})
	assert.Error(t, err)
	assert.Contains(t, stderr.String(), "missing failed")
}

func TestRunForEachPackage_ReadinessCheckFails(t *testing.T) {
	cmd, _, stderr := setupCmdBuffers()

	fc := setupFakeClient(t,
		newPR("pkg-a", "ns", porchapi.PackageRevisionLifecycleDraft),
	)
	// Set a readiness gate that is not met
	pr := &porchapi.PackageRevision{}
	_ = fc.Get(context.Background(), client.ObjectKey{Namespace: "ns", Name: "pkg-a"}, pr)
	pr.Spec.ReadinessGates = []porchapi.ReadinessGate{{ConditionType: "test.Ready"}}
	_ = fc.Update(context.Background(), pr)

	err := RunForEachPackage(context.Background(), "cmdrpkgtest", fc, cmd, "ns",
		[]string{"pkg-a"}, true, true,
		func(_ context.Context, _ client.Client, pr *porchapi.PackageRevision) (string, error) {
			return fmt.Sprintf("%s done", pr.Name), nil
		})
	assert.Error(t, err)
	assert.Contains(t, stderr.String(), "readiness conditions not met")
}

func TestRetryOnConflict_ReturnsErrorWhenFnFails(t *testing.T) {
	called := 0
	err := retryOnConflict(func() error {
		called++
		return fmt.Errorf("connection refused")
	})
	assert.ErrorContains(t, err, "connection refused")
	assert.Equal(t, 1, called, "non-conflict error should not retry")
}

func TestRetryOnConflict_ReturnsNilOnSuccess(t *testing.T) {
	err := retryOnConflict(func() error {
		return nil
	})
	assert.NoError(t, err)
}

// -- test helpers --

func setupCmdBuffers() (cmd *cobra.Command, stdout *bytes.Buffer, stderr *bytes.Buffer) {
	stdout = &bytes.Buffer{}
	stderr = &bytes.Buffer{}
	cmd = &cobra.Command{}
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	return
}

func newPR(name, ns string, lifecycle porchapi.PackageRevisionLifecycle) *porchapi.PackageRevision {
	return &porchapi.PackageRevision{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec:       porchapi.PackageRevisionSpec{Lifecycle: lifecycle},
	}
}

func setupFakeClient(t *testing.T, objs ...client.Object) client.Client {
	t.Helper()
	scheme, err := CreateScheme()
	require.NoError(t, err)
	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
}

func createTempKubeconfig(t *testing.T) string {
	t.Helper()
	content := `apiVersion: v1
kind: Config
clusters:
- cluster:
    server: https://127.0.0.1:6443
  name: test
contexts:
- context:
    cluster: test
    user: test
  name: test
current-context: test
users:
- name: test
  user:
    token: fake-token
`
	f, err := os.CreateTemp(t.TempDir(), "kubeconfig-*")
	require.NoError(t, err)
	_, err = f.WriteString(content)
	require.NoError(t, err)
	require.NoError(t, f.Close())
	return f.Name()
}
