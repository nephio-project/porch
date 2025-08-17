package porch

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"go.opentelemetry.io/contrib/exporters/autoexport"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/contrib/propagators/autoprop"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/trace"
	"k8s.io/klog/v2"
)

// Sets up OpenTelemetry with parameters
// from environment variables based on the
// opentelemetry.io/contrib/exporters/autoexport"
func SetupOpenTelemetry(ctx context.Context) error {
	setupTiming := time.Now()
	err := setupTracing(ctx)
	if err != nil {
		return err
	}
	err = setupMetrics(ctx)
	if err != nil {
		return err
	}
	http.DefaultTransport = otelhttp.NewTransport(http.DefaultClient.Transport)
	http.DefaultClient.Transport = http.DefaultTransport
	klog.Infof("OpenTelemetry initialized in %s", time.Since(setupTiming))
	return nil

}

func setupTracing(ctx context.Context) error {
	exp, err := autoexport.NewSpanExporter(ctx)
	if err != nil {
		return fmt.Errorf("failed to create span exporter: %w", err)
	}
	tp := trace.NewTracerProvider(trace.WithBatcher(exp))
	go func() {
		<-ctx.Done()
		if err := tp.Shutdown(ctx); err != nil {
			panic(err)
		}
	}()
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(autoprop.NewTextMapPropagator())

	return nil
}

func setupMetrics(ctx context.Context) error {
	mr, err := autoexport.NewMetricReader(ctx)
	if err != nil {
		return fmt.Errorf("failed to create metric reader: %w", err)
	}
	go func() {
		<-ctx.Done()
		if err := mr.Shutdown(ctx); err != nil {
			panic(err)
		}
	}()
	mp := metric.NewMeterProvider(metric.WithReader(mr))
	otel.SetMeterProvider(mp)

	return nil
}
