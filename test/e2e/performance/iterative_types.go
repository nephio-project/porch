package performance

import (
	"fmt"
	"time"
)

type MyDuration time.Duration

func (d MyDuration) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf("%.4f", d.Milliseconds())), nil
}

func (d MyDuration) Milliseconds() float64 {
	return float64(d) / 1_000_000
}

func Measure(f func()) MyDuration {
	start := time.Now()
	f()
	return MyDuration(time.Since(start))
}

func AvgDuration(total MyDuration, n int) MyDuration {
	return MyDuration(int64(total) / int64(n))
}

type IterationMetricsData struct {
	List,
	Create,
	UpdateResources,
	GetAfterResourceUpdate,
	Propose,
	GetAfterPropose,
	Approve,
	GetAfterPublish,
	DeleteProposed,
	GetAfterProposeDelete,
	Delete MyDuration
}

type FullMetricsData struct {
	ControlRevisionCount int

	CreateControlRevisionsTotal,
	CreateControlRevisionsAvg,
	DeleteControlRevisionsTotal,
	DeleteControlRevisionsAvg MyDuration

	IterationMetricsData `json:",inline"`

	//ServerMemoryBytes int64
	//ControllersMemoryBytes int64
	//FuncRunnerMemoryBytes  int64
	//
	//ServerAvgCPULoad      float32
	//ControllersAvgCPULoad float32
	//FuncRunnerAvgCPULoad  float32
}
