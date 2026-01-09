package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/devrev/pairdb/storage-node/internal/model"
	"go.uber.org/zap"
)

// StreamState represents the state of a streaming context
type StreamState string

const (
	StreamStateCopying   StreamState = "copying"   // Bulk copying historical data
	StreamStateStreaming StreamState = "streaming" // Live streaming new writes
	StreamStateSyncing   StreamState = "syncing"   // Verifying sync with checksums
	StreamStateCompleted StreamState = "completed" // Streaming finished
	StreamStateFailed    StreamState = "failed"    // Streaming failed
)

// KeyRange represents a range of keys to stream (based on hash values)
type KeyRange struct {
	StartHash uint64
	EndHash   uint64
}

// StreamContext tracks the state of streaming to a target node
type StreamContext struct {
	TargetNodeID string
	TargetHost   string
	TargetPort   int
	KeyRanges    []KeyRange
	State        StreamState

	// Metrics
	KeysCopied     int64
	KeysStreamed   int64
	BytesCopied    int64
	BytesStreamed  int64
	LastSyncTime   time.Time
	StartTime      time.Time

	mu sync.RWMutex
}

// GetState returns the current state (thread-safe)
func (sc *StreamContext) GetState() StreamState {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return sc.State
}

// SetState sets the state (thread-safe)
func (sc *StreamContext) SetState(state StreamState) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	sc.State = state
}

// IncrementKeysCopied atomically increments the keys copied counter
func (sc *StreamContext) IncrementKeysCopied() {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	sc.KeysCopied++
}

// IncrementKeysStreamed atomically increments the keys streamed counter
func (sc *StreamContext) IncrementKeysStreamed() {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	sc.KeysStreamed++
}

// StreamingManager manages data streaming to other storage nodes
// Used for node bootstrapping and removal
type StreamingManager struct {
	storageService *StorageService
	logger         *zap.Logger

	// Track active streams to other nodes
	activeStreams map[string]*StreamContext // targetNodeID -> StreamContext
	mu            sync.RWMutex

	// Configuration
	batchSize       int           // Keys per batch during copy
	streamBuffer    int           // Buffer size for live streaming
	checksumWorkers int           // Parallel checksum workers
	stopCh          chan struct{} // Signal to stop streaming
}

// NewStreamingManager creates a new streaming manager
func NewStreamingManager(
	storageSvc *StorageService,
	batchSize int,
	streamBuffer int,
	checksumWorkers int,
	logger *zap.Logger,
) *StreamingManager {
	if batchSize <= 0 {
		batchSize = 1000 // Default: 1000 keys per batch
	}
	if streamBuffer <= 0 {
		streamBuffer = 10000 // Default: 10k buffered writes
	}
	if checksumWorkers <= 0 {
		checksumWorkers = 4 // Default: 4 parallel checksum workers
	}

	return &StreamingManager{
		storageService:  storageSvc,
		logger:          logger,
		activeStreams:   make(map[string]*StreamContext),
		batchSize:       batchSize,
		streamBuffer:    streamBuffer,
		checksumWorkers: checksumWorkers,
		stopCh:          make(chan struct{}),
	}
}

// AddStream adds a new streaming context for a target node
func (sm *StreamingManager) AddStream(ctx context.Context, targetNodeID string, streamCtx *StreamContext) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if _, exists := sm.activeStreams[targetNodeID]; exists {
		return fmt.Errorf("stream already exists for node %s", targetNodeID)
	}

	streamCtx.StartTime = time.Now()
	sm.activeStreams[targetNodeID] = streamCtx

	sm.logger.Info("Added streaming context",
		zap.String("target_node", targetNodeID),
		zap.Int("key_ranges", len(streamCtx.KeyRanges)))

	return nil
}

// RemoveStream removes a streaming context
func (sm *StreamingManager) RemoveStream(targetNodeID string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	delete(sm.activeStreams, targetNodeID)

	sm.logger.Info("Removed streaming context",
		zap.String("target_node", targetNodeID))
}

// GetStream retrieves a streaming context
func (sm *StreamingManager) GetStream(targetNodeID string) (*StreamContext, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	streamCtx, exists := sm.activeStreams[targetNodeID]
	return streamCtx, exists
}

// GetActiveStreams returns all active streaming contexts
func (sm *StreamingManager) GetActiveStreams() []*StreamContext {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	streams := make([]*StreamContext, 0, len(sm.activeStreams))
	for _, ctx := range sm.activeStreams {
		streams = append(streams, ctx)
	}
	return streams
}

// ExecuteStreaming performs the full streaming lifecycle:
// 1. Bulk copy phase (historical data)
// 2. Live streaming phase (new writes)
// 3. Sync verification phase (checksum comparison)
func (sm *StreamingManager) ExecuteStreaming(ctx context.Context, streamCtx *StreamContext) error {
	sm.logger.Info("Starting streaming execution",
		zap.String("target_node", streamCtx.TargetNodeID),
		zap.Int("key_ranges", len(streamCtx.KeyRanges)))

	// Phase 1: Bulk Copy
	streamCtx.SetState(StreamStateCopying)
	if err := sm.bulkCopyPhase(ctx, streamCtx); err != nil {
		streamCtx.SetState(StreamStateFailed)
		return fmt.Errorf("bulk copy failed: %w", err)
	}

	// Phase 2: Live Streaming (already happening in background via interceptor)
	streamCtx.SetState(StreamStateStreaming)
	sm.logger.Info("Bulk copy complete, live streaming active",
		zap.String("target_node", streamCtx.TargetNodeID),
		zap.Int64("keys_copied", streamCtx.KeysCopied))

	// Phase 3: Sync Verification
	streamCtx.SetState(StreamStateSyncing)
	if err := sm.syncVerificationPhase(ctx, streamCtx); err != nil {
		streamCtx.SetState(StreamStateFailed)
		return fmt.Errorf("sync verification failed: %w", err)
	}

	// Streaming complete
	streamCtx.SetState(StreamStateCompleted)
	sm.logger.Info("Streaming completed successfully",
		zap.String("target_node", streamCtx.TargetNodeID),
		zap.Int64("keys_copied", streamCtx.KeysCopied),
		zap.Int64("keys_streamed", streamCtx.KeysStreamed),
		zap.Duration("duration", time.Since(streamCtx.StartTime)))

	return nil
}

// bulkCopyPhase performs the initial bulk copy of historical data
func (sm *StreamingManager) bulkCopyPhase(ctx context.Context, streamCtx *StreamContext) error {
	sm.logger.Info("Starting bulk copy phase",
		zap.String("target_node", streamCtx.TargetNodeID))

	for _, keyRange := range streamCtx.KeyRanges {
		// Scan local storage for keys in this hash range
		keys, err := sm.scanKeysInRange(ctx, keyRange)
		if err != nil {
			return fmt.Errorf("failed to scan keys in range: %w", err)
		}

		sm.logger.Debug("Scanning key range",
			zap.String("target_node", streamCtx.TargetNodeID),
			zap.Uint64("start_hash", keyRange.StartHash),
			zap.Uint64("end_hash", keyRange.EndHash),
			zap.Int("keys_found", len(keys)))

		// Copy keys in batches
		for i := 0; i < len(keys); i += sm.batchSize {
			end := i + sm.batchSize
			if end > len(keys) {
				end = len(keys)
			}

			batch := keys[i:end]
			if err := sm.copyBatch(ctx, streamCtx, batch); err != nil {
				sm.logger.Error("Failed to copy batch",
					zap.String("target_node", streamCtx.TargetNodeID),
					zap.Int("batch_size", len(batch)),
					zap.Error(err))
				// Continue with next batch (best effort)
				continue
			}
		}
	}

	sm.logger.Info("Bulk copy phase completed",
		zap.String("target_node", streamCtx.TargetNodeID),
		zap.Int64("keys_copied", streamCtx.KeysCopied),
		zap.Int64("bytes_copied", streamCtx.BytesCopied))

	return nil
}

// scanKeysInRange scans the storage for keys in the given hash range
func (sm *StreamingManager) scanKeysInRange(ctx context.Context, keyRange KeyRange) ([]string, error) {
	// In production, this would:
	// 1. Scan memtable for keys with hash in [startHash, endHash)
	// 2. Scan all SSTables for keys in range
	// 3. Merge and deduplicate results
	// 4. Return sorted list of keys

	// For now, return placeholder
	// This will be fully implemented once we have the memtable and sstable scanning APIs
	keys := []string{}

	sm.logger.Debug("Scanning key range (placeholder implementation)",
		zap.Uint64("start_hash", keyRange.StartHash),
		zap.Uint64("end_hash", keyRange.EndHash))

	return keys, nil
}

// copyBatch copies a batch of keys to the target node
func (sm *StreamingManager) copyBatch(ctx context.Context, streamCtx *StreamContext, keys []string) error {
	for _, key := range keys {
		// Parse tenant ID and key from composite key format (tenantID:key)
		tenantID, actualKey, err := sm.parseCompositeKey(key)
		if err != nil {
			sm.logger.Warn("Failed to parse composite key",
				zap.String("key", key),
				zap.Error(err))
			continue
		}

		// Read value and vector clock from local storage
		resp, err := sm.storageService.Read(ctx, tenantID, actualKey)
		if err != nil {
			sm.logger.Warn("Failed to read key for copy",
				zap.String("key", key),
				zap.Error(err))
			continue
		}

		// Send to target node
		if err := sm.sendToTargetNode(ctx, streamCtx, tenantID, actualKey, resp.Value, resp.VectorClock, false); err != nil {
			sm.logger.Warn("Failed to send key to target node",
				zap.String("key", key),
				zap.String("target_node", streamCtx.TargetNodeID),
				zap.Error(err))
			continue
		}

		// Update metrics
		streamCtx.IncrementKeysCopied()
		streamCtx.mu.Lock()
		streamCtx.BytesCopied += int64(len(resp.Value))
		streamCtx.mu.Unlock()
	}

	return nil
}

// sendToTargetNode sends a key-value pair to the target node
func (sm *StreamingManager) sendToTargetNode(
	ctx context.Context,
	streamCtx *StreamContext,
	tenantID, key string,
	value []byte,
	vectorClock model.VectorClock,
	isLiveWrite bool,
) error {
	// In production, this would:
	// 1. Create gRPC client connection to target node
	// 2. Call Write RPC with (tenantID, key, value, vectorClock)
	// 3. Mark write as "copy" vs "live" for conflict resolution
	// 4. Handle errors and retries

	// For now, just log
	sm.logger.Debug("Sending key to target node",
		zap.String("target_node", streamCtx.TargetNodeID),
		zap.String("tenant_id", tenantID),
		zap.String("key", key),
		zap.Bool("is_live_write", isLiveWrite),
		zap.Int("value_size", len(value)))

	return nil
}

// InterceptWrite is called during write operations to stream to target nodes
// This implements the "live streaming" phase
func (sm *StreamingManager) InterceptWrite(
	ctx context.Context,
	tenantID, key string,
	value []byte,
	vectorClock model.VectorClock,
	keyHash uint64,
) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	// Check if this key should be streamed to any target node
	for _, streamCtx := range sm.activeStreams {
		// Only stream during StreamStateStreaming phase
		if streamCtx.GetState() != StreamStateStreaming {
			continue
		}

		// Check if this key falls in the ranges we're streaming
		if sm.keyInRanges(keyHash, streamCtx.KeyRanges) {
			// Stream to target node (async, best effort)
			go func(ctx *StreamContext) {
				err := sm.sendToTargetNode(context.Background(), ctx, tenantID, key, value, vectorClock, true)
				if err != nil {
					sm.logger.Debug("Failed to stream live write",
						zap.String("key", key),
						zap.String("target", ctx.TargetNodeID),
						zap.Error(err))
				} else {
					ctx.IncrementKeysStreamed()
					ctx.mu.Lock()
					ctx.BytesStreamed += int64(len(value))
					ctx.mu.Unlock()
				}
			}(streamCtx)
		}
	}
}

// keyInRanges checks if a key hash falls within any of the specified ranges
func (sm *StreamingManager) keyInRanges(keyHash uint64, ranges []KeyRange) bool {
	for _, r := range ranges {
		// Check if keyHash is in range [startHash, endHash)
		if keyHash >= r.StartHash && keyHash < r.EndHash {
			return true
		}

		// Handle wrap-around case (e.g., startHash=0xFFFFFFFFFFFFFF00, endHash=0x0000000000000100)
		if r.StartHash > r.EndHash {
			if keyHash >= r.StartHash || keyHash < r.EndHash {
				return true
			}
		}
	}
	return false
}

// syncVerificationPhase verifies that target node has all data
func (sm *StreamingManager) syncVerificationPhase(ctx context.Context, streamCtx *StreamContext) error {
	sm.logger.Info("Starting sync verification phase",
		zap.String("target_node", streamCtx.TargetNodeID))

	// For each key range, compare checksums between local and remote
	for _, keyRange := range streamCtx.KeyRanges {
		localChecksum, err := sm.computeRangeChecksum(ctx, keyRange)
		if err != nil {
			return fmt.Errorf("failed to compute local checksum: %w", err)
		}

		remoteChecksum, err := sm.getRemoteChecksum(ctx, streamCtx, keyRange)
		if err != nil {
			return fmt.Errorf("failed to get remote checksum: %w", err)
		}

		if localChecksum != remoteChecksum {
			sm.logger.Warn("Checksum mismatch, re-syncing range",
				zap.String("target_node", streamCtx.TargetNodeID),
				zap.Uint64("start_hash", keyRange.StartHash),
				zap.Uint64("end_hash", keyRange.EndHash),
				zap.String("local_checksum", localChecksum),
				zap.String("remote_checksum", remoteChecksum))

			// Re-sync this range
			if err := sm.reSyncRange(ctx, streamCtx, keyRange); err != nil {
				return fmt.Errorf("re-sync failed for range: %w", err)
			}
		}
	}

	streamCtx.mu.Lock()
	streamCtx.LastSyncTime = time.Now()
	streamCtx.mu.Unlock()

	sm.logger.Info("Sync verification completed",
		zap.String("target_node", streamCtx.TargetNodeID))

	return nil
}

// computeRangeChecksum computes a checksum for all keys in a range
func (sm *StreamingManager) computeRangeChecksum(ctx context.Context, keyRange KeyRange) (string, error) {
	// In production, this would:
	// 1. Scan all keys in the range
	// 2. Compute MD5/SHA256 of concatenated (key, value, vectorClock)
	// 3. Return hex-encoded checksum

	// Placeholder
	checksum := fmt.Sprintf("checksum_%d_%d", keyRange.StartHash, keyRange.EndHash)
	return checksum, nil
}

// getRemoteChecksum retrieves the checksum from the target node
func (sm *StreamingManager) getRemoteChecksum(ctx context.Context, streamCtx *StreamContext, keyRange KeyRange) (string, error) {
	// In production, this would:
	// 1. Call gRPC endpoint on target node: GetRangeChecksum(startHash, endHash)
	// 2. Return the checksum

	// Placeholder - assume same checksum (sync successful)
	return sm.computeRangeChecksum(ctx, keyRange)
}

// reSyncRange re-syncs a specific key range that had checksum mismatch
func (sm *StreamingManager) reSyncRange(ctx context.Context, streamCtx *StreamContext, keyRange KeyRange) error {
	sm.logger.Info("Re-syncing key range",
		zap.String("target_node", streamCtx.TargetNodeID),
		zap.Uint64("start_hash", keyRange.StartHash),
		zap.Uint64("end_hash", keyRange.EndHash))

	// Get keys in this range
	keys, err := sm.scanKeysInRange(ctx, keyRange)
	if err != nil {
		return fmt.Errorf("failed to scan keys for re-sync: %w", err)
	}

	// Re-copy all keys in this range
	if err := sm.copyBatch(ctx, streamCtx, keys); err != nil {
		return fmt.Errorf("failed to re-copy keys: %w", err)
	}

	return nil
}

// parseCompositeKey parses a composite key in format "tenantID:key"
func (sm *StreamingManager) parseCompositeKey(compositeKey string) (tenantID, key string, err error) {
	// Find the first colon
	for i := 0; i < len(compositeKey); i++ {
		if compositeKey[i] == ':' {
			return compositeKey[:i], compositeKey[i+1:], nil
		}
	}
	return "", "", fmt.Errorf("invalid composite key format: %s", compositeKey)
}

// Stop stops all streaming operations
func (sm *StreamingManager) Stop() {
	close(sm.stopCh)
	sm.logger.Info("Streaming manager stopped")
}

// GetMetrics returns metrics for all active streams
func (sm *StreamingManager) GetMetrics() map[string]StreamMetrics {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	metrics := make(map[string]StreamMetrics)
	for nodeID, streamCtx := range sm.activeStreams {
		streamCtx.mu.RLock()
		metrics[nodeID] = StreamMetrics{
			TargetNodeID:  streamCtx.TargetNodeID,
			State:         string(streamCtx.State),
			KeysCopied:    streamCtx.KeysCopied,
			KeysStreamed:  streamCtx.KeysStreamed,
			BytesCopied:   streamCtx.BytesCopied,
			BytesStreamed: streamCtx.BytesStreamed,
			Duration:      time.Since(streamCtx.StartTime).Seconds(),
		}
		streamCtx.mu.RUnlock()
	}

	return metrics
}

// StreamMetrics represents metrics for a streaming operation
type StreamMetrics struct {
	TargetNodeID  string  `json:"target_node_id"`
	State         string  `json:"state"`
	KeysCopied    int64   `json:"keys_copied"`
	KeysStreamed  int64   `json:"keys_streamed"`
	BytesCopied   int64   `json:"bytes_copied"`
	BytesStreamed int64   `json:"bytes_streamed"`
	Duration      float64 `json:"duration_seconds"`
}
