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

// This file contains types used in PackageRevision status fields.
// These types are intentionally duplicated from the kpt library to maintain API independence.

// Condition contains details for the current condition
type Condition struct {
	// Type of the condition
	Type string `json:"type"`

	// Status of the condition (True, False, Unknown)
	Status ConditionStatus `json:"status"`

	// Reason is a brief machine-readable explanation for the condition's last transition
	Reason string `json:"reason,omitempty"`

	// Message is a human-readable explanation for the condition's last transition
	Message string `json:"message,omitempty"`
}

// ConditionStatus represents the status of a condition
type ConditionStatus string

const (
	ConditionTrue    ConditionStatus = "True"
	ConditionFalse   ConditionStatus = "False"
	ConditionUnknown ConditionStatus = "Unknown"
)

// The following types (Locator, OriginType, and GitLock) are intentionally duplicated from the kpt library
// to maintain API independence and avoid external dependencies in the API package.

// OriginType defines the type of origin for a package
type OriginType string

// Locator is a resolved locator for the last fetch of the package
type Locator struct {
	// Type is the type of origin.
	Type OriginType `json:"type,omitempty"`

	// Git is the resolved locator for a package on Git.
	Git *GitLock `json:"git,omitempty"`
}

// GitLock is the resolved locator for a package on Git
type GitLock struct {
	// Repo is the git repository that was fetched.
	// e.g. 'https://github.com/kubernetes/examples.git'
	Repo string `json:"repo,omitempty"`

	// Directory is the sub directory of the git repository that was fetched.
	// e.g. 'staging/cockroachdb'
	Directory string `json:"directory,omitempty"`

	// Ref can be a Git branch, tag, or a commit SHA-1 that was fetched.
	// e.g. 'master'
	Ref string `json:"ref,omitempty"`

	// Commit is the SHA-1 for the last fetch of the package.
	// This is set by kpt for bookkeeping purposes.
	Commit string `json:"commit,omitempty"`
}
