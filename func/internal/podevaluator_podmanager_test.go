package internal

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	pb "github.com/nephio-project/porch/func/evaluator"
	"google.golang.org/grpc"
	corev1 "k8s.io/api/core/v1"
	errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
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

	go server.Serve(lis)

	go func() {
		<-ctx.Done()
		server.GracefulStop()
		lis.Close()
	}()
	return nil
}

func TestPodManager(t *testing.T) {

	defaultSuccessEvalFunc := func(ctx context.Context, req *pb.EvaluateFunctionRequest) (*pb.EvaluateFunctionResponse, error) {
		return &pb.EvaluateFunctionResponse{ResourceList: []byte("thisShouldBeKRM"), Log: []byte("Success")}, nil
	}

	defaultImageMetadataCache := map[string]*digestAndEntrypoint{
		"apply-replacements": {
			digest:     "5245a52778d684fa698f69861fb2e058b308f6a74fed5bf2fe77d97bad5e071c",
			entrypoint: []string{"/apply-replacements"},
		},
	}

	tests := []struct {
		name               string
		expectFail         bool
		skip               bool
		kubeClient         client.WithWatch
		namespace          string
		wrapperServerImage string
		imageMetadataCache map[string]*digestAndEntrypoint
		evalFunc           func(ctx context.Context, req *pb.EvaluateFunctionRequest) (*pb.EvaluateFunctionResponse, error)
		functionImage      string
		podPatch           *corev1.Pod
		useGenerateName    bool
	}{
		{
			name:          "Connect to existing pod",
			skip:          false,
			expectFail:    false,
			functionImage: "apply-replacements",
			kubeClient: fake.NewClientBuilder().WithObjects([]client.Object{&corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "apply-replacements-5245a527",
					Namespace: "porch-fn-system",
					Labels: map[string]string{
						krmFunctionLabel: "apply-replacements-5245a527",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "function",
							Image: "apply-replacements",
						},
					},
				},
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
					Conditions: []corev1.PodCondition{
						{
							Type:   corev1.PodReady,
							Status: corev1.ConditionTrue,
						},
					},
					PodIP: "localhost",
				},
			}}...).Build(),
			namespace:          "porch-fn-system",
			wrapperServerImage: "wrapper-server",
			imageMetadataCache: defaultImageMetadataCache,
			evalFunc:           defaultSuccessEvalFunc,
			useGenerateName:    true,
		},
		{
			name:               "Create a new pod",
			skip:               false,
			expectFail:         false,
			functionImage:      "apply-replacements",
			kubeClient:         fake.NewClientBuilder().Build(),
			namespace:          "porch-fn-system",
			wrapperServerImage: "wrapper-server",
			imageMetadataCache: defaultImageMetadataCache,
			evalFunc:           defaultSuccessEvalFunc,
			useGenerateName:    true,
			podPatch: &corev1.Pod{
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
					Conditions: []corev1.PodCondition{
						{
							Type:   corev1.PodReady,
							Status: corev1.ConditionTrue,
						},
					},
					PodIP: "localhost",
				},
			},
		},
		{
			name:               "Pod startup takes too long",
			skip:               false,
			expectFail:         true,
			functionImage:      "apply-replacements",
			kubeClient:         fake.NewClientBuilder().Build(),
			namespace:          "porch-fn-system",
			wrapperServerImage: "wrapper-server",
			imageMetadataCache: defaultImageMetadataCache,
			evalFunc:           defaultSuccessEvalFunc,
			useGenerateName:    true,
			podPatch: &corev1.Pod{
				Status: corev1.PodStatus{
					Phase: corev1.PodPending,
					Conditions: []corev1.PodCondition{
						{
							Type:   corev1.PodScheduled,
							Status: corev1.ConditionTrue,
						},
					},
					PodIP: "localhost",
				},
			},
		},
		{
			name:          "Fail pod creation",
			skip:          false,
			expectFail:    true,
			functionImage: "apply-replacements",
			kubeClient: fake.NewClientBuilder().WithInterceptorFuncs(interceptor.Funcs{
				Create: func(ctx context.Context, client client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
					if obj.GetObjectKind().GroupVersionKind().Kind == "Pod" {
						return errors.NewInternalError(fmt.Errorf("Faked error"))
					}
					return nil
				},
			}).Build(),
			namespace:          "porch-fn-system",
			wrapperServerImage: "wrapper-server",
			imageMetadataCache: defaultImageMetadataCache,
			evalFunc:           defaultSuccessEvalFunc,
			useGenerateName:    true,
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
			}).WithObjects([]client.Object{&corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "apply-replacements-5245a527",
					Namespace: "porch-fn-system",
					Labels: map[string]string{
						krmFunctionLabel: "apply-replacements-5245a527",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "function",
							Image: "apply-replacements",
						},
					},
				},
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
					Conditions: []corev1.PodCondition{
						{
							Type:   corev1.PodReady,
							Status: corev1.ConditionTrue,
						},
					},
					PodIP: "localhost",
				},
			}}...).Build(),
			namespace:          "porch-fn-system",
			wrapperServerImage: "wrapper-server",
			imageMetadataCache: defaultImageMetadataCache,
			evalFunc:           defaultSuccessEvalFunc,
			useGenerateName:    true,
			podPatch: &corev1.Pod{
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
					Conditions: []corev1.PodCondition{
						{
							Type:   corev1.PodReady,
							Status: corev1.ConditionTrue,
						},
					},
					PodIP: "localhost",
				},
			},
		},
		{
			name:               "Has invalid function image name",
			skip:               false,
			expectFail:         true,
			functionImage:      "invalid@ociref.com",
			kubeClient:         fake.NewClientBuilder().Build(),
			namespace:          "porch-fn-system",
			wrapperServerImage: "wrapper-server",
			imageMetadataCache: defaultImageMetadataCache,
			evalFunc:           defaultSuccessEvalFunc,
			useGenerateName:    true,
		},
		{
			name:               "Invalid namespace name",
			skip:               false,
			expectFail:         true,
			functionImage:      "apply-replacements",
			kubeClient:         fake.NewClientBuilder().Build(),
			namespace:          "not a valid namespace",
			wrapperServerImage: "wrapper-server",
			imageMetadataCache: defaultImageMetadataCache,
			evalFunc:           defaultSuccessEvalFunc,
			useGenerateName:    true,
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
			podReadyCh := make(chan *imagePodAndGRPCClient)
			pm := &podManager{
				kubeClient:         tt.kubeClient,
				namespace:          tt.namespace,
				wrapperServerImage: tt.wrapperServerImage,
				imageMetadataCache: sync.Map{},
				podReadyCh:         podReadyCh,
				podReadyTimeout:    1 * time.Second,
			}

			for k, v := range tt.imageMetadataCache {
				pm.imageMetadataCache.Store(k, v)
			}

			fakeServer.evalFunc = tt.evalFunc

			//Execute the function under test
			go pm.getFuncEvalPodClient(ctx, tt.functionImage, time.Hour, tt.useGenerateName)

			if tt.podPatch != nil {
				go func() {
					watchPod, err := tt.kubeClient.Watch(ctx, &corev1.PodList{}, client.InNamespace(tt.namespace))
					if err != nil {
						t.Errorf("Failed to watch for pods: %v", err)
					}
					ev := <-watchPod.ResultChan()
					watchPod.Stop()

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
				if tt.kubeClient.Get(ctx, cc.pod, &pod); err != nil {
					t.Errorf("Failed to get pod: %v", err)
				}
				if !strings.HasPrefix(pod.Labels[krmFunctionLabel], tt.functionImage) {
					t.Errorf("Expected pod to have label starting wiht %s, got %s", tt.functionImage, pod.Labels[krmFunctionLabel])
				}
			}

		})
	}
}
