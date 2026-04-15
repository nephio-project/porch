// Copyright 2026 The kpt and Nephio Authors
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
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	kptfilev1 "github.com/kptdev/kpt/pkg/api/kptfile/v1"
	"github.com/kptdev/kpt/pkg/fn"
	pb "github.com/nephio-project/porch/func/evaluator"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"k8s.io/klog/v2"
	ptr "k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	testImageName = "test-image"
)

func TestPodEvaluatorExecution(t *testing.T) {
	t.Run("invalid semver constraint syntax", func(t *testing.T) {
		const sleep = 2 * time.Second

		ctx := t.Context()

		addr, err := startFakeServer(ctx, t, sleep, nil)
		require.NoError(t, err, "failed to start fake server")

		conn, err := grpc.NewClient(
			addr,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithNoProxy(),
		)
		require.NoError(t, err, "grpc dial failed")

		reqCh := make(chan *connectionRequest, 2)
		go func() {
			counter := &atomic.Int32{}
			for req := range reqCh {
				req.responseCh <- &connectionResponse{
					podData: podData{
						image:          req.image,
						grpcConnection: conn,
						podKey:         ptr.To(client.ObjectKey{}),
					},
					concurrentEvaluations: counter,
					err:                   nil,
				}
			}
		}()

		pcm := &podCacheManager{podManager: &podManager{}}

		pe := &podEvaluator{
			requestCh:       reqCh,
			podCacheManager: pcm,
		}

		req := &pb.EvaluateFunctionRequest{
			ResourceList: []byte(`{}`),
			Image:        defaultKRMImagePrefix + testImageName,
			// Invalid semver constraint, '>>' is not a valid operator
			// -> will cause a function not found error
			Tag: ">> 0.4.0 < 0.5.0",
		}
		_, err = pe.EvaluateFunction(ctx, req)
		assert.Contains(t, err.Error(), fmt.Sprintf("tag %q is not a valid semver constraint", req.Tag))
	})
	t.Run("failed to list tags for the image", func(t *testing.T) {
		// Override DOCKER_CONFIG in order to read a minimal,
		// empty config instead of the host's ~/.docker/config.json.
		dockerCfgDir := t.TempDir()
		// Creating an empty config.json (without the credsStore field) so that
		// Resolve() succeeds with anonymous auth. The subsequent remote.List()
		// call then fails because the registry rejects unauthenticated access.
		err := os.WriteFile(filepath.Join(dockerCfgDir, "config.json"), []byte(`{}`), 0600)
		require.NoError(t, err, "failed to write temp docker config")
		t.Setenv("DOCKER_CONFIG", dockerCfgDir)
		defer os.Unsetenv("DOCKER_CONFIG")

		const sleep = 2 * time.Second

		ctx := t.Context()

		addr, err := startFakeServer(ctx, t, sleep, nil)
		require.NoError(t, err, "failed to start fake server")

		conn, err := grpc.NewClient(
			addr,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithNoProxy(),
		)
		require.NoError(t, err, "grpc dial failed")

		reqCh := make(chan *connectionRequest, 2)
		go func() {
			counter := &atomic.Int32{}
			for req := range reqCh {
				req.responseCh <- &connectionResponse{
					podData: podData{
						image:          req.image,
						grpcConnection: conn,
						podKey:         ptr.To(client.ObjectKey{}),
					},
					concurrentEvaluations: counter,
					err:                   nil,
				}
			}
		}()

		pcm := &podCacheManager{
			// listRepositoryTagsFunc is nil in podManager, so the real listRepositoryTags path runs.
			podManager: &podManager{},
		}

		pe := &podEvaluator{
			requestCh:       reqCh,
			podCacheManager: pcm,
		}

		req := &pb.EvaluateFunctionRequest{
			ResourceList: []byte(`{}`),
			Image:        defaultKRMImagePrefix + testImageName,
			// The semver constraint is valid
			Tag: ">= 0.4.0 < 0.5.0",
		}
		_, err = pe.EvaluateFunction(ctx, req)
		assert.Contains(t, err.Error(), fmt.Sprintf("failed to list tags for image %q", req.Image))
	})
	t.Run("function does not match the semantic version constraints", func(t *testing.T) {
		const sleep = 2 * time.Second

		ctx := t.Context()

		addr, err := startFakeServer(ctx, t, sleep, nil)
		require.NoError(t, err, "failed to start fake server")

		conn, err := grpc.NewClient(
			addr,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithNoProxy(),
		)
		require.NoError(t, err, "grpc dial failed")

		reqCh := make(chan *connectionRequest, 2)
		go func() {
			counter := &atomic.Int32{}
			for req := range reqCh {
				req.responseCh <- &connectionResponse{
					podData: podData{
						image:          req.image,
						grpcConnection: conn,
						podKey:         ptr.To(client.ObjectKey{}),
					},
					concurrentEvaluations: counter,
					err:                   nil,
				}
			}
		}()

		pcm := &podCacheManager{
			podManager: &podManager{
				listRepositoryTagsFunc: func(ctx context.Context, image string) ([]string, error) {
					return []string{"v0.3.0", "v0.4.0", "v0.4.1", "v0.4.2", "v0.5.0"}, nil
				},
			},
		}

		pe := &podEvaluator{
			requestCh:       reqCh,
			podCacheManager: pcm,
		}

		req := &pb.EvaluateFunctionRequest{
			ResourceList: []byte(`{}`),
			Image:        defaultKRMImagePrefix + testImageName,
			// This constraint won't match any of the available tags returned by
			// listRepositoryTagsFunc -> will cause a function not found error
			Tag: "> 0.5.0 < 0.6.0",
		}
		_, err = pe.EvaluateFunction(ctx, req)
		assert.Equal(t, &fn.NotFoundError{
			Function: kptfilev1.Function{Image: req.Image},
		}, err)
	})
	t.Run("function execution error", func(t *testing.T) {
		const sleep = 0 * time.Second

		ctx := t.Context()

		evalErr := fmt.Errorf("simulated evaluation failure")
		addr, err := startFakeServer(ctx, t, sleep, evalErr)
		require.NoError(t, err, "failed to start fake server")

		conn, err := grpc.NewClient(
			addr,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithNoProxy(),
		)
		require.NoError(t, err, "grpc dial failed")

		reqCh := make(chan *connectionRequest, 2)
		go func() {
			counter := &atomic.Int32{}
			for req := range reqCh {
				req.responseCh <- &connectionResponse{
					podData: podData{
						image:          req.image,
						grpcConnection: conn,
						podKey:         ptr.To(client.ObjectKey{}),
					},
					concurrentEvaluations: counter,
					err:                   nil,
				}
			}
		}()

		pcm := &podCacheManager{
			podManager: &podManager{
				listRepositoryTagsFunc: func(ctx context.Context, image string) ([]string, error) {
					return []string{"v0.3.0", "v0.4.0", "v0.4.1", "v0.4.2", "v0.5.0"}, nil
				},
			},
		}

		pe := &podEvaluator{
			requestCh:       reqCh,
			podCacheManager: pcm,
		}

		req := &pb.EvaluateFunctionRequest{
			ResourceList: []byte(`{}`),
			Image:        defaultKRMImagePrefix + testImageName,
			Tag:          ">= 0.4.0 < 0.5.0",
		}
		_, err = pe.EvaluateFunction(ctx, req)
		assert.Contains(t, err.Error(), fmt.Sprintf("unable to evaluate %q with pod evaluator", defaultKRMImagePrefix+testImageName+":v0.4.2"))
	})
	t.Run("successful execution with semantic versioning", func(t *testing.T) {
		const sleep = 0 * time.Second

		ctx := t.Context()

		addr, err := startFakeServer(ctx, t, sleep, nil)
		require.NoError(t, err, "failed to start fake server")

		conn, err := grpc.NewClient(
			addr,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithNoProxy(),
		)
		require.NoError(t, err, "grpc dial failed")

		reqCh := make(chan *connectionRequest, 2)
		go func() {
			counter := &atomic.Int32{}
			for req := range reqCh {
				req.responseCh <- &connectionResponse{
					podData: podData{
						image:          req.image,
						grpcConnection: conn,
						podKey:         ptr.To(client.ObjectKey{}),
					},
					concurrentEvaluations: counter,
					err:                   nil,
				}
			}
		}()

		pcm := &podCacheManager{
			podManager: &podManager{
				listRepositoryTagsFunc: func(ctx context.Context, image string) ([]string, error) {
					return []string{"v0.3.0", "v0.4.0", "v0.4.1", "v0.4.2", "v0.5.0"}, nil
				},
			},
		}

		pe := &podEvaluator{
			requestCh:       reqCh,
			podCacheManager: pcm,
		}

		req := &pb.EvaluateFunctionRequest{
			ResourceList: []byte(`{}`),
			Image:        defaultKRMImagePrefix + testImageName,
			Tag:          ">= 0.4.0 < 0.5.0",
		}

		// Capture klog output by redirecting stderr
		oldStderr := os.Stderr
		r, w, _ := os.Pipe()
		os.Stderr = w

		resp, err := pe.EvaluateFunction(ctx, req)

		assert.NotNil(t, resp)
		assert.NoError(t, err)

		// Flush klog and restore stderr
		klog.Flush()
		w.Close()
		os.Stderr = oldStderr

		// Read captured output
		var logBuffer bytes.Buffer
		logBuffer.ReadFrom(r)
		logOutput := logBuffer.String()

		// Verify the klog message contains the expected version selection
		assert.Contains(t, logOutput, fmt.Sprintf(`Resolved image tag: %q (constraint "v0.4.2")`, defaultKRMImagePrefix+testImageName+":v0.4.2"))
		assert.Contains(t, logOutput, fmt.Sprintf(`evaluating %q succeeded`, defaultKRMImagePrefix+testImageName+":v0.4.2"))
	})
	t.Run("successful execution with explicit tagging", func(t *testing.T) {
		const sleep = 0 * time.Second

		ctx := t.Context()

		addr, err := startFakeServer(ctx, t, sleep, nil)
		require.NoError(t, err, "failed to start fake server")

		conn, err := grpc.NewClient(
			addr,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithNoProxy(),
		)
		require.NoError(t, err, "grpc dial failed")

		reqCh := make(chan *connectionRequest, 2)
		go func() {
			counter := &atomic.Int32{}
			for req := range reqCh {
				req.responseCh <- &connectionResponse{
					podData: podData{
						image:          req.image,
						grpcConnection: conn,
						podKey:         ptr.To(client.ObjectKey{}),
					},
					concurrentEvaluations: counter,
					err:                   nil,
				}
			}
		}()

		pcm := &podCacheManager{podManager: &podManager{}}

		pe := &podEvaluator{
			requestCh:       reqCh,
			podCacheManager: pcm,
		}

		req := &pb.EvaluateFunctionRequest{
			ResourceList: []byte(`{}`),
			Image:        defaultKRMImagePrefix + testImageName + ":v0.4.5",
		}

		// Capture klog output by redirecting stderr
		oldStderr := os.Stderr
		r, w, _ := os.Pipe()
		os.Stderr = w

		resp, err := pe.EvaluateFunction(ctx, req)

		assert.NotNil(t, resp)
		assert.NoError(t, err)

		// Flush klog and restore stderr
		klog.Flush()
		w.Close()
		os.Stderr = oldStderr

		// Read captured output
		var logBuffer bytes.Buffer
		logBuffer.ReadFrom(r)
		logOutput := logBuffer.String()

		// Verify the klog message contains the expected version selection
		assert.Contains(t, logOutput, fmt.Sprintf(`Image tag is empty, using the image with explicit tag: %q`, defaultKRMImagePrefix+testImageName+":v0.4.5"))
		assert.Contains(t, logOutput, fmt.Sprintf(`evaluating %q succeeded`, defaultKRMImagePrefix+testImageName+":v0.4.5"))
	})
	t.Run("successful execution with image tag and Tag field set", func(t *testing.T) {
		const sleep = 0 * time.Second

		ctx := t.Context()

		addr, err := startFakeServer(ctx, t, sleep, nil)
		require.NoError(t, err, "failed to start fake server")

		conn, err := grpc.NewClient(
			addr,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithNoProxy(),
		)
		require.NoError(t, err, "grpc dial failed")

		reqCh := make(chan *connectionRequest, 2)
		go func() {
			counter := &atomic.Int32{}
			for req := range reqCh {
				req.responseCh <- &connectionResponse{
					podData: podData{
						image:          req.image,
						grpcConnection: conn,
						podKey:         ptr.To(client.ObjectKey{}),
					},
					concurrentEvaluations: counter,
					err:                   nil,
				}
			}
		}()

		pcm := &podCacheManager{
			podManager: &podManager{
				listRepositoryTagsFunc: func(ctx context.Context, image string) ([]string, error) {
					return []string{"v0.3.0", "v0.4.0", "v0.4.1", "v0.4.2", "v0.5.0"}, nil
				},
			},
		}

		pe := &podEvaluator{
			requestCh:       reqCh,
			podCacheManager: pcm,
		}

		req := &pb.EvaluateFunctionRequest{
			ResourceList: []byte(`{}`),
			// Both the Image and Tag fields are set. Since , the Tag field
			// is not empty, it will take precedence, and the image name tag will be ignored,
			// and will be replaced by the resolved tag based on the semver constraint in the Tag field.
			Image: defaultKRMImagePrefix + testImageName + ":v0.3.1",
			Tag:   ">= 0.4.0 < 0.5.0",
		}

		// Capture klog output by redirecting stderr
		oldStderr := os.Stderr
		r, w, _ := os.Pipe()
		os.Stderr = w

		resp, err := pe.EvaluateFunction(ctx, req)

		assert.NotNil(t, resp)
		assert.NoError(t, err)

		// Flush klog and restore stderr
		klog.Flush()
		w.Close()
		os.Stderr = oldStderr

		// Read captured output
		var logBuffer bytes.Buffer
		logBuffer.ReadFrom(r)
		logOutput := logBuffer.String()

		// Verify the klog message contains the expected version selection
		assert.Contains(t, logOutput, fmt.Sprintf(`Resolved image tag: %q (constraint "v0.4.2")`, defaultKRMImagePrefix+testImageName+":v0.4.2"))
		assert.Contains(t, logOutput, fmt.Sprintf(`evaluating %q succeeded`, defaultKRMImagePrefix+testImageName+":v0.4.2"))
	})
}
