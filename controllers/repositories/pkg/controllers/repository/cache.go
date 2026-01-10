package repository

import (
	"context"
	"fmt"

	"github.com/nephio-project/porch/pkg/cache"
	cachetypes "github.com/nephio-project/porch/pkg/cache/types"
	externalrepotypes "github.com/nephio-project/porch/pkg/externalrepo/types"
	"github.com/nephio-project/porch/pkg/registry/porch"
	"github.com/nephio-project/porch/pkg/repository"
	"k8s.io/apimachinery/pkg/watch"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// createDBCache creates a database cache instance for standalone controller with sync events
func (r *RepositoryReconciler) createDBCache(ctx context.Context, mgr ctrl.Manager) error {
	log := ctrl.LoggerFrom(ctx)
	log.V(1).Info("Initializing database cache for repository controller")
	
	// Create WithWatch client from manager's config
	coreClient, err := client.NewWithWatch(mgr.GetConfig(), client.Options{
		Scheme: mgr.GetScheme(),
	})
	if err != nil {
		log.Error(err, "Failed to create WithWatch client for database cache")
		return fmt.Errorf("failed to create WithWatch client: %w", err)
	}

	dataSource := fmt.Sprintf("host=%s port=%s dbname=%s user=%s password=%s sslmode=disable",
		r.dbHost, r.dbPort, r.dbName, r.dbUser, r.dbPassword)
	log.V(2).Info("Database connection configured", "host", r.dbHost, "port", r.dbPort, "dbname", r.dbName)

	// Create no-op change notifier for repository controller
	noOpNotifier := &noOpRepoPRChangeNotifier{}

	options := cachetypes.CacheOptions{
		CoreClient:         coreClient,
		CacheType:          cachetypes.DBCacheType,
		EnableSyncEvents:   true, // Enable sync events for standalone controller
		RepoPRChangeNotifier: noOpNotifier,
		ExternalRepoOptions: externalrepotypes.ExternalRepoOptions{
			CredentialResolver: porch.NewCredentialResolver(coreClient, []porch.Resolver{
				porch.NewBasicAuthResolver(),
				porch.NewBearerTokenAuthResolver(),
			}),
			RepoOperationRetryAttempts: 3,
		},
		DBCacheOptions: cachetypes.DBCacheOptions{
			Driver:     r.dbDriver,
			DataSource: dataSource,
		},
	}

	cacheImpl, err := cache.GetCacheImpl(ctx, options)
	if err != nil {
		log.Error(err, "Failed to initialize database cache", "driver", r.dbDriver)
		return fmt.Errorf("failed to create database cache: %w", err)
	}

	r.Cache = cacheImpl
	log.Info("Database cache initialized successfully")
	return nil
}
// noOpRepoPRChangeNotifier is a no-op implementation for repository controller
type noOpRepoPRChangeNotifier struct{}

func (n *noOpRepoPRChangeNotifier) NotifyPackageRevisionChange(eventType watch.EventType, obj repository.PackageRevision) int {
	// Repository controller doesn't need to notify itself
	return 0
}