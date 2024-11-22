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

package internal

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	pb "github.com/nephio-project/porch/func/evaluator"
	v1 "github.com/nephio-project/porch/pkg/kpt/api/kptfile/v1"
	"github.com/nephio-project/porch/pkg/kpt/fn"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gopkg.in/yaml.v2"
	"k8s.io/klog/v2"
)

type ExecutableEvaluatorOptions struct {
	ConfigFileName   string // Path to the config file
	FunctionCacheDir string // Path to cached functions
}

type executableEvaluator struct {
	// Fast-path function cache
	cache map[string]string
}

type configuration struct {
	Functions []function `yaml:"functions"`
}

type function struct {
	Function string   `yaml:"function"`
	Images   []string `yaml:"images"`
}

var _ Evaluator = &executableEvaluator{}

func NewExecutableEvaluator(o ExecutableEvaluatorOptions) (Evaluator, error) {
	cache := map[string]string{}

	if o.ConfigFileName != "" {
		bytes, err := os.ReadFile(o.ConfigFileName)
		if err != nil {
			return nil, fmt.Errorf("failed to read configuration file %q: %w", o.ConfigFileName, err)
		}
		var cfg configuration
		if err := yaml.Unmarshal(bytes, &cfg); err != nil {
			return nil, fmt.Errorf("failed to parse configuration file %q: %w", o.ConfigFileName, err)
		}

		for _, fn := range cfg.Functions {
			for _, img := range fn.Images {
				if _, exists := cache[img]; exists {
					klog.Warningf("Ignoring duplicate image %q (%s)", img, fn.Function)
				} else {
					abs, err := filepath.Abs(filepath.Join(o.FunctionCacheDir, fn.Function))
					if err != nil {
						return nil, fmt.Errorf("failed to determine path to the cached function %q: %w", img, err)
					}
					klog.Infof("Caching %s as %s", img, abs)
					cache[img] = abs
				}
			}
		}
	}
	return &executableEvaluator{
		cache: cache,
	}, nil
}

func (e *executableEvaluator) EvaluateFunction(ctx context.Context, req *pb.EvaluateFunctionRequest) (*pb.EvaluateFunctionResponse, error) {
	binary, cached := e.cache[req.Image]
	if !cached {
		return nil, &fn.NotFoundError{
			Function: v1.Function{Image: req.Image},
		}
	}

	klog.Infof("Evaluating %q in executable mode", req.Image)
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, binary)
	cmd.Stdin = bytes.NewReader(req.ResourceList)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to execute function %q: %s (%s)", req.Image, err, stderr.String())
	}

	outbytes := stdout.Bytes()

	klog.Infof("Evaluated %q: stdout %d bytes, stderr:\n%s", req.Image, len(outbytes), stderr.String())

	// TODO: include stderr in the output?
	return &pb.EvaluateFunctionResponse{
		ResourceList: outbytes,
		Log:          stderr.Bytes(),
	}, nil
}
