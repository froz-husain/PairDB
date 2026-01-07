package server

import (
	"context"
	"fmt"
	"net/http"
	"runtime"
	"syscall"
	"time"

	"github.com/devrev/pairdb/storage-node/internal/metrics"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
)

// MetricsServer serves Prometheus metrics via HTTP
type MetricsServer struct {
	httpServer *http.Server
	metrics    *metrics.Metrics
	logger     *zap.Logger
	dataDir    string
	stopChan   chan struct{}
}

// MetricsServerConfig holds configuration for the metrics server
type MetricsServerConfig struct {
	Port    int
	DataDir string
}

// NewMetricsServer creates a new metrics server
func NewMetricsServer(cfg *MetricsServerConfig, m *metrics.Metrics, logger *zap.Logger) *MetricsServer {
	mux := http.NewServeMux()

	ms := &MetricsServer{
		httpServer: &http.Server{
			Addr:         fmt.Sprintf(":%d", cfg.Port),
			Handler:      mux,
			ReadTimeout:  5 * time.Second,
			WriteTimeout: 10 * time.Second,
			IdleTimeout:  60 * time.Second,
		},
		metrics:  m,
		logger:   logger,
		dataDir:  cfg.DataDir,
		stopChan: make(chan struct{}),
	}

	// Register Prometheus metrics handler
	mux.Handle("/metrics", promhttp.Handler())

	// Register health check endpoint
	mux.HandleFunc("/health", ms.healthHandler)

	// Register readiness endpoint
	mux.HandleFunc("/ready", ms.readyHandler)

	return ms
}

// Start starts the metrics server
func (s *MetricsServer) Start() error {
	s.logger.Info("Starting metrics server", zap.String("addr", s.httpServer.Addr))

	// Start system metrics collector
	go s.collectSystemMetrics()

	// Start HTTP server
	go func() {
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Error("Metrics server failed", zap.Error(err))
		}
	}()

	return nil
}

// Stop gracefully stops the metrics server
func (s *MetricsServer) Stop() error {
	s.logger.Info("Stopping metrics server")

	close(s.stopChan)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := s.httpServer.Shutdown(ctx); err != nil {
		return fmt.Errorf("metrics server shutdown failed: %w", err)
	}

	return nil
}

// healthHandler handles health check requests
func (s *MetricsServer) healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"status":"healthy","timestamp":"%s"}`, time.Now().Format(time.RFC3339))
}

// readyHandler handles readiness check requests
func (s *MetricsServer) readyHandler(w http.ResponseWriter, r *http.Request) {
	// Check disk space
	diskUsage, diskAvailable, err := s.getDiskStats()
	if err != nil {
		s.logger.Error("Failed to get disk stats", zap.Error(err))
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprintf(w, `{"status":"not_ready","reason":"disk_stats_unavailable"}`)
		return
	}

	// Check if disk usage is above 90%
	diskUsagePercent := float64(diskUsage) / float64(diskUsage+diskAvailable) * 100
	if diskUsagePercent > 90.0 {
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprintf(w, `{"status":"not_ready","reason":"disk_full","disk_usage_percent":%.2f}`, diskUsagePercent)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"status":"ready","timestamp":"%s","disk_usage_percent":%.2f}`,
		time.Now().Format(time.RFC3339), diskUsagePercent)
}

// collectSystemMetrics periodically collects system-level metrics
func (s *MetricsServer) collectSystemMetrics() {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.updateSystemMetrics()
		case <-s.stopChan:
			return
		}
	}
}

// updateSystemMetrics updates system-level metrics
func (s *MetricsServer) updateSystemMetrics() {
	// Get disk stats
	diskUsage, diskAvailable, err := s.getDiskStats()
	if err != nil {
		s.logger.Error("Failed to get disk stats", zap.Error(err))
	}

	// Get memory stats
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	// Get goroutine count
	goroutines := runtime.NumGoroutine()

	// Update metrics
	s.metrics.UpdateSystemStats(diskUsage, diskAvailable, int64(memStats.Alloc), goroutines)
}

// getDiskStats returns disk usage statistics for the data directory
func (s *MetricsServer) getDiskStats() (used int64, available int64, err error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(s.dataDir, &stat); err != nil {
		return 0, 0, fmt.Errorf("failed to stat filesystem: %w", err)
	}

	// Calculate available and used space
	available = int64(stat.Bavail) * int64(stat.Bsize)
	total := int64(stat.Blocks) * int64(stat.Bsize)
	used = total - int64(stat.Bfree)*int64(stat.Bsize)

	return used, available, nil
}
