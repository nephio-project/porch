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

package porch

import (
	"time"

	apiv1alpha1 "github.com/nephio-project/porch/api/porch/v1alpha1"
	porch "github.com/nephio-project/porch/api/porch/v1alpha1"
	"github.com/nephio-project/porch/pkg/engine"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apiserver/pkg/registry/rest"
	genericapiserver "k8s.io/apiserver/pkg/server"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type RESTStorageOptions struct {
	Scheme               *runtime.Scheme
	Codecs               serializer.CodecFactory
	CaD                  engine.CaDEngine
	CoreClient           client.WithWatch
	TimeoutPerRepository time.Duration
	MaxConcurrentLists   int
}

func (r *RESTStorageOptions) NewRESTStorage() (genericapiserver.APIGroupInfo, error) {
	packages := &packages{
		TableConvertor: packageTableConvertor,
		packageCommon: packageCommon{
			scheme:                   r.Scheme,
			cad:                      r.CaD,
			gr:                       porch.Resource("packages"),
			coreClient:               r.CoreClient,
			updateStrategy:           packageStrategy{},
			createStrategy:           packageStrategy{},
			ListTimeoutPerRepository: r.TimeoutPerRepository,
			MaxConcurrentLists:       r.MaxConcurrentLists,
		},
	}

	packageRevisions := &packageRevisions{
		TableConvertor: packageRevisionTableConvertor,
		packageCommon: packageCommon{
			scheme:                   r.Scheme,
			cad:                      r.CaD,
			gr:                       porch.Resource("packagerevisions"),
			coreClient:               r.CoreClient,
			updateStrategy:           packageRevisionStrategy{},
			createStrategy:           packageRevisionStrategy{},
			ListTimeoutPerRepository: r.TimeoutPerRepository,
			MaxConcurrentLists:       r.MaxConcurrentLists,
		},
	}

	packageRevisionsApproval := &packageRevisionsApproval{
		common: packageCommon{
			scheme:                   r.Scheme,
			cad:                      r.CaD,
			coreClient:               r.CoreClient,
			gr:                       porch.Resource("packagerevisions"),
			updateStrategy:           packageRevisionApprovalStrategy{},
			createStrategy:           packageRevisionApprovalStrategy{},
			ListTimeoutPerRepository: r.TimeoutPerRepository,
			MaxConcurrentLists:       r.MaxConcurrentLists,
		},
	}

	packageRevisionResources := &packageRevisionResources{
		TableConvertor: packageRevisionResourcesTableConvertor,
		packageCommon: packageCommon{
			scheme:                   r.Scheme,
			cad:                      r.CaD,
			gr:                       porch.Resource("packagerevisionresources"),
			coreClient:               r.CoreClient,
			ListTimeoutPerRepository: r.TimeoutPerRepository,
			MaxConcurrentLists:       r.MaxConcurrentLists,
		},
	}

	group := genericapiserver.NewDefaultAPIGroupInfo(porch.GroupName, r.Scheme, metav1.ParameterCodec, r.Codecs)

	group.VersionedResourcesStorageMap = map[string]map[string]rest.Storage{
		apiv1alpha1.SchemeGroupVersion.Version: {
			"packages":                  packages,
			"packagerevisions":          packageRevisions,
			"packagerevisions/approval": packageRevisionsApproval,
			"packagerevisionresources":  packageRevisionResources,
		},
	}

	{
		gvk := schema.GroupVersionKind{
			Group:   apiv1alpha1.GroupName,
			Version: apiv1alpha1.SchemeGroupVersion.Version,
			Kind:    "Package",
		}
		if err := r.Scheme.AddFieldLabelConversionFunc(gvk, convertPackageFieldSelector); err != nil {
			return group, err
		}
	}
	{
		gvk := schema.GroupVersionKind{
			Group:   apiv1alpha1.GroupName,
			Version: apiv1alpha1.SchemeGroupVersion.Version,
			Kind:    "PackageRevision",
		}
		if err := r.Scheme.AddFieldLabelConversionFunc(gvk, convertPackageRevisionFieldSelector); err != nil {
			return group, err
		}
	}
	{
		gvk := schema.GroupVersionKind{
			Group:   apiv1alpha1.GroupName,
			Version: apiv1alpha1.SchemeGroupVersion.Version,
			Kind:    "PackageRevisionResources",
		}
		if err := r.Scheme.AddFieldLabelConversionFunc(gvk, convertPackageRevisionFieldSelector); err != nil {
			return group, err
		}
	}

	return group, nil
}
