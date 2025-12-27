package porch

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"

	otlpmetrics "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	otlptraces "go.opentelemetry.io/proto/otlp/collector/trace/v1"
)

func TestOtelMetricsPushHTTP(t *testing.T) {
	requestWaitChannel := make(chan struct{})

	ts := httptest.NewServer(&mockHTTPMetricsServer{t: t, ch: requestWaitChannel})
	defer ts.Close()

	t.Setenv("OTEL_METRICS_EXPORTER", "otlp")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", ts.URL)
	t.Setenv("OTEL_EXPORTER_OTLP_PROTOCOL", "http/protobuf")

	ctx, cancel := context.WithCancel(context.Background())

	err := SetupOpenTelemetry(ctx)
	if err != nil {
		t.Errorf("SetupOpenTelemetry() error = %v", err)
		cancel()
	}
	cancel()
	<-requestWaitChannel
}

func TestOtelTracesPushHTTP(t *testing.T) {
	requestWaitChannel := make(chan struct{})

	ts := httptest.NewServer(&mockHTTPTraceServer{t: t, ch: requestWaitChannel})
	defer ts.Close()

	t.Setenv("OTEL_TRACES_EXPORTER", "otlp")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", ts.URL)
	t.Setenv("OTEL_EXPORTER_OTLP_PROTOCOL", "http/protobuf")

	ctx, cancel := context.WithCancel(context.Background())
	err := SetupOpenTelemetry(ctx)
	if err != nil {
		t.Errorf("SetupOpenTelemetry() error = %v", err)
		cancel()
	}
	cancel()
	<-requestWaitChannel
}
func TestSetupOpenTelemetryPrometheusEndpoint(t *testing.T) {
	// Find available port
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("Failed to find available port: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()

	t.Setenv("OTEL_METRICS_EXPORTER", "prometheus")
	t.Setenv("OTEL_EXPORTER_PROMETHEUS_HOST", "localhost")
	t.Setenv("OTEL_EXPORTER_PROMETHEUS_PORT", fmt.Sprintf("%d", port))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = SetupOpenTelemetry(ctx)
	if err != nil {
		t.Fatalf("SetupOpenTelemetry() error = %v", err)
	}

	// Make request to the Prometheus metrics endpoint
	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/metrics", port))
	if err != nil {
		t.Fatalf("Failed to get metrics: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}

	metricsText := string(body)
	if len(strings.TrimSpace(metricsText)) == 0 {
		t.Fatal("Expected metrics output, got empty response")
	}

	// Verify at least one metric is present
	lines := strings.Split(metricsText, "\n")
	metricCount := 0
	for _, line := range lines {
		if strings.HasPrefix(line, "#") || strings.TrimSpace(line) == "" {
			continue
		}
		metricCount++
	}

	if metricCount == 0 {
		t.Fatal("Expected at least one metric endpoint, got none")
	}
}

func TestOtelMetricsPushGRPC(t *testing.T) {
	requestWaitChannel := make(chan struct{})

	lis, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("Failed to listen: %v", err)
	}
	defer lis.Close()

	s := grpc.NewServer()
	otlpmetrics.RegisterMetricsServiceServer(s, &mockMetricsServer{t: t, ch: requestWaitChannel})

	go func() {
		if err := s.Serve(lis); err != nil {
			t.Errorf("Failed to serve: %v", err)
		}
	}()
	defer s.Stop()

	t.Setenv("OTEL_METRICS_EXPORTER", "otlp")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", fmt.Sprintf("http://localhost:%d", lis.Addr().(*net.TCPAddr).Port))
	t.Setenv("OTEL_EXPORTER_OTLP_PROTOCOL", "grpc")

	ctx, cancel := context.WithCancel(context.Background())

	err = SetupOpenTelemetry(ctx)
	if err != nil {
		t.Errorf("SetupOpenTelemetry() error = %v", err)
		cancel()
	}
	cancel()
	<-requestWaitChannel
}

func TestOtelTracesPushGRPC(t *testing.T) {
	requestWaitChannel := make(chan struct{})

	lis, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("Failed to listen: %v", err)
	}
	defer lis.Close()

	s := grpc.NewServer()
	otlptraces.RegisterTraceServiceServer(s, &mockTraceServer{t: t, ch: requestWaitChannel})

	go func() {
		if err := s.Serve(lis); err != nil {
			t.Errorf("Failed to serve: %v", err)
		}
	}()
	defer s.Stop()

	t.Setenv("OTEL_TRACES_EXPORTER", "otlp")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", fmt.Sprintf("http://localhost:%d", lis.Addr().(*net.TCPAddr).Port))
	t.Setenv("OTEL_EXPORTER_OTLP_PROTOCOL", "grpc")

	ctx, cancel := context.WithCancel(context.Background())

	err = SetupOpenTelemetry(ctx)
	if err != nil {
		t.Errorf("SetupOpenTelemetry() error = %v", err)
		cancel()
	}
	cancel()
	<-requestWaitChannel
}

type mockMetricsServer struct {
	otlpmetrics.UnimplementedMetricsServiceServer
	t  *testing.T
	ch chan struct{}
}

func (m *mockMetricsServer) Export(ctx context.Context, req *otlpmetrics.ExportMetricsServiceRequest) (*otlpmetrics.ExportMetricsServiceResponse, error) {
	if len(req.GetResourceMetrics()) < 1 {
		m.t.Errorf("expected at least one resource metric")
	}
	close(m.ch)
	return &otlpmetrics.ExportMetricsServiceResponse{}, nil
}

type mockTraceServer struct {
	otlptraces.UnimplementedTraceServiceServer
	t  *testing.T
	ch chan struct{}
}

func (m *mockTraceServer) Export(ctx context.Context, req *otlptraces.ExportTraceServiceRequest) (*otlptraces.ExportTraceServiceResponse, error) {
	if len(req.GetResourceSpans()) < 1 {
		m.t.Errorf("expected at least one resource span")
	}
	close(m.ch)
	return &otlptraces.ExportTraceServiceResponse{}, nil
}

type mockHTTPMetricsServer struct {
	t  *testing.T
	ch chan struct{}
}

func (m *mockHTTPMetricsServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		m.t.Errorf("error reading request body: %v", err)
		return
	}
	r.Body.Close()

	req := &otlpmetrics.ExportMetricsServiceRequest{}
	proto.Unmarshal(body, req)

	if len(req.GetResourceMetrics()) < 1 {
		m.t.Errorf("expected at least one resource metric")
	}
	close(m.ch)
}

type mockHTTPTraceServer struct {
	t  *testing.T
	ch chan struct{}
}

func (m *mockHTTPTraceServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		m.t.Errorf("error reading request body: %v", err)
		return
	}
	r.Body.Close()

	req := &otlptraces.ExportTraceServiceRequest{}
	proto.Unmarshal(body, req)

	if len(req.GetResourceSpans()) < 1 {
		m.t.Errorf("expected at least one resource span")
	}
	close(m.ch)
}