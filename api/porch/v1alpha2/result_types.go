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

// ResultList constants
const (
	// Deprecated: prefer ResultListGVK
	ResultListKind = "FunctionResultList"
	// Deprecated: prefer ResultListGVK
	ResultListGroup = "kpt.dev"
	// Deprecated: prefer ResultListGVK
	ResultListVersion = "v1"
	// Deprecated: prefer ResultListGVK
	ResultListAPIVersion = ResultListGroup + "/" + ResultListVersion
)

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
	// TODO(droot): This is required for making structured results subpackage aware.
	// Enable this once test harness supports filepath based assertions.
	// Pkg is OS specific Absolute path to the package.
	// Pkg string `yaml:"pkg,omitempty"`
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
