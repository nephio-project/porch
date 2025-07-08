// Copyright 2025 The kpt and Nephio Authors
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

// Package repository is the interface package for repositories.
package repository

import (
	"context"
	"fmt"

	"github.com/nephio-project/porch/api/porch/v1alpha1"
)

const (
	refPrefix       = "refs/"
	refHeadPrefix   = refPrefix + "heads/"
	refTagPrefix    = refPrefix + "tags/"
	refRemotePrefix = refPrefix + "remotes/"
	refNotePrefix   = refPrefix + "notes/"
	symrefPrefix    = "ref: "
)

func GetReferenceName(ctx context.Context, pr PackageRevision) string {
	// Handle published PRs, they have hard revisions and are Published or DeletionPropsoed
	if pr.Key().Revision > 0 {
		return fmt.Sprintf("%s%s/v%d", refTagPrefix, pr.Key().PKey().ToFullPathname(), pr.Key().Revision)
	}

	// Handle draft and proposed PRs
	switch pr.Lifecycle(ctx) {
	case v1alpha1.PackageRevisionLifecycleDraft:
		return fmt.Sprintf("%sorigin/drafts/%s/%s", refRemotePrefix, pr.Key().PKey().ToFullPathname(), pr.Key().WorkspaceName)

	case v1alpha1.PackageRevisionLifecycleProposed:
		return fmt.Sprintf("%sorigin/proposed/%s/%s", refRemotePrefix, pr.Key().PKey().ToFullPathname(), pr.Key().WorkspaceName)

	}

	// Handle placeholder PRs, they have a workspace name with the same name as the repo branch
	if pr.Key().Revision == -1 {
		return fmt.Sprintf("%sorigin/%s", refRemotePrefix, pr.Key().RKey().PlaceholderWSname)
	}

	// We can't ge the reference name, jsut return a blank string
	return ""
}
