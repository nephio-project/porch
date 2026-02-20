/*
 Copyright 2025 The Nephio Authors.

 Licensed under the Apache License, Version 2.0 (the "License");
 You may not use this file except in compliance with the License.
 You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

 Unless required by applicable law or agreed to in writing, software
 distributed under the License is distributed on an "AS IS" BASIS,
 WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 See the License for the specific language governing permissions and
 limitations under the License.
*/

package internal

import (
	"context"
	"flag"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"k8s.io/klog/v2"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func TestPodCacheManager(t *testing.T) {

	flagSet := flag.NewFlagSet("log-level", flag.ContinueOnError)
	klog.InitFlags(flagSet)
	_ = flagSet.Parse([]string{"--v", "5"})

	defaultImageMetadataCache := map[string]*digestAndEntrypoint{
		defaultImageName: {
			digest:     "5245a52778d684fa698f69861fb2e058b308f6a74fed5bf2fe77d97bad5e071c",
			entrypoint: []string{"/" + defaultImageName},
		},
	}

	defaultPodObjectMeta := metav1.ObjectMeta{
		Name:      defaultPodName,
		Namespace: defaultNamespace,
		Labels: map[string]string{
			krmFunctionImageLabel: defaultFunctionImageLabel,
		},
		Annotations: map[string]string{
			templateVersionAnnotation: inlineTemplateVersionv1,
		},
	}

	defaultPodSpec := corev1.PodSpec{
		Containers: []corev1.Container{
			{
				Name:  "function",
				Image: defaultImageName,
			},
		},
	}

	podStatusRunning := corev1.PodStatus{
		Phase: corev1.PodRunning,
		Conditions: []corev1.PodCondition{
			{
				Type:   corev1.PodReady,
				Status: corev1.ConditionTrue,
			},
		},
		PodIP: defaultPodIP,
	}

	defaultPodObject := &corev1.Pod{
		ObjectMeta: defaultPodObjectMeta,
		Spec:       defaultPodSpec,
		Status:     podStatusRunning,
	}

	defaultServiceObject := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      defaultServiceName,
			Namespace: defaultNamespace,
		},
		Spec: corev1.ServiceSpec{
			Type:      corev1.ServiceTypeClusterIP,
			ClusterIP: defaultServiceIP,
		},
	}

	defaultEndpointObject := &corev1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{
			Name:      defaultEndpointName,
			Namespace: defaultNamespace,
		},
		Subsets: []corev1.EndpointSubset{
			{
				Addresses: []corev1.EndpointAddress{
					{
						IP: defaultPodIP,
						TargetRef: &corev1.ObjectReference{
							Name:      defaultPodName,
							Namespace: defaultNamespace,
						},
					},
				},
			},
		},
	}

	// Fake client. When Pod creation is invoked, it creates the Pod if not present and patches it to Running status
	// When Service creation is invoked, it creates endpoint object in additon to service
	fakeClientCreateFixInterceptor := func(ctx context.Context, kubeClient client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
		if obj.GetObjectKind().GroupVersionKind().Kind == "Pod" {
			var canary corev1.Pod
			err := kubeClient.Get(ctx, client.ObjectKeyFromObject(obj), &canary)
			if err != nil {
				if errors.IsNotFound(err) {
					obj.SetName(defaultPodName)
					obj.SetGenerateName("")
					err = kubeClient.Create(ctx, obj)
					if err != nil {
						return err
					}

					pod := obj.(*corev1.Pod)
					pod.Status = podStatusRunning
					err = kubeClient.Status().Update(context.Background(), pod)

					if err != nil {
						t.Errorf("Failed to patch pod: %v", err)

					}

					return nil
				}
				return err
			}
		}

		if obj.GetObjectKind().GroupVersionKind().Kind == "Service" {
			var canary corev1.Service
			err := kubeClient.Get(ctx, client.ObjectKeyFromObject(obj), &canary)
			if err != nil {
				if errors.IsNotFound(err) {
					defaultServiceObject.ResourceVersion = ""
					err = kubeClient.Create(ctx, defaultServiceObject)
					if err != nil {
						return err
					}

					defaultEndpointObject.ResourceVersion = ""
					err = kubeClient.Create(ctx, defaultEndpointObject)
					if err != nil {
						return err
					}
					return nil
				}
				return err
			}
		}
		return nil
	}

	buildKubeClient := func(objects ...client.Object) client.WithWatch {
		return fake.NewClientBuilder().
			WithObjects(objects...).
			WithInterceptorFuncs(interceptor.Funcs{Create: fakeClientCreateFixInterceptor}).
			Build()
	}

	address := net.JoinHostPort(defaultServiceIP, defaultWrapperServerPort)
	grpcClient, err := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("Failed to create grpc client: %v", err)
	}

	blankCache := map[string]*podAndGRPCClient{}

	cacheWithDefaultPod := map[string]*podAndGRPCClient{}
	cacheWithDefaultPod[defaultImageName] = &podAndGRPCClient{
		pod:        client.ObjectKeyFromObject(defaultPodObject),
		grpcClient: grpcClient,
	}

	// These tests focus on the pod management logic in the pod cache manager covering
	// the mechanism of creating pod/service corresponding to invoked function,
	// waiting for it be ready via channel and managing the cache.
	podManagementTests := []struct {
		name       string
		expectFail bool
		skip       bool
		kubeClient client.WithWatch
		cache      map[string]*podAndGRPCClient
	}{
		{
			name:       "Pod Exists but not in Cache",
			skip:       false,
			expectFail: false,
			cache:      blankCache,
			kubeClient: buildKubeClient(defaultPodObject, defaultServiceObject, defaultEndpointObject),
		},
		{
			name:       "Pod Exists and in Cache",
			skip:       false,
			expectFail: false,
			cache:      cacheWithDefaultPod,
			kubeClient: buildKubeClient(defaultPodObject, defaultServiceObject, defaultEndpointObject),
		},
		{
			name:       "Pod does not Exists and not in Cache",
			skip:       false,
			expectFail: false,
			cache:      blankCache,
			kubeClient: buildKubeClient(),
		},
		{
			name:       "Pod does not Exists but in Cache",
			skip:       false,
			expectFail: false,
			cache:      cacheWithDefaultPod,
			kubeClient: fake.NewClientBuilder().
				WithInterceptorFuncs(interceptor.Funcs{Create: fakeClientCreateFixInterceptor}).Build(),
		},
		{
			name:       "Pod does not Exists but Service Exists and not in Cache",
			skip:       false,
			expectFail: false,
			cache:      blankCache,
			kubeClient: buildKubeClient(defaultServiceObject, defaultEndpointObject),
		},
		{
			name:       "Pod Exists but Service does not Exists and not in Cache",
			skip:       false,
			expectFail: false,
			cache:      blankCache,
			kubeClient: buildKubeClient(defaultPodObject),
		},
	}

	//Set up the podmanager and podcachemanager
	podReadyCh := make(chan *imagePodAndGRPCClient)
	requestCh := make(chan *clientConnRequest)
	waitlists := make(map[string][]chan<- *clientConnAndError)

	pm := &podManager{
		namespace:               defaultNamespace,
		wrapperServerImage:      defaultWrapperServerImage,
		imageMetadataCache:      sync.Map{},
		podReadyTimeout:         2 * time.Second,
		managerNamespace:        defaultNamespace,
		enablePrivateRegistries: false,
		podReadyCh:              podReadyCh,
	}

	for k, v := range defaultImageMetadataCache {
		pm.imageMetadataCache.Store(k, v)
	}

	pcm := &podCacheManager{
		// Setting to 5 minutes to avoid GC invocation
		gcScanInterval: 5 * time.Minute,
		podTTL:         10 * time.Minute,
		requestCh:      requestCh,
		podReadyCh:     podReadyCh,
		waitlists:      waitlists,
		podManager:     pm,
	}

	go pcm.podCacheManager()

	for _, tt := range podManagementTests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.skip {
				t.SkipNow()
			}

			pcm.cache = make(map[string]*podAndGRPCClient)
			for k, v := range tt.cache {
				pcm.cache[k] = v
			}
			pcm.podManager.kubeClient = tt.kubeClient

			clientConn := make(chan *clientConnAndError)
			requestCh <- &clientConnRequest{defaultImageName, clientConn}

			select {
			case cc := <-clientConn:
				if !tt.expectFail && cc.err != nil {
					t.Errorf("Expected to get client connection, got error: %v", cc.err)
				} else if tt.expectFail && cc.err == nil {
					t.Errorf("Expected to get error, got client connection")
				} else if cc.err == nil {
					if cc.grpcClient == nil {
						t.Errorf("Expected to get grpc client, got nil")
					}
				}

			case <-time.After(5 * time.Second):
				t.Errorf("Timed out waiting for client connection")
			}
		})
	}

	oldPodObject := &corev1.Pod{}
	deepCopyObject(defaultPodObject, oldPodObject)
	// Set the pod Retention time annotation to be older than the current time
	podTimestamp := time.Now().Add(-1 * time.Minute).Unix()
	oldPodObject.Annotations[reclaimAfterAnnotation] = fmt.Sprintf("%d", podTimestamp)

	newPodObject := &corev1.Pod{}
	deepCopyObject(defaultPodObject, newPodObject)
	// Set the pod Retention time annotation to be later than the current time
	podTimestamp = time.Now().Add(1 * time.Minute).Unix()
	newPodObject.Annotations[reclaimAfterAnnotation] = fmt.Sprintf("%d", podTimestamp)

	podObjectInvalidRetentionTimestamp := &corev1.Pod{}
	deepCopyObject(defaultPodObject, podObjectInvalidRetentionTimestamp)
	podObjectInvalidRetentionTimestamp.Annotations[reclaimAfterAnnotation] = "dummy-value"

	// These tests focus on the pod cleanup / garbage collection logic in the pod
	// cache manager covering automated deletion of pod/service after retention period
	podCleanupTests := []struct {
		name                   string
		expectFail             bool
		skip                   bool
		kubeClient             client.WithWatch
		cache                  map[string]*podAndGRPCClient
		podShouldBeDeleted     bool
		serviceShouldBeDeleted bool
		cacheShouldBeEmpty     bool
	}{
		{
			name:                   "Garbage Collector - Expired Pod Exists and cleaned up",
			expectFail:             false,
			skip:                   false,
			kubeClient:             buildKubeClient(oldPodObject, defaultServiceObject, defaultEndpointObject),
			cache:                  cacheWithDefaultPod,
			podShouldBeDeleted:     true,
			serviceShouldBeDeleted: true,
			cacheShouldBeEmpty:     true,
		},
		{
			name:                   "Garbage Collector - New Pod Exists and not cleaned up",
			expectFail:             false,
			skip:                   false,
			kubeClient:             buildKubeClient(newPodObject, defaultServiceObject, defaultEndpointObject),
			cache:                  cacheWithDefaultPod,
			podShouldBeDeleted:     false,
			serviceShouldBeDeleted: false,
			cacheShouldBeEmpty:     false,
		},
		{
			name:                   "Garbage Collector - Non Annotated Pod Exists and not cleaned up",
			expectFail:             false,
			skip:                   false,
			kubeClient:             buildKubeClient(defaultPodObject, defaultServiceObject, defaultEndpointObject),
			cache:                  cacheWithDefaultPod,
			podShouldBeDeleted:     false,
			serviceShouldBeDeleted: false,
			cacheShouldBeEmpty:     false,
		},
		{
			name:                   "Garbage Collector - Annotated Pod Exists with invalid timestamp and not cleaned up",
			expectFail:             false,
			skip:                   false,
			kubeClient:             buildKubeClient(podObjectInvalidRetentionTimestamp, defaultServiceObject, defaultEndpointObject),
			cache:                  cacheWithDefaultPod,
			podShouldBeDeleted:     false,
			serviceShouldBeDeleted: false,
			cacheShouldBeEmpty:     false,
		},
	}

	for _, tt := range podCleanupTests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.skip {
				t.SkipNow()
			}

			pcm.cache = make(map[string]*podAndGRPCClient)
			for k, v := range tt.cache {
				pcm.cache[k] = v
			}

			pcm.podManager.kubeClient = tt.kubeClient

			// Trigger garbage collection
			pcm.garbageCollector()

			// Wait for 1 second as garbage collector purges pods in goroutine
			<-time.After(1 * time.Second)

			var pod corev1.Pod
			err := pcm.podManager.kubeClient.Get(context.Background(), client.ObjectKeyFromObject(defaultPodObject), &pod)
			if tt.podShouldBeDeleted && err == nil {
				t.Errorf("Expected pod to be deleted, but it still exists")
			} else if !tt.podShouldBeDeleted && err != nil && !errors.IsNotFound(err) {
				t.Errorf("Expected pod to exist, but got error: %v", err)
			}

			var service corev1.Service
			err = pcm.podManager.kubeClient.Get(context.Background(), client.ObjectKeyFromObject(defaultServiceObject), &service)
			if tt.serviceShouldBeDeleted && err == nil {
				t.Errorf("Expected service to be deleted, but it still exists")
			} else if !tt.serviceShouldBeDeleted && err != nil && !errors.IsNotFound(err) {
				t.Errorf("Expected service to exist, but got error: %v", err)
			}

			if tt.cacheShouldBeEmpty {
				if len(pcm.cache) != 0 {
					t.Errorf("Expected cache to be empty, but it has %d entries", len(pcm.cache))
				}
			} else {
				if len(pcm.cache) == 0 {
					t.Errorf("Expected cache to have entries, but it is empty")
				} else if len(pcm.cache) != 1 {
					t.Errorf("Expected cache to have 1 entry, but it has %d entries", len(pcm.cache))
				}
			}
		})
	}
}

// Test PodEvaluatorOptions parallel parameter fields
func TestPodEvaluatorOptions_ParallelParameters(t *testing.T) {
	opts := PodEvaluatorOptions{
		PodNamespace:               "test-ns",
		MaxParallelPodsPerFunction: 3,
		MaxWaitlistLength:          5,
	}

	if opts.MaxParallelPodsPerFunction != 3 {
		t.Errorf("MaxParallelPodsPerFunction = %d, want 3", opts.MaxParallelPodsPerFunction)
	}
	if opts.MaxWaitlistLength != 5 {
		t.Errorf("MaxWaitlistLength = %d, want 5", opts.MaxWaitlistLength)
	}
}

// Test podInfo TotalLoad calculation
func TestPodInfo_TotalLoad(t *testing.T) {
	tests := []struct {
		name         string
		waitlistLen  int
		ongoingCount int32
		want         int
	}{
		{"idle pod", 0, 0, 0},
		{"waitlist only", 5, 0, 5},
		{"ongoing only", 0, 3, 3},
		{"both", 5, 3, 8},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pi := newPodInfo()
			// simulate waitlist length
			for i := 0; i < tt.waitlistLen; i++ {
				pi.waitlist = append(pi.waitlist, make(chan *clientConnAndError))
			}
			pi.ongoingEvaluations.Store(tt.ongoingCount)

			got := pi.totalLoad()
			if got != tt.want {
				t.Errorf("totalLoad() = %d, want %d", got, tt.want)
			}
		})
	}
}

// Test findBestPod selection logic
func TestFindBestPod(t *testing.T) {
	tests := []struct {
		name        string
		podLoads    []int // load of each pod
		maxWaitlist int
		maxPods     int
		wantIdx     int
		wantSpawn   bool
	}{
		{"no pods", nil, 10, 3, 0, true},
		{"one idle pod", []int{0}, 10, 3, 0, false},
		{"select lowest load", []int{5, 2, 8}, 10, 3, 1, false},
		{"spawn new pod when all overloaded", []int{15, 12}, 10, 3, 2, true},
		{"no spawn when at max", []int{15, 12, 20}, 10, 3, 1, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var pods []*podInfo
			for _, load := range tt.podLoads {
				pi := newPodInfo()
				pi.ongoingEvaluations.Store(int32(load))
				pods = append(pods, pi)
			}

			gotIdx, gotSpawn := findBestPod(pods, tt.maxWaitlist, tt.maxPods)
			if gotIdx != tt.wantIdx {
				t.Errorf("findBestPod() idx = %d, want %d", gotIdx, tt.wantIdx)
			}
			if gotSpawn != tt.wantSpawn {
				t.Errorf("findBestPod() spawn = %v, want %v", gotSpawn, tt.wantSpawn)
			}
		})
	}
}

// Test podInfo has grpcClient and pod fields (required for CL-3)
func TestPodInfo_HasRequiredFields(t *testing.T) {
	address := net.JoinHostPort(defaultServiceIP, defaultWrapperServerPort)
	grpcConn, err := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("failed to create grpc client: %v", err)
	}

	pi := newPodInfo()
	pi.pod = client.ObjectKey{Name: "test-pod", Namespace: "test-ns"}
	pi.grpcClient = grpcConn

	if pi.pod.Name != "test-pod" {
		t.Errorf("pod.Name = %s, want test-pod", pi.pod.Name)
	}
	if pi.pod.Namespace != "test-ns" {
		t.Errorf("pod.Namespace = %s, want test-ns", pi.pod.Namespace)
	}
	if pi.grpcClient == nil {
		t.Error("grpcClient should not be nil")
	}
}

// Test podCacheManager has multi-pod config fields
func TestPodCacheManager_MultiPodConfig(t *testing.T) {
	pcm := &podCacheManager{
		maxParallelPods: 3,
		maxWaitlistLen:  10,
	}

	if pcm.maxParallelPods != 3 {
		t.Errorf("maxParallelPods = %d, want 3", pcm.maxParallelPods)
	}
	if pcm.maxWaitlistLen != 10 {
		t.Errorf("maxWaitlistLen = %d, want 10", pcm.maxWaitlistLen)
	}
}

// Test podCacheManager has pods map
func TestPodCacheManager_PodsMap(t *testing.T) {
	pcm := &podCacheManager{
		pods: make(map[string][]*podInfo),
	}

	// add a pod
	image := "test-image"
	pi := newPodInfo()
	pi.pod = client.ObjectKey{Name: "pod1", Namespace: "ns"}
	pcm.pods[image] = append(pcm.pods[image], pi)

	if len(pcm.pods[image]) != 1 {
		t.Errorf("pods[%s] len = %d, want 1", image, len(pcm.pods[image]))
	}

	// add second pod
	pi2 := newPodInfo()
	pi2.pod = client.ObjectKey{Name: "pod2", Namespace: "ns"}
	pcm.pods[image] = append(pcm.pods[image], pi2)

	if len(pcm.pods[image]) != 2 {
		t.Errorf("pods[%s] len = %d, want 2", image, len(pcm.pods[image]))
	}
}

// Test handleMultiPodRequest uses findBestPod to select best pod
func TestHandleMultiPodRequest_SelectsBestPod(t *testing.T) {
	address := net.JoinHostPort(defaultServiceIP, defaultWrapperServerPort)
	grpcConn, _ := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()))

	// create pod objects for the fake client
	pod1 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "pod1", Namespace: "ns"},
		Status:     corev1.PodStatus{Phase: corev1.PodRunning},
	}
	pod2 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "pod2", Namespace: "ns"},
		Status:     corev1.PodStatus{Phase: corev1.PodRunning},
	}

	// create two pods, second has lower load
	pi1 := newPodInfo()
	pi1.grpcClient = grpcConn
	pi1.pod = client.ObjectKey{Name: "pod1", Namespace: "ns"}
	pi1.ongoingEvaluations.Store(5) // higher load

	pi2 := newPodInfo()
	pi2.grpcClient = grpcConn
	pi2.pod = client.ObjectKey{Name: "pod2", Namespace: "ns"}
	pi2.ongoingEvaluations.Store(1) // lower load

	pcm := &podCacheManager{
		pods:            map[string][]*podInfo{"test-image": {pi1, pi2}},
		maxParallelPods: 3,
		maxWaitlistLen:  10,
		podTTL:          30 * time.Minute,
		podManager: &podManager{
			kubeClient: fake.NewClientBuilder().WithObjects(pod1, pod2).Build(),
			namespace:  "ns",
		},
	}

	// send request, should select pi2 (lower load)
	respCh := make(chan *clientConnAndError, 1)
	req := &clientConnRequest{image: "test-image", grpcClientCh: respCh}

	pcm.handleMultiPodRequest(req)

	select {
	case resp := <-respCh:
		if resp.err != nil {
			t.Errorf("unexpected error: %v", resp.err)
		}
		if resp.grpcClient != grpcConn {
			t.Error("expected to get grpcClient")
		}
	default:
		t.Error("expected response on channel")
	}
}

// Test handleMultiPodRequest spawns new pod when none available
func TestHandleMultiPodRequest_SpawnsNewPod(t *testing.T) {
	pcm := &podCacheManager{
		pods:            map[string][]*podInfo{},
		maxParallelPods: 3,
		maxWaitlistLen:  10,
		podManager: &podManager{
			kubeClient: fake.NewClientBuilder().Build(),
			namespace:  defaultNamespace,
		},
	}

	respCh := make(chan *clientConnAndError, 1)
	req := &clientConnRequest{image: "new-image", grpcClientCh: respCh}

	pcm.handleMultiPodRequest(req)

	// should have created new podInfo
	if len(pcm.pods["new-image"]) != 1 {
		t.Errorf("expected 1 pod, got %d", len(pcm.pods["new-image"]))
	}

	// request should be in waitlist
	pi := pcm.pods["new-image"][0]
	if len(pi.waitlist) != 1 {
		t.Errorf("expected 1 waiter, got %d", len(pi.waitlist))
	}
}

// Test evictPodFromMultiPodCache removes the correct pod
func TestEvictPodFromMultiPodCache(t *testing.T) {
	pi1 := newPodInfo()
	pi1.pod = client.ObjectKey{Name: "pod1", Namespace: "ns"}

	pi2 := newPodInfo()
	pi2.pod = client.ObjectKey{Name: "pod2", Namespace: "ns"}

	pi3 := newPodInfo()
	pi3.pod = client.ObjectKey{Name: "pod3", Namespace: "ns"}

	pcm := &podCacheManager{
		pods: map[string][]*podInfo{
			"test-image": {pi1, pi2, pi3},
		},
	}

	// evict pod2
	pcm.evictPodFromMultiPodCache("test-image", "pod2")

	if len(pcm.pods["test-image"]) != 2 {
		t.Errorf("expected 2 pods, got %d", len(pcm.pods["test-image"]))
	}

	// verify remaining pods
	names := []string{}
	for _, pi := range pcm.pods["test-image"] {
		names = append(names, pi.pod.Name)
	}
	if names[0] != "pod1" || names[1] != "pod3" {
		t.Errorf("expected [pod1, pod3], got %v", names)
	}
}

// Test evictPodFromMultiPodCache with non-existent pod does nothing
func TestEvictPodFromMultiPodCache_NotFound(t *testing.T) {
	pi1 := newPodInfo()
	pi1.pod = client.ObjectKey{Name: "pod1", Namespace: "ns"}

	pcm := &podCacheManager{
		pods: map[string][]*podInfo{
			"test-image": {pi1},
		},
	}

	// try to evict non-existent pod
	pcm.evictPodFromMultiPodCache("test-image", "not-exists")

	// should still have 1 pod
	if len(pcm.pods["test-image"]) != 1 {
		t.Errorf("expected 1 pod, got %d", len(pcm.pods["test-image"]))
	}
}

// Test isCurrentCachedPod checks both single and multi-pod cache
func TestIsCurrentCachedPod_MultiPod(t *testing.T) {
	pi := newPodInfo()
	pi.pod = client.ObjectKey{Name: "multipod1", Namespace: "ns"}

	pcm := &podCacheManager{
		cache: map[string]*podAndGRPCClient{},
		pods: map[string][]*podInfo{
			"test-image": {pi},
		},
	}

	pod := &corev1.Pod{}
	pod.Name = "multipod1"
	pod.Namespace = "ns"

	// should find in multi-pod cache
	if !isCurrentCachedPod(pcm, "test-image", pod) {
		t.Error("expected to find pod in multi-pod cache")
	}

	// should not find non-existent pod
	pod.Name = "other-pod"
	if isCurrentCachedPod(pcm, "test-image", pod) {
		t.Error("should not find non-existent pod")
	}
}

// Test handleMultiPodReady assigns grpcClient to waiting pod
func TestHandleMultiPodReady_Success(t *testing.T) {
	address := net.JoinHostPort(defaultServiceIP, defaultWrapperServerPort)
	grpcConn, _ := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()))

	// create pod waiting for grpcClient
	pi := newPodInfo()
	respCh := make(chan *clientConnAndError, 1)
	pi.waitlist = append(pi.waitlist, respCh)

	pcm := &podCacheManager{
		pods: map[string][]*podInfo{
			"test-image": {pi},
		},
	}

	// simulate pod ready
	resp := &imagePodAndGRPCClient{
		image: "test-image",
		podAndGRPCClient: &podAndGRPCClient{
			grpcClient: grpcConn,
			pod:        client.ObjectKey{Name: "pod1", Namespace: "ns"},
		},
	}

	pcm.handleMultiPodReady(resp)

	// verify grpcClient assigned
	if pi.grpcClient != grpcConn {
		t.Error("expected grpcClient to be assigned")
	}
	if pi.pod.Name != "pod1" {
		t.Errorf("expected pod name pod1, got %s", pi.pod.Name)
	}

	// verify waiter notified
	select {
	case result := <-respCh:
		if result.err != nil {
			t.Errorf("unexpected error: %v", result.err)
		}
		if result.grpcClient != grpcConn {
			t.Error("expected grpcClient in response")
		}
	default:
		t.Error("expected response on channel")
	}

	// verify waitlist cleared
	if len(pi.waitlist) != 0 {
		t.Errorf("expected waitlist cleared, got %d", len(pi.waitlist))
	}
}

// Test handleMultiPodReady handles error from pod manager
func TestHandleMultiPodReady_Error(t *testing.T) {
	// create pod waiting for grpcClient
	pi := newPodInfo()
	respCh := make(chan *clientConnAndError, 1)
	pi.waitlist = append(pi.waitlist, respCh)

	pcm := &podCacheManager{
		pods: map[string][]*podInfo{
			"test-image": {pi},
		},
	}

	// simulate error
	resp := &imagePodAndGRPCClient{
		image: "test-image",
		err:   fmt.Errorf("pod creation failed"),
	}

	pcm.handleMultiPodReady(resp)

	// verify waiter notified with error
	select {
	case result := <-respCh:
		if result.err == nil {
			t.Error("expected error in response")
		}
	default:
		t.Error("expected response on channel")
	}

	// verify waitlist cleared
	if len(pi.waitlist) != 0 {
		t.Errorf("expected waitlist cleared, got %d", len(pi.waitlist))
	}
}

// Test handleMultiPodReady skips pods that already have grpcClient
func TestHandleMultiPodReady_SkipsReadyPod(t *testing.T) {
	address := net.JoinHostPort(defaultServiceIP, defaultWrapperServerPort)
	existingConn, _ := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	newConn, _ := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()))

	// first pod already has grpcClient
	pi1 := newPodInfo()
	pi1.grpcClient = existingConn

	// second pod is waiting
	pi2 := newPodInfo()
	respCh := make(chan *clientConnAndError, 1)
	pi2.waitlist = append(pi2.waitlist, respCh)

	pcm := &podCacheManager{
		pods: map[string][]*podInfo{
			"test-image": {pi1, pi2},
		},
	}

	resp := &imagePodAndGRPCClient{
		image: "test-image",
		podAndGRPCClient: &podAndGRPCClient{
			grpcClient: newConn,
			pod:        client.ObjectKey{Name: "pod2", Namespace: "ns"},
		},
	}

	pcm.handleMultiPodReady(resp)

	// pi1 should keep existing grpcClient
	if pi1.grpcClient != existingConn {
		t.Error("pi1 should keep existing grpcClient")
	}

	// pi2 should get new grpcClient
	if pi2.grpcClient != newConn {
		t.Error("pi2 should get new grpcClient")
	}
}

// Test redistributeWaitlist redistributes to existing ready pod
func TestRedistributeWaitlist_ToReadyPod(t *testing.T) {
	address := net.JoinHostPort(defaultServiceIP, defaultWrapperServerPort)
	grpcConn, _ := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()))

	// existing pod with grpcClient
	existingPod := newPodInfo()
	existingPod.grpcClient = grpcConn
	existingPod.pod = client.ObjectKey{Name: "existing", Namespace: "ns"}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "existing", Namespace: "ns"},
		Status:     corev1.PodStatus{Phase: corev1.PodRunning},
	}

	pcm := &podCacheManager{
		pods:            map[string][]*podInfo{"test-image": {existingPod}},
		maxParallelPods: 3,
		maxWaitlistLen:  10,
		podTTL:          30 * time.Minute,
		podManager: &podManager{
			kubeClient: fake.NewClientBuilder().WithObjects(pod).Build(),
			namespace:  "ns",
		},
	}

	// waitlist from failed pod
	respCh := make(chan *clientConnAndError, 1)
	waitlist := []chan<- *clientConnAndError{respCh}

	pcm.redistributeWaitlist("test-image", waitlist)

	// should receive grpcClient from existing pod
	select {
	case resp := <-respCh:
		if resp.err != nil {
			t.Errorf("unexpected error: %v", resp.err)
		}
		if resp.grpcClient != grpcConn {
			t.Error("expected grpcClient from existing pod")
		}
	default:
		t.Error("expected response on channel")
	}
}

// Test redistributeWaitlist spawns new pod when none available
func TestRedistributeWaitlist_SpawnsNewPod(t *testing.T) {
	pcm := &podCacheManager{
		pods:            map[string][]*podInfo{},
		maxParallelPods: 3,
		maxWaitlistLen:  10,
		podTTL:          30 * time.Minute,
		podManager: &podManager{
			kubeClient: fake.NewClientBuilder().Build(),
			namespace:  defaultNamespace,
		},
	}

	// waitlist from failed pod
	respCh := make(chan *clientConnAndError, 1)
	waitlist := []chan<- *clientConnAndError{respCh}

	pcm.redistributeWaitlist("test-image", waitlist)

	// should have created new podInfo with waitlist
	if len(pcm.pods["test-image"]) != 1 {
		t.Errorf("expected 1 pod, got %d", len(pcm.pods["test-image"]))
	}

	pi := pcm.pods["test-image"][0]
	if len(pi.waitlist) != 1 {
		t.Errorf("expected 1 waiter in new pod, got %d", len(pi.waitlist))
	}
}

// Test removePodAtIndex redistributes waitlist
func TestRemovePodAtIndex_RedistributesWaitlist(t *testing.T) {
	address := net.JoinHostPort(defaultServiceIP, defaultWrapperServerPort)
	grpcConn, _ := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()))

	// ready pod
	readyPod := newPodInfo()
	readyPod.grpcClient = grpcConn
	readyPod.pod = client.ObjectKey{Name: "ready", Namespace: "ns"}

	// failed pod with waitlist
	failedPod := newPodInfo()
	respCh := make(chan *clientConnAndError, 1)
	failedPod.waitlist = append(failedPod.waitlist, respCh)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "ready", Namespace: "ns"},
		Status:     corev1.PodStatus{Phase: corev1.PodRunning},
	}

	pcm := &podCacheManager{
		pods:            map[string][]*podInfo{"test-image": {readyPod, failedPod}},
		maxParallelPods: 3,
		maxWaitlistLen:  10,
		podTTL:          30 * time.Minute,
		podManager: &podManager{
			kubeClient: fake.NewClientBuilder().WithObjects(pod).Build(),
			namespace:  "ns",
		},
	}

	// remove failed pod (index 1)
	pcm.removePodAtIndex("test-image", 1)

	// should have only 1 pod now
	if len(pcm.pods["test-image"]) != 1 {
		t.Errorf("expected 1 pod, got %d", len(pcm.pods["test-image"]))
	}

	// waitlist item should have been redistributed to ready pod
	select {
	case resp := <-respCh:
		if resp.err != nil {
			t.Errorf("unexpected error: %v", resp.err)
		}
		if resp.grpcClient != grpcConn {
			t.Error("expected grpcClient from ready pod")
		}
	default:
		t.Error("expected response on channel")
	}
}

// Test validatePodHealth returns false for empty pod name
func TestValidatePodHealth_EmptyPodName(t *testing.T) {
	pi := newPodInfo()
	// pod.Name is empty by default

	pcm := &podCacheManager{
		pods: map[string][]*podInfo{"test-image": {pi}},
		podManager: &podManager{
			kubeClient: fake.NewClientBuilder().Build(),
		},
	}

	result := pcm.validatePodHealth("test-image", pi, 0)
	if result {
		t.Error("expected false for empty pod name")
	}
}

// Test validatePodHealth returns false and removes pod when not found
func TestValidatePodHealth_PodNotFound(t *testing.T) {
	pi := newPodInfo()
	pi.pod = client.ObjectKey{Name: "nonexistent", Namespace: "ns"}

	pcm := &podCacheManager{
		pods: map[string][]*podInfo{"test-image": {pi}},
		podManager: &podManager{
			kubeClient: fake.NewClientBuilder().Build(), // no pod in client
		},
	}

	result := pcm.validatePodHealth("test-image", pi, 0)
	if result {
		t.Error("expected false for pod not found")
	}

	// pod should be removed from cache
	if len(pcm.pods["test-image"]) != 0 {
		t.Errorf("expected pod to be removed, got %d pods", len(pcm.pods["test-image"]))
	}
}

// Test validatePodHealth returns false for pod being deleted
func TestValidatePodHealth_PodBeingDeleted(t *testing.T) {
	// create pod first, then update with deletionTimestamp
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "deleting-pod",
			Namespace:  "ns",
			Finalizers: []string{"test-finalizer"}, // need finalizer to set deletionTimestamp
		},
		Status: corev1.PodStatus{Phase: corev1.PodRunning},
	}

	kubeClient := fake.NewClientBuilder().WithObjects(pod).Build()

	// delete the pod to set deletionTimestamp
	_ = kubeClient.Delete(context.Background(), pod)

	pi := newPodInfo()
	pi.pod = client.ObjectKey{Name: "deleting-pod", Namespace: "ns"}

	pcm := &podCacheManager{
		pods: map[string][]*podInfo{"test-image": {pi}},
		podManager: &podManager{
			kubeClient: kubeClient,
		},
	}

	result := pcm.validatePodHealth("test-image", pi, 0)
	if result {
		t.Error("expected false for pod being deleted")
	}

	// pod should be removed from cache
	if len(pcm.pods["test-image"]) != 0 {
		t.Errorf("expected pod to be removed, got %d pods", len(pcm.pods["test-image"]))
	}
}

// Test validatePodHealth returns false for failed pod
func TestValidatePodHealth_PodFailed(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "failed-pod", Namespace: "ns"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "fn", Image: "test-image"}},
		},
		Status: corev1.PodStatus{Phase: corev1.PodFailed},
	}

	pi := newPodInfo()
	pi.pod = client.ObjectKey{Name: "failed-pod", Namespace: "ns"}

	pcm := &podCacheManager{
		pods:  map[string][]*podInfo{"test-image": {pi}},
		cache: map[string]*podAndGRPCClient{},
		podManager: &podManager{
			kubeClient: fake.NewClientBuilder().WithObjects(pod).Build(),
		},
	}

	result := pcm.validatePodHealth("test-image", pi, 0)
	if result {
		t.Error("expected false for failed pod")
	}

	// pod should be removed from cache
	if len(pcm.pods["test-image"]) != 0 {
		t.Errorf("expected pod to be removed, got %d pods", len(pcm.pods["test-image"]))
	}
}

// Test validatePodHealth returns true for healthy pod
func TestValidatePodHealth_Healthy(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "healthy-pod", Namespace: "ns"},
		Status:     corev1.PodStatus{Phase: corev1.PodRunning},
	}

	pi := newPodInfo()
	pi.pod = client.ObjectKey{Name: "healthy-pod", Namespace: "ns"}

	pcm := &podCacheManager{
		pods: map[string][]*podInfo{"test-image": {pi}},
		podManager: &podManager{
			kubeClient: fake.NewClientBuilder().WithObjects(pod).Build(),
		},
	}

	result := pcm.validatePodHealth("test-image", pi, 0)
	if !result {
		t.Error("expected true for healthy pod")
	}

	// pod should still be in cache
	if len(pcm.pods["test-image"]) != 1 {
		t.Errorf("expected pod to remain, got %d pods", len(pcm.pods["test-image"]))
	}
}

// Test removePodAtIndex with invalid index does nothing
func TestRemovePodAtIndex_InvalidIndex(t *testing.T) {
	pi := newPodInfo()
	pcm := &podCacheManager{
		pods: map[string][]*podInfo{"test-image": {pi}},
	}

	// negative index
	pcm.removePodAtIndex("test-image", -1)
	if len(pcm.pods["test-image"]) != 1 {
		t.Error("negative index should not remove pod")
	}

	// index out of range
	pcm.removePodAtIndex("test-image", 5)
	if len(pcm.pods["test-image"]) != 1 {
		t.Error("out of range index should not remove pod")
	}
}

// Test redistributeWaitlist adds to existing pod's waitlist when not ready
func TestRedistributeWaitlist_ToWaitingPod(t *testing.T) {
	// existing pod without grpcClient (still initializing)
	existingPod := newPodInfo()
	existingPod.pod = client.ObjectKey{Name: "waiting", Namespace: "ns"}
	// no grpcClient set

	pcm := &podCacheManager{
		pods:            map[string][]*podInfo{"test-image": {existingPod}},
		maxParallelPods: 3,
		maxWaitlistLen:  10,
		podTTL:          30 * time.Minute,
		podManager: &podManager{
			kubeClient: fake.NewClientBuilder().Build(),
			namespace:  "ns",
		},
	}

	// waitlist from failed pod
	respCh := make(chan *clientConnAndError, 1)
	waitlist := []chan<- *clientConnAndError{respCh}

	pcm.redistributeWaitlist("test-image", waitlist)

	// should be added to existing pod's waitlist
	if len(existingPod.waitlist) != 1 {
		t.Errorf("expected 1 waiter in existing pod, got %d", len(existingPod.waitlist))
	}
}

// Test handleMultiPodRequest re-selects pod when health check fails
func TestHandleMultiPodRequest_ReselectsWhenHealthCheckFails(t *testing.T) {
	address := net.JoinHostPort(defaultServiceIP, defaultWrapperServerPort)
	grpcConn, _ := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()))

	// healthy pod
	healthyPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "healthy", Namespace: "ns"},
		Status:     corev1.PodStatus{Phase: corev1.PodRunning},
	}

	// unhealthy pod (will fail validation - not found in client)
	unhealthyPi := newPodInfo()
	unhealthyPi.grpcClient = grpcConn
	unhealthyPi.pod = client.ObjectKey{Name: "unhealthy", Namespace: "ns"} // not in client

	// healthy backup pod
	healthyPi := newPodInfo()
	healthyPi.grpcClient = grpcConn
	healthyPi.pod = client.ObjectKey{Name: "healthy", Namespace: "ns"}
	healthyPi.ongoingEvaluations.Store(5) // higher load, would be second choice

	pcm := &podCacheManager{
		pods:            map[string][]*podInfo{"test-image": {unhealthyPi, healthyPi}},
		maxParallelPods: 3,
		maxWaitlistLen:  10,
		podTTL:          30 * time.Minute,
		podManager: &podManager{
			kubeClient: fake.NewClientBuilder().WithObjects(healthyPod).Build(), // only healthy pod exists
			namespace:  "ns",
		},
	}

	respCh := make(chan *clientConnAndError, 1)
	req := &clientConnRequest{image: "test-image", grpcClientCh: respCh}

	pcm.handleMultiPodRequest(req)

	// should get grpcClient from healthy pod after unhealthy was removed
	select {
	case resp := <-respCh:
		if resp.err != nil {
			t.Errorf("unexpected error: %v", resp.err)
		}
		if resp.grpcClient != grpcConn {
			t.Error("expected grpcClient from healthy pod")
		}
	default:
		t.Error("expected response on channel - request would have been lost without bug fix!")
	}

	// unhealthy pod should have been removed
	if len(pcm.pods["test-image"]) != 1 {
		t.Errorf("expected 1 pod remaining, got %d", len(pcm.pods["test-image"]))
	}
}

// Test handleMultiPodRequest spawns new pod when all pods fail health check
func TestHandleMultiPodRequest_SpawnsNewWhenAllUnhealthy(t *testing.T) {
	address := net.JoinHostPort(defaultServiceIP, defaultWrapperServerPort)
	grpcConn, _ := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()))

	// unhealthy pod (will fail validation - not found in client)
	unhealthyPi := newPodInfo()
	unhealthyPi.grpcClient = grpcConn
	unhealthyPi.pod = client.ObjectKey{Name: "unhealthy", Namespace: "ns"} // not in client

	pcm := &podCacheManager{
		pods:            map[string][]*podInfo{"test-image": {unhealthyPi}},
		maxParallelPods: 3,
		maxWaitlistLen:  10,
		podTTL:          30 * time.Minute,
		podManager: &podManager{
			kubeClient: fake.NewClientBuilder().Build(), // no pods exist
			namespace:  "ns",
		},
	}

	respCh := make(chan *clientConnAndError, 1)
	req := &clientConnRequest{image: "test-image", grpcClientCh: respCh}

	pcm.handleMultiPodRequest(req)

	// should have spawned new pod and added request to waitlist
	if len(pcm.pods["test-image"]) != 1 {
		t.Errorf("expected 1 new pod, got %d", len(pcm.pods["test-image"]))
	}

	newPod := pcm.pods["test-image"][0]
	if len(newPod.waitlist) != 1 {
		t.Errorf("expected request in waitlist, got %d", len(newPod.waitlist))
	}
}

// Test redistributeWaitlist spawns additional pod when overloaded
func TestRedistributeWaitlist_SpawnsAdditionalPod(t *testing.T) {
	address := net.JoinHostPort(defaultServiceIP, defaultWrapperServerPort)
	grpcConn, _ := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()))

	// existing pod with high load
	existingPod := newPodInfo()
	existingPod.grpcClient = grpcConn
	existingPod.pod = client.ObjectKey{Name: "busy", Namespace: "ns"}
	existingPod.ongoingEvaluations.Store(15) // high load

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "busy", Namespace: "ns"},
		Status:     corev1.PodStatus{Phase: corev1.PodRunning},
	}

	pcm := &podCacheManager{
		pods:            map[string][]*podInfo{"test-image": {existingPod}},
		maxParallelPods: 3,
		maxWaitlistLen:  5, // low threshold
		podTTL:          30 * time.Minute,
		podManager: &podManager{
			kubeClient: fake.NewClientBuilder().WithObjects(pod).Build(),
			namespace:  "ns",
		},
	}

	// multiple waitlist items
	waitlist := make([]chan<- *clientConnAndError, 3)
	for i := range waitlist {
		waitlist[i] = make(chan *clientConnAndError, 1)
	}

	pcm.redistributeWaitlist("test-image", waitlist)

	// should have spawned additional pod(s)
	if len(pcm.pods["test-image"]) < 2 {
		t.Errorf("expected at least 2 pods, got %d", len(pcm.pods["test-image"]))
	}
}

// Test evictPodFromMultiPodCache redistributes waitlist
func TestEvictPodFromMultiPodCache_RedistributesWaitlist(t *testing.T) {
	address := net.JoinHostPort(defaultServiceIP, defaultWrapperServerPort)
	grpcConn, _ := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()))

	// pod with waitlist
	pi := newPodInfo()
	pi.grpcClient = grpcConn
	pi.pod = client.ObjectKey{Name: "pod-to-evict", Namespace: "ns"}
	respCh := make(chan *clientConnAndError, 1)
	pi.waitlist = []chan<- *clientConnAndError{respCh}

	// healthy backup pod
	backupPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "backup", Namespace: "ns"},
		Status:     corev1.PodStatus{Phase: corev1.PodRunning},
	}
	backupPi := newPodInfo()
	backupPi.grpcClient = grpcConn
	backupPi.pod = client.ObjectKey{Name: "backup", Namespace: "ns"}

	pcm := &podCacheManager{
		pods:            map[string][]*podInfo{"test-image": {pi, backupPi}},
		maxParallelPods: 3,
		maxWaitlistLen:  10,
		podTTL:          30 * time.Minute,
		podManager: &podManager{
			kubeClient: fake.NewClientBuilder().WithObjects(backupPod).Build(),
			namespace:  "ns",
		},
	}

	// evict the first pod
	pcm.evictPodFromMultiPodCache("test-image", "pod-to-evict")

	// pod should be removed
	if len(pcm.pods["test-image"]) != 1 {
		t.Errorf("expected 1 pod remaining, got %d", len(pcm.pods["test-image"]))
	}

	// waitlist should have been redistributed (response sent to channel)
	select {
	case resp := <-respCh:
		if resp.err != nil {
			t.Errorf("unexpected error in redistributed request: %v", resp.err)
		}
	default:
		t.Error("waitlist item was not redistributed")
	}
}

// Test handleMultiPodRequest skips unhealthy pod after transient error
func TestHandleMultiPodRequest_SkipsUnhealthyPodOnTransientError(t *testing.T) {
	address := net.JoinHostPort(defaultServiceIP, defaultWrapperServerPort)
	grpcConn, _ := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()))

	// pod that will fail health check with transient error (return error, don't remove)
	transientErrorPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "transient-error", Namespace: "ns"},
		Status:     corev1.PodStatus{Phase: corev1.PodRunning},
	}
	pi1 := newPodInfo()
	pi1.grpcClient = grpcConn
	pi1.pod = client.ObjectKey{Name: "transient-error", Namespace: "ns"}

	// healthy backup pod
	healthyPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "healthy", Namespace: "ns"},
		Status:     corev1.PodStatus{Phase: corev1.PodRunning},
	}
	pi2 := newPodInfo()
	pi2.grpcClient = grpcConn
	pi2.pod = client.ObjectKey{Name: "healthy", Namespace: "ns"}
	pi2.ongoingEvaluations.Store(5) // higher load so pi1 is selected first

	// kubeClient that returns error for first pod
	callCount := 0
	kubeClient := fake.NewClientBuilder().
		WithObjects(transientErrorPod, healthyPod).
		WithInterceptorFuncs(interceptor.Funcs{
			Get: func(ctx context.Context, client client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				callCount++
				if key.Name == "transient-error" && callCount == 1 {
					return fmt.Errorf("transient network error")
				}
				return client.Get(ctx, key, obj, opts...)
			},
		}).Build()

	pcm := &podCacheManager{
		pods:            map[string][]*podInfo{"test-image": {pi1, pi2}},
		maxParallelPods: 3,
		maxWaitlistLen:  10,
		podTTL:          30 * time.Minute,
		podManager: &podManager{
			kubeClient: kubeClient,
			namespace:  "ns",
		},
	}

	respCh := make(chan *clientConnAndError, 1)
	req := &clientConnRequest{image: "test-image", grpcClientCh: respCh}

	pcm.handleMultiPodRequest(req)

	// should have received response from healthy pod
	select {
	case resp := <-respCh:
		if resp.err != nil {
			t.Errorf("unexpected error: %v", resp.err)
		}
		if resp.grpcClient != grpcConn {
			t.Error("expected grpcClient from healthy pod")
		}
	default:
		t.Error("expected response on channel")
	}
}

// Test redistributeWaitlist validates health before sending
func TestRedistributeWaitlist_ValidatesHealth(t *testing.T) {
	address := net.JoinHostPort(defaultServiceIP, defaultWrapperServerPort)
	grpcConn, _ := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()))

	// unhealthy pod (not found in kubeClient)
	unhealthyPi := newPodInfo()
	unhealthyPi.grpcClient = grpcConn
	unhealthyPi.pod = client.ObjectKey{Name: "unhealthy", Namespace: "ns"}

	// healthy pod
	healthyPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "healthy", Namespace: "ns"},
		Status:     corev1.PodStatus{Phase: corev1.PodRunning},
	}
	healthyPi := newPodInfo()
	healthyPi.grpcClient = grpcConn
	healthyPi.pod = client.ObjectKey{Name: "healthy", Namespace: "ns"}
	healthyPi.ongoingEvaluations.Store(5) // higher load

	pcm := &podCacheManager{
		pods:            map[string][]*podInfo{"test-image": {unhealthyPi, healthyPi}},
		maxParallelPods: 3,
		maxWaitlistLen:  10,
		podTTL:          30 * time.Minute,
		podManager: &podManager{
			kubeClient: fake.NewClientBuilder().WithObjects(healthyPod).Build(), // only healthy pod exists
			namespace:  "ns",
		},
	}

	respCh := make(chan *clientConnAndError, 1)
	waitlist := []chan<- *clientConnAndError{respCh}

	pcm.redistributeWaitlist("test-image", waitlist)

	// should receive response from healthy pod (after unhealthy is skipped)
	select {
	case resp := <-respCh:
		if resp.err != nil {
			t.Errorf("unexpected error: %v", resp.err)
		}
	default:
		t.Error("expected response on channel")
	}
}

// Test handleMultiPodRequest with multiple consecutive health check failures
func TestHandleMultiPodRequest_MultipleHealthCheckFailures(t *testing.T) {
	address := net.JoinHostPort(defaultServiceIP, defaultWrapperServerPort)
	grpcConn, _ := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()))

	// Three pods: first two will fail health check, third is healthy
	pi1 := newPodInfo()
	pi1.grpcClient = grpcConn
	pi1.pod = client.ObjectKey{Name: "unhealthy1", Namespace: "ns"}

	pi2 := newPodInfo()
	pi2.grpcClient = grpcConn
	pi2.pod = client.ObjectKey{Name: "unhealthy2", Namespace: "ns"}
	pi2.ongoingEvaluations.Store(1) // slightly higher load

	pi3 := newPodInfo()
	pi3.grpcClient = grpcConn
	pi3.pod = client.ObjectKey{Name: "healthy", Namespace: "ns"}
	pi3.ongoingEvaluations.Store(2) // highest load

	// Only healthy pod exists in kubeClient
	healthyPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "healthy", Namespace: "ns"},
		Status:     corev1.PodStatus{Phase: corev1.PodRunning},
	}

	pcm := &podCacheManager{
		pods:            map[string][]*podInfo{"test-image": {pi1, pi2, pi3}},
		maxParallelPods: 5,
		maxWaitlistLen:  10,
		podTTL:          30 * time.Minute,
		podManager: &podManager{
			kubeClient: fake.NewClientBuilder().WithObjects(healthyPod).Build(),
			namespace:  "ns",
		},
	}

	respCh := make(chan *clientConnAndError, 1)
	req := &clientConnRequest{image: "test-image", grpcClientCh: respCh}

	pcm.handleMultiPodRequest(req)

	// should eventually get response from healthy pod
	select {
	case resp := <-respCh:
		if resp.err != nil {
			t.Errorf("unexpected error: %v", resp.err)
		}
	default:
		t.Error("expected response on channel")
	}

	// unhealthy pods should have been removed
	if len(pcm.pods["test-image"]) != 1 {
		t.Errorf("expected 1 pod remaining, got %d", len(pcm.pods["test-image"]))
	}
}

// Test handleMultiPodRequest when all pods with grpcClient are unhealthy
func TestHandleMultiPodRequest_AllReadyPodsUnhealthy(t *testing.T) {
	address := net.JoinHostPort(defaultServiceIP, defaultWrapperServerPort)
	grpcConn, _ := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()))

	// Two pods with grpcClient, both unhealthy
	pi1 := newPodInfo()
	pi1.grpcClient = grpcConn
	pi1.pod = client.ObjectKey{Name: "unhealthy1", Namespace: "ns"}

	pi2 := newPodInfo()
	pi2.grpcClient = grpcConn
	pi2.pod = client.ObjectKey{Name: "unhealthy2", Namespace: "ns"}

	// One pod without grpcClient (initializing)
	pi3 := newPodInfo()
	pi3.pod = client.ObjectKey{Name: "initializing", Namespace: "ns"}

	pcm := &podCacheManager{
		pods:            map[string][]*podInfo{"test-image": {pi1, pi2, pi3}},
		maxParallelPods: 5,
		maxWaitlistLen:  10,
		podTTL:          30 * time.Minute,
		podManager: &podManager{
			kubeClient: fake.NewClientBuilder().Build(), // no pods exist
			namespace:  "ns",
		},
	}

	respCh := make(chan *clientConnAndError, 1)
	req := &clientConnRequest{image: "test-image", grpcClientCh: respCh}

	pcm.handleMultiPodRequest(req)

	// should be added to waitlist of the initializing pod
	if len(pi3.waitlist) != 1 {
		t.Errorf("expected 1 item in pi3 waitlist, got %d", len(pi3.waitlist))
	}
}

// Test redistributeWaitlist with all pods failing health checks
func TestRedistributeWaitlist_AllPodsFailHealthCheck(t *testing.T) {
	address := net.JoinHostPort(defaultServiceIP, defaultWrapperServerPort)
	grpcConn, _ := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()))

	// Pod that will fail health check
	pi := newPodInfo()
	pi.grpcClient = grpcConn
	pi.pod = client.ObjectKey{Name: "unhealthy", Namespace: "ns"}

	pcm := &podCacheManager{
		pods:            map[string][]*podInfo{"test-image": {pi}},
		maxParallelPods: 3,
		maxWaitlistLen:  10,
		podTTL:          30 * time.Minute,
		podManager: &podManager{
			kubeClient: fake.NewClientBuilder().Build(), // no pods exist
			namespace:  "ns",
		},
	}

	respCh := make(chan *clientConnAndError, 1)
	waitlist := []chan<- *clientConnAndError{respCh}

	pcm.redistributeWaitlist("test-image", waitlist)

	// after unhealthy pod removed, new pod should be spawned
	if len(pcm.pods["test-image"]) != 1 {
		t.Errorf("expected 1 new pod, got %d", len(pcm.pods["test-image"]))
	}

	// request should be in new pod's waitlist
	newPod := pcm.pods["test-image"][0]
	if len(newPod.waitlist) != 1 {
		t.Errorf("expected 1 item in waitlist, got %d", len(newPod.waitlist))
	}
}

// Test handleMultiPodRequest when pod has grpcClient but empty pod name
func TestHandleMultiPodRequest_EmptyPodName(t *testing.T) {
	address := net.JoinHostPort(defaultServiceIP, defaultWrapperServerPort)
	grpcConn, _ := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()))

	// Pod with grpcClient but empty pod name (edge case)
	pi := newPodInfo()
	pi.grpcClient = grpcConn
	// pi.pod is zero value (empty name)

	pcm := &podCacheManager{
		pods:            map[string][]*podInfo{"test-image": {pi}},
		maxParallelPods: 3,
		maxWaitlistLen:  10,
		podTTL:          30 * time.Minute,
		podManager: &podManager{
			kubeClient: fake.NewClientBuilder().Build(),
			namespace:  "ns",
		},
	}

	respCh := make(chan *clientConnAndError, 1)
	req := &clientConnRequest{image: "test-image", grpcClientCh: respCh}

	pcm.handleMultiPodRequest(req)

	// validatePodHealth returns false for empty pod name
	// should spawn new pod and add to waitlist
	if len(pcm.pods["test-image"]) < 1 {
		t.Error("expected at least 1 pod")
	}
}

// Test redistributeWaitlist when pod goes to waitlist (grpcClient nil) after health check loop
func TestRedistributeWaitlist_FallsToWaitlistAfterHealthCheckLoop(t *testing.T) {
	address := net.JoinHostPort(defaultServiceIP, defaultWrapperServerPort)
	grpcConn, _ := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()))

	// unhealthy pod (will be removed by health check)
	unhealthyPi := newPodInfo()
	unhealthyPi.grpcClient = grpcConn
	unhealthyPi.pod = client.ObjectKey{Name: "unhealthy", Namespace: "ns"}

	// waiting pod (no grpcClient, still initializing)
	waitingPi := newPodInfo()
	waitingPi.pod = client.ObjectKey{Name: "waiting", Namespace: "ns"}
	waitingPi.ongoingEvaluations.Store(0) // lowest load when grpcClient nil

	pcm := &podCacheManager{
		pods:            map[string][]*podInfo{"test-image": {unhealthyPi, waitingPi}},
		maxParallelPods: 3,
		maxWaitlistLen:  10,
		podTTL:          30 * time.Minute,
		podManager: &podManager{
			kubeClient: fake.NewClientBuilder().Build(), // unhealthy pod doesn't exist
			namespace:  "ns",
		},
	}

	respCh := make(chan *clientConnAndError, 1)
	waitlist := []chan<- *clientConnAndError{respCh}

	pcm.redistributeWaitlist("test-image", waitlist)

	// unhealthy pod should be removed, request should be in waiting pod's waitlist
	if len(pcm.pods["test-image"]) != 1 {
		t.Errorf("expected 1 pod, got %d", len(pcm.pods["test-image"]))
	}

	// request should be added to the waiting pod's waitlist
	if len(waitingPi.waitlist) != 1 {
		t.Errorf("expected 1 item in waitingPi waitlist, got %d", len(waitingPi.waitlist))
	}
}

// Test handleMultiPodRequest targetIdx bounds check when at max pods
func TestHandleMultiPodRequest_TargetIdxBoundsCheck(t *testing.T) {
	address := net.JoinHostPort(defaultServiceIP, defaultWrapperServerPort)
	grpcConn, _ := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()))

	// create a single pod with grpcClient
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "pod1", Namespace: "ns"},
		Status:     corev1.PodStatus{Phase: corev1.PodRunning},
	}
	pi := newPodInfo()
	pi.grpcClient = grpcConn
	pi.pod = client.ObjectKey{Name: "pod1", Namespace: "ns"}
	pi.ongoingEvaluations.Store(15) // high load

	pcm := &podCacheManager{
		pods:            map[string][]*podInfo{"test-image": {pi}},
		maxParallelPods: 1, // at max, no spawn allowed
		maxWaitlistLen:  5, // lower than load, but can't spawn
		podTTL:          30 * time.Minute,
		podManager: &podManager{
			kubeClient: fake.NewClientBuilder().WithObjects(pod).Build(),
			namespace:  "ns",
		},
	}

	respCh := make(chan *clientConnAndError, 1)
	req := &clientConnRequest{image: "test-image", grpcClientCh: respCh}

	// findBestPod returns idx=0 (best available), shouldSpawn=false (at max)
	// the existing pod should be used
	pcm.handleMultiPodRequest(req)

	// should get grpcClient from the existing pod
	select {
	case resp := <-respCh:
		if resp.err != nil {
			t.Errorf("unexpected error: %v", resp.err)
		}
		if resp.grpcClient != grpcConn {
			t.Error("expected grpcClient from existing pod")
		}
	default:
		t.Error("expected response on channel")
	}
}

// Test handleMultiPodRequest when all pods removed during health check iteration
func TestHandleMultiPodRequest_AllPodsRemovedDuringIteration(t *testing.T) {
	address := net.JoinHostPort(defaultServiceIP, defaultWrapperServerPort)
	grpcConn, _ := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()))

	// only unhealthy pods (will all be removed)
	pi1 := newPodInfo()
	pi1.grpcClient = grpcConn
	pi1.pod = client.ObjectKey{Name: "unhealthy1", Namespace: "ns"}

	pcm := &podCacheManager{
		pods:            map[string][]*podInfo{"test-image": {pi1}},
		maxParallelPods: 3,
		maxWaitlistLen:  10,
		podTTL:          30 * time.Minute,
		podManager: &podManager{
			kubeClient: fake.NewClientBuilder().Build(), // no pods exist
			namespace:  "ns",
		},
	}

	respCh := make(chan *clientConnAndError, 1)
	req := &clientConnRequest{image: "test-image", grpcClientCh: respCh}

	pcm.handleMultiPodRequest(req)

	// should spawn new pod and add to waitlist
	if len(pcm.pods["test-image"]) < 1 {
		t.Error("expected at least 1 new pod")
	}

	newPod := pcm.pods["test-image"][0]
	if len(newPod.waitlist) != 1 {
		t.Errorf("expected 1 item in waitlist, got %d", len(newPod.waitlist))
	}
}

// Test redistributeWaitlist bounds check when findBestPod returns beyond slice length
func TestRedistributeWaitlist_TargetIdxBoundsCheck(t *testing.T) {
	address := net.JoinHostPort(defaultServiceIP, defaultWrapperServerPort)
	grpcConn, _ := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()))

	// pod with high load
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "busy", Namespace: "ns"},
		Status:     corev1.PodStatus{Phase: corev1.PodRunning},
	}
	pi := newPodInfo()
	pi.grpcClient = grpcConn
	pi.pod = client.ObjectKey{Name: "busy", Namespace: "ns"}
	pi.ongoingEvaluations.Store(10)

	pcm := &podCacheManager{
		pods:            map[string][]*podInfo{"test-image": {pi}},
		maxParallelPods: 1, // at max, so spawn will suggest idx beyond slice
		maxWaitlistLen:  5,
		podTTL:          30 * time.Minute,
		podManager: &podManager{
			kubeClient: fake.NewClientBuilder().WithObjects(pod).Build(),
			namespace:  "ns",
		},
	}

	respCh := make(chan *clientConnAndError, 1)
	waitlist := []chan<- *clientConnAndError{respCh}

	pcm.redistributeWaitlist("test-image", waitlist)

	// should use the existing pod (after bounds adjustment)
	select {
	case resp := <-respCh:
		if resp.err != nil {
			t.Errorf("unexpected error: %v", resp.err)
		}
	default:
		t.Error("expected response on channel")
	}
}

// Test handleMultiPodRequest when re-selection loop finds no pods left
func TestHandleMultiPodRequest_ReselectionLoopNoPods(t *testing.T) {
	address := net.JoinHostPort(defaultServiceIP, defaultWrapperServerPort)
	grpcConn, _ := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()))

	// two unhealthy pods - first selected, then re-selection tries second
	pi1 := newPodInfo()
	pi1.grpcClient = grpcConn
	pi1.pod = client.ObjectKey{Name: "unhealthy1", Namespace: "ns"}
	pi1.ongoingEvaluations.Store(0) // lowest load, selected first

	pi2 := newPodInfo()
	pi2.grpcClient = grpcConn
	pi2.pod = client.ObjectKey{Name: "unhealthy2", Namespace: "ns"}
	pi2.ongoingEvaluations.Store(1)

	pcm := &podCacheManager{
		pods:            map[string][]*podInfo{"test-image": {pi1, pi2}},
		maxParallelPods: 3,
		maxWaitlistLen:  10,
		podTTL:          30 * time.Minute,
		podManager: &podManager{
			kubeClient: fake.NewClientBuilder().Build(), // no pods exist
			namespace:  "ns",
		},
	}

	respCh := make(chan *clientConnAndError, 1)
	req := &clientConnRequest{image: "test-image", grpcClientCh: respCh}

	pcm.handleMultiPodRequest(req)

	// all unhealthy pods removed, new pod should be spawned
	if len(pcm.pods["test-image"]) != 1 {
		t.Errorf("expected 1 new pod, got %d", len(pcm.pods["test-image"]))
	}

	newPod := pcm.pods["test-image"][0]
	if len(newPod.waitlist) != 1 {
		t.Errorf("expected 1 item in waitlist, got %d", len(newPod.waitlist))
	}
}
