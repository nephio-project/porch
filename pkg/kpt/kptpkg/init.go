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

package kptpkg

import (
	"bytes"
	"context"
	"html/template"
	"path/filepath"
	"strings"

	api "github.com/nephio-project/porch/api/porch/v1alpha1"
	"github.com/nephio-project/porch/internal/kpt/builtins"
	"github.com/nephio-project/porch/internal/kpt/pkg"
	"github.com/nephio-project/porch/internal/kpt/util/man"
	kptfile "github.com/nephio-project/porch/pkg/kpt/api/kptfile/v1"
	"github.com/nephio-project/porch/pkg/kpt/printer"
	"sigs.k8s.io/kustomize/kyaml/errors"
	"sigs.k8s.io/kustomize/kyaml/filesys"
	"sigs.k8s.io/kustomize/kyaml/kio/filters"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

// Initializer defines capability to initialize a kpt package.
type Initializer interface {
	Initialize(ctx context.Context, pkg filesys.FileSystem, opts InitOptions) error
}

// InitOptions contains customization options for package initialization.
type InitOptions struct {
	PkgName string
	PkgPath string
	// RelPath is used purely for printing info relative to current working dir of user.
	// It may or may not be same as PkgPath.
	RelPath             string
	Desc                string
	Keywords            []string
	Site                string
	ReadinessConditions []api.Condition
}

// DefaultInitilizer implements Initializer interface.
type DefaultInitializer struct{}

func (i *DefaultInitializer) Initialize(
	ctx context.Context,
	fsys filesys.FileSystem,
	opts InitOptions,
) error {
	p, err := pkg.New(fsys, opts.PkgPath)
	if err != nil {
		return err
	}

	var pkgName string
	if opts.PkgName != "" {
		pkgName = opts.PkgName
	} else {
		pkgName = string(p.DisplayPath)
	}

	up := string(p.UniquePath)
	if !fsys.Exists(string(p.UniquePath)) {
		return errors.Errorf("%s does not exist", p.UniquePath)
	}

	pr := printer.FromContextOrDie(ctx)

	if !fsys.Exists(filepath.Join(up, kptfile.KptFileName)) {
		pr.Printf("writing %s\n", filepath.Join(opts.RelPath, "Kptfile"))

		kptfileConditions := kptfile.ConvertApiConditions(opts.ReadinessConditions)
		readinessGates := func() (kptfileGates []kptfile.ReadinessGate) {
			for _, each := range kptfileConditions {
				kptfileGates = append(kptfileGates, kptfile.ReadinessGate{
					ConditionType: each.Type,
				})
			}
			return kptfileGates
		}()

		k := kptfile.KptFile{
			ResourceMeta: yaml.ResourceMeta{
				ObjectMeta: yaml.ObjectMeta{
					NameMeta: yaml.NameMeta{
						Name: pkgName,
					},
					// mark Kptfile as local-config
					Annotations: map[string]string{
						filters.LocalConfigAnnotation: "true",
					},
				},
			},
			Info: &kptfile.PackageInfo{
				Description: opts.Desc,
				Site:        opts.Site,
				Keywords:    opts.Keywords,
			},
		}
		if len(kptfileConditions) > 0 {
			k.Status = &kptfile.Status{
				Conditions: kptfileConditions,
			}
		}
		if len(readinessGates) > 0 {
			k.Info.ReadinessGates = readinessGates
		}

		// serialize the gvk when writing the Kptfile
		k.Kind = kptfile.TypeMeta.Kind
		k.APIVersion = kptfile.TypeMeta.APIVersion

		err = func() error {
			f, err := fsys.Create(filepath.Join(up, kptfile.KptFileName))
			if err != nil {
				return err
			}
			defer f.Close()
			e := yaml.NewEncoder(f)

			defer e.Close()
			return e.Encode(k)
		}()
		if err != nil {
			return err
		}
	}

	if !fsys.Exists(filepath.Join(up, man.ManFilename)) {
		pr.Printf("writing %s\n", filepath.Join(opts.RelPath, man.ManFilename))
		buff := &bytes.Buffer{}
		t, err := template.New("man").Parse(manTemplate)
		if err != nil {
			return err
		}
		templateData := map[string]string{
			"Name":        pkgName,
			"Description": opts.Desc,
		}

		err = t.Execute(buff, templateData)
		if err != nil {
			return err
		}

		// Replace single quotes with backticks.
		content := strings.ReplaceAll(buff.String(), "'", "`")

		err = fsys.WriteFile(filepath.Join(up, man.ManFilename), []byte(content))
		if err != nil {
			return err
		}
	}

	pkgContextPath := filepath.Join(up, builtins.PkgContextFile)
	if !fsys.Exists(pkgContextPath) {
		pr.Printf("writing %s\n", filepath.Join(opts.RelPath, builtins.PkgContextFile))
		if err := fsys.WriteFile(pkgContextPath, []byte(builtins.AbstractPkgContext())); err != nil {
			return err
		}
	}
	return nil
}

// manTemplate is the content for the automatically generated README.md file.
// It uses ' instead of ` since golang doesn't allow using ` in a raw string
// literal. We do a replace on the content before printing.
var manTemplate = `# {{.Name}}

## Description
{{.Description}}

## Usage

### Fetch the package
'kpt pkg get REPO_URI[.git]/PKG_PATH[@VERSION] {{.Name}}'
Details: https://kpt.dev/reference/cli/pkg/get/

### View package content
'kpt pkg tree {{.Name}}'
Details: https://kpt.dev/reference/cli/pkg/tree/

### Apply the package
'''
kpt live init {{.Name}}
kpt live apply {{.Name}} --reconcile-timeout=2m --output=table
'''
Details: https://kpt.dev/reference/cli/live/
`
