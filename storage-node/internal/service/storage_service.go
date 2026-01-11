package service

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"time"

	"github.com/devrev/pairdb/storage-node/internal/errors"
	"github.com/devrev/pairdb/storage-node/internal/model"
	"github.com/devrev/pairdb/storage-node/internal/storage/diskmanager"
	"github.com/devrev/pairdb/storage-node/internal/util/workerpool"
	"github.com/devrev/pairdb/storage-node/internal/validation"
	"go.uber.org/zap"
)

// StorageService is the main orchestration layer for storage operations
type StorageService struct {
	commitLogService   *CommitLogService
	memTableService    *MemTableService
	sstableService     *SSTableService
	cacheService       *CacheService
	vectorClockService *VectorClockService
	streamingManager   *StreamingManager // NEW: For Phase 2 live streaming
	diskManager        *diskmanager.DiskManager
	validator          *validation.Validator
	workerPool         *workerpool.WorkerPool
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
	diskMgr *diskmanager.DiskManager,
	workerPool *workerpool.WorkerPool,
	logger *zap.Logger,
	nodeID string,
) *StorageService {
	return &StorageService{
		commitLogService:   commitLogSvc,
		memTableService:    memTableSvc,
		sstableService:     sstableSvc,
		cacheService:       cacheSvc,
		vectorClockService: vectorClockSvc,
		streamingManager:   nil, // Set later via SetStreamingManager()
		diskManager:        diskMgr,
		validator:          validation.NewValidator(),
		workerPool:         workerPool,
		logger:             logger,
		nodeID:             nodeID,
	}
}

// SetStreamingManager sets the streaming manager (called after initialization to avoid circular dependency)
func (s *StorageService) SetStreamingManager(streamingMgr *StreamingManager) {
	s.streamingManager = streamingMgr
}

// Write handles write operations with validation and disk space checking
func (s *StorageService) Write(
	ctx context.Context,
	tenantID string,
	key string,
	value []byte,
	vectorClock model.VectorClock,
) (*WriteResponse, error) {
	startTime := time.Now()

	// Validate inputs using comprehensive validator
	if err := s.validator.ValidateWrite(tenantID, key, value, vectorClock); err != nil {
		s.logger.Warn("Write validation failed",
			zap.String("tenant_id", tenantID),
			zap.String("key", key),
			zap.Error(err))
		return nil, err
	}

	// Check disk space before write
	estimatedSize := validation.EstimateWriteSize(tenantID, key, value)
	if err := s.diskManager.CheckBeforeWrite(estimatedSize); err != nil {
		s.logger.Warn("Disk space check failed",
			zap.String("tenant_id", tenantID),
			zap.String("key", key),
			zap.Uint64("estimated_size", estimatedSize),
			zap.Error(err))
		return nil, err
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
		return nil, errors.CommitLogFailed("failed to append to commit log", err)
	}

	// Write to memtable
	memTableKey := s.buildKey(tenantID, key)
	memEntry := &model.MemTableEntry{
		Key:         memTableKey,
		Value:       value,
		VectorClock: vectorClock,
		Timestamp:   entry.Timestamp,
		IsTombstone: false,
	}

	if err := s.memTableService.Put(ctx, memEntry); err != nil {
		s.logger.Error("Failed to write to memtable",
			zap.String("key", memTableKey),
			zap.Error(err))
		return nil, errors.MemTableFailed("failed to write to memtable", err)
	}

	// Update cache
	s.cacheService.Put(memTableKey, value, vectorClock)

	// NEW: Phase 2 - Intercept write for live streaming
	if s.streamingManager != nil {
		// Compute key hash for range checking
		keyHash := s.computeKeyHash(tenantID, key)

		// Notify streaming manager (async, non-blocking)
		s.streamingManager.InterceptWrite(
			ctx,
			tenantID,
			key,
			value,
			vectorClock,
			keyHash,
		)
	}

	// Check if memtable needs flushing - use worker pool to avoid goroutine leak
	if s.memTableService.ShouldFlush() {
		s.triggerFlushAsync(ctx)
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
		// Check if it's a tombstone
		if entry.IsTombstone {
			return nil, errors.KeyNotFound(tenantID, key)
		}

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
		return nil, errors.SSTableFailed("sstable read failed", err)
	}

	if entry == nil {
		return nil, errors.KeyNotFound(tenantID, key)
	}

	// Check if it's a tombstone
	if entry.IsTombstone {
		return nil, errors.KeyNotFound(tenantID, key)
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

// Delete handles delete operations using tombstones
func (s *StorageService) Delete(
	ctx context.Context,
	tenantID string,
	key string,
	vectorClock model.VectorClock,
) error {
	startTime := time.Now()

	// Validate inputs
	if err := s.validator.ValidateTenantID(tenantID); err != nil {
		return err
	}
	if err := s.validator.ValidateKey(key); err != nil {
		return err
	}

	// Create delete entry (tombstone)
	entry := &model.CommitLogEntry{
		TenantID:      tenantID,
		Key:           key,
		Value:         nil, // Tombstone has no value
		VectorClock:   vectorClock,
		Timestamp:     time.Now().Unix(),
		OperationType: model.OperationTypeDelete,
	}

	// Write to commit log
	if err := s.commitLogService.Append(ctx, entry); err != nil {
		s.logger.Error("Failed to write delete to commit log",
			zap.String("tenant_id", tenantID),
			zap.String("key", key),
			zap.Error(err))
		return errors.CommitLogFailed("failed to append delete to commit log", err)
	}

	// Write tombstone to memtable
	memTableKey := s.buildKey(tenantID, key)
	memEntry := &model.MemTableEntry{
		Key:         memTableKey,
		Value:       nil,
		VectorClock: vectorClock,
		Timestamp:   entry.Timestamp,
		IsTombstone: true,
	}

	if err := s.memTableService.Put(ctx, memEntry); err != nil {
		s.logger.Error("Failed to write tombstone to memtable",
			zap.String("key", memTableKey),
			zap.Error(err))
		return errors.MemTableFailed("failed to write tombstone", err)
	}

	// Remove from cache
	s.cacheService.Remove(memTableKey)

	latency := time.Since(startTime)
	s.logger.Debug("Delete completed",
		zap.String("tenant_id", tenantID),
		zap.String("key", key),
		zap.Duration("latency", latency))

	return nil
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
		return errors.CommitLogFailed("repair commit log failed", err)
	}

	// Update memtable
	memTableKey := s.buildKey(tenantID, key)
	memEntry := &model.MemTableEntry{
		Key:         memTableKey,
		Value:       value,
		VectorClock: vectorClock,
		Timestamp:   entry.Timestamp,
		IsTombstone: false,
	}

	if err := s.memTableService.Put(ctx, memEntry); err != nil {
		return errors.MemTableFailed("repair memtable failed", err)
	}

	// Update cache
	s.cacheService.Put(memTableKey, value, vectorClock)

	s.logger.Info("Repair completed",
		zap.String("tenant_id", tenantID),
		zap.String("key", key))

	return nil
}

// triggerFlushAsync triggers memtable flush using worker pool to avoid goroutine leak
func (s *StorageService) triggerFlushAsync(parentCtx context.Context) {
	// Use worker pool to bound goroutine creation
	task := workerpool.Task{
		ID: fmt.Sprintf("flush-%d", time.Now().UnixNano()),
		Fn: func(ctx context.Context) error {
			s.logger.Info("Triggering memtable flush")
			return s.memTableService.Flush(ctx, s.sstableService)
		},
		Context: parentCtx, // Propagate parent context properly
	}

	if !s.workerPool.TrySubmit(task) {
		s.logger.Warn("Failed to submit flush task to worker pool, executing inline")
		// Fallback: execute inline if worker pool is full
		if err := s.memTableService.Flush(parentCtx, s.sstableService); err != nil {
			s.logger.Error("Memtable flush failed", zap.Error(err))
		}
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

// computeKeyHash computes the hash of a key for consistent hashing
// This is used by the streaming manager to determine if a key should be streamed
// Uses SHA-256 to match coordinator's consistent hashing algorithm
func (s *StorageService) computeKeyHash(tenantID, key string) uint64 {
	// Create composite key
	compositeKey := s.buildKey(tenantID, key)

	// Use SHA-256 hash (same as consistent hashing in coordinator)
	// This must match the algorithm in coordinator/internal/algorithm/consistent_hash.go
	h := sha256.New()
	h.Write([]byte(compositeKey))
	hashBytes := h.Sum(nil)

	// Convert first 8 bytes to uint64
	return binary.BigEndian.Uint64(hashBytes[:8])
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
