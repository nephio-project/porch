package benchmark

import (
	"context"
	"testing"

	e2e "github.com/nephio-project/porch/test/e2e"
)

// This ugliness is needed so the e2e test suite setup can be reused.
type BenchmarkSuite struct {
	//Inherit the test setup from the e2e test suite for now. Can change to something more production-ready like ginkgo later.
	e2e.TestSuiteWithGit
}

func TestBenchmark(t *testing.T) {
	e2e.RunSuite(&BenchmarkSuite{}, t)
}

func (t *BenchmarkSuite) TestCreateScenario(ctx context.Context) {

	t.Logf("Testrun")
}
