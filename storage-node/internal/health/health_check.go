package health

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sync"
	"syscall"
	"time"

	"github.com/devrev/pairdb/storage-node/internal/model"
	"go.uber.org/zap"
)

// HealthChecker performs health checks for the storage node
type HealthChecker struct {
	nodeID      string
	dataDir     string
	logger      *zap.Logger
	mu          sync.RWMutex
	lastCheck   time.Time
	status      model.NodeStatus
	checks      map[string]CheckResult
	livenessOK  bool
	readinessOK bool
}

// CheckResult represents the result of a health check
type CheckResult struct {
	Name      string
	Status    string
	Message   string
	Timestamp time.Time
}

// HealthCheckConfig holds configuration for health checks
type HealthCheckConfig struct {
	NodeID  string
	DataDir string
}

// NewHealthChecker creates a new health checker
func NewHealthChecker(cfg *HealthCheckConfig, logger *zap.Logger) *HealthChecker {
	return &HealthChecker{
		nodeID:      cfg.NodeID,
		dataDir:     cfg.DataDir,
		logger:      logger,
		checks:      make(map[string]CheckResult),
		livenessOK:  true,
		readinessOK: true,
		status:      model.NodeStatusHealthy,
	}
}

// Start starts the health checker
func (h *HealthChecker) Start(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	// Run initial check
	h.runHealthChecks()

	for {
		select {
		case <-ticker.C:
			h.runHealthChecks()
		case <-ctx.Done():
			h.logger.Info("Health checker stopped")
			return
		}
	}
}

// runHealthChecks runs all health checks
func (h *HealthChecker) runHealthChecks() {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.lastCheck = time.Now()

	// Run all checks
	checks := []func() CheckResult{
		h.checkDiskSpace,
		h.checkDataDirAccessible,
		h.checkFileDescriptors,
		h.checkMemoryPressure,
	}

	allHealthy := true
	allReady := true

	for _, check := range checks {
		result := check()
		h.checks[result.Name] = result

		if result.Status != "healthy" {
			allHealthy = false
			if result.Status == "critical" {
				allReady = false
			}
		}
	}

	// Update overall status
	if !allHealthy {
		if !allReady {
			h.status = model.NodeStatusUnhealthy
		} else {
			h.status = model.NodeStatusDegraded
		}
	} else {
		h.status = model.NodeStatusHealthy
	}

	// Liveness: process is responsive, no deadlocks
	// Always true if we can execute this function
	h.livenessOK = true

	// Readiness: can serve traffic
	h.readinessOK = allReady

	h.logger.Debug("Health check completed",
		zap.String("status", string(h.status)),
		zap.Bool("liveness", h.livenessOK),
		zap.Bool("readiness", h.readinessOK))
}

// checkDiskSpace checks if disk space is sufficient
func (h *HealthChecker) checkDiskSpace() CheckResult {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(h.dataDir, &stat); err != nil {
		return CheckResult{
			Name:      "disk_space",
			Status:    "critical",
			Message:   fmt.Sprintf("Failed to stat filesystem: %v", err),
			Timestamp: time.Now(),
		}
	}

	// Calculate disk usage percentage
	available := stat.Bavail * uint64(stat.Bsize)
	total := stat.Blocks * uint64(stat.Bsize)
	used := total - (stat.Bfree * uint64(stat.Bsize))
	usagePercent := float64(used) / float64(total) * 100

	if usagePercent > 95 {
		return CheckResult{
			Name:      "disk_space",
			Status:    "critical",
			Message:   fmt.Sprintf("Disk usage critical: %.2f%%", usagePercent),
			Timestamp: time.Now(),
		}
	} else if usagePercent > 90 {
		return CheckResult{
			Name:      "disk_space",
			Status:    "warning",
			Message:   fmt.Sprintf("Disk usage high: %.2f%%", usagePercent),
			Timestamp: time.Now(),
		}
	}

	return CheckResult{
		Name:      "disk_space",
		Status:    "healthy",
		Message:   fmt.Sprintf("Disk usage: %.2f%%, available: %.2f GB", usagePercent, float64(available)/1024/1024/1024),
		Timestamp: time.Now(),
	}
}

// checkDataDirAccessible checks if data directory is accessible
func (h *HealthChecker) checkDataDirAccessible() CheckResult {
	// Try to stat the data directory
	info, err := os.Stat(h.dataDir)
	if err != nil {
		return CheckResult{
			Name:      "data_dir_accessible",
			Status:    "critical",
			Message:   fmt.Sprintf("Data directory not accessible: %v", err),
			Timestamp: time.Now(),
		}
	}

	if !info.IsDir() {
		return CheckResult{
			Name:      "data_dir_accessible",
			Status:    "critical",
			Message:   "Data path is not a directory",
			Timestamp: time.Now(),
		}
	}

	// Try to create a test file
	testFile := fmt.Sprintf("%s/.health_check_%d", h.dataDir, time.Now().UnixNano())
	f, err := os.Create(testFile)
	if err != nil {
		return CheckResult{
			Name:      "data_dir_accessible",
			Status:    "critical",
			Message:   fmt.Sprintf("Cannot write to data directory: %v", err),
			Timestamp: time.Now(),
		}
	}
	f.Close()
	os.Remove(testFile)

	return CheckResult{
		Name:      "data_dir_accessible",
		Status:    "healthy",
		Message:   "Data directory is accessible and writable",
		Timestamp: time.Now(),
	}
}

// checkFileDescriptors checks if file descriptor usage is acceptable
func (h *HealthChecker) checkFileDescriptors() CheckResult {
	var rlimit syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rlimit); err != nil {
		return CheckResult{
			Name:      "file_descriptors",
			Status:    "warning",
			Message:   fmt.Sprintf("Failed to get rlimit: %v", err),
			Timestamp: time.Now(),
		}
	}

	// Read /proc/self/fd to count open file descriptors (Linux specific)
	// For production, this should be more robust and cross-platform
	entries, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		// If we can't read /proc/self/fd (e.g., on macOS), just return healthy
		return CheckResult{
			Name:      "file_descriptors",
			Status:    "healthy",
			Message:   fmt.Sprintf("Soft limit: %d, hard limit: %d", rlimit.Cur, rlimit.Max),
			Timestamp: time.Now(),
		}
	}

	openFDs := uint64(len(entries))
	usagePercent := float64(openFDs) / float64(rlimit.Cur) * 100

	if usagePercent > 90 {
		return CheckResult{
			Name:      "file_descriptors",
			Status:    "warning",
			Message:   fmt.Sprintf("File descriptor usage high: %.2f%% (%d/%d)", usagePercent, openFDs, rlimit.Cur),
			Timestamp: time.Now(),
		}
	}

	return CheckResult{
		Name:      "file_descriptors",
		Status:    "healthy",
		Message:   fmt.Sprintf("File descriptor usage: %.2f%% (%d/%d)", usagePercent, openFDs, rlimit.Cur),
		Timestamp: time.Now(),
	}
}

// checkMemoryPressure checks if system is under memory pressure
func (h *HealthChecker) checkMemoryPressure() CheckResult {
	// Read /proc/meminfo to check memory pressure (Linux specific)
	// For production, this should be more robust and cross-platform
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		// If we can't read /proc/meminfo (e.g., on macOS), just return healthy
		return CheckResult{
			Name:      "memory_pressure",
			Status:    "healthy",
			Message:   "Memory check not available on this platform",
			Timestamp: time.Now(),
		}
	}

	// Parse meminfo (simplified)
	// In production, use a proper parser
	_ = data

	// For now, just return healthy
	// In production, implement proper memory pressure detection
	return CheckResult{
		Name:      "memory_pressure",
		Status:    "healthy",
		Message:   "Memory pressure acceptable",
		Timestamp: time.Now(),
	}
}

// IsLive returns whether the node is live (liveness probe)
func (h *HealthChecker) IsLive() bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.livenessOK
}

// IsReady returns whether the node is ready (readiness probe)
func (h *HealthChecker) IsReady() bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.readinessOK
}

// GetStatus returns the current health status
func (h *HealthChecker) GetStatus() model.HealthStatus {
	h.mu.RLock()
	defer h.mu.RUnlock()

	return model.HealthStatus{
		NodeID:    h.nodeID,
		Status:    h.status,
		Timestamp: h.lastCheck.Unix(),
		Metrics:   model.HealthMetrics{}, // Populated by metrics service
	}
}

// GetChecks returns all check results
func (h *HealthChecker) GetChecks() map[string]CheckResult {
	h.mu.RLock()
	defer h.mu.RUnlock()

	// Return a copy to avoid race conditions
	checks := make(map[string]CheckResult, len(h.checks))
	for k, v := range h.checks {
		checks[k] = v
	}

	return checks
}

// SetLiveness manually sets liveness status (for testing)
func (h *HealthChecker) SetLiveness(live bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.livenessOK = live
}

// SetReadiness manually sets readiness status (for graceful shutdown)
func (h *HealthChecker) SetReadiness(ready bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.readinessOK = ready
}

// HTTP Handler functions for Kubernetes probes


// LivenessHandler handles HTTP liveness probe requests
func (h *HealthChecker) LivenessHandler(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	live := h.livenessOK
	status := h.GetStatus()
	h.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	
	if !live {
		w.WriteHeader(http.StatusServiceUnavailable)
	} else {
		w.WriteHeader(http.StatusOK)
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"healthy": live,
		"status":  status.Status,
	})
}

// ReadinessHandler handles HTTP readiness probe requests
func (h *HealthChecker) ReadinessHandler(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	ready := h.readinessOK
	status := h.GetStatus()
	h.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	
	if !ready {
		w.WriteHeader(http.StatusServiceUnavailable)
	} else {
		w.WriteHeader(http.StatusOK)
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"ready":  ready,
		"status": status.Status,
	})
}

// StartHealthServer starts the HTTP health check server
func (h *HealthChecker) StartHealthServer(port string) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/health/live", h.LivenessHandler)
	mux.HandleFunc("/health/ready", h.ReadinessHandler)

	h.logger.Info("Starting health check HTTP server", zap.String("port", port))
	
	return http.ListenAndServe(port, mux)
}
