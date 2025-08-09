package internal

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	pb "github.com/nephio-project/porch/func/evaluator"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// fakeFnEvalServer delays every EvaluateFunction call.
type fakeFnEvalServer struct {
	pb.UnimplementedFunctionEvaluatorServer
	delay time.Duration
}

func (s *fakeFnEvalServer) EvaluateFunction(_ context.Context, _ *pb.EvaluateFunctionRequest) (*pb.EvaluateFunctionResponse, error) {
	time.Sleep(s.delay)
	return &pb.EvaluateFunctionResponse{ResourceList: []byte("ok"), Log: []byte("log")}, nil
}

func startFakeServer(ctx context.Context, t *testing.T, delay time.Duration) (string, error) {
	lis, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
		return "", err
	}

	addr := lis.Addr().String()
	server := grpc.NewServer()
	pb.RegisterFunctionEvaluatorServer(server, &fakeFnEvalServer{delay: delay})
	go server.Serve(lis)

	go func() {
		<-ctx.Done()
		server.GracefulStop()
		lis.Close()
	}()

	return addr, nil
}

func TestPodEvaluatorExecutionSerial(t *testing.T) {
	const sleep = 2 * time.Second

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	addr, err := startFakeServer(ctx, t, sleep)
	if err != nil {
		t.Fatalf("failed to start fake server: %v", err)
	}

	conn, err := grpc.Dial(
		addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("grpc dial failed: %v", err)
	}

	client := &podAndGRPCClient{grpcClient: conn}
	reqCh := make(chan *clientConnRequest, 2)
	go func() {
		for req := range reqCh {
			req.grpcClientCh <- &clientConnAndError{podClient: client, err: nil}
		}
	}()

	pe := &podEvaluator{
		requestCh: reqCh,
	}

	req := &pb.EvaluateFunctionRequest{ResourceList: []byte(`[]`), Image: "test-image"}
	var wg sync.WaitGroup
	durations := make([]time.Duration, 2)
	wg.Add(2)
	for i := 0; i < 2; i++ {
		go func(i int) {
			defer wg.Done()
			start := time.Now()
			_, err := pe.EvaluateFunction(context.Background(), req)
			if err != nil {
				t.Errorf("EvaluateFunction[%d] error: %v", i, err)
			}
			durations[i] = time.Since(start)
		}(i)
	}
	wg.Wait()

	t.Logf("durations: first=%v, second=%v", durations[0], durations[1])
	if durations[0] > durations[1] {
		durations[0], durations[1] = durations[1], durations[0]
	}

	if durations[1] < 2*sleep {
		t.Errorf("expected serial but second took only %v", durations[1])
	}
}
