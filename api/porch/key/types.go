// Copyright 2026 The kpt Authors
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

package key

import (
	"fmt"
	"path/filepath"
	"strings"

	apiutil "github.com/kptdev/porch/api/util"
)

// PackageRevisionKey identifies one revision of a package.
type PackageRevisionKey struct {
	PkgKey PackageKey
	// Revision is the published revision number: 0 matches any revision and -1 is the main (unpublished) revision.
	Revision int
	// WorkspaceName is the draft name or the published version (for example "v1" or "v1.2.3"); empty matches any workspace.
	WorkspaceName string
}

func (k PackageRevisionKey) String() string {
	return fmt.Sprintf("%s:%d:%s", k.PkgKey.String(), k.Revision, k.WorkspaceName)
}

func (k PackageRevisionKey) K8SNS() string {
	return k.RKey().Namespace
}

func (k PackageRevisionKey) K8SName() string {
	return ComposePkgRevObjName(k)
}

// K8SName2PkgRevWSName returns the workspace of a PackageRevision object name.
// k8sName must be the package object name, a dot, and the workspace.
func K8SName2PkgRevWSName(k8sNamePkg, k8sName string) string {
	return k8sName[len(k8sNamePkg)+1:]
}

// PkgRevK8sName2Key parses a PackageRevision object name
// (repo.path.with.dots.package.workspace). A trailing dotted version such as
// v1.2.3 stays one workspace. The revision number is not in the name and stays 0.
func PkgRevK8sName2Key(k8sNamespace, k8sName string) (PackageRevisionKey, error) {
	workspaceName := apiutil.GetPRWorkspaceName(k8sName)

	conditionedK8SName := k8sName
	if strings.Contains(workspaceName, ".") {
		conditionedK8SName = k8sName[:len(k8sName)-len(workspaceName)] + strings.ReplaceAll(workspaceName, ".", "-")
	}

	parsedPRSlice := apiutil.SplitIn3OnDelimiter(conditionedK8SName, ".")
	parsedPkgSlice := apiutil.SplitIn3OnDelimiter(parsedPRSlice[0]+"."+parsedPRSlice[1], ".")

	packagePath := strings.ReplaceAll(parsedPkgSlice[1], ".", "/")
	if err := apiutil.ValidPkgRevObjName(parsedPRSlice[0], packagePath, parsedPkgSlice[2], parsedPRSlice[2]); err != nil {
		return PackageRevisionKey{}, err
	}

	return PackageRevisionKey{
		PkgKey: PackageKey{
			RepoKey: RepositoryKey{
				Namespace: k8sNamespace,
				Name:      parsedPRSlice[0],
			},
			Path:    packagePath,
			Package: parsedPkgSlice[2],
		},
		WorkspaceName: workspaceName,
	}, nil
}

func (k PackageRevisionKey) DeepCopy(outKey *PackageRevisionKey) {
	k.PkgKey.DeepCopy(&outKey.PkgKey)
	outKey.Revision = k.Revision
	outKey.WorkspaceName = k.WorkspaceName
}

func (k PackageRevisionKey) PKey() PackageKey {
	return k.PkgKey
}

func (k PackageRevisionKey) RKey() RepositoryKey {
	return k.PkgKey.RepoKey
}

// Matches reports whether k and other identify the same revision.
// A zero Revision and an empty WorkspaceName on k match any value.
func (k PackageRevisionKey) Matches(other PackageRevisionKey) bool {
	if k.Revision != 0 && k.Revision != other.Revision {
		return false
	}

	if k.WorkspaceName != "" && k.WorkspaceName != other.WorkspaceName {
		return false
	}

	return k.PkgKey.Matches(other.PkgKey)
}

// PackageKey identifies a package inside a repository.
type PackageKey struct {
	RepoKey RepositoryKey
	// Path is the directory under RepositoryKey.Path.
	Path string
	// Package is the leaf name.
	Package string
}

func (k PackageKey) K8SNS() string {
	return k.RepoKey.Namespace
}

func (k PackageKey) K8SName() string {
	return ComposePkgObjName(k)
}

// PkgK8sName2Key parses a Package object name (repo.path.with.dots.package).
// Dots in the path segment become slashes.
func PkgK8sName2Key(k8sNamespace, k8sName string) (PackageKey, error) {
	parsedPkgSlice := apiutil.SplitIn3OnDelimiter(k8sName, ".")

	packagePath := strings.ReplaceAll(parsedPkgSlice[1], ".", "/")
	if err := apiutil.ValidPkgObjName(parsedPkgSlice[0], packagePath, parsedPkgSlice[2]); err != nil {
		return PackageKey{}, err
	}

	return PackageKey{
		RepoKey: RepositoryKey{
			Namespace: k8sNamespace,
			Name:      parsedPkgSlice[0],
		},
		Path:    packagePath,
		Package: parsedPkgSlice[2],
	}, nil
}

func (k PackageKey) String() string {
	return fmt.Sprintf("%s:%s:%s", k.RepoKey.String(), k.Path, k.Package)
}

func (k PackageKey) DeepCopy(outKey *PackageKey) {
	k.RepoKey.DeepCopy(&outKey.RepoKey)
	outKey.Path = k.Path
	outKey.Package = k.Package
}

// ToPkgPathname returns Path/Package, relative to the repository directory.
func (k PackageKey) ToPkgPathname() string {
	return filepath.Join(k.Path, k.Package)
}

// ToFullPathname returns the package directory prefixed with RepositoryKey.Path
// (the git directory, or the OCI registry URL).
func (k PackageKey) ToFullPathname() string {
	return filepath.Join(k.RepoKey.Path, k.Path, k.Package)
}

// K8SName2PkgName returns the segment after the last dot of a Package object name.
func K8SName2PkgName(k8sName string) string {
	lastDotPos := strings.LastIndex(k8sName, ".")

	return k8sName[lastDotPos+1:]
}

// FromFullPathname builds a PackageKey from a package pathname (directory/name).
// repoKey is attached as given; it is not parsed from fullpath.
func FromFullPathname(repoKey RepositoryKey, fullpath string) PackageKey {
	path, name := SplitPackagePathName(fullpath)

	return PackageKey{
		RepoKey: repoKey,
		Path:    path,
		Package: name,
	}
}

// SplitPackagePathName splits a package pathname into the parent directory
// and the leaf package name. Leading and trailing slashes are ignored.
func SplitPackagePathName(fullpath string) (path, name string) {
	pkgPath := strings.Trim(fullpath, "/")
	slashIndex := strings.LastIndex(pkgPath, "/")

	if slashIndex >= 0 {
		return pkgPath[:slashIndex], pkgPath[slashIndex+1:]
	}

	return "", pkgPath
}

func (k PackageKey) RKey() RepositoryKey {
	return k.RepoKey
}

// Matches reports whether k and other identify the same package.
// Empty Path and Package on k match any value.
func (k PackageKey) Matches(other PackageKey) bool {
	if k.Path != "" && k.Path != other.Path {
		return false
	}

	if k.Package != "" && k.Package != other.Package {
		return false
	}

	return k.RepoKey.Matches(other.RepoKey)
}

// RepositoryKey identifies a Porch repository.
type RepositoryKey struct {
	Namespace string
	Name      string
	// Path is the directory inside a git repo, or an oci:// registry URL.
	Path string
	// PlaceholderWSname is the workspace of the repository's main revision (the git branch, or "OCI").
	PlaceholderWSname string
}

func (k RepositoryKey) K8SNS() string {
	return k.Namespace
}

func (k RepositoryKey) K8SName() string {
	return k.Name
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

// Matches reports whether k and other identify the same repository.
// Empty fields on k match any value.
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
