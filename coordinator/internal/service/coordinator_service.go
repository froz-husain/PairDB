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

// CoordinatorService is the main service that orchestrates all operations
type CoordinatorService struct {
	tenantService      *TenantService
	routingService     *RoutingService
	consistencyService *ConsistencyService
	idempotencyService *IdempotencyService
	conflictService    *ConflictService
	vcService          *VectorClockService
	migrationService   *MigrationService
	storageClient      *client.StorageClient
	writeTimeout       time.Duration
	readTimeout        time.Duration
	logger             *zap.Logger
}

// WriteResult represents the result of a write operation
type WriteResult struct {
	Success      bool
	Key          string
	VectorClock  model.VectorClock
	ReplicaCount int32
	Consistency  string
	IsDuplicate  bool
	ErrorMessage string
}

// ReadResult represents the result of a read operation
type ReadResult struct {
	Success      bool
	Key          string
	Value        []byte
	VectorClock  model.VectorClock
	ErrorMessage string
}

// NewCoordinatorService creates a new coordinator service
func NewCoordinatorService(
	tenantService *TenantService,
	routingService *RoutingService,
	consistencyService *ConsistencyService,
	idempotencyService *IdempotencyService,
	conflictService *ConflictService,
	vcService *VectorClockService,
	migrationService *MigrationService,
	storageClient *client.StorageClient,
	writeTimeout, readTimeout time.Duration,
	logger *zap.Logger,
) *CoordinatorService {
	return &CoordinatorService{
		tenantService:      tenantService,
		routingService:     routingService,
		consistencyService: consistencyService,
		idempotencyService: idempotencyService,
		conflictService:    conflictService,
		vcService:          vcService,
		migrationService:   migrationService,
		storageClient:      storageClient,
		writeTimeout:       writeTimeout,
		readTimeout:        readTimeout,
		logger:             logger,
	}
}

// WriteKeyValue handles write operations with consistency guarantees
func (s *CoordinatorService) WriteKeyValue(
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
			s.logger.Info("Returning cached idempotent response",
				zap.String("tenant_id", tenantID),
				zap.String("key", key),
				zap.String("idempotency_key", idempotencyKey))

			return &WriteResult{
				Success:      cachedResp.Success,
				Key:          key,
				VectorClock:  model.VectorClock{Entries: []model.VectorClockEntry{}}, // Deserialize from cachedResp.VectorClock
				ReplicaCount: cachedResp.ReplicaCount,
				Consistency:  cachedResp.Consistency,
				IsDuplicate:  true,
			}, nil
		}
	} else {
		// Generate server-side idempotency key
		idempotencyKey = s.idempotencyService.Generate(tenantID, key)
	}

	// Get tenant configuration
	tenant, err := s.tenantService.GetTenant(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to get tenant configuration: %w", err)
	}

	// Get replica nodes
	replicas, err := s.routingService.GetReplicas(ctx, tenantID, key, tenant.ReplicationFactor)
	if err != nil {
		return nil, fmt.Errorf("failed to get replicas: %w", err)
	}

	// Read-Modify-Write: Read existing value and vector clock first
	// This ensures causality is preserved across concurrent writes
	var vectorClock model.VectorClock
	existingValue, existingVC, err := s.readExistingValue(ctx, replicas, tenantID, key)
	if err != nil {
		// Key doesn't exist yet, create new vector clock
		s.logger.Debug("Key not found, creating new vector clock",
			zap.String("tenant_id", tenantID),
			zap.String("key", key))
		vectorClock = s.vcService.Increment(tenantID)
	} else {
		// Key exists, increment from existing vector clock to maintain causality
		s.logger.Debug("Key found, incrementing from existing vector clock",
			zap.String("tenant_id", tenantID),
			zap.String("key", key),
			zap.Any("existing_vc", existingVC))
		vectorClock = s.vcService.IncrementFrom(existingVC)

		// Log if value is being overwritten
		if len(existingValue) > 0 {
			s.logger.Info("Overwriting existing value",
				zap.String("tenant_id", tenantID),
				zap.String("key", key),
				zap.Int("old_size", len(existingValue)),
				zap.Int("new_size", len(value)))
		}
	}

	// Check for active migrations and get additional dual-write nodes
	additionalNodes := make([]*model.StorageNode, 0)
	if s.migrationService != nil && s.migrationService.IsDualWriteActive() {
		migrationNodes, err := s.migrationService.GetDualWriteNodes(ctx, tenantID, key, replicas)
		if err != nil {
			s.logger.Warn("Failed to get dual write nodes",
				zap.String("tenant_id", tenantID),
				zap.String("key", key),
				zap.Error(err))
		} else if len(migrationNodes) > 0 {
			s.logger.Debug("Dual write active, writing to additional nodes",
				zap.String("tenant_id", tenantID),
				zap.String("key", key),
				zap.Int("additional_nodes", len(migrationNodes)))
			additionalNodes = migrationNodes
		}
	}

	// Write to replicas with proper vector clock (includes dual write)
	result, err := s.writeToReplicas(ctx, replicas, tenantID, key, value, vectorClock, consistency, additionalNodes)
	if err != nil {
		return nil, err
	}

	// Store idempotency response
	idempResp := &IdempotencyResponse{
		Success:      result.Success,
		Key:          key,
		VectorClock:  []byte{}, // Serialize vectorClock
		ReplicaCount: result.ReplicaCount,
		Consistency:  consistency,
	}
	if err := s.idempotencyService.Store(ctx, tenantID, key, idempotencyKey, idempResp); err != nil {
		s.logger.Warn("Failed to store idempotency response",
			zap.String("tenant_id", tenantID),
			zap.String("key", key),
			zap.Error(err))
	}

	return result, nil
}

// ReadKeyValue handles read operations with consistency guarantees
func (s *CoordinatorService) ReadKeyValue(
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

	// Get replica nodes
	replicas, err := s.routingService.GetReplicas(ctx, tenantID, key, tenant.ReplicationFactor)
	if err != nil {
		return nil, fmt.Errorf("failed to get replicas: %w", err)
	}

	// Read from replicas
	return s.readFromReplicas(ctx, replicas, tenantID, key, consistency)
}

// writeToReplicas writes to multiple replicas in parallel
// additionalNodes parameter is for dual-write during migrations (optional, can be nil)
func (s *CoordinatorService) writeToReplicas(
	ctx context.Context,
	replicas []*model.StorageNode,
	tenantID, key string,
	value []byte,
	vectorClock model.VectorClock,
	consistency string,
	additionalNodes []*model.StorageNode,
) (*WriteResult, error) {
	ctx, cancel := context.WithTimeout(ctx, s.writeTimeout)
	defer cancel()

	requiredReplicas := s.consistencyService.GetRequiredReplicas(consistency, len(replicas))

	// Combine regular replicas with additional dual-write nodes
	allNodes := make([]*model.StorageNode, 0, len(replicas)+len(additionalNodes))
	allNodes = append(allNodes, replicas...)

	// Track which nodes are dual-write (for error handling)
	dualWriteNodes := make(map[string]bool)
	if len(additionalNodes) > 0 {
		allNodes = append(allNodes, additionalNodes...)
		for _, node := range additionalNodes {
			dualWriteNodes[node.NodeID] = true
		}
	}

	s.logger.Info("Writing to replicas",
		zap.String("tenant_id", tenantID),
		zap.String("key", key),
		zap.Int("total_replicas", len(replicas)),
		zap.Int("required_replicas", requiredReplicas),
		zap.Int("dual_write_nodes", len(additionalNodes)),
		zap.String("consistency", consistency))

	// Write to all nodes (replicas + dual-write nodes) in parallel
	responses := make([]*client.StorageResponse, 0, len(allNodes))
	var mu sync.Mutex

	g, gctx := errgroup.WithContext(ctx)

	for _, node := range allNodes {
		node := node // Capture loop variable
		isDualWrite := dualWriteNodes[node.NodeID]

		g.Go(func() error {
			resp, err := s.storageClient.Write(gctx, node, tenantID, key, value, vectorClock)
			if err != nil {
				if isDualWrite {
					// Dual-write failures are less critical (log as debug)
					s.logger.Debug("Dual-write failed to migration node (non-critical)",
						zap.String("tenant_id", tenantID),
						zap.String("key", key),
						zap.String("node_id", node.NodeID),
						zap.Error(err))
				} else {
					s.logger.Warn("Write failed to replica",
						zap.String("tenant_id", tenantID),
						zap.String("key", key),
						zap.String("node_id", node.NodeID),
						zap.Error(err))
				}

				// Don't return error, collect response
				// Dual-write failures don't count toward quorum
				if !isDualWrite {
					mu.Lock()
					responses = append(responses, &client.StorageResponse{
						NodeID:  node.NodeID,
						Success: false,
						Error:   err,
					})
					mu.Unlock()
				}
				return nil
			}

			// Collect successful responses (dual-write success doesn't count toward quorum)
			if !isDualWrite {
				mu.Lock()
				responses = append(responses, resp)
				mu.Unlock()
			} else {
				s.logger.Debug("Dual-write succeeded to migration node",
					zap.String("tenant_id", tenantID),
					zap.String("key", key),
					zap.String("node_id", node.NodeID))
			}
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
		zap.Int("required_replicas", requiredReplicas))

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

// readFromReplicas reads from multiple replicas in parallel
func (s *CoordinatorService) readFromReplicas(
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
		zap.Int("required_replicas", requiredReplicas),
		zap.String("consistency", consistency))

	// Read from replicas in parallel
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
				// Don't return error, collect response
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

	// Wait for all reads to complete or timeout
	_ = g.Wait()

	// Count successful reads
	successCount := 0
	for _, resp := range responses {
		if resp.Success && resp.Found {
			successCount++
		}
	}

	s.logger.Info("Read completed",
		zap.String("tenant_id", tenantID),
		zap.String("key", key),
		zap.Int("success_count", successCount),
		zap.Int("required_replicas", requiredReplicas))

	// Check if quorum is reached
	if !s.consistencyService.IsQuorumReached(successCount, len(replicas), consistency) {
		return &ReadResult{
			Success:      false,
			Key:          key,
			ErrorMessage: fmt.Sprintf("quorum not reached: %d/%d", successCount, requiredReplicas),
		}, fmt.Errorf("quorum not reached: %d/%d", successCount, requiredReplicas)
	}

	// Detect conflicts and get latest value
	latest, hasConflict := s.conflictService.DetectConflicts(responses)
	if latest == nil {
		return &ReadResult{
			Success:      false,
			Key:          key,
			ErrorMessage: "key not found",
		}, fmt.Errorf("key not found")
	}

	if hasConflict {
		s.logger.Warn("Conflict detected during read",
			zap.String("tenant_id", tenantID),
			zap.String("key", key))

		// Trigger repair
		if err := s.conflictService.TriggerRepair(ctx, tenantID, key, latest, replicas); err != nil {
			s.logger.Error("Failed to trigger repair",
				zap.String("tenant_id", tenantID),
				zap.String("key", key),
				zap.Error(err))
		}
	}

	return &ReadResult{
		Success:     true,
		Key:         key,
		Value:       latest.Value,
		VectorClock: latest.VectorClock,
	}, nil
}

// readExistingValue is a helper method to read the existing value and vector clock
// Used for read-modify-write operations to maintain causality
//
// Edge Case Handling:
// - During node addition, may read from bootstrapping node with incomplete data
// - If stale data is read, vector clock will be older, creating a causal anomaly
// - This is resolved by conflict detection and read repair on subsequent reads
// - The system converges to consistency through gossip and anti-entropy
func (s *CoordinatorService) readExistingValue(
	ctx context.Context,
	replicas []*model.StorageNode,
	tenantID, key string,
) ([]byte, model.VectorClock, error) {
	// Use ONE consistency for internal read to minimize latency
	// We only need one response to get the existing vector clock
	// Note: Could use QUORUM for stronger guarantees at cost of latency
	ctx, cancel := context.WithTimeout(ctx, s.readTimeout)
	defer cancel()

	s.logger.Debug("Reading existing value for write operation",
		zap.String("tenant_id", tenantID),
		zap.String("key", key))

	// Read from first available replica
	var mu sync.Mutex
	var firstResponse *client.StorageResponse
	responseReceived := false

	g, gctx := errgroup.WithContext(ctx)

	for _, replica := range replicas {
		replica := replica // Capture loop variable
		g.Go(func() error {
			// Skip if we already got a response
			mu.Lock()
			if responseReceived {
				mu.Unlock()
				return nil
			}
			mu.Unlock()

			resp, err := s.storageClient.Read(gctx, replica, tenantID, key)
			if err != nil {
				s.logger.Debug("Failed to read from replica",
					zap.String("node_id", replica.NodeID),
					zap.Error(err))
				return nil // Don't fail, try other replicas
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

	// Wait for all or first successful read
	_ = g.Wait()

	if firstResponse == nil || !firstResponse.Found {
		return nil, model.VectorClock{}, fmt.Errorf("key not found")
	}

	s.logger.Debug("Found existing value",
		zap.String("tenant_id", tenantID),
		zap.String("key", key),
		zap.String("node_id", firstResponse.NodeID),
		zap.Int("value_size", len(firstResponse.Value)))

	return firstResponse.Value, firstResponse.VectorClock, nil
}
