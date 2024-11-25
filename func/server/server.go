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

package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	pb "github.com/nephio-project/porch/func/evaluator"
	"github.com/nephio-project/porch/func/healthchecker"
	"github.com/nephio-project/porch/func/internal"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health/grpc_health_v1"
	"k8s.io/klog/v2"
)

const (
	execRuntime = "exec"
	podRuntime  = "pod"

	wrapperServerImageEnv = "WRAPPER_SERVER_IMAGE"
)

type options struct {
	// The server port
	port int
	// The runtime(s) to disable. Multiple runtimes should separated by `,`.
	disableRuntimes string

	// Parameters of ExecEvaluator
	exec internal.ExecutableEvaluatorOptions
	// Parameters of PodEvaluator
	pod internal.PodEvaluatorOptions
}

func main() {
	o := &options{}
	// generic flags
	flag.IntVar(&o.port, "port", 9445, "The server port")
	flag.StringVar(&o.disableRuntimes, "disable-runtimes", "", fmt.Sprintf("The runtime(s) to disable. Multiple runtimes should separated by `,`. Available runtimes: `%v`, `%v`.", execRuntime, podRuntime))
	// flags for the exec runtime
	flag.StringVar(&o.exec.FunctionCacheDir, "functions", "./functions", "Path to cached functions.")
	flag.StringVar(&o.exec.ConfigFileName, "config", "./config.yaml", "Path to the config file of the exec runtime.")
	// flags for the pod runtime
	flag.StringVar(&o.pod.PodCacheConfigFileName, "pod-cache-config", "/pod-cache-config/pod-cache-config.yaml", "Path to the pod cache config file. The file is map of function name to TTL.")
	flag.StringVar(&o.pod.PodNamespace, "pod-namespace", "porch-fn-system", "Namespace to run KRM functions pods.")
	flag.DurationVar(&o.pod.PodTTL, "pod-ttl", 30*time.Minute, "TTL for pods before GC.")
	flag.DurationVar(&o.pod.GcScanInterval, "scan-interval", time.Minute, "The interval of GC between scans.")
	flag.StringVar(&o.pod.FunctionPodTemplateName, "function-pod-template", "", "Configmap that contains a pod specification")
	flag.BoolVar(&o.pod.EnablePrivateRegistries, "enable-private-registries", false, "if true enables the use of private registries and their authentication")
	flag.StringVar(&o.pod.RegistryAuthSecretPath, "registry-auth-secret-path", "/var/tmp/config-secret/.dockerconfigjson", "The path of the secret used for authenticating to custom registries")
	flag.StringVar(&o.pod.RegistryAuthSecretName, "registry-auth-secret-name", "auth-secret", "The name of the secret used for authenticating to custom registries")
	flag.BoolVar(&o.pod.EnablePrivateRegistriesTls, "enable-private-registries-tls", false, "if enabled, will prioritize use of user provided TLS secret when accessing registries")
	flag.StringVar(&o.pod.TlsSecretPath, "tls-secret-path", "/var/tmp/tls-secret/", "The path of the secret used in tls configuration")
	flag.IntVar(&o.pod.MaxGrpcMessageSize, "max-request-body-size", 6*1024*1024, "Maximum size of grpc messages in bytes. Keep this in sync with porch-server's corresponding argument.")

	flag.Parse()

	if err := run(o); err != nil {
		fmt.Fprintf(os.Stderr, "unexpected error: %v\n", err)
		os.Exit(1)
	}
}

func run(o *options) error {
	address := fmt.Sprintf(":%d", o.port)
	lis, err := net.Listen("tcp", address)
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}

	availableRuntimes := map[string]struct{}{
		execRuntime: {},
		podRuntime:  {},
	}
	if o.disableRuntimes != "" {
		runtimesFromFlag := strings.Split(o.disableRuntimes, ",")
		for _, rt := range runtimesFromFlag {
			delete(availableRuntimes, rt)
		}
	}
	runtimes := []internal.Evaluator{}
	for rt := range availableRuntimes {
		switch rt {
		case execRuntime:
			execEval, err := internal.NewExecutableEvaluator(o.exec)
			if err != nil {
				return fmt.Errorf("failed to initialize executable evaluator: %w", err)
			}
			runtimes = append(runtimes, execEval)
		case podRuntime:
			o.pod.WrapperServerImage = os.Getenv(wrapperServerImageEnv)
			if o.pod.WrapperServerImage == "" {
				return fmt.Errorf("environment variable %v must be set to use pod function evaluator runtime", wrapperServerImageEnv)
			}
			podEval, err := internal.NewPodEvaluator(o.pod)
			if err != nil {
				return fmt.Errorf("failed to initialize pod evaluator: %w", err)
			}
			runtimes = append(runtimes, podEval)
		}
	}
	if len(runtimes) == 0 {
		klog.Warning("no runtime is enabled in function-runner")
	}
	evaluator := internal.NewMultiEvaluator(runtimes...)

	klog.Infof("Listening on %s", address)

	// Start the gRPC server
	server := grpc.NewServer(
		grpc.MaxRecvMsgSize(o.pod.MaxGrpcMessageSize),
		grpc.MaxSendMsgSize(o.pod.MaxGrpcMessageSize),
	)
	pb.RegisterFunctionEvaluatorServer(server, evaluator)
	healthService := healthchecker.NewHealthChecker()
	grpc_health_v1.RegisterHealthServer(server, healthService)
	if err := server.Serve(lis); err != nil {
		return fmt.Errorf("server failed: %w", err)
	}
	return nil
}
