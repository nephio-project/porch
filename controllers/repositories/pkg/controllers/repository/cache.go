package repository

import (
	"context"
	"fmt"

	"github.com/nephio-project/porch/pkg/cache"
	cachetypes "github.com/nephio-project/porch/pkg/cache/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// createDBCache creates a database cache instance for standalone controller with sync events
func (r *RepositoryReconciler) createDBCache(ctx context.Context, mgr ctrl.Manager) error {
	// Create WithWatch client from manager's config
	coreClient, err := client.NewWithWatch(mgr.GetConfig(), client.Options{
		Scheme: mgr.GetScheme(),
	})
	if err != nil {
		return fmt.Errorf("failed to create WithWatch client: %w", err)
	}

	dataSource := fmt.Sprintf("host=%s port=%s dbname=%s user=%s password=%s sslmode=disable",
		r.dbHost, r.dbPort, r.dbName, r.dbUser, r.dbPassword)

	options := cachetypes.CacheOptions{
		CoreClient:         coreClient,
		CacheType:          cachetypes.DBCacheType,
		EnableSyncEvents:   true, // Enable sync events for standalone controller
		DBCacheOptions: cachetypes.DBCacheOptions{
			Driver:     r.dbDriver,
			DataSource: dataSource,
		},
	}

	cacheImpl, err := cache.GetCacheImpl(ctx, options)
	if err != nil {
		return fmt.Errorf("failed to create database cache: %w", err)
	}

	r.Cache = cacheImpl
	return nil
}
