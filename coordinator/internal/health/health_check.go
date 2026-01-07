package health

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/devrev/pairdb/coordinator/internal/store"
	"go.uber.org/zap"
)

// HealthChecker provides health check endpoints
type HealthChecker struct {
	metadataStore    store.MetadataStore
	idempotencyStore store.IdempotencyStore
	cache            store.Cache
	logger           *zap.Logger
}

// HealthStatus represents the health status response
type HealthStatus struct {
	Status    string            `json:"status"`
	Timestamp int64             `json:"timestamp"`
	Checks    map[string]string `json:"checks,omitempty"`
}

// NewHealthChecker creates a new health checker
func NewHealthChecker(
	metadataStore store.MetadataStore,
	idempotencyStore store.IdempotencyStore,
	cache store.Cache,
	logger *zap.Logger,
) *HealthChecker {
	return &HealthChecker{
		metadataStore:    metadataStore,
		idempotencyStore: idempotencyStore,
		cache:            cache,
		logger:           logger,
	}
}

// LivenessHandler handles liveness probe requests
func (h *HealthChecker) LivenessHandler(w http.ResponseWriter, r *http.Request) {
	status := HealthStatus{
		Status:    "alive",
		Timestamp: time.Now().Unix(),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(status)
}

// ReadinessHandler handles readiness probe requests
func (h *HealthChecker) ReadinessHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	checks := make(map[string]string)
	allHealthy := true

	// Check metadata store (PostgreSQL)
	if err := h.checkMetadataStore(ctx); err != nil {
		h.logger.Error("Metadata store health check failed", zap.Error(err))
		checks["metadata_store"] = "unhealthy: " + err.Error()
		allHealthy = false
	} else {
		checks["metadata_store"] = "healthy"
	}

	// Check idempotency store (Redis)
	if err := h.checkIdempotencyStore(ctx); err != nil {
		h.logger.Error("Idempotency store health check failed", zap.Error(err))
		checks["idempotency_store"] = "unhealthy: " + err.Error()
		allHealthy = false
	} else {
		checks["idempotency_store"] = "healthy"
	}

	// Check cache
	if err := h.checkCache(ctx); err != nil {
		h.logger.Error("Cache health check failed", zap.Error(err))
		checks["cache"] = "unhealthy: " + err.Error()
		allHealthy = false
	} else {
		checks["cache"] = "healthy"
	}

	status := HealthStatus{
		Timestamp: time.Now().Unix(),
		Checks:    checks,
	}

	w.Header().Set("Content-Type", "application/json")

	if allHealthy {
		status.Status = "ready"
		w.WriteHeader(http.StatusOK)
	} else {
		status.Status = "not_ready"
		w.WriteHeader(http.StatusServiceUnavailable)
	}

	json.NewEncoder(w).Encode(status)
}

// checkMetadataStore checks if the metadata store is healthy
func (h *HealthChecker) checkMetadataStore(ctx context.Context) error {
	if h.metadataStore == nil {
		return nil // Skip if not initialized
	}
	return h.metadataStore.Ping(ctx)
}

// checkIdempotencyStore checks if the idempotency store is healthy
func (h *HealthChecker) checkIdempotencyStore(ctx context.Context) error {
	if h.idempotencyStore == nil {
		return nil // Skip if not initialized
	}
	return h.idempotencyStore.Ping(ctx)
}

// checkCache checks if the cache is healthy
func (h *HealthChecker) checkCache(ctx context.Context) error {
	if h.cache == nil {
		return nil // Skip if not initialized
	}
	// Cache is always available (in-memory), no ping needed
	return nil
}

// StartHealthServer starts the health check HTTP server
func StartHealthServer(hc *HealthChecker, port int, logger *zap.Logger) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/health/live", hc.LivenessHandler)
	mux.HandleFunc("/health/ready", hc.ReadinessHandler)

	addr := fmt.Sprintf(":%d", port)
	logger.Info("Starting health check server", zap.String("address", addr))

	server := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	return server.ListenAndServe()
}
