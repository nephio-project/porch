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

package copy

import (
	"context"
	"fmt"

	porchapiv1alpha1 "github.com/nephio-project/porch/api/porch/v1alpha1"
	"github.com/nephio-project/porch/internal/kpt/errors"
	"github.com/nephio-project/porch/internal/kpt/util/porch"
	"github.com/nephio-project/porch/pkg/cli/commands/rpkg/docs"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	command = "cmdrpkgcopy"
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
		Use:     "copy SOURCE_PACKAGE NAME",
		Aliases: []string{"edit"},
		Short:   docs.CopyShort,
		Long:    docs.CopyShort + "\n" + docs.CopyLong,
		Example: docs.CopyExamples,
		PreRunE: r.preRunE,
		RunE:    r.runE,
		Hidden:  porch.HidePorchCommands,
	}
	r.Command.Flags().StringVar(&r.workspace, "workspace", "", "Workspace name of the copy of the package.")
	return r
}

type runner struct {
	ctx     context.Context
	cfg     *genericclioptions.ConfigFlags
	client  client.Client
	Command *cobra.Command

	copy porchapiv1alpha1.PackageEditTaskSpec

	workspace string // Target package revision workspaceName
}

func (r *runner) preRunE(_ *cobra.Command, args []string) error {
	const op errors.Op = command + ".preRunE"
	client, err := porch.CreateClientWithFlags(r.cfg)
	if err != nil {
		return errors.E(op, err)
	}
	r.client = client

	if len(args) < 1 {
		return errors.E(op, fmt.Errorf("SOURCE_PACKAGE is a required positional argument"))
	}
	if len(args) > 1 {
		return errors.E(op, fmt.Errorf("too many arguments; SOURCE_PACKAGE is the only accepted positional arguments"))
	}

	r.copy.Source = &porchapiv1alpha1.PackageRevisionRef{
		Name: args[0],
	}
	return nil
}

func (r *runner) runE(cmd *cobra.Command, _ []string) error {
	const op errors.Op = command + ".runE"

	revisionSpec, err := r.getPackageRevisionSpec()
	if err != nil {
		return errors.E(op, err)
	}

	pr := &porchapiv1alpha1.PackageRevision{
		TypeMeta: metav1.TypeMeta{
			Kind:       "PackageRevision",
			APIVersion: porchapiv1alpha1.SchemeGroupVersion.Identifier(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: *r.cfg.Namespace,
		},
		Spec: *revisionSpec,
	}
	if err := r.client.Create(r.ctx, pr); err != nil {
		return errors.E(op, err)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "%s created\n", pr.Name)
	return nil
}

func (r *runner) getPackageRevisionSpec() (*porchapiv1alpha1.PackageRevisionSpec, error) {
	packageRevision := porchapiv1alpha1.PackageRevision{}
	err := r.client.Get(r.ctx, types.NamespacedName{
		Name:      r.copy.Source.Name,
		Namespace: *r.cfg.Namespace,
	}, &packageRevision)
	if err != nil {
		return nil, err
	}

	if r.workspace == "" {
		return nil, fmt.Errorf("--workspace is required to specify workspace name")
	}

	spec := &porchapiv1alpha1.PackageRevisionSpec{
		PackageName:    packageRevision.Spec.PackageName,
		WorkspaceName:  r.workspace,
		RepositoryName: packageRevision.Spec.RepositoryName,
	}

	spec.Tasks = []porchapiv1alpha1.Task{
		{
			Type: porchapiv1alpha1.TaskTypeEdit,
			Edit: &porchapiv1alpha1.PackageEditTaskSpec{
				Source: &porchapiv1alpha1.PackageRevisionRef{
					Name: packageRevision.Name,
				},
			},
		},
	}

	return spec, nil
}
