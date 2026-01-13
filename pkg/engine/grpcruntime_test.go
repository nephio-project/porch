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
	"bytes"
	"context"
	"errors"
	"net"
	"strings"
	"testing"

	v1 "github.com/kptdev/kpt/pkg/api/kptfile/v1"
	"github.com/nephio-project/porch/func/evaluator"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const testImage = "test-image:latest"

func TestNewGRPCFunctionRuntimeSuccess(t *testing.T) {
	addr, stop := startMockServer(t)
	defer stop()

	options := GRPCRuntimeOptions{
		FunctionRunnerAddress: addr,
		MaxGrpcMessageSize:    1024,
		DefaultImagePrefix:    "gcr.io/",
	}

	runtime, err := newGRPCFunctionRuntime(options)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if runtime == nil {
		t.Fatal("expected runtime to be non-nil")
	}
	if runtime.cc == nil {
		t.Error("expected connection to be non-nil")
	}
	if runtime.client == nil {
		t.Error("expected client to be non-nil")
	}
	runtime.Close()
}

func TestNewGRPCFunctionRuntimeEmptyAddress(t *testing.T) {
	options := GRPCRuntimeOptions{
		MaxGrpcMessageSize: 1024,
	}
	runtime, err := newGRPCFunctionRuntime(options)
	if err == nil {
		t.Error("expected error but got none")
		return
	}
	if !strings.Contains(err.Error(), "address is required") {
		t.Errorf("expected error to contain %q, got %q", "address is required", err.Error())
	}
	if runtime != nil {
		runtime.Close()
	}
}

func TestGRPCRuntimeGetRunner(t *testing.T) {
	addr, stop := startMockServer(t)
	defer stop()

	options := GRPCRuntimeOptions{
		FunctionRunnerAddress: addr,
		MaxGrpcMessageSize:    1024,
	}

	runtime, err := newGRPCFunctionRuntime(options)
	if err != nil {
		t.Fatalf("failed to create runtime: %v", err)
	}
	defer runtime.Close()

	fn := &v1.Function{
		Image: testImage,
	}

	runner, err := runtime.GetRunner(t.Context(), fn)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if runner == nil {
		t.Fatal("expected runner to be non-nil")
	}

	grpcRunner, ok := runner.(*grpcRunner)
	if !ok {
		t.Fatal("expected runner to be grpcRunner")
	}
	if grpcRunner.image != fn.Image {
		t.Errorf("expected image %q, got %q", fn.Image, grpcRunner.image)
	}
	if grpcRunner.ctx == nil {
		t.Error("expected context to be non-nil")
	}
	if grpcRunner.client == nil {
		t.Error("expected client to be non-nil")
	}
}

func TestGRPCRuntimeCloseWithConnection(t *testing.T) {
	addr, stop := startMockServer(t)
	defer stop()

	options := GRPCRuntimeOptions{
		FunctionRunnerAddress: addr,
		MaxGrpcMessageSize:    1024,
	}

	runtime, err := newGRPCFunctionRuntime(options)
	if err != nil {
		t.Fatalf("failed to create runtime: %v", err)
	}

	err = runtime.Close()
	if err != nil {
		t.Errorf("Close() error = %v", err)
	}
	if runtime.cc != nil {
		t.Error("expected connection to be nil after close")
	}
}

func TestGRPCRuntimeCloseMultipleCalls(t *testing.T) {
	addr, stop := startMockServer(t)
	defer stop()

	options := GRPCRuntimeOptions{
		FunctionRunnerAddress: addr,
		MaxGrpcMessageSize:    1024,
	}

	runtime, err := newGRPCFunctionRuntime(options)
	if err != nil {
		t.Fatalf("failed to create runtime: %v", err)
	}

	// First close
	err = runtime.Close()
	if err != nil {
		t.Errorf("first Close() error = %v", err)
	}
	if runtime.cc != nil {
		t.Error("expected connection to be nil after first close")
	}

	// Second close should not error
	err = runtime.Close()
	if err != nil {
		t.Errorf("second Close() error = %v", err)
	}
}

func TestGRPCRunnerRunSuccess(t *testing.T) {
	client := &mockClient{
		evaluateFunc: func(ctx context.Context, req *evaluator.EvaluateFunctionRequest) (*evaluator.EvaluateFunctionResponse, error) {
			return &evaluator.EvaluateFunctionResponse{
				ResourceList: []byte(`apiVersion: config.kubernetes.io/v1alpha1
kind: ResourceList
items:
  - apiVersion: v1
    kind: ConfigMap
    metadata:
      name: my-cm
      namespace: test-ns
    data:
      foo: bar
`),
			}, nil
		},
	}
	runner := &grpcRunner{
		ctx:    t.Context(),
		client: client,
		image:  testImage,
	}

	reader := strings.NewReader(`apiVersion: config.kubernetes.io/v1alpha1
kind: ResourceList
items:
  - apiVersion: v1
    kind: ConfigMap
    metadata:
      name: my-cm
      namespace: old
    data:
      foo: bar
functionConfig:
  apiVersion: v1
  kind: ConfigMap
  data:
    namespace: test-ns
`)
	var writer bytes.Buffer

	err := runner.Run(reader, &writer)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !strings.Contains(writer.String(), "test-ns") {
		t.Errorf("expected output to contain namespace test-ns, got %q", writer.String())
	}
}

func TestGRPCRunnerRunEvaluationError(t *testing.T) {
	client := &mockClient{
		evaluateFunc: func(ctx context.Context, req *evaluator.EvaluateFunctionRequest) (*evaluator.EvaluateFunctionResponse, error) {
			return nil, status.Error(codes.Internal, "evaluation failed")
		},
	}

	runner := &grpcRunner{
		ctx:    t.Context(),
		client: client,
		image:  testImage,
	}

	reader := strings.NewReader(`apiVersion: config.kubernetes.io/v1alpha1
kind: ResourceList
items:
  - apiVersion: v1
    kind: ConfigMap
    metadata:
      name: my-cm
      namespace: old
    data:
      foo: bar
`)
	var writer bytes.Buffer

	err := runner.Run(reader, &writer)
	if err == nil {
		t.Error("expected error but got none")
	}
	if !strings.Contains(err.Error(), "func eval") {
		t.Errorf("expected error to contain %q, got %q", "func eval", err.Error())
	}
}

func TestGRPCRunnerRunReadError(t *testing.T) {
	runner := &grpcRunner{
		ctx:    t.Context(),
		client: nil,
		image:  testImage,
	}

	reader := &errorReader{}
	var writer bytes.Buffer

	err := runner.Run(reader, &writer)
	if err == nil {
		t.Error("expected error but got none")
		return
	}
	if !strings.Contains(err.Error(), "failed to read function runner input") {
		t.Errorf("expected error to contain read error message, got %q", err.Error())
	}
}

func TestGRPCRunnerRunWriteError(t *testing.T) {
	client := &mockClient{
		evaluateFunc: func(ctx context.Context, req *evaluator.EvaluateFunctionRequest) (*evaluator.EvaluateFunctionResponse, error) {
			return &evaluator.EvaluateFunctionResponse{
				ResourceList: []byte(`apiVersion: config.kubernetes.io/v1alpha1
kind: ResourceList
items:
  - apiVersion: v1
    kind: ConfigMap
    metadata:
      name: my-cm
      namespace: test-ns
    data:
      foo: bar
`),
			}, nil
		},
	}

	runner := &grpcRunner{
		ctx:    t.Context(),
		client: client,
		image:  testImage,
	}

	reader := strings.NewReader(`apiVersion: config.kubernetes.io/v1alpha1
kind: ResourceList
items:
  - apiVersion: v1
    kind: ConfigMap
    metadata:
      name: my-cm
      namespace: old
    data:
      foo: bar
`)
	writer := &errorWriter{}

	err := runner.Run(reader, writer)
	if err == nil {
		t.Error("expected error but got none")
		return
	}
	if !strings.Contains(err.Error(), "failed to write function runner output") {
		t.Errorf("expected error to contain write error message, got %q", err.Error())
	}
}

type mockClient struct {
	evaluateFunc func(context.Context, *evaluator.EvaluateFunctionRequest) (*evaluator.EvaluateFunctionResponse, error)
}

func (m *mockClient) EvaluateFunction(ctx context.Context, req *evaluator.EvaluateFunctionRequest, opts ...grpc.CallOption) (*evaluator.EvaluateFunctionResponse, error) {
	return m.evaluateFunc(ctx, req)
}

type mockFunctionEvaluator struct {
	evaluator.UnimplementedFunctionEvaluatorServer
	evaluateFunc func(context.Context, *evaluator.EvaluateFunctionRequest) (*evaluator.EvaluateFunctionResponse, error)
}

func (m *mockFunctionEvaluator) EvaluateFunction(ctx context.Context, req *evaluator.EvaluateFunctionRequest) (*evaluator.EvaluateFunctionResponse, error) {
	return m.evaluateFunc(ctx, req)
}

func startMockServer(t *testing.T) (string, func()) {
	mock := &mockFunctionEvaluator{}
	server := grpc.NewServer()
	evaluator.RegisterFunctionEvaluatorServer(server, mock)
	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	go func() {
		server.Serve(lis)
	}()
	return lis.Addr().String(), server.Stop
}

type errorReader struct{}

func (e *errorReader) Read(p []byte) (n int, err error) {
	return 0, errors.New("read error")
}

type errorWriter struct{}

func (e *errorWriter) Write(p []byte) (n int, err error) {
	return 0, errors.New("write error")
}
