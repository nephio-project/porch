// Copyright 2022, 2026 The kpt and Nephio Authors
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

package suiteutils

import (
	"context"
	"os"

	porchclient "github.com/nephio-project/porch/api/generated/clientset/versioned"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

type MultiClusterTestSuite struct {
	TestSuite
	scheme       *runtime.Scheme
	clients      map[string]client.Client
	porchclients map[string]*porchclient.Clientset
	readers      map[string]client.Client
	kubeClients  map[string]kubernetes.Interface
	PorchRoot    string
}

func (t *MultiClusterTestSuite) SetupSuite() {
	t.ctx = context.Background()

	t.clients = make(map[string]client.Client)
	t.porchclients = make(map[string]*porchclient.Clientset)
	t.readers = make(map[string]client.Client)
	t.kubeClients = make(map[string]kubernetes.Interface)

	t.scheme = createClientScheme(t.T())
}

func (t *MultiClusterTestSuite) UseKubeconfigFile(kubeconfigPath string) {
	t.T().Helper()
	os.Setenv(clientcmd.RecommendedConfigPathEnvVar, t.PorchRoot+kubeconfigPath)
	cfg, err := config.GetConfig()
	if err != nil {
		t.Fatalf("Unable to switch clusters - error loading Kubernetes client config from file %q: %v", kubeconfigPath, err)
	}

	if cachedClient, found := t.clients[kubeconfigPath]; found {
		t.Client = cachedClient
	} else {
		if newClient, err := client.New(cfg, client.Options{
			Scheme: t.scheme,
		}); err == nil {
			t.clients[kubeconfigPath] = newClient
			t.Client = newClient
		} else {
			t.Fatalf("Failed to initialize k8s client (%s): %v", cfg.Host, err)
		}
	}

	if cachedPorchClient, found := t.porchclients[kubeconfigPath]; found {
		t.Clientset = cachedPorchClient
	} else {
		if newClient, err := porchclient.NewForConfig(cfg); err != nil {
			t.Fatalf("Failed to initialize Porch client (%s): %v", cfg.Host, err)
		} else {
			t.porchclients[kubeconfigPath] = newClient
			t.Clientset = newClient
		}
	}
	if cs, err := porchclient.NewForConfig(cfg); err != nil {
		t.Fatalf("Failed to initialize Porch clientset: %v", err)
	} else {
		t.Clientset = cs
	}

	// Direct API reader to avoid stale cache reads
	if cachedReader, found := t.readers[kubeconfigPath]; found {
		t.Reader = cachedReader
	} else {
		if newReader, err := client.New(cfg, client.Options{
			Scheme: t.scheme,
			Cache: &client.CacheOptions{
				DisableFor: []client.Object{},
			},
		}); err != nil {
			t.Fatalf("Failed to initialize direct API reader (%s): %v", cfg.Host, err)
		} else {
			t.readers[kubeconfigPath] = newReader
			t.Reader = newReader
		}
	}

	if cachedKubeClient, found := t.kubeClients[kubeconfigPath]; found {
		t.KubeClient = cachedKubeClient
	} else {
		if newKubeClient, err := kubernetes.NewForConfig(cfg); err != nil {
			t.Fatalf("failed to initialize kubernetes clientset: %v", err)
		} else {
			t.kubeClients[kubeconfigPath] = newKubeClient
			t.KubeClient = newKubeClient
		}
	}

	t.Logf("Now using kubeconfig file %q", kubeconfigPath)
}

func (t *MultiClusterTestSuite) DropCachedClients(kubeconfigPath string) {
	delete(t.clients, kubeconfigPath)
	delete(t.porchclients, kubeconfigPath)
	delete(t.readers, kubeconfigPath)
	delete(t.kubeClients, kubeconfigPath)
}
