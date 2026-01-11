package service

import (
	"context"
	"fmt"
	"time"

	"github.com/devrev/pairdb/coordinator/internal/client"
	"github.com/devrev/pairdb/coordinator/internal/model"
	"github.com/devrev/pairdb/coordinator/internal/store"
	"go.uber.org/zap"
)

// HintReplayService manages replay of missed writes to recovered nodes
type HintReplayService struct {
	hintStore         store.HintStore
	storageNodeClient *client.StorageNodeClient
	logger            *zap.Logger
	batchSize         int
	replayInterval    time.Duration
	maxRetries        int
}

// NewHintReplayService creates a new hint replay service
func NewHintReplayService(
	hintStore store.HintStore,
	storageNodeClient *client.StorageNodeClient,
	logger *zap.Logger,
) *HintReplayService {
	return &HintReplayService{
		hintStore:         hintStore,
		storageNodeClient: storageNodeClient,
		logger:            logger,
		batchSize:         100,       // Replay 100 hints at a time
		replayInterval:    time.Second, // 1 second between batches
		maxRetries:        3,         // Retry failed hints up to 3 times
	}
}

// ReplayHintsForNode replays all hints for a specific node
// This is called before transitioning a node to NORMAL state after bootstrap
func (s *HintReplayService) ReplayHintsForNode(ctx context.Context, node *model.StorageNode) error {
	s.logger.Info("Starting hint replay for node",
		zap.String("node_id", node.NodeID))

	// Get hint count
	totalHints, err := s.hintStore.GetHintCount(ctx, node.NodeID)
	if err != nil {
		return fmt.Errorf("failed to get hint count: %w", err)
	}

	if totalHints == 0 {
		s.logger.Info("No hints to replay for node",
			zap.String("node_id", node.NodeID))
		return nil
	}

	s.logger.Info("Replaying hints for node",
		zap.String("node_id", node.NodeID),
		zap.Int64("total_hints", totalHints))

	replayed := int64(0)
	failed := int64(0)

	// Replay hints in batches
	for {
		// Get next batch of hints
		hints, err := s.hintStore.GetHintsForNode(ctx, node.NodeID, s.batchSize)
		if err != nil {
			return fmt.Errorf("failed to get hints: %w", err)
		}

		if len(hints) == 0 {
			break // All hints replayed
		}

		// Replay each hint in the batch
		for _, hint := range hints {
			if err := s.replayHint(ctx, node, hint); err != nil {
				s.logger.Warn("Failed to replay hint",
					zap.String("hint_id", hint.HintID),
					zap.String("node_id", node.NodeID),
					zap.Error(err))
				failed++

				// Check retry limit
				if hint.ReplayCount >= s.maxRetries {
					s.logger.Error("Hint exceeded max retries, deleting",
						zap.String("hint_id", hint.HintID),
						zap.Int("replay_count", hint.ReplayCount))
					if err := s.hintStore.DeleteHint(ctx, hint.HintID); err != nil {
						s.logger.Error("Failed to delete failed hint",
							zap.String("hint_id", hint.HintID),
							zap.Error(err))
					}
				}
				continue
			}

			// Successfully replayed, delete hint
			if err := s.hintStore.DeleteHint(ctx, hint.HintID); err != nil {
				s.logger.Error("Failed to delete replayed hint",
					zap.String("hint_id", hint.HintID),
					zap.Error(err))
			}

			replayed++
		}

		// Rate limiting: wait between batches
		if len(hints) == s.batchSize {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(s.replayInterval):
				// Continue to next batch
			}
		}
	}

	s.logger.Info("Hint replay completed for node",
		zap.String("node_id", node.NodeID),
		zap.Int64("total_hints", totalHints),
		zap.Int64("replayed", replayed),
		zap.Int64("failed", failed))

	return nil
}

// replayHint replays a single hint to a node
func (s *HintReplayService) replayHint(ctx context.Context, node *model.StorageNode, hint *model.Hint) error {
	s.logger.Debug("Replaying hint",
		zap.String("hint_id", hint.HintID),
		zap.String("node_id", node.NodeID),
		zap.String("tenant_id", hint.TenantID),
		zap.String("key", hint.Key))

	// Create write request for the hint
	writeReq := &client.WriteRequest{
		TenantID:    hint.TenantID,
		Key:         hint.Key,
		Value:       hint.Value,
		VectorClock: hint.VectorClock,
		Timestamp:   hint.Timestamp,
	}

	// Write to node
	resp, err := s.storageNodeClient.Write(ctx, node, writeReq)
	if err != nil {
		return fmt.Errorf("failed to write hint: %w", err)
	}

	if !resp.Success {
		return fmt.Errorf("hint write unsuccessful: %s", resp.Message)
	}

	return nil
}

// ScheduleReplay schedules hint replay for a node at a specific time
// This is useful for spreading out replay load across multiple nodes
func (s *HintReplayService) ScheduleReplay(ctx context.Context, node *model.StorageNode, delay time.Duration) {
	s.logger.Info("Scheduling hint replay",
		zap.String("node_id", node.NodeID),
		zap.Duration("delay", delay))

	go func() {
		select {
		case <-ctx.Done():
			s.logger.Info("Hint replay canceled",
				zap.String("node_id", node.NodeID))
			return
		case <-time.After(delay):
			if err := s.ReplayHintsForNode(context.Background(), node); err != nil {
				s.logger.Error("Scheduled hint replay failed",
					zap.String("node_id", node.NodeID),
					zap.Error(err))
			}
		}
	}()
}

// CleanupOldHints periodically cleans up old hints (TTL-based)
func (s *HintReplayService) CleanupOldHints(ctx context.Context, ttl time.Duration) {
	s.logger.Info("Starting hint cleanup",
		zap.Duration("ttl", ttl))

	ticker := time.NewTicker(1 * time.Hour) // Run cleanup every hour
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("Hint cleanup stopped")
			return
		case <-ticker.C:
			deleted, err := s.hintStore.CleanupOldHints(ctx, ttl)
			if err != nil {
				s.logger.Error("Failed to cleanup old hints",
					zap.Error(err))
			} else if deleted > 0 {
				s.logger.Info("Cleaned up old hints",
					zap.Int64("deleted", deleted),
					zap.Duration("ttl", ttl))
			}
		}
	}
}
