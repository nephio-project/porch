package apiserver

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	repocontroller "github.com/nephio-project/porch/controllers/repositories/pkg/controllers/repository"
	cachetypes "github.com/nephio-project/porch/pkg/cache/types"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
)

func TestBuildCompleteScheme(t *testing.T) {
	scheme, err := buildCompleteScheme()
	require.NoError(t, err)
	require.NotNil(t, scheme)

	// Test singleton behavior - calling again should return same instance
	scheme2, err := buildCompleteScheme()
	require.NoError(t, err)
	assert.Same(t, scheme, scheme2, "expected buildCompleteScheme to return singleton instance")
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

	assert.Equal(t, 5, mgr.config.MaxConcurrentReconciles)
	assert.Equal(t, 10, mgr.config.MaxConcurrentSyncs)
	assert.Equal(t, 30*time.Second, mgr.config.HealthCheckFrequency)
	assert.Equal(t, 5*time.Minute, mgr.config.FullSyncFrequency)
}

func TestSetupEmbeddedControllerManager(t *testing.T) {
	tests := []struct {
		name      string
		cacheType cachetypes.CacheType
		expectNil bool
	}{
		{
			name:      "returns nil when DB cache type",
			cacheType: cachetypes.DBCacheType,
			expectNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := completedConfig{
				ExtraConfig: &ExtraConfig{
					CacheOptions: cachetypes.CacheOptions{
						CacheType: tt.cacheType,
					},
				},
			}

			result, err := c.setupEmbeddedControllerManager()
			assert.NoError(t, err)
			if tt.expectNil {
				assert.Nil(t, result)
			}
		})
	}
}

func TestCreateEmbeddedControllerManagerFunction(t *testing.T) {
	// Test the standalone createEmbeddedControllerManager function
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

	mgr, err := createEmbeddedControllerManager(restConfig, scheme, config)
	require.NoError(t, err)
	require.NotNil(t, mgr)
	assert.Equal(t, 10, mgr.config.MaxConcurrentReconciles)
	assert.Equal(t, 5, mgr.config.MaxConcurrentSyncs)
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
				require.Error(t, err)
				assert.Contains(t, err.Error(), "error")
				assert.Nil(t, scheme)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, scheme)
			}
		})
	}
}
