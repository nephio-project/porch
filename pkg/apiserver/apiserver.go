// Copyright 2022, 2024-2025 The kpt and Nephio Authors
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

package apiserver

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/kptdev/kpt/pkg/lib/runneroptions"
	"github.com/nephio-project/porch/api/porch/install"
	porchapi "github.com/nephio-project/porch/api/porch/v1alpha1"
	configapi "github.com/nephio-project/porch/controllers/repositories/api/v1alpha1"
	repocontroller "github.com/nephio-project/porch/controllers/repositories/pkg/controllers/repository"
	internalapi "github.com/nephio-project/porch/internal/api/porchinternal/v1alpha1"
	"github.com/nephio-project/porch/pkg/cache"
	cachetypes "github.com/nephio-project/porch/pkg/cache/types"
	"github.com/nephio-project/porch/pkg/engine"
	"github.com/nephio-project/porch/pkg/registry/porch"
	"google.golang.org/api/option"
	"google.golang.org/api/sts/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	genericapiserver "k8s.io/apiserver/pkg/server"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/component-base/compatibility"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	// Scheme defines methods for serializing and deserializing API objects.
	Scheme = runtime.NewScheme()
	// Codecs provides methods for retrieving codecs and serializers for specific
	// versions and content types.
	Codecs = serializer.NewCodecFactory(Scheme)
	// completeScheme is a singleton for the complete scheme with all types
	completeScheme *runtime.Scheme
	schemeOnce     sync.Once
)

func init() {
	install.Install(Scheme)

	// we need to add the options to empty v1
	// TODO fix the server code to avoid this
	metav1.AddToGroupVersion(Scheme, schema.GroupVersion{Version: "v1"})

	// TODO: keep the generic API server from wanting this
	unversioned := schema.GroupVersion{Group: "", Version: "v1"}
	Scheme.AddUnversionedTypes(unversioned,
		&metav1.Status{},
		&metav1.APIVersions{},
		&metav1.APIGroupList{},
		&metav1.APIGroup{},
		&metav1.APIResourceList{},
	)
}

// RepoControllerConfig holds configuration for the embedded Repository controller
type RepoControllerConfig struct {
	MaxConcurrentReconciles int
	MaxConcurrentSyncs      int
	HealthCheckFrequency    time.Duration
	FullSyncFrequency       time.Duration
}

// ExtraConfig holds custom apiserver config
type ExtraConfig struct {
	CoreAPIKubeconfigPath    string
	GRPCRuntimeOptions       engine.GRPCRuntimeOptions
	CacheOptions             cachetypes.CacheOptions
	RepoControllerConfig     RepoControllerConfig
	ListTimeoutPerRepository time.Duration
	MaxConcurrentLists       int
	UseLegacySync            bool // Use legacy background sync (true) vs controller-based sync (false)
}

// Config defines the config for the apiserver
type Config struct {
	GenericConfig *genericapiserver.RecommendedConfig
	ExtraConfig   ExtraConfig
}

// PorchServer contains state for a Kubernetes cluster master/api server.
type PorchServer struct {
	GenericAPIServer           *genericapiserver.GenericAPIServer
	coreClient                 client.WithWatch
	cache                      cachetypes.Cache
	repoCacheSyncFrequency     time.Duration
	ListTimeoutPerRepository   time.Duration
	repoOperationRetryAttempts int
	ExtraConfig                *ExtraConfig
	embeddedController         *EmbeddedControllerManager
}

type completedConfig struct {
	GenericConfig genericapiserver.CompletedConfig
	ExtraConfig   *ExtraConfig
}

// CompletedConfig embeds a private pointer that cannot be instantiated outside of this package.
type CompletedConfig struct {
	*completedConfig
}

// Complete fills in any fields not set that are required to have valid data. It's mutating the receiver.
func (cfg *Config) Complete() CompletedConfig {
	cfg.GenericConfig.EffectiveVersion = compatibility.NewEffectiveVersionFromString("1.0", "1.0", "1.0")

	c := completedConfig{
		cfg.GenericConfig.Complete(),
		&cfg.ExtraConfig,
	}

	return CompletedConfig{&c}
}

// buildCompleteScheme returns a singleton runtime scheme with all necessary types registered
func buildCompleteScheme() (*runtime.Scheme, error) {
	var err error
	schemeOnce.Do(func() {
		scheme := runtime.NewScheme()
		if e := configapi.AddToScheme(scheme); e != nil {
			err = fmt.Errorf("error adding configapi to scheme: %w", e)
			return
		}
		if e := porchapi.AddToScheme(scheme); e != nil {
			err = fmt.Errorf("error adding porchapi to scheme: %w", e)
			return
		}
		if e := corev1.AddToScheme(scheme); e != nil {
			err = fmt.Errorf("error adding corev1 to scheme: %w", e)
			return
		}
		if e := internalapi.AddToScheme(scheme); e != nil {
			err = fmt.Errorf("error adding internalapi to scheme: %w", e)
			return
		}
		completeScheme = scheme
	})
	return completeScheme, err
}

func (c completedConfig) getRestConfig() (*rest.Config, error) {
	kubeconfig := c.ExtraConfig.CoreAPIKubeconfigPath
	if kubeconfig == "" {
		icc, err := rest.InClusterConfig()
		if err != nil {
			return nil, fmt.Errorf("failed to load in-cluster config (specify --kubeconfig if not running in-cluster): %w", err)
		}
		return icc, nil
	} else {
		loadingRules := &clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfig}
		loader := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, &clientcmd.ConfigOverrides{})

		cc, err := loader.ClientConfig()
		if err != nil {
			return nil, fmt.Errorf("failed to load config %q: %w", kubeconfig, err)
		}
		return cc, nil
	}
}

func (c completedConfig) buildClient() (client.WithWatch, error) {
	restConfig, err := c.getRestConfig()
	if err != nil {
		return nil, err
	}

	// set high qps/burst limits since this will effectively limit API server responsiveness
	restConfig.QPS = 200
	restConfig.Burst = 400

	scheme, err := buildCompleteScheme()
	if err != nil {
		return nil, err
	}

	return client.NewWithWatch(restConfig, client.Options{Scheme: scheme})
}

func (c completedConfig) getCoreV1Client() (*corev1client.CoreV1Client, error) {
	restConfig, err := c.getRestConfig()
	if err != nil {
		return nil, err
	}

	corev1Client, err := corev1client.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("error building corev1 client: %w", err)
	}
	return corev1Client, nil
}

// createEmbeddedController creates embedded controller manager
func (c completedConfig) createEmbeddedController(coreClient client.WithWatch) (*EmbeddedControllerManager, error) {
	restConfig, err := c.getRestConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get rest config: %w", err)
	}

	scheme, err := buildCompleteScheme()
	if err != nil {
		return nil, fmt.Errorf("failed to build scheme: %w", err)
	}

	config := repocontroller.EmbeddedConfig{
		MaxConcurrentReconciles:    c.ExtraConfig.RepoControllerConfig.MaxConcurrentReconciles,
		MaxConcurrentSyncs:         c.ExtraConfig.RepoControllerConfig.MaxConcurrentSyncs,
		HealthCheckFrequency:       c.ExtraConfig.RepoControllerConfig.HealthCheckFrequency,
		FullSyncFrequency:          c.ExtraConfig.RepoControllerConfig.FullSyncFrequency,
		RepoOperationRetryAttempts: c.ExtraConfig.CacheOptions.RepoOperationRetryAttempts,
	}

	return createEmbeddedController(coreClient, restConfig, scheme, config)
}

// New returns a new instance of PorchServer from the given config.
func (c completedConfig) New(ctx context.Context) (*PorchServer, error) {
	// TODO: REMOVE AFTER ASYNC IMPLEMENTATION IS READY.
	// Set the default request timeout just above hardcoded ctx timeout
	c.GenericConfig.RequestTimeout = 291 * time.Second
	genericServer, err := c.GenericConfig.New("porch-apiserver", genericapiserver.NewEmptyDelegate())
	if err != nil {
		return nil, err
	}

	coreClient, err := c.buildClient()
	if err != nil {
		return nil, fmt.Errorf("failed to build client for core apiserver: %w", err)
	}

	coreV1Client, err := c.getCoreV1Client()
	if err != nil {
		return nil, err
	}

	stsClient, err := sts.NewService(context.Background(), option.WithoutAuthentication())
	if err != nil {
		return nil, fmt.Errorf("failed to build sts client: %w", err)
	}

	resolverChain := []porch.Resolver{
		porch.NewBasicAuthResolver(),
		porch.NewBearerTokenAuthResolver(),
		porch.NewGcloudWIResolver(coreV1Client, stsClient),
	}

	credentialResolver := porch.NewCredentialResolver(coreClient, resolverChain)
	caBundleResolver := porch.NewCredentialResolver(coreClient, []porch.Resolver{porch.NewCaBundleResolver()})
	referenceResolver := porch.NewReferenceResolver(coreClient)
	userInfoProvider := &porch.ApiserverUserInfoProvider{}

	watcherMgr := engine.NewWatcherManager()

	c.ExtraConfig.CacheOptions.CoreClient = coreClient
	c.ExtraConfig.CacheOptions.RepoPRChangeNotifier = watcherMgr
	c.ExtraConfig.CacheOptions.ExternalRepoOptions.CredentialResolver = credentialResolver
	c.ExtraConfig.CacheOptions.ExternalRepoOptions.CaBundleResolver = caBundleResolver
	c.ExtraConfig.CacheOptions.ExternalRepoOptions.UserInfoProvider = userInfoProvider
	c.ExtraConfig.CacheOptions.ExternalRepoOptions.RepoOperationRetryAttempts = c.ExtraConfig.CacheOptions.RepoOperationRetryAttempts

	c.ExtraConfig.CacheOptions.UseLegacySync = c.ExtraConfig.UseLegacySync

	// Create embedded repo controller if use-legacy-sync is disabled and using CR cache
	var embeddedController *EmbeddedControllerManager
	if !c.ExtraConfig.UseLegacySync && c.ExtraConfig.CacheOptions.CacheType == cachetypes.CRCacheType {
		embeddedController, err = c.createEmbeddedController(coreClient)
		if err != nil {
			return nil, fmt.Errorf("failed to create embedded controller: %w", err)
		}
	}

	cacheImpl, err := cache.GetCacheImpl(ctx, c.ExtraConfig.CacheOptions)

	if err != nil {
		return nil, fmt.Errorf("failed to create repository cache: %w", err)
	}

	runnerOptionsResolver := func(namespace string) runneroptions.RunnerOptions {
		runnerOptions := runneroptions.RunnerOptions{}
		runnerOptions.InitDefaults(c.ExtraConfig.GRPCRuntimeOptions.DefaultImagePrefix)
		return runnerOptions
	}

	cad, err := engine.NewCaDEngine(
		engine.WithCache(cacheImpl),
		// The order of registering the function runtimes matters here. When
		// evaluating a function, the runtimes will be tried in the same
		// order as they are registered.
		engine.WithBuiltinFunctionRuntime(c.ExtraConfig.GRPCRuntimeOptions.DefaultImagePrefix),
		engine.WithGRPCFunctionRuntime(c.ExtraConfig.GRPCRuntimeOptions),
		engine.WithCredentialResolver(credentialResolver),
		engine.WithRunnerOptionsResolver(runnerOptionsResolver),
		engine.WithReferenceResolver(referenceResolver),
		engine.WithUserInfoProvider(userInfoProvider),
		engine.WithWatcherManager(watcherMgr),
		engine.WithRepoOperationRetryAttempts(c.ExtraConfig.CacheOptions.RepoOperationRetryAttempts),
	)
	if err != nil {
		return nil, err
	}

	restStorageOptions := porch.RESTStorageOptions{
		Scheme:               Scheme,
		Codecs:               Codecs,
		CaD:                  cad,
		CoreClient:           coreClient,
		TimeoutPerRepository: c.ExtraConfig.ListTimeoutPerRepository,
		MaxConcurrentLists:   c.ExtraConfig.MaxConcurrentLists,
	}
	porchGroup, err := restStorageOptions.NewRESTStorage()
	if err != nil {
		return nil, err
	}

	s := &PorchServer{
		GenericAPIServer:   genericServer,
		coreClient:         coreClient,
		cache:              cacheImpl,
		ExtraConfig:        c.ExtraConfig,
		embeddedController: embeddedController,
		// Set background job periodic frequency the same as repo sync frequency.
		repoCacheSyncFrequency:     c.ExtraConfig.CacheOptions.RepoSyncFrequency,
		ListTimeoutPerRepository:   c.ExtraConfig.ListTimeoutPerRepository,
		repoOperationRetryAttempts: c.ExtraConfig.CacheOptions.RepoOperationRetryAttempts,
	}

	// Install the groups.
	if err := s.GenericAPIServer.InstallAPIGroups(&porchGroup); err != nil {
		return nil, err
	}

	return s, nil
}

func (s *PorchServer) Run(ctx context.Context) error {
	if s.ExtraConfig.UseLegacySync {
		klog.Info("Using legacy SyncManager")
		porch.RunBackground(ctx, s.coreClient, s.cache,
			porch.WithPeriodicRepoSyncFrequency(s.repoCacheSyncFrequency),
			porch.WithListTimeoutPerRepo(s.ListTimeoutPerRepository),
			porch.WithRepoOperationRetryAttempts(s.repoOperationRetryAttempts),
		)
	} else if s.embeddedController != nil {
		klog.Info("Starting embedded controller")
		s.embeddedController.cache = s.cache
		go func() {
			if err := s.embeddedController.Start(ctx); err != nil {
				klog.Error(err, "Embedded controller failed")
			}
		}()
	} else {
		klog.Info("Using standalone controller")
	}

	// TODO: Reconsider if the existence of CERT_STORAGE_DIR was a good inidcator for webhook setup,
	// but for now we keep backward compatiblity
	certStorageDir, found := os.LookupEnv("CERT_STORAGE_DIR")
	if found && strings.TrimSpace(certStorageDir) != "" {
		if err := setupWebhooks(ctx, s.coreClient); err != nil {
			klog.Errorf("%v\n", err)
			return err
		}
	} else {
		klog.Infoln("Cert storage dir not provided, skipping webhook setup")
	}
	return s.GenericAPIServer.PrepareRun().RunWithContext(ctx)
}
