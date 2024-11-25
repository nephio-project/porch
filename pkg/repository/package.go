// Copyright 2022, 2024 The kpt and Nephio Authors
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

package repository

import (
	"context"
	"fmt"

	api "github.com/nephio-project/porch/api/porch/v1alpha1"
	configapi "github.com/nephio-project/porch/api/porchconfig/v1alpha1"
	"github.com/nephio-project/porch/internal/kpt/builtins"
	"github.com/nephio-project/porch/pkg/objects"
	"github.com/nephio-project/porch/pkg/util"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type PackageFetcher struct {
	RepoOpener        RepositoryOpener
	ReferenceResolver ReferenceResolver
}

func (p *PackageFetcher) FetchRevision(ctx context.Context, packageRef *api.PackageRevisionRef, namespace string) (PackageRevision, error) {
	repositoryName, err := util.ParseRepositoryName(packageRef.Name)
	if err != nil {
		return nil, err
	}
	var resolved configapi.Repository
	if err := p.ReferenceResolver.ResolveReference(ctx, namespace, repositoryName, &resolved); err != nil {
		return nil, fmt.Errorf("cannot find repository %s/%s: %w", namespace, repositoryName, err)
	}

	repo, err := p.RepoOpener.OpenRepository(ctx, &resolved)
	if err != nil {
		return nil, err
	}

	revisions, err := repo.ListPackageRevisions(ctx, ListPackageRevisionFilter{KubeObjectName: packageRef.Name})
	if err != nil {
		return nil, err
	}

	var revision PackageRevision
	for _, rev := range revisions {
		if rev.KubeObjectName() == packageRef.Name {
			revision = rev
			break
		}
	}
	if revision == nil {
		return nil, fmt.Errorf("cannot find package revision %q", packageRef.Name)
	}

	return revision, nil
}

func (p *PackageFetcher) FetchResources(ctx context.Context, packageRef *api.PackageRevisionRef, namespace string) (*api.PackageRevisionResources, error) {
	revision, err := p.FetchRevision(ctx, packageRef, namespace)
	if err != nil {
		return nil, err
	}

	resources, err := revision.GetResources(ctx)
	if err != nil {
		return nil, fmt.Errorf("cannot read contents of package %q: %w", packageRef.Name, err)
	}
	return resources, nil
}

func BuildPackageConfig(ctx context.Context, obj *api.PackageRevision, parent PackageRevision) (*builtins.PackageConfig, error) {
	config := &builtins.PackageConfig{}

	parentPath := ""

	var parentConfig *unstructured.Unstructured
	if parent != nil {
		parentObj, err := parent.GetPackageRevision(ctx)
		if err != nil {
			return nil, err
		}
		parentPath = parentObj.Spec.PackageName

		resources, err := parent.GetResources(ctx)
		if err != nil {
			return nil, fmt.Errorf("error getting resources from parent package %q: %w", parentObj.Name, err)
		}
		configMapObj, err := extractContextConfigMap(resources.Spec.Resources)
		if err != nil {
			return nil, fmt.Errorf("error getting configuration from parent package %q: %w", parentObj.Name, err)
		}
		parentConfig = configMapObj

		if parentConfig != nil {
			// TODO: Should we support kinds other than configmaps?
			var parentConfigMap corev1.ConfigMap
			if err := runtime.DefaultUnstructuredConverter.FromUnstructured(parentConfig.Object, &parentConfigMap); err != nil {
				return nil, fmt.Errorf("error parsing ConfigMap from parent configuration: %w", err)
			}
			if s := parentConfigMap.Data[builtins.ConfigKeyPackagePath]; s != "" {
				parentPath = s + "/" + parentPath
			}
		}
	}

	if parentPath == "" {
		config.PackagePath = obj.Spec.PackageName
	} else {
		config.PackagePath = parentPath + "/" + obj.Spec.PackageName
	}

	return config, nil
}

// ExtractContextConfigMap returns the package-context configmap, if found
func extractContextConfigMap(resources map[string]string) (*unstructured.Unstructured, error) {
	unstructureds, err := objects.Parser{}.AsUnstructureds(resources)
	if err != nil {
		return nil, err
	}

	var matches []*unstructured.Unstructured
	for _, o := range unstructureds {
		configMapGK := schema.GroupKind{Kind: "ConfigMap"}
		if o.GroupVersionKind().GroupKind() == configMapGK {
			if o.GetName() == builtins.PkgContextName {
				matches = append(matches, o)
			}
		}
	}
	if len(matches) == 0 {
		return nil, nil
	}

	if len(matches) > 1 {
		return nil, fmt.Errorf("found multiple configmaps matching name %q", builtins.PkgContextFile)
	}

	return matches[0], nil
}
