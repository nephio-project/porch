// Copyright 2022, 2024-2025 The kpt and Nephio Authors
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
	"path/filepath"
	"slices"
	"strings"

	"github.com/GoogleContainerTools/kpt-functions-sdk/go/fn"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/nephio-project/porch/api/porch/v1alpha1"
	api "github.com/nephio-project/porch/api/porch/v1alpha1"
	kptfile "github.com/nephio-project/porch/pkg/kpt/api/kptfile/v1"
	"github.com/nephio-project/porch/pkg/util"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// TODO: 	"sigs.k8s.io/kustomize/kyaml/filesys" FileSystem?
type PackageResources struct {
	Contents map[string]string
}

func (pr *PackageResources) SetPrStatusCondition(condition api.Condition) {
	pr.EditKptfile(func(parsedKptfile *kptfile.KptFile) {
		kptfileCondition := kptfile.ConvertApiCondition(condition)
		if parsedKptfile.Status == nil {
			parsedKptfile.Status = &kptfile.Status{}
		}

		if index := slices.IndexFunc(parsedKptfile.Status.Conditions, func(aCondition kptfile.Condition) bool {
			return aCondition.Type == kptfileCondition.Type
		}); index == -1 {
			// Conditions in Kptfile don't already include the desired Condition -
			// check if we have a pipeline and if so, add the Condition
			parsedKptfile.Status.Conditions = append(parsedKptfile.Status.Conditions, kptfileCondition)
		} else {
			parsedKptfile.Status.Conditions[index] = kptfileCondition
		}
	})
}

func (pr *PackageResources) EditKptfile(editFunc func(*kptfile.KptFile)) {
	parsedKptfile := pr.GetKptfile()

	editFunc(parsedKptfile)

	pr.Contents[kptfile.KptFileName] = func() string {
		yamlKptfile, _ := parsedKptfile.ToYamlString()
		return yamlKptfile
	}()
}

func (pr *PackageResources) GetKptfile() *kptfile.KptFile {
	parsedKptfile, _ :=
		kptfile.FromKubeObject(
			func() *fn.KubeObject {
				kubeObject, _ := util.YamlToKubeObject(
					pr.Contents[kptfile.KptFileName])
				return kubeObject
			}())
	return &parsedKptfile
}

type PackageRevisionKey struct {
	PkgKey        PackageKey
	Revision      int
	WorkspaceName string
}

func (k PackageRevisionKey) String() string {
	return fmt.Sprintf("%s:%d:%s", k.PkgKey.String(), k.Revision, string(k.WorkspaceName))
}

func (k PackageRevisionKey) DeepCopy(outKey *PackageRevisionKey) {
	k.PkgKey.DeepCopy(&outKey.PkgKey)
	outKey.Revision = k.Revision
	outKey.WorkspaceName = k.WorkspaceName
}

func (k PackageRevisionKey) GetPackageKey() PackageKey {
	return k.PkgKey
}

func (k PackageRevisionKey) GetRepositoryKey() RepositoryKey {
	return k.PkgKey.RepoKey
}

func (k PackageRevisionKey) Matches(other PackageRevisionKey) bool {
	if k.Revision != 0 && k.Revision != other.Revision {
		return false
	}

	if k.WorkspaceName != "" && k.WorkspaceName != other.WorkspaceName {
		return false
	}

	return k.PkgKey.Matches(other.PkgKey)
}

type PackageKey struct {
	RepoKey       RepositoryKey
	Path, Package string
}

func (k PackageKey) String() string {
	return fmt.Sprintf("%s:%s:%s", k.RepoKey.String(), k.Path, k.Package)
}

func (k PackageKey) DeepCopy(outKey *PackageKey) {
	k.RepoKey.DeepCopy(&outKey.RepoKey)
	outKey.Path = k.Path
	outKey.Package = k.Package
}

func (k PackageKey) ToPkgPathname() string {
	return filepath.Join(k.Path, k.Package)
}

func (k PackageKey) ToFullPathname() string {
	return filepath.Join(k.RepoKey.Path, k.Path, k.Package)
}

func FromFullPathname(repoKey RepositoryKey, fullpath string) PackageKey {
	pkgPath := strings.Trim(strings.TrimPrefix(fullpath, repoKey.Path), "/")
	slashIndex := strings.LastIndex(pkgPath, "/")

	if slashIndex >= 0 {
		return PackageKey{
			RepoKey: repoKey,
			Path:    pkgPath[:slashIndex],
			Package: pkgPath[slashIndex+1:],
		}
	} else {
		return PackageKey{
			RepoKey: repoKey,
			Package: pkgPath,
		}
	}
}

func (k PackageKey) GetRepositoryKey() RepositoryKey {
	return k.RepoKey
}

func (k PackageKey) Matches(other PackageKey) bool {
	if k.Path != "" && k.Path != other.Path {
		return false
	}

	if k.Package != "" && k.Package != other.Package {
		return false
	}

	return k.RepoKey.Matches(other.RepoKey)
}

type RepositoryKey struct {
	Namespace, Name, Path, PlaceholderWSname string
}

func (k RepositoryKey) String() string {
	return fmt.Sprintf("%s:%s:%s:%s", k.Namespace, k.Name, k.Path, string(k.PlaceholderWSname))
}

func (k RepositoryKey) DeepCopy(outKey *RepositoryKey) {
	outKey.Name = k.Name
	outKey.Namespace = k.Namespace
	outKey.Path = k.Path
	outKey.PlaceholderWSname = k.PlaceholderWSname
}

func (k RepositoryKey) Matches(other RepositoryKey) bool {
	if k.Namespace != "" && k.Namespace != other.Namespace {
		return false
	}
	if k.Name != "" && k.Name != other.Name {
		return false
	}

	if k.Path != "" && k.Path != other.Path {
		return false
	}

	if k.PlaceholderWSname != "" && k.PlaceholderWSname != other.PlaceholderWSname {
		return false
	}

	return true
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

	Key() PackageRevisionKey

	// Set the repository of this package revision
	SetRepository(repository Repository)

	// Lifecycle returns the current lifecycle state of the package.
	Lifecycle(ctx context.Context) v1alpha1.PackageRevisionLifecycle

	// UpdateLifecycle updates the desired lifecycle of the package. This can only
	// be used for Published package revisions to go from Published to DeletionProposed
	// or vice versa. Draft revisions should use PackageDraft.UpdateLifecycle.
	UpdateLifecycle(ctx context.Context, lifecycle v1alpha1.PackageRevisionLifecycle) error

	// GetPackageRevision returns the PackageRevision ("DRY") API representation of this package-revision
	GetPackageRevision(ctx context.Context) (*v1alpha1.PackageRevision, error)

	// GetResources returns the PackageRevisionResources ("WET") API representation of this package-revision
	// TODO: return PackageResources or filesystem abstraction?
	GetResources(ctx context.Context) (*v1alpha1.PackageRevisionResources, error)

	// GetUpstreamLock returns the kpt lock information.
	GetUpstreamLock(ctx context.Context) (kptfile.Upstream, kptfile.UpstreamLock, error)

	// GetKptfile returns the Kptfile for the package
	GetKptfile(ctx context.Context) (kptfile.KptFile, error)

	// GetLock returns the current revision's lock information.
	// This will be the upstream info for downstream revisions.
	GetLock() (kptfile.Upstream, kptfile.UpstreamLock, error)

	// ResourceVersion returns the Kube resource version of the package
	ResourceVersion() string

	// Create the main package revision
	// TODO: This is a git thing and probably shouldn't be on the generic PackageRevision interface
	ToMainPackageRevision(ctx context.Context) PackageRevision

	// Get the Kubernetes metadata for the package revision
	GetMeta() metav1.ObjectMeta

	// Set the Kubernetes metadata for the package revision
	SetMeta(ctx context.Context, meta metav1.ObjectMeta) error
}

// Package is an abstract package.
type Package interface {
	// KubeObjectName returns an encoded name for the object that should be unique.
	// More "readable" values are returned by Key()
	KubeObjectName() string

	Key() PackageKey

	// GetPackage returns the object representing this package
	GetPackage(ctx context.Context) *v1alpha1.PorchPackage

	// GetLatestRevision returns the name of the package revision that is the "latest" package
	// revision belonging to this package
	GetLatestRevision(ctx context.Context) int
}

type PackageRevisionDraft interface {
	Key() PackageRevisionKey
	GetMeta() metav1.ObjectMeta
	UpdateResources(context.Context, *v1alpha1.PackageRevisionResources, *v1alpha1.Task) error
	// Updates desired lifecycle of the package. The lifecycle is applied on Close.
	UpdateLifecycle(context.Context, v1alpha1.PackageRevisionLifecycle) error
}

// ListPackageRevisionFilter is a predicate for filtering PackageRevision objects;
// only matching PackageRevision objects will be returned.
type ListPackageRevisionFilter struct {
	Key PackageRevisionKey

	// KubeObjectName matches the generated kubernetes object name.
	KubeObjectName string

	// Lifecycle matches the spec.lifecycle of the package
	Lifecycle v1alpha1.PackageRevisionLifecycle
}

// Matches returns true if the provided PackageRevision satisfies the conditions in the filter.
func (f *ListPackageRevisionFilter) Matches(ctx context.Context, p PackageRevision) bool {
	if !f.Key.Matches(p.Key()) {
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
	CreatePackageRevisionDraft(ctx context.Context, obj *v1alpha1.PackageRevision) (PackageRevisionDraft, error)

	// ClosePackageRevisionDraft closes out a Package Revision Draft
	ClosePackageRevisionDraft(ctx context.Context, prd PackageRevisionDraft, version int) (PackageRevision, error)

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
	Close(ctx context.Context) error

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
