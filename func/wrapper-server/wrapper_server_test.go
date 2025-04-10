package main

import (
	"bytes"
	"context"
	"fmt"
	pb "github.com/nephio-project/porch/func/evaluator"
	"github.com/nephio-project/porch/func/internal"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"net"
	"sigs.k8s.io/kustomize/kyaml/kio"
	"sigs.k8s.io/kustomize/kyaml/yaml"
	"strings"
	"testing"
)

func (e *singleFunctionEvaluator) Start(ctx context.Context, o *options) (net.Listener, error) {
	address := fmt.Sprintf(":%d", o.port)
	lis, err := net.Listen("tcp", address)
	if err != nil {
		return lis, err
	}

	server := grpc.NewServer()
	pb.RegisterFunctionEvaluatorServer(server, e)

	go server.Serve(lis)
	go func() {
		ctx.Done()
		server.GracefulStop()
		lis.Close()
	}()
	return lis, nil
}

func TestWrapperServerEvaluate(t *testing.T) {

	internal.SetLogLevel(5)

	tests := []struct {
		name       string
		expectFail bool
		skip       bool
		req        *pb.EvaluateFunctionRequest
	}{
		{
			name:       "Success",
			expectFail: false,
			skip:       false,
			req: &pb.EvaluateFunctionRequest{
				ResourceList: createMockResourceList([]string{}, "../../examples/config/new-package.yaml"),
				Image:        "test-image",
			},
		},
	}

	opts := &options{
		port:               9446,
		maxGrpcMessageSize: 6 * 1024 * 1024,
		entrypoint:         []string{"cat"},
	}

	evaluator := &singleFunctionEvaluator{
		entrypoint: opts.entrypoint,
	}

	srvCtx := context.WithoutCancel(context.Background())

	lis, err := evaluator.Start(srvCtx, opts)
	if err != nil {
		t.Errorf("Failed to set up grpc server for testing %v", err)
		t.FailNow()
	}
	defer srvCtx.Done()

	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Errorf("Failed to create grpc connection for testing %v", err)
		t.FailNow()
	}
	defer conn.Close()
	//client := pb.NewFunctionEvaluatorClient(conn)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := evaluator.EvaluateFunction(context.Background(), tt.req)
			if (err != nil) != tt.expectFail {
			}
			if resp != nil {
			}
		})
	}
}

func createMockResourceList(args []string, pkg string) []byte {
	r := kio.LocalPackageReader{
		PackagePath:        pkg,
		IncludeSubpackages: true,
		WrapBareSeqNode:    true,
	}

	cfg, err := mockConfigmap(args)
	if err != nil {
		fmt.Print(err.Error())
		return nil
	}

	var b bytes.Buffer
	w := kio.ByteWriter{
		Writer:                &b,
		KeepReaderAnnotations: true,
		Style:                 0,
		FunctionConfig:        cfg,
		WrappingKind:          kio.ResourceListKind,
		WrappingAPIVersion:    kio.ResourceListAPIVersion,
	}

	if err := (kio.Pipeline{Inputs: []kio.Reader{r}, Outputs: []kio.Writer{w}}).Execute(); err != nil {
		return nil
	}

	return b.Bytes()
}

func mockConfigmap(args []string) (*yaml.RNode, error) {
	if len(args) == 0 {
		return nil, nil
	}

	data := map[string]string{}
	for _, a := range args {
		split := strings.SplitN(a, "=", 2)
		if len(split) != 2 {
			return nil, fmt.Errorf("invalid config value: %q", a)
		}
		data[split[0]] = split[1]
	}
	node := yaml.NewMapRNode(&data)
	if node == nil {
		return nil, nil
	}

	// create ConfigMap resource to contain function config
	configMap := yaml.MustParse(`
apiVersion: v1
kind: ConfigMap
metadata:
  name: function-input
data: {}
`)
	if err := configMap.PipeE(yaml.SetField("data", node)); err != nil {
		return nil, err
	}
	return configMap, nil
}
