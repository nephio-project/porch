package internal

import (
	"context"
	"net"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	pb "github.com/nephio-project/porch/func/evaluator"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
	go func() {
		err := server.Serve(lis)
		if err != nil {
			t.Errorf("fake grpc server returned with an error: %v", err)
		}
	}()

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

	conn, err := grpc.NewClient(
		addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithNoProxy(),
	)
	if err != nil {
		t.Fatalf("grpc dial failed: %v", err)
	}

	reqCh := make(chan *connectionRequest, 2)
	go func() {
		lock := &sync.Mutex{}
		counter := &atomic.Int32{}
		for req := range reqCh {
			req.responseCh <- &connectionResponse{
				podData: podData{
					image:          req.image,
					grpcConnection: conn,
					podKey:         client.ObjectKey{},
				},
				fnEvaluationMutex:     lock,
				concurrentEvaluations: counter,
				err:                   nil,
			}
		}
	}()

	pe := &podEvaluator{
		requestCh: reqCh,
	}

	req := &pb.EvaluateFunctionRequest{ResourceList: []byte(`{}`), Image: "test-image"}
	var wg sync.WaitGroup
	funcCallCount := 3
	durations := make([]time.Duration, funcCallCount)
	wg.Add(funcCallCount)
	start := time.Now()
	for i := range funcCallCount {
		go func(i int) {
			defer wg.Done()
			_, err := pe.EvaluateFunction(context.Background(), req)
			if err != nil {
				t.Errorf("ERROR: EvaluateFunction[%d]: %v", i, err)
			}
			durations[i] = time.Since(start)
		}(i)
	}
	wg.Wait()

	t.Logf("durations: %v", durations)

	slices.Sort(durations)
	if durations[funcCallCount-1] < time.Duration(funcCallCount)*sleep {
		t.Errorf("expected serial but slowest took only %v", durations[funcCallCount-1])
	}
}
