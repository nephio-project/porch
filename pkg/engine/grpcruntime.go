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

package engine

import (
	"context"
	"fmt"
	"io"

	"github.com/nephio-project/porch/func/evaluator"
	"github.com/nephio-project/porch/pkg/kpt"
	v1 "github.com/nephio-project/porch/pkg/kpt/api/kptfile/v1"
	"github.com/nephio-project/porch/pkg/kpt/fn"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"k8s.io/klog/v2"
)

type GRPCRuntimeOptions struct {
	FunctionRunnerAddress string
	MaxGrpcMessageSize    int
	DefaultImagePrefix    string
}

type grpcRuntime struct {
	cc     *grpc.ClientConn
	client evaluator.FunctionEvaluatorClient
}

func newGRPCFunctionRuntime(options GRPCRuntimeOptions) (*grpcRuntime, error) {
	if options.FunctionRunnerAddress == "" {
		return nil, fmt.Errorf("address is required to instantiate gRPC function runtime")
	}

	klog.Infof("Dialing grpc function runner %q", options.FunctionRunnerAddress)

	cc, err := grpc.NewClient(options.FunctionRunnerAddress,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(options.MaxGrpcMessageSize),
			grpc.MaxCallSendMsgSize(options.MaxGrpcMessageSize),
		),
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to dial grpc function evaluator: %w", err)
	}

	return &grpcRuntime{
		cc:     cc,
		client: evaluator.NewFunctionEvaluatorClient(cc),
	}, err
}

var _ kpt.FunctionRuntime = &grpcRuntime{}

func (gr *grpcRuntime) GetRunner(ctx context.Context, fn *v1.Function) (fn.FunctionRunner, error) {
	// TODO: Check if the function is actually available?
	return &grpcRunner{
		ctx:    ctx,
		client: gr.client,
		image:  fn.Image,
	}, nil
}

func (gr *grpcRuntime) Close() error {
	var err error
	if gr.cc != nil {
		if err = gr.cc.Close(); err != nil {
			klog.Warningf("Failed to close grpc client connection: %v", err)
		}
		gr.cc = nil
	}
	return err
}

type grpcRunner struct {
	ctx    context.Context
	client evaluator.FunctionEvaluatorClient
	image  string
}

var _ fn.FunctionRunner = &grpcRunner{}

func (gr *grpcRunner) Run(r io.Reader, w io.Writer) error {
	in, err := io.ReadAll(r)
	if err != nil {
		return fmt.Errorf("failed to read function runner input: %w", err)
	}

	res, err := gr.client.EvaluateFunction(gr.ctx, &evaluator.EvaluateFunctionRequest{
		ResourceList: in,
		Image:        gr.image,
	})
	if err != nil {
		return fmt.Errorf("func eval %q failed: %w", gr.image, err)
	}
	if _, err := w.Write(res.ResourceList); err != nil {
		return fmt.Errorf("failed to write function runner output: %w", err)
	}
	return nil
}
