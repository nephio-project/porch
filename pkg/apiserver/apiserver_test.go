package apiserver

import (
	"testing"
	"time"

	repocontroller "github.com/nephio-project/porch/controllers/repositories/pkg/controllers/repository"
	cachetypes "github.com/nephio-project/porch/pkg/cache/types"
)

func TestBuildCompleteScheme(t *testing.T) {
	scheme, err := buildCompleteScheme()
	if err != nil {
		t.Fatalf("buildCompleteScheme failed: %v", err)
	}
	if scheme == nil {
		t.Fatal("expected scheme to be non-nil")
	}

	// Test singleton behavior - calling again should return same instance
	scheme2, err := buildCompleteScheme()
	if err != nil {
		t.Fatalf("buildCompleteScheme second call failed: %v", err)
	}
	if scheme != scheme2 {
		t.Error("expected buildCompleteScheme to return singleton instance")
	}
}

func TestEmbeddedControllerManagerStructure(t *testing.T) {
	// Test that EmbeddedControllerManager can be instantiated
	mgr := &EmbeddedControllerManager{
		config: repocontroller.EmbeddedConfig{
			MaxConcurrentReconciles: 5,
			MaxConcurrentSyncs:      10,
			HealthCheckFrequency:    30 * time.Second,
			FullSyncFrequency:       5 * time.Minute,
		},
	}

	if mgr.config.MaxConcurrentReconciles != 5 {
		t.Errorf("expected MaxConcurrentReconciles 5, got %d", mgr.config.MaxConcurrentReconciles)
	}
	if mgr.config.MaxConcurrentSyncs != 10 {
		t.Errorf("expected MaxConcurrentSyncs 10, got %d", mgr.config.MaxConcurrentSyncs)
	}
	if mgr.config.HealthCheckFrequency != 30*time.Second {
		t.Errorf("expected HealthCheckFrequency 30s, got %v", mgr.config.HealthCheckFrequency)
	}
	if mgr.config.FullSyncFrequency != 5*time.Minute {
		t.Errorf("expected FullSyncFrequency 5m, got %v", mgr.config.FullSyncFrequency)
	}
}

func TestSetupEmbeddedController(t *testing.T) {
	tests := []struct {
		name          string
		useLegacySync bool
		cacheType     cachetypes.CacheType
		expectNil     bool
	}{
		{
			name:          "returns nil when legacy sync enabled",
			useLegacySync: true,
			cacheType:     cachetypes.CRCacheType,
			expectNil:     true,
		},
		{
			name:          "returns nil when DB cache type",
			useLegacySync: false,
			cacheType:     cachetypes.DBCacheType,
			expectNil:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := completedConfig{
				ExtraConfig: &ExtraConfig{
					UseLegacySync: tt.useLegacySync,
					CacheOptions: cachetypes.CacheOptions{
						CacheType: tt.cacheType,
					},
				},
			}

			result, err := c.setupEmbeddedController(nil)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if tt.expectNil && result != nil {
				t.Error("expected nil controller")
			}
		})
	}
}
