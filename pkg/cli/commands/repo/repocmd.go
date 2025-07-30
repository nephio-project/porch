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

package repo

import (
	"context"
	"flag"
	"fmt"

	"github.com/nephio-project/porch/internal/kpt/util/porch"
	"github.com/nephio-project/porch/pkg/cli/commands/repo/docs"
	"github.com/nephio-project/porch/pkg/cli/commands/repo/get"
	"github.com/nephio-project/porch/pkg/cli/commands/repo/reg"
	"github.com/nephio-project/porch/pkg/cli/commands/repo/unreg"
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/rest"
)

func NewCommand(ctx context.Context, version string) *cobra.Command {
	repo := &cobra.Command{
		Use:     "repo",
		Aliases: []string{"repository"},
		Short:   docs.RepoShort,
		Long:    docs.RepoLong,
		RunE: func(cmd *cobra.Command, args []string) error {
			h, err := cmd.Flags().GetBool("help")
			if err != nil {
				return fmt.Errorf("failed to get help flag: %w", err)
			}
			if h {
				err := cmd.Help()
				if err != nil {
					return fmt.Errorf("failed to display help: %w", err)
				}
				return nil
			}
			err = cmd.Usage()
			if err != nil {
				return fmt.Errorf("failed to display usage: %w", err)
			}
			return nil
		},
		Hidden: porch.HidePorchCommands,
	}

	pf := repo.PersistentFlags()

	kubeflags := genericclioptions.NewConfigFlags(true)
	kubeflags.AddFlags(pf)

	kubeflags.WrapConfigFn = func(rc *rest.Config) *rest.Config {
		rc.UserAgent = fmt.Sprintf("porchctl/%s", version)
		return rc
	}

	pf.AddGoFlagSet(flag.CommandLine)

	repo.AddCommand(
		reg.NewCommand(ctx, kubeflags),
		get.NewCommand(ctx, kubeflags),
		unreg.NewCommand(ctx, kubeflags),
	)

	return repo
}
