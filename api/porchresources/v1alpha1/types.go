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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PackageRevisionResources represents the contents of a package revision.
// This resource is in resources.porch.kpt.dev API group (separate from PackageRevision)
// due to Kubernetes routing constraints and remains as aggregated API due to size (1-100MB).
// Use controller-runtime client to access this resource.
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
	// Resources are the KRM resources in the package (filename -> YAML content).
	Resources map[string]string `json:"resources,omitempty"`
}

// PackageRevisionResourcesStatus represents state of the rendered package resources
type PackageRevisionResourcesStatus struct {
	// RenderStatus contains the result of rendering the package resources.
	RenderStatus RenderStatus `json:"renderStatus,omitempty"`
}

// RenderStatus represents the result of performing render operation on package resources
type RenderStatus struct {
	// Result of the task execution
	Result *Result `json:"result,omitempty"`

	// Err will be set if the rendering encountered an error
	Err string `json:"err,omitempty"`
}

// ResultList contains aggregated results from multiple functions.
// These types are intentionally duplicated from the kpt library to maintain API independence.
//
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:skip
type ResultList struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// ExitCode is the exit code of kpt command
	ExitCode int `json:"exitCode"`
	// Items contain a list of function result
	Items []*Result `json:"items,omitempty"`
}

// Result contains the structured result from an individual function
type Result struct {
	// Image is the full name of the image that generates this result
	// Image and Exec are mutually exclusive
	Image string `json:"image,omitempty"`
	// ExecPath is the absolute os-specific path to the executable file
	// If user provides an executable file with commands, ExecPath should
	// contain the entire input string.
	ExecPath string `json:"exec,omitempty"`
	// Stderr is the content in function stderr
	Stderr string `json:"stderr,omitempty"`
	// ExitCode is the exit code from running the function
	ExitCode int `json:"exitCode"`
	// Results is the list of results for the function
	Results []ResultItem `json:"results,omitempty"`
}

// ResultItem defines a validation result
type ResultItem struct {
	// Message is a human readable message. This field is required.
	Message string `json:"message,omitempty"`

	// Severity is the severity of this result
	Severity string `json:"severity,omitempty"`

	// ResourceRef is a reference to a resource.
	// Required fields: apiVersion, kind, name.
	ResourceRef *ResourceIdentifier `json:"resourceRef,omitempty"`

	// Field is a reference to the field in a resource this result refers to
	Field *Field `json:"field,omitempty"`

	// File references a file containing the resource this result refers to
	File *File `json:"file,omitempty"`

	// Tags is an unstructured key value map stored with a result that may be set
	// by external tools to store and retrieve arbitrary metadata
	Tags map[string]string `json:"tags,omitempty"`
}

// File references a file containing a resource
type File struct {
	// Path is relative path to the file containing the resource.
	// This field is required.
	Path string `json:"path,omitempty"`

	// Index is the index into the file containing the resource
	// (i.e. if there are multiple resources in a single file)
	Index int `json:"index,omitempty"`
}

// Field references a field in a resource
type Field struct {
	// Path is the field path. This field is required.
	Path string `json:"path,omitempty"`

	// CurrentValue is the current field value
	CurrentValue string `json:"currentValue,omitempty"`

	// ProposedValue is the proposed value of the field to fix an issue.
	ProposedValue string `json:"proposedValue,omitempty"`
}

// ResourceIdentifier contains the information needed to uniquely identify a resource in a cluster
type ResourceIdentifier struct {
	metav1.TypeMeta `json:",inline"`
	NameMeta        `json:",inline"`
}

// NameMeta contains name information
type NameMeta struct {
	// Name is the metadata.name field of a Resource
	Name string `json:"name,omitempty"`
	// Namespace is the metadata.namespace field of a Resource
	Namespace string `json:"namespace,omitempty"`
}
