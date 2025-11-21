// Copyright 2022, 2025 The kpt and Nephio Authors
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

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	porchclient "github.com/nephio-project/porch/api/generated/clientset/versioned"
	porchapi "github.com/nephio-project/porch/api/porch/v1alpha1"
	configapi "github.com/nephio-project/porch/api/porchconfig/v1alpha1"
	internalapi "github.com/nephio-project/porch/internal/api/porchinternal/v1alpha1"
	internalpkg "github.com/nephio-project/porch/internal/kpt/pkg"
	"github.com/nephio-project/porch/pkg/externalrepo/git"
	kptfilev1 "github.com/nephio-project/porch/pkg/kpt/api/kptfile/v1"
	"github.com/nephio-project/porch/pkg/repository"
	"github.com/stretchr/testify/suite"
	appsv1 "k8s.io/api/apps/v1"
	coreapi "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	aggregatorv1 "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

const (
	// TODO: accept a flag?
	PorchTestConfigFile = "porch-test-config.yaml"
	updateGoldenFiles   = "UPDATE_GOLDEN_FILES"
	TestGitServerImage  = "test-git-server"
)

type GitConfig struct {
	Repo      string   `json:"repo"`
	Branch    string   `json:"branch"`
	Directory string   `json:"directory"`
	Username  string   `json:"username"`
	Password  Password `json:"password"`
}

type OciConfig struct {
	Registry string `json:"registry"`
}

type Password string

func (p Password) String() string {
	return "*************"
}

type TestSuite struct {
	suite.Suite
	ctx context.Context

	Kubeconfig *rest.Config
	Client     client.Client
	Reader     client.Reader

	// Strongly-typed client handy for reading e.g. pod logs
	KubeClient kubernetes.Interface

	Clientset porchclient.Interface

	Namespace            string // K8s namespace for this test run
	TestRunnerIsLocal    bool   // Tests running against local dev porch
	porchServerInCluster *bool  // Cached result of IsPorchServerInCluster check

	gcpBlueprintsRepo  string
	gcpBucketRef       string
	gcpRedisBucketRef  string
	gcpHierarchyRef    string
	kptFunctionRef     string
	kptRepo            string
	gcrPrefix          string
}

func (t *TestSuite) SetupSuite() {
	t.ctx = context.Background()
	t.Initialize()
}

func RunInParallel(functions ...func() any) []any {
	var group sync.WaitGroup
	var results []any
	for _, eachFunction := range functions {
		group.Add(1)
		go func() {
			defer group.Done()
			if reflect.TypeOf(eachFunction).NumOut() == 0 {
				results = append(results, nil)
				eachFunction()
			} else {
				eachResult := eachFunction()

				results = append(results, eachResult)
			}
		}()
	}
	group.Wait()
	return results
}

func (t *TestSuite) Initialize() {
	cfg, err := config.GetConfig()
	if err != nil {
		t.Skipf("Skipping test suite - cannot obtain k8s client config: %v", err)
	}

	t.Logf("Testing against server: %q", cfg.Host)
	cfg.UserAgent = "Porch Test"
	t.Logf("using timeout %v", cfg.Timeout)

	scheme := createClientScheme(t.T())

	if c, err := client.New(cfg, client.Options{
		Scheme: scheme,
	}); err != nil {
		t.Fatalf("Failed to initialize k8s client (%s): %v", cfg.Host, err)
	} else {
		t.Client = c
		t.Kubeconfig = cfg
	}

	// Create a direct API reader to avoid stale cache reads
	if r, err := client.New(cfg, client.Options{
		Scheme: scheme,
		Cache: &client.CacheOptions{
			DisableFor: []client.Object{},
		},
	}); err != nil {
		t.Fatalf("Failed to initialize direct API reader (%s): %v", cfg.Host, err)
	} else {
		t.Reader = r
	}

	if kubeClient, err := kubernetes.NewForConfig(cfg); err != nil {
		t.Fatalf("failed to initialize kubernetes clientset: %v", err)
	} else {
		t.KubeClient = kubeClient
	}

	if cs, err := porchclient.NewForConfig(cfg); err != nil {
		t.Fatalf("Failed to initialize Porch clientset: %v", err)
	} else {
		t.Clientset = cs
	}

	t.TestRunnerIsLocal = !t.IsTestRunnerInCluster()

	namespace := fmt.Sprintf("porch-test-%d", time.Now().UnixMicro())
	t.CreateF(&coreapi.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
		},
	})

	t.Namespace = namespace
	c := t.Client
	t.Cleanup(func() {
		if err := c.Delete(t.GetContext(), &coreapi.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespace,
			},
		}); err != nil {
			t.Errorf("Failed to clean up namespace %q: %v", namespace, err)
		} else {
			t.Logf("Successfully cleaned up namespace %q", namespace)
		}
	})
}

func (t *TestSuite) IsPorchServerInCluster() bool {
	if t.porchServerInCluster != nil {
		return *t.porchServerInCluster
	}

	porch := aggregatorv1.APIService{}
	t.GetF(client.ObjectKey{
		Name: porchapi.SchemeGroupVersion.Version + "." + porchapi.SchemeGroupVersion.Group,
	}, &porch)
	service := coreapi.Service{}
	t.GetF(client.ObjectKey{
		Namespace: porch.Spec.Service.Namespace,
		Name:      porch.Spec.Service.Name,
	}, &service)

	result := len(service.Spec.Selector) > 0
	t.porchServerInCluster = &result
	return result
}

func (t *TestSuite) IsTestRunnerInCluster() bool {
	runLocally := os.Getenv("RUN_E2E_LOCALLY")
	if strings.ToLower(runLocally) == "true" {
		return false
	}

	porch := aggregatorv1.APIService{}
	ctx := context.TODO()
	t.GetF(client.ObjectKey{
		Name: porchapi.SchemeGroupVersion.Version + "." + porchapi.SchemeGroupVersion.Group,
	}, &porch)
	service := coreapi.Service{}
	err := t.Client.Get(ctx, client.ObjectKey{
		Namespace: porch.Spec.Service.Namespace,
		Name:      "function-runner",
	}, &service)
	if err != nil {
		return false
	}
	return len(service.Spec.Selector) > 0
}

func (t *TestSuite) CreateGitRepo() GitConfig {
	// Deploy Git server via k8s client.
	t.Logf("creating git server in cluster")
	if t.IsTestRunnerInCluster() {
		return t.createInClusterGitServer(!t.IsPorchServerInCluster())
	} else {
		return createLocalGitServer(t.T())
	}
}

func (t *TestSuite) Name() string {
	t.T().Helper()
	return t.T().Name()
}

func (t *TestSuite) GetContext() context.Context {
	t.T().Helper()
	return t.ctx
}

func (t *TestSuite) Skipf(format string, args ...any) {
	t.T().Helper()
	t.T().Skipf(format, args...)
}

func (t *TestSuite) Cleanup(f func()) {
	t.T().Helper()
	t.T().Cleanup(f)
}

func (t *TestSuite) Log(args ...any) {
	t.T().Helper()
	args = append([]any{"INFO:"}, args)
	t.T().Log(args...)
}

func (t *TestSuite) Logf(format string, args ...any) {
	t.T().Helper()
	t.T().Logf("INFO: "+format, args...)
}

func (t *TestSuite) Error(args ...any) {
	t.T().Helper()
	args = append([]any{"ERROR:"}, args...)
	t.T().Error(args...)
}

func (t *TestSuite) Errorf(format string, args ...any) {
	t.T().Helper()
	t.T().Errorf("ERROR: "+format, args...)
}

func (t *TestSuite) Fatal(args ...any) {
	t.T().Helper()
	args = append([]any{"FATAL:"}, args...)
	t.T().Fatal(args...)
}

func (t *TestSuite) Fatalf(format string, args ...any) {
	t.T().Helper()
	t.T().Fatalf("FATAL: "+format, args...)
}

type ErrorHandler func(format string, args ...interface{})

func (t *TestSuite) get(key client.ObjectKey, obj client.Object, eh ErrorHandler) {
	t.T().Helper()
	if err := t.Reader.Get(t.GetContext(), key, obj); err != nil {
		eh("failed to get resource %T %s: %v", obj, key, err)
	}
}

func (t *TestSuite) list(list client.ObjectList, opts []client.ListOption, eh ErrorHandler) {
	t.T().Helper()
	if err := t.Reader.List(t.GetContext(), list, opts...); err != nil {
		eh("failed to list resources %T %+v: %v", list, list, err)
	}
}

func DebugFormat(obj client.Object) string {
	var s string
	gvk := obj.GetObjectKind().GroupVersionKind()
	if gvk.Empty() {
		s = fmt.Sprintf("%T", obj)
	} else {
		s = gvk.Kind
	}
	ns := obj.GetNamespace()
	if ns != "" {
		s += " " + ns + "/"
	} else {
		s += " "
	}
	s += obj.GetName()
	return s
}

func (t *TestSuite) create(obj client.Object, opts []client.CreateOption, eh ErrorHandler) {
	t.T().Helper()
	t.Logf("creating object %v", DebugFormat(obj))
	start := time.Now()
	defer func() {
		t.T().Helper()
		t.Logf("took %v to create %s/%s", time.Since(start), obj.GetNamespace(), obj.GetName())
	}()

	if err := t.Client.Create(t.GetContext(), obj, opts...); err != nil {
		eh("failed to create resource %s: %v", DebugFormat(obj), err)
	}
}

func (t *TestSuite) createOrUpdate(obj client.Object, opts []client.CreateOption, eh ErrorHandler) {
	t.T().Helper()
	t.Logf("creating object %v", DebugFormat(obj))
	start := time.Now()
	defer func() {
		t.T().Helper()
		t.Logf("took %v to create %s/%s", time.Since(start), obj.GetNamespace(), obj.GetName())
	}()

	if err := t.Client.Create(t.GetContext(), obj, opts...); err != nil {
		if apierrors.IsAlreadyExists(err) {
			t.Logf("failed to create resource - attempting to update resource %v", DebugFormat(obj))

			if err := t.Client.Update(t.GetContext(), obj); err != nil {
				eh("failed to update resource %s: %v", DebugFormat(obj), err)
			}
		} else {
			eh("failed to create resource %s: %v", DebugFormat(obj), err)
		}
	}
}

func (t *TestSuite) delete(obj client.Object, opts []client.DeleteOption, eh ErrorHandler) {
	t.T().Helper()
	t.Logf("deleting object %v", DebugFormat(obj))

	if err := t.Client.Delete(t.GetContext(), obj, opts...); err != nil {
		eh("failed to delete resource %s: %v", DebugFormat(obj), err)
	}
}

func (t *TestSuite) update(obj client.Object, opts []client.UpdateOption, eh ErrorHandler) {
	t.T().Helper()
	t.Logf("updating object %v", DebugFormat(obj))

	if err := t.Client.Update(t.GetContext(), obj, opts...); err != nil {
		eh("failed to update resource %s: %v", DebugFormat(obj), err)
	}
}

func (t *TestSuite) patch(obj client.Object, patch client.Patch, opts []client.PatchOption, eh ErrorHandler) {
	t.T().Helper()
	t.Logf("patching object %v", DebugFormat(obj))

	if err := t.Client.Patch(t.GetContext(), obj, patch, opts...); err != nil {
		eh("failed to patch resource %s: %v", DebugFormat(obj), err)
	}
}

func (t *TestSuite) updateApproval(obj *porchapi.PackageRevision, opts metav1.UpdateOptions, eh ErrorHandler) *porchapi.PackageRevision {
	t.T().Helper()
	t.Logf("updating approval of %v", DebugFormat(obj))
	if res, err := t.Clientset.PorchV1alpha1().PackageRevisions(obj.Namespace).UpdateApproval(t.GetContext(), obj.Name, obj, opts); err != nil {
		eh("failed to update approval of %s/%s: %v", obj.Namespace, obj.Name, err)
		return nil
	} else {
		return res
	}
}

// deleteAllOf(ctx context.Context, obj Object, opts ...DeleteAllOfOption) error

func (t *TestSuite) GetE(key client.ObjectKey, obj client.Object) {
	t.T().Helper()
	t.get(key, obj, t.Errorf)
}

func (t *TestSuite) GetF(key client.ObjectKey, obj client.Object) {
	t.T().Helper()
	t.get(key, obj, t.Fatalf)
}

func (t *TestSuite) ListE(list client.ObjectList, opts ...client.ListOption) {
	t.T().Helper()
	t.list(list, opts, t.Errorf)
}

func (t *TestSuite) ListF(list client.ObjectList, opts ...client.ListOption) {
	t.T().Helper()
	t.list(list, opts, t.Fatalf)
}

func (t *TestSuite) CreateF(obj client.Object, opts ...client.CreateOption) {
	t.T().Helper()
	t.create(obj, opts, t.Fatalf)
}

func (t *TestSuite) CreateE(obj client.Object, opts ...client.CreateOption) {
	t.T().Helper()
	t.create(obj, opts, t.Errorf)
}

func (t *TestSuite) CreateOrUpdateF(obj client.Object, opts ...client.CreateOption) {
	t.T().Helper()
	t.createOrUpdate(obj, opts, t.Fatalf)
}

func (t *TestSuite) DeleteF(obj client.Object, opts ...client.DeleteOption) {
	t.T().Helper()
	t.delete(obj, opts, t.Fatalf)
}

func (t *TestSuite) DeleteE(obj client.Object, opts ...client.DeleteOption) {
	t.T().Helper()
	t.delete(obj, opts, t.Errorf)
}

func (t *TestSuite) DeleteL(obj client.Object, opts ...client.DeleteOption) {
	t.T().Helper()
	t.delete(obj, opts, t.Logf)
}

// DeleteEH calls delete with a custom ErrorHandler
func (t *TestSuite) DeleteEH(obj client.Object, eh ErrorHandler, opts ...client.DeleteOption) {
	t.T().Helper()
	t.delete(obj, opts, eh)
}

func (t *TestSuite) UpdateF(obj client.Object, opts ...client.UpdateOption) {
	t.T().Helper()
	t.update(obj, opts, t.Fatalf)
}

func (t *TestSuite) UpdateE(obj client.Object, opts ...client.UpdateOption) {
	t.T().Helper()
	t.update(obj, opts, t.Errorf)
}

func (t *TestSuite) PatchF(obj client.Object, patch client.Patch, opts ...client.PatchOption) {
	t.T().Helper()
	t.patch(obj, patch, opts, t.Fatalf)
}

func (t *TestSuite) PatchE(obj client.Object, patch client.Patch, opts ...client.PatchOption) {
	t.T().Helper()
	t.patch(obj, patch, opts, t.Errorf)
}

func (t *TestSuite) UpdateApprovalL(pr *porchapi.PackageRevision, opts metav1.UpdateOptions) *porchapi.PackageRevision {
	t.T().Helper()
	return t.updateApproval(pr, opts, t.Logf)
}

func (t *TestSuite) UpdateApprovalF(pr *porchapi.PackageRevision, opts metav1.UpdateOptions) *porchapi.PackageRevision {
	t.T().Helper()
	return t.updateApproval(pr, opts, t.Fatalf)
}

// DeleteAllOf(ctx context.Context, obj Object, opts ...DeleteAllOfOption) error

func createClientScheme(t *testing.T) *runtime.Scheme {
	scheme := runtime.NewScheme()

	for _, api := range (runtime.SchemeBuilder{
		porchapi.AddToScheme,
		internalapi.AddToScheme,
		configapi.AddToScheme,
		coreapi.AddToScheme,
		aggregatorv1.AddToScheme,
		appsv1.AddToScheme,
	}) {
		if err := api(scheme); err != nil {
			t.Fatalf("Failed to initialize test k8s api client")
		}
	}
	return scheme
}

func createLocalGitServer(t *testing.T) GitConfig {
	tmp, err := os.MkdirTemp("", "porch-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp directory for Git repository: %v", err)
		return GitConfig{}
	}

	t.Cleanup(func() {
		if err := os.RemoveAll(tmp); err != nil {
			t.Errorf("Failed to delete Git temp directory %q: %v", tmp, err)
		}
	})

	var gitRepoOptions []git.GitRepoOption
	repos := git.NewDynamicRepos(tmp, gitRepoOptions)

	server, err := git.NewGitServer(repos)
	if err != nil {
		t.Fatalf("Failed to start git server: %v", err)
		return GitConfig{}
	}

	var wg sync.WaitGroup
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() {
		cancel()
		wg.Wait()
	})

	addressChannel := make(chan net.Addr)

	wg.Add(1)
	go func() {
		defer wg.Done()
		err := server.ListenAndServe(ctx, "127.0.0.1:0", addressChannel)
		if err != nil {
			if err == http.ErrServerClosed {
				t.Log("Git server shut down successfully")
			} else {
				t.Errorf("Git server exited with error: %v", err)
			}
		}
	}()

	// Wait for server to start up
	address, ok := <-addressChannel
	if !ok {
		t.Errorf("Server failed to start")
		return GitConfig{}
	}

	return GitConfig{
		Repo:      fmt.Sprintf("http://%s", address),
		Branch:    "main",
		Directory: "/",
	}
}

func (t *TestSuite) createInClusterGitServer(exposeByLoadBalancer bool) GitConfig {
	// Determine git-server image name. Use the same container registry and tag as the Porch server,
	// replacing base image name with `git-server`. TODO: Make configurable?

	var porch appsv1.Deployment
	t.GetF(client.ObjectKey{
		Namespace: "porch-system",
		Name:      "function-runner",
	}, &porch)

	gitImage := InferGitServerImage(porch.Spec.Template.Spec.Containers[0].Image)

	var deploymentKey = client.ObjectKey{
		Namespace: t.Namespace,
		Name:      "git-server",
	}
	var replicas int32 = 1
	var selector = map[string]string{
		"git-server": strings.ReplaceAll(t.Name(), "/", "_"),
	}
	t.CreateF(&appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      deploymentKey.Name,
			Namespace: deploymentKey.Namespace,
			Annotations: map[string]string{
				"kpt.dev/porch-test": t.Name(),
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: selector,
			},
			Template: coreapi.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: selector,
				},
				Spec: coreapi.PodSpec{
					Containers: []coreapi.Container{
						{
							Name:  "git-server",
							Image: gitImage,
							Args:  []string{},
							Ports: []coreapi.ContainerPort{
								{
									ContainerPort: 8080,
									Protocol:      coreapi.ProtocolTCP,
								},
							},
							ImagePullPolicy: coreapi.PullIfNotPresent,
						},
					},
				},
			},
		},
	})

	t.Cleanup(func() {
		t.DeleteE(&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      deploymentKey.Name,
				Namespace: deploymentKey.Namespace,
			},
		})
	})

	t.Cleanup(func() {
		t.DumpLogsForDeploymentE(deploymentKey)
	})

	serviceKey := client.ObjectKey{
		Namespace: t.Namespace,
		Name:      "git-server-service",
	}
	service := coreapi.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceKey.Name,
			Namespace: serviceKey.Namespace,
			Annotations: map[string]string{
				"kpt.dev/porch-test": t.Name(),
			},
		},
		Spec: coreapi.ServiceSpec{
			Ports: []coreapi.ServicePort{
				{
					Protocol: coreapi.ProtocolTCP,
					Port:     8080,
					TargetPort: intstr.IntOrString{
						Type:   intstr.Int,
						IntVal: 8080,
					},
				},
			},
			Selector: selector,
		},
	}
	if exposeByLoadBalancer {
		service.Spec.Type = coreapi.ServiceTypeLoadBalancer
	}
	t.CreateF(&service)

	t.Cleanup(func() {
		t.DeleteE(&coreapi.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      serviceKey.Name,
				Namespace: serviceKey.Namespace,
			},
		})
	})

	t.Logf("Waiting for git-server to start ...")

	// Wait a minute for git server to start up.
	giveUp := time.Now().Add(time.Minute)

	for {
		time.Sleep(1 * time.Second)

		var deployment appsv1.Deployment
		t.GetF(deploymentKey, &deployment)

		ready := true
		if ready && deployment.Generation != deployment.Status.ObservedGeneration {
			t.Logf("waiting for ObservedGeneration %v to match Generation %v", deployment.Status.ObservedGeneration, deployment.Generation)
			ready = false
		}
		if ready && deployment.Status.UpdatedReplicas < deployment.Status.Replicas {
			t.Logf("waiting for UpdatedReplicas %d to match Replicas %d", deployment.Status.UpdatedReplicas, deployment.Status.Replicas)
			ready = false
		}
		if ready && deployment.Status.AvailableReplicas < deployment.Status.Replicas {
			t.Logf("waiting for AvailableReplicas %d to match Replicas %d", deployment.Status.AvailableReplicas, deployment.Status.Replicas)
			ready = false
		}

		if ready {
			ready = false // Until we've seen Available condition
			for _, condition := range deployment.Status.Conditions {
				if condition.Type == "Available" {
					ready = true
					if condition.Status != "True" {
						t.Logf("waiting for status.condition %v", condition)
						ready = false
					}
				}
			}
		}

		if ready {
			break
		}

		if time.Now().After(giveUp) {
			t.Fatalf("git server failed to start: %s", &deployment)
			return GitConfig{}
		}
	}

	t.Logf("git server is up")

	t.Logf("Waiting for git-server-service to be ready ...")

	// Check the Endpoint and Service resources for readiness
	gitUrl := fmt.Sprintf("http://%s.%s.svc.cluster.local:8080", serviceKey.Name, serviceKey.Namespace)
	giveUp = time.Now().Add(time.Minute)
	for {
		time.Sleep(1 * time.Second)
		if time.Now().After(giveUp) {
			t.Fatalf("git-server-service not ready on time")
			return GitConfig{}
		}

		var endpoint coreapi.Endpoints
		err := t.Client.Get(t.GetContext(), serviceKey, &endpoint)
		if err != nil || !endpointIsReady(&endpoint) {
			t.Logf("waiting for Endpoint to be ready: %+v", endpoint)
			continue
		}
		if exposeByLoadBalancer {
			var svc coreapi.Service
			// if svc is empty we will just continue
			_ = t.Client.Get(t.GetContext(), serviceKey, &svc)
			if len(svc.Status.LoadBalancer.Ingress) == 0 || svc.Status.LoadBalancer.Ingress[0].IP == "" {
				t.Logf("waiting for LoadBalancer to be assigned: %+v", svc)
				continue
			}
			t.Logf("LoadBalancer IP was assigned to git-server-service: %s", svc.Status.LoadBalancer.Ingress[0].IP)
			gitUrl = fmt.Sprintf("http://%s:8080", svc.Status.LoadBalancer.Ingress[0].IP)
		}

		t.Log("git-server-service is ready")
		break
	}

	return GitConfig{
		Repo:      gitUrl,
		Branch:    "main",
		Directory: "/",
	}
}

func InferGitServerImage(porchImage string) string {
	slash := strings.LastIndex(porchImage, "/")
	repo := porchImage[:slash+1]
	image := porchImage[slash+1:]
	colon := strings.LastIndex(image, ":")
	tag := image[colon+1:]

	return repo + TestGitServerImage + ":" + tag
}

func endpointIsReady(endpoints *coreapi.Endpoints) bool {
	if len(endpoints.Subsets) == 0 {
		return false
	}
	for _, s := range endpoints.Subsets {
		if len(s.Addresses) == 0 {
			return false
		}
		for _, a := range s.Addresses {
			if a.IP == "" {
				return false
			}
		}
	}
	return true
}

func (t *TestSuite) ParseKptfileF(resources *porchapi.PackageRevisionResources) *kptfilev1.KptFile {
	t.T().Helper()
	contents, ok := resources.Spec.Resources[kptfilev1.KptFileName]
	if !ok {
		t.Fatalf("Kptfile not found in %s/%s package", resources.Namespace, resources.Name)
	}
	kptfile, err := internalpkg.DecodeKptfile(strings.NewReader(contents))
	if err != nil {
		t.Fatalf("Cannot decode Kptfile (%s): %v", contents, err)
	}
	return kptfile
}

func (t *TestSuite) SaveKptfileF(resources *porchapi.PackageRevisionResources, kptfile *kptfilev1.KptFile) {
	t.T().Helper()
	b, err := yaml.MarshalWithOptions(kptfile, &yaml.EncoderOptions{SeqIndent: yaml.WideSequenceStyle})
	if err != nil {
		t.Fatalf("Failed saving Kptfile: %v", err)
	}
	resources.Spec.Resources[kptfilev1.KptFileName] = string(b)
}

func (t *TestSuite) FindAndDecodeF(resources *porchapi.PackageRevisionResources, name string, value interface{}) {
	t.T().Helper()
	contents, ok := resources.Spec.Resources[name]
	if !ok {
		t.Fatalf("Cannot find %q in %s/%s package", name, resources.Namespace, resources.Name)
	}
	var temp interface{}
	d := yaml.NewDecoder(strings.NewReader(contents))
	if err := d.Decode(&temp); err != nil {
		t.Fatalf("Cannot decode yaml %q in %s/%s package: %v", name, resources.Namespace, resources.Name, err)
	}
	jsonData, err := json.Marshal(temp)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	if err := json.Unmarshal(jsonData, value); err != nil {
		t.Fatalf("json.Unmarshal failed; %v", err)
	}
}

func (t *TestSuite) CompareGoldenFileYAML(goldenPath string, gotContents string) string {
	t.T().Helper()
	gotContents = t.normalizeYamlOrdering(gotContents)

	if os.Getenv(updateGoldenFiles) != "" {
		if err := os.WriteFile(goldenPath, []byte(gotContents), 0644); err != nil {
			t.Fatalf("Failed to update golden file %q: %v", goldenPath, err)
		}
	}
	golden, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("Failed to read golden file %q: %v", goldenPath, err)
	}
	return cmp.Diff(string(golden), gotContents)
}

func (t *TestSuite) normalizeYamlOrdering(contents string) string {
	t.T().Helper()
	var data interface{}
	if err := yaml.Unmarshal([]byte(contents), &data); err != nil {
		// not yaml.
		t.Fatalf("Failed to unmarshal yaml: %v\n%s\n", err, contents)
	}

	var stable bytes.Buffer
	encoder := yaml.NewEncoder(&stable)
	encoder.SetIndent(2)
	if err := encoder.Encode(data); err != nil {
		t.Fatalf("Failed to re-encode yaml output: %v", err)
	}
	return stable.String()
}

func (t *TestSuite) MustFindPackageRevision(packages *porchapi.PackageRevisionList, name repository.PackageRevisionKey) *porchapi.PackageRevision {
	t.T().Helper()
	for i := range packages.Items {
		pr := &packages.Items[i]
		if pr.Spec.RepositoryName == name.RKey().Name &&
			pr.Spec.PackageName == name.PKey().Package &&
			pr.Spec.Revision == name.Revision {
			return pr
		}
	}
	t.Fatalf("Failed to find package %q", name)
	return nil
}
