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

package main

import (
	"bytes"
	"context"
	"flag"
	pb "github.com/nephio-project/porch/func/evaluator"
	"k8s.io/klog/v2"
	"sigs.k8s.io/kustomize/kyaml/kio"
	"testing"
)

func TestWrapperServerEvaluate(t *testing.T) {

	flagSet := flag.NewFlagSet("log-level", flag.ContinueOnError)
	klog.InitFlags(flagSet)
	_ = flagSet.Parse([]string{"--v", "5"})

	tests := []struct {
		name       string
		expectFail bool
		skip       bool
		evaluator  singleFunctionEvaluator
		req        *pb.EvaluateFunctionRequest
	}{
		{
			name:       "Successful evaluation",
			expectFail: false,
			skip:       false,
			evaluator: singleFunctionEvaluator{
				entrypoint: []string{"./search_replace_test.sh", "default", "namespace1"},
			},
			req: &pb.EvaluateFunctionRequest{
				ResourceList: createMockResourceList("../../examples/config/oci-repository.yaml"),
				Image:        "new-package",
			},
		},
		{
			name:       "Unsuccessful function evaluation1",
			expectFail: false,
			skip:       false,
			evaluator: singleFunctionEvaluator{
				entrypoint: []string{"sed", "-i", "s/$search_term/$replace_term/g"},
			},
			req: &pb.EvaluateFunctionRequest{
				ResourceList: createMockResourceList("../../examples/config/new-package.yaml"),
				Image:        "new-package",
			},
		},
		{
			name:       "Unsuccessful function evaluation3",
			expectFail: false,
			skip:       false,
			evaluator: singleFunctionEvaluator{
				entrypoint: []string{""},
			},
			req: &pb.EvaluateFunctionRequest{
				ResourceList: createMockResourceList("../../examples/config/new-package.yaml"),
				Image:        "new-package",
			},
		},
		{
			name:       "Unsuccessful function evaluation3",
			expectFail: false,
			skip:       false,
			evaluator: singleFunctionEvaluator{
				entrypoint: []string{"docker", "run", "hello-world"},
			},
			req: &pb.EvaluateFunctionRequest{
				ResourceList: createMockResourceList("../../examples/config/new-package.yaml"),
				Image:        "new-package",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := tt.evaluator.EvaluateFunction(context.Background(), tt.req)
			if (err != nil) != tt.expectFail {
				klog.Infof("%v", err)
			}
			if resp != nil {
			}
		})
	}
}

func createMockResourceList(pkg string) []byte {
	r := kio.LocalPackageReader{
		PackagePath:        pkg,
		IncludeSubpackages: true,
		WrapBareSeqNode:    true,
	}

	var b bytes.Buffer
	w := kio.ByteWriter{
		Writer:                &b,
		KeepReaderAnnotations: true,
		Style:                 0,
		FunctionConfig:        nil,
		WrappingKind:          kio.ResourceListKind,
		WrappingAPIVersion:    kio.ResourceListAPIVersion,
	}

	if err := (kio.Pipeline{Inputs: []kio.Reader{r}, Outputs: []kio.Writer{w}}).Execute(); err != nil {
		panic(err)
		return nil
	}

	return b.Bytes()
}
