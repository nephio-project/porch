// Copyright 2022, 2025 The kpt and Nephio Authors
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

package git

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	porchapi "github.com/nephio-project/porch/api/porch/v1alpha1"
	"github.com/nephio-project/porch/pkg/repository"
)

const (
	MainBranch BranchName = "main"

	branchPrefixInLocalRepo  = "refs/remotes/" + OriginName + "/"
	branchPrefixInRemoteRepo = "refs/heads/"
	tagsPrefixInLocalRepo    = "refs/tags/"
	tagsPrefixInRemoteRepo   = "refs/tags/"

	branchRefSpec config.RefSpec = config.RefSpec("+" + branchPrefixInRemoteRepo + "*:" + branchPrefixInLocalRepo + "*")
	tagRefSpec    config.RefSpec = config.RefSpec("+" + tagsPrefixInRemoteRepo + "*:" + tagsPrefixInLocalRepo + "*")

	draftsPrefix             = "drafts/"
	draftsPrefixInLocalRepo  = branchPrefixInLocalRepo + draftsPrefix
	draftsPrefixInRemoteRepo = branchPrefixInRemoteRepo + draftsPrefix

	proposedPrefix             = "proposed/"
	proposedPrefixInLocalRepo  = branchPrefixInLocalRepo + proposedPrefix
	proposedPrefixInRemoteRepo = branchPrefixInRemoteRepo + proposedPrefix

	deletionProposedPrefix             = "deletionProposed/"
	deletionProposedPrefixInLocalRepo  = branchPrefixInLocalRepo + deletionProposedPrefix
	deletionProposedPrefixInRemoteRepo = branchPrefixInRemoteRepo + deletionProposedPrefix
)

var (
	// The default fetch spec contains both branches and tags.
	// This enables push of a tag which will automatically update
	// its local reference, avoiding explicitly setting of refs.
	defaultFetchSpec []config.RefSpec = []config.RefSpec{
		branchRefSpec,
		tagRefSpec,
	}

	// DO NOT USE for fetches. Used for reverse reference mapping only.
	reverseFetchSpec []config.RefSpec = []config.RefSpec{
		config.RefSpec(branchPrefixInLocalRepo + "*:" + branchPrefixInRemoteRepo + "*"),
		config.RefSpec(tagsPrefixInLocalRepo + "*:" + tagsPrefixInRemoteRepo + "*"),
	}
)

// BranchName represents a relative branch name (i.e. 'main', 'drafts/bucket/v1')
// and supports transformation to the ReferenceName in local (cached) repository
// (those references are in the form 'refs/remotes/origin/...') or in the remote
// repository (those references are in the form 'refs/heads/...').
type BranchName string

func (b BranchName) RefInRemote() plumbing.ReferenceName {
	return plumbing.ReferenceName(branchPrefixInRemoteRepo + string(b))
}

func (b BranchName) RefInLocal() plumbing.ReferenceName {
	return plumbing.ReferenceName(branchPrefixInLocalRepo + string(b))
}

func (b BranchName) ForceFetchSpec() config.RefSpec {
	return config.RefSpec(fmt.Sprintf("+%s:%s", b.RefInRemote(), b.RefInLocal()))
}

func isProposedBranchNameInLocal(n plumbing.ReferenceName) bool {
	return strings.HasPrefix(n.String(), proposedPrefixInLocalRepo)
}

func getProposedBranchNameInLocal(n plumbing.ReferenceName) (BranchName, bool) {
	b, ok := trimOptionalPrefix(n.String(), proposedPrefixInLocalRepo)
	return BranchName(b), ok
}

func isDraftBranchNameInLocal(n plumbing.ReferenceName) bool {
	return strings.HasPrefix(n.String(), draftsPrefixInLocalRepo)
}

func getDraftBranchNameInLocal(n plumbing.ReferenceName) (BranchName, bool) {
	b, ok := trimOptionalPrefix(n.String(), draftsPrefixInLocalRepo)
	return BranchName(b), ok
}

func getdeletionProposedBranchNameInLocal(n plumbing.ReferenceName) (BranchName, bool) {
	b, ok := trimOptionalPrefix(n.String(), deletionProposedPrefixInLocalRepo)
	return BranchName(b), ok
}

func isBranchInLocalRepo(n plumbing.ReferenceName) bool {
	return strings.HasPrefix(n.String(), branchPrefixInLocalRepo)
}

func getBranchNameInLocalRepo(n plumbing.ReferenceName) (string, bool) {
	return trimOptionalPrefix(n.String(), branchPrefixInLocalRepo)
}

func isTagInLocalRepo(n plumbing.ReferenceName) bool {
	return strings.HasPrefix(n.String(), tagsPrefixInLocalRepo)
}

func getTagNameInLocalRepo(n plumbing.ReferenceName) (string, bool) {
	return trimOptionalPrefix(n.String(), tagsPrefixInLocalRepo)
}

func createDraftName(key repository.PackageRevisionKey) BranchName {
	return BranchName(draftsPrefix + filepath.Join(key.PkgKey.ToFullPathname(), string(key.WorkspaceName)))
}

func createProposedName(key repository.PackageRevisionKey) BranchName {
	return BranchName(proposedPrefix + filepath.Join(key.PkgKey.ToFullPathname(), string(key.WorkspaceName)))
}

func createDeletionProposedName(key repository.PackageRevisionKey) BranchName {
	if key.Revision > 0 {
		return BranchName(deletionProposedPrefix + filepath.Join(key.PkgKey.ToFullPathname(), "v"+repository.Revision2Str(key.Revision)))
	} else {
		return BranchName(deletionProposedPrefix + filepath.Join(key.PkgKey.ToFullPathname(), "/"+key.WorkspaceName))
	}
}

func trimOptionalPrefix(s, prefix string) (string, bool) {
	if strings.HasPrefix(s, prefix) {
		return strings.TrimPrefix(s, prefix), true
	}
	return "", false
}

func createFinalTagNameInLocal(key repository.PackageRevisionKey) plumbing.ReferenceName {
	if key.Revision > 0 {
		return plumbing.ReferenceName(tagsPrefixInLocalRepo + filepath.Join(key.PkgKey.ToFullPathname(), "v"+repository.Revision2Str(key.Revision)))
	} else {
		return plumbing.ReferenceName(tagsPrefixInLocalRepo + filepath.Join(key.PkgKey.ToFullPathname(), "/"+key.WorkspaceName))
	}
}

func refInLocalFromRefInRemote(n plumbing.ReferenceName) (plumbing.ReferenceName, error) {
	return translateReference(n, defaultFetchSpec)
}

func refInRemoteFromRefInLocal(n plumbing.ReferenceName) (plumbing.ReferenceName, error) {
	return translateReference(n, reverseFetchSpec)
}

func translateReference(n plumbing.ReferenceName, specs []config.RefSpec) (plumbing.ReferenceName, error) {
	for _, spec := range specs {
		if spec.Match(n) {
			return spec.Dst(n), nil
		}
	}
	return "", fmt.Errorf("cannot translate reference %s", n)
}

func getReferenceName(ctx context.Context, pr repository.PackageRevision) string {
	// Handle published PRs, they have hard revisions and are Published or DeletionPropsoed
	if pr.Key().Revision > 0 {
		return fmt.Sprintf("%s%s/v%d", tagsPrefixInLocalRepo, pr.Key().PKey().ToFullPathname(), pr.Key().Revision)
	}

	// Handle draft and proposed PRs
	switch pr.Lifecycle(ctx) {
	case porchapi.PackageRevisionLifecycleDraft:
		return fmt.Sprintf("%s%s/%s", draftsPrefixInLocalRepo, pr.Key().PKey().ToFullPathname(), pr.Key().WorkspaceName)

	case porchapi.PackageRevisionLifecycleProposed:
		return fmt.Sprintf("%s%s/%s", proposedPrefixInLocalRepo, pr.Key().PKey().ToFullPathname(), pr.Key().WorkspaceName)

	}

	// Handle placeholder PRs, they have a workspace name with the same name as the repo branch
	if pr.Key().Revision == -1 {
		return fmt.Sprintf("%s%s", branchPrefixInLocalRepo, pr.Key().RKey().PlaceholderWSname)
	}

	// We can't ge the reference name, jsut return a blank string
	return ""
}
