package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// Operation duration metrics
	operationDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "porch_operation_duration_seconds",
			Help:    "Duration of Porch operations in seconds",
			Buckets: []float64{0.1, 0.5, 1, 2, 5, 10, 30, 60},
		},
		[]string{"operation", "repository", "package", "status"},
	)

	// Operation counter metrics
	operationCounter = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "porch_operations_total",
			Help: "Total number of Porch operations",
		},
		[]string{"operation", "repository", "package", "status"},
	)

	// Repository metrics
	repositoryCounter = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "porch_repositories_created_total",
			Help: "Total number of repositories created",
		},
	)

	// Package metrics
	packageCounter = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "porch_packages_created_total",
			Help: "Total number of packages created",
		},
	)
)

// recordMetric records both duration and count for an operation
func recordMetric(operation, repoName, pkgName string, duration float64, err error) {
	status := "success"
	if err != nil {
		status = "error"
	}

	operationDuration.WithLabelValues(operation, repoName, pkgName, status).Observe(duration)
	operationCounter.WithLabelValues(operation, repoName, pkgName, status).Inc()
}
