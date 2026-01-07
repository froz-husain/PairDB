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
	replicas, err := s.routingService.GetReplicas(tenantID, key, tenant.ReplicationFactor)
	if err != nil {
		return nil, fmt.Errorf("failed to get replicas: %w", err)
	}

	// Generate new vector clock
	vectorClock := s.vcService.Increment(tenantID)

	// Write to replicas
	result, err := s.writeToReplicas(ctx, replicas, tenantID, key, value, vectorClock, consistency)
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
	replicas, err := s.routingService.GetReplicas(tenantID, key, tenant.ReplicationFactor)
	if err != nil {
		return nil, fmt.Errorf("failed to get replicas: %w", err)
	}

	// Read from replicas
	return s.readFromReplicas(ctx, replicas, tenantID, key, consistency)
}

// writeToReplicas writes to multiple replicas in parallel
func (s *CoordinatorService) writeToReplicas(
	ctx context.Context,
	replicas []*model.StorageNode,
	tenantID, key string,
	value []byte,
	vectorClock model.VectorClock,
	consistency string,
) (*WriteResult, error) {
	ctx, cancel := context.WithTimeout(ctx, s.writeTimeout)
	defer cancel()

	requiredReplicas := s.consistencyService.GetRequiredReplicas(consistency, len(replicas))

	s.logger.Info("Writing to replicas",
		zap.String("tenant_id", tenantID),
		zap.String("key", key),
		zap.Int("total_replicas", len(replicas)),
		zap.Int("required_replicas", requiredReplicas),
		zap.String("consistency", consistency))

	// Write to replicas in parallel
	responses := make([]*client.StorageResponse, 0, len(replicas))
	var mu sync.Mutex

	g, gctx := errgroup.WithContext(ctx)

	for _, replica := range replicas {
		replica := replica // Capture loop variable
		g.Go(func() error {
			resp, err := s.storageClient.Write(gctx, replica, tenantID, key, value, vectorClock)
			if err != nil {
				s.logger.Warn("Write failed to replica",
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
