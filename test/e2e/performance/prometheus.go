package performance

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type Operation string

const (
	OperationGet            Operation = "get"
	OperationCreate         Operation = "create"
	OperationUpdate         Operation = "update"
	OperationUpdateApproval Operation = "update-approval"
	OperationPatch          Operation = "patch"
	OperationPropose        Operation = "propose"
	OperationPublish        Operation = "publish"
	OperationProposeDelete  Operation = "propose-delete"
	OperationDelete         Operation = "delete"
	OperationList           Operation = "list"
)

const (
	LabelKind      = "kind"
	LabelOperation = "operation"
	LabelName      = "name"
)

var labels = []string{LabelKind, LabelOperation, LabelName}

var (
	KindPackageRevision          = "PackageRevision"
	KindPackageRevisionResources = "PackageRevisionResources"
	KindRepository               = "Repository"
)

var (
	// Operation duration metrics
	operationDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "porch_operation_duration_ms",
			Help: "Duration of Porch operations in milliseconds",
			// Buckets: []float64{math.Inf(1)},
			Buckets: []float64{10, 25, 50, 100, 250},
		},
		labels,
	)

	// Operation counter metrics
	operationCounter = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "porch_operations_total",
			Help: "Total number of Porch operations",
		},
		labels,
	)

	// How many repositories there are
	repositoryGauge = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "porch_repositories_count",
			Help: "Total number of repositories present",
		},
	)

	// How many package revisions there are
	packageRevisionGuage = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "porch_package_revisions_count",
			Help: "Total number of package revisions present",
		},
	)
)

// RecordMetric records both duration and count for an operation
func RecordMetric(kind string, operation Operation, name string, duration float64) {
	operationDuration.WithLabelValues(kind, string(operation), name).Observe(duration)
	operationCounter.WithLabelValues(kind, string(operation), name).Inc()
}

func MeasureAndRecord(op Operation, obj any, fn func()) {
	time := Measure(func() { fn() })
	RecordMetric(KindOf(obj), op, NameOf(obj), time.Milliseconds())
}
