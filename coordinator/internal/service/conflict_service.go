package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/devrev/pairdb/coordinator/internal/client"
	"github.com/devrev/pairdb/coordinator/internal/model"
	"go.uber.org/zap"
)

// RepairRequest represents a repair operation
type RepairRequest struct {
	TenantID    string
	Key         string
	Value       []byte
	VectorClock model.VectorClock
	Timestamp   int64
	Replicas    []*model.StorageNode
}

// ConflictService detects and repairs data conflicts
type ConflictService struct {
	storageClient  *client.StorageClient
	vcService      *VectorClockService
	repairQueue    chan *RepairRequest
	repairWorkers  int
	repairAsync    bool
	logger         *zap.Logger
	wg             sync.WaitGroup
	stopCh         chan struct{}
}

// NewConflictService creates a new conflict service
func NewConflictService(
	storageClient *client.StorageClient,
	vcService *VectorClockService,
	repairWorkers int,
	repairAsync bool,
	logger *zap.Logger,
) *ConflictService {
	cs := &ConflictService{
		storageClient: storageClient,
		vcService:     vcService,
		repairQueue:   make(chan *RepairRequest, 1000),
		repairWorkers: repairWorkers,
		repairAsync:   repairAsync,
		logger:        logger,
		stopCh:        make(chan struct{}),
	}

	// Start repair workers
	for i := 0; i < repairWorkers; i++ {
		cs.wg.Add(1)
		go cs.repairWorker(i)
	}

	return cs
}

// DetectConflicts detects conflicts in read responses and returns the latest value
func (s *ConflictService) DetectConflicts(responses []*client.StorageResponse) (*client.StorageResponse, bool) {
	if len(responses) == 0 {
		return nil, false
	}

	// Find the response with the highest vector clock
	var latest *client.StorageResponse
	hasConflict := false

	for _, resp := range responses {
		if !resp.Success || !resp.Found {
			continue
		}

		if latest == nil {
			latest = resp
			continue
		}

		// Compare vector clocks
		comparison := s.vcService.Compare(resp.VectorClock, latest.VectorClock)

		switch comparison {
		case model.VectorClockAfter:
			// resp is after latest, update latest
			latest = resp
		case model.VectorClockBefore:
			// resp is before latest, keep latest
			continue
		case model.VectorClockConcurrent:
			// Concurrent writes - conflict detected
			hasConflict = true
			// Use timestamp as tiebreaker
			if resp.Timestamp > latest.Timestamp {
				latest = resp
			}
		case model.VectorClockEqual:
			// Same version, no conflict
			continue
		}
	}

	if latest == nil {
		return nil, false
	}

	return latest, hasConflict
}

// TriggerRepair triggers a repair operation for inconsistent replicas
func (s *ConflictService) TriggerRepair(
	ctx context.Context,
	tenantID, key string,
	latest *client.StorageResponse,
	replicas []*model.StorageNode,
) error {
	// Find replicas that need repair
	needsRepair := make([]*model.StorageNode, 0)

	for _, replica := range replicas {
		// Check if this replica returned a response
		found := false
		for _, resp := range []*client.StorageResponse{latest} {
			if resp.NodeID == replica.NodeID {
				found = true
				break
			}
		}

		// If this replica didn't return the latest, it needs repair
		if !found {
			needsRepair = append(needsRepair, replica)
		}
	}

	if len(needsRepair) == 0 {
		s.logger.Debug("No replicas need repair",
			zap.String("tenant_id", tenantID),
			zap.String("key", key))
		return nil
	}

	repairReq := &RepairRequest{
		TenantID:    tenantID,
		Key:         key,
		Value:       latest.Value,
		VectorClock: latest.VectorClock,
		Timestamp:   latest.Timestamp,
		Replicas:    needsRepair,
	}

	if s.repairAsync {
		// Async repair
		select {
		case s.repairQueue <- repairReq:
			s.logger.Info("Queued repair request",
				zap.String("tenant_id", tenantID),
				zap.String("key", key),
				zap.Int("replicas_to_repair", len(needsRepair)))
		default:
			s.logger.Warn("Repair queue full, dropping repair request",
				zap.String("tenant_id", tenantID),
				zap.String("key", key))
			return fmt.Errorf("repair queue full")
		}
		return nil
	}

	// Sync repair
	return s.executeRepair(ctx, repairReq)
}

// repairWorker processes repair requests from the queue
func (s *ConflictService) repairWorker(workerID int) {
	defer s.wg.Done()

	s.logger.Info("Repair worker started", zap.Int("worker_id", workerID))

	for {
		select {
		case repairReq := <-s.repairQueue:
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			if err := s.executeRepair(ctx, repairReq); err != nil {
				s.logger.Error("Repair failed",
					zap.Int("worker_id", workerID),
					zap.String("tenant_id", repairReq.TenantID),
					zap.String("key", repairReq.Key),
					zap.Error(err))
			}
			cancel()
		case <-s.stopCh:
			s.logger.Info("Repair worker stopped", zap.Int("worker_id", workerID))
			return
		}
	}
}

// executeRepair executes the actual repair operation
func (s *ConflictService) executeRepair(ctx context.Context, req *RepairRequest) error {
	s.logger.Info("Executing repair",
		zap.String("tenant_id", req.TenantID),
		zap.String("key", req.Key),
		zap.Int("replicas", len(req.Replicas)))

	successCount := 0
	errorCount := 0

	// Send repair requests to all replicas in parallel
	var wg sync.WaitGroup
	for _, replica := range req.Replicas {
		wg.Add(1)
		go func(node *model.StorageNode) {
			defer wg.Done()

			err := s.storageClient.Repair(
				ctx,
				node,
				req.TenantID,
				req.Key,
				req.Value,
				req.VectorClock,
				req.Timestamp,
			)

			if err != nil {
				s.logger.Error("Repair failed for replica",
					zap.String("tenant_id", req.TenantID),
					zap.String("key", req.Key),
					zap.String("node_id", node.NodeID),
					zap.Error(err))
				errorCount++
			} else {
				s.logger.Debug("Repair successful for replica",
					zap.String("tenant_id", req.TenantID),
					zap.String("key", req.Key),
					zap.String("node_id", node.NodeID))
				successCount++
			}
		}(replica)
	}

	wg.Wait()

	if errorCount > 0 {
		s.logger.Warn("Repair completed with errors",
			zap.String("tenant_id", req.TenantID),
			zap.String("key", req.Key),
			zap.Int("success", successCount),
			zap.Int("errors", errorCount))
		return fmt.Errorf("repair completed with %d errors out of %d replicas", errorCount, len(req.Replicas))
	}

	s.logger.Info("Repair completed successfully",
		zap.String("tenant_id", req.TenantID),
		zap.String("key", req.Key),
		zap.Int("replicas_repaired", successCount))

	return nil
}

// Stop stops the conflict service
func (s *ConflictService) Stop() {
	close(s.stopCh)
	s.wg.Wait()
	s.logger.Info("Conflict service stopped")
}

// GetQueueSize returns the current size of the repair queue
func (s *ConflictService) GetQueueSize() int {
	return len(s.repairQueue)
}
