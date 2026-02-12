// Copyright 2025 The kpt and Nephio Authors
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
	"net"
	"os"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

// podCacheManager manages the cache of the pods and the corresponding GRPC clients.
// It also does the garbage collection after pods' TTL.
// It has 2 receive-only channels: connectionRequestCh and podReadyCh.
// It listens to the connectionRequestCh channel and receives clientConnRequest from the
// GRPC request handlers and add them in the waitlists.
// It also listens to the podReadyCh channel. If a pod is ready, it notifies the
// goroutines by sending back the GRPC client by lookup the waitlists mapping.
type podCacheManager struct {
	gcScanInterval time.Duration
	podTTL         time.Duration

	// connectionRequestCh receives requests for a connection to a KRM function evaluator pod
	connectionRequestCh <-chan *connectionRequest
	// podReadyCh is a channel to receive the information when a pod is ready.
	podReadyCh <-chan *podReadyResponse

	// functions maps KRM function image names to its pods and waitlist information.
	mu        sync.RWMutex
	functions map[string]*functionInfo

	podManager *podManager

	maxWaitlistLength          int
	maxParallelPodsPerFunction int
	configMap                  map[string]podCacheConfigEntry
}

type podCacheConfigEntry struct {
	Name                       string                      `json:"name,omitempty"`
	TimeToLive                 string                      `json:"timeToLive,omitempty"`
	MaxWaitlistLength          int                         `json:"maxWaitlistLength,omitempty"`
	MaxParallelPodsPerFunction int                         `json:"maxParallelPodsPerFunction,omitempty"`
	Resources                  corev1.ResourceRequirements `json:"resources,omitempty"`
}

// functionInfo holds the list of all pod instances for the same KRM function image.
type functionInfo struct {
	// status of all pods belonging to the same KRM function image
	pods []functionPodInfo
}

// functionPodInfo represents the state of a single pod instance.
type functionPodInfo struct {
	// podData contains the information about the pod, returned by the podManager
	// It is nil until the pod is actually started
	*podData
	// waitlist is used to temporarily store connection requests until the pod is started
	waitlist []chan<- *connectionResponse
	// time of last function evaluation, used by the garbage collector to identify idle pods
	lastActivity time.Time
	// mutex used to prevent concurrent fn evaluations in the same pod
	fnEvaluationMutex *sync.Mutex
	// the number of currently ongoing and waiting fn evaluations in the pod
	concurrentEvaluations *atomic.Int32
}

func (pcm *podCacheManager) redistributeLoad(image string, fn *functionInfo, connections []chan<- *connectionResponse) bool {
	pcm.removeUnhealthyPods(fn, false)
	redistributed := false
	for _, ch := range connections {
		bestPodIndex, _ := pcm.findBestPod(fn)
		if bestPodIndex != -1 {
			pod := pcm.functions[image].pods[bestPodIndex]
			if pod.podData != nil {
				pod.SendResponse(ch, nil)
			} else {
				pod.waitlist = append(pod.waitlist, ch)
			}
			redistributed = true
		}
	}
	return redistributed
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
		case req := <-pcm.connectionRequestCh:
			fn := pcm.FunctionInfo(req.image)

			shouldScaleUp := false
			pcm.removeUnhealthyPods(fn, false)
			bestPodIndex, bestWaitlistLen := pcm.findBestPod(fn)
			_, maxWaitlist, maxPods := pcm.getParamsForImage(req.image)
			if bestPodIndex == -1 {
				shouldScaleUp = true
			} else {
				if bestWaitlistLen >= maxWaitlist && len(fn.pods) < maxPods {
					shouldScaleUp = true
				}
			}

			if shouldScaleUp {
				klog.Infof("Scaling up for image %s. No idle pods available. Starting a new pod.", req.image)

				fn.pods = append(fn.pods, NewPodInfo(req.responseCh))
				go pcm.podManager.getFuncEvalPodClient(context.Background(), req.image, len(fn.pods), pcm.configMap[req.image], true)
			} else {
				pod := &fn.pods[bestPodIndex]
				klog.Infof("Queuing request for %s on pod instance #%d (queue length will be %d)", req.image, bestPodIndex, bestWaitlistLen+1)
				pod.lastActivity = time.Now()
				pod.concurrentEvaluations.Add(1)
				if pod.podData != nil {
					pod.SendResponse(req.responseCh, nil)
				} else {
					pod.waitlist = append(pod.waitlist, req.responseCh)
				}
			}

		case podReadyMsg := <-pcm.podReadyCh:
			if podReadyMsg.image == "" {
				klog.Error("Received a 'pod ready' message with an empty KRM image name. This indicates a logical error in the code.")
				continue
			}
			fn, ok := pcm.functions[podReadyMsg.image]
			if !ok {
				klog.Errorf("Received a ready pod for %q, but the KRM function is missing from the pool! Ignoring.", podReadyMsg.image)
				continue
			}
			// Find the first pod with nil podData, which means it is pending creation.
			toUpdate := slices.IndexFunc(fn.pods, func(pod functionPodInfo) bool {
				return pod.podData == nil
			})
			if toUpdate == -1 {
				klog.Errorf("Received a ready pod for %q, but no pending instance was found in the pod pool. Total of %d pods was in the pool. Ignoring.", podReadyMsg.image, len(fn.pods))
				continue
			}

			if podReadyMsg.err != nil {
				klog.Warningf("Pod creation failed for image %s: %v", podReadyMsg.image, podReadyMsg.err)
				waitListToRedistribute := fn.pods[toUpdate].waitlist
				failedPod := fn.pods[toUpdate]
				fn.pods = slices.Delete(fn.pods, toUpdate, toUpdate+1)
				redistributed := false
				if len(fn.pods) > 0 {
					redistributed = pcm.redistributeLoad(podReadyMsg.image, fn, waitListToRedistribute)
				}
				if !redistributed {
					for _, ch := range waitListToRedistribute {
						failedPod.SendResponse(ch, podReadyMsg.err)
					}
				}
				pcm.DeletePodWithServiceInBackgroundByObjectKey(podReadyMsg.podData)
				continue
			}

			pod := &fn.pods[toUpdate]
			pod.podData = &podReadyMsg.podData
			pod.lastActivity = time.Now()
			klog.Infof("New pod %s is ready for image %s. Total number of pods for image: %d", podReadyMsg.podKey.Name, podReadyMsg.image, len(fn.pods))
			for _, ch := range pod.waitlist {
				pod.SendResponse(ch, nil)
			}
			pod.waitlist = nil

		case <-tick:
			pcm.garbageCollector()
		}
	}
}

// loadPodCacheConfig loads the pod cache configuration from the given YAML file path.
// It parses the YAML into a slice of podCacheConfigEntry, then builds a map from function image name to its config entry.
// This map is used to look up per-function pod cache parameters (TTL, maxWaitlist, maxPods).
func loadPodCacheConfig(configPath string) (map[string]podCacheConfigEntry, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}
	var entries []podCacheConfigEntry
	if err := yaml.Unmarshal(data, &entries); err != nil {
		return nil, err
	}
	configMap := make(map[string]podCacheConfigEntry)
	for _, entry := range entries {
		configMap[entry.Name] = entry
		klog.V(2).Infof("Loaded pod cache config: %s -> TTL: %s, MaxWaitlistLength: %d, MaxParallelPodsPerFunction: %d",
			entry.Name, entry.TimeToLive, entry.MaxWaitlistLength, entry.MaxParallelPodsPerFunction)
	}

	klog.Infof("Loaded %d pod cache configurations from %s", len(configMap), configPath)
	return configMap, nil
}

// getParamsForImage returns the pod cache parameters (TTL, maxWaitlist, maxPods) for the given function image.
// If the image is present in the configMap, it returns the specific parameters for that image.
// Otherwise, it falls back to the global defaults (pcm.podTTL, pcm.maxWaitlistLength, pcm.maxParallelPodsPerFunction).
func (pcm *podCacheManager) getParamsForImage(image string) (ttl time.Duration, maxWaitlist, maxPods int) {
	if entry, ok := pcm.configMap[image]; ok {
		parsedTTL, err := time.ParseDuration(entry.TimeToLive)
		if err != nil || parsedTTL == 0 {
			parsedTTL = pcm.podTTL
		}
		maxWaitlist := entry.MaxWaitlistLength
		if maxWaitlist == 0 {
			maxWaitlist = pcm.maxWaitlistLength
		}
		maxPods := entry.MaxParallelPodsPerFunction
		if maxPods == 0 {
			maxPods = pcm.maxParallelPodsPerFunction
		}
		return parsedTTL, maxWaitlist, maxPods
	}
	return pcm.podTTL, pcm.maxWaitlistLength, pcm.maxParallelPodsPerFunction
}

func (pcm *podCacheManager) FunctionInfo(image string) *functionInfo {
	fn, ok := pcm.functions[image]
	if !ok {
		fn = &functionInfo{}
		pcm.functions[image] = fn
	}
	return fn
}

func (pcm *podCacheManager) retrieveFunctionPods(ctx context.Context) error {
	_, templateVersion, err := pcm.podManager.getBasePodTemplate(ctx)
	if err != nil {
		klog.Errorf("failed to generate a base pod template: %v", err)
		return fmt.Errorf("failed to generate a base pod template: %w", err)
	}

	podList := &corev1.PodList{}
	err = pcm.podManager.kubeClient.List(ctx, podList, client.InNamespace(pcm.podManager.namespace), client.HasLabels{krmFunctionImageLabel})
	if err != nil {
		klog.Warningf("error when listing pods in namespace: %q: %v", pcm.podManager.namespace, err)
	}
	if err == nil && len(podList.Items) > 0 {
		for _, pod := range podList.Items {
			if pod.DeletionTimestamp == nil {
				if isPodTemplateSameVersion(&pod, templateVersion) {

					// Service name is Image Label set on Pod manifest
					serviceName := pod.Labels[krmFunctionImageLabel]
					podKey := client.ObjectKeyFromObject(&pod)

					serviceTemplate, err := pcm.podManager.retrieveOrCreateService(ctx, serviceName)
					if err != nil {
						return err
					}
					serviceKey := client.ObjectKeyFromObject(serviceTemplate)

					//nolint:staticcheck
					var endpoint corev1.Endpoints
					if err := pcm.podManager.kubeClient.Get(ctx, serviceKey, &endpoint); err != nil {
						return err
					}
					// Remove the pod if more than one address is found in the endpoint
					if len(endpoint.Subsets[0].Addresses) > 1 {
						err = pcm.deletePodAndWait(&pod)
						if err != nil {
							klog.Errorf("failed to delete pod %s/%s: %v", pod.Namespace, pod.Name, err)
						}
						continue
					}

					image := pod.Spec.Containers[0].Image
					fn := pcm.FunctionInfo(image)
					if len(fn.pods) < pcm.maxParallelPodsPerFunction && pod.Status.Phase == corev1.PodRunning {
						pData, err := pcm.podManager.createPodData(ctx, serviceKey, podKey, image)
						if err == nil {
							klog.Infof("retrieved function evaluator pod %s/%s for %s", pod.Namespace, pod.Name, image)
							fn.pods = append(fn.pods, NewPodInfo(nil))
							pcm.podManager.podReadyCh <- &podReadyResponse{
								podData: *pData,
								err:     nil,
							}
							continue
						}
					}

					klog.Infof("Max parallel pods reached for %q, deleting %s/%s", image, pod.Namespace, pod.Name)
					pcm.DeletePodInBackground(&pod)
					pcm.DeleteServiceInBackground(serviceTemplate)
				}
			}
		}
	}
	return nil
}

// warmupCache starts preloading 1 pod in the background for each function specified in podCacheConfig
func (pcm *podCacheManager) warmupCache(podCacheConfig string) error {
	start := time.Now()
	defer func() {
		klog.Infof("cache warming is completed and it took %v", time.Since(start))
	}()
	content, err := os.ReadFile(podCacheConfig)
	if err != nil {
		return err
	}
	var entries []podCacheConfigEntry
	err = yaml.Unmarshal(content, &entries)
	if err != nil {
		return err
	}

	forEachConcurrently(entries, func(entry podCacheConfigEntry) {
		pcm.mu.Lock()
		fn := pcm.FunctionInfo(entry.Name)
		pcm.mu.Unlock()
		if len(fn.pods) == 0 {
			fn.pods = append(fn.pods, NewPodInfo(nil))
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			pcm.podManager.getFuncEvalPodClient(ctx, entry.Name, 1, pcm.configMap[entry.Name], false)
		}
		klog.Infof("preloaded pod cache for function %v", entry.Name)
	})
	return nil
}

func forEachConcurrently(m []podCacheConfigEntry, fn func(k podCacheConfigEntry)) {
	var wg sync.WaitGroup
	for _, v := range m {
		v := v

		wg.Add(1)
		go func() {
			defer wg.Done()
			fn(v)
		}()
	}
	// Wait for all the functions to complete.
	wg.Wait()
}

// findBestPod returns with the index of the least loaded healthy pod for the given function.
// If there are no suitable pods, it returns with -1.
func (pcm *podCacheManager) findBestPod(fn *functionInfo) (int, int) {
	if fn == nil {
		return -1, 0
	}
	if len(fn.pods) == 0 {
		return -1, 0
	}
	bestPodIdx := 0
	bestWaitlistLen := fn.pods[0].WaitlistLen()
	for i := 1; i < len(fn.pods); i++ {
		waitlistLen := fn.pods[i].WaitlistLen()
		if waitlistLen < bestWaitlistLen {
			bestPodIdx = i
			bestWaitlistLen = waitlistLen
		}
	}
	return bestPodIdx, bestWaitlistLen
}

// removeUnhealthyPods removes unhealthy pods from the function's pod list.
// If removeIdle is true, it will also remove idle pods that have reached their TTL.
func (pcm *podCacheManager) removeUnhealthyPods(fn *functionInfo, removeIdle bool) {
	if fn == nil {
		return
	}
	fn.pods = slices.DeleteFunc(fn.pods, func(pod functionPodInfo) bool {
		removeFromCache := false
		if pod.podData == nil {
			// pod is under creation
			return false
		}

		k8sPod := &corev1.Pod{}
		err := pcm.podManager.kubeClient.Get(context.Background(), *pod.podKey, k8sPod)
		if err != nil {
			if apierrors.IsNotFound(err) {
				klog.Infof("Removing deleted pod from cache for image %s", pod.image)
			} else {
				klog.Errorf("Failed to get pod %v, removing from cache: %v", pod.podKey, err)
			}
			removeFromCache = true
		}

		service := &corev1.Service{}
		err = pcm.podManager.kubeClient.Get(context.Background(), *pod.serviceKey, service)
		if err != nil {
			if apierrors.IsNotFound(err) {
				klog.Infof("Removing deleted service from cache for image %s", pod.image)
			} else {
				klog.Errorf("Failed to get service %v, removing from cache: %v", pod.serviceKey, err)
			}
			removeFromCache = true
		}

		err = pcm.podManager.kubeClient.Get(context.Background(), *pod.serviceKey, service)
		if err != nil {
			klog.Warningf("unable to find expected service %s namespace %s: %v", pod.serviceKey.Name, k8sPod.Namespace, err)
		}

		if k8sPod.Status.Phase == corev1.PodFailed {
			klog.Errorf("Evicting pod in failed state (%s/%s) from cache for image %s", k8sPod.Namespace, k8sPod.Name, pod.image)
			removeFromCache = true
		}

		serviceUrl := service.Name + "." + service.Namespace + serviceDnsNameSuffix
		if net.JoinHostPort(serviceUrl, defaultWrapperServerPort) != pod.grpcConnection.Target() {
			klog.Errorf("Evicting pod whose pod IP doesn't match with its grpc connection (%s/%s) from cache for image %s", k8sPod.Namespace, k8sPod.Name, pod.image)
			removeFromCache = true
		}
		ttl, _, _ := pcm.getParamsForImage(pod.image)
		if removeIdle && pod.WaitlistLen() == 0 && time.Since(pod.lastActivity) > ttl {
			klog.Infof("Removing idle pod %q that reached its TTL from cache for image %s", k8sPod.Name, pod.image)
			removeFromCache = true
		}

		if removeFromCache {
			pcm.DeletePodInBackground(k8sPod)
			pcm.DeleteServiceInBackground(service)
		}

		return removeFromCache
	})
}

// garbageCollector runs periodically and removes unhealthy and idle pods from the pool.
// TODO: We can use Watch + periodically reconciliation to manage the pods,
// the pod evaluator will become a controller.
func (pcm *podCacheManager) garbageCollector() {
	// Process each image's pods
	for image, fn := range pcm.functions {
		pcm.removeUnhealthyPods(fn, true)

		// Clean up empty slices
		if len(fn.pods) == 0 {
			delete(pcm.functions, image)
		}
	}
}

func (pcm *podCacheManager) DeletePodWithServiceInBackgroundByObjectKey(podData podData) {
	k8sPod := &corev1.Pod{}
	if podData.podKey != nil {
		err := pcm.podManager.kubeClient.Get(context.Background(), *podData.podKey, k8sPod)
		if err != nil {
			klog.Warningf("unable to find pod %s in namespace: %s: %v", podData.podKey.Name, podData.podKey.Namespace, err)
		}
		pcm.DeletePodInBackground(k8sPod)
	}

	service := &corev1.Service{}
	if podData.serviceKey != nil {
		err := pcm.podManager.kubeClient.Get(context.Background(), *podData.serviceKey, service)
		if err != nil {
			klog.Warningf("unable to find service %s in namespace %s: %v", podData.serviceKey.Name, podData.serviceKey.Namespace, err)
		}
		pcm.DeleteServiceInBackground(service)
	}
}

func (pcm *podCacheManager) deletePodAndWait(k8sPod *corev1.Pod) error {
	err := pcm.podManager.kubeClient.Delete(context.Background(), k8sPod)
	if err != nil {
		klog.Errorf("Failed to delete pod %s/%s from cluster: %v", k8sPod.Namespace, k8sPod.Name, err)
	}

	if e := wait.PollUntilContextTimeout(context.Background(), 100*time.Millisecond, pcm.podManager.podReadyTimeout, true, func(ctx context.Context) (done bool, err error) {
		var current corev1.Pod
		err = pcm.podManager.kubeClient.Get(context.Background(), client.ObjectKeyFromObject(k8sPod), &current)
		if apierrors.IsNotFound(err) {
			return true, nil
		} else if err != nil {
			return false, fmt.Errorf("error while waiting for deletion: %w", err)
		}
		return false, nil
	}); e != nil {
		return fmt.Errorf("error occurred when waiting the deletion of pod. If the error is caused by timeout, you may want to examine the pod in namespace %q. Error: %w", pcm.podManager.namespace, e)
	}
	return nil
}

func (pcm *podCacheManager) DeletePodInBackground(k8sPod *corev1.Pod) {
	go func() {
		if k8sPod != nil && k8sPod.DeletionTimestamp.IsZero() && k8sPod.Name != "" {
			err := pcm.podManager.kubeClient.Delete(context.Background(), k8sPod)
			if err != nil {
				klog.Errorf("Failed to delete pod %s/%s from cluster: %v", k8sPod.Namespace, k8sPod.Name, err)
			}
		}
	}()
}

func (pcm *podCacheManager) DeleteServiceInBackground(svc *corev1.Service) {
	go func() {
		if svc != nil && svc.DeletionTimestamp.IsZero() && svc.Name != "" {
			err := pcm.podManager.kubeClient.Delete(context.Background(), svc)
			if err != nil {
				klog.Warningf("unable to delete service %s/%s: %v", svc.Namespace, svc.Name, err)
			}
		}
	}()
}

func NewPodInfo(firstResponseCh chan<- *connectionResponse) functionPodInfo {
	pod := functionPodInfo{
		waitlist:              []chan<- *connectionResponse{},
		podData:               nil, // This will be filled in when the pod is ready.
		lastActivity:          time.Now(),
		concurrentEvaluations: &atomic.Int32{},
		fnEvaluationMutex:     &sync.Mutex{},
	}
	if firstResponseCh != nil {
		pod.waitlist = append(pod.waitlist, firstResponseCh)
		pod.concurrentEvaluations.Add(1)
	}
	return pod
}

// SendResponse sends a reply to the connection request containing the pod data.
// If err != nil it sends `err` as an error response.
// It sends and error response if the pod is not ready yet (this shouldn't happen).
func (pod *functionPodInfo) SendResponse(responseCh chan<- *connectionResponse, err error) {
	switch {
	case err != nil:
		responseCh <- &connectionResponse{
			err: err,
		}
	case pod.podData == nil:
		responseCh <- &connectionResponse{
			err: fmt.Errorf("pod is not ready, connection response sent prematurely. This is logical error in the code"),
		}
	default:
		responseCh <- &connectionResponse{
			podData:               *pod.podData,
			fnEvaluationMutex:     pod.fnEvaluationMutex,
			concurrentEvaluations: pod.concurrentEvaluations,
			err:                   nil,
		}
	}
}

// WaitlistLen returns with the number of fn evaluations currently handled by the pod
func (pod functionPodInfo) WaitlistLen() int {
	return int(pod.concurrentEvaluations.Load())
}
