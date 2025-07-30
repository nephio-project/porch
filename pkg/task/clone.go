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

package task

import (
	"context"
	"fmt"
	"os"

	api "github.com/nephio-project/porch/api/porch/v1alpha1"
	configapi "github.com/nephio-project/porch/api/porchconfig/v1alpha1"
	"github.com/nephio-project/porch/internal/kpt/builtins"
	"github.com/nephio-project/porch/pkg/externalrepo/git"
	externalrepotypes "github.com/nephio-project/porch/pkg/externalrepo/types"
	"github.com/nephio-project/porch/pkg/kpt"
	v1 "github.com/nephio-project/porch/pkg/kpt/api/kptfile/v1"
	"github.com/nephio-project/porch/pkg/repository"
	pkgerrors "github.com/pkg/errors"
	"go.opentelemetry.io/otel/trace"
	"k8s.io/klog/v2"
)

var _ mutation = &clonePackageMutation{}

type clonePackageMutation struct {
	task *api.Task

	// namespace is the namespace against which we resolve references.
	// TODO: merge namespace into referenceResolver?
	namespace string

	name               string // package target name
	isDeployment       bool   // is the package deployable instance
	repoOpener         repository.RepositoryOpener
	credentialResolver repository.CredentialResolver
	referenceResolver  repository.ReferenceResolver

	// packageConfig contains the package configuration.
	packageConfig *builtins.PackageConfig
}

func (m *clonePackageMutation) apply(ctx context.Context, resources repository.PackageResources) (repository.PackageResources, *api.TaskResult, error) {
	ctx, span := tracer.Start(ctx, "clonePackageMutation::apply", trace.WithAttributes())
	defer span.End()

	var cloned repository.PackageResources
	var err error

	if ref := m.task.Clone.Upstream.UpstreamRef; ref != nil {
		cloned, err = m.cloneFromRegisteredRepository(ctx, ref)
	} else if git := m.task.Clone.Upstream.Git; git != nil {
		cloned, err = m.cloneFromGit(ctx, git)
	} else if oci := m.task.Clone.Upstream.Oci; oci != nil {
		cloned, err = m.cloneFromOci(ctx, oci)
	} else {
		err = fmt.Errorf("invalid clone source (neither of git, oci, nor upstream were specified)")
	}

	if err != nil {
		return repository.PackageResources{}, nil, err
	}

	// Add any pre-existing parts of the config that have not been overwritten by the clone operation.
	for k, v := range resources.Contents {
		if _, exists := cloned.Contents[k]; !exists {
			cloned.Contents[k] = v
		}
	}

	if m.isDeployment {
		// TODO(droot): executing this as mutation is not really needed, but can be
		// refactored once we finalize the task/mutation/commit model.
		genPkgContextMutation, err := newPackageContextGeneratorMutation(m.packageConfig)
		if err != nil {
			return repository.PackageResources{}, nil, err
		}
		cloned, _, err = genPkgContextMutation.apply(ctx, cloned)
		if err != nil {
			return repository.PackageResources{}, nil, fmt.Errorf("failed to generate deployment context: %w", err)
		}
	}

	// ensure merge-key comment is added to newly added resources.
	// this operation is done on best effort basis because if upstream contains
	// valid YAML but invalid KRM resources, merge-key operation will fail
	// but shouldn't result in overall clone operation.
	result, err := ensureMergeKey(ctx, cloned)
	if err != nil {
		klog.Infof("failed to add merge-key to resources %v", err)
	}

	return result, &api.TaskResult{Task: m.task}, nil
}

func (m *clonePackageMutation) cloneFromRegisteredRepository(ctx context.Context, ref *api.PackageRevisionRef) (repository.PackageResources, error) {
	if ref.Name == "" {
		return repository.PackageResources{}, fmt.Errorf("upstreamRef.name is required")
	}

	upstreamRevision, err := (&repository.PackageFetcher{
		RepoOpener:        m.repoOpener,
		ReferenceResolver: m.referenceResolver,
	}).FetchRevision(ctx, ref, m.namespace)
	if err != nil {
		return repository.PackageResources{}, fmt.Errorf("failed to fetch package revision %q: %w", ref.Name, err)
	}

	resources, err := upstreamRevision.GetResources(ctx)
	if err != nil {
		return repository.PackageResources{}, fmt.Errorf("cannot read contents of package %q: %w", ref.Name, err)
	}

	upstream, lock, err := upstreamRevision.GetLock()
	if err != nil {
		return repository.PackageResources{}, fmt.Errorf("cannot determine upstream lock for package %q: %w", ref.Name, err)
	}

	// Update Kptfile
	if err := kpt.UpdateKptfileUpstream(m.name, resources.Spec.Resources, upstream, lock); err != nil {
		return repository.PackageResources{}, fmt.Errorf("failed to apply upstream lock to package %q: %w", ref.Name, err)
	}

	return repository.PackageResources{
		Contents: resources.Spec.Resources,
	}, nil
}

func (m *clonePackageMutation) cloneFromGit(ctx context.Context, gitPackage *api.GitPackage) (repository.PackageResources, error) {
	// TODO: Cache unregistered repositories with appropriate cache eviction policy.
	// TODO: Separate low-level repository access from Repository abstraction?

	spec := configapi.GitRepository{
		Repo:      gitPackage.Repo,
		Directory: gitPackage.Directory,
		SecretRef: configapi.SecretRef{
			Name: gitPackage.SecretRef.Name,
		},
	}

	dir, err := os.MkdirTemp("", "clone-git-package-*")
	if err != nil {
		return repository.PackageResources{}, pkgerrors.Wrap(err, "cannot create temporary directory to clone Git repository")
	}
	defer os.RemoveAll(dir)

	r, err := git.OpenRepository(ctx, "", m.namespace, &spec, false, dir, git.GitRepositoryOptions{
		ExternalRepoOptions: externalrepotypes.ExternalRepoOptions{
			CredentialResolver: m.credentialResolver,
		},
		MainBranchStrategy: git.SkipVerification, // We are only reading so we don't need the main branch to exist.
	})
	if err != nil {
		return repository.PackageResources{}, pkgerrors.Wrap(err, "cannot clone Git repository")
	}

	revision, lock, err := r.GetPackageRevision(ctx, gitPackage.Ref, gitPackage.Directory)
	if err != nil {
		return repository.PackageResources{}, pkgerrors.Wrapf(err, "cannot find package %s@%s:", gitPackage.Directory, gitPackage.Ref)
	}

	resources, err := revision.GetResources(ctx)
	if err != nil {
		return repository.PackageResources{}, pkgerrors.Wrap(err, "cannot read package resources")
	}

	contents := resources.Spec.Resources

	// Update Kptfile
	if err := kpt.UpdateKptfileUpstream(m.name, contents, v1.Upstream{
		Type: v1.GitOrigin,
		Git: &v1.Git{
			Repo:      lock.Repo,
			Directory: lock.Directory,
			Ref:       lock.Ref,
		},
	}, v1.UpstreamLock{
		Type: v1.GitOrigin,
		Git:  &lock,
	}); err != nil {
		return repository.PackageResources{}, pkgerrors.Wrapf(err, "failed to clone package %s@%s", gitPackage.Directory, gitPackage.Ref)
	}

	return repository.PackageResources{
		Contents: contents,
	}, nil
}

func (m *clonePackageMutation) cloneFromOci(_ context.Context, _ *api.OciPackage) (repository.PackageResources, error) {
	return repository.PackageResources{}, fmt.Errorf("clone from OCI is not implemented")
}
