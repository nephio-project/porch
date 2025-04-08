package performance

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"reflect"

	porchapi "github.com/nephio-project/porch/api/porch/v1alpha1"
	configapi "github.com/nephio-project/porch/api/porchconfig/v1alpha1"
	. "github.com/nephio-project/porch/test/e2e"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	metricsPort = flag.Uint("port", 2113, "Port on which to expose metrics")
)

type hasName interface {
	GetName() string
}

func kindOf(o any) string {
	if c, ok := o.(client.Object); ok {
		if kind := c.GetObjectKind().GroupVersionKind().Kind; kind != "" {
			return kind
		}
	}
	return reflect.TypeOf(o).Elem().Name()
}

func nameOf(o any) string {
	switch c := o.(type) {
	case *porchapi.PackageRevision:
		return c.Spec.PackageName
	case *porchapi.PackageRevisionResources:
		return c.Spec.PackageName
	case *configapi.Repository:
		return c.Name
	case hasName:
		return c.GetName()
	default:
		return ""
	}
}

func Record(op Operation, obj any, fn func()) {
	time := Measure(func() { fn() })
	recordMetric(kindOf(obj), op, nameOf(obj), time.Milliseconds())
}

type PerformanceSuite struct {
	TestSuiteWithGit

	metricsCtx context.Context
}

func (t *PerformanceSuite) SetupSuite() {
	t.TestSuiteWithGit.SetupSuite()
	t.ServeMetrics()
}

func (t *PerformanceSuite) ServeMetrics() {
	go func() {
		t.Logf("Starting metrics server")
		http.Handle("/metrics", promhttp.Handler())
		if err := http.ListenAndServe(fmt.Sprintf(":%d", *metricsPort), nil); err != nil {
			t.Fatalf("Error starting metrics server: %v", err)
		}
		t.Logf("Metrics server stopped")
	}()
}

func (t *PerformanceSuite) incrementGage(obj client.Object) {
	if !t.T().Failed() {
		switch kindOf(obj) {
		case KindPackageRevision:
			packageRevisionGuage.Inc()
		case KindRepository:
			repositoryGauge.Inc()
		}
	}
}

func (t *PerformanceSuite) decrementGage(obj client.Object) {
	if !t.T().Failed() {
		switch kindOf(obj) {
		case KindPackageRevision:
			packageRevisionGuage.Dec()
		case KindRepository:
			repositoryGauge.Dec()
		}
	}
}

func (t *PerformanceSuite) GetE(key client.ObjectKey, obj client.Object) {
	t.T().Helper()
	Record(OperationGet, obj, func() { t.TestSuiteWithGit.GetE(key, obj) })
}

func (t *PerformanceSuite) GetF(key client.ObjectKey, obj client.Object) {
	t.T().Helper()
	Record(OperationGet, obj, func() { t.TestSuiteWithGit.GetF(key, obj) })
}

func (t *PerformanceSuite) ListE(list client.ObjectList, opts ...client.ListOption) {
	t.T().Helper()
	Record(OperationList, list, func() { t.TestSuiteWithGit.ListE(list, opts...) })
}

func (t *PerformanceSuite) ListF(list client.ObjectList, opts ...client.ListOption) {
	t.T().Helper()
	Record(OperationList, list, func() { t.TestSuiteWithGit.ListF(list, opts...) })
}

func (t *PerformanceSuite) CreateF(obj client.Object, opts ...client.CreateOption) {
	t.T().Helper()
	Record(OperationCreate, obj, func() { t.TestSuiteWithGit.CreateF(obj, opts...) })

	t.incrementGage(obj)
}

func (t *PerformanceSuite) CreateE(obj client.Object, opts ...client.CreateOption) {
	t.T().Helper()
	Record(OperationCreate, obj, func() { t.TestSuiteWithGit.CreateE(obj, opts...) })

	t.incrementGage(obj)
}

func (t *PerformanceSuite) DeleteF(obj client.Object, opts ...client.DeleteOption) {
	t.T().Helper()
	Record(OperationDelete, obj, func() { t.TestSuiteWithGit.DeleteF(obj, opts...) })

	t.decrementGage(obj)
}

func (t *PerformanceSuite) DeleteE(obj client.Object, opts ...client.DeleteOption) {
	t.T().Helper()
	Record(OperationDelete, obj, func() { t.TestSuiteWithGit.DeleteE(obj, opts...) })

	t.decrementGage(obj)
}

func (t *PerformanceSuite) DeleteL(obj client.Object, opts ...client.DeleteOption) {
	t.T().Helper()
	Record(OperationDelete, obj, func() { t.TestSuiteWithGit.DeleteL(obj, opts...) })

	t.decrementGage(obj)
}

func (t *PerformanceSuite) UpdateF(obj client.Object, opts ...client.UpdateOption) {
	t.T().Helper()
	Record(OperationUpdate, obj, func() { t.TestSuiteWithGit.UpdateF(obj, opts...) })
}

func (t *PerformanceSuite) UpdateE(obj client.Object, opts ...client.UpdateOption) {
	t.T().Helper()
	Record(OperationUpdate, obj, func() { t.TestSuiteWithGit.UpdateE(obj, opts...) })
}

func (t *PerformanceSuite) PatchF(obj client.Object, patch client.Patch, opts ...client.PatchOption) {
	t.T().Helper()
	Record(OperationPatch, obj, func() { t.TestSuiteWithGit.PatchF(obj, patch, opts...) })
}

func (t *PerformanceSuite) PatchE(obj client.Object, patch client.Patch, opts ...client.PatchOption) {
	t.T().Helper()
	Record(OperationPatch, obj, func() { t.TestSuiteWithGit.PatchE(obj, patch, opts...) })
}

func (t *PerformanceSuite) UpdateApprovalL(pr *porchapi.PackageRevision, opts metav1.UpdateOptions) *porchapi.PackageRevision {
	t.T().Helper()
	var ret *porchapi.PackageRevision
	Record(OperationUpdateApproval, pr, func() { ret = t.TestSuiteWithGit.UpdateApprovalL(pr, opts) })
	return ret
}

func (t *PerformanceSuite) UpdateApprovalF(pr *porchapi.PackageRevision, opts metav1.UpdateOptions) *porchapi.PackageRevision {
	t.T().Helper()
	var ret *porchapi.PackageRevision
	Record(OperationUpdateApproval, pr, func() { ret = t.TestSuiteWithGit.UpdateApprovalL(pr, opts) })
	return ret
}

// copied, so the operation is recorded
func (t *PerformanceSuite) CreatePackageDraftF(repository, packageName, workspace string) *porchapi.PackageRevision {
	t.T().Helper()
	pr := t.CreatePackageSkeleton(repository, packageName, workspace)
	pr.Spec.Tasks = []porchapi.Task{
		{
			Type: porchapi.TaskTypeInit,
			Init: &porchapi.PackageInitTaskSpec{},
		},
	}
	t.CreateF(pr)
	return pr
}
