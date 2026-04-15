// Copyright 2022, 2026 The kpt and Nephio Authors
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
	"slices"
	"strings"

	semver "github.com/Masterminds/semver/v3"
	kptfilev1 "github.com/kptdev/kpt/pkg/api/kptfile/v1"
	"github.com/kptdev/kpt/pkg/fn"
	pb "github.com/nephio-project/porch/func/evaluator"
	regclientref "github.com/regclient/regclient/types/ref"
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

func (e *executableEvaluator) filterByConstraint(req *pb.EvaluateFunctionRequest) (string, error) {
	c, err := semver.NewConstraint(req.Tag)
	if err != nil {
		return "", &fn.NotFoundError{
			Function: kptfilev1.Function{Image: req.Image},
		}
	}

	type candidate struct {
		baseName string
		version  *semver.Version
		binary   string
	}

	// Filter the cache map by semver constraint validation
	var filteredCache []candidate
	for img, bin := range e.cache {
		// Extract the version string after ":v" in the image name
		idx := strings.LastIndex(img, ":v")
		// Currently, hashed and latest tagged images would be filtered out
		if idx == -1 {
			continue
		}

		baseName := img[:idx]
		if baseName != req.Image {
			continue
		}

		versionStr := img[idx+1:] // skip past ":"
		v, err := semver.NewVersion(versionStr)
		if err != nil {
			klog.Infof("Failed to parse version %q from cached image %q: %v", versionStr, img, err)
			continue
		}

		cand := candidate{
			baseName: baseName,
			version:  v,
			binary:   bin,
		}
		if ok, _ := c.Validate(v); ok {
			filteredCache = append(filteredCache, cand)
		}
	}

	// Check if any matching image was found
	if len(filteredCache) == 0 {
		klog.Infof("Image %q with constraint %q is not found in the cache", req.Image, req.Tag)
		return "", &fn.NotFoundError{
			Function: kptfilev1.Function{Image: req.Image},
		}
	}

	// Sort by semver and select the greatest version
	slices.SortFunc(filteredCache, func(a, b candidate) int {
		return a.version.Compare(b.version)
	})

	selected := filteredCache[len(filteredCache)-1]
	klog.Infof("Selected image \"%s:%s\" (version %q) for request %q",
		selected.baseName, selected.version.Original(), selected.version, req.Image)

	return selected.binary, nil
}

func (e *executableEvaluator) EvaluateFunction(ctx context.Context, req *pb.EvaluateFunctionRequest) (*pb.EvaluateFunctionResponse, error) {
	var selectedBinary string
	if req.Tag != "" {
		ref, err := regclientref.New(req.Image)
		if err != nil {
			return nil, fmt.Errorf("failed to parse image %q as reference: %w", req.Image, err)
		}
		ref.Tag = ""
		ref.Digest = ""
		req.Image = ref.CommonName()

		selectedBinary, err = e.filterByConstraint(req)
		if err != nil {
			return nil, err
		}
	} else {
		klog.Infof("Image tag is empty, using the image with explicit tag: %q", req.Image)
		binary, cached := e.cache[req.Image]
		if !cached {
			klog.Infof("Image %q is not found in the cache", req.Image)
			return nil, &fn.NotFoundError{
				Function: kptfilev1.Function{Image: req.Image},
			}
		}
		selectedBinary = binary
	}

	klog.Infof("Evaluating %q in executable mode", req.Image)
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, selectedBinary) // #nosec G204 -- variables controlled internally
	cmd.Stdin = bytes.NewReader(req.ResourceList)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		klog.V(4).Infof("Resource List: %s", req.ResourceList)
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
