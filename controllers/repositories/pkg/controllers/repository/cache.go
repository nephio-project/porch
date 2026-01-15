package repository

import (
	"context"
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/nephio-project/porch/pkg/cache"
	cachetypes "github.com/nephio-project/porch/pkg/cache/types"
	externalrepotypes "github.com/nephio-project/porch/pkg/externalrepo/types"
	"github.com/nephio-project/porch/pkg/registry/porch"
	"github.com/nephio-project/porch/pkg/repository"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (r *RepositoryReconciler) createCacheFromEnv(ctx context.Context, mgr ctrl.Manager) error {
	if err := r.validateCacheType(); err != nil {
		return err
	}

	coreClient, err := client.NewWithWatch(mgr.GetConfig(), client.Options{
		Scheme: mgr.GetScheme(),
	})
	if err != nil {
		return fmt.Errorf("failed to create client: %w", err)
	}

	// Read DB configuration from environment variables
	dbOptions, err := r.setupDBCacheOptionsFromEnv()
	if err != nil {
		return fmt.Errorf("failed to setup DB cache options: %w", err)
	}

	// Setup cache directory for git repositories
	cacheDir := r.determineCacheDirectory()
	klog.Infof("[Repository Controller] Using git cache directory: %s", cacheDir)

	// Create credential resolver for git authentication
	credentialResolver, caBundleResolver := r.createCredentialResolvers(coreClient)

	// Simple user info provider for standalone controller
	userInfoProvider := &simpleUserInfoProvider{}

	options := r.buildCacheOptions(coreClient, dbOptions, cacheDir, credentialResolver, caBundleResolver, userInfoProvider)

	r.Cache, err = cache.GetCacheImpl(ctx, options)
	return err
}

// validateCacheType checks if the cache type is valid for standalone controller
func (r *RepositoryReconciler) validateCacheType() error {
	if strings.ToUpper(r.cacheType) == string(cachetypes.CRCacheType) {
		return fmt.Errorf("standalone controller requires DB cache")
	}
	return nil
}

// determineCacheDirectory resolves the cache directory with fallback logic
// Priority: 1. GIT_CACHE_DIR env var, 2. User cache dir, 3. Temp dir
func (r *RepositoryReconciler) determineCacheDirectory() string {
	cacheDir := os.Getenv("GIT_CACHE_DIR")
	if cacheDir == "" {
		var err error
		cacheDir, err = os.UserCacheDir()
		if err != nil {
			cacheDir = os.TempDir()
			klog.Warningf("Cannot find user cache directory, using temporary directory %q", cacheDir)
		}
		cacheDir = cacheDir + "/porch-repo-controller"
	}
	return cacheDir
}

// createCredentialResolvers creates the credential and CA bundle resolvers
func (r *RepositoryReconciler) createCredentialResolvers(coreClient client.Client) (repository.CredentialResolver, repository.CredentialResolver) {
	resolverChain := []porch.Resolver{
		porch.NewBasicAuthResolver(),
		porch.NewBearerTokenAuthResolver(),
	}
	credentialResolver := porch.NewCredentialResolver(coreClient, resolverChain)
	caBundleResolver := porch.NewCredentialResolver(coreClient, []porch.Resolver{porch.NewCaBundleResolver()})
	return credentialResolver, caBundleResolver
}

// buildCacheOptions constructs the cache options from all components
func (r *RepositoryReconciler) buildCacheOptions(
	coreClient client.WithWatch,
	dbOptions cachetypes.DBCacheOptions,
	cacheDir string,
	credentialResolver repository.CredentialResolver,
	caBundleResolver repository.CredentialResolver,
	userInfoProvider repository.UserInfoProvider,
) cachetypes.CacheOptions {
	return cachetypes.CacheOptions{
		CoreClient:     coreClient,
		CacheType:      cachetypes.CacheType(r.cacheType),
		DBCacheOptions: dbOptions,
		ExternalRepoOptions: externalrepotypes.ExternalRepoOptions{
			LocalDirectory:             cacheDir,
			UseUserDefinedCaBundle:     r.useUserDefinedCaBundle,
			CredentialResolver:         credentialResolver,
			CaBundleResolver:           caBundleResolver,
			UserInfoProvider:           userInfoProvider,
			RepoOperationRetryAttempts: r.RepoOperationRetryAttempts,
		},
		RepoPRChangeNotifier: cachetypes.NewNoOpRepoPRChangeNotifier(),
		UseLegacySync:        false, // Controllers handle sync
	}
}

func (r *RepositoryReconciler) setupDBCacheOptionsFromEnv() (cachetypes.DBCacheOptions, error) {
	dbDriver := os.Getenv("DB_DRIVER")
	dbHost := os.Getenv("DB_HOST")
	dbPort := os.Getenv("DB_PORT")
	dbName := os.Getenv("DB_NAME")
	dbUser := os.Getenv("DB_USER")
	dbUserPass := os.Getenv("DB_PASSWORD")
	dbSSLMode := strings.ToLower(os.Getenv("DB_SSL_MODE"))

	if dbDriver == "" {
		dbDriver = "pgx"
		klog.Infof("[Repository Controller] DB_DRIVER not provided, defaulting to: %v", dbDriver)
	}

	missingVars := []string{}
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
	if dbSSLMode == "" || dbSSLMode == "disable" {
		if dbUserPass == "" {
			missingVars = append(missingVars, "DB_PASSWORD")
		}
	}

	if len(missingVars) > 0 {
		return cachetypes.DBCacheOptions{}, fmt.Errorf("missing required environment variables: %v", missingVars)
	}

	// Build connection string
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
		return cachetypes.DBCacheOptions{}, fmt.Errorf("unsupported DB driver: %s", dbDriver)
	}

	return cachetypes.DBCacheOptions{
		Driver:     dbDriver,
		DataSource: connStr,
	}, nil
}

// simpleUserInfoProvider provides default user info for git commits
type simpleUserInfoProvider struct{}

func (p *simpleUserInfoProvider) GetUserInfo(ctx context.Context) *repository.UserInfo {
	return &repository.UserInfo{
		Name:  "porch-controller",
		Email: "porch-controller@kpt.dev",
	}
}
