package diskmanager

import (
	"fmt"
	"sync"
	"syscall"
	"time"

	"go.uber.org/zap"
)

// DiskManager monitors disk space and enforces write policies
type DiskManager struct {
	dataDir              string
	logger               *zap.Logger
	mu                   sync.RWMutex
	lastCheck            time.Time
	cachedUsagePercent   float64
	cachedAvailableBytes uint64
	checkInterval        time.Duration

	// Thresholds
	warningThreshold     float64 // Start warning at this percentage (e.g., 80%)
	throttleThreshold    float64 // Throttle writes at this percentage (e.g., 90%)
	circuitBreakerThreshold float64 // Stop all writes at this percentage (e.g., 95%)

	// State
	isThrottled          bool
	isCircuitBroken      bool
}

// DiskManagerConfig holds configuration for disk manager
type DiskManagerConfig struct {
	DataDir                 string
	CheckInterval           time.Duration
	WarningThreshold        float64
	ThrottleThreshold       float64
	CircuitBreakerThreshold float64
}

// NewDiskManager creates a new disk manager with specified thresholds
func NewDiskManager(cfg *DiskManagerConfig, logger *zap.Logger) (*DiskManager, error) {
	if cfg.DataDir == "" {
		return nil, fmt.Errorf("data directory is required")
	}

	dm := &DiskManager{
		dataDir:                 cfg.DataDir,
		logger:                  logger,
		checkInterval:           cfg.CheckInterval,
		warningThreshold:        cfg.WarningThreshold,
		throttleThreshold:       cfg.ThrottleThreshold,
		circuitBreakerThreshold: cfg.CircuitBreakerThreshold,
	}

	// Perform initial check
	if err := dm.checkDiskSpace(); err != nil {
		logger.Warn("Initial disk space check failed", zap.Error(err))
	}

	return dm, nil
}

// DefaultConfig returns default disk manager configuration
func DefaultConfig(dataDir string) *DiskManagerConfig {
	return &DiskManagerConfig{
		DataDir:                 dataDir,
		CheckInterval:           10 * time.Second,
		WarningThreshold:        80.0,
		ThrottleThreshold:       90.0,
		CircuitBreakerThreshold: 95.0,
	}
}

// CheckBeforeWrite checks if a write of the given size can proceed
// Returns an error if write should be rejected
func (dm *DiskManager) CheckBeforeWrite(estimatedBytes uint64) error {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	// Refresh cache if stale
	if time.Since(dm.lastCheck) > dm.checkInterval {
		dm.mu.RUnlock()
		dm.mu.Lock()
		if err := dm.checkDiskSpace(); err != nil {
			dm.logger.Warn("Disk space check failed", zap.Error(err))
		}
		dm.mu.Unlock()
		dm.mu.RLock()
	}

	// Check circuit breaker
	if dm.isCircuitBroken {
		return &DiskSpaceError{
			Code:             ErrCodeDiskFull,
			Message:          fmt.Sprintf("disk usage at %.2f%%, circuit breaker engaged", dm.cachedUsagePercent),
			UsagePercent:     dm.cachedUsagePercent,
			AvailableBytes:   dm.cachedAvailableBytes,
			IsCircuitBroken:  true,
		}
	}

	// Check if throttled
	if dm.isThrottled {
		// Allow small writes during throttling, reject large ones
		if estimatedBytes > dm.cachedAvailableBytes/10 {
			return &DiskSpaceError{
				Code:            ErrCodeDiskThrottled,
				Message:         fmt.Sprintf("disk usage at %.2f%%, write throttled", dm.cachedUsagePercent),
				UsagePercent:    dm.cachedUsagePercent,
				AvailableBytes:  dm.cachedAvailableBytes,
				IsThrottled:     true,
			}
		}
	}

	// Check if requested write would fit
	if estimatedBytes > dm.cachedAvailableBytes {
		return &DiskSpaceError{
			Code:            ErrCodeInsufficientSpace,
			Message:         fmt.Sprintf("insufficient space: need %d bytes, have %d bytes", estimatedBytes, dm.cachedAvailableBytes),
			UsagePercent:    dm.cachedUsagePercent,
			AvailableBytes:  dm.cachedAvailableBytes,
		}
	}

	return nil
}

// checkDiskSpace checks current disk usage and updates state
// Must be called with write lock held
func (dm *DiskManager) checkDiskSpace() error {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(dm.dataDir, &stat); err != nil {
		return fmt.Errorf("failed to stat filesystem: %w", err)
	}

	// Calculate usage
	totalBytes := stat.Blocks * uint64(stat.Bsize)
	availableBytes := stat.Bavail * uint64(stat.Bsize)
	usedBytes := totalBytes - availableBytes
	usagePercent := (float64(usedBytes) / float64(totalBytes)) * 100.0

	// Update cache
	dm.cachedUsagePercent = usagePercent
	dm.cachedAvailableBytes = availableBytes
	dm.lastCheck = time.Now()

	// Update state based on thresholds
	previouslyThrottled := dm.isThrottled
	previouslyBroken := dm.isCircuitBroken

	dm.isCircuitBroken = usagePercent >= dm.circuitBreakerThreshold
	dm.isThrottled = usagePercent >= dm.throttleThreshold && !dm.isCircuitBroken

	// Log state changes
	if dm.isCircuitBroken && !previouslyBroken {
		dm.logger.Error("Disk circuit breaker ENGAGED",
			zap.Float64("usage_percent", usagePercent),
			zap.Uint64("available_bytes", availableBytes),
			zap.Float64("threshold", dm.circuitBreakerThreshold))
	} else if !dm.isCircuitBroken && previouslyBroken {
		dm.logger.Info("Disk circuit breaker DISENGAGED",
			zap.Float64("usage_percent", usagePercent),
			zap.Uint64("available_bytes", availableBytes))
	}

	if dm.isThrottled && !previouslyThrottled && !dm.isCircuitBroken {
		dm.logger.Warn("Disk write throttling ENABLED",
			zap.Float64("usage_percent", usagePercent),
			zap.Uint64("available_bytes", availableBytes),
			zap.Float64("threshold", dm.throttleThreshold))
	} else if !dm.isThrottled && previouslyThrottled {
		dm.logger.Info("Disk write throttling DISABLED",
			zap.Float64("usage_percent", usagePercent),
			zap.Uint64("available_bytes", availableBytes))
	}

	if usagePercent >= dm.warningThreshold && !dm.isThrottled && !dm.isCircuitBroken {
		dm.logger.Warn("Disk usage warning",
			zap.Float64("usage_percent", usagePercent),
			zap.Uint64("available_bytes", availableBytes),
			zap.Float64("warning_threshold", dm.warningThreshold))
	}

	return nil
}

// GetDiskUsage returns current disk usage statistics
func (dm *DiskManager) GetDiskUsage() DiskUsageStats {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	// Refresh if stale
	if time.Since(dm.lastCheck) > dm.checkInterval {
		dm.mu.RUnlock()
		dm.mu.Lock()
		dm.checkDiskSpace()
		dm.mu.Unlock()
		dm.mu.RLock()
	}

	return DiskUsageStats{
		UsagePercent:    dm.cachedUsagePercent,
		AvailableBytes:  dm.cachedAvailableBytes,
		IsThrottled:     dm.isThrottled,
		IsCircuitBroken: dm.isCircuitBroken,
		LastCheck:       dm.lastCheck,
	}
}

// ForceCheck forces an immediate disk space check
func (dm *DiskManager) ForceCheck() error {
	dm.mu.Lock()
	defer dm.mu.Unlock()
	return dm.checkDiskSpace()
}

// DiskUsageStats contains disk usage statistics
type DiskUsageStats struct {
	UsagePercent    float64
	AvailableBytes  uint64
	IsThrottled     bool
	IsCircuitBroken bool
	LastCheck       time.Time
}

// Error codes for disk space errors
type ErrorCode int

const (
	ErrCodeDiskFull ErrorCode = iota + 1
	ErrCodeDiskThrottled
	ErrCodeInsufficientSpace
)

// DiskSpaceError represents a disk space related error
type DiskSpaceError struct {
	Code             ErrorCode
	Message          string
	UsagePercent     float64
	AvailableBytes   uint64
	IsThrottled      bool
	IsCircuitBroken  bool
}

func (e *DiskSpaceError) Error() string {
	return e.Message
}

// IsDiskSpaceError checks if an error is a disk space error
func IsDiskSpaceError(err error) bool {
	_, ok := err.(*DiskSpaceError)
	return ok
}

// IsCircuitBroken checks if the error indicates circuit breaker is engaged
func IsCircuitBroken(err error) bool {
	if dse, ok := err.(*DiskSpaceError); ok {
		return dse.IsCircuitBroken
	}
	return false
}
