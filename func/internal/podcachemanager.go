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
	"net"
	"os"
	"strconv"
	"sync"
	"time"

	"google.golang.org/grpc"
	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

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
	cache map[string]*podAndGRPCClient
	// waitlists is a mapping from image name to a list of channels that are
	// waiting for the GRPC client connections.
	waitlists map[string][]chan<- *clientConnAndError

	podManager *podManager
}

type clientConnRequest struct {
	image string

	// grpcConn is a channel that a grpc client should be sent back.
	grpcClientCh chan<- *clientConnAndError
}

type clientConnAndError struct {
	podClient *podAndGRPCClient
	err       error
}

type podAndGRPCClient struct {
	grpcClient *grpc.ClientConn
	pod        client.ObjectKey
	mu         sync.Mutex
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
							req.grpcClientCh <- &clientConnAndError{podClient: podAndCl}
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
			if resp.err != nil {
				klog.Warningf("received error from the pod manager: %v", resp.err)
			} else {
				pcm.cache[resp.image] = resp.podAndGRPCClient
			}
			// notify all the goroutines that are waiting for the GRPC client.
			channels := pcm.waitlists[resp.image]
			delete(pcm.waitlists, resp.image)
			for i := range channels {
				cce := &clientConnAndError{
					podClient: resp.podAndGRPCClient,
					err:       resp.err,
				}
				channels[i] <- cce
			}
		case <-tick:
			// synchronous GC
			pcm.garbageCollector()
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

	if cached, ok := pcm.cache[image]; ok && cached.pod.Name == pod.Name {
		delete(pcm.cache, image)
		klog.Infof("Evicted failed pod %v/%v from cache", pod.Namespace, pod.Name)
	}

	return true
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
			}
		}
	}
}

func deletePodWithContext(kubeClient client.Client, pod *corev1.Pod) error {
	return kubeClient.Delete(context.Background(), pod)
}

func isCurrentCachedPod(pcm *podCacheManager, image string, pod *corev1.Pod) bool {
	cached, ok := pcm.cache[image]
	return ok && cached.pod.Name == pod.Name
}

// deleteFailedPod deletes a failed pod and removes it from cache if it is still the current pod.
func (pcm *podCacheManager) deleteFailedPod(pod *corev1.Pod, image string) {
	err := deletePodWithContext(pcm.podManager.kubeClient, pod)
	if err != nil {
		klog.Warningf("Failed to delete failed pod %v/%v: %v", pod.Namespace, pod.Name, err)
		return
	}

	if isCurrentCachedPod(pcm, image, pod) {
		delete(pcm.cache, image)
		klog.Infof("Evicted pod from cache for image %v", image)
	}
}
