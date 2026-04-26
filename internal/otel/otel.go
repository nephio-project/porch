// Copyright 2026 The kpt and Nephio Authors
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

package porch

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	prombridge "go.opentelemetry.io/contrib/bridges/prometheus"
	"go.opentelemetry.io/contrib/exporters/autoexport"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/contrib/propagators/autoprop"
	"go.opentelemetry.io/otel"
	otelprometheus "go.opentelemetry.io/otel/exporters/prometheus"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/trace"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"k8s.io/klog/v2"
	controllerruntimemetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	metricsSetUp   bool
	metricsHandler http.Handler
)

func IsMetricsSetUp() bool {
	return metricsSetUp
}

func MetricsHandler() http.Handler {
	return metricsHandler
}

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
	http.DefaultTransport = otelhttp.NewTransport(http.DefaultTransport)
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
		if err := tp.Shutdown(context.Background()); err != nil {
			panic(err)
		}
	}()
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(autoprop.NewTextMapPropagator())

	return nil
}

func setupMetrics(ctx context.Context) error {
	exporter := os.Getenv("OTEL_METRICS_EXPORTER")

	autoexport.WithFallbackMetricProducer(func(ctx context.Context) (sdkmetric.Producer, error) {
		return prombridge.NewMetricProducer(
			prombridge.WithGatherer(prometheus.Gatherers{
				prometheus.DefaultGatherer,
				controllerruntimemetrics.Registry,
			}),
		), nil
	})

	if exporter == "prometheus" {
		promExp, err := otelprometheus.New(
			otelprometheus.WithRegisterer(prometheus.DefaultRegisterer),
		)
		if err != nil {
			return fmt.Errorf("failed to create prometheus exporter: %w", err)
		}

		mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(promExp))
		go func() {
			<-ctx.Done()
			if err := mp.Shutdown(context.Background()); err != nil {
				klog.Warningf("metrics provider shutdown error: %v", err)
			}
		}()
		otel.SetMeterProvider(mp)

		gatherers := prometheus.Gatherers{
			prometheus.DefaultGatherer,
			controllerruntimemetrics.Registry,
		}
		metricsHandler = promhttp.HandlerFor(gatherers, promhttp.HandlerOpts{
			ErrorHandling: promhttp.ContinueOnError,
		})
	} else {
		promExp, err := otelprometheus.New(
			otelprometheus.WithRegisterer(prometheus.DefaultRegisterer),
		)
		if err != nil {
			return fmt.Errorf("failed to create prometheus exporter: %w", err)
		}

		autoMr, err := autoexport.NewMetricReader(ctx)
		if err != nil {
			return fmt.Errorf("failed to create metric reader: %w", err)
		}
		go func() {
			<-ctx.Done()
			if err := autoMr.Shutdown(context.Background()); err != nil {
				klog.Warningf("metrics reader shutdown error: %v", err)
			}
		}()

		mp := sdkmetric.NewMeterProvider(
			sdkmetric.WithReader(promExp),
			sdkmetric.WithReader(autoMr),
		)
		go func() {
			<-ctx.Done()
			if err := mp.Shutdown(context.Background()); err != nil {
				klog.Warningf("metrics provider shutdown error: %v", err)
			}
		}()
		otel.SetMeterProvider(mp)

		gatherers := prometheus.Gatherers{
			prometheus.DefaultGatherer,
			controllerruntimemetrics.Registry,
		}
		metricsHandler = promhttp.HandlerFor(gatherers, promhttp.HandlerOpts{
			ErrorHandling: promhttp.ContinueOnError,
		})
	}

	metricsSetUp = true
	return nil
}
