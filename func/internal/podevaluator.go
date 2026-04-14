// Copyright 2022-2026 The kpt and Nephio Authors
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

package internal

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Masterminds/semver/v3"
	kptfilev1 "github.com/kptdev/kpt/pkg/api/kptfile/v1"
	"github.com/kptdev/kpt/pkg/fn"
	"github.com/nephio-project/porch/func/evaluator"
	"github.com/nephio-project/porch/pkg/util"
	pkgerrors "github.com/pkg/errors"
	regclientref "github.com/regclient/regclient/types/ref"
	"google.golang.org/grpc"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

const (
	defaultWrapperServerPort  = "9446"
	volumeName                = "wrapper-server-tools"
	volumeMountPath           = "/wrapper-server-tools"
	wrapperServerBin          = "wrapper-server"
	gRPCProbeBin              = "grpc-health-probe"
	krmFunctionImageLabel     = "fn.kpt.dev/image"
	templateVersionAnnotation = "fn.kpt.dev/template-version"
	inlineTemplateVersionv1   = "inline-v1"
	fieldManagerName          = "krm-function-runner"
	functionContainerName     = "function"
	defaultManagerNamespace   = "porch-system"
	defaultRegistry           = "gcr.io/kpt-fn/"
	serviceDnsNameSuffix      = ".svc.cluster.local"
	channelBufferSize         = 128
)

type podEvaluator struct {
	requestCh chan<- *connectionRequest

	podCacheManager *podCacheManager
}

type PodEvaluatorOptions struct {
	PodNamespace               string        // Namespace to run KRM functions pods in
	WrapperServerImage         string        // Container image name of the wrapper server
	GcScanInterval             time.Duration // Time interval between Garbage Collector scans
	PodTTL                     time.Duration // Time-to-live for pods before GC
	PodCacheConfigFileName     string        // Path to the pod cache config file. The file is map of function name to TTL.
	FunctionPodTemplateName    string        // Configmap that contains a pod specification
	WarmUpPodCacheOnStartup    bool          // If true, pod-cache-config image pods will be deployed at startup
	EnablePrivateRegistries    bool          // If true enables the use of private registries and their authentication
	RegistryAuthSecretPath     string        // The path of the secret used for authenticating to custom registries
	RegistryAuthSecretName     string        // The name of the secret used for authenticating to custom registries
	EnablePrivateRegistriesTls bool          // If enabled, will prioritize use of user provided TLS secret when accessing registries
	TlsSecretPath              string        // The path of the secret used in tls configuration
	MaxGrpcMessageSize         int           // Maximum size of grpc messages in bytes
	DefaultImagePrefix         string        // Default image prefix to use when no prefix is given for an image
	MaxWaitlistLength          int           // Maximum waitlist length per pod
	MaxParallelPodsPerFunction int           // Maximum parallel pods per function
}

var _ Evaluator = &podEvaluator{}

type podData struct {
	// the OCI image name of the KRM function
	image string
	// connection to the grpc server running in the fn evaluator pod
	grpcConnection *grpc.ClientConn
	// namespaced name of the pod
	podKey *client.ObjectKey
	// namespaced name of the service
	serviceKey *client.ObjectKey
}

type connectionRequest struct {
	// the OCI image name of the KRM function
	image string
	// responseCh is the channel to send the response back.
	responseCh chan<- *connectionResponse
}

type connectionResponse struct {
	podData
	// the number of currently ongoing and waiting fn evaluations in the pod
	concurrentEvaluations *atomic.Int32
	// err indicates the error that prevents us to allocate a pod for the fn evaluator
	err error
}

type podReadyResponse struct {
	podData
	// err indicates the error that prevents us to allocate a pod for the fn evaluator
	err error
}

func NewPodEvaluator(ctx context.Context, o PodEvaluatorOptions) (Evaluator, error) {

	restCfg, err := config.GetConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get rest config: %w", err)
	}
	// Give it a slightly higher QPS to prevent unnecessary client-side throttling.
	if restCfg.QPS < 30 {
		restCfg.QPS = 30.0
		restCfg.Burst = 45
	}

	cl, err := client.New(restCfg, client.Options{})
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}

	maxWaitlist := o.MaxWaitlistLength
	if maxWaitlist <= 0 {
		maxWaitlist = 2
	}
	maxPods := o.MaxParallelPodsPerFunction
	if maxPods <= 0 {
		maxPods = 1
	}

	managerNs, err := util.GetInClusterNamespace()
	if err != nil {
		klog.Errorf("failed to get the namespace where the function-runner is running: %v", err)
		klog.Warningf("unable to get the namespace where the function-runner is running, assuming it's a test setup, defaulting to : %v", defaultManagerNamespace)
		managerNs = defaultManagerNamespace
	}

	reqCh := make(chan *connectionRequest, channelBufferSize)
	readyCh := make(chan *podReadyResponse, channelBufferSize)

	var podCacheConfig map[string]podCacheConfigEntry
	if o.PodCacheConfigFileName != "" {
		var err error
		podCacheConfig, err = loadPodCacheConfig(o.PodCacheConfigFileName)
		if err != nil {
			klog.Warningf("unable to load pod cache config file: %v", err)
		}
	}

	pe := &podEvaluator{
		requestCh: reqCh,
		podCacheManager: &podCacheManager{
			gcScanInterval:             o.GcScanInterval,
			podTTL:                     o.PodTTL,
			connectionRequestCh:        reqCh,
			podReadyCh:                 readyCh,
			functions:                  map[string]*functionInfo{},
			maxWaitlistLength:          maxWaitlist,
			maxParallelPodsPerFunction: maxPods,
			configMap:                  podCacheConfig,

			podManager: &podManager{
				kubeClient:              cl,
				namespace:               o.PodNamespace,
				wrapperServerImage:      o.WrapperServerImage,
				podReadyCh:              readyCh,
				functionPodTemplateName: o.FunctionPodTemplateName,
				podReadyTimeout:         60 * time.Second,
				managerNamespace:        managerNs,
				maxGrpcMessageSize:      o.MaxGrpcMessageSize,

				enablePrivateRegistries:    o.EnablePrivateRegistries,
				registryAuthSecretPath:     o.RegistryAuthSecretPath,
				registryAuthSecretName:     o.RegistryAuthSecretName,
				enablePrivateRegistriesTls: o.EnablePrivateRegistriesTls,
				tlsSecretPath:              o.TlsSecretPath,
			},
		},
	}
	go pe.podCacheManager.podCacheManager(ctx)

	err = pe.podCacheManager.retrieveFunctionPods(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve existing pods: %w", err)
	}

	if o.WarmUpPodCacheOnStartup {
		// TODO(mengqiy): add watcher that support reloading the cache when the config file was changed.
		err = pe.podCacheManager.warmupCache(o.PodCacheConfigFileName)
		// If we can't warm up the cache, we can still proceed without it.
		if err != nil {
			klog.Warningf("unable to warm up the pod cache: %v", err)
		}
	}

	return pe, nil
}

func (pe *podEvaluator) filterByConstraint(ctx context.Context, req *evaluator.EvaluateFunctionRequest) (string, error) {
	c, err := semver.NewConstraint(req.Tag)
	if err != nil {
		return "", pkgerrors.Wrapf(err, "tag %q is not a valid semver constraint", req.Tag)
	}

	tags, err := pe.podCacheManager.podManager.listRepositoryTags(ctx, req.Image)
	if err != nil {
		return "", pkgerrors.Wrapf(err, "failed to list tags for image %q", req.Image)
	}

	type candidate struct {
		tag     string
		version *semver.Version
	}

	var candidates []candidate
	for _, tag := range tags {
		if !strings.HasPrefix(tag, "v") {
			continue
		}
		v, err := semver.NewVersion(tag)
		if err != nil {
			klog.V(3).Infof("Failed to parse version %q from tag of image %q: %v", tag, req.Image, err)
			continue
		}
		if c.Check(v) {
			candidates = append(candidates, candidate{tag: tag, version: v})
		}
	}

	if len(candidates) == 0 {
		return "", &fn.NotFoundError{
			Function: kptfilev1.Function{Image: req.Image},
		}
	}

	slices.SortFunc(candidates, func(a, b candidate) int {
		return a.version.Compare(b.version)
	})

	selected := candidates[len(candidates)-1]

	return selected.tag, nil
}

func (pe *podEvaluator) EvaluateFunction(ctx context.Context, req *evaluator.EvaluateFunctionRequest) (*evaluator.EvaluateFunctionResponse, error) {
	starttime := time.Now()
	var tag string
	defer func() {
		klog.Infof("evaluating %v in pod took %v", req.Image, time.Since(starttime))
	}()

	if req.Tag != "" {
		ref, err := regclientref.New(req.Image)
		if err != nil {
			return nil, fmt.Errorf("failed to parse image %q as reference: %w", req.Image, err)
		}
		// If the image already carries an inline tag, strip it
		// so filterByConstraint gets a bare repository name, and
		// we don't produce a double-tag
		if ref.Tag != "" {
			if stripped := strings.TrimSuffix(req.Image, ":"+ref.Tag); stripped != req.Image {
				klog.Infof("Image %q already contains tag %q; stripping it in favor of Tag constraint %q", req.Image, ref.Tag, req.Tag)
				req.Image = stripped
			}
		}
		tag, err = pe.filterByConstraint(ctx, req)
		if err != nil {
			return nil, err
		}
		req.Image = fmt.Sprintf("%s:%s", req.Image, tag)
		klog.Infof("Resolved image tag: %q (constraint %q)", req.Image, tag)
	} else {
		klog.Infof("Image tag is empty, using the image with explicit tag: %q", req.Image)
	}

	// make a buffer for the channel to prevent unnecessary blocking when the pod cache manager sends it to multiple waiting goroutine in batch.
	responseChannel := make(chan *connectionResponse, 1)
	// Send a request to request a grpc client.
	pe.requestCh <- &connectionRequest{
		image:      req.Image,
		responseCh: responseChannel,
	}

	// Waiting for the client from the channel. This step is blocking.
	select {
	case pod := <-responseChannel:
		if pod == nil || pod.grpcConnection == nil || pod.err != nil {
			return nil, fmt.Errorf("unable to get the grpc client to the pod for %v: %w", req.Image, pod.err)
		}

		defer pod.concurrentEvaluations.Add(-1)

		resp, err := evaluator.NewFunctionEvaluatorClient(pod.grpcConnection).EvaluateFunction(ctx, req)
		if err != nil {
			klog.V(4).Infof("Resource List: %s", req.ResourceList)
			return nil, fmt.Errorf("unable to evaluate %v with pod evaluator: %w", req.Image, err)
		}
		// Log stderr when the function succeeded. If the function fails, stderr will be surfaced to the users.
		if len(resp.Log) > 0 {
			klog.Warningf("evaluating %v succeeded, but stderr is: %v", req.Image, string(resp.Log))
		}
		return resp, nil
	case <-ctx.Done():
		return nil, fmt.Errorf("function evaluation timed out for %v: %w", req.Image, ctx.Err())
	}
}
