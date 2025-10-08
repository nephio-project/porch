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

	"slices"

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

	upstream   = "upstream"
	downstream = "downstream"
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
		Use:     "upgrade SOURCE_PACKAGE_REVISION",
		PreRunE: r.preRunE,
		RunE:    r.runE,
		Short:   docs.UpgradeShort,
		Long:    docs.UpgradeShort + "\n" + docs.UpgradeLong,
		Example: docs.UpgradeExamples,
		Hidden:  porch.HidePorchCommands,
	}
	r.Command.Flags().IntVar(&r.revision, "revision", 0, "Revision of the upstream package to upgrade to.")
	r.Command.Flags().StringVar(&r.workspace, "workspace", "", "Workspace name of the upgrade package revision.")
	r.Command.Flags().StringVar(&r.strategy, "strategy", "resource-merge", "Strategy to use for the upgrade. Options: resource-merge (default), fast-forward, force-delete-replace, copy-merge.")
	r.Command.Flags().StringVar(&r.discover, "discover", "",
		`If set, search for available updates instead of performing an update.
Setting this to 'upstream' will discover upstream updates of downstream packages.
Setting this to 'downstream' will discover downstream package revisions of upstream packages that need to be updated.`)
	return r
}

type runner struct {
	ctx     context.Context
	cfg     *genericclioptions.ConfigFlags
	client  client.Client
	Command *cobra.Command

	revision  int // Target package revision
	workspace string
	strategy  string // Merge strategy to use, default is "resource-merge"

	discover string // If set, discover updates rather than do updates

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

	switch r.discover {
	case "":
		if len(args) < 1 {
			return errors.E(op, fmt.Errorf("SOURCE_PACKAGE_REVISION is a required positional argument"))
		}
		if len(args) > 1 {
			return errors.E(op, fmt.Errorf("too many arguments; SOURCE_PACKAGE_REVISION is the only accepted positional arguments"))
		}
		if r.revision < 0 {
			return errors.E(op, fmt.Errorf("revision must be positive (and not main)"))
		}
		if r.workspace == "" {
			return errors.E(op, fmt.Errorf("workspace is required"))
		}
		if r.strategy != "" {
			validStrategies := []string{string(porchapi.ResourceMerge), string(porchapi.FastForward), string(porchapi.ForceDeleteReplace), string(porchapi.CopyMerge)}
			valid := slices.Contains(validStrategies, r.strategy)
			if !valid {
				return errors.E(op, fmt.Errorf("invalid strategy %q; must be one of: %v", r.strategy, validStrategies))
			}
		}
	case upstream, downstream:
		// do nothing
	default:
		return errors.E(op, fmt.Errorf("argument for 'discover' must be one of 'upstream' or 'downstream'"))
	}

	packageRevisionList := porchapi.PackageRevisionList{}
	listOpts := []client.ListOption{}
	if r.cfg.Namespace != nil && *r.cfg.Namespace != "" {
		listOpts = append(listOpts, client.InNamespace(*r.cfg.Namespace))
	}
	if err := r.client.List(r.ctx, &packageRevisionList, listOpts...); err != nil {
		return errors.E(op, err)
	}
	r.prs = packageRevisionList.Items

	return nil
}

func (r *runner) runE(cmd *cobra.Command, args []string) error {
	const op errors.Op = command + ".runE"

	if r.discover != "" {
		if err := r.discoverUpdates(cmd, args); err != nil {
			return errors.E(op, err)
		}
		return nil
	}

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

	oldUpstreamName := r.findUpstreamName(pr)
	if oldUpstreamName == "" {
		return nil, pkgerrors.Errorf("upstream source not found for package revision %q:"+
			" no clone or upgrade type package revision was found in the history of the package", pr.Spec.PackageName)
	}

	oldUpstreamPr := r.findPackageRevision(oldUpstreamName)
	if oldUpstreamPr == nil {
		return nil, pkgerrors.Errorf("upstream package revision %s no longer exists", oldUpstreamName)
	}
	if !oldUpstreamPr.IsPublished() {
		return nil, pkgerrors.Errorf("old upstream package revision %s is not published", oldUpstreamPr.Name)
	}
	var newUpstreamPr *porchapi.PackageRevision
	if r.revision == 0 {
		newUpstreamPr = r.findLatestPackageRevisionForRef(oldUpstreamPr.Spec.PackageName)
		if newUpstreamPr == nil {
			return nil, pkgerrors.Errorf("failed to find latest published revision for package %s (--revision was %d)", pr.Spec.PackageName, r.revision)
		}
	} else {
		newUpstreamPr = r.findPackageRevisionForRef(oldUpstreamPr.Spec.PackageName, r.revision)
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
				Name: oldUpstreamPr.Name,
			},
			NewUpstream: porchapi.PackageRevisionRef{
				Name: newUpstreamPr.Name,
			},
			LocalPackageRevisionRef: porchapi.PackageRevisionRef{
				Name: pr.Name,
			},
			Strategy: porchapi.PackageMergeStrategy(r.strategy),
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

func (r *runner) findPackageRevisionForRef(name string, revision int) *porchapi.PackageRevision {
	for i := range r.prs {
		pr := r.prs[i]
		if pr.Spec.PackageName == name && pr.IsPublished() && pr.Spec.Revision == revision {
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

func (r *runner) findUpstreamName(pr *porchapi.PackageRevision) string {
	switch pr.Spec.Tasks[0].Type {
	case porchapi.TaskTypeClone:
		return pr.Spec.Tasks[0].Clone.Upstream.UpstreamRef.Name
	case porchapi.TaskTypeEdit:
		if n := r.findEditOrigin(pr); n != "" {
			return n
		}
		if pr.Status.UpstreamLock != nil {
			if up := r.findUpstreamByLock(pr.Status.UpstreamLock); up != nil {
				return up.Name
			}
		}
		return ""
	case porchapi.TaskTypeUpgrade:
		return pr.Spec.Tasks[0].Upgrade.NewUpstream.Name
	default:
		return ""
	}
}

func (r *runner) findEditOrigin(currentPr *porchapi.PackageRevision) string {
	pr := currentPr
	for pr != nil && pr.Spec.Tasks[0].Type == porchapi.TaskTypeEdit {
		sourceName := pr.Spec.Tasks[0].Edit.Source.Name
		pr = nil
		for _, p := range r.prs {
			if p.Name == sourceName {
				pr = &p
				break
			}
		}
	}
	if pr != nil {
		return r.findUpstreamName(pr)
	}
	return ""
}

func (r *runner) findUpstreamByLock(lock *porchapi.UpstreamLock) *porchapi.PackageRevision {
	if lock == nil || lock.Git == nil {
		return nil
	}

	target := lock.Git
	var bestMatch *porchapi.PackageRevision

	for i := range r.prs {
		candidate := r.prs[i]

		if !r.matchesTarget(candidate, target) {
			continue
		}

		if target.Ref != "" && candidate.Status.UpstreamLock.Git.Ref == target.Ref {
			if bestMatch == nil || candidate.Spec.Revision > bestMatch.Spec.Revision {
				tmp := candidate
				bestMatch = &tmp
			}
		}
	}

	return bestMatch
}

func (r *runner) matchesTarget(candidate porchapi.PackageRevision, target *porchapi.GitLock) bool {
	if !candidate.IsPublished() {
		return false
	}
	upstream := candidate.Status.UpstreamLock
	if upstream == nil || upstream.Git == nil {
		return false
	}

	cGit := upstream.Git
	if cGit.Repo != target.Repo || cGit.Directory != target.Directory {
		return false
	}

	return true
}
