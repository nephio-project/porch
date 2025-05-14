// Copyright 2025 The Nephio Authors
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

package upgrade

import (
	"context"
	"fmt"

	porchapi "github.com/nephio-project/porch/api/porch/v1alpha1"
	"github.com/nephio-project/porch/internal/kpt/errors"
	"github.com/nephio-project/porch/internal/kpt/util/porch"
	"github.com/nephio-project/porch/pkg/cli/commands/rpkg/docs"
	pkgerrors "github.com/pkg/errors"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	command = "cmdrpkgupgrade"
)

func NewCommand(ctx context.Context, rcg *genericclioptions.ConfigFlags) *cobra.Command {
	return newRunner(ctx, rcg).Command
}

func newRunner(ctx context.Context, rcg *genericclioptions.ConfigFlags) *runner {
	r := &runner{
		ctx: ctx,
		cfg: rcg,
	}
	r.Command = &cobra.Command{
		Use:     "upgrade SOURCE_PACKAGE",
		PreRunE: r.preRunE,
		RunE:    r.runE,
		Short:   docs.UpgradeShort,
		Long:    docs.UpgradeShort + "\n" + docs.UpgradeLong,
		Example: docs.UpgradeExamples,
		Hidden:  porch.HidePorchCommands,
	}
	r.Command.Flags().IntVar(&r.revision, "revision", 0, "Revision of the upstream package to upgrade to.")
	r.Command.Flags().StringVar(&r.workspace, "workspace", "", "Workspace name of the upgrade package revision.")
	return r
}

type runner struct {
	ctx     context.Context
	cfg     *genericclioptions.ConfigFlags
	client  client.Client
	Command *cobra.Command

	revision  int // Target package revision
	workspace string

	// there are multiple places where we need access to all package revisions, so
	// we store it in the runner
	prs []porchapi.PackageRevision
}

func (r *runner) preRunE(_ *cobra.Command, args []string) error {
	const op errors.Op = command + ".preRunE"
	c, err := porch.CreateClientWithFlags(r.cfg)
	if err != nil {
		return errors.E(op, err)
	}
	r.client = c

	if len(args) < 1 {
		return errors.E(op, fmt.Errorf("SOURCE_PACKAGE is a required positional argument"))
	}
	if len(args) > 1 {
		return errors.E(op, fmt.Errorf("too many arguments; SOURCE_PACKAGE is the only accepted positional arguments"))
	}
	if r.revision < 0 {
		return errors.E(op, fmt.Errorf("revision must be positive (and not main)"))
	}
	if r.workspace == "" {
		return errors.E(op, fmt.Errorf("workspace is required"))
	}

	packageRevisionList := porchapi.PackageRevisionList{}
	if err := r.client.List(r.ctx, &packageRevisionList, &client.ListOptions{}); err != nil {
		return errors.E(op, err)
	}
	r.prs = packageRevisionList.Items

	return nil
}

func (r *runner) runE(cmd *cobra.Command, args []string) error {
	const op errors.Op = command + ".runE"

	pr := r.findPackageRevision(args[0])
	if pr == nil {
		return errors.E(op, pkgerrors.Errorf("could not find package revision %s", args[0]))
	}
	key := client.ObjectKeyFromObject(pr)
	var newPr *porchapi.PackageRevision
	err := retry.RetryOnConflict(retry.DefaultRetry, func() (err error) {
		if err = r.client.Get(r.ctx, key, pr); err != nil {
			return err
		}
		newPr, err = r.doUpgrade(pr)
		return err
	})
	if err != nil {
		return errors.E(op, err)
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s upgraded to %s\n", pr.Name, newPr.Name); err != nil {
		return errors.E(op, err)
	}

	return nil
}

func (r *runner) doUpgrade(pr *porchapi.PackageRevision) (*porchapi.PackageRevision, error) {
	if !pr.IsPublished() {
		return nil, pkgerrors.Errorf("to upgrade a package, it must be in a published state, not %q", pr.Spec.Lifecycle)
	}

	cloneTask := r.findCloneTask(pr)
	if cloneTask == nil {
		return nil, pkgerrors.Errorf("upstream source not found for package rev %q; only cloned packages can be updated", pr.Spec.PackageName)
	}

	upstreamPr := r.findPackageRevision(cloneTask.Clone.Upstream.UpstreamRef.Name)
	if upstreamPr == nil {
		return nil, pkgerrors.Errorf("upstream package revision %s no longer exists", cloneTask.Clone.Upstream.UpstreamRef.Name)
	}
	if !upstreamPr.IsPublished() {
		return nil, pkgerrors.Errorf("old upstream package revision %s is not published", upstreamPr.Name)
	}
	var newUpstreamPr *porchapi.PackageRevision
	if r.revision == 0 {
		newUpstreamPr = r.findLatestPackageRevisionForRef(upstreamPr.Spec.PackageName)
		if newUpstreamPr == nil {
			return nil, pkgerrors.Errorf("failed to find latest published revision for package %s (--revision was %d)", pr.Spec.PackageName, r.revision)
		}
	} else {
		newUpstreamPr = r.findPackageRevisionForRef(upstreamPr.Spec.PackageName, r.revision)
		if newUpstreamPr == nil {
			return nil, pkgerrors.Errorf("revision %d does not exist for package %s", r.revision, pr.Spec.PackageName)
		}
	}

	if !newUpstreamPr.IsPublished() {
		return nil, pkgerrors.Errorf("new upstream package revision %s is not published", newUpstreamPr.Name)
	}

	upgradeTask := &porchapi.Task{
		Type: porchapi.TaskTypeUpgrade,
		Upgrade: &porchapi.PackageUpgradeTaskSpec{
			OldUpstream: porchapi.PackageRevisionRef{
				Name: upstreamPr.Name,
			},
			NewUpstream: porchapi.PackageRevisionRef{
				Name: newUpstreamPr.Name,
			},
			LocalPackageRevisionRef: porchapi.PackageRevisionRef{
				Name: pr.Name,
			},
			Strategy: porchapi.ResourceMerge,
		},
	}
	newPr := makePackageRevision(pr, r.workspace, upgradeTask)

	err := r.client.Create(r.ctx, newPr)
	return newPr, pkgerrors.Wrapf(err, "failed to do create package revision %q", newPr.Name)
}

func makePackageRevision(oldLocal *porchapi.PackageRevision, workspace string, task *porchapi.Task) *porchapi.PackageRevision {
	return &porchapi.PackageRevision{
		TypeMeta: metav1.TypeMeta{
			APIVersion: porchapi.SchemeGroupVersion.String(),
			Kind:       "PackageRevision",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: oldLocal.Namespace,
		},
		Spec: porchapi.PackageRevisionSpec{
			PackageName:    oldLocal.Spec.PackageName,
			RepositoryName: oldLocal.Spec.RepositoryName,
			WorkspaceName:  workspace,
			Tasks:          []porchapi.Task{*task},
		},
	}
}

func (r *runner) findPackageRevision(prName string) *porchapi.PackageRevision {
	for i := range r.prs {
		pr := r.prs[i]
		if pr.Name == prName {
			return &pr
		}
	}
	return nil
}

func (r *runner) findCloneTask(pr *porchapi.PackageRevision) *porchapi.Task {
	if len(pr.Spec.Tasks) == 0 {
		return nil
	}
	if firstTask := pr.Spec.Tasks[0]; firstTask.Type == porchapi.TaskTypeClone {
		return &firstTask
	}
	return nil
}

func (r *runner) findPackageRevisionForRef(name string, revision int) *porchapi.PackageRevision {
	for i := range r.prs {
		pr := r.prs[i]
		if pr.Spec.PackageName == name && pr.Spec.Revision == revision {
			return &pr
		}
	}
	return nil
}

func (r *runner) findLatestPackageRevisionForRef(name string) *porchapi.PackageRevision {
	latest := 0
	var output *porchapi.PackageRevision
	for _, pr := range r.prs {
		if pr.Spec.PackageName == name && pr.IsPublished() && pr.Spec.Revision > latest {
			latest = pr.Spec.Revision
			output = &pr
		}
	}
	return output
}
