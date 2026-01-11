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

// CleanupService manages safe data cleanup after topology changes
// Enforces grace periods and quorum verification before deletion
type CleanupService struct {
	metadataStore     store.MetadataStore
	storageNodeClient *client.StorageNodeClient
	routingService    *RoutingService
	gracePeriod       time.Duration // Default: 24 hours
	logger            *zap.Logger
}

// NewCleanupService creates a new cleanup service
func NewCleanupService(
	metadataStore store.MetadataStore,
	storageNodeClient *client.StorageNodeClient,
	routingService *RoutingService,
	gracePeriod time.Duration,
	logger *zap.Logger,
) *CleanupService {
	if gracePeriod == 0 {
		gracePeriod = 24 * time.Hour // Default to 24 hours
	}

	return &CleanupService{
		metadataStore:     metadataStore,
		storageNodeClient: storageNodeClient,
		routingService:    routingService,
		gracePeriod:       gracePeriod,
		logger:            logger,
	}
}

// ScheduleCleanup schedules cleanup for a pending change after the grace period
func (s *CleanupService) ScheduleCleanup(ctx context.Context, changeID string) error {
	s.logger.Info("Scheduling cleanup for pending change",
		zap.String("change_id", changeID),
		zap.Duration("grace_period", s.gracePeriod))

	// Get the pending change
	change, err := s.metadataStore.GetPendingChange(ctx, changeID)
	if err != nil {
		return fmt.Errorf("failed to get pending change: %w", err)
	}

	// Schedule cleanup after grace period
	go func() {
		// Wait for grace period
		time.Sleep(s.gracePeriod)

		// Execute cleanup with new context (original may be canceled)
		cleanupCtx := context.Background()
		if err := s.ExecuteCleanup(cleanupCtx, change); err != nil {
			s.logger.Error("Failed to execute scheduled cleanup",
				zap.String("change_id", changeID),
				zap.Error(err))
		}
	}()

	return nil
}

// VerifyCleanupSafe checks if it's safe to cleanup data after a topology change
func (s *CleanupService) VerifyCleanupSafe(ctx context.Context, change *model.PendingChange) (bool, error) {
	s.logger.Info("Verifying cleanup safety",
		zap.String("change_id", change.ChangeID),
		zap.String("type", change.Type))

	// Check 1: PendingChange must be completed
	if change.Status != model.PendingChangeCompleted {
		s.logger.Warn("Cleanup verification failed: change not completed",
			zap.String("change_id", change.ChangeID),
			zap.String("status", string(change.Status)))
		return false, fmt.Errorf("change not completed: %s", change.Status)
	}

	// Check 2: No active streaming
	for sourceNode, progress := range change.Progress {
		if progress.State != "completed" {
			s.logger.Warn("Cleanup verification failed: streaming still active",
				zap.String("change_id", change.ChangeID),
				zap.String("source_node", sourceNode),
				zap.String("state", progress.State))
			return false, fmt.Errorf("streaming still active for %s", sourceNode)
		}
	}

	// Check 3: Grace period must have passed
	// Use CompletedAt if available, otherwise LastUpdated
	completedTime := change.CompletedAt
	if completedTime.IsZero() {
		completedTime = change.LastUpdated
	}

	timeSinceCompletion := time.Since(completedTime)
	if timeSinceCompletion < s.gracePeriod {
		remaining := s.gracePeriod - timeSinceCompletion
		s.logger.Warn("Cleanup verification failed: grace period not elapsed",
			zap.String("change_id", change.ChangeID),
			zap.Duration("time_since_completion", timeSinceCompletion),
			zap.Duration("grace_period", s.gracePeriod),
			zap.Duration("remaining", remaining))
		return false, fmt.Errorf("grace period not elapsed: %v remaining", remaining)
	}

	// Check 4: Quorum verification
	if !s.verifyQuorum(ctx, change) {
		s.logger.Warn("Cleanup verification failed: quorum verification failed",
			zap.String("change_id", change.ChangeID))
		return false, fmt.Errorf("quorum verification failed")
	}

	s.logger.Info("Cleanup verification passed",
		zap.String("change_id", change.ChangeID))

	return true, nil
}

// verifyQuorum verifies that a quorum of replicas have the data for all ranges
func (s *CleanupService) verifyQuorum(ctx context.Context, change *model.PendingChange) bool {
	replicationFactor := 3 // TODO: Get from config/tenant settings

	s.logger.Info("Verifying quorum for cleanup",
		zap.String("change_id", change.ChangeID),
		zap.Int("ranges", len(change.Ranges)),
		zap.Int("replication_factor", replicationFactor))

	// For each range affected by the topology change
	for _, tokenRange := range change.Ranges {
		// Get replicas that should own this range
		// Note: This uses the current ring state (after topology change)
		replicas := s.getReplicasForRange(ctx, tokenRange, replicationFactor)

		if len(replicas) < replicationFactor {
			s.logger.Warn("Not enough replicas available for range",
				zap.String("change_id", change.ChangeID),
				zap.String("range_start", fmt.Sprintf("%d", tokenRange.Start)),
				zap.String("range_end", fmt.Sprintf("%d", tokenRange.End)),
				zap.Int("available_replicas", len(replicas)),
				zap.Int("required", replicationFactor))
			return false
		}

		// Verify data presence on replicas
		consistentCount := 0
		for _, replica := range replicas {
			if s.verifyReplicaHasRange(ctx, replica, tokenRange) {
				consistentCount++
			}
		}

		// Check if quorum is met
		quorum := (replicationFactor / 2) + 1
		if consistentCount < quorum {
			s.logger.Warn("Quorum not met for range",
				zap.String("change_id", change.ChangeID),
				zap.String("range_start", fmt.Sprintf("%d", tokenRange.Start)),
				zap.String("range_end", fmt.Sprintf("%d", tokenRange.End)),
				zap.Int("consistent_replicas", consistentCount),
				zap.Int("required_quorum", quorum))
			return false
		}

		s.logger.Debug("Quorum verified for range",
			zap.String("range_start", fmt.Sprintf("%d", tokenRange.Start)),
			zap.String("range_end", fmt.Sprintf("%d", tokenRange.End)),
			zap.Int("consistent_replicas", consistentCount),
			zap.Int("required_quorum", quorum))
	}

	s.logger.Info("Quorum verification passed for all ranges",
		zap.String("change_id", change.ChangeID))

	return true
}

// getReplicasForRange gets replicas that should own a specific token range
// This is a simplified implementation - in production would use the routing service
func (s *CleanupService) getReplicasForRange(ctx context.Context, tokenRange model.TokenRange, rf int) []*model.StorageNode {
	// Get all storage nodes
	nodes, err := s.metadataStore.ListStorageNodes(ctx)
	if err != nil {
		s.logger.Error("Failed to get storage nodes for range",
			zap.Error(err))
		return []*model.StorageNode{}
	}

	// Filter to nodes in NORMAL state (authoritative)
	normalNodes := make([]*model.StorageNode, 0)
	for _, node := range nodes {
		if node.State == model.NodeStateNormal {
			normalNodes = append(normalNodes, node)
		}
	}

	// For simplicity, return first RF nodes
	// In production, would use consistent hashing to determine correct replicas
	if len(normalNodes) < rf {
		return normalNodes
	}

	return normalNodes[:rf]
}

// verifyReplicaHasRange verifies that a replica has data for a specific range
// This is a simplified implementation - in production would query the storage node
func (s *CleanupService) verifyReplicaHasRange(ctx context.Context, replica *model.StorageNode, tokenRange model.TokenRange) bool {
	// TODO: Implement actual verification by querying storage node
	// For now, assume verification passes if node is NORMAL
	return replica.State == model.NodeStateNormal
}

// ExecuteCleanup executes the cleanup operation after verifying safety
func (s *CleanupService) ExecuteCleanup(ctx context.Context, change *model.PendingChange) error {
	s.logger.Info("Executing cleanup",
		zap.String("change_id", change.ChangeID),
		zap.String("type", change.Type))

	// Verify cleanup is safe
	safe, err := s.VerifyCleanupSafe(ctx, change)
	if err != nil || !safe {
		return fmt.Errorf("cleanup verification failed: %w", err)
	}

	// Execute cleanup based on change type
	switch change.Type {
	case "decommission":
		// For decommission, cleanup is already done by removing the node
		// This would clean up any remaining metadata or temporary data
		s.logger.Info("Decommission cleanup completed (node already removed)",
			zap.String("change_id", change.ChangeID),
			zap.String("node_id", change.NodeID))

	case "bootstrap":
		// For bootstrap, no cleanup needed (node was added)
		s.logger.Info("Bootstrap cleanup completed (no action needed)",
			zap.String("change_id", change.ChangeID),
			zap.String("node_id", change.NodeID))

	default:
		s.logger.Warn("Unknown change type for cleanup",
			zap.String("change_id", change.ChangeID),
			zap.String("type", change.Type))
	}

	// Mark pending change as cleaned up
	if err := s.metadataStore.DeletePendingChange(ctx, change.ChangeID); err != nil {
		s.logger.Error("Failed to delete pending change after cleanup",
			zap.String("change_id", change.ChangeID),
			zap.Error(err))
		return fmt.Errorf("failed to delete pending change: %w", err)
	}

	s.logger.Info("Cleanup executed successfully",
		zap.String("change_id", change.ChangeID))

	return nil
}

// ForceCleanup forces cleanup without safety checks (emergency use only)
func (s *CleanupService) ForceCleanup(ctx context.Context, changeID string, reason string) error {
	s.logger.Warn("FORCE CLEANUP requested - bypassing safety checks",
		zap.String("change_id", changeID),
		zap.String("reason", reason))

	change, err := s.metadataStore.GetPendingChange(ctx, changeID)
	if err != nil {
		return fmt.Errorf("failed to get pending change: %w", err)
	}

	// Delete pending change without verification
	if err := s.metadataStore.DeletePendingChange(ctx, changeID); err != nil {
		return fmt.Errorf("failed to force delete pending change: %w", err)
	}

	s.logger.Warn("Force cleanup completed",
		zap.String("change_id", changeID),
		zap.String("type", change.Type),
		zap.String("reason", reason))

	return nil
}
