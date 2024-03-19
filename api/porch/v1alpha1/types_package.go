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

// PorchPkg
// +k8s:openapi-gen=true
type PorchPkg struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PorchPkgSpec   `json:"spec,omitempty"`
	Status PorchPkgStatus `json:"status,omitempty"`
}

// PorchPkgList
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type PorchPkgList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []PorchPkg `json:"items"`
}

// PorchPkgSpec defines the desired state of PorchPkg
type PorchPkgSpec struct {
	// PorchPkgName identifies the PorchPkg in the repository.
	PorchPkgName string `json:"packageName,omitempty"`

	// RepositoryName is the name of the Repository object containing this Package.
	RepositoryName string `json:"repository,omitempty"`
}

// PorchPkgStatus defines the observed state of PorchPkg
type PorchPkgStatus struct {
	// LatestRevision identifies the PorchPkg revision that is the latest
	// published PorchPkg revision belonging to this PorchPkg
	LatestRevision string `json:"latestRevision,omitempty"`
}
