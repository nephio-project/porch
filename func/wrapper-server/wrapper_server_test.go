package main

import (
	"bytes"
	"context"
	pb "github.com/nephio-project/porch/func/evaluator"
	"github.com/nephio-project/porch/func/internal"
	"sigs.k8s.io/kustomize/kyaml/kio"
	"testing"
)

func TestWrapperServerEvaluate(t *testing.T) {

	internal.SetLogLevel(5)

	tests := []struct {
		name       string
		expectFail bool
		skip       bool
		evaluator  singleFunctionEvaluator
		req        *pb.EvaluateFunctionRequest
	}{
		{
			name:       "New Package Example",
			expectFail: false,
			skip:       false,
			evaluator: singleFunctionEvaluator{
				entrypoint: []string{"cat"},
			},
			req: &pb.EvaluateFunctionRequest{
				ResourceList: createMockResourceList("../../examples/config/new-package.yaml"),
				Image:        "test-image",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := tt.evaluator.EvaluateFunction(context.Background(), tt.req)
			if (err != nil) != tt.expectFail {
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
		return nil
	}

	return b.Bytes()
}
