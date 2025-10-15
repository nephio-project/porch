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
	"bytes"
	"context"
	"encoding/gob"
	"flag"
	"fmt"
	"net"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"k8s.io/klog/v2"

	pb "github.com/nephio-project/porch/func/evaluator"
	"google.golang.org/grpc"
	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

const (
	defaultImageName               = "apply-replacements"
	defaultPodName                 = "apply-replacements-5245a527"
	defaultNamespace               = "porch-fn-system"
	defaultServiceName             = defaultPodName
	defaultEndpointName            = defaultServiceName
	defaultFunctionImageLabel      = defaultPodName
	defaultWrapperServerImage      = "wrapper-server"
	defaultPodIP                   = "10.10.10.10"
	defaultServiceIP               = "20.10.10.10"
	defaultFunctionPodTemplateName = "function-pod-template"
)

type fakeFunctionEvalServer struct {
	pb.UnimplementedFunctionEvaluatorServer
	evalFunc func(ctx context.Context, req *pb.EvaluateFunctionRequest) (*pb.EvaluateFunctionResponse, error)
	port     string
}

func (f *fakeFunctionEvalServer) EvaluateFunction(ctx context.Context, req *pb.EvaluateFunctionRequest) (*pb.EvaluateFunctionResponse, error) {
	return f.evalFunc(ctx, req)
}

func (f *fakeFunctionEvalServer) Start(ctx context.Context) error {
	lis, err := net.Listen("tcp", ":"+f.port)

	if err != nil {
		return err
	}

	server := grpc.NewServer()
	pb.RegisterFunctionEvaluatorServer(server, f)
	//nolint:errcheck
	go server.Serve(lis)

	go func() {
		<-ctx.Done()
		server.GracefulStop()
		lis.Close()
	}()
	return nil
}

func TestPodManager(t *testing.T) {

	flagSet := flag.NewFlagSet("log-level", flag.ContinueOnError)
	klog.InitFlags(flagSet)
	_ = flagSet.Parse([]string{"--v", "5"})

	defaultSuccessEvalFunc := func(ctx context.Context, req *pb.EvaluateFunctionRequest) (*pb.EvaluateFunctionResponse, error) {
		return &pb.EvaluateFunctionResponse{ResourceList: []byte("thisShouldBeKRM"), Log: []byte("Success")}, nil
	}

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

	deletionInProgessPodObjectMeta := metav1.ObjectMeta{}
	deepCopyObject(&defaultPodObjectMeta, &deletionInProgessPodObjectMeta)
	deletionInProgessPodObjectMeta.DeletionTimestamp = &metav1.Time{Time: time.Now()}
	deletionInProgessPodObjectMeta.Finalizers = []string{"test-finalizer"}

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

	podStatusNotRunning := corev1.PodStatus{
		Phase: corev1.PodRunning,
		Conditions: []corev1.PodCondition{
			{
				Type:   corev1.PodReady,
				Status: corev1.ConditionFalse,
			},
		},
		PodIP: "",
	}

	podStatusFailed := corev1.PodStatus{
		Phase: corev1.PodFailed,
		Conditions: []corev1.PodCondition{
			{
				Type:   corev1.PodReady,
				Status: corev1.ConditionFalse,
			},
		},
		PodIP: "",
	}

	podStatusPending := corev1.PodStatus{
		Phase: corev1.PodPending,
		Conditions: []corev1.PodCondition{
			{
				Type:   corev1.PodScheduled,
				Status: corev1.ConditionTrue,
			},
		},
		PodIP: "",
	}

	defaultPodObject := &corev1.Pod{
		ObjectMeta: defaultPodObjectMeta,
		Spec:       defaultPodSpec,
		Status:     podStatusRunning,
	}

	deletionInProgressPodObject := &corev1.Pod{
		ObjectMeta: deletionInProgessPodObjectMeta,
		Spec:       defaultPodSpec,
		Status:     podStatusNotRunning,
	}

	failedPodObject := &corev1.Pod{
		ObjectMeta: deletionInProgessPodObjectMeta,
		Spec:       defaultPodSpec,
		Status:     podStatusFailed,
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

	dummySecretTemplate := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-secret",
			Namespace: "test-namespace",
		},
		Data: map[string][]byte{
			"test-key": []byte("test-value"),
		},
	}

	basePodTemplate := &corev1.Pod{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Pod",
		},
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				"cluster-autoscaler.kubernetes.io/safe-to-evict": "true",
			},
		},
		Spec: corev1.PodSpec{
			InitContainers: []corev1.Container{
				{
					Name:  "copy-wrapper-server",
					Image: "wrapper-server-init",
					Command: []string{
						"cp",
						"-a",
						"/wrapper-server/.",
						volumeMountPath,
					},
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      volumeName,
							MountPath: volumeMountPath,
						},
					},
				},
			},
			Containers: []corev1.Container{
				{
					Name:    functionContainerName,
					Image:   "to-be-replaced",
					Command: []string{filepath.Join(volumeMountPath, wrapperServerBin)},
					ReadinessProbe: &corev1.Probe{
						ProbeHandler: corev1.ProbeHandler{
							// TODO: use the k8s native GRPC prober when it has been rolled out in GKE.
							// https://kubernetes.io/docs/tasks/configure-pod-container/configure-liveness-readiness-startup-probes/#define-a-grpc-liveness-probe
							Exec: &corev1.ExecAction{
								Command: []string{
									filepath.Join(volumeMountPath, gRPCProbeBin),
									"-addr", net.JoinHostPort("localhost", defaultWrapperServerPort),
								},
							},
						},
					},
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      volumeName,
							MountPath: volumeMountPath,
						},
					},
				},
			},
			Volumes: []corev1.Volume{
				{
					Name: volumeName,
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{},
					},
				},
			},
		},
	}

	baseServiceTemplate := &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Service",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      defaultServiceName,
			Namespace: defaultNamespace,
		},
		Spec: corev1.ServiceSpec{
			Type:      corev1.ServiceTypeClusterIP,
			ClusterIP: defaultServiceIP,
		},
	}

	// Fake client. When Pod creation is invoked, it creates the Pod if not present
	// When Service creation is invoked, it creates endpoint object in additon to service
	fakeClientCreateFixInterceptor := func(ctx context.Context, kubeClient client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
		if obj.GetObjectKind().GroupVersionKind().Kind == "Pod" {
			var canary corev1.Pod
			err := kubeClient.Get(ctx, client.ObjectKeyFromObject(obj), &canary)
			if err != nil {
				if errors.IsNotFound(err) {
					err = kubeClient.Create(ctx, obj)
					if err != nil {
						return err
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

	tests := []struct {
		name                    string
		expectFail              bool
		skip                    bool
		kubeClient              client.WithWatch
		namespace               string
		wrapperServerImage      string
		imageMetadataCache      map[string]*digestAndEntrypoint
		evalFunc                func(ctx context.Context, req *pb.EvaluateFunctionRequest) (*pb.EvaluateFunctionResponse, error)
		functionImage           string
		podPatch                *corev1.Pod
		functionPodTemplateName string
		managerNamespace        string
		podPatchAfter           time.Duration
	}{
		{
			name:          "Pod is in deleting state",
			skip:          false,
			expectFail:    false,
			functionImage: defaultImageName,
			kubeClient: fake.NewClientBuilder().WithObjects([]client.Object{
				deletionInProgressPodObject,
				defaultServiceObject,
				defaultEndpointObject,
			}...).WithInterceptorFuncs(interceptor.Funcs{
				Create: fakeClientCreateFixInterceptor,
			}).Build(),
			namespace:          defaultNamespace,
			wrapperServerImage: defaultWrapperServerImage,
			imageMetadataCache: defaultImageMetadataCache,
			evalFunc:           defaultSuccessEvalFunc,
			podPatch: &corev1.Pod{
				Status: podStatusRunning,
			},
		},
		{
			name:          "Create a new pod and new service",
			skip:          false,
			expectFail:    false,
			functionImage: defaultImageName,
			kubeClient: fake.NewClientBuilder().WithInterceptorFuncs(interceptor.Funcs{
				Create: fakeClientCreateFixInterceptor,
			}).Build(),
			namespace:          defaultNamespace,
			wrapperServerImage: defaultWrapperServerImage,
			imageMetadataCache: defaultImageMetadataCache,
			evalFunc:           defaultSuccessEvalFunc,
			podPatch: &corev1.Pod{
				Status: podStatusRunning,
			},
		},
		{
			name:               "Pod startup takes too long",
			skip:               false,
			expectFail:         true,
			functionImage:      defaultImageName,
			kubeClient:         fake.NewClientBuilder().Build(),
			namespace:          defaultNamespace,
			wrapperServerImage: defaultWrapperServerImage,
			imageMetadataCache: defaultImageMetadataCache,
			evalFunc:           defaultSuccessEvalFunc,
			podPatch: &corev1.Pod{
				Status: podStatusPending,
			},
		},
		{
			name:               "Pod startup takes some time",
			skip:               false,
			expectFail:         true,
			functionImage:      defaultImageName,
			kubeClient:         fake.NewClientBuilder().Build(),
			namespace:          defaultNamespace,
			wrapperServerImage: defaultWrapperServerImage,
			imageMetadataCache: defaultImageMetadataCache,
			evalFunc:           defaultSuccessEvalFunc,
			podPatchAfter:      100 * time.Millisecond,
			podPatch: &corev1.Pod{
				Status: podStatusPending,
			},
		},
		{
			name:          "Fail pod creation",
			skip:          false,
			expectFail:    true,
			functionImage: defaultImageName,
			kubeClient: fake.NewClientBuilder().WithInterceptorFuncs(interceptor.Funcs{
				Create: func(ctx context.Context, client client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
					if obj.GetObjectKind().GroupVersionKind().Kind == "Pod" {
						return errors.NewInternalError(fmt.Errorf("Faked error"))
					}
					return nil
				},
			}).Build(),
			namespace:          defaultNamespace,
			wrapperServerImage: defaultWrapperServerImage,
			imageMetadataCache: defaultImageMetadataCache,
			evalFunc:           defaultSuccessEvalFunc,
		},
		{ //This is current behavior, but is it correct?
			name:          "If listing pods fail, try to create a new one",
			skip:          false,
			expectFail:    false,
			functionImage: "apply-replacements",
			kubeClient: fake.NewClientBuilder().WithInterceptorFuncs(interceptor.Funcs{
				List: func(ctx context.Context, client client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
					_, ok := list.(*corev1.PodList)
					if ok {
						return errors.NewInternalError(fmt.Errorf("Faked error"))
					}
					return nil
				},
			}).WithObjects([]client.Object{
				defaultPodObject,
				defaultServiceObject,
				defaultEndpointObject,
			}...).Build(),
			namespace:          defaultNamespace,
			wrapperServerImage: defaultWrapperServerImage,
			imageMetadataCache: defaultImageMetadataCache,
			evalFunc:           defaultSuccessEvalFunc,
			podPatch: &corev1.Pod{
				Status: podStatusRunning,
			},
		},
		{
			name:               "Has invalid function image name",
			skip:               false,
			expectFail:         true,
			functionImage:      "invalid@ociref.com",
			kubeClient:         fake.NewClientBuilder().Build(),
			namespace:          defaultNamespace,
			wrapperServerImage: defaultWrapperServerImage,
			imageMetadataCache: defaultImageMetadataCache,
			evalFunc:           defaultSuccessEvalFunc,
		},
		{
			name:               "Invalid namespace name",
			skip:               false,
			expectFail:         true,
			functionImage:      defaultImageName,
			kubeClient:         fake.NewClientBuilder().Build(),
			namespace:          "not a valid namespace",
			wrapperServerImage: defaultWrapperServerImage,
			imageMetadataCache: defaultImageMetadataCache,
			evalFunc:           defaultSuccessEvalFunc,
		},
		{
			name:                    "Function template configmap not found",
			skip:                    false,
			expectFail:              true,
			functionImage:           defaultImageName,
			kubeClient:              fake.NewClientBuilder().Build(),
			namespace:               defaultNamespace,
			wrapperServerImage:      defaultWrapperServerImage,
			imageMetadataCache:      defaultImageMetadataCache,
			evalFunc:                defaultSuccessEvalFunc,
			functionPodTemplateName: "function-pod-template",
			managerNamespace:        defaultManagerNamespace,
		},
		{
			name:          "Function template invalid resource type",
			skip:          false,
			expectFail:    true,
			functionImage: defaultImageName,
			kubeClient: fake.NewClientBuilder().WithObjects([]client.Object{&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      defaultFunctionPodTemplateName,
					Namespace: defaultManagerNamespace,
				},
				Data: map[string]string{
					"template":        string(marshalToYamlOrPanic(dummySecretTemplate)),
					"serviceTemplate": string(marshalToYamlOrPanic(baseServiceTemplate)),
				},
			}}...).Build(),
			namespace:               defaultNamespace,
			wrapperServerImage:      defaultWrapperServerImage,
			imageMetadataCache:      defaultImageMetadataCache,
			evalFunc:                defaultSuccessEvalFunc,
			functionPodTemplateName: "function-pod-template",
			managerNamespace:        defaultManagerNamespace,
		},
		{
			name:          "Function template under invalid key",
			skip:          false,
			expectFail:    true,
			functionImage: defaultImageName,
			kubeClient: fake.NewClientBuilder().WithObjects([]client.Object{&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      defaultFunctionPodTemplateName,
					Namespace: defaultManagerNamespace,
				},
				Data: map[string]string{
					"not-template": string(marshalToYamlOrPanic(basePodTemplate)),
				},
			}}...).Build(),
			namespace:               defaultNamespace,
			wrapperServerImage:      defaultWrapperServerImage,
			imageMetadataCache:      defaultImageMetadataCache,
			evalFunc:                defaultSuccessEvalFunc,
			functionPodTemplateName: "function-pod-template",
			managerNamespace:        defaultManagerNamespace,
		},
		{
			name:          "Function template present but Service template missing",
			skip:          false,
			expectFail:    true,
			functionImage: defaultImageName,
			kubeClient: fake.NewClientBuilder().WithObjects([]client.Object{&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      defaultFunctionPodTemplateName,
					Namespace: defaultManagerNamespace,
				},
				Data: map[string]string{
					"template": string(marshalToYamlOrPanic(basePodTemplate)),
				},
			}}...).Build(),
			namespace:               defaultNamespace,
			wrapperServerImage:      defaultWrapperServerImage,
			imageMetadataCache:      defaultImageMetadataCache,
			evalFunc:                defaultSuccessEvalFunc,
			functionPodTemplateName: "function-pod-template",
			managerNamespace:        defaultManagerNamespace,
			podPatch: &corev1.Pod{
				Status: podStatusRunning,
			},
		},
		{
			name:          "Function template update is applied when pod is requested",
			skip:          false,
			expectFail:    false,
			functionImage: defaultImageName,
			kubeClient: fake.NewClientBuilder().WithObjects([]client.Object{&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      defaultFunctionPodTemplateName,
					Namespace: defaultManagerNamespace,
				},
				Data: map[string]string{
					"template":        string(marshalToYamlOrPanic(basePodTemplate)),
					"serviceTemplate": string(marshalToYamlOrPanic(baseServiceTemplate)),
				},
			},
				defaultPodObject,
				defaultEndpointObject,
			}...).Build(),
			namespace:               defaultNamespace,
			wrapperServerImage:      defaultWrapperServerImage,
			imageMetadataCache:      defaultImageMetadataCache,
			evalFunc:                defaultSuccessEvalFunc,
			functionPodTemplateName: "function-pod-template",
			managerNamespace:        defaultManagerNamespace,
			podPatch: &corev1.Pod{
				Status: podStatusRunning,
			},
		},
		{
			name:          "Failed pod is deleted and new one is created",
			skip:          false,
			expectFail:    false,
			functionImage: defaultImageName,
			kubeClient: fake.NewClientBuilder().WithObjects([]client.Object{
				failedPodObject,
				defaultServiceObject,
				defaultEndpointObject,
			}...).WithInterceptorFuncs(interceptor.Funcs{
				Create: fakeClientCreateFixInterceptor,
			}).Build(),
			namespace:          defaultNamespace,
			wrapperServerImage: defaultWrapperServerImage,
			imageMetadataCache: defaultImageMetadataCache,
			evalFunc:           defaultSuccessEvalFunc,
			podPatch: &corev1.Pod{
				Status: podStatusRunning,
			},
		},
	}

	fakeServer := &fakeFunctionEvalServer{
		port: defaultWrapperServerPort,
	}
	srvCtx := context.WithoutCancel(context.Background())

	err := fakeServer.Start(srvCtx)
	if err != nil {
		t.Errorf("Failed to set up grpc server for testing %v", err)
		t.FailNow()
	}
	defer srvCtx.Done()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.skip {
				t.SkipNow()
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			//Set up the pod manager
			podReadyCh := make(chan *podReadyResponse)
			pm := &podManager{
				kubeClient:              tt.kubeClient,
				namespace:               tt.namespace,
				wrapperServerImage:      tt.wrapperServerImage,
				imageMetadataCache:      sync.Map{},
				podReadyCh:              podReadyCh,
				podReadyTimeout:         1 * time.Second,
				functionPodTemplateName: tt.functionPodTemplateName,
				managerNamespace:        tt.managerNamespace,

				maxGrpcMessageSize: 4 * 1024 * 1024,

				enablePrivateRegistries: false,
				registryAuthSecretPath:  "/var/tmp/config-secret/.dockerconfigjson",
				registryAuthSecretName:  "auth-secret",

				enablePrivateRegistriesTls: false,
				tlsSecretPath:              "/var/tmp/tls-secret/",
			}

			for k, v := range tt.imageMetadataCache {
				pm.imageMetadataCache.Store(k, v)
			}

			fakeServer.evalFunc = tt.evalFunc

			//Execute the function under test
			go pm.getFuncEvalPodClient(ctx, tt.functionImage, time.Hour)

			if tt.podPatch != nil {
				go func() {
					watchPod, err := tt.kubeClient.Watch(ctx, &corev1.PodList{}, client.InNamespace(tt.namespace))
					if err != nil {
						t.Errorf("Failed to watch for pods: %v", err)
					}
					ev := <-watchPod.ResultChan()
					watchPod.Stop()

					if tt.podPatchAfter > 0 {
						<-time.After(tt.podPatchAfter)
					}
					pod := ev.Object.(*corev1.Pod)
					//Not ideal, but fakeClient.Patch doesn't seem to do merging correctly
					pod.Status = tt.podPatch.Status
					err = pm.kubeClient.Status().Update(ctx, pod)

					if err != nil {
						t.Errorf("Failed to patch pod: %v", err)

					}
				}()
			}

			cc := <-podReadyCh
			if cc.err != nil && !tt.expectFail {
				t.Errorf("Expected to get ready pod, got error: %v", cc.err)
			} else if cc.err == nil {
				if tt.expectFail {
					t.Errorf("Expected to get error, got ready pod")
				}
				var pod corev1.Pod
				if err := tt.kubeClient.Get(ctx, cc.podKey, &pod); err != nil {
					t.Errorf("Failed to get pod: %v", err)
				}

				if !strings.HasPrefix(pod.Labels[krmFunctionImageLabel], tt.functionImage) {
					t.Errorf("Expected pod to have label starting with %s, got %s", tt.functionImage, pod.Labels[krmFunctionImageLabel])
				}
				if pod.Spec.Containers[0].Image != tt.functionImage {
					t.Errorf("Expected pod to have image %s, got %s", tt.functionImage, pod.Spec.Containers[0].Image)
				}

			}

		})
	}
}

// Fake client handles pod patches incorrectly in case the pod doesn't exist
func fakeClientPatchFixInterceptor(ctx context.Context, kubeClient client.WithWatch, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	if obj.GetObjectKind().GroupVersionKind().Kind == "Pod" {
		var canary corev1.Pod
		err := kubeClient.Get(ctx, client.ObjectKeyFromObject(obj), &canary)
		if err != nil {
			if errors.IsNotFound(err) {
				err = kubeClient.Create(ctx, obj)
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

func marshalToYamlOrPanic(obj interface{}) []byte {
	data, err := yaml.Marshal(obj)
	if err != nil {
		panic(err)
	}
	return data
}

func deepCopyObject(in, out interface{}) {
	buf := bytes.Buffer{}
	if err := gob.NewEncoder(&buf).Encode(in); err != nil {
		panic(err)
	}

	if err := gob.NewDecoder(&buf).Decode(out); err != nil {
		panic(err)
	}
}
