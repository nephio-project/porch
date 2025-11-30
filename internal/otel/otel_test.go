package porch

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"google.golang.org/protobuf/proto"

	otlpmetrics "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	otlptraces "go.opentelemetry.io/proto/otlp/collector/trace/v1"
)

func TestOtelMetricsPush(t *testing.T) {

	requestWaitChannel := make(chan struct{})

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("error reading request body: %v", err)
			return
		}
		err = r.Body.Close()
		if err != nil {
			t.Errorf("error closing request body: %v", err)

		}
		req := &otlpmetrics.ExportMetricsServiceRequest{}
		proto.Unmarshal(body, req)

		if len(req.GetResourceMetrics()) < 1 {
			t.Errorf("expected at least one resource metric")

		}
		close(requestWaitChannel)
	}))
	t.Setenv("OTEL_METRICS_EXPORTER", "otlp")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", ts.URL)
	t.Setenv("OTEL_EXPORTER_OTLP_PROTOCOL", "http/protobuf")

	ctx, cancel := context.WithCancel(context.Background())

	err := SetupOpenTelemetry(ctx)
	if err != nil {
		t.Errorf("SetupOpenTelemetry() error = %v, wantErr %v", err, false)
		cancel()
	}
	t.Log("Close the Otel collector for force a flush towards the server")
	cancel()
	<-requestWaitChannel
	ts.Close()
}

func TestOtelTracesPush(t *testing.T) {
	requestWaitChannel := make(chan struct{})

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("error reading request body: %v", err)
			return
		}
		err = r.Body.Close()
		if err != nil {
			t.Errorf("error closing request body: %v", err)

		}
		req := &otlptraces.ExportTraceServiceRequest{}
		proto.Unmarshal(body, req)

		if len(req.GetResourceSpans()) < 1 {
			t.Errorf("expected at least one resource span")

		}
		close(requestWaitChannel)

	}))
	os.Setenv("OTEL_TRACES_EXPORTER", "otlp")
	os.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", ts.URL)
	os.Setenv("OTEL_EXPORTER_OTLP_PROTOCOL", "http/protobuf")

	ctx, cancel := context.WithCancel(context.Background())
	err := SetupOpenTelemetry(ctx)
	if err != nil {
		t.Errorf("SetupOpenTelemetry() error = %v, wantErr %v", err, false)
		cancel()
		return
	}
	t.Log("Close the Otel collector for force a flush towards the server")
	cancel()
	<-requestWaitChannel
	ts.Close()
}
