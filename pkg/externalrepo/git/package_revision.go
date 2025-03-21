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

package git

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/nephio-project/porch/api/porch/v1alpha1"
	"github.com/nephio-project/porch/pkg/errors"
	kptfilev1 "github.com/nephio-project/porch/pkg/kpt/api/kptfile/v1"
	"github.com/nephio-project/porch/pkg/repository"
	"github.com/nephio-project/porch/pkg/util"
	pkgerrors "github.com/pkg/errors"
	"go.opentelemetry.io/otel/trace"
	"k8s.io/klog/v2"
)


func (r *gitRepository) ListPackageRevisions(ctx context.Context, filter repository.ListPackageRevisionFilter) ([]repository.PackageRevision, error) {
	ctx, span := tracer.Start(ctx, "gitRepository::ListPackageRevisions", trace.WithAttributes())
	defer span.End()

	r.mutex.Lock()
	defer r.mutex.Unlock()

	pkgRevs, err := r.listPackageRevisions(ctx, filter)
	if err != nil {
		return nil, err
	}
	var repoPkgRevs []repository.PackageRevision
	for i := range pkgRevs {
		repoPkgRevs = append(repoPkgRevs, pkgRevs[i])
	}
	return repoPkgRevs, nil
}

func (r *gitRepository) listPackageRevisions(ctx context.Context, filter repository.ListPackageRevisionFilter) ([]*gitPackageRevision, error) {
	ctx, span := tracer.Start(ctx, "gitRepository::listPackageRevisions", trace.WithAttributes())
	defer span.End()

	if err := r.fetchRemoteRepository(ctx); err != nil {
		return nil, err
	}

	refs, err := r.repo.References()
	if err != nil {
		return nil, err
	}

	var main *plumbing.Reference
	var drafts []*gitPackageRevision
	var result []*gitPackageRevision

	mainBranch := r.branch.RefInLocal() // Looking for the registered branch

	for ref, err := refs.Next(); err == nil; ref, err = refs.Next() {
		switch name := ref.Name(); {
		case name == mainBranch:
			main = ref
			continue

		case isProposedBranchNameInLocal(ref.Name()), isDraftBranchNameInLocal(ref.Name()):
			klog.Infof("Loading draft from %q", ref.Name())
			draft, err := r.loadDraft(ctx, ref)
			if err != nil {
				return nil, pkgerrors.Wrapf(err, "failed to load package draft %q", name.String())
			}
			if draft != nil {
				drafts = append(drafts, draft)
			} else {
				klog.Warningf("no package draft found for ref %v", ref)
			}
		case isTagInLocalRepo(ref.Name()):
			klog.Infof("Loading tag from %q", ref.Name())
			tagged, err := r.loadTaggedPackage(ctx, ref)
			if err != nil {
				if tagged == nil {
					// this tag is not associated with any package (e.g. could be a release tag)
					klog.Warningf("Failed to load tagged package from ref %q: %s", ref.Name(), err)
					continue
				}
				klog.Warningf("Error loading tagged package from ref %q: %s", ref.Name(), err)
			}
			if tagged != nil && filter.Matches(ctx, tagged) {
				result = append(result, tagged)
			}
		}
	}

	if main != nil {
		// TODO: ignore packages that are unchanged in main branch, compared to a tagged version?
		mainpkgs, err := r.discoverFinalizedPackages(ctx, main)
		if err != nil {
			if len(mainpkgs) == 0 {
				return nil, err
			}
			klog.Warningf("Error discovering finalized packages: %s", err)
		}
		for _, p := range mainpkgs {
			if filter.Matches(ctx, p) {
				result = append(result, p)
			}
		}
	}

	for _, p := range drafts {
		if filter.Matches(ctx, p) {
			result = append(result, p)
		}
	}

	return result, nil
}

func (r *gitRepository) CreatePackageRevision(ctx context.Context, obj *v1alpha1.PackageRevision) (repository.PackageRevisionDraft, error) {
	_, span := tracer.Start(ctx, "gitRepository::CreatePackageRevision", trace.WithAttributes())
	defer span.End()
	r.mutex.Lock()
	defer r.mutex.Unlock()

	var base plumbing.Hash
	refName := r.branch.RefInLocal()
	switch main, err := r.repo.Reference(refName, true); {
	case err == nil:
		base = main.Hash()
	case err == plumbing.ErrReferenceNotFound:
		// reference not found - empty repository. Package draft has no parent commit
	default:
		return nil, fmt.Errorf("error when resolving target branch for the package: %w", err)
	}

	if err := util.ValidPkgRevObjName(r.name, r.directory, obj.Spec.PackageName, string(obj.Spec.WorkspaceName)); err != nil {
		return nil, fmt.Errorf("failed to create packagerevision: %w", err)
	}

	packagePath := filepath.Join(r.directory, obj.Spec.PackageName)

	// TODO use git branches to leverage uniqueness
	draft := createDraftName(packagePath, obj.Spec.WorkspaceName)

	// TODO: This should also create a new 'Package' resource if one does not already exist

	return &gitPackageRevisionDraft{
		parent:        r,
		path:          packagePath,
		workspaceName: obj.Spec.WorkspaceName,
		lifecycle:     v1alpha1.PackageRevisionLifecycleDraft,
		updated:       time.Now(),
		base:          nil, // Creating a new package
		tasks:         nil, // Creating a new package
		branch:        draft,
		commit:        base,
	}, nil
}

func (r *gitRepository) UpdatePackageRevision(ctx context.Context, old repository.PackageRevision) (repository.PackageRevisionDraft, error) {
	ctx, span := tracer.Start(ctx, "gitRepository::UpdatePackageRevision", trace.WithAttributes())
	defer span.End()
	r.mutex.Lock()
	defer r.mutex.Unlock()

	oldGitPackage, ok := old.(*gitPackageRevision)
	if !ok {
		return nil, fmt.Errorf("cannot update non-git package %T", old)
	}

	ref := oldGitPackage.ref
	if ref == nil {
		return nil, fmt.Errorf("cannot update final package")
	}

	head, err := r.repo.Reference(ref.Name(), true)
	if err != nil {
		return nil, fmt.Errorf("cannot find draft package branch %q: %w", ref.Name(), err)
	}

	rev, err := r.loadDraft(ctx, head)
	if err != nil {
		return nil, fmt.Errorf("cannot load draft package: %w", err)
	}
	if rev == nil {
		return nil, fmt.Errorf("cannot load draft package %q (package not found)", ref.Name())
	}

	// Fetch lifecycle directly from the repository rather than from the gitPackageRevision. This makes
	// sure we don't end up requesting the same lock twice.
	lifecycle := r.getLifecycle(oldGitPackage)

	return &gitPackageRevisionDraft{
		parent:        r,
		path:          oldGitPackage.path,
		revision:      oldGitPackage.revision,
		workspaceName: oldGitPackage.workspaceName,
		lifecycle:     lifecycle,
		updated:       rev.updated,
		base:          rev.ref,
		tree:          rev.tree,
		commit:        rev.commit,
		tasks:         rev.tasks,
	}, nil
}

func (r *gitRepository) DeletePackageRevision(ctx context.Context, old repository.PackageRevision) error {
	ctx, span := tracer.Start(ctx, "gitRepository::DeletePackageRevision", trace.WithAttributes())
	defer span.End()
	r.mutex.Lock()
	defer r.mutex.Unlock()

	oldGit, ok := old.(*gitPackageRevision)
	if !ok {
		return fmt.Errorf("cannot delete non-git package: %T", old)
	}

	ref := oldGit.ref
	if ref == nil {
		// This is an internal error. In some rare cases (see GetPackageRevision below) we create
		// package revisions without refs. They should never be returned via the API though.
		return fmt.Errorf("cannot delete package with no ref: %s", oldGit.path)
	}

	// We can only delete packages which have their own ref. Refs that are shared with other packages
	// (main branch, tag that doesn't contain package path in its name, ...) cannot be deleted.

	refSpecs := newPushRefSpecBuilder()

	switch rn := ref.Name(); {
	case rn.IsTag():
		// Delete tag only if it is package-specific.
		name := createFinalTagNameInLocal(oldGit.path, oldGit.revision)
		if rn != name {
			return fmt.Errorf("cannot delete package tagged with a tag that is not specific to the package: %s", rn)
		}

		// Delete the tag
		refSpecs.AddRefToDelete(ref)

		// If this revision was proposed for deletion, we need to delete the associated branch.
		if err := r.removeDeletionProposedBranchIfExists(ctx, oldGit.path, oldGit.revision); err != nil {
			return err
		}

	case isDraftBranchNameInLocal(rn), isProposedBranchNameInLocal(rn):
		// PackageRevision is proposed or draft; delete the branch directly.
		refSpecs.AddRefToDelete(ref)

	case isBranchInLocalRepo(rn):
		// Delete package from the branch
		commitHash, err := r.createPackageDeleteCommit(ctx, rn, oldGit)
		if err != nil {
			return err
		}

		// Remove the proposed for deletion branch. We end up here when users
		// try to delete the main branch version of a packagerevision.
		if err := r.removeDeletionProposedBranchIfExists(ctx, oldGit.path, oldGit.revision); err != nil {
			return err
		}

		// Update the reference
		refSpecs.AddRefToPush(commitHash, rn)

	default:
		return fmt.Errorf("cannot delete package with the ref name %s", rn)
	}

	// Update references
	if err := r.pushAndCleanup(ctx, refSpecs); err != nil {
		return fmt.Errorf("failed to update git references: %v", err)
	}

	return nil
}

// Creates a commit which deletes the package from the branch, and returns its commit hash.
// If the branch doesn't exist, will return zero hash and no error.
func (r *gitRepository) createPackageDeleteCommit(ctx context.Context, branch plumbing.ReferenceName, pkg *gitPackageRevision) (plumbing.Hash, error) {
	var zero plumbing.Hash

	local, err := refInRemoteFromRefInLocal(branch)
	if err != nil {
		return zero, err
	}
	// Fetch the branch
	// TODO: Fetch only as part of conflict resolution & Retry
	switch err := r.doGitWithAuth(ctx, func(auth transport.AuthMethod) error {
		return r.repo.Fetch(&git.FetchOptions{
			RemoteName: OriginName,
			RefSpecs:   []config.RefSpec{config.RefSpec(fmt.Sprintf("+%s:%s", local, branch))},
			Auth:       auth,
			Tags:       git.NoTags,
			CABundle:   r.caBundle,
		})
	}); err {
	case nil, git.NoErrAlreadyUpToDate:
		// ok
	default:
		return zero, fmt.Errorf("failed to fetch remote repository: %w", err)
	}

	// find the branch
	ref, err := r.repo.Reference(branch, true)
	if err != nil {
		// branch doesn't exist, and therefore package doesn't exist either.
		klog.Infof("Branch %q no longer exist, deleting a package from it is unnecessary", branch)
		return zero, nil
	}
	commit, err := r.repo.CommitObject(ref.Hash())
	if err != nil {
		return zero, fmt.Errorf("failed to resolve main branch to commit: %w", err)
	}
	root, err := commit.Tree()
	if err != nil {
		return zero, fmt.Errorf("failed to find commit tree for %s: %w", ref, err)
	}

	packagePath := pkg.path

	// Find the package in the tree
	switch _, err := root.FindEntry(packagePath); err {
	case object.ErrEntryNotFound:
		// Package doesn't exist; no need to delete it
		return zero, nil
	case nil:
		// found
	default:
		return zero, fmt.Errorf("failed to find package %q in the repositrory ref %q: %w,", packagePath, ref, err)
	}

	// Create commit helper. Use zero hash for the initial package tree. Commit helper will initialize trees
	// without TreeEntry for this package present - the package is deleted.
	ch, err := newCommitHelper(r, r.userInfoProvider, commit.Hash, packagePath, zero)
	if err != nil {
		return zero, fmt.Errorf("failed to initialize commit of package %q to %q: %w", packagePath, ref, err)
	}

	message := fmt.Sprintf("Delete %s", packagePath)
	commitHash, _, err := ch.commit(ctx, message, packagePath)
	if err != nil {
		return zero, fmt.Errorf("failed to commit package %q to %q: %w", packagePath, ref, err)
	}
	return commitHash, nil
}

func (r *gitRepository) removeDeletionProposedBranchIfExists(ctx context.Context, path, revision string) error {
	refSpecsForDeletionProposed := newPushRefSpecBuilder()
	deletionProposedBranch := createDeletionProposedName(path, revision)
	refSpecsForDeletionProposed.AddRefToDelete(plumbing.NewHashReference(deletionProposedBranch.RefInLocal(), plumbing.ZeroHash))
	if err := r.pushAndCleanup(ctx, refSpecsForDeletionProposed); err != nil {
		if pkgerrors.Is(err, git.NoErrAlreadyUpToDate) {
			// the deletionProposed branch might not have existed, so we ignore this error
			klog.Warningf("branch %s does not exist", deletionProposedBranch)
		} else {
			klog.Errorf("unexpected error while removing deletionProposed branch: %v", err)
			return err
		}
	}
	delete(r.deletionProposedCache, deletionProposedBranch)

	return nil
}

func (r *gitRepository) GetPackageRevision(ctx context.Context, version, path string) (repository.PackageRevision, kptfilev1.GitLock, error) {
	ctx, span := tracer.Start(ctx, "gitRepository::GetPackageRevision", trace.WithAttributes())
	defer span.End()
	r.mutex.Lock()
	defer r.mutex.Unlock()

	var hash plumbing.Hash

	// Trim leading and trailing slashes
	path = strings.Trim(path, "/")

	// Versions map to gitRepo tags in one of two ways:
	//
	// * directly (tag=version)- but then this means that all packages in the repo must be versioned together.
	// * prefixed (tag=<packageDir/<version>) - solving the co-versioning problem.
	//
	// We have to check both forms when looking up a version.
	refNames := []string{}
	if path != "" {
		refNames = append(refNames, path+"/"+version)
		// HACK: Is this always refs/remotes/origin ?  Is it ever not (i.e. do we need both forms?)
		refNames = append(refNames, "refs/remotes/origin/"+path+"/"+version)
	}
	refNames = append(refNames, version)
	// HACK: Is this always refs/remotes/origin ?  Is it ever not (i.e. do we need both forms?)
	refNames = append(refNames, "refs/remotes/origin/"+version)

	for _, ref := range refNames {
		if resolved, err := r.repo.ResolveRevision(plumbing.Revision(ref)); err != nil {
			if pkgerrors.Is(err, plumbing.ErrReferenceNotFound) {
				continue
			}
			return nil, kptfilev1.GitLock{}, pkgerrors.Wrapf(err, "error resolving git reference %q", ref)
		} else {
			hash = *resolved
			break
		}
	}

	if hash.IsZero() {
		r.dumpAllRefs()
		r.dumpAllBranches()

		return nil, kptfilev1.GitLock{}, pkgerrors.Errorf("cannot find git reference (tried %v)", refNames)
	}

	return r.loadPackageRevision(ctx, version, path, hash)
}

func (r *gitRepository) loadPackageRevision(ctx context.Context, version, path string, hash plumbing.Hash) (repository.PackageRevision, kptfilev1.GitLock, error) {
	ctx, span := tracer.Start(ctx, "gitRepository::loadPackageRevision", trace.WithAttributes())
	defer span.End()

	if !packageInDirectory(path, r.directory) {
		return nil, kptfilev1.GitLock{}, pkgerrors.Errorf("cannot find package %s@%s; package is not under the Repository.spec.directory", path, version)
	}

	origin, err := r.repo.Remote("origin")
	if err != nil {
		return nil, kptfilev1.GitLock{}, pkgerrors.Wrap(err, "cannot determine repository origin")
	}

	lock := kptfilev1.GitLock{
		Repo:      origin.Config().URLs[0],
		Directory: path,
		Ref:       version,
	}

	commit, err := r.repo.CommitObject(hash)
	if err != nil {
		return nil, lock, pkgerrors.Wrapf(err, "cannot resolve git reference %s (hash: %s) to commit", version, hash)
	}
	lock.Commit = commit.Hash.String()

	krmPackage, err := r.findPackage(commit, path)
	if err != nil {
		return nil, lock, err
	}

	if krmPackage == nil {
		return nil, lock, pkgerrors.Errorf("cannot find package %s@%s", path, version)
	}

	var ref *plumbing.Reference = nil // Cannot determine ref; this package will be considered final (immutable).

	var revision string
	var workspace string
	last := strings.LastIndex(version, "/")

	if strings.HasPrefix(version, "drafts/") || strings.HasPrefix(version, "proposed/") {
		// the passed in version is a ref to an unpublished package revision
		workspace = version[last+1:]
	} else {
		// the passed in version is a ref to a published package revision
		if version == string(r.branch) || last < 0 {
			revision = version
		} else {
			revision = version[last+1:]
		}
		workspace = getPkgWorkspace(commit, krmPackage, ref)
		if workspace == "" {
			workspace = revision
		}
	}

	packageRevision, err := krmPackage.buildGitPackageRevision(ctx, revision, workspace, ref)
	if err != nil {
		return nil, lock, err
	}
	return packageRevision, lock, nil
}

func (r *gitRepository) loadTaggedPackage(ctx context.Context, tag *plumbing.Reference) (*gitPackageRevision, error) {
	ctx, span := tracer.Start(ctx, "gitRepository::loadTaggedPackage", trace.WithAttributes())
	defer span.End()

	name, ok := getTagNameInLocalRepo(tag.Name())
	if !ok {
		return nil, pkgerrors.Errorf("invalid tag ref: %q", tag)
	}
	slash := strings.LastIndex(name, "/")

	if slash < 0 {
		// tag=<version>
		// could be a release tag or something else, we ignore these types of tags
		return nil, pkgerrors.Errorf("could not find slash in %q", name)

	}

	// tag=<package path>/version
	path, revision := name[:slash], name[slash+1:]

	if !packageInDirectory(path, r.directory) {
		return nil, nil
	}

	resolvedHash, err := r.repo.ResolveRevision(plumbing.Revision(tag.Hash().String()))
	if err != nil {
		return nil, pkgerrors.Wrapf(err, "cannot resolve tag %q to git revision", name)
	}
	commit, err := r.repo.CommitObject(*resolvedHash)
	if err != nil {
		return nil, pkgerrors.Wrapf(err, "cannot resolve tag %q (hash: %q) to commit (corrupted repository?)", name, resolvedHash)
	}

	krmPackage, err := r.findPackage(commit, path)
	if err != nil {
		return nil, pkgerrors.Wrapf(err, "skipping %q; cannot find %q (corrupted repository?)", name, path)
	}

	if krmPackage == nil {
		return nil, pkgerrors.Errorf("skipping %q: Kptfile not found", name)
	}

	workspaceName := getPkgWorkspace(commit, krmPackage, tag)
	if workspaceName == "" {
		klog.Warningf("Failed to get package workspace name for package %q (will use revision name)", name)
	}

	packageRevision, err := krmPackage.buildGitPackageRevision(ctx, revision, workspaceName, tag)
	if err != nil {
		if packageRevision == nil {
			return nil, err
		}
		klog.Warningf("Error building git package revision %q: %s", path, err)
	}

	return packageRevision, nil

}

func (r *gitRepository) dumpAllRefs() {
	refs, err := r.repo.References()
	if err != nil {
		klog.Warningf("failed to get references: %v", err)
	} else {
		for {
			ref, err := refs.Next()
			if err != nil {
				if err != io.EOF {
					klog.Warningf("failed to get next reference: %v", err)
				}
				break
			}
			klog.Infof("ref %#v", ref.Name())
		}
	}
}

func (r *gitRepository) dumpAllBranches() {
	branches, err := r.repo.Branches()
	if err != nil {
		klog.Warningf("failed to get branches: %v", err)
	} else {
		for {
			branch, err := branches.Next()
			if err != nil {
				if err != io.EOF {
					klog.Warningf("failed to get next branch: %v", err)
				}
				break
			}
			klog.Infof("branch %#v", branch.Name())
		}
	}
}

func (r *gitRepository) discoverFinalizedPackages(ctx context.Context, ref *plumbing.Reference) ([]*gitPackageRevision, error) {
	ctx, span := tracer.Start(ctx, "gitRepository::discoverFinalizedPackages", trace.WithAttributes())
	defer span.End()

	commit, err := r.repo.CommitObject(ref.Hash())
	if err != nil {
		return nil, err
	}

	var revision string
	if rev, ok := getBranchNameInLocalRepo(ref.Name()); ok {
		revision = rev
	} else if rev, ok = getTagNameInLocalRepo(ref.Name()); ok {
		revision = rev
	} else {
		// TODO: ignore the ref instead?
		return nil, pkgerrors.Errorf("cannot determine revision from ref: %q", rev)
	}

	krmPackages, err := r.discoverPackagesInTree(commit, DiscoverPackagesOptions{FilterPrefix: r.directory, Recurse: true})
	if err != nil {
		return nil, err
	}

	ec := errors.NewErrorCollector().WithSeparator(";").WithFormat("{%s}")
	var result []*gitPackageRevision
	for _, krmPackage := range krmPackages.packages {
		workspace := getPkgWorkspace(commit, krmPackage, ref)
		if workspace == "" {
			klog.Warningf("Failed to get workspace name for package %q (will use revision name): %s", krmPackage.path, err)
		}
		packageRevision, err := krmPackage.buildGitPackageRevision(ctx, revision, workspace, ref)
		if err != nil {
			ec.Add(pkgerrors.Wrapf(err, "failed to build git package revision for package %s", krmPackage.path))
			continue
		}
		result = append(result, packageRevision)
	}

	return result, ec.Join()
}