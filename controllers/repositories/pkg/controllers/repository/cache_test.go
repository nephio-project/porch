package repository

import (
	"context"
	"os"
	"testing"

	cachetypes "github.com/nephio-project/porch/pkg/cache/types"
	"github.com/nephio-project/porch/pkg/repository"
	mockclient "github.com/nephio-project/porch/test/mockery/mocks/external/sigs.k8s.io/controller-runtime/pkg/client"
)

func TestSetupDBCacheOptionsFromEnv(t *testing.T) {
	tests := []struct {
		name        string
		envVars     map[string]string
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid pgx configuration",
			envVars: map[string]string{
				"DB_DRIVER":   "pgx",
				"DB_HOST":     "localhost",
				"DB_PORT":     "5432",
				"DB_NAME":     "porch",
				"DB_USER":     "testuser",
				"DB_PASSWORD": "testpass",
			},
			expectError: false,
		},
		{
			name: "valid mysql configuration",
			envVars: map[string]string{
				"DB_DRIVER":   "mysql",
				"DB_HOST":     "localhost",
				"DB_PORT":     "3306",
				"DB_NAME":     "porch",
				"DB_USER":     "testuser",
				"DB_PASSWORD": "testpass",
			},
			expectError: false,
		},
		{
			name: "pgx with SSL mode",
			envVars: map[string]string{
				"DB_DRIVER":   "pgx",
				"DB_HOST":     "localhost",
				"DB_PORT":     "5432",
				"DB_NAME":     "porch",
				"DB_USER":     "testuser",
				"DB_SSL_MODE": "require",
			},
			expectError: false,
		},
		{
			name: "missing DB_HOST",
			envVars: map[string]string{
				"DB_PORT":     "5432",
				"DB_NAME":     "porch",
				"DB_USER":     "testuser",
				"DB_PASSWORD": "testpass",
			},
			expectError: true,
			errorMsg:    "DB_HOST",
		},
		{
			name: "missing DB_PORT",
			envVars: map[string]string{
				"DB_HOST":     "localhost",
				"DB_NAME":     "porch",
				"DB_USER":     "testuser",
				"DB_PASSWORD": "testpass",
			},
			expectError: true,
			errorMsg:    "DB_PORT",
		},
		{
			name: "missing DB_NAME",
			envVars: map[string]string{
				"DB_HOST":     "localhost",
				"DB_PORT":     "5432",
				"DB_USER":     "testuser",
				"DB_PASSWORD": "testpass",
			},
			expectError: true,
			errorMsg:    "DB_NAME",
		},
		{
			name: "missing DB_USER",
			envVars: map[string]string{
				"DB_HOST":     "localhost",
				"DB_PORT":     "5432",
				"DB_NAME":     "porch",
				"DB_PASSWORD": "testpass",
			},
			expectError: true,
			errorMsg:    "DB_USER",
		},
		{
			name: "missing DB_PASSWORD without SSL",
			envVars: map[string]string{
				"DB_HOST": "localhost",
				"DB_PORT": "5432",
				"DB_NAME": "porch",
				"DB_USER": "testuser",
			},
			expectError: true,
			errorMsg:    "DB_PASSWORD",
		},
		{
			name: "unsupported driver",
			envVars: map[string]string{
				"DB_DRIVER":   "sqlite",
				"DB_HOST":     "localhost",
				"DB_PORT":     "5432",
				"DB_NAME":     "porch",
				"DB_USER":     "testuser",
				"DB_PASSWORD": "testpass",
			},
			expectError: true,
			errorMsg:    "unsupported DB driver",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear all DB env vars first
			os.Unsetenv("DB_DRIVER")
			os.Unsetenv("DB_HOST")
			os.Unsetenv("DB_PORT")
			os.Unsetenv("DB_NAME")
			os.Unsetenv("DB_USER")
			os.Unsetenv("DB_PASSWORD")
			os.Unsetenv("DB_SSL_MODE")

			// Set test env vars
			for k, v := range tt.envVars {
				os.Setenv(k, v)
			}

			r := &RepositoryReconciler{}
			opts, err := r.setupDBCacheOptionsFromEnv()

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
				} else if tt.errorMsg != "" && !contains(err.Error(), tt.errorMsg) {
					t.Errorf("expected error containing %q, got %q", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if opts.Driver == "" {
					t.Error("expected driver to be set")
				}
				if opts.DataSource == "" {
					t.Error("expected data source to be set")
				}
			}
		})
	}
}

func TestSimpleUserInfoProvider(t *testing.T) {
	provider := &simpleUserInfoProvider{}
	ctx := context.Background()

	userInfo := provider.GetUserInfo(ctx)

	if userInfo == nil {
		t.Fatal("expected user info, got nil")
	}
	if userInfo.Name != "porch-controller" {
		t.Errorf("expected name 'porch-controller', got %q", userInfo.Name)
	}
	if userInfo.Email != "porch-controller@kpt.dev" {
		t.Errorf("expected email 'porch-controller@kpt.dev', got %q", userInfo.Email)
	}
}

func TestSimpleUserInfoProviderReturnsConsistentInfo(t *testing.T) {
	provider := &simpleUserInfoProvider{}
	ctx := context.Background()

	info1 := provider.GetUserInfo(ctx)
	info2 := provider.GetUserInfo(ctx)

	if info1.Name != info2.Name {
		t.Error("user info should be consistent across calls")
	}
	if info1.Email != info2.Email {
		t.Error("user info should be consistent across calls")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || 
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// Ensure simpleUserInfoProvider implements the interface
var _ repository.UserInfoProvider = (*simpleUserInfoProvider)(nil)

func TestValidateCacheType(t *testing.T) {
	tests := []struct {
		name        string
		cacheType   string
		expectError bool
	}{
		{
			name:        "valid DB cache type",
			cacheType:   "db",
			expectError: false,
		},
		{
			name:        "valid DB cache type uppercase",
			cacheType:   "DB",
			expectError: false,
		},
		{
			name:        "invalid CR cache type",
			cacheType:   "cr",
			expectError: true,
		},
		{
			name:        "invalid CR cache type uppercase",
			cacheType:   "CR",
			expectError: true,
		},
		{
			name:        "empty cache type",
			cacheType:   "",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &RepositoryReconciler{cacheType: tt.cacheType}
			err := r.validateCacheType()

			if tt.expectError && err == nil {
				t.Error("expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestDetermineCacheDirectory(t *testing.T) {
	tests := []struct {
		name           string
		gitCacheDirEnv string
		expectSuffix   string
	}{
		{
			name:           "uses GIT_CACHE_DIR env var",
			gitCacheDirEnv: "/custom/cache/dir",
			expectSuffix:   "",
		},
		{
			name:           "falls back to user cache dir",
			gitCacheDirEnv: "",
			expectSuffix:   "/porch-repo-controller",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save and restore env var
			oldEnv := os.Getenv("GIT_CACHE_DIR")
			defer os.Setenv("GIT_CACHE_DIR", oldEnv)

			if tt.gitCacheDirEnv != "" {
				os.Setenv("GIT_CACHE_DIR", tt.gitCacheDirEnv)
			} else {
				os.Unsetenv("GIT_CACHE_DIR")
			}

			r := &RepositoryReconciler{}
			cacheDir := r.determineCacheDirectory()

			if cacheDir == "" {
				t.Error("cache directory should not be empty")
			}

			if tt.gitCacheDirEnv != "" {
				if cacheDir != tt.gitCacheDirEnv {
					t.Errorf("expected cache dir %q, got %q", tt.gitCacheDirEnv, cacheDir)
				}
			} else if tt.expectSuffix != "" {
				if !contains(cacheDir, tt.expectSuffix) {
					t.Errorf("expected cache dir to contain %q, got %q", tt.expectSuffix, cacheDir)
				}
			}
		})
	}
}

func TestBuildCacheOptions(t *testing.T) {
	r := &RepositoryReconciler{
		cacheType:                  "db",
		useUserDefinedCaBundle:     true,
		RepoOperationRetryAttempts: 3,
	}

	dbOptions := cachetypes.DBCacheOptions{
		Driver:     "pgx",
		DataSource: "postgres://test",
	}

	userInfoProvider := &simpleUserInfoProvider{}

	options := r.buildCacheOptions(nil, dbOptions, "/test/cache", nil, nil, userInfoProvider)

	if options.CacheType != cachetypes.CacheType("db") {
		t.Errorf("expected cache type 'db', got %q", options.CacheType)
	}
	if options.DBCacheOptions.Driver != "pgx" {
		t.Errorf("expected driver 'pgx', got %q", options.DBCacheOptions.Driver)
	}
	if options.ExternalRepoOptions.LocalDirectory != "/test/cache" {
		t.Errorf("expected cache dir '/test/cache', got %q", options.ExternalRepoOptions.LocalDirectory)
	}
	if options.ExternalRepoOptions.UseUserDefinedCaBundle != true {
		t.Error("expected UseUserDefinedCaBundle to be true")
	}
	if options.ExternalRepoOptions.RepoOperationRetryAttempts != 3 {
		t.Errorf("expected retry attempts 3, got %d", options.ExternalRepoOptions.RepoOperationRetryAttempts)
	}
	if options.ExternalRepoOptions.UserInfoProvider != userInfoProvider {
		t.Error("expected user info provider to be set")
	}
}

func TestCreateCredentialResolvers(t *testing.T) {
	// Use the existing mockery client
	mockClient := mockclient.NewMockClient(t)
	
	r := &RepositoryReconciler{}
	credResolver, caResolver := r.createCredentialResolvers(mockClient)

	if credResolver == nil {
		t.Error("expected credential resolver to be created")
	}
	if caResolver == nil {
		t.Error("expected CA bundle resolver to be created")
	}
}
