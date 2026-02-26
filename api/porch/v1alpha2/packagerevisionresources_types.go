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

package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PackageRevisionResources represents the contents of a package revision.
// This resource remains as an aggregated API due to size constraints (1-100MB).
// Use controller-runtime client to access this resource (no code-gen clients).
//
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +k8s:openapi-gen=true
// +kubebuilder:skip
type PackageRevisionResources struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PackageRevisionResourcesSpec   `json:"spec,omitempty"`
	Status PackageRevisionResourcesStatus `json:"status,omitempty"`
}

// PackageRevisionResourcesList contains a list of PackageRevisionResources
//
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:skip
type PackageRevisionResourcesList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []PackageRevisionResources `json:"items"`
}

// PackageRevisionResourcesSpec contains the content of a package
type PackageRevisionResourcesSpec struct {
	// PackageName identifies the package in the repository.
	PackageName string `json:"packageName,omitempty"`

	// WorkspaceName identifies the workspace of the package.
	WorkspaceName string `json:"workspaceName,omitempty"`

	// Revision identifies the version of the package.
	Revision int `json:"revision,omitempty"`

	// RepositoryName is the name of the Repository object containing this package.
	RepositoryName string `json:"repository,omitempty"`

	// Resources are the KRM resources in the package (filename -> YAML content).
	Resources map[string]string `json:"resources,omitempty"`
}

// PackageRevisionResourcesStatus represents state of the rendered package resources
type PackageRevisionResourcesStatus struct {
	// RenderStatus contains the result of rendering the package resources.
	RenderStatus RenderStatus `json:"renderStatus,omitempty"`
}
