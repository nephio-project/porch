// Copyright 2022 The kpt and Nephio Authors
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

package init

import (
	"context"
	"fmt"

	porchapi "github.com/nephio-project/porch/api/porch/v1alpha1"
	"github.com/nephio-project/porch/internal/kpt/errors"
	"github.com/nephio-project/porch/internal/kpt/util/porch"
	"github.com/nephio-project/porch/pkg/cli/commands/rpkg/docs"
	"github.com/nephio-project/porch/pkg/cli/commands/rpkg/util"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	command = "cmdrpkginit"
)

func NewCommand(ctx context.Context, rcg *genericclioptions.ConfigFlags) *cobra.Command {
	return newRunner(ctx, rcg).Command
}

func newRunner(ctx context.Context, rcg *genericclioptions.ConfigFlags) *runner {
	r := &runner{
		ctx: ctx,
		cfg: rcg,
	}
	c := &cobra.Command{
		Use:     "init PACKAGE_NAME",
		Short:   docs.InitShort,
		Long:    docs.InitShort + "\n" + docs.InitLong,
		Example: docs.InitExamples,
		PreRunE: r.preRunE,
		RunE:    r.runE,
		Hidden:  porch.HidePorchCommands,
	}
	r.Command = c

	c.Flags().StringVar(&r.Description, "description", "sample description", "short description of the package.")
	c.Flags().StringSliceVar(&r.Keywords, "keywords", []string{}, "list of keywords for the package.")
	c.Flags().StringVar(&r.Site, "site", "", "link to page with information about the package.")
	c.Flags().StringVar(&r.repository, "repository", "", "Repository to which package will be created.")
	c.Flags().StringVar(&r.workspace, "workspace", "", "Workspace name of the package.")

	return r
}

type runner struct {
	ctx     context.Context
	cfg     *genericclioptions.ConfigFlags
	client  client.Client
	Command *cobra.Command

	// Flags
	Keywords    []string
	Description string
	Site        string
	name        string // Target package name
	repository  string // Target repository
	workspace   string // Target workspace name
}

func (r *runner) preRunE(_ *cobra.Command, args []string) error {
	const op errors.Op = command + ".preRunE"

	client, err := porch.CreateClientWithFlags(r.cfg)
	if err != nil {
		return errors.E(op, err)
	}
	r.client = client

	if len(args) < 1 {
		return errors.E(op, "PACKAGE_NAME is a required positional argument")
	}

	if r.repository == "" {
		return errors.E(op, fmt.Errorf("--repository is required to specify target repository"))
	}

	if r.workspace == "" {
		return errors.E(op, fmt.Errorf("--workspace is required to specify workspace name"))
	}

	r.name = args[0]
	pkgExists, err := util.PackageAlreadyExists(r.ctx, r.client, r.repository, r.name, *r.cfg.Namespace)
	if err != nil {
		return err
	}
	if pkgExists {
		return fmt.Errorf("`init` cannot create a new revision for package %q that already exists in repo %q; make subsequent revisions using `copy`",
			r.name, r.repository)
	}
	return nil
}

func (r *runner) runE(cmd *cobra.Command, _ []string) error {
	const op errors.Op = command + ".runE"

	pr := &porchapi.PorchPkgRevision{
		TypeMeta: metav1.TypeMeta{
			Kind:       "PackageRevision",
			APIVersion: porchapi.SchemeGroupVersion.Identifier(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: *r.cfg.Namespace,
		},
		Spec: porchapi.PorchPkgRevisionSpec{
			PorchPkgName:   r.name,
			WorkspaceName:  porchapi.WorkspaceName(r.workspace),
			RepositoryName: r.repository,
			Tasks: []porchapi.Task{
				{
					Type: porchapi.TaskTypeInit,
					Init: &porchapi.PorchPkgInitTaskSpec{
						Description: r.Description,
						Keywords:    r.Keywords,
						Site:        r.Site,
					},
				},
			},
		},
		Status: porchapi.PorchPkgRevisionStatus{},
	}
	if err := r.client.Create(r.ctx, pr); err != nil {
		return errors.E(op, err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "%s created\n", pr.Name)
	return nil
}
