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
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	kptfilev1 "github.com/kptdev/kpt/pkg/api/kptfile/v1"
	fnsdk "github.com/kptdev/krm-functions-sdk/go/fn"
	porchapi "github.com/nephio-project/porch/api/porch/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/utils/ptr"
)

const (
	testNs      = "ns"
	testPRRName = "test-repo.test-package.v1"

	testKptfileMinimal = `apiVersion: kpt.dev/v1
kind: Kptfile
metadata:
  name: test-package
`
)

func TestGetResourceFileKubeObject(t *testing.T) {
	const testConfigMapYAML = `apiVersion: v1
kind: ConfigMap
metadata:
  name: test-config
  namespace: ns
`

	testCases := map[string]struct {
		prr *porchapi.PackageRevisionResources

		file string
		kind string
		name string

		wantErr  string
		wantKind string
		wantName string
	}{
		"nil resources": {
			prr: &porchapi.PackageRevisionResources{
				ObjectMeta: metav1.ObjectMeta{Namespace: testNs, Name: testPRRName},
				Spec:       porchapi.PackageRevisionResourcesSpec{Resources: nil},
			},
			file:    "cm.yaml",
			wantErr: "nil resources found",
		},
		"file not found": {
			prr: &porchapi.PackageRevisionResources{
				ObjectMeta: metav1.ObjectMeta{Namespace: testNs, Name: testPRRName},
				Spec: porchapi.PackageRevisionResourcesSpec{
					Resources: map[string]string{"other.yaml": testConfigMapYAML},
				},
			},
			file:    "cm.yaml",
			wantErr: `"cm.yaml" not found`,
		},
		"invalid yaml": {
			prr: &porchapi.PackageRevisionResources{
				ObjectMeta: metav1.ObjectMeta{Namespace: testNs, Name: testPRRName},
				Spec: porchapi.PackageRevisionResourcesSpec{
					Resources: map[string]string{"bad.yaml": "@&:: not valid yaml"},
				},
			},
			file:    "bad.yaml",
			wantErr: "failed to parse",
		},
		"kind not found": {
			prr: &porchapi.PackageRevisionResources{
				ObjectMeta: metav1.ObjectMeta{Namespace: testNs, Name: testPRRName},
				Spec: porchapi.PackageRevisionResourcesSpec{
					Resources: map[string]string{"cm.yaml": testConfigMapYAML},
				},
			},
			file:    "cm.yaml",
			kind:    "Secret",
			wantErr: `does not contain kind "Secret"`,
		},
		"name not found": {
			prr: &porchapi.PackageRevisionResources{
				ObjectMeta: metav1.ObjectMeta{Namespace: testNs, Name: testPRRName},
				Spec: porchapi.PackageRevisionResourcesSpec{
					Resources: map[string]string{"cm.yaml": testConfigMapYAML},
				},
			},
			file:    "cm.yaml",
			kind:    "ConfigMap",
			name:    "other",
			wantErr: `does not contain resource named "other"`,
		},
		"success no kind or name": {
			prr: &porchapi.PackageRevisionResources{
				ObjectMeta: metav1.ObjectMeta{Namespace: testNs, Name: testPRRName},
				Spec: porchapi.PackageRevisionResourcesSpec{
					Resources: map[string]string{"cm.yaml": testConfigMapYAML},
				},
			},
			file:     "cm.yaml",
			wantKind: "ConfigMap",
			wantName: "test-config",
		},
		"success with matching kind and name": {
			prr: &porchapi.PackageRevisionResources{
				ObjectMeta: metav1.ObjectMeta{Namespace: testNs, Name: testPRRName},
				Spec: porchapi.PackageRevisionResourcesSpec{
					Resources: map[string]string{"cm.yaml": testConfigMapYAML},
				},
			},
			file:     "cm.yaml",
			kind:     "ConfigMap",
			name:     "test-config",
			wantKind: "ConfigMap",
			wantName: "test-config",
		},
		"empty kind skips kind check, wrong kind in file": {
			prr: &porchapi.PackageRevisionResources{
				ObjectMeta: metav1.ObjectMeta{Namespace: testNs, Name: testPRRName},
				Spec: porchapi.PackageRevisionResourcesSpec{
					Resources: map[string]string{"cm.yaml": testConfigMapYAML},
				},
			},
			file:     "cm.yaml",
			kind:     "",
			wantKind: "ConfigMap",
			wantName: "test-config",
		},
		"empty name skips name check": {
			prr: &porchapi.PackageRevisionResources{
				ObjectMeta: metav1.ObjectMeta{Namespace: testNs, Name: testPRRName},
				Spec: porchapi.PackageRevisionResourcesSpec{
					Resources: map[string]string{"cm.yaml": testConfigMapYAML},
				},
			},
			file:     "cm.yaml",
			kind:     "ConfigMap",
			name:     "",
			wantKind: "ConfigMap",
			wantName: "test-config",
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			ko, err := GetResourceFileKubeObject(tc.prr, tc.file, tc.kind, tc.name)
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				assert.Nil(t, ko)
			} else {
				require.NoError(t, err)
				require.NotNil(t, ko)
				assert.Equal(t, tc.wantKind, ko.GetKind())
				assert.Equal(t, tc.wantName, ko.GetName())
			}
		})
	}
}

func TestGetResourceVersion(t *testing.T) {
	const (
		testKptRevisionMetadataWithVersion = `apiVersion: config.kubernetes.io/v1
kind: KptRevisionMetadata
metadata:
  name: test-repo.test-package.v1
  namespace: ns
  resourceVersion: "42"
`

		testConfigMapWrongKindYAML = `apiVersion: v1
kind: ConfigMap
metadata:
  name: other-cm
`
	)

	testCases := map[string]struct {
		prr     *porchapi.PackageRevisionResources
		want    string
		wantErr string
	}{
		"missing file": {
			prr: &porchapi.PackageRevisionResources{
				ObjectMeta: metav1.ObjectMeta{Namespace: testNs, Name: testPRRName},
				Spec: porchapi.PackageRevisionResourcesSpec{
					Resources: map[string]string{},
				},
			},
			wantErr: "not found",
		},
		"success": {
			prr: &porchapi.PackageRevisionResources{
				ObjectMeta: metav1.ObjectMeta{Namespace: testNs, Name: testPRRName},
				Spec: porchapi.PackageRevisionResourcesSpec{
					Resources: map[string]string{
						kptfilev1.RevisionMetaDataFileName: testKptRevisionMetadataWithVersion,
					},
				},
			},
			want: "42",
		},
		"wrong kind": {
			prr: &porchapi.PackageRevisionResources{
				ObjectMeta: metav1.ObjectMeta{Namespace: testNs, Name: testPRRName},
				Spec: porchapi.PackageRevisionResourcesSpec{
					Resources: map[string]string{
						kptfilev1.RevisionMetaDataFileName: testConfigMapWrongKindYAML,
					},
				},
			},
			wantErr: `does not contain kind "KptRevisionMetadata"`,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			got, err := GetResourceVersion(tc.prr)
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				assert.Empty(t, got)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.want, got)
			}
		})
	}
}

func TestAddRevisionMetadata(t *testing.T) {
	prr := &porchapi.PackageRevisionResources{
		TypeMeta: metav1.TypeMeta{
			APIVersion: porchapi.SchemeGroupVersion.String(),
			Kind:       "PackageRevisionResources",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:            testPRRName,
			Namespace:       testNs,
			ResourceVersion: "1",
		},
		Spec: porchapi.PackageRevisionResourcesSpec{
			Resources: map[string]string{
				"Kptfile": testKptfileMinimal,
			},
		},
	}

	require.NoError(t, AddRevisionMetadata(prr))
	raw, ok := prr.Spec.Resources[kptfilev1.RevisionMetaDataFileName]
	require.Truef(t, ok, "expected %q in resources", kptfilev1.RevisionMetaDataFileName)
	ko, err := fnsdk.ParseKubeObject([]byte(raw))
	require.NoError(t, err)
	assert.Equal(t, kptfilev1.RevisionMetaDataKind, ko.GetKind())
	assert.Equal(t, testPRRName, ko.GetName())
}

func TestReadRevisionMetadataFromDir(t *testing.T) {
	testKptRevisionMetadata := fmt.Sprintf(`apiVersion: config.kubernetes.io/v1
kind: KptRevisionMetadata
metadata:
  name: %s
  resourceVersion: "1"
`, testPRRName)

	testKptRevisionMetadataMultiDoc := testKptRevisionMetadata + "\n---\n" + strings.Replace(testKptRevisionMetadata, "1", "2", 1)

	testCases := map[string]struct {
		files   map[string]string
		wantErr string
	}{
		"empty path": {
			files:   nil,
			wantErr: "must specify package path",
		},
		"no matching files": {
			files: map[string]string{
				"Kptfile": testKptfileMinimal,
			},
			wantErr: "expected exactly one rnode",
		},
		"multiple in same file": {
			files: map[string]string{
				kptfilev1.RevisionMetaDataFileName: testKptRevisionMetadataMultiDoc,
			},
			wantErr: "expected exactly one rnode",
		},
		"success": {
			files: map[string]string{
				kptfilev1.RevisionMetaDataFileName: testKptRevisionMetadata,
			},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			dir := ""
			if tc.files != nil {
				dir = t.TempDir()
				for name, content := range tc.files {
					require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600))
				}
			}
			ko, err := ReadRevisionMetadataFromDir(dir)
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				assert.Nil(t, ko)
			} else {
				require.NoError(t, err)
				require.NotNil(t, ko)
				assert.Equal(t, testPRRName, ko.GetName())
			}
		})
	}
}

func TestRemoveRevisionMetadata(t *testing.T) {
	testCases := map[string]struct {
		resources map[string]string
	}{
		"success": {
			resources: map[string]string{
						kptfilev1.RevisionMetaDataFileName: "data",
					},
		},
		"nil resources is no-op": {
			resources: nil,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			prr := &porchapi.PackageRevisionResources{
				Spec: porchapi.PackageRevisionResourcesSpec{
					Resources: tc.resources,
				},
			}
			require.NoError(t, RemoveRevisionMetadata(prr))
			if tc.resources != nil {
				require.NotNil(t, prr.Spec.Resources)
				assert.NotContainsf(t, prr.Spec.Resources, kptfilev1.RevisionMetaDataFileName, "expected %q to be removed", kptfilev1.RevisionMetaDataFileName)
			} else {
				assert.Nil(t, prr.Spec.Resources)
			}
		})
	}
}

func TestEnsureNamespace(t *testing.T) {
	const kcfgNs = "ctx-ns"

	testKubeconfigWithContextNS := fmt.Sprintf(`apiVersion: v1
kind: Config
clusters:
- cluster:
    server: https://127.0.0.1:6443
    insecure-skip-tls-verify: true
  name: test
contexts:
- context:
    cluster: test
    user: test
    namespace: %s
  name: test
current-context: test
users:
- name: test
  user: {}
`, kcfgNs)

	const testKubeconfigNoNamespace = `apiVersion: v1
kind: Config
clusters:
- cluster:
    server: https://127.0.0.1:6443
    insecure-skip-tls-verify: true
  name: test
contexts:
- context:
    cluster: test
    user: test
  name: test
current-context: test
users:
- name: test
  user: {}
`

	tempDir := t.TempDir()
	withNsKubeconfigPath := filepath.Join(tempDir, "with-ns-kubeconfig")
	require.NoError(t, os.WriteFile(withNsKubeconfigPath, []byte(testKubeconfigWithContextNS), 0o600))

	noNsKubeconfigPath := filepath.Join(tempDir, "no-ns-kubeconfig")
	require.NoError(t, os.WriteFile(noNsKubeconfigPath, []byte(testKubeconfigNoNamespace), 0o600))

	testCases := map[string]struct {
		kubeconfigPath string
		envVar         string
		ns             *string

		expectedNs string
	}{
		"flag wins over kubeconfig": {
			kubeconfigPath: withNsKubeconfigPath,
			ns:             ptr.To("flag-ns"),
			expectedNs:     "flag-ns",
		},
		"flag with empty kubeconfig context": {
			kubeconfigPath: noNsKubeconfigPath,
			ns:             ptr.To("explicit"),
			expectedNs:     "explicit",
		},
		"empty flag": {
			kubeconfigPath: withNsKubeconfigPath,
			ns:             ptr.To(""),
			expectedNs:     "ctx-ns",
		},
		"nil flag": {
			kubeconfigPath: withNsKubeconfigPath,
			ns:             nil,
			expectedNs:     "ctx-ns",
		},
		"env var empty kubeconfig context": {
			kubeconfigPath: noNsKubeconfigPath,
			ns:             ptr.To(""),
			envVar:         "env-ns",
			expectedNs:     "env-ns",
		},
		"full fallback": {
			kubeconfigPath: noNsKubeconfigPath,
			ns:             ptr.To(""),
			expectedNs:     "default",
		},
		"invalid kubeconfig path": {
			kubeconfigPath: "missing-kubeconfig",
			ns:             ptr.To(""),
			expectedNs:     "default",
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			if tc.envVar != "" {
				t.Setenv("NAMESPACE", tc.envVar)
			}
			cfg := &genericclioptions.ConfigFlags{
				KubeConfig: &tc.kubeconfigPath,
				Namespace:  tc.ns,
			}
			assert.Equal(t, tc.expectedNs, EnsureNamespace(cfg))
		})
	}
}

func TestIsSamePackage(t *testing.T) {
	const impossibleName = "definitely-not-possible"
	testMetadata := fmt.Sprintf(`apiVersion: config.kubernetes.io/v1
kind: KptRevisionMetadata
metadata:
  name: %s
`, testPRRName)
	testMetadataImpossible := fmt.Sprintf(`apiVersion: config.kubernetes.io/v1
kind: KptRevisionMetadata
metadata:
  name: %s
`, impossibleName)

	testCases := map[string]struct {
		prName              string
		metadataFileContent string
		expectSame          bool
	}{
		"read error": {
			prName:     testPRRName,
			expectSame: false,
		},
		"equal": {
			metadataFileContent: testMetadata,
			prName:              "test-repo.test-package.v2",
			expectSame:          true,
		},
		"non-equal": {
			metadataFileContent: testMetadata,
			prName:              "test-repo.test-package-2.v1",
			expectSame:          false,
		},
		"full compare equal": {
			metadataFileContent: testMetadataImpossible,
			prName:              "definitely-not-possible",
			expectSame:          true,
		},
		"full compare non-equal": {
			metadataFileContent: testMetadataImpossible,
			prName:              "definitely-not-possible-as-well",
			expectSame:          false,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			dir := ""
			if tc.metadataFileContent != "" {
				dir = t.TempDir()
				require.NoError(t, os.WriteFile(filepath.Join(dir, kptfilev1.RevisionMetaDataFileName), []byte(tc.metadataFileContent), 0o600))
			}
			assert.Equal(t, tc.expectSame, IsSamePackage(dir, tc.prName))
		})
	}
}
