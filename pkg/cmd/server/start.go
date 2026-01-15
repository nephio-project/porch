// Copyright 2022, 2025 The kpt and Nephio Authors
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

package server

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"time"

	"github.com/kptdev/kpt/pkg/lib/runneroptions"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	clientset "github.com/nephio-project/porch/api/generated/clientset/versioned"
	informers "github.com/nephio-project/porch/api/generated/informers/externalversions"
	sampleopenapi "github.com/nephio-project/porch/api/generated/openapi"
	porchapi "github.com/nephio-project/porch/api/porch/v1alpha1"
	"github.com/nephio-project/porch/pkg/apiserver"
	cachetypes "github.com/nephio-project/porch/pkg/cache/types"
	"github.com/nephio-project/porch/pkg/engine"
	"github.com/nephio-project/porch/pkg/externalrepo/git"
	externalrepotypes "github.com/nephio-project/porch/pkg/externalrepo/types"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apiserver/pkg/admission"
	"k8s.io/apiserver/pkg/endpoints/openapi"
	genericapiserver "k8s.io/apiserver/pkg/server"
	genericoptions "k8s.io/apiserver/pkg/server/options"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"k8s.io/klog/v2"
	netutils "k8s.io/utils/net"
)

const (
	defaultEtcdPathPrefix = "/registry/porch.kpt.dev"
	OpenAPITitle          = "Porch"
	OpenAPIVersion        = "0.1"
)

// PorchServerOptions contains state for master/api server
type PorchServerOptions struct {
	RecommendedOptions         *genericoptions.RecommendedOptions
	CacheDirectory             string
	CacheType                  string
	CoreAPIKubeconfigPath      string
	DbCacheDriver              string
	DbCacheDataSource          string
	DefaultImagePrefix         string
	FunctionRunnerAddress      string
	ListTimeoutPerRepository   time.Duration
	LocalStandaloneDebugging   bool // Enables local standalone running/debugging of the apiserver.
	MaxConcurrentLists         int
	MaxRequestBodySize         int
	RepoSyncFrequency          time.Duration
	RepoOperationRetryAttempts int
	RetryableGitErrors         []string // Additional retryable git error patterns
	RepoMaxConcurrentReconciles int
	RepoMaxConcurrentSyncs      int
	RepoHealthCheckFrequency    time.Duration
	RepoFullSyncFrequency       time.Duration
	SharedInformerFactory      informers.SharedInformerFactory
	StdOut                     io.Writer
	StdErr                     io.Writer
	UseLegacySync              bool
	UseLegacySyncSet           bool // Track if flag was explicitly set
	UseUserDefinedCaBundle     bool
}

// NewPorchServerOptions returns a new PorchServerOptions
func NewPorchServerOptions(out, errOut io.Writer) *PorchServerOptions {
	//
	// GroupVersions served by this server
	//
	versions := schema.GroupVersions{
		porchapi.SchemeGroupVersion,
	}

	o := &PorchServerOptions{
		RecommendedOptions: genericoptions.NewRecommendedOptions(
			defaultEtcdPathPrefix,
			apiserver.Codecs.LegacyCodec(versions...),
		),

		StdOut: out,
		StdErr: errOut,
	}
	o.RecommendedOptions.Etcd.StorageConfig.EncodeVersioner = versions
	o.RecommendedOptions.Etcd = nil
	return o
}

// NewCommandStartPorchServer provides a CLI handler for 'start master' command
// with a default PorchServerOptions.
func NewCommandStartPorchServer(ctx context.Context, defaults *PorchServerOptions) *cobra.Command {
	o := *defaults
	cmd := &cobra.Command{
		Short: "Launch a porch API server",
		Long:  "Launch a porch API server",
		RunE: func(c *cobra.Command, args []string) error {
			// Check if use-legacy-sync was explicitly set
			if c.Flags().Changed("use-legacy-sync") {
				o.UseLegacySyncSet = true
			}
			if err := o.Complete(); err != nil {
				return err
			}
			if err := o.Validate(args); err != nil {
				return err
			}
			if err := o.RunPorchServer(ctx); err != nil {
				return err
			}
			return nil
		},
	}

	flags := cmd.Flags()
	o.AddFlags(flags)

	return cmd
}

// Validate validates PorchServerOptions
func (o PorchServerOptions) Validate(args []string) error {
	errors := []error{}
	errors = append(errors, o.RecommendedOptions.Validate()...)

	if !cachetypes.IsACacheType(o.CacheType) {
		errors = append(errors, fmt.Errorf("specified cache-type %s is not supported", o.CacheType))
	}

	// Validate deployment mode constraints
	if err := o.validateDeploymentMode(); err != nil {
		errors = append(errors, err)
	}

	if o.MaxConcurrentLists < 0 {
		return fmt.Errorf("invalid value for max-parallel-repo-lists: 0 for no limit; > 0 for set limit")
	}

	return utilerrors.NewAggregate(errors)
}

// validateDeploymentMode validates the deployment mode configuration
func (o PorchServerOptions) validateDeploymentMode() error {
	if o.UseLegacySync {
		klog.Warningf("Legacy sync mode is deprecated")
	}
	return nil
}

// Complete fills in fields required to have valid data
func (o *PorchServerOptions) Complete() error {
	o.CoreAPIKubeconfigPath = o.RecommendedOptions.CoreAPI.CoreAPIKubeconfigPath

	if o.LocalStandaloneDebugging {
		if os.Getenv("KUBERNETES_SERVICE_HOST") != "" || os.Getenv("KUBERNETES_SERVICE_PORT") != "" {
			klog.Fatalf("--standalone-debug-mode must not be used when running in k8s")
		} else {
			o.RecommendedOptions.Authorization = nil
			o.RecommendedOptions.Admission = genericoptions.NewAdmissionOptions()
			o.RecommendedOptions.Authentication.RemoteKubeConfigFileOptional = true
		}
	} else {
		// This is needed in case the porch-server runs outside of the cluster, but without the --standalone-debug-mode flag.
		o.RecommendedOptions.Authentication.RemoteKubeConfigFile = o.CoreAPIKubeconfigPath
		o.RecommendedOptions.Authorization.RemoteKubeConfigFile = o.CoreAPIKubeconfigPath
	}

	if o.CacheDirectory == "" {
		cache, err := os.UserCacheDir()
		if err != nil {
			cache = os.TempDir()
			klog.Warningf("Cannot find user cache directory, using temporary directory %q", cache)
		}
		o.CacheDirectory = cache + "/porch"
	}

	o.CacheType = strings.ToUpper(o.CacheType)
	
	// Apply smart defaults for use-legacy-sync based on cache type if not explicitly set
	if !o.UseLegacySyncSet {
		if o.CacheType == string(cachetypes.DBCacheType) {
			// DB cache: default to legacy sync (no standalone controller required)
			o.UseLegacySync = true
			klog.Infof("Defaulting to legacy sync for DB cache (set --use-legacy-sync=false to use controller-based sync with standalone controller)")
		} else {
			// CR cache: default to controller-based sync (embedded controller available)
			o.UseLegacySync = false
			klog.Infof("Defaulting to controller-based sync for CR cache (embedded controller)")
		}
	}
	
	if o.CacheType == string(cachetypes.DBCacheType) {
		if err := o.setupDBCacheConn(); err != nil {
			return err
		}
	}

	// Parse and append additional retryable git errors
	if len(o.RetryableGitErrors) > 0 {
		git.AppendRetryableErrors(o.RetryableGitErrors)
	}

	return nil
}

func (o *PorchServerOptions) setupDBCacheConn() error {
	dbDriver := os.Getenv("DB_DRIVER")
	dbHost := os.Getenv("DB_HOST")
	dbPort := os.Getenv("DB_PORT")
	dbName := os.Getenv("DB_NAME")
	dbUser := os.Getenv("DB_USER")
	dbUserPass := os.Getenv("DB_PASSWORD")
	// DB_SSL_MODE is optional - Default is "disable"
	dbSSLMode := os.Getenv("DB_SSL_MODE")
	dbSSLMode = strings.ToLower(dbSSLMode)

	missingVars := []string{}
	if dbDriver == "" {
		dbDriver = "pgx"
		klog.Infof("DB_DRIVER not provided, defaulting to use db driver: %v", dbDriver)
	}
	if dbHost == "" {
		missingVars = append(missingVars, "DB_HOST")
	}
	if dbPort == "" {
		missingVars = append(missingVars, "DB_PORT")
	}
	if dbName == "" {
		missingVars = append(missingVars, "DB_NAME")
	}
	if dbUser == "" {
		missingVars = append(missingVars, "DB_USER")
	}
	// DB_PASSWORD is not needed if SSL mode is set.
	if dbSSLMode == "" || dbSSLMode == "disable" {
		if dbUserPass == "" {
			missingVars = append(missingVars, "DB_PASSWORD")
		}
	}

	if len(missingVars) > 0 {
		return fmt.Errorf("missing required environment variables: %v", missingVars)
	}

	// Build connection string based on the DB type
	var connStr string
	switch dbDriver {
	case "pgx":
		if dbSSLMode == "" || dbSSLMode == "disable" {
			connStr = fmt.Sprintf("postgres://%s:%s@%s/%s?sslmode=disable", dbUser, dbUserPass, net.JoinHostPort(dbHost, dbPort), dbName)
		} else {
			connStr = fmt.Sprintf("postgres://%s@%s/%s?sslmode=%s", dbUser, net.JoinHostPort(dbHost, dbPort), dbName, dbSSLMode)
		}

	case "mysql":
		connStr = fmt.Sprintf("%s:%s@tcp(%s:%s)/%s", dbUser, dbUserPass, dbHost, dbPort, dbName)
	default:
		return fmt.Errorf("unsupported DB driver: %s", dbDriver)
	}

	// Set the DB cache options
	o.DbCacheDriver = dbDriver
	o.DbCacheDataSource = connStr

	return nil
}

// Config returns config for the api server given PorchServerOptions
func (o *PorchServerOptions) Config() (*apiserver.Config, error) {
	// TODO have a "real" external address
	if err := o.RecommendedOptions.SecureServing.MaybeDefaultWithSelfSignedCerts("localhost", nil, []net.IP{netutils.ParseIPSloppy("127.0.0.1")}); err != nil {
		return nil, fmt.Errorf("error creating self-signed certificates: %w", err)
	}

	o.RecommendedOptions.ExtraAdmissionInitializers = func(c *genericapiserver.RecommendedConfig) ([]admission.PluginInitializer, error) {
		client, err := clientset.NewForConfig(c.LoopbackClientConfig)
		if err != nil {
			return nil, err
		}
		informerFactory := informers.NewSharedInformerFactory(client, c.LoopbackClientConfig.Timeout)
		o.SharedInformerFactory = informerFactory
		return []admission.PluginInitializer{}, nil
	}

	serverConfig := genericapiserver.NewRecommendedConfig(apiserver.Codecs)

	serverConfig.OpenAPIConfig = genericapiserver.DefaultOpenAPIConfig(sampleopenapi.GetOpenAPIDefinitions, openapi.NewDefinitionNamer(apiserver.Scheme))
	serverConfig.OpenAPIConfig.Info.Title = OpenAPITitle
	serverConfig.OpenAPIConfig.Info.Version = OpenAPIVersion

	serverConfig.OpenAPIV3Config = genericapiserver.DefaultOpenAPIV3Config(sampleopenapi.GetOpenAPIDefinitions, openapi.NewDefinitionNamer(apiserver.Scheme))
	serverConfig.OpenAPIConfig.Info.Title = OpenAPITitle
	serverConfig.OpenAPIConfig.Info.Version = OpenAPIVersion
	serverConfig.MaxRequestBodyBytes = int64(o.MaxRequestBodySize)

	if err := o.RecommendedOptions.ApplyTo(serverConfig); err != nil {
		return nil, err
	}

	config := &apiserver.Config{
		GenericConfig: serverConfig,
		ExtraConfig: apiserver.ExtraConfig{
			CoreAPIKubeconfigPath: o.CoreAPIKubeconfigPath,
			GRPCRuntimeOptions: engine.GRPCRuntimeOptions{
				FunctionRunnerAddress: o.FunctionRunnerAddress,
				MaxGrpcMessageSize:    o.MaxRequestBodySize,
				DefaultImagePrefix:    o.DefaultImagePrefix,
			},
			CacheOptions: cachetypes.CacheOptions{
				ExternalRepoOptions: externalrepotypes.ExternalRepoOptions{
					LocalDirectory:         o.CacheDirectory,
					UseUserDefinedCaBundle: o.UseUserDefinedCaBundle,
				},
				RepoSyncFrequency:          o.RepoSyncFrequency,
				RepoOperationRetryAttempts: o.RepoOperationRetryAttempts,
				CacheType:                  cachetypes.CacheType(o.CacheType),
				DBCacheOptions: cachetypes.DBCacheOptions{
					Driver:     o.DbCacheDriver,
					DataSource: o.DbCacheDataSource,
				},
			},
			RepoControllerConfig: apiserver.RepoControllerConfig{
				MaxConcurrentReconciles: o.RepoMaxConcurrentReconciles,
				MaxConcurrentSyncs:      o.RepoMaxConcurrentSyncs,
				HealthCheckFrequency:    o.RepoHealthCheckFrequency,
				FullSyncFrequency:       o.RepoFullSyncFrequency,
			},
			ListTimeoutPerRepository: o.ListTimeoutPerRepository,
			MaxConcurrentLists:       o.MaxConcurrentLists,
			UseLegacySync:         o.UseLegacySync,
		},
	}
	return config, nil
}

// RunPorchServer starts a new PorchServer given PorchServerOptions
func (o PorchServerOptions) RunPorchServer(ctx context.Context) error {
	config, err := o.Config()
	if err != nil {
		return err
	}

	server, err := config.Complete().New(ctx)
	if err != nil {
		return err
	}

	if config.GenericConfig.SharedInformerFactory != nil {
		server.GenericAPIServer.AddPostStartHookOrDie("start-sample-server-informers", func(context genericapiserver.PostStartHookContext) error {
			config.GenericConfig.SharedInformerFactory.Start(context.Done())
			o.SharedInformerFactory.Start(context.Done())
			return nil
		})
	}

	return server.Run(ctx)
}

func (o *PorchServerOptions) AddFlags(fs *pflag.FlagSet) {
	// Add base flags
	o.RecommendedOptions.AddFlags(fs)
	utilfeature.DefaultMutableFeatureGate.AddFlag(fs)

	// Debugging flags
	if os.Getenv("KUBERNETES_SERVICE_HOST") == "" && os.Getenv("KUBERNETES_SERVICE_PORT") == "" {
		fs.BoolVar(&o.LocalStandaloneDebugging, "standalone-debug-mode", false,
			"Under the local-debug mode the apiserver will allow all access to its resources without "+
				"authorizing the requests, this flag is only intended for debugging in your workstation.")
	}

	// Cache configuration
	fs.StringVar(&o.CacheDirectory, "cache-directory", "", "Directory where Porch server stores repository and package caches.")
	fs.StringVar(&o.CacheType, "cache-type", string(cachetypes.DefaultCacheType), "Type of cache to use for cacheing repos, supported types are \"CR\" (Custom Resource) and \"DB\" (DataBase)")
	fs.StringVar(&o.DbCacheDriver, "db-cache-driver", cachetypes.DefaultDBCacheDriver, "Database driver to use when for the database cache")
	fs.StringVar(&o.DbCacheDataSource, "db-cache-data-source", "", "Address of the database, for example \"postgresql://user:pass@hostname:port/database\"")

	// Function runner configuration
	fs.StringVar(&o.DefaultImagePrefix, "default-image-prefix", runneroptions.GHCRImagePrefix, "Default prefix for unqualified function names")
	fs.StringVar(&o.FunctionRunnerAddress, "function-runner", "", "Address of the function runner gRPC service.")
	fs.IntVar(&o.MaxRequestBodySize, "max-request-body-size", 6*1024*1024, "Maximum size of the request body in bytes. Keep this in sync with function-runner's corresponding argument.")

	// Repository operations configuration
	fs.BoolVar(&o.UseUserDefinedCaBundle, "use-user-cabundle", false, "Determine whether to use a user-defined CaBundle for TLS towards the repository system.")
	fs.IntVar(&o.RepoOperationRetryAttempts, "repo-operation-retry-attempts", 3, "Number of retry attempts for repository operations.")
	fs.StringSliceVar(&o.RetryableGitErrors, "retryable-git-errors", nil, "Additional retryable git error patterns. Can be specified multiple times or as comma-separated values.")
	fs.DurationVar(&o.ListTimeoutPerRepository, "list-timeout-per-repo", 20*time.Second, "Maximum amount of time to wait for a repository list request.")
	fs.IntVar(&o.MaxConcurrentLists, "max-parallel-repo-lists", 10, "Maximum number of repositories to list in parallel.")

	// Repository sync configuration (legacy)
	fs.DurationVar(&o.RepoSyncFrequency, "repo-sync-frequency", 10*time.Minute, "Frequency at which registered repository CRs will be synced (legacy sync only).")

	// Repository controller configuration (controller-based sync)
	fs.IntVar(&o.RepoMaxConcurrentReconciles, "repo-max-concurrent-reconciles", 100, "Maximum number of repository reconciliations to run concurrently.")
	fs.IntVar(&o.RepoMaxConcurrentSyncs, "repo-max-concurrent-syncs", 50, "Maximum number of repository syncs to run concurrently.")
	fs.DurationVar(&o.RepoHealthCheckFrequency, "repo-health-check-frequency", 5*time.Minute, "Frequency at which repository health checks are performed.")
	fs.DurationVar(&o.RepoFullSyncFrequency, "repo-full-sync-frequency", 1*time.Hour, "Frequency at which full repository syncs are performed.")
	
	// Repository Sync mode selection
	fs.BoolVar(&o.UseLegacySync, "use-legacy-sync", false, "Use legacy sync mode (deprecated). Defaults: false for CR cache (embedded controller), true for DB cache (requires standalone controller if false).")
}
