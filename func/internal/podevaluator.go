// Copyright 2022 The kpt and Nephio Authors
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
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	containerregistry "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/nephio-project/porch/func/evaluator"
	util "github.com/nephio-project/porch/pkg/util"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/wait"
	yamlutil "k8s.io/apimachinery/pkg/util/yaml"
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
	reclaimAfterAnnotation    = "fn.kpt.dev/reclaim-after"
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
	requestCh chan<- *clientConnRequest

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
	MaxParallelPodsPerFunction int           // Max concurrent pods per function image (default 1)
	MaxWaitlistLength          int           // Waitlist threshold to spawn new pod
}

var _ Evaluator = &podEvaluator{}

func NewPodEvaluator(o PodEvaluatorOptions) (Evaluator, error) {

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

	managerNs, err := util.GetInClusterNamespace()
	if err != nil {
		klog.Errorf("failed to get the namespace where the function-runner is running: %v", err)
		klog.Warningf("unable to get the namespace where the function-runner is running, assuming it's a test setup, defaulting to : %v", defaultManagerNamespace)
		managerNs = defaultManagerNamespace
	}

	reqCh := make(chan *clientConnRequest, channelBufferSize)
	readyCh := make(chan *imagePodAndGRPCClient, channelBufferSize)

	// Set defaults for multi-pod config
	maxParallelPods := o.MaxParallelPodsPerFunction
	if maxParallelPods <= 0 {
		maxParallelPods = 1
	}
	maxWaitlistLen := o.MaxWaitlistLength
	if maxWaitlistLen <= 0 {
		maxWaitlistLen = 10
	}

	pe := &podEvaluator{
		requestCh: reqCh,
		podCacheManager: &podCacheManager{
			gcScanInterval:  o.GcScanInterval,
			podTTL:          o.PodTTL,
			requestCh:       reqCh,
			podReadyCh:      readyCh,
			cache:           map[string]*podAndGRPCClient{},
			waitlists:       map[string][]chan<- *clientConnAndError{},
			pods:            map[string][]*podInfo{},
			maxParallelPods: maxParallelPods,
			maxWaitlistLen:  maxWaitlistLen,

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
	go pe.podCacheManager.podCacheManager()

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

func (pe *podEvaluator) EvaluateFunction(ctx context.Context, req *evaluator.EvaluateFunctionRequest) (*evaluator.EvaluateFunctionResponse, error) {
	starttime := time.Now()
	defer func() {
		klog.Infof("evaluating %v in pod took %v", req.Image, time.Since(starttime))
	}()
	// make a buffer for the channel to prevent unnecessary blocking when the pod cache manager sends it to multiple waiting goroutine in batch.
	ccChan := make(chan *clientConnAndError, 1)
	// Send a request to request a grpc client.
	pe.requestCh <- &clientConnRequest{
		image:        req.Image,
		grpcClientCh: ccChan,
	}

	// Waiting for the client from the channel. This step is blocking.
	cc := <-ccChan
	if cc.err != nil {
		return nil, fmt.Errorf("unable to get the grpc client to the pod for %v: %w", req.Image, cc.err)
	}

	// track ongoing evaluations for load balancing (multi-pod mode only)
	// counter was already incremented in podCacheManager before sending
	if cc.podInfo != nil {
		defer cc.podInfo.ongoingEvaluations.Add(-1)

		// ensure sequential execution per pod
		cc.podInfo.fnEvaluationMutex.Lock()
		defer cc.podInfo.fnEvaluationMutex.Unlock()
	}

	resp, err := evaluator.NewFunctionEvaluatorClient(cc.grpcClient).EvaluateFunction(ctx, req)
	if err != nil {
		klog.V(4).Infof("Resource List: %s", req.ResourceList)
		return nil, fmt.Errorf("unable to evaluate %v with pod evaluator: %w", req.Image, err)
	}
	// Log stderr when the function succeeded. If the function fails, stderr will be surfaced to the users.
	if len(resp.Log) > 0 {
		klog.Warningf("evaluating %v succeeded, but stderr is: %v", req.Image, string(resp.Log))
	}
	return resp, nil
}

// podCacheManager manages the cache of the pods and the corresponding GRPC clients.
// It also does the garbage collection after pods' TTL.
// It has 2 receive-only channels: requestCh and podReadyCh.
// It listens to the requestCh channel and receives clientConnRequest from the
// GRPC request handlers and add them in the waitlists.
// It also listens to the podReadyCh channel. If a pod is ready, it notifies the
// goroutines by sending back the GRPC client by lookup the waitlists mapping.

// Each function pod managed by podCacheManager has a frontend ClusterIP type
// service added to it to make Porch modules compatible for deployment into a
// cluster using service mesh like Istio
// https://github.com/nephio-project/nephio/issues/879
type podCacheManager struct {
	gcScanInterval time.Duration
	podTTL         time.Duration

	// requestCh is a receive-only channel to receive
	requestCh <-chan *clientConnRequest
	// podReadyCh is a channel to receive the information when a pod is ready.
	podReadyCh <-chan *imagePodAndGRPCClient

	// cache is a mapping from image name to <pod + grpc client>.
	// Note: when maxParallelPods > 1, pods map is used instead
	cache map[string]*podAndGRPCClient
	// waitlists is a mapping from image name to a list of channels that are
	// waiting for the GRPC client connections.
	waitlists map[string][]chan<- *clientConnAndError

	// pods is the multi-pod mode map, each image can have multiple pods
	pods map[string][]*podInfo

	// multi-pod config
	maxParallelPods int // max pods per image
	maxWaitlistLen  int // spawn new pod when waitlist exceeds this

	podManager *podManager
}

type clientConnRequest struct {
	image string

	// grpcConn is a channel that a grpc client should be sent back.
	grpcClientCh chan<- *clientConnAndError
}

type clientConnAndError struct {
	grpcClient *grpc.ClientConn
	err        error
	// podInfo is set in multi-pod mode for tracking ongoing evaluations
	podInfo *podInfo
}

// podInfo holds per-pod state for multi-pod load balancing
type podInfo struct {
	grpcClient *grpc.ClientConn
	pod        client.ObjectKey

	// waitlist for this pod
	waitlist []chan<- *clientConnAndError

	// ongoing request count (atomic for concurrent access)
	ongoingEvaluations atomic.Int32

	// mutex to ensure sequential execution per pod
	fnEvaluationMutex sync.Mutex

	lastActivity time.Time
}

func newPodInfo() *podInfo {
	return &podInfo{
		waitlist:     make([]chan<- *clientConnAndError, 0),
		lastActivity: time.Now(),
	}
}

// totalLoad returns the pod's total load (waitlist + ongoing)
func (p *podInfo) totalLoad() int {
	return len(p.waitlist) + int(p.ongoingEvaluations.Load())
}

// findBestPod finds the pod with lowest load, suggests spawning if all exceed threshold
func findBestPod(pods []*podInfo, maxWaitlist, maxPods int) (idx int, shouldSpawn bool) {
	if len(pods) == 0 {
		return 0, true
	}

	bestIdx := 0
	bestLoad := pods[0].totalLoad()

	for i := 1; i < len(pods); i++ {
		load := pods[i].totalLoad()
		if load < bestLoad {
			bestIdx = i
			bestLoad = load
		}
	}

	// spawn new pod if lowest load exceeds threshold and not at max
	if bestLoad >= maxWaitlist && len(pods) < maxPods {
		return len(pods), true
	}

	return bestIdx, false
}

type podAndGRPCClient struct {
	grpcClient *grpc.ClientConn
	pod        client.ObjectKey
}

type imagePodAndGRPCClient struct {
	image string
	*podAndGRPCClient
	err error
}

// warmupCache creates the pods and warms up the cache.
func (pcm *podCacheManager) warmupCache(podCacheConfig string) error {
	start := time.Now()
	defer func() {
		klog.Infof("cache warming is completed and it took %v", time.Since(start))
	}()
	content, err := os.ReadFile(podCacheConfig)
	if err != nil {
		return err
	}
	var podsTTL map[string]string
	err = yaml.Unmarshal(content, &podsTTL)
	if err != nil {
		return err
	}

	// We precreate the pods (concurrently) to speed it up.
	forEachConcurrently(podsTTL, func(fnImage string, ttlStr string) {
		klog.Infof("preloading pod cache for function %v with TTL %v", fnImage, ttlStr)

		ttl, err := time.ParseDuration(ttlStr)
		if err != nil {
			klog.Warningf("unable to parse duration from the config file for function %v: %v", fnImage, err)
			ttl = pcm.podTTL
		}

		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()

		// We invoke the function with useGenerateName=false so that the pod name is fixed,
		// since we want to ensure only one pod is created for each function.
		pcm.podManager.getFuncEvalPodClient(ctx, fnImage, ttl, false)
		klog.Infof("preloaded pod cache for function %v", fnImage)
	})

	return nil
}

// forEachConcurrently runs fn for each entry in the map m, in parallel goroutines.
// It waits for each to finish before returning.
func forEachConcurrently(m map[string]string, fn func(k string, v string)) {
	var wg sync.WaitGroup
	for k, v := range m {
		k := k
		v := v

		wg.Add(1)
		go func() {
			defer wg.Done()
			fn(k, v)
		}()
	}
	// Wait for all the functions to complete.
	wg.Wait()
}

// podCacheManager responds to the requestCh and the podReadyCh and does the
// garbage collection synchronously.
// We must run this method in one single goroutine. Doing it this way simplify
// design around concurrency.
func (pcm *podCacheManager) podCacheManager() {
	//nolint:staticcheck
	tick := time.Tick(pcm.gcScanInterval)
	for {
		select {
		case req := <-pcm.requestCh:
			// use multi-pod logic when enabled
			if pcm.maxParallelPods > 1 {
				pcm.handleMultiPodRequest(req)
				continue
			}
			// single pod logic below
			podAndCl, found := pcm.cache[req.image]
			if found && podAndCl != nil {
				// Ensure the pod still exists and is not being deleted before sending the grpc client back to the channel.
				// We can't simply return grpc client from the cache and let evaluator try to connect to the pod.
				// If the pod is deleted by others, it will take ~10 seconds for the evaluator to fail.
				// Wasting 10 second is so much, so we check if the pod still exist first.
				pod := &corev1.Pod{}
				err := pcm.podManager.kubeClient.Get(context.Background(), podAndCl.pod, pod)
				deleteCacheEntry := false
				if err == nil {
					// If the cached pod is in Failed state, delete it and evict from cache to trigger a fresh pod.
					if pod.Status.Phase == corev1.PodFailed {
						klog.Warningf("pod %v/%v is in Failed state. Deleting and recreating", pod.Namespace, pod.Name)
						go pcm.deleteFailedPod(pod, req.image)
						deleteCacheEntry = true
					}

					// GRPC client is made to Service IP instead of PoD directly, hence target host to be verified
					// against Service Cluster IP

					// Service name is Image Label set on Pod manifest
					serviceName := pod.Labels[krmFunctionImageLabel]
					service := &corev1.Service{}
					serviceKey := client.ObjectKey{Namespace: pod.Namespace, Name: serviceName}
					err = pcm.podManager.kubeClient.Get(context.Background(), serviceKey, service)
					if err != nil {
						// Service lookup should not fail.
						// If it does, purge the cached entry and let Pod/Service be created again
						deleteCacheEntry = true
					} else {
						serviceUrl := service.Name + "." + service.Namespace + serviceDnsNameSuffix
						if pod.DeletionTimestamp == nil && net.JoinHostPort(serviceUrl, defaultWrapperServerPort) == podAndCl.grpcClient.Target() {
							klog.Infof("reusing the connection to pod %v/%v to evaluate %v", pod.Namespace, pod.Name, req.image)
							req.grpcClientCh <- &clientConnAndError{grpcClient: podAndCl.grpcClient}
							go patchPodWithUnixTimeAnnotation(pcm.podManager.kubeClient, podAndCl.pod, pcm.podTTL)
							break
						} else {
							deleteCacheEntry = true
						}
					}
				} else if errors.IsNotFound(err) {
					deleteCacheEntry = true
				}
				// We delete the cache entry if the pod has been deleted or being deleted.
				if deleteCacheEntry {
					delete(pcm.cache, req.image)
				}
			}
			_, found = pcm.waitlists[req.image]
			if !found {
				pcm.waitlists[req.image] = []chan<- *clientConnAndError{}
				// We invoke the function with useGenerateName=true to avoid potential name collision, since if pod foo is
				// being deleted and we can't use the same name.
				go pcm.podManager.getFuncEvalPodClient(context.Background(), req.image, pcm.podTTL, true)
			}
			list := pcm.waitlists[req.image]
			pcm.waitlists[req.image] = append(list, req.grpcClientCh)
		case resp := <-pcm.podReadyCh:
			// use multi-pod logic when enabled
			if pcm.maxParallelPods > 1 {
				pcm.handleMultiPodReady(resp)
				continue
			}
			// single pod logic below
			if resp.err != nil {
				klog.Warningf("received error from the pod manager: %v", resp.err)
			} else {
				pcm.cache[resp.image] = resp.podAndGRPCClient
			}
			// notify all the goroutines that are waiting for the GRPC client.
			channels := pcm.waitlists[resp.image]
			delete(pcm.waitlists, resp.image)
			for i := range channels {
				cce := &clientConnAndError{err: resp.err}
				if resp.podAndGRPCClient != nil {
					cce.grpcClient = resp.grpcClient
				}
				// The channel has one buffer size, nothing will be blocking.
				channels[i] <- cce
			}
		case <-tick:
			// synchronous GC
			pcm.garbageCollector()
		}
	}
}

// handleMultiPodRequest handles requests in multi-pod mode.
// Uses findBestPod to select optimal pod, spawns new pod when needed.
func (pcm *podCacheManager) handleMultiPodRequest(req *clientConnRequest) {
	pods := pcm.pods[req.image]
	idx, shouldSpawn := findBestPod(pods, pcm.maxWaitlistLen, pcm.maxParallelPods)

	// spawn new pod if needed
	if shouldSpawn {
		pi := newPodInfo()
		pcm.pods[req.image] = append(pcm.pods[req.image], pi)
		go pcm.podManager.getFuncEvalPodClient(context.Background(), req.image, pcm.podTTL, true)
	}

	// ensure we have pods
	pods = pcm.pods[req.image]
	if len(pods) == 0 {
		// should not happen, but safeguard to prevent request from hanging
		klog.Warningf("multi-pod: no pods after spawn for %s, this should not happen", req.image)
		pi := newPodInfo()
		pi.waitlist = append(pi.waitlist, req.grpcClientCh)
		pcm.pods[req.image] = append(pcm.pods[req.image], pi)
		go pcm.podManager.getFuncEvalPodClient(context.Background(), req.image, pcm.podTTL, true)
		return
	}

	// select target pod
	targetIdx := idx
	if targetIdx >= len(pods) {
		targetIdx = len(pods) - 1
	}
	targetPod := pods[targetIdx]

	// return grpc client if pod is ready
	if targetPod.grpcClient != nil {
		// health check: verify pod still exists and is healthy
		if pcm.validatePodHealth(req.image, targetPod, targetIdx) {
			// increment load counter before sending to prevent race in load balancing
			targetPod.ongoingEvaluations.Add(1)
			req.grpcClientCh <- &clientConnAndError{grpcClient: targetPod.grpcClient, podInfo: targetPod}
			targetPod.lastActivity = time.Now()
			// update TTL annotation
			go patchPodWithUnixTimeAnnotation(pcm.podManager.kubeClient, targetPod.pod, pcm.podTTL)
			return
		}
		// pod was unhealthy, try to find another healthy pod
		// track the failed pod to avoid re-selecting it
		failedPodName := targetPod.pod.Name

		pods = pcm.pods[req.image]
		if len(pods) == 0 {
			// no pods left, spawn new one and wait
			pi := newPodInfo()
			pi.waitlist = append(pi.waitlist, req.grpcClientCh)
			pcm.pods[req.image] = append(pcm.pods[req.image], pi)
			go pcm.podManager.getFuncEvalPodClient(context.Background(), req.image, pcm.podTTL, true)
			return
		}

		// try to find a healthy pod, excluding the one that just failed
		// use index-safe iteration since validatePodHealth may modify the slice
		for {
			pods = pcm.pods[req.image]
			if len(pods) == 0 {
				break
			}

			foundCandidate := false
			for i, pi := range pods {
				// skip the pod that just failed validation
				if pi.pod.Name == failedPodName {
					continue
				}
				// skip pods without ready grpcClient
				if pi.grpcClient == nil {
					continue
				}
				foundCandidate = true
				// validate this pod's health before using
				if pcm.validatePodHealth(req.image, pi, i) {
					pi.ongoingEvaluations.Add(1)
					req.grpcClientCh <- &clientConnAndError{grpcClient: pi.grpcClient, podInfo: pi}
					pi.lastActivity = time.Now()
					go patchPodWithUnixTimeAnnotation(pcm.podManager.kubeClient, pi.pod, pcm.podTTL)
					return
				}
				// health check failed and may have modified slice, restart iteration
				break
			}

			// no more candidates with grpcClient
			if !foundCandidate {
				break
			}
		}

		// no healthy pods with ready grpcClient found, add to a waitlist
		// re-read pods in case they were modified by health checks
		pods = pcm.pods[req.image]
		if len(pods) == 0 {
			// all pods removed, spawn new one
			pi := newPodInfo()
			pi.waitlist = append(pi.waitlist, req.grpcClientCh)
			pcm.pods[req.image] = append(pcm.pods[req.image], pi)
			go pcm.podManager.getFuncEvalPodClient(context.Background(), req.image, pcm.podTTL, true)
			return
		}
		// find best pod for waitlist (could be one without grpcClient)
		newIdx, _ := findBestPod(pods, pcm.maxWaitlistLen, pcm.maxParallelPods)
		if newIdx >= len(pods) {
			newIdx = len(pods) - 1
		}
		targetPod = pods[newIdx]
	}

	// add to pod's waitlist
	targetPod.waitlist = append(targetPod.waitlist, req.grpcClientCh)
}

// validatePodHealth checks if a pod is still healthy for use
// Returns false if pod should be removed from cache
func (pcm *podCacheManager) validatePodHealth(image string, pi *podInfo, podIdx int) bool {
	if pi.pod.Name == "" {
		return false
	}

	pod := &corev1.Pod{}
	err := pcm.podManager.kubeClient.Get(context.Background(), pi.pod, pod)
	if err != nil {
		if errors.IsNotFound(err) {
			klog.Warningf("multi-pod: pod %v not found, removing from cache", pi.pod)
			pcm.removePodAtIndex(image, podIdx)
		}
		return false
	}

	// check if pod is being deleted
	if pod.DeletionTimestamp != nil {
		klog.Warningf("multi-pod: pod %v is being deleted, removing from cache", pi.pod)
		pcm.removePodAtIndex(image, podIdx)
		return false
	}

	// check if pod is failed
	if pod.Status.Phase == corev1.PodFailed {
		klog.Warningf("multi-pod: pod %v is in Failed state, removing from cache", pi.pod)
		go pcm.deleteFailedPod(pod, image)
		pcm.removePodAtIndex(image, podIdx)
		return false
	}

	return true
}

// removePodAtIndex removes a pod from the pods slice at given index
// and redistributes any pending waitlist items to other pods
func (pcm *podCacheManager) removePodAtIndex(image string, idx int) {
	pods := pcm.pods[image]
	if idx < 0 || idx >= len(pods) {
		return
	}

	// save waitlist for redistribution
	failedPod := pods[idx]
	waitlist := failedPod.waitlist
	failedPod.waitlist = nil

	// close grpcClient to prevent connection leak
	// Note: in-flight evaluations using this connection will fail and should be retried upstream
	if failedPod.grpcClient != nil {
		if err := failedPod.grpcClient.Close(); err != nil {
			klog.Warningf("failed to close grpc client for pod at index %d: %v", idx, err)
		}
		failedPod.grpcClient = nil
	}

	// remove pod from slice
	pcm.pods[image] = append(pods[:idx], pods[idx+1:]...)
	klog.Infof("Removed pod at index %d from multi-pod cache for image %s", idx, image)

	// redistribute waitlist to other pods
	if len(waitlist) > 0 {
		pcm.redistributeWaitlist(image, waitlist)
	}
}

// redistributeWaitlist distributes pending requests to available pods
func (pcm *podCacheManager) redistributeWaitlist(image string, waitlist []chan<- *clientConnAndError) {
	pods := pcm.pods[image]

	// if no pods available, spawn new one and add all to its waitlist
	if len(pods) == 0 {
		klog.Infof("No pods available for %s, spawning new pod for %d waiting requests", image, len(waitlist))
		pi := newPodInfo()
		pi.waitlist = waitlist
		pcm.pods[image] = append(pcm.pods[image], pi)
		go pcm.podManager.getFuncEvalPodClient(context.Background(), image, pcm.podTTL, true)
		return
	}

	// distribute to pods with ready grpcClient, or to waitlists
	for _, ch := range waitlist {
		// re-read pods as health checks may have modified the list
		pods = pcm.pods[image]
		if len(pods) == 0 {
			// all pods gone, spawn new one
			pi := newPodInfo()
			pi.waitlist = append(pi.waitlist, ch)
			pcm.pods[image] = append(pcm.pods[image], pi)
			go pcm.podManager.getFuncEvalPodClient(context.Background(), image, pcm.podTTL, true)
			continue
		}

		// find best pod for this request
		idx, shouldSpawn := findBestPod(pods, pcm.maxWaitlistLen, pcm.maxParallelPods)

		if shouldSpawn && idx >= len(pods) {
			// spawn new pod
			pi := newPodInfo()
			pi.waitlist = append(pi.waitlist, ch)
			pcm.pods[image] = append(pcm.pods[image], pi)
			go pcm.podManager.getFuncEvalPodClient(context.Background(), image, pcm.podTTL, true)
			continue
		}

		// select target pod
		targetIdx := idx
		if targetIdx >= len(pods) {
			targetIdx = len(pods) - 1
		}
		targetPod := pods[targetIdx]

		// if pod is ready, validate health before sending
		if targetPod.grpcClient != nil {
			if pcm.validatePodHealth(image, targetPod, targetIdx) {
				// increment counter before sending to prevent race in load balancing
				targetPod.ongoingEvaluations.Add(1)
				ch <- &clientConnAndError{grpcClient: targetPod.grpcClient, podInfo: targetPod}
				targetPod.lastActivity = time.Now()
				go patchPodWithUnixTimeAnnotation(pcm.podManager.kubeClient, targetPod.pod, pcm.podTTL)
				continue
			}
			// pod unhealthy, re-read pods and find another
			for {
				pods = pcm.pods[image]
				if len(pods) == 0 {
					pi := newPodInfo()
					pi.waitlist = append(pi.waitlist, ch)
					pcm.pods[image] = append(pcm.pods[image], pi)
					go pcm.podManager.getFuncEvalPodClient(context.Background(), image, pcm.podTTL, true)
					break
				}

				newIdx, _ := findBestPod(pods, pcm.maxWaitlistLen, pcm.maxParallelPods)
				if newIdx >= len(pods) {
					newIdx = len(pods) - 1
				}
				targetPod = pods[newIdx]

				// if pod not ready, add to waitlist
				if targetPod.grpcClient == nil {
					targetPod.waitlist = append(targetPod.waitlist, ch)
					break
				}

				// validate health before sending
				if pcm.validatePodHealth(image, targetPod, newIdx) {
					targetPod.ongoingEvaluations.Add(1)
					ch <- &clientConnAndError{grpcClient: targetPod.grpcClient, podInfo: targetPod}
					targetPod.lastActivity = time.Now()
					go patchPodWithUnixTimeAnnotation(pcm.podManager.kubeClient, targetPod.pod, pcm.podTTL)
					break
				}
				// health check failed, loop again with fresh pod list
			}
			continue
		}

		// add to waitlist
		targetPod.waitlist = append(targetPod.waitlist, ch)
	}

	klog.Infof("Redistributed %d waiting requests for %s", len(waitlist), image)
}

// handleMultiPodReady handles pod ready notification in multi-pod mode
func (pcm *podCacheManager) handleMultiPodReady(resp *imagePodAndGRPCClient) {
	if resp.err != nil {
		klog.Warningf("multi-pod: received error from pod manager: %v", resp.err)
		// notify first waiting pod if any
		pods := pcm.pods[resp.image]
		for _, pi := range pods {
			if len(pi.waitlist) > 0 && pi.grpcClient == nil {
				for _, ch := range pi.waitlist {
					ch <- &clientConnAndError{err: resp.err}
				}
				pi.waitlist = nil
				break
			}
		}
		return
	}

	// find and update the corresponding podInfo
	pods := pcm.pods[resp.image]

	// helper function to assign grpcClient and notify waiters
	assignClientToPod := func(pi *podInfo) {
		pi.grpcClient = resp.grpcClient
		pi.pod = resp.pod
		pi.lastActivity = time.Now()

		// notify all waiters with podInfo for tracking
		// increment counter before sending to prevent race in load balancing
		for _, ch := range pi.waitlist {
			pi.ongoingEvaluations.Add(1)
			ch <- &clientConnAndError{grpcClient: resp.grpcClient, podInfo: pi}
		}
		pi.waitlist = nil
	}

	// first, try to match by pod identity (namespace/name) for accurate assignment
	// when multiple pods are initializing concurrently
	if resp.pod.Name != "" {
		for _, pi := range pods {
			if pi.pod.Name == resp.pod.Name && pi.pod.Namespace == resp.pod.Namespace {
				assignClientToPod(pi)
				return
			}
		}
	}

	// fallback: assign to first pod without grpcClient
	// this handles the case when pod identity is not yet set
	for _, pi := range pods {
		if pi.grpcClient == nil {
			assignClientToPod(pi)
			break
		}
	}
}

// handleFailedPod checks if a pod is in Failed state and deletes it along with evicting it from the cache.
func (pcm *podCacheManager) handleFailedPod(pod *corev1.Pod) bool {
	if pod.Status.Phase != corev1.PodFailed {
		return false
	}

	image := pod.Spec.Containers[0].Image
	klog.Warningf("Found failed pod %v/%v, deleting immediately", pod.Namespace, pod.Name)
	go pcm.deleteFailedPod(pod, image)

	// evict from single-pod cache
	if cached, ok := pcm.cache[image]; ok && cached.pod.Name == pod.Name {
		delete(pcm.cache, image)
		klog.Infof("Evicted failed pod %v/%v from cache", pod.Namespace, pod.Name)
	}

	// evict from multi-pod cache
	pcm.evictPodFromMultiPodCache(image, pod.Name)

	return true
}

// evictPodFromMultiPodCache removes a pod by name from the pods map
// and properly cleans up resources (waitlist redistribution, connection close)
func (pcm *podCacheManager) evictPodFromMultiPodCache(image, podName string) {
	pods := pcm.pods[image]
	for i, pi := range pods {
		if pi.pod.Name == podName {
			// save waitlist before removal
			waitlist := pi.waitlist
			pi.waitlist = nil

			// close grpcClient if present to prevent connection leak
			if pi.grpcClient != nil {
				if err := pi.grpcClient.Close(); err != nil {
					klog.Warningf("failed to close grpc client for evicted pod %s: %v", podName, err)
				}
				pi.grpcClient = nil
			}

			// remove from slice
			pcm.pods[image] = append(pods[:i], pods[i+1:]...)
			klog.Infof("Evicted pod %s from multi-pod cache for image %s", podName, image)

			// redistribute waitlist to remaining pods
			if len(waitlist) > 0 {
				klog.Infof("Redistributing %d waitlist items from evicted pod %s", len(waitlist), podName)
				pcm.redistributeWaitlist(image, waitlist)
			}
			return
		}
	}
}

// TODO: We can use Watch + periodically reconciliation to manage the pods,
// the pod evaluator will become a controller.
func (pcm *podCacheManager) garbageCollector() {
	var err error
	podList := &corev1.PodList{}
	err = pcm.podManager.kubeClient.List(context.Background(), podList, client.InNamespace(pcm.podManager.namespace), client.HasLabels{krmFunctionImageLabel})
	if err != nil {
		klog.Warningf("unable to list pods in namespace %v: %v", pcm.podManager.namespace, err)
		return
	}
	for i, pod := range podList.Items {
		// Immediately delete and evict any pod found in Failed state, regardless of TTL expiry.
		if pcm.handleFailedPod(&pod) {
			continue
		}
		// If a pod is being deleted, skip it.
		if pod.DeletionTimestamp != nil {
			continue
		}
		reclaimAfterStr, found := pod.Annotations[reclaimAfterAnnotation]
		// If a pod doesn't have a last-use annotation, we patch it. This should not happen, but if it happens,
		// we give another TTL before deleting it.
		if !found {
			go patchPodWithUnixTimeAnnotation(pcm.podManager.kubeClient, client.ObjectKeyFromObject(&pod), pcm.podTTL)
			continue
		} else {
			reclaimAfter, err := strconv.ParseInt(reclaimAfterStr, 10, 64)
			// If the annotation is ill-formatted, we patch it with the current time and will try to GC it later.
			// This should not happen, but if it happens, we give another TTL before deleting it.
			if err != nil {
				klog.Warningf("unable to convert the Unix time string to int64: %v", err)
				go patchPodWithUnixTimeAnnotation(pcm.podManager.kubeClient, client.ObjectKeyFromObject(&pod), pcm.podTTL)
				continue
			}
			// If the current time is after the reclaim-later annotation in the pod, we delete the pod and remove the corresponding cache entry.
			if time.Now().After(time.Unix(reclaimAfter, 0)) {
				var serviceIP string

				// Service name is Image Label set on Pod manifest
				serviceName := pod.Labels[krmFunctionImageLabel]
				service := &corev1.Service{}
				serviceKey := client.ObjectKey{Namespace: pod.Namespace, Name: serviceName}
				err = pcm.podManager.kubeClient.Get(context.Background(), serviceKey, service)
				if err != nil {
					klog.Warningf("unable to find expected service %s namespace %s: %v", serviceName, pod.Namespace, err)
				} else {
					serviceIP = service.Spec.ClusterIP
					klog.Infof("Found service named %s with Cluster IP %s", serviceName, serviceIP)
				}

				go func(po corev1.Pod, svc corev1.Service) {
					klog.Infof("deleting pod %v/%v", po.Namespace, po.Name)
					err := pcm.podManager.kubeClient.Delete(context.Background(), &po)
					if err != nil {
						klog.Warningf("unable to delete pod %v/%v: %v", po.Namespace, po.Name, err)
					}

					klog.Infof("deleting service %v/%v", svc.Namespace, svc.Name)
					err = pcm.podManager.kubeClient.Delete(context.Background(), &svc)
					if err != nil {
						klog.Warningf("unable to delete service %v/%v: %v", svc.Namespace, svc.Name, err)
					}
				}(podList.Items[i], *service)

				image := pod.Spec.Containers[0].Image
				// evict from single-pod cache
				podAndCl, found := pcm.cache[image]
				if found {
					target, _, err := net.SplitHostPort(podAndCl.grpcClient.Target())
					// Service IP should match the Grcp Client target IP for image in cache; log warning if different
					if err == nil && serviceIP != "" && target != serviceIP {
						klog.Warningf("IP of Service %s/%s [%s] does not match the IP of GRPC client [%s] cached for Pod %s/%s",
							pod.Namespace, serviceName, serviceIP, pod.Namespace, pod.Name, target)
					}

					// We delete the cache entry anyway
					delete(pcm.cache, image)
				}
				// evict from multi-pod cache
				pcm.evictPodFromMultiPodCache(image, pod.Name)
			}
		}
	}
}

func deletePodWithContext(kubeClient client.Client, pod *corev1.Pod) error {
	return kubeClient.Delete(context.Background(), pod)
}

func isCurrentCachedPod(pcm *podCacheManager, image string, pod *corev1.Pod) bool {
	// check single-pod cache
	cached, ok := pcm.cache[image]
	if ok && cached.pod.Name == pod.Name {
		return true
	}
	// check multi-pod cache
	for _, pi := range pcm.pods[image] {
		if pi.pod.Name == pod.Name {
			return true
		}
	}
	return false
}

// deleteFailedPod deletes a failed pod and removes it from cache if it is still the current pod.
func (pcm *podCacheManager) deleteFailedPod(pod *corev1.Pod, image string) {
	err := deletePodWithContext(pcm.podManager.kubeClient, pod)
	if err != nil {
		klog.Warningf("Failed to delete failed pod %v/%v: %v", pod.Namespace, pod.Name, err)
		return
	}

	// evict from single-pod cache
	if cached, ok := pcm.cache[image]; ok && cached.pod.Name == pod.Name {
		delete(pcm.cache, image)
		klog.Infof("Evicted pod from cache for image %v", image)
	}
}

// podManager is responsible for:
// - creating a pod
// - retrieving an existing pod
// - waiting for the pod to be running and ready
// - caching the metadata (e.g. entrypoint) for the image.
type podManager struct {
	// kubeClient is the kubernetes client
	kubeClient client.Client
	// namespace holds the namespace where the executors run
	namespace string

	//Namespace where the function-runner is running
	managerNamespace string

	// wrapperServerImage is the image name of the wrapper server
	wrapperServerImage string

	// podReadyCh is a channel to receive requests to get GRPC client from each function evaluation request handler.
	podReadyCh chan<- *imagePodAndGRPCClient

	// imageMetadataCache is a cache of image name to digestAndEntrypoint.
	// Only podManager is allowed to touch this cache.
	// Its underlying type is map[string]*digestAndEntrypoint.
	imageMetadataCache sync.Map

	// podReadyTimeout is the timeout podManager will wait for the pod to be ready before reporting an error
	podReadyTimeout time.Duration

	// The name of the configmap in the same namespace as the function-runner is running.
	// It should contain a pod manifest yaml in .data.template and frontend service
	// manifest yaml in .data.serviceTemplate
	// The pod manifest is expected to set up wrapper-server as the entrypoint
	// of the main container, which must be called "function".
	// Pod manager will replace the image
	functionPodTemplateName string

	// The maximum size of grpc messages sent to KRM function evaluator pods
	maxGrpcMessageSize int

	// If true enables the use of private registries and their authentication
	enablePrivateRegistries bool
	// The path of the secret used for authenticating to custom registries
	registryAuthSecretPath string
	// The name of the secret used for authenticating to custom registries
	registryAuthSecretName string
	// If enabled, will prioritize use of user provided TLS secret when accessing registries
	enablePrivateRegistriesTls bool
	// The path of the secret used in tls configuration
	tlsSecretPath string
}

type digestAndEntrypoint struct {
	// digest is a hex string
	digest string
	// entrypoint is the entrypoint of the image
	entrypoint []string
}

// getFuncEvalPodClient ensures there is a pod running and ready for the image.
// It will send it to the podReadyCh channel when the pod is ready. ttl is the
// time-to-live period for the pod. If useGenerateName is false, it will try to
// create a pod with a fixed name. Otherwise, it will create a pod and let the
// apiserver to generate the name from a template.
func (pm *podManager) getFuncEvalPodClient(ctx context.Context, image string, ttl time.Duration, useGenerateName bool) {
	c, err := func() (*podAndGRPCClient, error) {
		podTemplate, err := pm.retrieveOrCreatePod(ctx, image, ttl, useGenerateName)
		if err != nil {
			return nil, err
		}

		// Service name is Image Label set on Pod manifest
		serviceName := podTemplate.Labels[krmFunctionImageLabel]
		podKey := client.ObjectKeyFromObject(podTemplate)

		serviceKey, err := pm.retrieveOrCreateService(ctx, serviceName)
		if err != nil {
			return nil, err
		}

		serviceUrl, err := pm.getServiceUrlOnceEndpointActive(ctx, serviceKey, podKey)
		if err != nil {
			return nil, err
		}

		if serviceUrl == "" {
			return nil, fmt.Errorf("service %s/%s does not have valid Url", serviceKey.Namespace, serviceKey.Name)
		}

		address := net.JoinHostPort(serviceUrl, defaultWrapperServerPort)
		cc, err := grpc.NewClient(address,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithDefaultCallOptions(
				grpc.MaxCallRecvMsgSize(pm.maxGrpcMessageSize),
				grpc.MaxCallSendMsgSize(pm.maxGrpcMessageSize),
				grpc.WaitForReady(true),
			),
		)
		if err != nil {
			return nil, fmt.Errorf("failed to dial grpc function evaluator on %q for pod %s/%s: %w", address, podKey.Namespace, podKey.Name, err)
		}
		return &podAndGRPCClient{
			pod:        podKey,
			grpcClient: cc,
		}, err
	}()
	pm.podReadyCh <- &imagePodAndGRPCClient{
		image:            image,
		podAndGRPCClient: c,
		err:              err,
	}
}

func (pm *podManager) InspectOrCreateSecret(ctx context.Context, registryAuthSecretPath string, registryAuthSecretName string) error {
	podSecret := &corev1.Secret{}
	// using pod manager client since this secret is only related to these pods and nothing else
	err := pm.kubeClient.Get(context.Background(), client.ObjectKey{
		Name:      registryAuthSecretName,
		Namespace: pm.namespace,
	}, podSecret)
	if err != nil {
		if client.IgnoreNotFound(err) != nil {
			// Error other than "not found" occurred
			return err
		}
		klog.Infof("Secret for private registry pods does not exist and is required. Generating Secret Now")
		dockerConfigBytes, err := os.ReadFile(registryAuthSecretPath)
		if err != nil {
			return err
		}
		// Secret does not exist, create it
		podSecret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      registryAuthSecretName,
				Namespace: pm.namespace,
			},
			Data: map[string][]byte{
				".dockerconfigjson": dockerConfigBytes,
			},
			Type: corev1.SecretTypeDockerConfigJson,
		}
		err = pm.kubeClient.Create(ctx, podSecret)
		if err != nil {
			return err
		}

		klog.Infof("Private registry secret created successfully")
	} else {
		klog.Infof("Private registry secret already exists")
		// use the bytes Data of the user secret and compare it to the data of the pod secret
		dockerConfigBytes, err := os.ReadFile(registryAuthSecretPath)
		if err != nil {
			return err
		}
		// Compare the data of the two secrets
		if string(podSecret.Data[".dockerconfigjson"]) == string(dockerConfigBytes) {
			klog.Infof("The data content of the user given secret matches the private registry secret.")
		} else {
			klog.Infof("The data content of the private registry secret does not match given secret")
			// Patch the secret on the pods with the data from the user secret
			podSecret.Data[".dockerconfigjson"] = dockerConfigBytes
			err = pm.kubeClient.Update(ctx, podSecret)
			if err != nil {
				return err
			}
			klog.Infof("Private registry secret patched successfully.")
		}
	}
	return nil
}

// DockerConfig represents the structure of Docker config.json
type DockerConfig struct {
	Auths map[string]authn.AuthConfig `json:"auths"`
}

// imageDigestAndEntrypoint gets the entrypoint of a container image by looking at its metadata.
func (pm *podManager) imageDigestAndEntrypoint(ctx context.Context, image string) (*digestAndEntrypoint, error) {
	start := time.Now()
	defer func() {
		klog.Infof("getting image metadata for %v took %v", image, time.Since(start))
	}()

	ref, err := name.ParseReference(image)
	if err != nil {
		klog.Errorf("we got an error parsing the ref %v", err)
		return nil, err
	}

	var auth authn.Authenticator
	if pm.enablePrivateRegistries && !strings.HasPrefix(image, defaultRegistry) {
		if err := pm.ensureCustomAuthSecret(ctx, pm.registryAuthSecretPath, pm.registryAuthSecretName); err != nil {
			return nil, err
		}

		auth, err = pm.getCustomAuth(ref, pm.registryAuthSecretPath)
		if err != nil {
			return nil, err
		}
	} else {
		auth, err = authn.DefaultKeychain.Resolve(ref.Context())
		if err != nil {
			klog.Errorf("error resolving default keychain: %v", err)
			return nil, err
		}
	}

	return pm.getImageMetadata(ctx, ref, auth, image)
}

// ensureCustomAuthSecret ensures that, if an image from a custom registry is requested, the appropriate credentials are passed into a secret for function pods to use when pulling. If the secret does not already exist, it is created.
func (pm *podManager) ensureCustomAuthSecret(ctx context.Context, registryAuthSecretPath string, registryAuthSecretName string) error {
	if err := pm.InspectOrCreateSecret(ctx, registryAuthSecretPath, registryAuthSecretName); err != nil {
		return err
	}
	return nil
}

// getCustomAuth reads and parses the custom registry auth file from the mounted secret.
func (pm *podManager) getCustomAuth(ref name.Reference, registryAuthSecretPath string) (authn.Authenticator, error) {
	dockerConfigBytes, err := os.ReadFile(registryAuthSecretPath)
	if err != nil {
		klog.Errorf("error reading authentication file %v", err)
		return nil, err
	}

	var dockerConfig DockerConfig
	if err := json.Unmarshal(dockerConfigBytes, &dockerConfig); err != nil {
		klog.Errorf("error unmarshaling authentication file %v", err)
		return nil, err
	}

	return authn.FromConfig(dockerConfig.Auths[ref.Context().RegistryStr()]), nil
}

// getImageMetadata retrieves the image digest and entrypoint.
func (pm *podManager) getImageMetadata(ctx context.Context, ref name.Reference, auth authn.Authenticator, image string) (*digestAndEntrypoint, error) {
	img, err := pm.getImage(ctx, ref, auth, image)
	if err != nil {
		return nil, err
	}
	hash, err := img.Digest()
	if err != nil {
		return nil, err
	}
	configFile, err := img.ConfigFile()
	if err != nil {
		return nil, err
	}
	// TODO: to handle all scenario, we should follow https://docs.docker.com/engine/reference/builder/#understand-how-cmd-and-entrypoint-interact.
	cfg := configFile.Config
	entrypoint := cfg.Entrypoint
	if len(entrypoint) == 0 {
		entrypoint = cfg.Cmd
	}
	de := &digestAndEntrypoint{
		digest:     hash.Hex,
		entrypoint: entrypoint,
	}
	pm.imageMetadataCache.Store(image, de)
	return de, nil
}

func (pm *podManager) getImage(ctx context.Context, ref name.Reference, auth authn.Authenticator, image string) (containerregistry.Image, error) {
	// if private registries or their appropriate tls configuration are disabled in the config we pull image with default operation otherwise try and use their tls cert's
	if !pm.enablePrivateRegistries || strings.HasPrefix(image, defaultRegistry) || !pm.enablePrivateRegistriesTls {
		return remote.Image(ref, remote.WithAuth(auth), remote.WithContext(ctx))
	}
	tlsFile := "ca.crt"
	// Check if mounted secret location contains CA file.
	if _, err := os.Stat(pm.tlsSecretPath); os.IsNotExist(err) {
		return nil, err
	}
	if _, errCRT := os.Stat(filepath.Join(pm.tlsSecretPath, "ca.crt")); os.IsNotExist(errCRT) {
		if _, errPEM := os.Stat(filepath.Join(pm.tlsSecretPath, "ca.pem")); os.IsNotExist(errPEM) {
			return nil, fmt.Errorf("ca.crt not found: %v, and ca.pem also not found: %v", errCRT, errPEM)
		}
		tlsFile = "ca.pem"
	}
	// Load the custom TLS configuration
	tlsConfig, err := loadTLSConfig(filepath.Join(pm.tlsSecretPath, tlsFile))
	if err != nil {
		return nil, err
	}
	// Create a custom HTTPS transport
	transport := createTransport(tlsConfig)

	// Attempt image pull with given custom TLS cert
	img, tlsErr := remote.Image(ref, remote.WithAuth(auth), remote.WithContext(ctx), remote.WithTransport(transport))
	if tlsErr != nil {
		// If manifest doesn't exist, return immediately without retry
		if matched, _ := regexp.MatchString("manifest[_ ]unknown", strings.ToLower(tlsErr.Error())); matched {
			return nil, tlsErr
		}
		// Attempt without given custom TLS cert but with default keychain
		klog.Errorf("Pulling image %s with the provided TLS Cert has failed with error %v", image, tlsErr)
		klog.Infof("Attempting image pull with default keychain instead of provided TLS Cert")
		return remote.Image(ref, remote.WithAuth(auth), remote.WithContext(ctx))
	}
	return img, tlsErr
}

func loadTLSConfig(caCertPath string) (*tls.Config, error) {
	// Read the CA certificate file
	caCert, err := os.ReadFile(caCertPath)
	if err != nil {
		return nil, err
	}
	// Append the CA certificate to the system pool
	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM(caCert) {
		return nil, fmt.Errorf("failed to append certificates from PEM")
	}
	// Create a tls.Config with the CA pool
	tlsConfig := &tls.Config{
		RootCAs:    caCertPool,
		MinVersion: tls.VersionTLS12,
	}
	return tlsConfig, nil
}

func createTransport(tlsConfig *tls.Config) *http.Transport {
	return &http.Transport{
		TLSClientConfig: tlsConfig,
	}
}

// retrieveOrCreatePod retrieves or creates a pod for an image.
func (pm *podManager) retrieveOrCreatePod(ctx context.Context, image string, ttl time.Duration, useGenerateName bool) (*corev1.Pod, error) {
	var de *digestAndEntrypoint
	var replacePod bool
	var currentPod *corev1.Pod
	var err error
	val, found := pm.imageMetadataCache.Load(image)
	if !found {
		de, err = pm.imageDigestAndEntrypoint(ctx, image)
		if err != nil {
			return nil, fmt.Errorf("unable to get the entrypoint for %v: %w", image, err)
		}
	} else {
		de = val.(*digestAndEntrypoint)
	}

	podId, err := podID(image, de.digest)
	if err != nil {
		return nil, err
	}

	// Try to retrieve the pod. Lookup the pod by label to see if there is a pod that can be reused.
	// Looking it up locally may not work if there are more than one instance of the function runner,
	// since the pod may be created by one the other instance and the current instance is not aware of it.
	// TODO: It's possible to set up a Watch in the fn runner namespace, and always try to maintain a up-to-date local cache.
	podList := &corev1.PodList{}
	podTemplate, templateVersion, err := pm.getBasePodTemplate(ctx)
	if err != nil {
		klog.Errorf("failed to generate a base pod template: %v", err)
		return nil, fmt.Errorf("failed to generate a base pod template: %w", err)
	}
	pm.appendImagePullSecret(image, podTemplate)
	err = pm.kubeClient.List(ctx, podList, client.InNamespace(pm.namespace), client.MatchingLabels(map[string]string{krmFunctionImageLabel: podId}))
	if err != nil {
		klog.Warningf("error when listing pods for %q: %v", image, err)
	}
	if err == nil && len(podList.Items) > 0 {
		// TODO: maybe we should randomly pick one that is no being deleted.
		for _, pod := range podList.Items {
			if pod.DeletionTimestamp == nil {
				if isPodTemplateSameVersion(&pod, templateVersion) {
					klog.Infof("retrieved function evaluator pod %v/%v for %q", pod.Namespace, pod.Name, image)
					return &pod, nil
				} else {
					replacePod = true
					currentPod = &pod
					break
				}
			}
		}
	}

	err = pm.patchNewPodContainer(podTemplate, *de, image)
	if err != nil {
		return nil, fmt.Errorf("unable to apply the pod: %w", err)
	}
	pm.patchNewPodMetadata(podTemplate, ttl, podId, templateVersion)

	// Server-side apply doesn't support name generation. We have to use Create
	// if we need to use name generation.
	if useGenerateName || replacePod {
		podTemplate.GenerateName = podId + "-"
		err = pm.kubeClient.Create(ctx, podTemplate, client.FieldOwner(fieldManagerName))
		if err != nil {
			if currentPod != nil {
				klog.V(4).Infof("Pod Name: %s Pod Status: %v", currentPod.Name, currentPod.Status)
			}
			return nil, fmt.Errorf("unable to apply the pod: %w", err)
		}
		if replacePod {
			err = pm.kubeClient.Delete(ctx, currentPod)
			if err != nil {
				return nil, fmt.Errorf("unable to clean up previous pod: %w", err)
			}
		}
	} else {
		podTemplate.Name = podId
		err = pm.kubeClient.Patch(ctx, podTemplate, client.Apply, client.FieldOwner(fieldManagerName))
		if err != nil {
			return nil, fmt.Errorf("unable to apply the pod: %w", err)
		}
	}

	klog.Infof("created KRM function evaluator pod %v/%v for %q", podTemplate.Namespace, podTemplate.Name, image)
	return podTemplate, nil
}

// Either gets the pod template from configmap, or from an inlined pod template. Also provides the version of the template
func (pm *podManager) getBasePodTemplate(ctx context.Context) (*corev1.Pod, string, error) {
	if pm.functionPodTemplateName != "" {
		podTemplateCm := &corev1.ConfigMap{}

		err := pm.kubeClient.Get(ctx, client.ObjectKey{
			Name:      pm.functionPodTemplateName,
			Namespace: pm.managerNamespace,
		}, podTemplateCm)
		if err != nil {
			klog.Errorf("Could not get Configmap containing function pod template: %s/%s", pm.managerNamespace, pm.functionPodTemplateName)
			return nil, "", err
		}

		podTemplate, ok := podTemplateCm.Data["template"]
		if !ok {
			return nil, "", fmt.Errorf("function pod template with key template does not exist in Configmap %s", pm.functionPodTemplateName)
		}

		decoder := yamlutil.NewYAMLOrJSONDecoder(strings.NewReader(podTemplate), 100)
		var basePodTemplate corev1.Pod
		err = decoder.Decode(&basePodTemplate)

		if err != nil {
			return nil, "", fmt.Errorf("unable to decode function pod template: %w", err)
		}

		return &basePodTemplate, podTemplateCm.ResourceVersion, nil
	} else {

		inlineBasePodTemplate := &corev1.Pod{
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
						Image: pm.wrapperServerImage,
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

		return inlineBasePodTemplate, inlineTemplateVersionv1, nil
	}
}

// retrieveOrCreateService retrieves or creates a Service pointing to the Function PoD
func (pm *podManager) retrieveOrCreateService(ctx context.Context, serviceName string) (client.ObjectKey, error) {

	// Try to retrieve the existing Service, if present
	existingService := &corev1.Service{}
	err := pm.kubeClient.Get(ctx, client.ObjectKey{Namespace: pm.namespace, Name: serviceName}, existingService)

	if err == nil {
		klog.Infof("retrieved function evaluator service %v/%v", existingService.Namespace, existingService.Name)
		return client.ObjectKey{Namespace: existingService.Namespace, Name: existingService.Name}, nil
	}

	// If the service does not exist, generate template and create it
	if errors.IsNotFound(err) {
		serviceTemplate, err := pm.getBaseServiceTemplate(ctx)
		if err != nil {
			return client.ObjectKey{}, err
		}

		serviceTemplate.Namespace = pm.namespace
		serviceTemplate.Name = serviceName
		if serviceTemplate.Labels == nil {
			serviceTemplate.Labels = map[string]string{}
		}
		serviceTemplate.Labels["app.kubernetes.io/name"] = serviceName
		if serviceTemplate.Spec.Selector == nil {
			serviceTemplate.Spec.Selector = map[string]string{}
		}
		// Pod has same Label as Service Name (PodId) for correct mapping
		serviceTemplate.Spec.Selector[krmFunctionImageLabel] = serviceName

		err = pm.kubeClient.Create(ctx, serviceTemplate, client.FieldOwner(fieldManagerName))
		if err != nil {
			klog.Errorf("failed to create service %v/%v: %v", pm.namespace, serviceName, err)
			// Try to retrieve again
			if errors.IsAlreadyExists(err) {
				klog.Infof("service %v/%v already exists - trying to retrieve it", pm.namespace, serviceName)
				err = pm.kubeClient.Get(ctx, client.ObjectKey{Namespace: pm.namespace, Name: serviceName}, existingService)
				if err == nil {
					klog.Infof("retrieved function evaluator service %v/%v", existingService.Namespace, existingService.Name)
					return client.ObjectKey{Namespace: existingService.Namespace, Name: existingService.Name}, nil
				}
			}
			return client.ObjectKey{}, err
		}

		klog.Infof("created frontend service %v for function evaluator pod", serviceName)
		return client.ObjectKeyFromObject(serviceTemplate), nil
	}

	// Return other errors
	klog.Errorf("failed to retrieve service %v/%v: %v", pm.namespace, serviceName, err)
	return client.ObjectKey{}, err
}

func (pm *podManager) getBaseServiceTemplate(ctx context.Context) (*corev1.Service, error) {
	if pm.functionPodTemplateName != "" {
		serviceTemplateCm := &corev1.ConfigMap{}

		err := pm.kubeClient.Get(ctx, client.ObjectKey{
			Name:      pm.functionPodTemplateName,
			Namespace: pm.managerNamespace,
		}, serviceTemplateCm)
		if err != nil {
			klog.Errorf("Could not get ConfigMap containing function service template: %s/%s", pm.managerNamespace, pm.functionPodTemplateName)
			return nil, err
		}

		serviceTemplate, ok := serviceTemplateCm.Data["serviceTemplate"]
		if !ok {
			return nil, fmt.Errorf("function pod service template with key serviceTemplate does not exist in Configmap %s", pm.functionPodTemplateName)
		}

		decoder := yamlutil.NewYAMLOrJSONDecoder(strings.NewReader(serviceTemplate), 100)
		var baseServiceTemplate corev1.Service
		err = decoder.Decode(&baseServiceTemplate)

		if err != nil {
			return nil, fmt.Errorf("unable to decode function service template: %w", err)
		}

		return &baseServiceTemplate, nil
	} else {
		servicePort, _ := strconv.Atoi(defaultWrapperServerPort)
		inlineBaseServiceTemplate := &corev1.Service{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "v1",
				Kind:       "Service",
			},
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					"internal.kpt.dev/upstream-identifier": "|Service|porch-system|function-runner",
				},
				Labels: map[string]string{
					"app.kubernetes.io/name": "to-be-replaced",
				},
			},
			Spec: corev1.ServiceSpec{
				Selector: map[string]string{
					krmFunctionImageLabel: "to-be-replaced",
				},
				Ports: []corev1.ServicePort{
					{
						Protocol:   corev1.ProtocolTCP,
						Port:       int32(servicePort), // #nosec G115 G109 -- servicePort always in 1–65535
						TargetPort: intstr.FromInt(servicePort),
					},
				},
				Type: corev1.ServiceTypeClusterIP,
			},
		}

		return inlineBaseServiceTemplate, nil
	}
}

func hasImagePullSecret(podTemplate *corev1.Pod, secretName string) bool {
	for _, secret := range podTemplate.Spec.ImagePullSecrets {
		if secret.Name == secretName {
			return true
		}
	}
	return false
}

// if a custom image is requested, append the secret provided to authenticate to the imagePullSecret of the kpt function pod if it does not already exist
func (pm *podManager) appendImagePullSecret(image string, podTemplate *corev1.Pod) {
	if pm.enablePrivateRegistries && !strings.HasPrefix(image, defaultRegistry) {
		if !hasImagePullSecret(podTemplate, pm.registryAuthSecretName) {
			podTemplate.Spec.ImagePullSecrets = append(podTemplate.Spec.ImagePullSecrets, corev1.LocalObjectReference{
				Name: pm.registryAuthSecretName,
			})
		}
	}
}

// Patches the expected port, and the original entrypoint and image of the kpt function into the function container
func (pm *podManager) patchNewPodContainer(pod *corev1.Pod, de digestAndEntrypoint, image string) error {
	var patchedContainer bool
	for i := range pod.Spec.Containers {
		container := &pod.Spec.Containers[i]
		if container.Name == functionContainerName {
			container.Args = append(container.Args,
				"--port", defaultWrapperServerPort,
				"--max-request-body-size", strconv.Itoa(pm.maxGrpcMessageSize),
				"--",
			)
			container.Args = append(container.Args, de.entrypoint...)
			container.Image = image
			patchedContainer = true
		}
	}
	if !patchedContainer {
		return fmt.Errorf("failed to find the %v container in the pod", functionContainerName)
	}
	return nil
}

// Patch labels and annotations so the cache manager can keep track of the pod
func (pm *podManager) patchNewPodMetadata(pod *corev1.Pod, ttl time.Duration, podId, templateVersion string) {
	pod.Namespace = pm.namespace
	annotations := pod.Annotations
	if annotations == nil {
		annotations = make(map[string]string)
	}
	annotations[reclaimAfterAnnotation] = fmt.Sprintf("%v", time.Now().Add(ttl).Unix())
	annotations[templateVersionAnnotation] = templateVersion
	pod.Annotations = annotations

	labels := pod.Labels
	if labels == nil {
		labels = make(map[string]string)
	}
	labels[krmFunctionImageLabel] = podId
	pod.Labels = labels
}

// getServiceUrlOnceEndpointActive retrieves the service full FQDN Url once the backend PoD is
// ready and Endpoint is active.
// Returned URL is in the format <service-name>.<namespace>.svc.cluster.local
func (pm *podManager) getServiceUrlOnceEndpointActive(ctx context.Context, serviceKey client.ObjectKey, podKey client.ObjectKey) (string, error) {
	var service corev1.Service
	var pod corev1.Pod
	//nolint:staticcheck
	var endpoint corev1.Endpoints
	var podReady = false

	if e := wait.PollUntilContextTimeout(ctx, 100*time.Millisecond, pm.podReadyTimeout, true, func(ctx context.Context) (done bool, err error) {
		if !podReady {
			err = pm.kubeClient.Get(ctx, podKey, &pod)
			if err != nil {
				return false, err
			}

			if pod.Status.Phase != "Running" {
				return false, nil
			}

			for _, cond := range pod.Status.Conditions {
				if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionTrue {
					podReady = true
				}
			}
			return false, nil
		}

		podIP := pod.Status.PodIP

		if err := pm.kubeClient.Get(ctx, serviceKey, &service); err != nil {
			return false, err
		}

		// Wait till the service has an active endpoint
		if err := pm.kubeClient.Get(ctx, serviceKey, &endpoint); err != nil {
			return false, err
		}

		if len(endpoint.Subsets) == 0 || len(endpoint.Subsets[0].Addresses) == 0 {
			return false, nil
		}

		// check if pod IP exists in endpoint addresses
		podIPFound := false
		for _, addr := range endpoint.Subsets[0].Addresses {
			if addr.IP == podIP {
				podIPFound = true
				break
			}
		}
		if !podIPFound {
			klog.Warningf("pod IP %s not found in service %s/%s endpoints", podIP, serviceKey.Namespace, serviceKey.Name)
			return false, fmt.Errorf("pod IP %s not found in service endpoints", podIP)
		}

		return true, nil
	}); e != nil {
		return "", fmt.Errorf("error occurred when waiting the pod and service to be ready. If the error is caused by timeout, you may want to examine the pod/service in namespace %q. Error: %w", pm.namespace, e)
	}

	return service.Name + "." + service.Namespace + serviceDnsNameSuffix, nil
}

// patchPodWithUnixTimeAnnotation patches the pod with the new updated TTL annotation.
func patchPodWithUnixTimeAnnotation(cl client.Client, podKey client.ObjectKey, ttl time.Duration) {
	patch := []byte(fmt.Sprintf(`{"metadata":{"annotations":{"%v": "%v"}}}`, reclaimAfterAnnotation, time.Now().Add(ttl).Unix()))
	pod := &corev1.Pod{}
	pod.Namespace = podKey.Namespace
	pod.Name = podKey.Name
	if err := cl.Patch(context.Background(), pod, client.RawPatch(types.MergePatchType, patch)); err != nil {
		klog.Warningf("unable to patch last-use annotation for pod %v/%v: %v", podKey.Namespace, podKey.Name, err)
	}
}

// Returns kpt function name and unique podId to be used as Pod Name
func podID(image, hash string) (string, error) {
	ref, err := name.ParseReference(image)
	if err != nil {
		return "", fmt.Errorf("unable to parse image reference %v: %w", image, err)
	}

	// repoName will be something like gcr.io/kpt-fn/set-namespace
	repoName := ref.Context().Name()
	parts := strings.Split(repoName, "/")
	name := strings.ReplaceAll(parts[len(parts)-1], "_", "-")
	return fmt.Sprintf("%v-%v", name, hash[:8]), nil
}

func isPodTemplateSameVersion(pod *corev1.Pod, templateVersion string) bool {
	currVersion, found := pod.Annotations[templateVersionAnnotation]
	if !found || currVersion != templateVersion {
		return false
	}
	return true
}
