package unit

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/devrev/pairdb/api-gateway/internal/config"
	"github.com/devrev/pairdb/api-gateway/internal/grpc"
	"github.com/devrev/pairdb/api-gateway/internal/server"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func TestNewServer(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	
	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:            8080,
			ReadTimeout:     30 * time.Second,
			WriteTimeout:    30 * time.Second,
			IdleTimeout:     120 * time.Second,
			ShutdownTimeout: 30 * time.Second,
		},
		Coordinator: config.CoordinatorConfig{
			Endpoints: []string{"localhost:50051"},
			Timeout:   30 * time.Second,
		},
		RateLimiter: config.RateLimiterConfig{
			Enabled:           true,
			RequestsPerSecond: 1000,
			BurstSize:         100,
		},
	}
	
	grpcCfg := config.CoordinatorConfig{
		Endpoints:             []string{"localhost:50051"},
		Timeout:               30 * time.Second,
		MaxReceiveMessageSize: 16 * 1024 * 1024,
		MaxSendMessageSize:    16 * 1024 * 1024,
	}
	
	grpcClient, err := grpc.NewClient(grpcCfg, logger)
	assert.NoError(t, err)
	defer grpcClient.Close()
	
	srv := server.NewServer(cfg, grpcClient, logger)
	assert.NotNil(t, srv)
}

func TestServer_SetupRoutes(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	
	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:            8080,
			ReadTimeout:     30 * time.Second,
			WriteTimeout:    30 * time.Second,
			IdleTimeout:     120 * time.Second,
			ShutdownTimeout: 30 * time.Second,
		},
		Coordinator: config.CoordinatorConfig{
			Endpoints: []string{"localhost:50051"},
			Timeout:   30 * time.Second,
		},
		RateLimiter: config.RateLimiterConfig{
			Enabled:           false, // Disable for testing
			RequestsPerSecond: 1000,
			BurstSize:         100,
		},
	}
	
	grpcCfg := config.CoordinatorConfig{
		Endpoints:             []string{"localhost:50051"},
		Timeout:               30 * time.Second,
		MaxReceiveMessageSize: 16 * 1024 * 1024,
		MaxSendMessageSize:    16 * 1024 * 1024,
	}
	
	grpcClient, err := grpc.NewClient(grpcCfg, logger)
	assert.NoError(t, err)
	defer grpcClient.Close()
	
	srv := server.NewServer(cfg, grpcClient, logger)
	srv.SetupRoutes()
	
	router := srv.GetRouter()
	assert.NotNil(t, router)
	
	handler := srv.GetHandler()
	assert.NotNil(t, handler)
}

func TestServer_HealthEndpoint(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	
	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:            8080,
			ReadTimeout:     30 * time.Second,
			WriteTimeout:    30 * time.Second,
			IdleTimeout:     120 * time.Second,
			ShutdownTimeout: 30 * time.Second,
		},
		Coordinator: config.CoordinatorConfig{
			Endpoints: []string{"localhost:50051"},
			Timeout:   30 * time.Second,
		},
		RateLimiter: config.RateLimiterConfig{
			Enabled: false,
		},
	}
	
	grpcCfg := config.CoordinatorConfig{
		Endpoints:             []string{"localhost:50051"},
		Timeout:               30 * time.Second,
		MaxReceiveMessageSize: 16 * 1024 * 1024,
		MaxSendMessageSize:    16 * 1024 * 1024,
	}
	
	grpcClient, err := grpc.NewClient(grpcCfg, logger)
	assert.NoError(t, err)
	defer grpcClient.Close()
	
	srv := server.NewServer(cfg, grpcClient, logger)
	srv.SetupRoutes()
	
	// Test health endpoint
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	
	srv.GetHandler().ServeHTTP(w, req)
	
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "healthy")
}

func TestServer_NotFoundHandler(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	
	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:            8080,
			ReadTimeout:     30 * time.Second,
			WriteTimeout:    30 * time.Second,
			IdleTimeout:     120 * time.Second,
			ShutdownTimeout: 30 * time.Second,
		},
		Coordinator: config.CoordinatorConfig{
			Endpoints: []string{"localhost:50051"},
			Timeout:   30 * time.Second,
		},
		RateLimiter: config.RateLimiterConfig{
			Enabled: false,
		},
	}
	
	grpcCfg := config.CoordinatorConfig{
		Endpoints:             []string{"localhost:50051"},
		Timeout:               30 * time.Second,
		MaxReceiveMessageSize: 16 * 1024 * 1024,
		MaxSendMessageSize:    16 * 1024 * 1024,
	}
	
	grpcClient, err := grpc.NewClient(grpcCfg, logger)
	assert.NoError(t, err)
	defer grpcClient.Close()
	
	srv := server.NewServer(cfg, grpcClient, logger)
	srv.SetupRoutes()
	
	// Test not found
	req := httptest.NewRequest(http.MethodGet, "/nonexistent", nil)
	w := httptest.NewRecorder()
	
	srv.GetHandler().ServeHTTP(w, req)
	
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestServer_Shutdown(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	
	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:            18080, // Use different port to avoid conflicts
			ReadTimeout:     30 * time.Second,
			WriteTimeout:    30 * time.Second,
			IdleTimeout:     120 * time.Second,
			ShutdownTimeout: 30 * time.Second,
		},
		Coordinator: config.CoordinatorConfig{
			Endpoints: []string{"localhost:50051"},
			Timeout:   30 * time.Second,
		},
		RateLimiter: config.RateLimiterConfig{
			Enabled: false,
		},
	}
	
	grpcCfg := config.CoordinatorConfig{
		Endpoints:             []string{"localhost:50051"},
		Timeout:               30 * time.Second,
		MaxReceiveMessageSize: 16 * 1024 * 1024,
		MaxSendMessageSize:    16 * 1024 * 1024,
	}
	
	grpcClient, err := grpc.NewClient(grpcCfg, logger)
	assert.NoError(t, err)
	defer grpcClient.Close()
	
	srv := server.NewServer(cfg, grpcClient, logger)
	srv.SetupRoutes()
	
	// Start server async
	errChan := srv.StartAsync()
	
	// Give it a moment to start
	time.Sleep(200 * time.Millisecond)
	
	// Shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	err = srv.Shutdown(ctx)
	assert.NoError(t, err)
	
	// Check for startup errors
	select {
	case err := <-errChan:
		// Expect nil or no error
		if err != nil && err != http.ErrServerClosed {
			t.Errorf("Unexpected server error: %v", err)
		}
	default:
	}
}

func TestServer_WithRateLimiter(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	
	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:            8080,
			ReadTimeout:     30 * time.Second,
			WriteTimeout:    30 * time.Second,
			IdleTimeout:     120 * time.Second,
			ShutdownTimeout: 30 * time.Second,
		},
		Coordinator: config.CoordinatorConfig{
			Endpoints: []string{"localhost:50051"},
			Timeout:   30 * time.Second,
		},
		RateLimiter: config.RateLimiterConfig{
			Enabled:           true,
			RequestsPerSecond: 1000,
			BurstSize:         100,
		},
	}
	
	grpcCfg := config.CoordinatorConfig{
		Endpoints:             []string{"localhost:50051"},
		Timeout:               30 * time.Second,
		MaxReceiveMessageSize: 16 * 1024 * 1024,
		MaxSendMessageSize:    16 * 1024 * 1024,
	}
	
	grpcClient, err := grpc.NewClient(grpcCfg, logger)
	assert.NoError(t, err)
	defer grpcClient.Close()
	
	srv := server.NewServer(cfg, grpcClient, logger)
	srv.SetupRoutes()
	
	// Test health endpoint with rate limiter enabled
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	
	srv.GetHandler().ServeHTTP(w, req)
	
	assert.Equal(t, http.StatusOK, w.Code)
}

