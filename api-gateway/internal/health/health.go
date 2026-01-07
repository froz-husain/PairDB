// Package health provides health check endpoints for the API Gateway.
package health

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/devrev/pairdb/api-gateway/internal/grpc"
	"go.uber.org/zap"
)

// HealthCheck manages health check functionality.
type HealthCheck struct {
	grpcClient *grpc.Client
	logger     *zap.Logger
	mu         sync.RWMutex
	ready      bool
	lastCheck  time.Time
	checkInterval time.Duration
}

// NewHealthCheck creates a new HealthCheck instance.
func NewHealthCheck(grpcClient *grpc.Client, logger *zap.Logger) *HealthCheck {
	hc := &HealthCheck{
		grpcClient:    grpcClient,
		logger:        logger,
		ready:         false,
		checkInterval: 5 * time.Second,
	}
	
	// Start background health check
	go hc.backgroundCheck()
	
	return hc
}

// LivenessResponse represents the response for the liveness check.
type LivenessResponse struct {
	Status string `json:"status"`
}

// ReadinessResponse represents the response for the readiness check.
type ReadinessResponse struct {
	Status    string            `json:"status"`
	Checks    map[string]string `json:"checks,omitempty"`
	Error     string            `json:"error,omitempty"`
}

// LivenessHandler handles GET /health requests.
// Returns 200 OK if the process is running.
func (hc *HealthCheck) LivenessHandler(w http.ResponseWriter, r *http.Request) {
	resp := LivenessResponse{
		Status: "healthy",
	}
	
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}

// ReadinessHandler handles GET /ready requests.
// Returns 200 OK if the service can handle requests (gRPC connection is healthy).
func (hc *HealthCheck) ReadinessHandler(w http.ResponseWriter, r *http.Request) {
	hc.mu.RLock()
	isReady := hc.ready
	hc.mu.RUnlock()
	
	w.Header().Set("Content-Type", "application/json")
	
	if isReady {
		resp := ReadinessResponse{
			Status: "ready",
			Checks: map[string]string{
				"coordinator": "healthy",
			},
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(resp)
		return
	}
	
	// Perform a fresh check if not ready
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	
	err := hc.grpcClient.HealthCheck(ctx)
	if err != nil {
		resp := ReadinessResponse{
			Status: "not_ready",
			Checks: map[string]string{
				"coordinator": "unhealthy",
			},
			Error: err.Error(),
		}
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(resp)
		return
	}
	
	// Update ready status
	hc.mu.Lock()
	hc.ready = true
	hc.lastCheck = time.Now()
	hc.mu.Unlock()
	
	resp := ReadinessResponse{
		Status: "ready",
		Checks: map[string]string{
			"coordinator": "healthy",
		},
	}
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}

// backgroundCheck performs periodic health checks.
func (hc *HealthCheck) backgroundCheck() {
	ticker := time.NewTicker(hc.checkInterval)
	defer ticker.Stop()
	
	for range ticker.C {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		err := hc.grpcClient.HealthCheck(ctx)
		cancel()
		
		hc.mu.Lock()
		if err != nil {
			hc.ready = false
			hc.logger.Warn("health check failed", zap.Error(err))
		} else {
			hc.ready = true
		}
		hc.lastCheck = time.Now()
		hc.mu.Unlock()
	}
}

// IsReady returns the current readiness status.
func (hc *HealthCheck) IsReady() bool {
	hc.mu.RLock()
	defer hc.mu.RUnlock()
	return hc.ready
}

// SetReady sets the readiness status (for testing).
func (hc *HealthCheck) SetReady(ready bool) {
	hc.mu.Lock()
	defer hc.mu.Unlock()
	hc.ready = ready
}

