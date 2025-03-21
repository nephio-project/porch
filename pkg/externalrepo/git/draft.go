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
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/nephio-project/porch/api/porch/v1alpha1"
	"github.com/nephio-project/porch/pkg/repository"
	pkgerrors "github.com/pkg/errors"
	"go.opentelemetry.io/otel/trace"
	"k8s.io/klog/v2"
)

type gitPackageRevisionDraft struct {
	prKey   repository.PackageRevisionKey
	repo    *gitRepository // repo is repo containing the package
	updated time.Time
	tasks   []v1alpha1.Task

	// New value of the package revision lifecycle
	lifecycle v1alpha1.PackageRevisionLifecycle

	// ref to the base of the package update commit chain (used for conditional push)
	base *plumbing.Reference

	// name of the branch where the changes will be pushed
	branch BranchName

	// Current HEAD of the package changes (commit sha)
	commit plumbing.Hash

	// Cached tree of the package itself, some descendent of commit.Tree()
	tree plumbing.Hash
}

var _ repository.PackageRevisionDraft = &gitPackageRevisionDraft{}

func (d *gitPackageRevisionDraft) Key() repository.PackageRevisionKey {
	return d.prKey
}

func (d *gitPackageRevisionDraft) UpdateResources(ctx context.Context, new *v1alpha1.PackageRevisionResources, change *v1alpha1.Task) error {
	ctx, span := tracer.Start(ctx, "gitPackageDraft::UpdateResources", trace.WithAttributes())
	defer span.End()

	return d.parent.updateDraftResources(ctx, d, new, change)
}

func (d *gitPackageRevisionDraft) UpdateLifecycle(ctx context.Context, new v1alpha1.PackageRevisionLifecycle) error {
	d.lifecycle = new
	return nil
}

func (d *gitPackageRevisionDraft) GetName() string {
	packageDirectory := d.parent.directory
	packageName := strings.TrimPrefix(d.path, packageDirectory+"/")
	return packageName
}

func (r *gitRepository) updateDraftResources(ctx context.Context, draft *gitPackageRevisionDraft, new *v1alpha1.PackageRevisionResources, change *v1alpha1.Task) error {
	ctx, span := tracer.Start(ctx, "gitRepository::UpdateResources", trace.WithAttributes())
	defer span.End()
	r.mutex.Lock()
	defer r.mutex.Unlock()

	ch, err := newCommitHelper(r, r.userInfoProvider, draft.commit, draft.path, plumbing.ZeroHash)
	if err != nil {
		return pkgerrors.Wrap(err, "failed to commit package:")
	}

	for k, v := range new.Spec.Resources {
		if err := ch.storeFile(filepath.Join(draft.path, k), v); err != nil {
			return err
		}
	}

	// Because we can't read the package back without a Kptfile, make sure one is present
	{
		p := filepath.Join(draft.path, "Kptfile")
		_, err := ch.readFile(p)
		if os.IsNotExist(err) {
			// We could write the file here; currently we return an error
			return pkgerrors.Wrap(err, "package must contain Kptfile at root")
		}
	}

	annotation := &gitAnnotation{
		PackagePath:   draft.path,
		WorkspaceName: draft.workspaceName,
		Revision:      draft.revision,
		Task:          change,
	}
	message := "Intermediate commit"
	if change != nil {
		message += fmt.Sprintf(": %s", change.Type)
		draft.tasks = append(draft.tasks, *change)
	}
	message += "\n"

	message, err = AnnotateCommitMessage(message, annotation)
	if err != nil {
		return err
	}

	commitHash, packageTree, err := ch.commit(ctx, message, draft.path)
	if err != nil {
		return pkgerrors.Wrap(err, "failed to commit package: %w")
	}

	draft.tree = packageTree
	draft.commit = commitHash
	return nil
}

// loadDraft will load the draft package.  If the package isn't found (we now require a Kptfile), it will return (nil, nil)
func (r *gitRepository) loadDraft(ctx context.Context, ref *plumbing.Reference) (*gitPackageRevision, error) {
	ctx, span := tracer.Start(ctx, "gitRepository::loadDraft", trace.WithAttributes())
	defer span.End()

	name, workspaceName, err := parseDraftName(ref)
	if err != nil {
		return nil, err
	}

	// Only load drafts in the directory specified at repository registration.
	if !packageInDirectory(name, r.directory) {
		return nil, nil
	}

	commit, err := r.repo.CommitObject(ref.Hash())
	if err != nil {
		return nil, pkgerrors.Wrap(err, "cannot resolve draft branch to commit (corrupted repository?)")
	}

	krmPackage, err := r.findPackage(commit, name)
	if err != nil {
		return nil, err
	}

	if krmPackage == nil {
		klog.Warningf("draft package %q was not found", name)
		return nil, nil
	}

	packageRevision, err := krmPackage.buildGitPackageRevision(ctx, "", workspaceName, ref)
	if err != nil {
		return nil, err
	}

	return packageRevision, nil
}

func parseDraftName(draft *plumbing.Reference) (name, workspaceName string, err error) {
	refName := draft.Name()
	var suffix string
	if b, ok := getDraftBranchNameInLocal(refName); ok {
		suffix = string(b)
	} else if b, ok = getProposedBranchNameInLocal(refName); ok {
		suffix = string(b)
	} else {
		return "", "", fmt.Errorf("invalid draft ref name: %q", refName)
	}

	revIndex := strings.LastIndex(suffix, "/")
	if revIndex <= 0 {
		return "", "", fmt.Errorf("invalid draft ref name; missing workspaceName suffix: %q", refName)
	}
	name, workspaceName = suffix[:revIndex], suffix[revIndex+1:]
	return name, workspaceName, nil
}