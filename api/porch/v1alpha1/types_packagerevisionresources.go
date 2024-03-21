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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// PorchPkgRevisionResources
// +k8s:openapi-gen=true
type PorchPkgRevisionResources struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PorchPkgRevisionResourcesSpec   `json:"spec,omitempty"`
	Status PorchPkgRevisionResourcesStatus `json:"status,omitempty"`
}

// PorchPkgRevisionResourcesList
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type PorchPkgRevisionResourcesList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []PorchPkgRevisionResources `json:"items"`
}

// PorchPkgRevisionResourcesSpec represents resources (as ResourceList serialized as yaml string) of the PorchPkgRevision.
type PorchPkgRevisionResourcesSpec struct {
	// PorchPkgName identifies the PorchPkg in the repository.
	PorchPkgName string `json:"packageName,omitempty"`

	// WorkspaceName identifies the workspace of the PorchPkg.
	WorkspaceName WorkspaceName `json:"workspaceName,omitempty"`

	// Revision identifies the version of the PorchPkg.
	Revision string `json:"revision,omitempty"`

	// RepositoryName is the name of the Repository object containing this PorchPkg.
	RepositoryName string `json:"repository,omitempty"`

	// Resources are the content of the PorchPkg.
	Resources map[string]string `json:"resources,omitempty"`
}

// PorchPkgRevisionResourcesStatus represents state of the rendered PorchPkg resources.
type PorchPkgRevisionResourcesStatus struct {
	// RenderStatus contains the result of rendering the PorchPkg resources.
	RenderStatus RenderStatus `json:"renderStatus,omitempty"`
}
