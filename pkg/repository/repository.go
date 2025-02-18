// Copyright 2022, 2024 The kpt and Nephio Authors
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

package repository

import (
	"context"
	"fmt"

	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/nephio-project/porch/api/porch/v1alpha1"
	kptfile "github.com/nephio-project/porch/pkg/kpt/api/kptfile/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// TODO: 	"sigs.k8s.io/kustomize/kyaml/filesys" FileSystem?
type PackageResources struct {
	Contents map[string]string
}

type PackageRevisionKey struct {
	Namespace, Repository, Package, Revision string
	WorkspaceName                            v1alpha1.WorkspaceName
}

func (n PackageRevisionKey) String() string {
	return fmt.Sprintf("%s.%s.%s.v%s.%s", n.Namespace, n.Repository, n.Package, n.Revision, string(n.WorkspaceName))
}

func (n PackageRevisionKey) NonNSString() string {
	return fmt.Sprintf("%s.%s.v%s.%s", n.Repository, n.Package, n.Revision, string(n.WorkspaceName))
}

func (n PackageRevisionKey) PackageKey() PackageKey {
	return PackageKey{
		Namespace:  n.Namespace,
		Repository: n.Repository,
		Package:    n.Package,
	}
}

func (n PackageRevisionKey) RepositoryKey() RepositoryKey {
	return RepositoryKey{
		Namespace:  n.Namespace,
		Repository: n.Repository,
	}
}

type PackageKey struct {
	Namespace, Repository, Package string
}

func (n PackageKey) String() string {
	return fmt.Sprintf("%s.%s.%s", n.Namespace, n.Repository, n.Package)
}

func (n PackageKey) NonNSString() string {
	return fmt.Sprintf("%s.%s", n.Repository, n.Package)
}

func (n PackageKey) RepositoryKey() RepositoryKey {
	return RepositoryKey{
		Namespace:  n.Namespace,
		Repository: n.Repository,
	}
}

type RepositoryKey struct {
	Namespace, Repository string
}

func (n RepositoryKey) String() string {
	return fmt.Sprintf("%s.%s", n.Namespace, n.Repository)
}

func (n RepositoryKey) NonNSString() string {
	return n.Repository
}

// PackageRevision is an abstract package version.
// We have a single object for both Revision and Resources, because conceptually they are one object.
// The best way we've found (so far) to represent them in k8s is as two resources, but they map to the same object.
// Interesting reading: https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#differing-representations
type PackageRevision interface {
	// KubeObjectName returns an encoded name for the object that should be unique.
	// More "readable" values are returned by Key()
	KubeObjectName() string

	// KubeObjectNamespace returns the namespace in which the PackageRevision
	// belongs.
	KubeObjectNamespace() string

	// UID returns a unique identifier for the PackageRevision.
	UID() types.UID

	// Key returns the "primary key" of the package.
	Key() PackageRevisionKey

	// Lifecycle returns the current lifecycle state of the package.
	Lifecycle(ctx context.Context) v1alpha1.PackageRevisionLifecycle

	// UpdateLifecycle updates the desired lifecycle of the package. This can only
	// be used for Published package revisions to go from Published to DeletionProposed
	// or vice versa. Draft revisions should use PackageDraft.UpdateLifecycle.
	UpdateLifecycle(ctx context.Context, new v1alpha1.PackageRevisionLifecycle) error

	// GetPackageRevision returns the PackageRevision ("DRY") API representation of this package-revision
	GetPackageRevision(context.Context) (*v1alpha1.PackageRevision, error)

	// GetResources returns the PackageRevisionResources ("WET") API representation of this package-revision
	// TODO: return PackageResources or filesystem abstraction?
	GetResources(context.Context) (*v1alpha1.PackageRevisionResources, error)

	// GetUpstreamLock returns the kpt lock information.
	GetUpstreamLock(context.Context) (kptfile.Upstream, kptfile.UpstreamLock, error)

	// GetKptfile returns the Kptfile for hte package
	GetKptfile(context.Context) (kptfile.KptFile, error)

	// GetLock returns the current revision's lock information.
	// This will be the upstream info for downstream revisions.
	GetLock() (kptfile.Upstream, kptfile.UpstreamLock, error)

	// ResourceVersion returns the Kube resource version of the package
	ResourceVersion() string

	// Create the main package revision
	// TODO: This is a git thing and probably shouldn't be on the generic PackageRevision interface
	ToMainPackageRevision() PackageRevision

	// Get the Kubernetes metadata for the package revision
	GetMeta() metav1.ObjectMeta

	// Set the Kubernetes metadata for the package revision
	SetMeta(metav1.ObjectMeta)
}

// Package is an abstract package.
type Package interface {
	// KubeObjectName returns an encoded name for the object that should be unique.
	// More "readable" values are returned by Key()
	KubeObjectName() string

	// Key returns the "primary key" of the package.
	Key() PackageKey

	// GetPackage returns the object representing this package
	GetPackage() *v1alpha1.PorchPackage

	// GetLatestRevision returns the name of the package revision that is the "latest" package
	// revision belonging to this package
	GetLatestRevision() string
}

type PackageRevisionDraft interface {
	UpdateResources(ctx context.Context, new *v1alpha1.PackageRevisionResources, task *v1alpha1.Task) error
	// Updates desired lifecycle of the package. The lifecycle is applied on Close.
	UpdateLifecycle(ctx context.Context, new v1alpha1.PackageRevisionLifecycle) error
	GetName() string
}

// ListPackageRevisionFilter is a predicate for filtering PackageRevision objects;
// only matching PackageRevision objects will be returned.
type ListPackageRevisionFilter struct {
	// KubeObjectName matches the generated kubernetes object name.
	KubeObjectName string

	// Package matches the name of the package (spec.package)
	Package string

	// WorkspaceName matches the description of the package (spec.workspaceName)
	WorkspaceName v1alpha1.WorkspaceName

	// Revision matches the revision of the package (spec.revision)
	Revision string

	// Lifecycle matches the spec.lifecycle of the package
	Lifecycle v1alpha1.PackageRevisionLifecycle
}

// Matches returns true if the provided PackageRevision satisfies the conditions in the filter.
func (f *ListPackageRevisionFilter) Matches(ctx context.Context, p PackageRevision) bool {
	packageKey := p.Key()

	if f.Package != "" && f.Package != packageKey.Package {
		return false
	}
	if f.Revision != "" && f.Revision != packageKey.Revision {
		return false
	}
	if f.WorkspaceName != "" && f.WorkspaceName != packageKey.WorkspaceName {
		return false
	}
	if f.KubeObjectName != "" && f.KubeObjectName != p.KubeObjectName() {
		return false
	}
	if f.Lifecycle != "" && f.Lifecycle != p.Lifecycle(ctx) {
		return false
	}
	return true
}

// ListPackageFilter is a predicate for filtering Package objects;
// only matching Package objects will be returned.
type ListPackageFilter struct {
	// KubeObjectName matches the generated kubernetes object name.
	KubeObjectName string

	// Package matches the name of the package (spec.package)
	Package string
}

// Matches returns true if the provided Package satisfies the conditions in the filter.
func (f *ListPackageFilter) Matches(p Package) bool {
	if f.Package != "" && f.Package != p.Key().Package {
		return false
	}
	if f.KubeObjectName != "" && f.KubeObjectName != p.KubeObjectName() {
		return false
	}
	return true
}

// Repository is the interface for interacting with packages in repositories
// TODO: we may need interface to manage repositories too. Stay tuned.
type Repository interface {
	// ListPackageRevisions lists the existing package revisions in the repository
	ListPackageRevisions(ctx context.Context, filter ListPackageRevisionFilter) ([]PackageRevision, error)

	// CreatePackageRevision creates a new package revision
	CreatePackageRevision(ctx context.Context, obj *v1alpha1.PackageRevision) (PackageRevisionDraft, error)

	// ClosePackageRevisionDraft closes out a Package Revision Draft
	ClosePackageRevisionDraft(ctx context.Context, prd PackageRevisionDraft, version string) (PackageRevision, error)

	// DeletePackageRevision deletes a package revision
	DeletePackageRevision(ctx context.Context, old PackageRevision) error

	// UpdatePackageRevision updates a package
	UpdatePackageRevision(ctx context.Context, old PackageRevision) (PackageRevisionDraft, error)

	// ListPackages lists all packages in the repository
	ListPackages(ctx context.Context, filter ListPackageFilter) ([]Package, error)

	// CreatePackage creates a new package
	CreatePackage(ctx context.Context, obj *v1alpha1.PorchPackage) (Package, error)

	// DeletePackage deletes a package
	DeletePackage(ctx context.Context, old Package) error

	// Version returns a string that is guaranteed to be different if any change has been made to the repo contents
	Version(ctx context.Context) (string, error)

	// Close cleans up any resources associated with the repository
	Close() error

	// Refresh the repository
	Refresh(ctx context.Context) error
}

// The definitions below would be more appropriately located in a package usable by any Porch component.
// They are located in repository package because repository is one such package though thematically
// they rather belong to a package of their own.

type Credential interface {
	Valid() bool
	ToAuthMethod() transport.AuthMethod
	ToString() string
}

type CredentialResolver interface {
	ResolveCredential(ctx context.Context, namespace, name string) (Credential, error)
}

type UserInfo struct {
	Name  string
	Email string
}

// UserInfoProvider providers name of the authenticated user on whose behalf the request
// is being processed.
type UserInfoProvider interface {
	// GetUserInfo returns the information about the user on whose behalf the request is being
	// processed, if any. If user cannot be determnined, returns nil.
	GetUserInfo(ctx context.Context) *UserInfo
}
