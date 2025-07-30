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

package pull

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	porchapi "github.com/nephio-project/porch/api/porch/v1alpha1"
	"github.com/nephio-project/porch/internal/kpt/errors"
	"github.com/nephio-project/porch/internal/kpt/util/cmdutil"
	"github.com/nephio-project/porch/internal/kpt/util/porch"
	"github.com/nephio-project/porch/pkg/cli/commands/rpkg/docs"
	"github.com/nephio-project/porch/pkg/cli/commands/rpkg/util"
	kptfilev1 "github.com/nephio-project/porch/pkg/kpt/api/kptfile/v1"
	"github.com/nephio-project/porch/pkg/kpt/printer"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/kustomize/kyaml/kio"
	"sigs.k8s.io/kustomize/kyaml/kio/kioutil"
)

const (
	command = "cmdrpkgpull"
)

func newRunner(ctx context.Context, rcg *genericclioptions.ConfigFlags) *runner {
	r := &runner{
		ctx: ctx,
		cfg: rcg,
	}
	c := &cobra.Command{
		Use:        "pull PACKAGE [DIR]",
		Aliases:    []string{"source", "read"},
		SuggestFor: []string{},
		Short:      docs.PullShort,
		Long:       docs.PullShort + "\n" + docs.PullLong,
		Example:    docs.PullExamples,
		PreRunE:    r.preRunE,
		RunE:       r.runE,
		Hidden:     porch.HidePorchCommands,
	}
	r.Command = c
	return r
}

func NewCommand(ctx context.Context, rcg *genericclioptions.ConfigFlags) *cobra.Command {
	return newRunner(ctx, rcg).Command
}

type runner struct {
	ctx     context.Context
	cfg     *genericclioptions.ConfigFlags
	client  client.Client
	Command *cobra.Command
	printer printer.Printer
}

func (r *runner) preRunE(_ *cobra.Command, _ []string) error {
	const op errors.Op = command + ".preRunE"
	config, err := r.cfg.ToRESTConfig()
	if err != nil {
		return errors.E(op, err)
	}

	scheme, err := createScheme()
	if err != nil {
		return errors.E(op, err)
	}

	c, err := client.New(config, client.Options{Scheme: scheme})
	if err != nil {
		return errors.E(op, err)
	}

	r.client = c
	r.printer = printer.FromContextOrDie(r.ctx)
	return nil
}

func (r *runner) runE(_ *cobra.Command, args []string) error {
	const op errors.Op = command + ".runE"

	if len(args) == 0 {
		return errors.E(op, "PACKAGE is a required positional argument")
	}

	packageName := args[0]

	var resources porchapi.PackageRevisionResources
	if err := r.client.Get(r.ctx, client.ObjectKey{
		Namespace: *r.cfg.Namespace,
		Name:      packageName,
	}, &resources); err != nil {
		return errors.E(op, err)
	}

	if err := util.AddRevisionMetadata(&resources); err != nil {
		return errors.E(op, err)
	}

	if len(args) > 1 {
		if err := writeToDir(resources.Spec.Resources, args[1]); err != nil {
			return errors.E(op, err)
		}
	} else {
		if err := writeToWriter(resources.Spec.Resources, r.printer.OutStream()); err != nil {
			return errors.E(op, err)
		}
	}
	return nil
}

func writeToDir(resources map[string]string, dir string) error {
	if err := cmdutil.CheckDirectoryNotPresent(dir); err != nil {
		return fmt.Errorf("failed to check directory %s: %w", dir, err)
	}
	if err := os.MkdirAll(dir, 0750); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	for k, v := range resources {
		f := filepath.Join(dir, k)
		d := filepath.Dir(f)
		if err := os.MkdirAll(d, 0750); err != nil {
			return fmt.Errorf("failed to create subdirectory %s: %w", d, err)
		}
		if err := os.WriteFile(f, []byte(v), 0644); err != nil {
			return fmt.Errorf("failed to write file %s: %w", f, err)
		}
	}
	return nil
}

func writeToWriter(resources map[string]string, out io.Writer) error {
	keys := make([]string, 0, len(resources))
	for k := range resources {
		if !includeFile(k) {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Create kio readers
	inputs := []kio.Reader{}
	for _, k := range keys {
		v := resources[k]
		inputs = append(inputs, &kio.ByteReader{
			Reader: strings.NewReader(v),
			SetAnnotations: map[string]string{
				kioutil.PathAnnotation: k,
			},
			DisableUnwrapping: true,
		})
	}

	err := kio.Pipeline{
		Inputs: inputs,
		Outputs: []kio.Writer{
			kio.ByteWriter{
				Writer:                out,
				KeepReaderAnnotations: true,
				WrappingKind:          kio.ResourceListKind,
				WrappingAPIVersion:    kio.ResourceListAPIVersion,
				Sort:                  true,
			},
		},
	}.Execute()
	if err != nil {
		return fmt.Errorf("failed to execute kio pipeline: %w", err)
	}
	return nil
}

func createScheme() (*runtime.Scheme, error) {
	scheme := runtime.NewScheme()

	for _, api := range (runtime.SchemeBuilder{
		porchapi.AddToScheme,
	}) {
		if err := api(scheme); err != nil {
			return nil, err
		}
	}
	return scheme, nil
}

var matchResourceContents = append(kio.MatchAll, kptfilev1.KptFileName, kptfilev1.RevisionMetaDataFileName)

func includeFile(path string) bool {
	for _, m := range matchResourceContents {
		// Only use the filename for the check for whether we should
		// include the file.
		f := filepath.Base(path)
		if matched, err := filepath.Match(m, f); err == nil && matched {
			return true
		}
	}
	return false
}
