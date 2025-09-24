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

package kpt

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/nephio-project/porch/internal/kpt/fnruntime"
	"github.com/nephio-project/porch/internal/kpt/pkg"
	"github.com/nephio-project/porch/internal/kpt/util/render"
	fnresult "github.com/nephio-project/porch/pkg/kpt/api/fnresult/v1"
	"github.com/nephio-project/porch/pkg/kpt/fn"
	"github.com/nephio-project/porch/pkg/kpt/printer"
	"k8s.io/klog/v2"
	"sigs.k8s.io/kustomize/kyaml/filesys"
)

func NewRenderer(runnerOptions fnruntime.RunnerOptions) fn.Renderer {
	return &renderer{runnerOptions: runnerOptions}
}

type renderer struct {
	runnerOptions fnruntime.RunnerOptions
}

var _ fn.Renderer = &renderer{}

func (r *renderer) Render(ctx context.Context, pkg filesys.FileSystem, opts fn.RenderOptions) (*fnresult.ResultList, error) {
	rr := render.Renderer{
		PkgPath:       opts.PkgPath,
		Runtime:       opts.Runtime,
		FileSystem:    pkg,
		RunnerOptions: r.runnerOptions,
	}
	return rr.Execute(printer.WithContext(ctx, &packagePrinter{}))
}

type packagePrinter struct{}

var _ printer.Printer = &packagePrinter{}

const packagePrefixFormat = "Package: %q"

func (p *packagePrinter) PrintPackage(pkg *pkg.Pkg, leadingNewline bool) {
	p.Printf(packagePrefixFormat, pkg.DisplayPath)
}

func (p *packagePrinter) Printf(format string, args ...interface{}) {
	klog.Infof(format, args...)
}

func (p *packagePrinter) OptPrintf(opt *printer.Options, format string, args ...interface{}) {
	if opt == nil {
		p.Printf(format, args...)
		return
	}
	var prefix string
	if !opt.PkgDisplayPath.Empty() {
		prefix = fmt.Sprintf(packagePrefixFormat, string(opt.PkgDisplayPath))
	} else if !opt.PkgPath.Empty() {
		prefix = fmt.Sprintf(packagePrefixFormat, string(opt.PkgPath))
	}
	p.Printf(prefix+format, args...)
}

func (p *packagePrinter) OutStream() io.Writer {
	return os.Stdout
}

func (p *packagePrinter) ErrStream() io.Writer {
	return os.Stderr
}
