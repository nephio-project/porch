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

	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/klog/v2"
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
	functions map[string]*functionInfo

	podManager *podManager

	maxWaitlistLength          int
	maxParallelPodsPerFunction int
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

func (pcm *podCacheManager) redistributeLoad(image string, fn *functionInfo, connections []chan<- *connectionResponse) {
	pcm.removeUnhealthyPods(fn, false)
	for _, ch := range connections {
		bestPodIndex, _ := pcm.findBestPod(fn)
		pod := pcm.functions[image].pods[bestPodIndex]
		if pod.podData != nil {
			pod.SendResponse(ch, nil)
		} else {
			pod.waitlist = append(pod.waitlist, ch)
		}
	}
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
			//if pcm.podManager.imageResolver != nil {
			//	req.image = pcm.podManager.imageResolver(req.image)
			//}
			fn := pcm.FunctionInfo(req.image)

			shouldScaleUp := false
			pcm.removeUnhealthyPods(fn, false)
			bestPodIndex, bestWaitlistLen := pcm.findBestPod(fn)
			if bestPodIndex == -1 {
				shouldScaleUp = true
			} else {
				if bestWaitlistLen >= pcm.maxWaitlistLength && len(fn.pods) < pcm.maxParallelPodsPerFunction {
					shouldScaleUp = true
				}
			}

			if shouldScaleUp {
				klog.Infof("Scaling up for image %s. No idle pods available. Starting a new pod.", req.image)

				fn.pods = append(fn.pods, NewPodInfo(req.responseCh))
				go pcm.podManager.getFuncEvalPodClient(context.Background(), req.image, pcm.podTTL)
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
				if len(fn.pods) > 0 {
					fn.pods = slices.Delete(fn.pods, toUpdate, toUpdate+1)
					pcm.redistributeLoad(podReadyMsg.image, fn, waitListToRedistribute)
				} else {
					for _, ch := range waitListToRedistribute {
						fn.pods[toUpdate].SendResponse(ch, podReadyMsg.err)
					}
					fn.pods = slices.Delete(fn.pods, toUpdate, toUpdate+1)
				}
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

func (pcm *podCacheManager) FunctionInfo(image string) *functionInfo {
	fn, ok := pcm.functions[image]
	if !ok {
		fn = &functionInfo{}
		pcm.functions[image] = fn
	}
	return fn
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
	var podsTTL map[string]string
	err = yaml.Unmarshal(content, &podsTTL)
	if err != nil {
		return err
	}

	for fnImage, ttlStr := range podsTTL {
		klog.Infof("preloading pod cache for function %v with TTL %v", fnImage, ttlStr)

		ttl, err := time.ParseDuration(ttlStr)
		if err != nil {
			klog.Warningf("unable to parse TTL duration (%s) from the config file for function %q. Using default (%s) instead: %v", ttlStr, fnImage, pcm.podTTL, err)
			ttl = pcm.podTTL
		}

		fn := pcm.FunctionInfo(fnImage)
		if len(fn.pods) == 0 {
			fn.pods = append(fn.pods, NewPodInfo(nil))
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
				defer cancel()
				pcm.podManager.getFuncEvalPodClient(ctx, fnImage, ttl)
			}()
		}
	}
	return nil
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
		if pod.podData == nil {
			// pod is under creation
			return false
		}

		k8sPod := &corev1.Pod{}
		err := pcm.podManager.kubeClient.Get(context.Background(), pod.podData.podKey, k8sPod)
		if err != nil {
			if apierrors.IsNotFound(err) {
				klog.Infof("Removing deleted pod from cache for image %v", pod.podData.image)
			} else {
				klog.Errorf("Failed to get pod %v, removing from cache: %v", pod.podData.podKey, err)
			}
			return true
		}

		if !k8sPod.DeletionTimestamp.IsZero() {
			// Pod is under deletion
			return true
		}

		if k8sPod.Status.Phase == corev1.PodFailed {
			klog.Errorf("Evicting pod in failed state (%v/%v) from cache for image %v", k8sPod.Namespace, k8sPod.Name, pod.podData.image)
			pcm.DeletePodInBackground(k8sPod)
			return true
		}

		if net.JoinHostPort(k8sPod.Status.PodIP, defaultWrapperServerPort) != pod.podData.grpcConnection.Target() {
			klog.Errorf("Evicting pod whose pod IP doesn't match with its grpc connection (%v/%v) from cache for image %v", k8sPod.Namespace, k8sPod.Name, pod.podData.image)
			pcm.DeletePodInBackground(k8sPod)
			return true
		}
		if removeIdle && pod.WaitlistLen() == 0 && time.Since(pod.lastActivity) > pcm.podTTL {
			klog.Infof("Removing idle pod %q that reached its TTL from cache for image %v", k8sPod.Name, pod.podData.image)
			pcm.DeletePodInBackground(k8sPod)
			return true
		}

		return false
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

func (pcm *podCacheManager) DeletePodInBackground(k8sPod *corev1.Pod) {
	go func() {
		err := pcm.podManager.kubeClient.Delete(context.Background(), k8sPod)
		if err != nil {
			klog.Errorf("Failed to delete pod %v/%v from cluster: %v", k8sPod.Namespace, k8sPod.Name, err)
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
			err: fmt.Errorf("Pod is not ready, connection response sent prematurely. This is logical error in the code."),
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
