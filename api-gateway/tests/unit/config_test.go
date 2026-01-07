package unit

import (
	"os"
	"testing"
	"time"

	"github.com/devrev/pairdb/api-gateway/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigLoad_Defaults(t *testing.T) {
	// Load config without a file - should use defaults
	cfg, err := config.Load("")
	require.NoError(t, err)
	assert.NotNil(t, cfg)

	// Check defaults
	assert.Equal(t, 8080, cfg.Server.Port)
	assert.Equal(t, 30*time.Second, cfg.Server.ReadTimeout)
	assert.Equal(t, 30*time.Second, cfg.Server.WriteTimeout)
	assert.Equal(t, 120*time.Second, cfg.Server.IdleTimeout)
	assert.Equal(t, 30*time.Second, cfg.Server.ShutdownTimeout)

	assert.Len(t, cfg.Coordinator.Endpoints, 1)
	assert.Equal(t, "localhost:50051", cfg.Coordinator.Endpoints[0])
	assert.Equal(t, 30*time.Second, cfg.Coordinator.Timeout)
	assert.Equal(t, 3, cfg.Coordinator.MaxRetries)

	assert.True(t, cfg.RateLimiter.Enabled)
	assert.Equal(t, 1000.0, cfg.RateLimiter.RequestsPerSecond)
	assert.Equal(t, 100, cfg.RateLimiter.BurstSize)

	assert.True(t, cfg.Metrics.Enabled)
	assert.Equal(t, 9090, cfg.Metrics.Port)
	assert.Equal(t, "/metrics", cfg.Metrics.Path)
}

func TestConfigLoad_FromEnvironment(t *testing.T) {
	// Set environment variables
	os.Setenv("API_GATEWAY_SERVER_PORT", "9000")
	os.Setenv("API_GATEWAY_COORDINATOR_TIMEOUT", "60s")
	defer func() {
		os.Unsetenv("API_GATEWAY_SERVER_PORT")
		os.Unsetenv("API_GATEWAY_COORDINATOR_TIMEOUT")
	}()

	cfg, err := config.Load("")
	require.NoError(t, err)
	assert.NotNil(t, cfg)

	assert.Equal(t, 9000, cfg.Server.Port)
	assert.Equal(t, 60*time.Second, cfg.Coordinator.Timeout)
}

func TestConfigValidate_ValidConfig(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:            8080,
			ReadTimeout:     30 * time.Second,
			WriteTimeout:    30 * time.Second,
			IdleTimeout:     120 * time.Second,
			ShutdownTimeout: 30 * time.Second,
		},
		Coordinator: config.CoordinatorConfig{
			Endpoints:  []string{"localhost:50051"},
			Timeout:    30 * time.Second,
			MaxRetries: 3,
		},
		RateLimiter: config.RateLimiterConfig{
			Enabled:           true,
			RequestsPerSecond: 1000,
			BurstSize:         100,
		},
		Metrics: config.MetricsConfig{
			Enabled: true,
			Port:    9090,
			Path:    "/metrics",
		},
	}

	err := cfg.Validate()
	assert.NoError(t, err)
}

func TestConfigValidate_InvalidServerPort(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Port: 0,
		},
		Coordinator: config.CoordinatorConfig{
			Endpoints: []string{"localhost:50051"},
			Timeout:   30 * time.Second,
		},
	}

	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid server port")
}

func TestConfigValidate_NoCoordinatorEndpoints(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Port: 8080,
		},
		Coordinator: config.CoordinatorConfig{
			Endpoints: []string{},
			Timeout:   30 * time.Second,
		},
	}

	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "at least one coordinator endpoint")
}

func TestConfigValidate_InvalidCoordinatorTimeout(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Port: 8080,
		},
		Coordinator: config.CoordinatorConfig{
			Endpoints: []string{"localhost:50051"},
			Timeout:   0,
		},
	}

	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "coordinator timeout must be positive")
}

func TestConfigValidate_InvalidRateLimiter(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Port: 8080,
		},
		Coordinator: config.CoordinatorConfig{
			Endpoints: []string{"localhost:50051"},
			Timeout:   30 * time.Second,
		},
		RateLimiter: config.RateLimiterConfig{
			Enabled:           true,
			RequestsPerSecond: 0,
			BurstSize:         100,
		},
	}

	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "rate limiter requests per second must be positive")
}

func TestConfigValidate_InvalidMetricsPort(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Port: 8080,
		},
		Coordinator: config.CoordinatorConfig{
			Endpoints: []string{"localhost:50051"},
			Timeout:   30 * time.Second,
		},
		RateLimiter: config.RateLimiterConfig{
			Enabled: false,
		},
		Metrics: config.MetricsConfig{
			Enabled: true,
			Port:    70000,
		},
	}

	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid metrics port")
}

