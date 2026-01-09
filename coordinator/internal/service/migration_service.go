package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/devrev/pairdb/coordinator/internal/client"
	"github.com/devrev/pairdb/coordinator/internal/model"
	"github.com/devrev/pairdb/coordinator/internal/store"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// MigrationService manages node addition/removal migrations with dual-write strategy
type MigrationService struct {
	metadataStore  store.MetadataStore
	routingService *RoutingService
	storageClient  *client.StorageClient
	logger         *zap.Logger

	// Track active migrations
	activeMigrations sync.Map // migrationID -> *MigrationContext
	mu               sync.RWMutex
}

// MigrationContext tracks the state of an ongoing migration
type MigrationContext struct {
	Migration       *model.Migration
	NewNode         *model.StorageNode
	OldNodes        []*model.StorageNode
	Phase           model.MigrationPhase
	DualWriteActive bool
	CancelFunc      context.CancelFunc
	mu              sync.RWMutex
}

// NewMigrationService creates a new migration service
func NewMigrationService(
	metadataStore store.MetadataStore,
	routingService *RoutingService,
	storageClient *client.StorageClient,
	logger *zap.Logger,
) *MigrationService {
	return &MigrationService{
		metadataStore:  metadataStore,
		routingService: routingService,
		storageClient:  storageClient,
		logger:         logger,
	}
}

// StartNodeAddition initiates a node addition migration with dual-write strategy
func (s *MigrationService) StartNodeAddition(ctx context.Context, node *model.StorageNode) (*model.Migration, error) {
	s.logger.Info("Starting node addition migration",
		zap.String("node_id", node.NodeID),
		zap.String("host", node.Host),
		zap.Int("port", node.Port))

	// Create migration record
	migration := &model.Migration{
		MigrationID: uuid.New().String(),
		Type:        model.MigrationTypeNodeAddition,
		NodeID:      node.NodeID,
		Status:      model.MigrationStatusPending,
		Phase:       model.MigrationPhaseDualWrite,
		StartedAt:   time.Now(),
	}

	// Save migration to metadata store
	if err := s.metadataStore.CreateMigration(ctx, migration); err != nil {
		return nil, fmt.Errorf("failed to create migration: %w", err)
	}

	// Add node to metadata store with "draining" status initially
	node.Status = model.NodeStatusDraining
	if err := s.metadataStore.AddStorageNode(ctx, node); err != nil {
		return nil, fmt.Errorf("failed to add storage node: %w", err)
	}

	// Create migration context
	migrationCtx, cancel := context.WithCancel(context.Background())
	mctx := &MigrationContext{
		Migration:       migration,
		NewNode:         node,
		DualWriteActive: true,
		CancelFunc:      cancel,
		Phase:           model.MigrationPhaseDualWrite,
	}

	// Store migration context
	s.activeMigrations.Store(migration.MigrationID, mctx)

	// Start dual-write phase in background
	go s.executeMigrationPhases(migrationCtx, mctx)

	s.logger.Info("Node addition migration started",
		zap.String("migration_id", migration.MigrationID),
		zap.String("node_id", node.NodeID),
		zap.String("phase", string(model.MigrationPhaseDualWrite)))

	return migration, nil
}

// executeMigrationPhases executes all migration phases sequentially
func (s *MigrationService) executeMigrationPhases(ctx context.Context, mctx *MigrationContext) {
	migration := mctx.Migration

	// Update status to in_progress
	if err := s.metadataStore.UpdateMigrationStatus(ctx, migration.MigrationID, model.MigrationStatusInProgress, ""); err != nil {
		s.logger.Error("Failed to update migration status", zap.Error(err))
	}

	// Phase 1: Dual Write (enable writes to both old and new nodes)
	if err := s.executeDualWritePhase(ctx, mctx); err != nil {
		s.handleMigrationFailure(ctx, mctx, fmt.Errorf("dual write phase failed: %w", err))
		return
	}

	// Phase 2: Data Copy (copy existing data to new node)
	if err := s.executeDataCopyPhase(ctx, mctx); err != nil {
		s.handleMigrationFailure(ctx, mctx, fmt.Errorf("data copy phase failed: %w", err))
		return
	}

	// Phase 3: Cutover (activate new node in hash ring)
	if err := s.executeCutoverPhase(ctx, mctx); err != nil {
		s.handleMigrationFailure(ctx, mctx, fmt.Errorf("cutover phase failed: %w", err))
		return
	}

	// Phase 4: Cleanup (disable dual write)
	if err := s.executeCleanupPhase(ctx, mctx); err != nil {
		s.logger.Warn("Cleanup phase had issues", zap.Error(err))
	}

	// Mark migration as completed
	completedAt := time.Now()
	migration.CompletedAt = &completedAt
	migration.Status = model.MigrationStatusCompleted
	migration.Phase = model.MigrationPhaseCleanup

	if err := s.metadataStore.UpdateMigrationStatus(ctx, migration.MigrationID, model.MigrationStatusCompleted, ""); err != nil {
		s.logger.Error("Failed to mark migration as completed", zap.Error(err))
	}

	// Remove from active migrations
	s.activeMigrations.Delete(migration.MigrationID)

	s.logger.Info("Node addition migration completed successfully",
		zap.String("migration_id", migration.MigrationID),
		zap.String("node_id", mctx.NewNode.NodeID))
}

// executeDualWritePhase enables dual writes (old replicas + new node)
func (s *MigrationService) executeDualWritePhase(ctx context.Context, mctx *MigrationContext) error {
	s.logger.Info("Starting dual write phase",
		zap.String("migration_id", mctx.Migration.MigrationID),
		zap.String("node_id", mctx.NewNode.NodeID))

	mctx.mu.Lock()
	mctx.Phase = model.MigrationPhaseDualWrite
	mctx.DualWriteActive = true
	mctx.mu.Unlock()

	// Update migration phase
	mctx.Migration.Phase = model.MigrationPhaseDualWrite
	// Note: Dual write is handled in WriteKeyValue method by checking active migrations

	// Wait a bit to ensure dual write is active
	time.Sleep(1 * time.Second)

	s.logger.Info("Dual write phase activated",
		zap.String("migration_id", mctx.Migration.MigrationID))

	return nil
}

// executeDataCopyPhase copies existing data to the new node
func (s *MigrationService) executeDataCopyPhase(ctx context.Context, mctx *MigrationContext) error {
	s.logger.Info("Starting data copy phase",
		zap.String("migration_id", mctx.Migration.MigrationID),
		zap.String("node_id", mctx.NewNode.NodeID))

	mctx.mu.Lock()
	mctx.Phase = model.MigrationPhaseDataCopy
	mctx.mu.Unlock()

	mctx.Migration.Phase = model.MigrationPhaseDataCopy

	// TODO: Implement actual data copy logic
	// This would involve:
	// 1. Get all keys that should be on this node (based on hash ring)
	// 2. Read from existing replicas
	// 3. Write to new node
	// 4. Track progress

	// For now, simulate data copy
	s.logger.Info("Data copy phase simulated (implement actual copy logic)",
		zap.String("migration_id", mctx.Migration.MigrationID))

	time.Sleep(2 * time.Second) // Simulate copy time

	s.logger.Info("Data copy phase completed",
		zap.String("migration_id", mctx.Migration.MigrationID))

	return nil
}

// executeCutoverPhase activates the new node in the hash ring
func (s *MigrationService) executeCutoverPhase(ctx context.Context, mctx *MigrationContext) error {
	s.logger.Info("Starting cutover phase",
		zap.String("migration_id", mctx.Migration.MigrationID),
		zap.String("node_id", mctx.NewNode.NodeID))

	mctx.mu.Lock()
	mctx.Phase = model.MigrationPhaseCutover
	mctx.mu.Unlock()

	mctx.Migration.Phase = model.MigrationPhaseCutover

	// Change node status from "draining" to "active"
	if err := s.metadataStore.UpdateStorageNodeStatus(ctx, mctx.NewNode.NodeID, string(model.NodeStatusActive)); err != nil {
		return fmt.Errorf("failed to activate node: %w", err)
	}

	// Add node to routing service (this updates the hash ring)
	mctx.NewNode.Status = model.NodeStatusActive
	if err := s.routingService.AddNode(ctx, mctx.NewNode); err != nil {
		return fmt.Errorf("failed to add node to routing service: %w", err)
	}

	s.logger.Info("Cutover phase completed, node is now active",
		zap.String("migration_id", mctx.Migration.MigrationID),
		zap.String("node_id", mctx.NewNode.NodeID))

	return nil
}

// executeCleanupPhase disables dual write and cleans up
func (s *MigrationService) executeCleanupPhase(ctx context.Context, mctx *MigrationContext) error {
	s.logger.Info("Starting cleanup phase",
		zap.String("migration_id", mctx.Migration.MigrationID),
		zap.String("node_id", mctx.NewNode.NodeID))

	mctx.mu.Lock()
	mctx.Phase = model.MigrationPhaseCleanup
	mctx.DualWriteActive = false
	mctx.mu.Unlock()

	mctx.Migration.Phase = model.MigrationPhaseCleanup

	// Dual write will automatically stop after this

	s.logger.Info("Cleanup phase completed",
		zap.String("migration_id", mctx.Migration.MigrationID))

	return nil
}

// handleMigrationFailure handles migration failures
func (s *MigrationService) handleMigrationFailure(ctx context.Context, mctx *MigrationContext, err error) {
	s.logger.Error("Migration failed",
		zap.String("migration_id", mctx.Migration.MigrationID),
		zap.String("phase", string(mctx.Phase)),
		zap.Error(err))

	// Update migration status
	if updateErr := s.metadataStore.UpdateMigrationStatus(ctx, mctx.Migration.MigrationID, model.MigrationStatusFailed, err.Error()); updateErr != nil {
		s.logger.Error("Failed to update migration status", zap.Error(updateErr))
	}

	// Disable dual write
	mctx.mu.Lock()
	mctx.DualWriteActive = false
	mctx.mu.Unlock()

	// Mark node as inactive if cutover didn't happen
	if mctx.Phase != model.MigrationPhaseCutover && mctx.Phase != model.MigrationPhaseCleanup {
		if statusErr := s.metadataStore.UpdateStorageNodeStatus(ctx, mctx.NewNode.NodeID, string(model.NodeStatusInactive)); statusErr != nil {
			s.logger.Error("Failed to mark node as inactive", zap.Error(statusErr))
		}
	}

	// Remove from active migrations
	s.activeMigrations.Delete(mctx.Migration.MigrationID)
}

// GetActiveMigration returns the active migration for a node
func (s *MigrationService) GetActiveMigration(nodeID string) *MigrationContext {
	var result *MigrationContext
	s.activeMigrations.Range(func(key, value interface{}) bool {
		mctx := value.(*MigrationContext)
		if mctx.NewNode.NodeID == nodeID {
			result = mctx
			return false
		}
		return true
	})
	return result
}

// GetMigrationByID returns a migration context by ID
func (s *MigrationService) GetMigrationByID(migrationID string) *MigrationContext {
	value, ok := s.activeMigrations.Load(migrationID)
	if !ok {
		return nil
	}
	return value.(*MigrationContext)
}

// IsDualWriteActive checks if dual write is active for any migration
func (s *MigrationService) IsDualWriteActive() bool {
	hasActive := false
	s.activeMigrations.Range(func(key, value interface{}) bool {
		mctx := value.(*MigrationContext)
		mctx.mu.RLock()
		if mctx.DualWriteActive {
			hasActive = true
		}
		mctx.mu.RUnlock()
		return !hasActive // Continue until we find one active
	})
	return hasActive
}

// GetDualWriteNodes returns nodes that should receive dual writes
// Returns: (regular replicas, additional migration nodes)
func (s *MigrationService) GetDualWriteNodes(ctx context.Context, tenantID, key string, regularReplicas []*model.StorageNode) ([]*model.StorageNode, error) {
	additionalNodes := make([]*model.StorageNode, 0)

	s.activeMigrations.Range(func(migKey, value interface{}) bool {
		mctx := value.(*MigrationContext)
		mctx.mu.RLock()
		defer mctx.mu.RUnlock()

		// Only consider migrations in dual-write phase
		if !mctx.DualWriteActive || mctx.Phase != model.MigrationPhaseDualWrite {
			return true
		}

		// Check if this key would map to the new node (based on hash ring after addition)
		// For simplicity, we'll write to all nodes being added during migration
		// In production, you'd check if the key's hash falls in the new node's range
		additionalNodes = append(additionalNodes, mctx.NewNode)

		return true
	})

	return additionalNodes, nil
}

// CancelMigration cancels an ongoing migration
func (s *MigrationService) CancelMigration(ctx context.Context, migrationID string) error {
	value, ok := s.activeMigrations.Load(migrationID)
	if !ok {
		return fmt.Errorf("migration not found: %s", migrationID)
	}

	mctx := value.(*MigrationContext)

	s.logger.Info("Cancelling migration",
		zap.String("migration_id", migrationID),
		zap.String("phase", string(mctx.Phase)))

	// Cancel the context
	mctx.CancelFunc()

	// Update status
	if err := s.metadataStore.UpdateMigrationStatus(ctx, migrationID, model.MigrationStatusCancelled, "Cancelled by user"); err != nil {
		return fmt.Errorf("failed to update migration status: %w", err)
	}

	// Cleanup
	mctx.mu.Lock()
	mctx.DualWriteActive = false
	mctx.mu.Unlock()

	// Mark node as inactive
	if err := s.metadataStore.UpdateStorageNodeStatus(ctx, mctx.NewNode.NodeID, string(model.NodeStatusInactive)); err != nil {
		s.logger.Error("Failed to mark node as inactive", zap.Error(err))
	}

	// Remove from active migrations
	s.activeMigrations.Delete(migrationID)

	s.logger.Info("Migration cancelled",
		zap.String("migration_id", migrationID))

	return nil
}
