package service

import (
	"context"
	"fmt"
	"time"

	"github.com/devrev/pairdb/storage-node/internal/model"
	"go.uber.org/zap"
)

// StorageService is the main orchestration layer for storage operations
type StorageService struct {
	commitLogService   *CommitLogService
	memTableService    *MemTableService
	sstableService     *SSTableService
	cacheService       *CacheService
	vectorClockService *VectorClockService
	logger             *zap.Logger
	nodeID             string
}

// NewStorageService creates a new storage service
func NewStorageService(
	commitLogSvc *CommitLogService,
	memTableSvc *MemTableService,
	sstableSvc *SSTableService,
	cacheSvc *CacheService,
	vectorClockSvc *VectorClockService,
	logger *zap.Logger,
	nodeID string,
) *StorageService {
	return &StorageService{
		commitLogService:   commitLogSvc,
		memTableService:    memTableSvc,
		sstableService:     sstableSvc,
		cacheService:       cacheSvc,
		vectorClockService: vectorClockSvc,
		logger:             logger,
		nodeID:             nodeID,
	}
}

// Write handles write operations
func (s *StorageService) Write(
	ctx context.Context,
	tenantID string,
	key string,
	value []byte,
	vectorClock model.VectorClock,
) (*WriteResponse, error) {
	startTime := time.Now()

	// Validate inputs
	if err := s.validateWrite(tenantID, key, value); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	// Create commit log entry
	entry := &model.CommitLogEntry{
		TenantID:      tenantID,
		Key:           key,
		Value:         value,
		VectorClock:   vectorClock,
		Timestamp:     time.Now().Unix(),
		OperationType: model.OperationTypeWrite,
	}

	// Write to commit log (durability)
	if err := s.commitLogService.Append(ctx, entry); err != nil {
		s.logger.Error("Failed to write to commit log",
			zap.String("tenant_id", tenantID),
			zap.String("key", key),
			zap.Error(err))
		return nil, fmt.Errorf("commit log write failed: %w", err)
	}

	// Write to memtable
	memTableKey := s.buildKey(tenantID, key)
	memEntry := &model.MemTableEntry{
		Key:         memTableKey,
		Value:       value,
		VectorClock: vectorClock,
		Timestamp:   entry.Timestamp,
	}

	if err := s.memTableService.Put(ctx, memEntry); err != nil {
		s.logger.Error("Failed to write to memtable",
			zap.String("key", memTableKey),
			zap.Error(err))
		return nil, fmt.Errorf("memtable write failed: %w", err)
	}

	// Update cache
	s.cacheService.Put(memTableKey, value, vectorClock)

	// Check if memtable needs flushing
	if s.memTableService.ShouldFlush() {
		go s.triggerFlush()
	}

	latency := time.Since(startTime)
	s.logger.Debug("Write completed",
		zap.String("tenant_id", tenantID),
		zap.String("key", key),
		zap.Duration("latency", latency))

	return &WriteResponse{
		Success:     true,
		VectorClock: vectorClock,
	}, nil
}

// Read handles read operations
func (s *StorageService) Read(
	ctx context.Context,
	tenantID string,
	key string,
) (*ReadResponse, error) {
	startTime := time.Now()
	memTableKey := s.buildKey(tenantID, key)

	// Check cache first
	if entry, found := s.cacheService.Get(memTableKey); found {
		s.logger.Debug("Cache hit",
			zap.String("tenant_id", tenantID),
			zap.String("key", key))

		return &ReadResponse{
			Success:     true,
			Value:       entry.Value,
			VectorClock: entry.VectorClock,
			Source:      "cache",
		}, nil
	}

	// Check memtable
	if entry, found := s.memTableService.Get(ctx, memTableKey); found {
		s.logger.Debug("MemTable hit",
			zap.String("tenant_id", tenantID),
			zap.String("key", key))

		// Update cache
		s.cacheService.Put(memTableKey, entry.Value, entry.VectorClock)

		return &ReadResponse{
			Success:     true,
			Value:       entry.Value,
			VectorClock: entry.VectorClock,
			Source:      "memtable",
		}, nil
	}

	// Search SSTables
	entry, err := s.sstableService.Get(ctx, tenantID, key)
	if err != nil {
		s.logger.Error("SSTable read failed",
			zap.String("tenant_id", tenantID),
			zap.String("key", key),
			zap.Error(err))
		return nil, fmt.Errorf("sstable read failed: %w", err)
	}

	if entry == nil {
		return nil, fmt.Errorf("key not found")
	}

	// Update cache
	s.cacheService.Put(memTableKey, entry.Value, entry.VectorClock)

	latency := time.Since(startTime)
	s.logger.Debug("Read completed",
		zap.String("tenant_id", tenantID),
		zap.String("key", key),
		zap.String("source", "sstable"),
		zap.Duration("latency", latency))

	return &ReadResponse{
		Success:     true,
		Value:       entry.Value,
		VectorClock: entry.VectorClock,
		Source:      "sstable",
	}, nil
}

// Repair handles repair operations
func (s *StorageService) Repair(
	ctx context.Context,
	tenantID string,
	key string,
	value []byte,
	vectorClock model.VectorClock,
) error {
	// Create repair entry
	entry := &model.CommitLogEntry{
		TenantID:      tenantID,
		Key:           key,
		Value:         value,
		VectorClock:   vectorClock,
		Timestamp:     time.Now().Unix(),
		OperationType: model.OperationTypeRepair,
	}

	// Write to commit log
	if err := s.commitLogService.Append(ctx, entry); err != nil {
		return fmt.Errorf("repair commit log failed: %w", err)
	}

	// Update memtable
	memTableKey := s.buildKey(tenantID, key)
	memEntry := &model.MemTableEntry{
		Key:         memTableKey,
		Value:       value,
		VectorClock: vectorClock,
		Timestamp:   entry.Timestamp,
	}

	if err := s.memTableService.Put(ctx, memEntry); err != nil {
		return fmt.Errorf("repair memtable failed: %w", err)
	}

	// Update cache
	s.cacheService.Put(memTableKey, value, vectorClock)

	s.logger.Info("Repair completed",
		zap.String("tenant_id", tenantID),
		zap.String("key", key))

	return nil
}

// triggerFlush triggers memtable flush to SSTable
func (s *StorageService) triggerFlush() {
	ctx := context.Background()

	s.logger.Info("Triggering memtable flush")

	if err := s.memTableService.Flush(ctx, s.sstableService); err != nil {
		s.logger.Error("Memtable flush failed", zap.Error(err))
	}
}

// validateWrite validates write parameters
func (s *StorageService) validateWrite(tenantID, key string, value []byte) error {
	if tenantID == "" {
		return fmt.Errorf("tenant_id is required")
	}
	if key == "" {
		return fmt.Errorf("key is required")
	}
	if value == nil {
		return fmt.Errorf("value is required")
	}
	return nil
}

// buildKey creates composite key
func (s *StorageService) buildKey(tenantID, key string) string {
	return fmt.Sprintf("%s:%s", tenantID, key)
}

// WriteResponse represents the response from a write operation
type WriteResponse struct {
	Success     bool
	VectorClock model.VectorClock
}

// ReadResponse represents the response from a read operation
type ReadResponse struct {
	Success     bool
	Value       []byte
	VectorClock model.VectorClock
	Source      string
}
