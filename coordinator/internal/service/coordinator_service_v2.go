package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/devrev/pairdb/coordinator/internal/client"
	"github.com/devrev/pairdb/coordinator/internal/model"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

// CoordinatorServiceV2 follows Cassandra/DynamoDB pattern:
// - No migration state checks
// - Hinted handoff for failures
// - Read repair for consistency
// - Immediate ring updates
type CoordinatorServiceV2 struct {
	tenantService       *TenantService
	routingService      *RoutingService
	consistencyService  *ConsistencyService
	idempotencyService  *IdempotencyService
	conflictService     *ConflictService
	vcService           *VectorClockService
	hintedHandoffService *HintedHandoffService
	storageClient       *client.StorageClient
	writeTimeout        time.Duration
	readTimeout         time.Duration
	logger              *zap.Logger
}

// NewCoordinatorServiceV2 creates a new coordinator service (V2 - Cassandra/DynamoDB pattern)
func NewCoordinatorServiceV2(
	tenantService *TenantService,
	routingService *RoutingService,
	consistencyService *ConsistencyService,
	idempotencyService *IdempotencyService,
	conflictService *ConflictService,
	vcService *VectorClockService,
	hintedHandoffService *HintedHandoffService,
	storageClient *client.StorageClient,
	writeTimeout, readTimeout time.Duration,
	logger *zap.Logger,
) *CoordinatorServiceV2 {
	return &CoordinatorServiceV2{
		tenantService:        tenantService,
		routingService:       routingService,
		consistencyService:   consistencyService,
		idempotencyService:   idempotencyService,
		conflictService:      conflictService,
		vcService:            vcService,
		hintedHandoffService: hintedHandoffService,
		storageClient:        storageClient,
		writeTimeout:         writeTimeout,
		readTimeout:          readTimeout,
		logger:               logger,
	}
}

// WriteKeyValue handles write operations following Cassandra pattern
// Key changes from V1:
// - NO migration state checks
// - Hinted handoff for failed writes
// - Simpler code path
func (s *CoordinatorServiceV2) WriteKeyValue(
	ctx context.Context,
	tenantID, key string,
	value []byte,
	consistency, idempotencyKey string,
) (*WriteResult, error) {
	// Normalize consistency level
	consistency, err := s.consistencyService.NormalizeConsistencyLevel(consistency)
	if err != nil {
		return nil, err
	}

	// Check idempotency
	if idempotencyKey != "" {
		cachedResp, err := s.idempotencyService.Get(ctx, tenantID, key, idempotencyKey)
		if err != nil {
			s.logger.Error("Failed to check idempotency",
				zap.String("tenant_id", tenantID),
				zap.String("key", key),
				zap.Error(err))
		} else if cachedResp != nil {
			// Return cached response
			return &WriteResult{
				Success:      cachedResp.Success,
				Key:          key,
				VectorClock:  model.VectorClock{Entries: []model.VectorClockEntry{}},
				ReplicaCount: cachedResp.ReplicaCount,
				Consistency:  cachedResp.Consistency,
				IsDuplicate:  true,
			}, nil
		}
	} else {
		idempotencyKey = s.idempotencyService.Generate(tenantID, key)
	}

	// Get tenant configuration
	tenant, err := s.tenantService.GetTenant(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to get tenant configuration: %w", err)
	}

	// Get write replica nodes (includes NORMAL, BOOTSTRAPPING, LEAVING)
	// KEY CHANGE: Ring already includes bootstrapping nodes!
	// No need to check migration state or get additional nodes
	replicas, err := s.routingService.GetWriteReplicas(ctx, tenantID, key, tenant.ReplicationFactor)
	if err != nil {
		return nil, fmt.Errorf("failed to get write replicas: %w", err)
	}

	// Read-Modify-Write: Read existing value and vector clock first
	var vectorClock model.VectorClock
	existingValue, existingVC, err := s.readExistingValue(ctx, replicas, tenantID, key)
	if err != nil {
		// Key doesn't exist yet, create new vector clock
		vectorClock = s.vcService.Increment(tenantID)
	} else {
		// Key exists, increment from existing vector clock
		vectorClock = s.vcService.IncrementFrom(existingVC)

		if len(existingValue) > 0 {
			s.logger.Info("Overwriting existing value",
				zap.String("tenant_id", tenantID),
				zap.String("key", key))
		}
	}

	// Write to replicas with hinted handoff
	// KEY CHANGE: Simpler code, hinted handoff built-in
	result, err := s.writeToReplicasWithHints(ctx, replicas, tenantID, key, value, vectorClock, consistency)
	if err != nil {
		return nil, err
	}

	// Store idempotency response
	idempResp := &IdempotencyResponse{
		Success:      result.Success,
		Key:          key,
		VectorClock:  []byte{},
		ReplicaCount: result.ReplicaCount,
		Consistency:  consistency,
	}
	if err := s.idempotencyService.Store(ctx, tenantID, key, idempotencyKey, idempResp); err != nil {
		s.logger.Warn("Failed to store idempotency response", zap.Error(err))
	}

	return result, nil
}

// ReadKeyValue handles read operations with read repair
// KEY CHANGE: Read repair added for eventual consistency
func (s *CoordinatorServiceV2) ReadKeyValue(
	ctx context.Context,
	tenantID, key string,
	consistency string,
) (*ReadResult, error) {
	// Normalize consistency level
	consistency, err := s.consistencyService.NormalizeConsistencyLevel(consistency)
	if err != nil {
		return nil, err
	}

	// Get tenant configuration
	tenant, err := s.tenantService.GetTenant(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to get tenant configuration: %w", err)
	}

	// Get read replica nodes (includes NORMAL, BOOTSTRAPPING, LEAVING)
	replicas, err := s.routingService.GetReadReplicas(ctx, tenantID, key, tenant.ReplicationFactor)
	if err != nil {
		return nil, fmt.Errorf("failed to get read replicas: %w", err)
	}

	// Read from replicas with read repair
	return s.readFromReplicasWithRepair(ctx, replicas, tenantID, key, consistency)
}

// writeToReplicasWithHints writes to replicas with hinted handoff
// Follows Cassandra pattern: store hints for failed writes
func (s *CoordinatorServiceV2) writeToReplicasWithHints(
	ctx context.Context,
	replicas []*model.StorageNode,
	tenantID, key string,
	value []byte,
	vectorClock model.VectorClock,
	consistency string,
) (*WriteResult, error) {
	ctx, cancel := context.WithTimeout(ctx, s.writeTimeout)
	defer cancel()

	// Cassandra-correct quorum calculation: based on authoritative replicas only
	requiredReplicas := s.consistencyService.GetRequiredReplicasForWriteSet(consistency, replicas)

	s.logger.Info("Writing to replicas",
		zap.String("tenant_id", tenantID),
		zap.String("key", key),
		zap.Int("total_replicas", len(replicas)),
		zap.Int("required_replicas", requiredReplicas),
		zap.String("consistency", consistency))

	// Write to all replicas in parallel
	responses := make([]*client.StorageResponse, 0, len(replicas))
	var mu sync.Mutex

	g, gctx := errgroup.WithContext(ctx)

	for _, node := range replicas {
		node := node // Capture loop variable

		g.Go(func() error {
			resp, err := s.storageClient.Write(gctx, node, tenantID, key, value, vectorClock)
			if err != nil {
				s.logger.Warn("Write failed to replica",
					zap.String("tenant_id", tenantID),
					zap.String("key", key),
					zap.String("node_id", node.NodeID),
					zap.Error(err))

				// KEY CHANGE: Store hint for failed write (Cassandra pattern)
				s.hintedHandoffService.StoreHint(node.NodeID, tenantID, key, value, vectorClock)

				// Still collect failure response for quorum check
				mu.Lock()
				responses = append(responses, &client.StorageResponse{
					NodeID:  node.NodeID,
					Success: false,
					Error:   err,
				})
				mu.Unlock()
				return nil // Don't fail the errgroup
			}

			// Collect successful response
			mu.Lock()
			responses = append(responses, resp)
			mu.Unlock()
			return nil
		})
	}

	// Wait for all writes to complete or timeout
	_ = g.Wait()

	// Count successful writes
	successCount := 0
	for _, resp := range responses {
		if resp.Success {
			successCount++
		}
	}

	s.logger.Info("Write completed",
		zap.String("tenant_id", tenantID),
		zap.String("key", key),
		zap.Int("success_count", successCount),
		zap.Int("required_replicas", requiredReplicas),
		zap.Int("hints_stored", len(replicas)-successCount))

	// Check if quorum is reached
	if !s.consistencyService.IsQuorumReached(successCount, len(replicas), consistency) {
		return &WriteResult{
			Success:      false,
			Key:          key,
			ReplicaCount: int32(successCount),
			Consistency:  consistency,
			ErrorMessage: fmt.Sprintf("quorum not reached: %d/%d", successCount, requiredReplicas),
		}, fmt.Errorf("quorum not reached: %d/%d", successCount, requiredReplicas)
	}

	return &WriteResult{
		Success:      true,
		Key:          key,
		VectorClock:  vectorClock,
		ReplicaCount: int32(successCount),
		Consistency:  consistency,
	}, nil
}

// readFromReplicasWithRepair reads from replicas and performs read repair
// Follows Cassandra pattern: update stale replicas in background
func (s *CoordinatorServiceV2) readFromReplicasWithRepair(
	ctx context.Context,
	replicas []*model.StorageNode,
	tenantID, key string,
	consistency string,
) (*ReadResult, error) {
	ctx, cancel := context.WithTimeout(ctx, s.readTimeout)
	defer cancel()

	requiredReplicas := s.consistencyService.GetRequiredReplicas(consistency, len(replicas))

	s.logger.Info("Reading from replicas",
		zap.String("tenant_id", tenantID),
		zap.String("key", key),
		zap.Int("total_replicas", len(replicas)),
		zap.Int("required_replicas", requiredReplicas))

	// Read from all replicas in parallel
	responses := make([]*client.StorageResponse, 0, len(replicas))
	var mu sync.Mutex

	g, gctx := errgroup.WithContext(ctx)

	for _, replica := range replicas {
		replica := replica // Capture loop variable
		g.Go(func() error {
			resp, err := s.storageClient.Read(gctx, replica, tenantID, key)
			if err != nil {
				s.logger.Warn("Read failed from replica",
					zap.String("tenant_id", tenantID),
					zap.String("key", key),
					zap.String("node_id", replica.NodeID),
					zap.Error(err))

				mu.Lock()
				responses = append(responses, &client.StorageResponse{
					NodeID:  replica.NodeID,
					Success: false,
					Error:   err,
				})
				mu.Unlock()
				return nil
			}

			mu.Lock()
			responses = append(responses, resp)
			mu.Unlock()
			return nil
		})
	}

	// Wait for all reads
	_ = g.Wait()

	// Count successful reads
	successCount := 0
	for _, resp := range responses {
		if resp.Success && resp.Found {
			successCount++
		}
	}

	// Check if quorum is reached
	if !s.consistencyService.IsQuorumReached(successCount, len(replicas), consistency) {
		return &ReadResult{
			Success:      false,
			Key:          key,
			ErrorMessage: fmt.Sprintf("quorum not reached: %d/%d", successCount, requiredReplicas),
		}, fmt.Errorf("quorum not reached: %d/%d", successCount, requiredReplicas)
	}

	// Get latest value (resolve conflicts)
	latest, hasConflict := s.conflictService.DetectConflicts(responses)
	if latest == nil {
		return &ReadResult{
			Success:      false,
			Key:          key,
			ErrorMessage: "key not found",
		}, fmt.Errorf("key not found")
	}

	// KEY CHANGE: Read repair - update stale replicas in background (Cassandra pattern)
	if hasConflict {
		s.logger.Info("Conflict detected during read, performing read repair",
			zap.String("tenant_id", tenantID),
			zap.String("key", key))

		// Trigger repair asynchronously
		go s.performReadRepair(context.Background(), tenantID, key, latest, responses)
	}

	return &ReadResult{
		Success:     true,
		Key:         key,
		Value:       latest.Value,
		VectorClock: latest.VectorClock,
	}, nil
}

// performReadRepair updates stale replicas with the latest value
// This is Cassandra's "read repair" mechanism for eventual consistency
func (s *CoordinatorServiceV2) performReadRepair(
	ctx context.Context,
	tenantID, key string,
	latest *client.StorageResponse,
	allResponses []*client.StorageResponse,
) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	repairedCount := 0

	for _, resp := range allResponses {
		// Skip if this response has the latest version
		if resp.VectorClock.Equals(latest.VectorClock) {
			continue
		}

		// Update stale replica
		node := &model.StorageNode{NodeID: resp.NodeID}
		_, err := s.storageClient.Write(ctx, node, tenantID, key, latest.Value, latest.VectorClock)
		if err != nil {
			s.logger.Warn("Read repair failed",
				zap.String("node_id", resp.NodeID),
				zap.String("key", key),
				zap.Error(err))

			// Store hint for failed repair
			s.hintedHandoffService.StoreHint(node.NodeID, tenantID, key, latest.Value, latest.VectorClock)
		} else {
			repairedCount++
			s.logger.Debug("Read repair succeeded",
				zap.String("node_id", resp.NodeID),
				zap.String("key", key))
		}
	}

	if repairedCount > 0 {
		s.logger.Info("Read repair completed",
			zap.String("tenant_id", tenantID),
			zap.String("key", key),
			zap.Int("repaired_count", repairedCount))
	}
}

// readExistingValue reads the existing value for read-modify-write
func (s *CoordinatorServiceV2) readExistingValue(
	ctx context.Context,
	replicas []*model.StorageNode,
	tenantID, key string,
) ([]byte, model.VectorClock, error) {
	ctx, cancel := context.WithTimeout(ctx, s.readTimeout)
	defer cancel()

	// Read from first available replica
	var mu sync.Mutex
	var firstResponse *client.StorageResponse
	responseReceived := false

	g, gctx := errgroup.WithContext(ctx)

	for _, replica := range replicas {
		replica := replica
		g.Go(func() error {
			mu.Lock()
			if responseReceived {
				mu.Unlock()
				return nil
			}
			mu.Unlock()

			resp, err := s.storageClient.Read(gctx, replica, tenantID, key)
			if err != nil {
				return nil
			}

			if resp.Success && resp.Found {
				mu.Lock()
				if !responseReceived {
					firstResponse = resp
					responseReceived = true
				}
				mu.Unlock()
			}

			return nil
		})
	}

	_ = g.Wait()

	if firstResponse == nil || !firstResponse.Found {
		return nil, model.VectorClock{}, fmt.Errorf("key not found")
	}

	return firstResponse.Value, firstResponse.VectorClock, nil
}
