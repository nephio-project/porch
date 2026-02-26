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

// TaskType defines the type of task
type TaskType string

const (
	TaskTypeInit    TaskType = "init"
	TaskTypeClone   TaskType = "clone"
	TaskTypeEdit    TaskType = "edit"
	TaskTypeUpgrade TaskType = "upgrade"
	TaskTypeRender  TaskType = "render"
	TaskTypePush    TaskType = "push"
	TaskTypeNone    TaskType = ""
)

// Task represents an operation performed on a package revision
type Task struct {
	Type    TaskType                `json:"type"`
	Init    *PackageInitTaskSpec    `json:"init,omitempty"`
	Clone   *PackageCloneTaskSpec   `json:"clone,omitempty"`
	Edit    *PackageEditTaskSpec    `json:"edit,omitempty"`
	Upgrade *PackageUpgradeTaskSpec `json:"upgrade,omitempty"`
}

// TaskResult contains the result of executing a task
type TaskResult struct {
	Task         *Task         `json:"task"`
	RenderStatus *RenderStatus `json:"renderStatus,omitempty"`
}

// RenderStatus represents the result of performing render operation on package resources
type RenderStatus struct {
	Result ResultList `json:"result,omitempty"`
	Err    string     `json:"error"`
}

// PackageInitTaskSpec defines the package initialization task
type PackageInitTaskSpec struct {
	// Subpackage is a directory path to a subpackage to initialize. If unspecified, the main package will be initialized.
	Subpackage string `json:"subpackage,omitempty"`
	// Description is a short description of the package.
	Description string `json:"description,omitempty"`
	// Keywords is a list of keywords describing the package.
	Keywords []string `json:"keywords,omitempty"`
	// Site is a link to page with information about the package.
	Site string `json:"site,omitempty"`
}

// PackageCloneTaskSpec defines the package clone task
type PackageCloneTaskSpec struct {
	// Upstream is the reference to the upstream package to clone.
	Upstream UpstreamPackage `json:"upstreamRef,omitempty"`
}

// PackageEditTaskSpec defines the package edit task
type PackageEditTaskSpec struct {
	// Source is the reference to the source PackageRevision to copy from.
	Source *PackageRevisionRef `json:"sourceRef,omitempty"`
}

// PackageUpgradeTaskSpec defines the package upgrade task
type PackageUpgradeTaskSpec struct {
	// OldUpstream is the reference to the original upstream package revision that is
	// the common ancestor of the local package and the new upstream package revision.
	OldUpstream PackageRevisionRef `json:"oldUpstreamRef,omitempty"`

	// NewUpstream is the reference to the new upstream package revision that the
	// local package will be upgraded to.
	NewUpstream PackageRevisionRef `json:"newUpstreamRef,omitempty"`

	// LocalPackageRevisionRef is the reference to the local package revision that
	// contains all the local changes on top of the OldUpstream package revision.
	LocalPackageRevisionRef PackageRevisionRef `json:"localPackageRevisionRef,omitempty"`

	// Strategy defines which strategy should be used to update the package. It defaults to 'resource-merge'.
	//  * resource-merge: Perform a structural comparison of the original /
	//    updated resources, and merge the changes into the local package.
	//  * fast-forward: Fail without updating if the local package was modified
	//    since it was fetched.
	//  * force-delete-replace: Wipe all the local changes to the package and replace
	//    it with the remote version.
	//  * copy-merge: Copy all the remote changes to the local package.
	Strategy PackageMergeStrategy `json:"strategy,omitempty"`
}

// PackageMergeStrategy defines the strategy for merging package changes
type PackageMergeStrategy string

const (
	ResourceMerge      PackageMergeStrategy = "resource-merge"
	FastForward        PackageMergeStrategy = "fast-forward"
	ForceDeleteReplace PackageMergeStrategy = "force-delete-replace"
	CopyMerge          PackageMergeStrategy = "copy-merge"
)
