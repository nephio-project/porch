// Copyright 2023,2026 The kpt and Nephio Authors
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
	"strings"

	kptfilev1 "github.com/kptdev/kpt/pkg/api/kptfile/v1"
	fnsdk "github.com/kptdev/krm-functions-sdk/go/fn"
	porchapi "github.com/nephio-project/porch/api/porch/v1alpha1"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"sigs.k8s.io/kustomize/kyaml/kio"
)

func GetResourceFileKubeObject(prr *porchapi.PackageRevisionResources, file, kind, name string) (*fnsdk.KubeObject, error) {
	if prr.Spec.Resources == nil {
		return nil, fmt.Errorf("nil resources found for PackageRevisionResources '%s/%s'", prr.Namespace, prr.Name)
	}

	if _, ok := prr.Spec.Resources[file]; !ok {
		return nil, fmt.Errorf("%q not found in PackageRevisionResources '%s/%s'", file, prr.Namespace, prr.Name)
	}

	ko, err := fnsdk.ParseKubeObject([]byte(prr.Spec.Resources[file]))
	if err != nil {
		return nil, fmt.Errorf("failed to parse %q of PackageRevisionResources %s/%s: %w", file, prr.Namespace, prr.Name, err)
	}
	if kind != "" && ko.GetKind() != kind {
		return nil, fmt.Errorf("%q does not contain kind %q in PackageRevisionResources '%s/%s'", file, kind, prr.Namespace, prr.Name)
	}
	if name != "" && ko.GetName() != name {
		return nil, fmt.Errorf("%q does not contain resource named %q in PackageRevisionResources '%s/%s'", file, name, prr.Namespace, prr.Name)
	}

	return ko, nil
}

func GetResourceVersion(prr *porchapi.PackageRevisionResources) (string, error) {
	ko, err := GetResourceFileKubeObject(prr, kptfilev1.RevisionMetaDataFileName, kptfilev1.RevisionMetaDataKind, "")
	if err != nil {
		return "", err
	}
	rv, _, _ := ko.NestedString("metadata", "resourceVersion")
	return rv, nil
}

func AddRevisionMetadata(prr *porchapi.PackageRevisionResources) error {
	kptMetaDataKo := fnsdk.NewEmptyKubeObject()
	if err := kptMetaDataKo.SetAPIVersion(prr.APIVersion); err != nil {
		return fmt.Errorf("cannot set Api Version: %v", err)
	}
	if err := kptMetaDataKo.SetKind(kptfilev1.RevisionMetaDataKind); err != nil {
		return fmt.Errorf("cannot set Kind: %v", err)
	}
	if err := kptMetaDataKo.SetNestedField(prr.GetObjectMeta(), "metadata"); err != nil {
		return fmt.Errorf("cannot set metadata: %v", err)
	}
	prr.Spec.Resources[kptfilev1.RevisionMetaDataFileName] = kptMetaDataKo.String()

	return nil
}

func ReadRevisionMetadataFromDir(path string) (*fnsdk.KubeObject, error) {
	reader := &kio.LocalPackageReader{
		PackagePath:           path,
		PackageFileName:       kptfilev1.KptFileName,
		MatchFilesGlob:        []string{kptfilev1.RevisionMetaDataFileName},
		IncludeSubpackages:    false,
		ErrorIfNonResources:   false,
		OmitReaderAnnotations: false,
		PreserveSeqIndent:     true,
	}

	rnodes, err := reader.Read()
	if err != nil {
		return nil, err
	}
	if len(rnodes) != 1 {
		return nil, fmt.Errorf("expected exactly one rnode for file %q, got %d",
			kptfilev1.RevisionMetaDataFileName, len(rnodes))
	}

	return fnsdk.MoveToKubeObject(rnodes[0]), nil
}

func RemoveRevisionMetadata(prr *porchapi.PackageRevisionResources) error {
	delete(prr.Spec.Resources, kptfilev1.RevisionMetaDataFileName)
	return nil
}

// EnsureNamespace tries to return a namespace from multiple different configs before defaulting to "default"
//
// Should only be used if the intention cannot be all namespaces!
func EnsureNamespace(cfg *genericclioptions.ConfigFlags) string {
	// if --namespace is set just return that
	if cfg.Namespace != nil && *cfg.Namespace != "" {
		return *cfg.Namespace
	}

	// try getting the namespace from the kubeconfig context
	if kcfgNs, _, err := cfg.ToRawKubeConfigLoader().Namespace(); err == nil && kcfgNs != "" {
		return kcfgNs
	}

	// try getting it from an env var
	if envNs := os.Getenv("NAMESPACE"); envNs != "" {
		return envNs
	}

	// fall back to "default"
	return "default"
}

func IsSamePackage(dir, prName string) bool {
	ko, err := ReadRevisionMetadataFromDir(dir)
	if err != nil {
		return false
	}

	return trimWorkspace(ko.GetName()) == trimWorkspace(prName)
}

func trimWorkspace(prName string) string {
	i := strings.LastIndex(prName, ".")

	if i == -1 {
		return prName
	}

	return prName[:i]
}
