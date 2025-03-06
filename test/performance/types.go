
package metrics

import (
    "time"
)

// OperationMetrics holds metrics for a single operation
type OperationMetrics struct {
    Operation string
    Duration  time.Duration
    Error     error
}

// TestMetrics holds metrics for a test iteration
type TestMetrics struct {
    RepoName string
    PkgName  string
    Metrics  []OperationMetrics
}

// Stats holds statistics for operations
type Stats struct {
    Min   time.Duration
    Max   time.Duration
    Total time.Duration
    Count int
}
