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

package git

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/nephio-project/porch/v4/api/porch/v1alpha1"
	"github.com/nephio-project/porch/v4/internal/kpt/pkg"
	kptfile "github.com/nephio-project/porch/v4/pkg/kpt/api/kptfile/v1"
	"github.com/nephio-project/porch/v4/pkg/repository"
	"github.com/nephio-project/porch/v4/pkg/util"
	"go.opentelemetry.io/otel/trace"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

type gitPackageRevision struct {
	prKey     repository.PackageRevisionKey
	repo      *gitRepository // repo is repo containing the package
	updated   time.Time
	updatedBy string
	ref       *plumbing.Reference // ref is the Git reference at which the package exists
	tree      plumbing.Hash       // Cached tree of the package itself, some descendent of commit.Tree()
	commit    plumbing.Hash       // Current version of the package (commit sha)
	tasks     []v1alpha1.Task
	metadata  metav1.ObjectMeta
}

var _ repository.PackageRevision = &gitPackageRevision{}

func (c *gitPackageRevision) KubeObjectName() string {
	return repository.ComposePkgRevObjName(c.Key())
}

func (c *gitPackageRevision) KubeObjectNamespace() string {
	return c.Key().PkgKey.RepoKey.Namespace
}

func (c *gitPackageRevision) UID() types.UID {
	return util.GenerateUid("packagerevision:", c.KubeObjectNamespace(), c.KubeObjectName())
}

func (p *gitPackageRevision) ResourceVersion() string {
	return p.commit.String()
}

func (p *gitPackageRevision) Key() repository.PackageRevisionKey {
	return p.prKey
}

func (p *gitPackageRevision) SetRepository(repo repository.Repository) {
	p.repo = repo.(*gitRepository)
}

func (p *gitPackageRevision) GetPackageRevision(ctx context.Context) (*v1alpha1.PackageRevision, error) {
	ctx, span := tracer.Start(ctx, "gitPackageRevision::GetPackageRevision", trace.WithAttributes())
	defer span.End()

	key := p.Key()

	_, lock, _ := p.GetUpstreamLock(ctx)
	lockCopy := &v1alpha1.UpstreamLock{}

	// TODO: Use kpt definition of UpstreamLock in the package revision status
	// when https://github.com/GoogleContainerTools/kpt/issues/3297 is complete.
	// Until then, we have to translate from one type to another.
	if lock.Git != nil {
		lockCopy = &v1alpha1.UpstreamLock{
			Type: v1alpha1.OriginType(lock.Type),
			Git: &v1alpha1.GitLock{
				Repo:      lock.Git.Repo,
				Directory: lock.Git.Directory,
				Commit:    lock.Git.Commit,
				Ref:       lock.Git.Ref,
			},
		}
	}

	kf, _ := p.GetKptfile(ctx)

	status := v1alpha1.PackageRevisionStatus{
		UpstreamLock: lockCopy,
		Deployment:   p.repo.deployment,
		Conditions:   repository.ToApiConditions(kf),
	}

	if v1alpha1.LifecycleIsPublished(p.Lifecycle(ctx)) {
		if !p.updated.IsZero() {
			status.PublishedAt = metav1.Time{Time: p.updated}
		}
		if p.updatedBy != "" {
			status.PublishedBy = p.updatedBy
		}
	}

	return &v1alpha1.PackageRevision{
		TypeMeta: metav1.TypeMeta{
			Kind:       "PackageRevision",
			APIVersion: v1alpha1.SchemeGroupVersion.Identifier(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:            p.KubeObjectName(),
			Namespace:       p.KubeObjectNamespace(),
			UID:             p.UID(),
			ResourceVersion: p.commit.String(),
			CreationTimestamp: metav1.Time{
				Time: p.metadata.CreationTimestamp.Time,
			},
		},
		Spec: v1alpha1.PackageRevisionSpec{
			PackageName:    key.PkgKey.ToPkgPathname(),
			RepositoryName: key.PkgKey.RepoKey.Name,
			Lifecycle:      p.Lifecycle(ctx),
			Tasks:          p.tasks,
			ReadinessGates: repository.ToApiReadinessGates(kf),
			WorkspaceName:  key.WorkspaceName,
			Revision:       key.Revision,
		},
		Status: status,
	}, nil
}

func (p *gitPackageRevision) GetResources(ctx context.Context) (*v1alpha1.PackageRevisionResources, error) {
	resources, err := p.repo.GetResources(p.tree)
	if err != nil {
		return nil, fmt.Errorf("failed to load package resources: %w", err)
	}

	return &v1alpha1.PackageRevisionResources{
		TypeMeta: metav1.TypeMeta{
			Kind:       "PackageRevisionResources",
			APIVersion: v1alpha1.SchemeGroupVersion.Identifier(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:            p.KubeObjectName(),
			Namespace:       p.KubeObjectNamespace(),
			UID:             p.UID(),
			ResourceVersion: p.commit.String(),
			CreationTimestamp: metav1.Time{
				Time: p.metadata.CreationTimestamp.Time,
			},
			OwnerReferences: []metav1.OwnerReference{}, // TODO: should point to repository resource
		},
		Spec: v1alpha1.PackageRevisionResourcesSpec{
			PackageName:    p.Key().PkgKey.ToPkgPathname(),
			WorkspaceName:  p.Key().WorkspaceName,
			Revision:       p.Key().Revision,
			RepositoryName: p.Key().PkgKey.RepoKey.Name,

			Resources: resources,
		},
	}, nil
}

// Creates a gitPackageRevision reference that is acting as the main branch package revision.
// It doesn't do any git operations, so this package should only be used if the actual git updates
// were exectued on the main branch.
func (p *gitPackageRevision) ToMainPackageRevision(context.Context) repository.PackageRevision {
	//Need to compute a separate reference, otherwise the ref will be the same as the versioned package,
	//while the main gitPackageRevision needs to point at the main branch.

	mainBranchRef := plumbing.NewHashReference(p.repo.branch.RefInLocal(), p.commit)
	mainPr := &gitPackageRevision{
		repo:      p.repo,
		prKey:     p.prKey,
		updated:   p.updated,
		updatedBy: p.updatedBy,
		ref:       mainBranchRef,
		tree:      p.tree,
		commit:    p.commit,
		tasks:     p.tasks,
	}
	mainPr.prKey.Revision = -1
	mainPr.prKey.WorkspaceName = mainPr.Key().GetPackageKey().GetRepositoryKey().PlaceholderWSname

	return mainPr
}

func (p *gitPackageRevision) GetKptfile(ctx context.Context) (kptfile.KptFile, error) {
	resources, err := p.repo.GetResources(p.tree)
	if err != nil {
		return kptfile.KptFile{}, fmt.Errorf("error loading package resources: %w", err)
	}
	kfString, found := resources[kptfile.KptFileName]
	if !found {
		return kptfile.KptFile{}, fmt.Errorf("packagerevision does not have a Kptfile")
	}
	kf, err := pkg.DecodeKptfile(strings.NewReader(kfString))
	if err != nil {
		return kptfile.KptFile{}, fmt.Errorf("error decoding Kptfile: %w", err)
	}
	return *kf, nil
}

// GetUpstreamLock returns the upstreamLock info present in the Kptfile of the package.
func (p *gitPackageRevision) GetUpstreamLock(ctx context.Context) (kptfile.Upstream, kptfile.UpstreamLock, error) {
	kf, err := p.GetKptfile(ctx)
	if err != nil {
		return kptfile.Upstream{}, kptfile.UpstreamLock{}, fmt.Errorf("cannot determine package lock; cannot retrieve resources: %w", err)
	}

	if kf.Upstream == nil || kf.UpstreamLock == nil || kf.Upstream.Git == nil {
		// the package doesn't have any upstream package.
		return kptfile.Upstream{}, kptfile.UpstreamLock{}, nil
	}
	return *kf.Upstream, *kf.UpstreamLock, nil
}

// GetLock returns the self version of the package. Think of it as the Git commit information
// that represent the package revision of this package. Please note that it uses Upstream types
// to represent this information but it has no connection with the associated upstream package (if any).
func (p *gitPackageRevision) GetLock() (kptfile.Upstream, kptfile.UpstreamLock, error) {
	repo, err := p.repo.GetRepo()
	if err != nil {
		return kptfile.Upstream{}, kptfile.UpstreamLock{}, fmt.Errorf("cannot determine package lock: %w", err)
	}

	if p.ref == nil {
		return kptfile.Upstream{}, kptfile.UpstreamLock{}, fmt.Errorf("cannot determine package lock; package has no ref")
	}

	ref, err := refInRemoteFromRefInLocal(p.ref.Name())
	if err != nil {
		return kptfile.Upstream{}, kptfile.UpstreamLock{}, fmt.Errorf("cannot determine package lock for %q: %v", p.ref, err)
	}

	return kptfile.Upstream{
			Type: kptfile.GitOrigin,
			Git: &kptfile.Git{
				Repo:      repo,
				Directory: p.prKey.PkgKey.ToPkgPathname(),
				Ref:       ref.Short(),
			},
		}, kptfile.UpstreamLock{
			Type: kptfile.GitOrigin,
			Git: &kptfile.GitLock{
				Repo:      repo,
				Directory: p.prKey.PkgKey.ToPkgPathname(),
				Ref:       ref.Short(),
				Commit:    p.commit.String(),
			},
		}, nil
}

func (p *gitPackageRevision) Lifecycle(ctx context.Context) v1alpha1.PackageRevisionLifecycle {
	return p.repo.GetLifecycle(ctx, p)
}

func (p *gitPackageRevision) UpdateLifecycle(ctx context.Context, new v1alpha1.PackageRevisionLifecycle) error {
	ctx, span := tracer.Start(ctx, "gitPackageRevision::UpdateLifecycle", trace.WithAttributes())
	defer span.End()

	return p.repo.UpdateLifecycle(ctx, p, new)
}

func (p *gitPackageRevision) GetMeta() metav1.ObjectMeta {
	return p.metadata
}

func (p *gitPackageRevision) SetMeta(_ context.Context, metadata metav1.ObjectMeta) error {
	p.metadata = metadata
	return nil
}
