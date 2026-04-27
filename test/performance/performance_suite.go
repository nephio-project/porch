// Copyright 2026 The Nephio Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package metrics

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	porchclient "github.com/nephio-project/porch/api/generated/clientset/versioned"
	porchapi "github.com/nephio-project/porch/api/porch/v1alpha1"
	configapi "github.com/nephio-project/porch/api/porchconfig/v1alpha1"
	metrics "github.com/nephio-project/porch/internal/metrics"
	porchotel "github.com/nephio-project/porch/internal/otel"
	pkgerrors "github.com/pkg/errors"
	"github.com/stretchr/testify/suite"
	coreapi "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

const (
	prometheusPort = 9095
	giteaUsername  = "nephio"
	giteaPassword  = "secret"
)

var (
	scheme = runtime.NewScheme()

	namespace          = flag.String("namespace", "porch-metrics", "Kubernetes namespace to use for the test")
	numRepos           = flag.Int("repos", 1, "Number of repositories to create")
	numPackages        = flag.Int("packages", 5, "Number of packages per repository")
	numRevisions       = flag.Int("revisions", 5, "Number of package revisions per package")
	repoParallelism    = flag.Int("repo-parallelism", 1, "Number of repositories to create in parallel")
	packageParallelism = flag.Int("package-parallelism", 1, "Number of packages to create in parallel per repository")
	errorRate          = flag.Float64("error-rate", 0.1, "Maximum percentage of package revisions allowed to fail lifecycle transition")
	enableDeletion     = flag.Bool("enable-deletion", false, "Enable deletion of package revisions at the end of the test")
	enablePrometheus   = flag.Bool("enable-prometheus", false, "Enable Prometheus metrics server on port 9091")
	prrPaddingInMB     = flag.Int("prr-padding-mb", 0, "Amount of padding data in MB to add to each PackageRevisionResource to increase its size and test performance with larger resources")

	metricsLogFile       = flag.String("metrics-log-prefix", "porch-metrics", "Prefix for the timestamped metrics log file")
	resultsFile          = flag.String("results-file", "load_test_results.txt", "File name for test results")
	fullLogFile          = flag.String("detailed-log-file", "load_test.log", "File name for detailed log")
	lifecycleCSV         = flag.String("repo-results-csv", "load_test_lifecycle_results.csv", "File name for repository results CSV")
	operationsCSV        = flag.String("operations-csv", "load_test_operations_results.csv", "File name for operations details CSV")
	deletionCSV          = flag.String("deletion-csv", "load_test_deletion_results.csv", "File name for deletion operations CSV")
	kptfilePath          = flag.String("kptfile-path", "resources/Kptfile", "Path to the Kptfile")
	packageResourcesPath = flag.String("package-resources-path", "resources/deployment.yaml", "Path to the package resources")

	retryBackoff = wait.Backoff{
		Duration: 50 * time.Millisecond,
		Steps:    100,
		Factor:   1.25,
		Cap:      30 * time.Second,
	}
)

type PerfTestSuite struct {
	suite.Suite
	ctx       context.Context
	cancelCtx context.CancelFunc
	client    client.Client
	clientSet porchclient.Interface

	testLogger       *TestLogger
	resultsLogger    *ResultsLogger
	otelResources    *porchotel.OTelResources
	enablePrometheus bool

	metrics      map[string]TestMetrics
	metricsMutex sync.RWMutex

	testOptions TestOptions
	logOptions  LogOptions
	csvOptions  CSVOptions
}

type TestOptions struct {
	namespace            string
	numRepos             int
	numPkgs              int
	numRevs              int
	repoParallelism      int
	packageParallelism   int
	errorRate            float64
	enableDeletion       bool
	kptfilePath          string
	packageResourcesPath string
	krmFnRegistryURL     string
	prrPaddingInMB       int
}

type LogOptions struct {
	metricsLogFile string
	resultsFile    string
	fullLogFile    string
}

type CSVOptions struct {
	lifecycleCSV  string
	operationsCSV string
	deletionCSV   string
}

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(porchapi.AddToScheme(scheme))
	utilruntime.Must(configapi.AddToScheme(scheme))
}

func (t *PerfTestSuite) recordRepoMetric(repoName, opKey string, op OperationMetrics) {
	t.metricsMutex.Lock()
	defer t.metricsMutex.Unlock()
	t.metrics[repoName].repoOps[opKey] = op
}

func (t *PerfTestSuite) recordPkgRevMetric(repoName, pkgName string, revisionNum int, opKey string, op OperationMetrics) {
	t.metricsMutex.Lock()
	defer t.metricsMutex.Unlock()
	t.metrics[repoName].pkgRevMetrics[pkgName][revisionNum].Metrics[opKey] = op
}

func (t *PerfTestSuite) initPkgRevMetrics(repoName, pkgName string, revisionNum int) {
	t.metricsMutex.Lock()
	defer t.metricsMutex.Unlock()
	t.metrics[repoName].pkgRevMetrics[pkgName][revisionNum] = PackageRevisionMetrics{
		pkgName:  pkgName,
		Revision: revisionNum,
		Metrics:  make(map[string]OperationMetrics),
	}
}

func getEnvWithDefault(key, defaultValue string) string {
	_ = godotenv.Load(filepath.Join("..", "..", ".env"))
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func (t *PerfTestSuite) SetupSuite() {
	if os.Getenv("LOAD_TEST") != "1" && os.Getenv("MAX_PR_TEST=1") != "1" {
		t.T().Skipf("Skipping performance tests in non-load test environment")
	}

	flag.Parse()

	t.metrics = make(map[string]TestMetrics)
	t.cancelOnSignal()
	t.testOptions = TestOptions{
		namespace:            *namespace,
		numRepos:             *numRepos,
		numPkgs:              *numPackages,
		numRevs:              *numRevisions,
		repoParallelism:      *repoParallelism,
		packageParallelism:   *packageParallelism,
		errorRate:            *errorRate,
		enableDeletion:       *enableDeletion,
		kptfilePath:          *kptfilePath,
		packageResourcesPath: *packageResourcesPath,
		krmFnRegistryURL:     getEnvWithDefault("KRM_FN_REGISTRY_URL", "ghcr.io/kptdev/krm-functions-catalog"),
		prrPaddingInMB:       *prrPaddingInMB,
	}

	t.logOptions = LogOptions{
		metricsLogFile: *metricsLogFile,
		resultsFile:    *resultsFile,
		fullLogFile:    *fullLogFile,
	}

	t.csvOptions = CSVOptions{
		lifecycleCSV:  *lifecycleCSV,
		operationsCSV: *operationsCSV,
		deletionCSV:   *deletionCSV,
	}

	logger, err := t.NewTestLogger(t.logOptions.metricsLogFile)
	if err != nil {
		t.T().Fatalf("Failed to create logger: %v", err)
	}

	resultsLogger, err := t.NewResultsLogger(t.logOptions.resultsFile, t.logOptions.fullLogFile)
	if err != nil {
		t.T().Fatalf("Failed to create results logger: %v", err)
	}

	cfg, err := config.GetConfig()
	if err != nil {
		t.T().Fatalf("Failed to get config: %v", err)
	}

	c, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		t.T().Fatalf("Failed to create client: %v", err)
	}

	clientSet, err := porchclient.NewForConfig(cfg)
	if err != nil {
		t.T().Fatalf("Failed to create Porch clientset: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.ctx = ctx
	t.cancelCtx = cancel
	t.client = c
	t.clientSet = clientSet
	t.testLogger = logger
	t.resultsLogger = resultsLogger
	t.enablePrometheus = *enablePrometheus

	if t.enablePrometheus {
		os.Setenv("OTEL_EXPORTER_PROMETHEUS_PORT", fmt.Sprintf("%d", prometheusPort))
		os.Setenv("OTEL_METRICS_EXPORTER", "prometheus")
		os.Setenv("OTEL_TRACES_EXPORTER", "none")
		var err error
		t.otelResources, err = porchotel.SetupOpenTelemetry(ctx)
		if err != nil {
			t.T().Fatalf("Failed to setup OpenTelemetry: %v", err)
		}
		t.T().Logf("OTel metrics server started on port %v", prometheusPort)
		metrics.PerfTestSetTestRunInfo("porch-performance-test", t.testOptions.namespace, time.Now())
	}

	t.T().Logf("  Running load test with:")
	t.T().Logf("  Namespace: %s", t.testOptions.namespace)
	t.T().Logf("  %d repositories", t.testOptions.numRepos)
	t.T().Logf("  %d packages per repository", t.testOptions.numPkgs)
	t.T().Logf("  %d revisions per package", t.testOptions.numRevs)
	t.T().Logf("  Prometheus metrics: %v", t.enablePrometheus)

	if err = t.setupNamespaceAndSecret(); err != nil {
		t.T().Fatalf("failed to setup namespace and secret: %v", err)
	}
	t.T().Logf("Created namespace %s and gitea secret", t.testOptions.namespace)

	t.T().Log("\n=== Cleaning up existing resources from previous runs ===")
	if err = t.cleanupExistingResources(); err != nil {
		t.T().Logf("Warning: Failed to cleanup existing resources: %v", err)
	}
	t.T().Log("Cleanup complete, ready to start test")
}

func (t *PerfTestSuite) TearDownSuite() {
	if t.cancelCtx != nil {
		t.cancelCtx()
	}
	if t.otelResources != nil {
		if err := t.otelResources.Flush(); err != nil {
			t.T().Logf("Warning: Failed to flush metrics: %v", err)
		}
		t.T().Logf("Waiting 20 seconds before shutting down metrics server to ensure final scrapes complete...")
		time.Sleep(20 * time.Second)

		if err := t.otelResources.ShutdownWithTimeout(5 * time.Second); err != nil {
			t.T().Logf("Warning: Failed to shutdown OpenTelemetry: %v", err)
		}
	}
	if t.testLogger != nil {
		if err := t.testLogger.Close(); err != nil {
			t.T().Logf("Warning: Failed to close test logger: %v", err)
		}
	}
	if t.resultsLogger != nil {
		if err := t.resultsLogger.Close(); err != nil {
			t.T().Logf("Warning: Failed to close results logger: %v", err)
		}
	}
}

func (t *PerfTestSuite) cancelOnSignal() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		sig := <-sigChan
		t.T().Logf("\nReceived signal %v, cancelling test context...", sig)
		if t.cancelCtx != nil {
			t.cancelCtx()
		}
	}()
}

func (t *PerfTestSuite) setupNamespaceAndSecret() error {
	ns := &coreapi.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: t.testOptions.namespace,
		},
	}

	err := t.client.Create(t.ctx, ns)
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return pkgerrors.Wrapf(err, "failed to create namespace %s", t.testOptions.namespace)
	}

	secret := &coreapi.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gitea",
			Namespace: t.testOptions.namespace,
		},
		Type: coreapi.SecretTypeBasicAuth,
		StringData: map[string]string{
			"username": giteaUsername,
			"password": giteaPassword,
		},
	}

	err = t.client.Create(t.ctx, secret)
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return pkgerrors.Wrapf(err, "failed to create gitea secret in namespace %s", t.testOptions.namespace)
	}

	return nil
}

func (t *PerfTestSuite) cleanupExistingResources() error {
	var repoList configapi.RepositoryList
	if err := t.client.List(t.ctx, &repoList, client.InNamespace(t.testOptions.namespace)); err != nil {
		if !apierrors.IsNotFound(err) {
			return pkgerrors.Wrap(err, "failed to list repositories")
		}
	} else {
		for _, repo := range repoList.Items {
			if err := t.client.Delete(t.ctx, &repo); err != nil {
				if !apierrors.IsNotFound(err) {
					t.T().Errorf("failed to delete Repository %s: %v", repo.Name, err)
				}
			}
		}
		if len(repoList.Items) > 0 {
			t.T().Logf("deleted %d existing Repositories", len(repoList.Items))
			time.Sleep(5 * time.Second)
		}
	}

	deletedCount := 0
	for i := 0; i < t.testOptions.numRepos; i++ {
		repoName := fmt.Sprintf("%s-test-%d", t.testOptions.namespace, i)
		if err := deleteGiteaRepo(repoName); err == nil {
			deletedCount++
		}
	}
	if deletedCount > 0 {
		t.T().Logf("deleted %d existing Gitea repositories", deletedCount)
	}

	return nil
}

func (t *PerfTestSuite) createAndSetupRepo(repoName string) {
	t.metricsMutex.Lock()
	t.metrics[repoName] = TestMetrics{
		RepoName:      repoName,
		repoOps:       make(map[string]OperationMetrics),
		pkgRevMetrics: make(map[string]map[int]PackageRevisionMetrics),
	}
	t.metricsMutex.Unlock()

	start := time.Now()
	err := createGiteaRepo(repoName)
	duration := time.Since(start)

	t.recordRepoMetric(repoName, giteaRepoCreate, OperationMetrics{
		Operation: fmt.Sprintf("%s:%s", giteaRepoCreate, repoName),
		Duration:  duration,
		Error:     err,
		Timestamp: start,
	})

	if t.enablePrometheus {
		metrics.PerfTestRecordMetric(giteaRepoCreate, repoName, "", duration, err)
	}

	if err != nil {
		t.T().Errorf("Failed to create Gitea repository: %v", err)
		return
	}

	start = time.Now()
	repo := &configapi.Repository{
		ObjectMeta: metav1.ObjectMeta{
			Name:      repoName,
			Namespace: t.testOptions.namespace,
		},
		Spec: configapi.RepositorySpec{
			Type: "git",
			Git: &configapi.GitRepository{
				Repo:   fmt.Sprintf("http://gitea.gitea.svc.cluster.local:3000/nephio/%s", repoName),
				Branch: "main",
				SecretRef: configapi.SecretRef{
					Name: "gitea",
				},
				CreateBranch: true,
			},
		},
	}

	err = t.client.Create(t.ctx, repo)
	duration = time.Since(start)

	t.recordRepoMetric(repoName, porchRepoCreate, OperationMetrics{
		Operation: fmt.Sprintf("%s:%s", porchRepoCreate, repoName),
		Duration:  duration,
		Error:     err,
		Timestamp: start,
	})

	if t.enablePrometheus {
		metrics.PerfTestRecordMetric(porchRepoCreate, repoName, "", duration, err)
	}

	if err != nil {
		t.T().Errorf("Failed to create Porch repository: %v", err)
		return
	}

	if t.enablePrometheus {
		metrics.PerfTestIncrementRepositoryCounter()
	}
	startWait := time.Now()
	err = t.waitForRepository(repoName, 60*time.Second)
	duration = time.Since(startWait)

	t.recordRepoMetric(repoName, repoWait, OperationMetrics{
		Operation: fmt.Sprintf("%s:%s", repoWait, repoName),
		Duration:  duration,
		Error:     err,
		Timestamp: start,
	})

	if t.enablePrometheus {
		metrics.PerfTestRecordMetric(repoWait, repoName, "", duration, err)
	}
}

func createGiteaRepo(repoName string) error {
	giteaURL := "http://localhost:3000/api/v1/user/repos"
	payload := map[string]interface{}{
		"name":        repoName,
		"description": "Test repository for Porch metrics",
		"private":     false,
		"auto_init":   true,
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return pkgerrors.Wrap(err, "failed to marshal payload")
	}

	req, err := http.NewRequest("POST", giteaURL, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return pkgerrors.Wrap(err, "failed to create request")
	}

	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(giteaUsername, giteaPassword)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return pkgerrors.Wrapf(err, "failed to create repo %s", repoName)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return pkgerrors.Errorf("failed to create repo, status: %d", resp.StatusCode)
	}

	return nil
}

func deleteGiteaRepo(repoName string) error {
	giteaURL := fmt.Sprintf("http://localhost:3000/api/v1/repos/nephio/%s", repoName)

	req, err := http.NewRequest("DELETE", giteaURL, nil)
	if err != nil {
		return pkgerrors.Wrap(err, "failed to create delete request")
	}

	req.SetBasicAuth(giteaUsername, giteaPassword)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return pkgerrors.Wrapf(err, "failed to delete repo %s", repoName)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusNotFound {
		return pkgerrors.Errorf("failed to delete repo, status: %d", resp.StatusCode)
	}

	return nil
}
func (t *PerfTestSuite) waitForRepository(name string, timeout time.Duration) error {
	start := time.Now()
	for {
		if time.Since(start) > timeout {
			return pkgerrors.Errorf("timeout waiting for repository to be ready")
		}

		var repo configapi.Repository
		err := t.client.Get(t.ctx, client.ObjectKey{Namespace: t.testOptions.namespace, Name: name}, &repo)
		if err != nil {
			return err
		}

		t.T().Logf("\nRepository conditions at %v:", time.Since(start))
		t.T().Logf("Spec: %+v", repo.Spec)
		t.T().Logf("Status: %+v", repo.Status)

		ready := false
		for _, cond := range repo.Status.Conditions {
			t.T().Logf("  - Type: %s, Status: %s, Message: %s",
				cond.Type, cond.Status, cond.Message)
			if cond.Type == "Ready" && cond.Status == "True" {
				ready = true
				break
			}
		}

		if ready {
			return nil
		}

		time.Sleep(2 * time.Second)
	}
}

func (t *PerfTestSuite) doLifecycle(repoName, pkgName string, revisionNum int) (string, error) {
	var list porchapi.PackageRevisionList
	var task []porchapi.Task

	t.initPkgRevMetrics(repoName, pkgName, revisionNum)

	start := time.Now()
	err := retry.RetryOnConflict(retryBackoff, func() error {
		return t.client.List(t.ctx, &list, client.InNamespace(t.testOptions.namespace))
	})
	duration := time.Since(start)

	t.recordPkgRevMetric(repoName, pkgName, revisionNum, pkgRevList, OperationMetrics{
		Operation: fmt.Sprintf("%s:%d", pkgRevList, revisionNum),
		Duration:  duration,
		Error:     err,
		Timestamp: start,
	})

	if t.enablePrometheus {
		metrics.PerfTestRecordMetric(pkgRevList, repoName, pkgName, duration, err)
	}

	if err != nil {
		return "", err
	}

	var latestPR *porchapi.PackageRevision
	for i := range list.Items {
		pr := &list.Items[i]
		if pr.Spec.PackageName == pkgName &&
			pr.Spec.RepositoryName == repoName &&
			pr.Spec.Lifecycle == porchapi.PackageRevisionLifecyclePublished {
			if latestPR == nil || pr.Spec.Revision > latestPR.Spec.Revision {
				latestPR = pr
			}
		}
	}

	if revisionNum == 1 {
		task = []porchapi.Task{
			{
				Type: porchapi.TaskTypeInit,
				Init: &porchapi.PackageInitTaskSpec{
					Description: fmt.Sprintf("Test package %s for Porch metrics", pkgName),
					Keywords:    []string{"test", "metrics"},
					Site:        "https://nephio.org",
				},
			},
		}
		if t.enablePrometheus {
			metrics.PerfTestIncrementPackageCounter()
		}
	} else if latestPR != nil {
		task = []porchapi.Task{
			{
				Type: porchapi.TaskTypeEdit,
				Edit: &porchapi.PackageEditTaskSpec{
					Source: &porchapi.PackageRevisionRef{
						Name: latestPR.Name,
					},
				},
			},
		}
	}

	workspace := fmt.Sprintf("v%d", revisionNum)
	pkgRev := &porchapi.PackageRevision{
		TypeMeta: metav1.TypeMeta{
			Kind:       "PackageRevision",
			APIVersion: porchapi.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: t.testOptions.namespace,
		},
		Spec: porchapi.PackageRevisionSpec{
			PackageName:    pkgName,
			WorkspaceName:  workspace,
			RepositoryName: repoName,
			Tasks:          task,
		},
	}

	if err = t.createPackageRevision(pkgRev, repoName, revisionNum); err != nil {
		return "", err
	}

	if err = t.updateOrCreatePackageRevisionResources(repoName, pkgName, pkgRev.Name, revisionNum); err != nil {
		return "", err
	}

	if err = t.proposeAndApprovePackage(repoName, pkgName, pkgRev.Name, revisionNum); err != nil {
		return "", err
	}

	return pkgRev.Name, nil
}

func (t *PerfTestSuite) createPackageRevision(pkgRev *porchapi.PackageRevision, repoName string, revisionNum int) error {
	start := time.Now()
	if t.enablePrometheus {
		metrics.PerfTestRecordActiveOperation(pkgRevCreate, 1)
		defer metrics.PerfTestRecordActiveOperation(pkgRevCreate, -1)
	}

	err := retry.RetryOnConflict(retryBackoff, func() error {
		return t.client.Create(t.ctx, pkgRev)
	})
	duration := time.Since(start)

	t.recordPkgRevMetric(repoName, pkgRev.Spec.PackageName, revisionNum, pkgRevCreate, OperationMetrics{
		Operation: fmt.Sprintf("%s:%d", pkgRevCreate, revisionNum),
		Duration:  duration,
		Error:     err,
		Timestamp: start,
	})

	if t.enablePrometheus {
		metrics.PerfTestRecordMetric(pkgRevCreate, repoName, pkgRev.Spec.PackageName, duration, err)
		metrics.PerfTestRecordPackageRevision(pkgRevCreate, err)
	}

	if err != nil {
		return err
	}

	return nil
}

func (t *PerfTestSuite) updateOrCreatePackageRevisionResources(repoName, pkgName, pkgRevName string, revisionNum int) error {
	var resources porchapi.PackageRevisionResources

	start := time.Now()
	err := retry.RetryOnConflict(retryBackoff, func() error {
		return t.client.Get(t.ctx, client.ObjectKey{Namespace: t.testOptions.namespace, Name: pkgRevName}, &resources)
	})
	duration := time.Since(start)
	t.recordPkgRevMetric(repoName, pkgName, revisionNum, pkgRevResourcesGet, OperationMetrics{
		Operation: fmt.Sprintf("%s:%d", pkgRevResourcesGet, revisionNum),
		Duration:  duration,
		Error:     err,
		Timestamp: start,
	})

	if t.enablePrometheus {
		metrics.PerfTestRecordMetric(pkgRevResourcesGet, repoName, pkgName, duration, err)
	}

	if err != nil {
		return err
	}

	pkgResources := t.createPackageResources(pkgRevName)
	if resources.Spec.Resources == nil {
		resources.Spec.Resources = make(map[string]string)
	}
	for name, content := range pkgResources {
		resources.Spec.Resources[name] = content
	}

	start = time.Now()
	err = retry.RetryOnConflict(retryBackoff, func() error {
		return t.client.Update(t.ctx, &resources)
	})
	duration = time.Since(start)
	t.recordPkgRevMetric(repoName, pkgName, revisionNum, pkgRevUpdate, OperationMetrics{
		Operation: fmt.Sprintf("%s:%d", pkgRevUpdate, revisionNum),
		Duration:  duration,
		Error:     err,
		Timestamp: start,
	})

	if t.enablePrometheus {
		metrics.PerfTestRecordMetric(pkgRevUpdate, repoName, pkgName, duration, err)
	}

	if err != nil {
		return err
	}

	return nil
}

func (t *PerfTestSuite) createPackageResources(pkgName string) map[string]string {
	resources := make(map[string]string)

	testDataSize := t.testOptions.prrPaddingInMB * 1024 * 1024

	resources["Kptfile"] = t.readResourcesFromDir(t.testOptions.kptfilePath)
	resources["deployment.yaml"] = t.readResourcesFromDir(t.testOptions.packageResourcesPath)

	resources["Kptfile"] = strings.ReplaceAll(resources["Kptfile"], "CHANGE_ME", pkgName)
	resources["Kptfile"] = strings.ReplaceAll(resources["Kptfile"], "REGISTRY_URL", t.testOptions.krmFnRegistryURL)
	resources["deployment.yaml"] = strings.ReplaceAll(resources["deployment.yaml"], "CHANGE_ME", pkgName) + strings.Repeat("a", testDataSize)

	return resources
}

func (t *PerfTestSuite) proposeAndApprovePackage(repoName, pkgName, pkgRevName string, revisionNum int) error {
	var pkg porchapi.PackageRevision

	start := time.Now()
	err := retry.RetryOnConflict(retryBackoff, func() error {
		return t.client.Get(t.ctx, client.ObjectKey{Namespace: t.testOptions.namespace, Name: pkgRevName}, &pkg)
	})
	duration := time.Since(start)
	t.recordPkgRevMetric(repoName, pkgName, revisionNum, pkgRevGet, OperationMetrics{
		Operation: fmt.Sprintf("%s:%d", pkgRevGet, revisionNum),
		Duration:  duration,
		Error:     err,
		Timestamp: start,
	})

	if t.enablePrometheus {
		metrics.PerfTestRecordMetric(pkgRevGet, repoName, pkgName, duration, err)
	}

	if err != nil {
		return err
	}

	start = time.Now()
	initialLifecycle := pkg.Spec.Lifecycle
	err = retry.RetryOnConflict(retryBackoff, func() error {
		if err := t.client.Get(t.ctx, client.ObjectKey{Namespace: t.testOptions.namespace, Name: pkgRevName}, &pkg); err != nil {
			return err
		}
		pkg.Spec.Lifecycle = porchapi.PackageRevisionLifecycleProposed
		return t.client.Update(t.ctx, &pkg)
	})
	duration = time.Since(start)

	t.recordPkgRevMetric(repoName, pkgName, revisionNum, pkgRevPropose, OperationMetrics{
		Operation: fmt.Sprintf("%s:%d", pkgRevPropose, revisionNum),
		Duration:  duration,
		Error:     err,
		Timestamp: start,
	})

	if t.enablePrometheus {
		metrics.PerfTestRecordMetric(pkgRevPropose, repoName, pkgName, duration, err)
		metrics.PerfTestRecordLifecycleTransition(string(initialLifecycle), string(porchapi.PackageRevisionLifecycleProposed), repoName, pkgName, duration, err)
	}

	if err != nil {
		return err
	}

	start = time.Now()
	err = retry.RetryOnConflict(retryBackoff, func() error {
		return t.client.Get(t.ctx, client.ObjectKey{Namespace: t.testOptions.namespace, Name: pkgRevName}, &pkg)
	})
	duration = time.Since(start)
	t.recordPkgRevMetric(repoName, pkgName, revisionNum, pkgRevGetProposed, OperationMetrics{
		Operation: fmt.Sprintf("%s:%d", pkgRevGetProposed, revisionNum),
		Duration:  duration,
		Error:     err,
		Timestamp: start,
	})

	if t.enablePrometheus {
		metrics.PerfTestRecordMetric(pkgRevGetProposed, repoName, pkgName, duration, err)
	}

	if err != nil {
		return err
	}

	start = time.Now()
	err = retry.RetryOnConflict(retryBackoff, func() error {
		if err := t.client.Get(t.ctx, client.ObjectKey{Namespace: t.testOptions.namespace, Name: pkgRevName}, &pkg); err != nil {
			return err
		}
		pkg.Spec.Lifecycle = porchapi.PackageRevisionLifecyclePublished
		_, err := t.clientSet.PorchV1alpha1().PackageRevisions(t.testOptions.namespace).UpdateApproval(t.ctx, pkgRevName, &pkg, metav1.UpdateOptions{})
		return err
	})
	duration = time.Since(start)

	t.recordPkgRevMetric(repoName, pkgName, revisionNum, pkgRevPublished, OperationMetrics{
		Operation: fmt.Sprintf("%s:%d", pkgRevPublished, revisionNum),
		Duration:  duration,
		Error:     err,
		Timestamp: start,
	})

	if t.enablePrometheus {
		metrics.PerfTestRecordMetric(pkgRevPublished, repoName, pkgName, duration, err)
		metrics.PerfTestRecordLifecycleTransition(string(porchapi.PackageRevisionLifecycleProposed), string(porchapi.PackageRevisionLifecyclePublished), repoName, pkgName, duration, err)
	}

	return nil
}

func (t *PerfTestSuite) deletePackageRevision(repoName, pkgName, pkgRevName string, revisionNum int) error {
	var pkgRev porchapi.PackageRevision
	err := retry.RetryOnConflict(retryBackoff, func() error {
		return t.client.Get(t.ctx, client.ObjectKey{Namespace: t.testOptions.namespace, Name: pkgRevName}, &pkgRev)
	})
	if err != nil {
		return err
	}

	start := time.Now()
	initialLifecycle := pkgRev.Spec.Lifecycle
	err = retry.RetryOnConflict(retryBackoff, func() error {
		if err := t.client.Get(t.ctx, client.ObjectKey{Namespace: t.testOptions.namespace, Name: pkgRevName}, &pkgRev); err != nil {
			return err
		}
		pkgRev.Spec.Lifecycle = porchapi.PackageRevisionLifecycleDeletionProposed
		return t.client.Update(t.ctx, &pkgRev)
	})
	duration := time.Since(start)
	t.recordPkgRevMetric(repoName, pkgName, revisionNum, pkgRevProposeDeletion, OperationMetrics{
		Operation: fmt.Sprintf("%s:%d", pkgRevProposeDeletion, revisionNum),
		Duration:  duration,
		Error:     err,
		Timestamp: start,
	})

	if t.enablePrometheus {
		metrics.PerfTestRecordMetric(pkgRevProposeDeletion, repoName, pkgName, duration, err)
		metrics.PerfTestRecordLifecycleTransition(string(initialLifecycle), string(porchapi.PackageRevisionLifecycleDeletionProposed), repoName, pkgName, duration, err)
	}

	if err != nil {
		return err
	}

	start = time.Now()
	err = retry.RetryOnConflict(retryBackoff, func() error {
		if err := t.client.Get(t.ctx, client.ObjectKey{Namespace: t.testOptions.namespace, Name: pkgRevName}, &pkgRev); err != nil {
			return err
		}
		return t.client.Delete(t.ctx, &pkgRev)
	})
	duration = time.Since(start)
	t.recordPkgRevMetric(repoName, pkgName, revisionNum, pkgRevDelete, OperationMetrics{
		Operation: fmt.Sprintf("%s:%d", pkgRevDelete, revisionNum),
		Duration:  duration,
		Error:     err,
		Timestamp: start,
	})

	if t.enablePrometheus {
		metrics.PerfTestRecordMetric(pkgRevDelete, repoName, pkgName, duration, err)
		metrics.PerfTestRecordLifecycleTransition(string(porchapi.PackageRevisionLifecycleDeletionProposed), "deleted", repoName, pkgName, duration, err)
	}

	return nil
}

func (t *PerfTestSuite) readResourcesFromDir(dir string) string {
	t.T().Helper()
	var content []byte
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			content, err = os.ReadFile(path)
			if err != nil {
				t.T().Fatalf("ReadFile(%q) failed: %v", path, err)
			}
		}
		return nil
	})
	if err != nil {
		t.T().Fatalf("WalkDir(%s) failed: %v", dir, err)
	}
	return string(content)
}
