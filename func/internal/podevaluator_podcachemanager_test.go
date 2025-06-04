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
					defaultPodObject.ResourceVersion = ""
					defaultPodObject.Name = defaultPodName
					defaultPodObject.GenerateName = ""
					//klog.Infof("Creating pod %s in namespace %s as %v", defaultPodName, defaultNamespace, obj)
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

	tests := []struct {
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
		gcScanInterval: 60 * time.Second,
		requestCh:      requestCh,
		podReadyCh:     podReadyCh,
		waitlists:      waitlists,
		podManager:     pm,
	}

	go pcm.podCacheManager()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.skip {
				t.SkipNow()
			}

			pcm.cache = tt.cache
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
}
