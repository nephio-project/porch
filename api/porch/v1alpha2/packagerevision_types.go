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

// PackageRevision represents a package revision.
// In v1alpha2, PackageRevision is a CRD stored in etcd.
// Use controller-runtime client to access this resource (no code-gen clients).
//
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +k8s:openapi-gen=true
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:subresource:approval
// +kubebuilder:resource:path=packagerevisions,singular=packagerevision,shortName=rpkg
// +kubebuilder:printcolumn:name="Package",type=string,JSONPath=`.spec.packageName`
// +kubebuilder:printcolumn:name="WorkspaceName",type=string,JSONPath=`.spec.workspaceName`
// +kubebuilder:printcolumn:name="Revision",type=string,JSONPath=`.spec.revision`
// +kubebuilder:printcolumn:name="Latest",type=string,JSONPath=".metadata.labels['kpt.dev/latest-revision']"
// +kubebuilder:printcolumn:name="Lifecycle",type=string,JSONPath=`.spec.lifecycle`
// +kubebuilder:printcolumn:name="Repository",type=string,JSONPath=`.spec.repository`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
type PackageRevision struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PackageRevisionSpec   `json:"spec,omitempty"`
	Status PackageRevisionStatus `json:"status,omitempty"`
}

// PackageRevisionList contains a list of PackageRevision
//
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true
type PackageRevisionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []PackageRevision `json:"items"`
}

// Key and value of the latest package revision label
const (
	LatestPackageRevisionKey   = "kpt.dev/latest-revision"
	LatestPackageRevisionValue = "true"
)

// PkgRevFieldSelector defines field selectors for PackageRevision
type PkgRevFieldSelector string

const (
	PkgRevSelectorName          PkgRevFieldSelector = "metadata.name"
	PkgRevSelectorNamespace     PkgRevFieldSelector = "metadata.namespace"
	PkgRevSelectorRevision      PkgRevFieldSelector = "spec.revision"
	PkgRevSelectorPackageName   PkgRevFieldSelector = "spec.packageName"
	PkgRevSelectorRepository    PkgRevFieldSelector = "spec.repository"
	PkgRevSelectorWorkspaceName PkgRevFieldSelector = "spec.workspaceName"
	PkgRevSelectorLifecycle     PkgRevFieldSelector = "spec.lifecycle"
)

// PackageRevisionSelectableFields lists all selectable fields for PackageRevision
var PackageRevisionSelectableFields = []PkgRevFieldSelector{
	PkgRevSelectorName,
	PkgRevSelectorNamespace,
	PkgRevSelectorRevision,
	PkgRevSelectorPackageName,
	PkgRevSelectorRepository,
	PkgRevSelectorWorkspaceName,
	PkgRevSelectorLifecycle,
}

// PackageRevisionLifecycle represents the lifecycle state of a package revision
type PackageRevisionLifecycle string

const (
	PackageRevisionLifecycleDraft            PackageRevisionLifecycle = "Draft"
	PackageRevisionLifecycleProposed         PackageRevisionLifecycle = "Proposed"
	PackageRevisionLifecyclePublished        PackageRevisionLifecycle = "Published"
	PackageRevisionLifecycleDeletionProposed PackageRevisionLifecycle = "DeletionProposed"
)

// PackageRevisionSpec defines the desired state of PackageRevision
type PackageRevisionSpec struct {
	// PackageName identifies the package in the repository.
	PackageName string `json:"packageName,omitempty"`

	// RepositoryName is the name of the Repository object containing this package.
	RepositoryName string `json:"repository,omitempty"`

	// WorkspaceName is a short, unique description of the changes contained in this package revision.
	WorkspaceName string `json:"workspaceName,omitempty"`

	// Revision identifies the version of the package.
	Revision int `json:"revision,omitempty"`

	// Deprecated. Parent references a package that provides resources to us
	Parent *ParentReference `json:"parent,omitempty"`

	// Lifecycle specifies the lifecycle state of the package revision.
	Lifecycle PackageRevisionLifecycle `json:"lifecycle,omitempty"`

	// The task slice holds zero or more tasks that describe the operations
	// performed on the packagerevision. The are essentially a replayable history
	// of the packagerevision,
	//
	// Packagerevisions that were not created in Porch may have an
	// empty task list.
	//
	// Packagerevisions created and managed through Porch will always
	// have either an Init, Edit, or a Clone task as the first entry in their
	// task list. This represent packagerevisions created from scratch, based
	// a copy of a different revision in the same package, or a packagerevision
	// cloned from another package.
	// Each change to the packagerevision will result in a correspondig
	// task being added to the list of tasks. It will describe the operation
	// performed and will have a corresponding entry (commit or layer) in git
	// or oci.
	// The task slice describes the history of the packagerevision, so it
	// is an append only list (We might introduce some kind of compaction in the
	// future to keep the number of tasks at a reasonable number).
	Tasks []Task `json:"tasks,omitempty"`

	// ReadinessGates specifies conditions that must be met before the package is considered ready.
	ReadinessGates []ReadinessGate `json:"readinessGates,omitempty"`

	// PackageMetadata contains labels and annotations for the package.
	PackageMetadata *PackageMetadata `json:"packageMetadata,omitempty"`
}

// PackageMetadata contains metadata for a package
type PackageMetadata struct {
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

// ReadinessGate specifies a condition that must be met
type ReadinessGate struct {
	ConditionType string `json:"conditionType,omitempty"`
}

// Deprecated. ParentReference is a reference to a parent package
type ParentReference struct {
	// TODO: Should this be a revision or a package?

	// Name is the name of the parent PackageRevision
	Name string `json:"name"`
}

// PackageRevisionStatus defines the observed state of PackageRevision
type PackageRevisionStatus struct {
	// UpstreamLock identifies the upstream data for this package.
	UpstreamLock *Locator `json:"upstreamLock,omitempty"`

	// SelfLock identifies the location of the current package's data
	SelfLock *Locator `json:"selfLock,omitempty"`

	// PublishedBy is the identity of the user who approved the packagerevision.
	PublishedBy string `json:"publishedBy,omitempty"`

	// PublishedAt is the time when the packagerevision were approved.
	PublishedAt metav1.Time `json:"publishTimestamp,omitempty"`

	// Deployment is true if this is a deployment package (in a deployment repository).
	Deployment bool `json:"deployment,omitempty"`

	// Conditions describes the reconciliation state of the object.
	Conditions []Condition `json:"conditions,omitempty"`
}
