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
	kptfilev1 "github.com/nephio-project/porch/pkg/kpt/api/kptfile/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// PorchPkgVariant represents an upstream and downstream porch package pair.
// The upstream package should already exist. The PorchPkgVariant controller is
// responsible for creating the downstream package revisions based on the spec.
type PorchPkgVariant struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PorchPkgVariantSpec   `json:"spec,omitempty"`
	Status PorchPkgVariantStatus `json:"status,omitempty"`
}

func (o *PorchPkgVariant) GetSpec() *PorchPkgVariantSpec {
	if o == nil {
		return nil
	}
	return &o.Spec
}

type AdoptionPolicy string
type DeletionPolicy string

const (
	AdoptionPolicyAdoptExisting AdoptionPolicy = "adoptExisting"
	AdoptionPolicyAdoptNone     AdoptionPolicy = "adoptNone"

	DeletionPolicyDelete DeletionPolicy = "delete"
	DeletionPolicyOrphan DeletionPolicy = "orphan"

	Finalizer = "config.porch.kpt.dev/packagevariants"
)

// PorchPkgVariantSpec defines the desired state of PorchPkgVariant
type PorchPkgVariantSpec struct {
	Upstream   *Upstream   `json:"upstream,omitempty"`
	Downstream *Downstream `json:"downstream,omitempty"`

	AdoptionPolicy AdoptionPolicy `json:"adoptionPolicy,omitempty"`
	DeletionPolicy DeletionPolicy `json:"deletionPolicy,omitempty"`

	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`

	PorchPkgContext *PorchPkgContext    `json:"packageContext,omitempty"`
	Pipeline        *kptfilev1.Pipeline `json:"pipeline,omitempty"`
	Injectors       []InjectionSelector `json:"injectors,omitempty"`
}

type Upstream struct {
	Repo     string `json:"repo,omitempty"`
	PorchPkg string `json:"package,omitempty"`
	Revision string `json:"revision,omitempty"`
}

type Downstream struct {
	Repo     string `json:"repo,omitempty"`
	PorchPkg string `json:"package,omitempty"`
}

// PorchPkgContext defines the data to be added or removed from the
// kptfile.kpt.dev ConfigMap during reconciliation.
type PorchPkgContext struct {
	Data       map[string]string `json:"data,omitempty"`
	RemoveKeys []string          `json:"removeKeys,omitempty"`
}

// InjectionSelector specifies how to select in-cluster objects for
// resolving injection points.
type InjectionSelector struct {
	Group   *string `json:"group,omitempty"`
	Version *string `json:"version,omitempty"`
	Kind    *string `json:"kind,omitempty"`
	Name    string  `json:"name"`
}

// PorchPkgVariantStatus defines the observed state of PorchPkgVariant
type PorchPkgVariantStatus struct {
	// Conditions describes the reconciliation state of the object.
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// DownstreamTargets contains the downstream targets that the PorchPkgVariant
	// either created or adopted.
	DownstreamTargets []DownstreamTarget `json:"downstreamTargets,omitempty"`
}

type DownstreamTarget struct {
	Name string `json:"name,omitempty"`
}

//+kubebuilder:object:root=true

// PorchPkgVariantList contains a list of PorchPkgVariant
type PorchPkgVariantList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PorchPkgVariant `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PorchPkgVariant{}, &PorchPkgVariantList{})
}
