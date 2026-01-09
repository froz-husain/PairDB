package service

import (
	"context"
	"sync"
	"time"

	"github.com/devrev/pairdb/coordinator/internal/client"
	"github.com/devrev/pairdb/coordinator/internal/model"
	"go.uber.org/zap"
)

// HintedHandoffService manages hints for temporarily unavailable nodes
// Following Cassandra's hinted handoff pattern for eventual consistency
type HintedHandoffService struct {
	storageClient *client.StorageClient
	logger        *zap.Logger

	// Store hints: nodeID -> list of hints
	hints     map[string][]*Hint
	mu        sync.RWMutex
	maxHints  int           // Max hints per node before dropping
	hintTTL   time.Duration // Time to keep hints before expiry
	replayInt time.Duration // Interval for replaying hints

	stopCh chan struct{}
}

// Hint represents a write that failed to reach a node
type Hint struct {
	NodeID      string
	TenantID    string
	Key         string
	Value       []byte
	VectorClock model.VectorClock
	Timestamp   time.Time
	Retries     int
}

// NewHintedHandoffService creates a new hinted handoff service
func NewHintedHandoffService(
	storageClient *client.StorageClient,
	maxHints int,
	hintTTL time.Duration,
	replayInterval time.Duration,
	logger *zap.Logger,
) *HintedHandoffService {
	if maxHints <= 0 {
		maxHints = 10000 // Default: 10k hints per node
	}
	if hintTTL == 0 {
		hintTTL = 3 * time.Hour // Default: 3 hours
	}
	if replayInterval == 0 {
		replayInterval = 10 * time.Second // Default: 10 seconds
	}

	return &HintedHandoffService{
		storageClient: storageClient,
		hints:         make(map[string][]*Hint),
		maxHints:      maxHints,
		hintTTL:       hintTTL,
		replayInt:     replayInterval,
		logger:        logger,
		stopCh:        make(chan struct{}),
	}
}

// Start begins the hint replay background process
func (h *HintedHandoffService) Start() {
	h.logger.Info("Starting hinted handoff service",
		zap.Int("max_hints_per_node", h.maxHints),
		zap.Duration("hint_ttl", h.hintTTL),
		zap.Duration("replay_interval", h.replayInt))

	ticker := time.NewTicker(h.replayInt)
	go func() {
		for {
			select {
			case <-ticker.C:
				h.replayAllHints()
			case <-h.stopCh:
				ticker.Stop()
				return
			}
		}
	}()
}

// Stop stops the hint replay process
func (h *HintedHandoffService) Stop() {
	close(h.stopCh)
	h.logger.Info("Hinted handoff service stopped")
}

// StoreHint stores a hint for a node that failed to receive a write
func (h *HintedHandoffService) StoreHint(nodeID, tenantID, key string, value []byte, vectorClock model.VectorClock) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Check if we've exceeded max hints for this node
	if len(h.hints[nodeID]) >= h.maxHints {
		h.logger.Warn("Max hints reached for node, dropping oldest hint",
			zap.String("node_id", nodeID),
			zap.Int("max_hints", h.maxHints))

		// Drop oldest hint
		h.hints[nodeID] = h.hints[nodeID][1:]
	}

	hint := &Hint{
		NodeID:      nodeID,
		TenantID:    tenantID,
		Key:         key,
		Value:       value,
		VectorClock: vectorClock,
		Timestamp:   time.Now(),
		Retries:     0,
	}

	h.hints[nodeID] = append(h.hints[nodeID], hint)

	h.logger.Debug("Hint stored",
		zap.String("node_id", nodeID),
		zap.String("tenant_id", tenantID),
		zap.String("key", key),
		zap.Int("total_hints_for_node", len(h.hints[nodeID])))
}

// replayAllHints attempts to replay hints for all nodes
func (h *HintedHandoffService) replayAllHints() {
	h.mu.RLock()
	nodeIDs := make([]string, 0, len(h.hints))
	for nodeID := range h.hints {
		nodeIDs = append(nodeIDs, nodeID)
	}
	h.mu.RUnlock()

	for _, nodeID := range nodeIDs {
		go h.replayHintsForNode(nodeID)
	}
}

// replayHintsForNode replays all hints for a specific node
func (h *HintedHandoffService) replayHintsForNode(nodeID string) {
	h.mu.RLock()
	nodeHints := h.hints[nodeID]
	if len(nodeHints) == 0 {
		h.mu.RUnlock()
		return
	}

	// Create a copy to avoid holding lock during replay
	hintsToReplay := make([]*Hint, len(nodeHints))
	copy(hintsToReplay, nodeHints)
	h.mu.RUnlock()

	h.logger.Debug("Replaying hints",
		zap.String("node_id", nodeID),
		zap.Int("hint_count", len(hintsToReplay)))

	successCount := 0
	expiredCount := 0
	failedCount := 0

	for _, hint := range hintsToReplay {
		// Check if hint has expired
		if time.Since(hint.Timestamp) > h.hintTTL {
			h.removeHint(nodeID, hint)
			expiredCount++
			h.logger.Debug("Hint expired",
				zap.String("node_id", nodeID),
				zap.String("key", hint.Key),
				zap.Duration("age", time.Since(hint.Timestamp)))
			continue
		}

		// Attempt to replay hint
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		err := h.replayHint(ctx, hint)
		cancel()

		if err == nil {
			// Success - remove hint
			h.removeHint(nodeID, hint)
			successCount++
			h.logger.Debug("Hint replayed successfully",
				zap.String("node_id", nodeID),
				zap.String("key", hint.Key))
		} else {
			// Failure - increment retry count
			hint.Retries++
			failedCount++
			h.logger.Debug("Hint replay failed",
				zap.String("node_id", nodeID),
				zap.String("key", hint.Key),
				zap.Int("retries", hint.Retries),
				zap.Error(err))

			// If too many retries, remove hint
			if hint.Retries >= 10 {
				h.removeHint(nodeID, hint)
				h.logger.Warn("Hint max retries exceeded, dropping",
					zap.String("node_id", nodeID),
					zap.String("key", hint.Key))
			}
		}
	}

	if successCount > 0 || expiredCount > 0 || failedCount > 0 {
		h.logger.Info("Hint replay completed",
			zap.String("node_id", nodeID),
			zap.Int("success", successCount),
			zap.Int("expired", expiredCount),
			zap.Int("failed", failedCount))
	}
}

// replayHint replays a single hint to the target node
func (h *HintedHandoffService) replayHint(ctx context.Context, hint *Hint) error {
	// Get the node information (simplified - in production, fetch from metadata store)
	node := &model.StorageNode{
		NodeID: hint.NodeID,
		// Host and Port would be fetched from metadata store
	}

	// Attempt to write to the node
	_, err := h.storageClient.Write(ctx, node, hint.TenantID, hint.Key, hint.Value, hint.VectorClock)
	if err != nil {
		return err
	}

	return nil
}

// removeHint removes a hint from the store
func (h *HintedHandoffService) removeHint(nodeID string, hintToRemove *Hint) {
	h.mu.Lock()
	defer h.mu.Unlock()

	nodeHints := h.hints[nodeID]
	newHints := make([]*Hint, 0, len(nodeHints)-1)

	for _, hint := range nodeHints {
		// Remove hint by comparing key and timestamp
		if hint.Key != hintToRemove.Key || !hint.Timestamp.Equal(hintToRemove.Timestamp) {
			newHints = append(newHints, hint)
		}
	}

	if len(newHints) == 0 {
		delete(h.hints, nodeID)
	} else {
		h.hints[nodeID] = newHints
	}
}

// GetHintCount returns the number of hints for a node
func (h *HintedHandoffService) GetHintCount(nodeID string) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.hints[nodeID])
}

// GetTotalHintCount returns the total number of hints across all nodes
func (h *HintedHandoffService) GetTotalHintCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()

	total := 0
	for _, hints := range h.hints {
		total += len(hints)
	}
	return total
}

// ClearHintsForNode clears all hints for a specific node
// Used when a node is permanently removed
func (h *HintedHandoffService) ClearHintsForNode(nodeID string) int {
	h.mu.Lock()
	defer h.mu.Unlock()

	count := len(h.hints[nodeID])
	delete(h.hints, nodeID)

	h.logger.Info("Cleared hints for node",
		zap.String("node_id", nodeID),
		zap.Int("hints_cleared", count))

	return count
}
