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

func TestSetupOpenTelemetry(t *testing.T) {
	tests := []struct {
		name                string
		exporterType        string
		protocol            string
		endpoint            string
		metricsExporterType string
		metricsProtocol     string
		prometheusHost      string
		prometheusPort      string
		wantError           bool
	}{
		{
			name:                "Valid OTEL exporter with grpc protocol and endpoint",
			exporterType:        "otlp",
			protocol:            "grpc",
			endpoint:            "localhost:4317",
			metricsExporterType: "none",
			metricsProtocol:     "",
			prometheusHost:      "",
			prometheusPort:      "",
			wantError:           false,
		},
		{
			name:                "Valid OTEL exporter with http/protobuf protocol and endpoint",
			exporterType:        "otlp",
			protocol:            "http/protobuf",
			endpoint:            "http://localhost:4317",
			metricsExporterType: "otlp",
			metricsProtocol:     "",
			prometheusHost:      "",
			prometheusPort:      "",
			wantError:           false,
		},
		{
			name:                "Invalid OTEL exporter with invalid protocol",
			exporterType:        "otlp",
			protocol:            "invalid",
			endpoint:            "http://localhost:4317",
			metricsExporterType: "otlp",
			metricsProtocol:     "",
			prometheusHost:      "",
			prometheusPort:      "",
			wantError:           true,
		},
		{
			name:                "Invalid OTEL exporter with invalid endpoint",
			exporterType:        "otlp",
			protocol:            "grpc",
			endpoint:            "invalid",
			metricsExporterType: "",
			metricsProtocol:     "",
			prometheusHost:      "",
			prometheusPort:      "",
			wantError:           true,
		},
		{
			name:                "Invalid OTEL exporter with invalid metrics exporter type",
			exporterType:        "otlp",
			protocol:            "grpc",
			endpoint:            "localhost:4317",
			metricsExporterType: "",
			metricsProtocol:     "",
			prometheusHost:      "",
			prometheusPort:      "",
			wantError:           true,
		},
		{
			name:                "Invalid OTEL exporter with invalid metrics protocol",
			exporterType:        "otlp",
			protocol:            "grpc",
			endpoint:            "localhost:4317",
			metricsExporterType: "prometheus",
			metricsProtocol:     "invalid",
			prometheusHost:      "",
			prometheusPort:      "",
			wantError:           true,
		},
		{
			name:                "Invalid OTEL exporter with invalid prometheus host",
			exporterType:        "otlp",
			protocol:            "grpc",
			endpoint:            "localhost:4317",
			metricsExporterType: "",
			metricsProtocol:     "",
			prometheusHost:      "",
			prometheusPort:      "",
			wantError:           true,
		},
		{
			name:         "Valid OTEL exporter with console export",
			exporterType: "console",
			wantError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set the environment variables
			if tt.exporterType != "" {
				os.Setenv("OTEL_TRACES_EXPORTER", tt.exporterType)
			}
			if tt.protocol != "" {
				os.Setenv("OTEL_EXPORTER_OTLP_PROTOCOL", tt.protocol)
			}
			if tt.endpoint != "" {
				os.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", tt.endpoint)
			}
			if tt.metricsExporterType != "" {
				os.Setenv("OTEL_METRICS_EXPORTER", tt.metricsExporterType)
			}
			if tt.metricsProtocol != "" {
				os.Setenv("OTEL_EXPORTER_OTLP_METRICS_PROTOCOL", tt.metricsProtocol)
			}
			if tt.prometheusHost != "" {
				os.Setenv("OTEL_EXPORTER_PROMETHEUS_HOST", tt.prometheusHost)
			}
			if tt.prometheusPort != "" {
				os.Setenv("OTEL_EXPORTER_PROMETHEUS_PORT", tt.prometheusPort)
			}

			// Call the SetupOpenTelemetry function
			ctx, cancel := context.WithCancel(context.Background())
			err := SetupOpenTelemetry(ctx)
			cancel()

			// Check if the error is as expected
			if (err != nil) != tt.wantError {
				t.Errorf("SetupOpenTelemetry() error = %v, wantErr %v", err, tt.wantError)
				return
			}
		})
	}
}

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
	cancel()
	<-requestWaitChannel
	ts.Close()
}
