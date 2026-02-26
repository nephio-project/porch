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

// RepositoryType specifies the type of repository
type RepositoryType string

const (
	RepositoryTypeGit RepositoryType = "git"
)

// UpstreamPackage specifies an upstream package source
type UpstreamPackage struct {
	// Type of the repository (i.e. git). If empty, upstreamRef will be used.
	Type RepositoryType `json:"type,omitempty"`

	// Git upstream package specification. Required if type is git.
	Git *GitPackage `json:"git,omitempty"`

	// UpstreamRef is the reference to the package from a registered repository.
	UpstreamRef *PackageRevisionRef `json:"upstreamRef,omitempty"`
}

// GitPackage describes a package stored in a Git repository
type GitPackage struct {
	// Repo is the address of the Git repository, for example:
	// https://github.com/GoogleCloudPlatform/blueprints.git
	Repo string `json:"repo"`

	// Ref is the git ref containing the package. Ref can be a branch, tag, or commit SHA.
	Ref string `json:"ref"`

	// Directory within the Git repository where the packages are stored.
	Directory string `json:"directory"`

	// SecretRef is a reference to secret containing authentication credentials.
	SecretRef SecretRef `json:"secretRef,omitempty"`
}

// SecretRef is a reference to a Secret
type SecretRef struct {
	// Name of the secret. The secret is expected to be located in the same namespace.
	Name string `json:"name"`
}

// PackageRevisionRef is a reference to a PackageRevision resource
type PackageRevisionRef struct {
	// Name is the name of the referenced PackageRevision resource.
	Name string `json:"name"`
}

// RepositoryRef identifies a reference to a Repository resource
type RepositoryRef struct {
	// Name of the Repository resource referenced.
	Name string `json:"name"`
}
