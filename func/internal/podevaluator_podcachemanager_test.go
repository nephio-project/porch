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

	go pcm.podCacheManager(t.Context())

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
