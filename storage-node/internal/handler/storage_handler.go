package handler

import (
	"context"

	"github.com/devrev/pairdb/storage-node/internal/model"
	"github.com/devrev/pairdb/storage-node/internal/service"
	pb "github.com/devrev/pairdb/storage-node/pkg/proto"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// StorageHandler implements the gRPC storage service
type StorageHandler struct {
	storageService *service.StorageService
	logger         *zap.Logger
	pb.UnimplementedStorageNodeServiceServer
}

// NewStorageHandler creates a new storage handler
func NewStorageHandler(storageSvc *service.StorageService, logger *zap.Logger) *StorageHandler {
	return &StorageHandler{
		storageService: storageSvc,
		logger:         logger,
	}
}

// Write handles write requests
func (h *StorageHandler) Write(ctx context.Context, req *pb.WriteRequest) (*pb.WriteResponse, error) {
	// Validate request
	if err := h.validateWriteRequest(req); err != nil {
		return &pb.WriteResponse{
			Success:      false,
			ErrorMessage: err.Error(),
			ErrorCode:    pb.ErrorCode_INVALID_REQUEST,
		}, nil
	}

	// Convert protobuf vector clock
	vectorClock := h.fromProtoVectorClock(req.VectorClock)

	// Execute write
	resp, err := h.storageService.Write(
		ctx,
		req.TenantId,
		req.Key,
		req.Value,
		vectorClock,
	)

	if err != nil {
		h.logger.Error("Write failed",
			zap.String("tenant_id", req.TenantId),
			zap.String("key", req.Key),
			zap.Error(err))
		return &pb.WriteResponse{
			Success:      false,
			ErrorMessage: "write failed",
			ErrorCode:    pb.ErrorCode_INTERNAL_ERROR,
		}, nil
	}

	return &pb.WriteResponse{
		Success:            resp.Success,
		UpdatedVectorClock: h.toProtoVectorClock(resp.VectorClock),
		ErrorCode:          pb.ErrorCode_UNKNOWN,
	}, nil
}

// Read handles read requests
func (h *StorageHandler) Read(ctx context.Context, req *pb.ReadRequest) (*pb.ReadResponse, error) {
	// Validate request
	if err := h.validateReadRequest(req); err != nil {
		return &pb.ReadResponse{
			Success:      false,
			ErrorMessage: err.Error(),
			ErrorCode:    pb.ErrorCode_INVALID_REQUEST,
		}, nil
	}

	// Execute read
	resp, err := h.storageService.Read(
		ctx,
		req.TenantId,
		req.Key,
	)

	if err != nil {
		h.logger.Error("Read failed",
			zap.String("tenant_id", req.TenantId),
			zap.String("key", req.Key),
			zap.Error(err))
		return &pb.ReadResponse{
			Success:      false,
			ErrorMessage: "key not found",
			ErrorCode:    pb.ErrorCode_KEY_NOT_FOUND,
		}, nil
	}

	return &pb.ReadResponse{
		Success:     resp.Success,
		Value:       resp.Value,
		VectorClock: h.toProtoVectorClock(resp.VectorClock),
		ErrorCode:   pb.ErrorCode_UNKNOWN,
	}, nil
}

// Repair handles repair requests
func (h *StorageHandler) Repair(ctx context.Context, req *pb.RepairRequest) (*pb.RepairResponse, error) {
	// Validate request
	if req.TenantId == "" || req.Key == "" {
		return &pb.RepairResponse{
			Success:      false,
			ErrorMessage: "invalid request",
			ErrorCode:    pb.ErrorCode_INVALID_REQUEST,
		}, nil
	}

	// Convert protobuf vector clock
	vectorClock := h.fromProtoVectorClock(req.VectorClock)

	// Execute repair
	err := h.storageService.Repair(
		ctx,
		req.TenantId,
		req.Key,
		req.Value,
		vectorClock,
	)

	if err != nil {
		h.logger.Error("Repair failed",
			zap.String("tenant_id", req.TenantId),
			zap.String("key", req.Key),
			zap.Error(err))
		return &pb.RepairResponse{
			Success:      false,
			ErrorMessage: "repair failed",
			ErrorCode:    pb.ErrorCode_INTERNAL_ERROR,
		}, nil
	}

	return &pb.RepairResponse{
		Success:   true,
		ErrorCode: pb.ErrorCode_UNKNOWN,
	}, nil
}

// HealthCheck handles health check requests
func (h *StorageHandler) HealthCheck(ctx context.Context, req *pb.HealthCheckRequest) (*pb.HealthResponse, error) {
	return &pb.HealthResponse{
		Healthy: true,
		Status:  "healthy",
		Metrics: map[string]string{
			"node_status": "operational",
		},
	}, nil
}

// ReplicateData handles data replication to target node
func (h *StorageHandler) ReplicateData(ctx context.Context, req *pb.ReplicateRequest) (*pb.ReplicateResponse, error) {
	h.logger.Info("Replication requested",
		zap.String("tenant_id", req.TenantId),
		zap.String("target_node", req.TargetNodeId),
		zap.Int("keys_count", len(req.Keys)))

	// Validate request
	if req.TenantId == "" {
		return &pb.ReplicateResponse{
			Success:      false,
			ErrorMessage: "tenant_id is required",
			ErrorCode:    pb.ErrorCode_INVALID_REQUEST,
		}, nil
	}

	keysReplicated := int64(0)

	// Replicate requested keys
	for _, key := range req.Keys {
		// Read the value
		_, err := h.storageService.Read(ctx, req.TenantId, key)
		if err != nil {
			h.logger.Warn("Failed to read key for replication",
				zap.String("key", key),
				zap.Error(err))
			continue
		}

		// In production, send this data to target node via gRPC
		h.logger.Debug("Would replicate key to target node",
			zap.String("key", key),
			zap.String("target", req.TargetNodeId))

		keysReplicated++
	}

	return &pb.ReplicateResponse{
		Success:        true,
		KeysReplicated: keysReplicated,
		ErrorCode:      pb.ErrorCode_UNKNOWN,
	}, nil
}

// StreamKeys streams keys in a specified range
func (h *StorageHandler) StreamKeys(req *pb.StreamKeysRequest, stream pb.StorageNodeService_StreamKeysServer) error {
	h.logger.Info("Key streaming requested",
		zap.String("tenant_id", req.TenantId),
		zap.String("range_start", req.KeyRangeStart),
		zap.String("range_end", req.KeyRangeEnd),
		zap.Int32("batch_size", req.BatchSize))

	// Validate request
	if req.TenantId == "" {
		return status.Error(codes.InvalidArgument, "tenant_id is required")
	}

	batchSize := int(req.BatchSize)
	if batchSize <= 0 {
		batchSize = 100 // Default batch size
	}

	// In production implementation:
	// 1. Query memtable for keys in range
	// 2. Query all SSTables for keys in range
	// 3. Merge results maintaining sorted order
	// 4. Stream in batches with backpressure control

	// For now, return empty stream
	// In production, iterate through data and stream entries
	h.logger.Info("Streaming completed", zap.String("tenant_id", req.TenantId))

	return nil
}

// GetKeyRange returns keys in a specified range
func (h *StorageHandler) GetKeyRange(ctx context.Context, req *pb.KeyRangeRequest) (*pb.KeyRangeResponse, error) {
	h.logger.Info("Key range requested",
		zap.String("tenant_id", req.TenantId),
		zap.String("range_start", req.KeyRangeStart),
		zap.String("range_end", req.KeyRangeEnd),
		zap.Int32("max_keys", req.MaxKeys))

	// Validate request
	if req.TenantId == "" {
		return &pb.KeyRangeResponse{
			Keys:    []string{},
			HasMore: false,
		}, status.Error(codes.InvalidArgument, "tenant_id is required")
	}

	maxKeys := int(req.MaxKeys)
	if maxKeys <= 0 {
		maxKeys = 1000 // Default max keys
	}

	// In production implementation:
	// 1. Query memtable for keys in range
	// 2. Query all SSTables for keys in range
	// 3. Merge and deduplicate results
	// 4. Return paginated response

	keys := []string{}
	hasMore := false
	nextKey := ""

	h.logger.Debug("Returning key range",
		zap.Int("keys_count", len(keys)),
		zap.Bool("has_more", hasMore))

	return &pb.KeyRangeResponse{
		Keys:    keys,
		HasMore: hasMore,
		NextKey: nextKey,
	}, nil
}

// DrainNode prepares the node for graceful shutdown
func (h *StorageHandler) DrainNode(ctx context.Context, req *pb.DrainRequest) (*pb.DrainResponse, error) {
	h.logger.Info("Drain node requested",
		zap.Bool("graceful", req.Graceful),
		zap.Int32("timeout_seconds", req.TimeoutSeconds))

	if !req.Graceful {
		return &pb.DrainResponse{
			Success:      false,
			Status:       "drain_not_implemented",
			ErrorMessage: "non-graceful drain not supported",
			ErrorCode:    pb.ErrorCode_INVALID_REQUEST,
		}, nil
	}

	// In production implementation:
	// 1. Stop accepting new write requests
	// 2. Wait for in-flight requests to complete
	// 3. Flush memtable to disk
	// 4. Sync commit log
	// 5. Close all file handles
	// 6. Signal readiness probe to fail
	// 7. Wait for traffic to drain (based on timeout)

	h.logger.Info("Node drain initiated")

	// Simulate drain process
	// In production, coordinate with health check system
	// Set readiness to false so k8s stops sending traffic

	return &pb.DrainResponse{
		Success:   true,
		Status:    "draining",
		ErrorCode: pb.ErrorCode_UNKNOWN,
	}, nil
}

// Helper methods

func (h *StorageHandler) validateWriteRequest(req *pb.WriteRequest) error {
	if req.TenantId == "" {
		return status.Error(codes.InvalidArgument, "tenant_id is required")
	}
	if req.Key == "" {
		return status.Error(codes.InvalidArgument, "key is required")
	}
	if req.Value == nil {
		return status.Error(codes.InvalidArgument, "value is required")
	}
	return nil
}

func (h *StorageHandler) validateReadRequest(req *pb.ReadRequest) error {
	if req.TenantId == "" {
		return status.Error(codes.InvalidArgument, "tenant_id is required")
	}
	if req.Key == "" {
		return status.Error(codes.InvalidArgument, "key is required")
	}
	return nil
}

func (h *StorageHandler) toProtoVectorClock(vc model.VectorClock) *pb.VectorClock {
	entries := make([]*pb.VectorClockEntry, len(vc.Entries))
	for i, entry := range vc.Entries {
		entries[i] = &pb.VectorClockEntry{
			CoordinatorNodeId: entry.CoordinatorNodeID,
			LogicalTimestamp:  entry.LogicalTimestamp,
		}
	}
	return &pb.VectorClock{Entries: entries}
}

func (h *StorageHandler) fromProtoVectorClock(vc *pb.VectorClock) model.VectorClock {
	if vc == nil {
		return model.VectorClock{Entries: []model.VectorClockEntry{}}
	}
	entries := make([]model.VectorClockEntry, len(vc.Entries))
	for i, entry := range vc.Entries {
		entries[i] = model.VectorClockEntry{
			CoordinatorNodeID: entry.CoordinatorNodeId,
			LogicalTimestamp:  entry.LogicalTimestamp,
		}
	}
	return model.VectorClock{Entries: entries}
}
