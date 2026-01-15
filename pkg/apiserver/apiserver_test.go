package apiserver

import (
	"fmt"
	"testing"
	"time"

	repocontroller "github.com/nephio-project/porch/controllers/repositories/pkg/controllers/repository"
	cachetypes "github.com/nephio-project/porch/pkg/cache/types"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
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

func TestCreateEmbeddedControllerFunction(t *testing.T) {
	// Test the standalone createEmbeddedController function
	scheme := runtime.NewScheme()
	config := repocontroller.EmbeddedConfig{
		MaxConcurrentReconciles: 10,
		MaxConcurrentSyncs:      5,
		HealthCheckFrequency:    1 * time.Minute,
		FullSyncFrequency:       10 * time.Minute,
	}

	// Use minimal rest config
	restConfig := &rest.Config{
		Host: "https://localhost:6443",
	}

	mgr, err := createEmbeddedController(nil, restConfig, scheme, config)
	if err != nil {
		t.Fatalf("createEmbeddedController failed: %v", err)
	}
	if mgr == nil {
		t.Fatal("expected non-nil controller manager")
	}
	if mgr.config.MaxConcurrentReconciles != 10 {
		t.Errorf("expected MaxConcurrentReconciles 10, got %d", mgr.config.MaxConcurrentReconciles)
	}
	if mgr.config.MaxConcurrentSyncs != 5 {
		t.Errorf("expected MaxConcurrentSyncs 5, got %d", mgr.config.MaxConcurrentSyncs)
	}
}

func TestBuildSchemeWithTypes(t *testing.T) {
	tests := []struct {
		name        string
		builders    []schemeBuilder
		expectError bool
	}{
		{
			name: "success with valid builders",
			builders: []schemeBuilder{
				func(s *runtime.Scheme) error {
					return corev1.AddToScheme(s)
				},
			},
			expectError: false,
		},
		{
			name: "error from first builder",
			builders: []schemeBuilder{
				func(s *runtime.Scheme) error {
					return fmt.Errorf("mock error")
				},
			},
			expectError: true,
		},
		{
			name: "error from second builder",
			builders: []schemeBuilder{
				func(s *runtime.Scheme) error {
					return corev1.AddToScheme(s)
				},
				func(s *runtime.Scheme) error {
					return fmt.Errorf("second builder error")
				},
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme, err := buildSchemeWithTypes(tt.builders...)
			if tt.expectError {
				if err == nil {
					t.Error("expected error but got none")
				}
				if scheme != nil {
					t.Error("expected nil scheme on error")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if scheme == nil {
					t.Error("expected non-nil scheme")
				}
			}
		})
	}
}
