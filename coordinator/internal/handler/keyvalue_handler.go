package handler

import (
	"context"
	"errors"

	"github.com/devrev/pairdb/coordinator/internal/model"
	"github.com/devrev/pairdb/coordinator/internal/service"
	pb "github.com/devrev/pairdb/coordinator/pkg/proto"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// KeyValueHandler handles key-value operations
type KeyValueHandler struct {
	pb.UnimplementedCoordinatorServiceServer
	coordinatorService *service.CoordinatorService
	logger             *zap.Logger
}

// NewKeyValueHandler creates a new key-value handler
func NewKeyValueHandler(
	coordinatorService *service.CoordinatorService,
	logger *zap.Logger,
) *KeyValueHandler {
	return &KeyValueHandler{
		coordinatorService: coordinatorService,
		logger:             logger,
	}
}

// WriteKeyValue handles write requests
func (h *KeyValueHandler) WriteKeyValue(
	ctx context.Context,
	req *pb.WriteKeyValueRequest,
) (*pb.WriteKeyValueResponse, error) {
	// Validate request
	if err := h.validateWriteRequest(req); err != nil {
		h.logger.Warn("Invalid write request", zap.Error(err))
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	h.logger.Info("Received write request",
		zap.String("tenant_id", req.TenantId),
		zap.String("key", req.Key),
		zap.String("consistency", req.Consistency),
		zap.Int("value_size", len(req.Value)))

	// Execute write
	result, err := h.coordinatorService.WriteKeyValue(
		ctx,
		req.TenantId,
		req.Key,
		req.Value,
		req.Consistency,
		req.IdempotencyKey,
	)

	if err != nil {
		h.logger.Error("Write failed",
			zap.String("tenant_id", req.TenantId),
			zap.String("key", req.Key),
			zap.Error(err))

		// Map errors to gRPC status codes
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, status.Error(codes.DeadlineExceeded, "write timeout")
		}
		if errors.Is(err, context.Canceled) {
			return nil, status.Error(codes.Canceled, "write canceled")
		}
		return nil, status.Error(codes.Internal, err.Error())
	}

	// Convert result to proto response
	resp := &pb.WriteKeyValueResponse{
		Success:      result.Success,
		Key:          result.Key,
		IdempotencyKey: req.IdempotencyKey,
		VectorClock:  h.toProtoVectorClock(result.VectorClock),
		ReplicaCount: result.ReplicaCount,
		Consistency:  result.Consistency,
		IsDuplicate:  result.IsDuplicate,
		ErrorMessage: result.ErrorMessage,
	}

	h.logger.Info("Write completed",
		zap.String("tenant_id", req.TenantId),
		zap.String("key", req.Key),
		zap.Bool("success", result.Success),
		zap.Int32("replica_count", result.ReplicaCount))

	return resp, nil
}

// ReadKeyValue handles read requests
func (h *KeyValueHandler) ReadKeyValue(
	ctx context.Context,
	req *pb.ReadKeyValueRequest,
) (*pb.ReadKeyValueResponse, error) {
	// Validate request
	if err := h.validateReadRequest(req); err != nil {
		h.logger.Warn("Invalid read request", zap.Error(err))
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	h.logger.Info("Received read request",
		zap.String("tenant_id", req.TenantId),
		zap.String("key", req.Key),
		zap.String("consistency", req.Consistency))

	// Execute read
	result, err := h.coordinatorService.ReadKeyValue(
		ctx,
		req.TenantId,
		req.Key,
		req.Consistency,
	)

	if err != nil {
		h.logger.Error("Read failed",
			zap.String("tenant_id", req.TenantId),
			zap.String("key", req.Key),
			zap.Error(err))

		// Map errors to gRPC status codes
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, status.Error(codes.DeadlineExceeded, "read timeout")
		}
		if errors.Is(err, context.Canceled) {
			return nil, status.Error(codes.Canceled, "read canceled")
		}
		return nil, status.Error(codes.Internal, err.Error())
	}

	// Convert result to proto response
	resp := &pb.ReadKeyValueResponse{
		Success:      result.Success,
		Key:          result.Key,
		Value:        result.Value,
		VectorClock:  h.toProtoVectorClock(result.VectorClock),
		ErrorMessage: result.ErrorMessage,
	}

	h.logger.Info("Read completed",
		zap.String("tenant_id", req.TenantId),
		zap.String("key", req.Key),
		zap.Bool("success", result.Success),
		zap.Int("value_size", len(result.Value)))

	return resp, nil
}

// validateWriteRequest validates write request parameters
func (h *KeyValueHandler) validateWriteRequest(req *pb.WriteKeyValueRequest) error {
	if req.TenantId == "" {
		return errors.New("tenant_id is required")
	}
	if req.Key == "" {
		return errors.New("key is required")
	}
	if len(req.Value) == 0 {
		return errors.New("value is required")
	}
	if len(req.Value) > 1024*1024 { // 1MB limit
		return errors.New("value size exceeds limit (1MB)")
	}
	if req.Consistency != "" && req.Consistency != "one" && req.Consistency != "quorum" && req.Consistency != "all" {
		return errors.New("invalid consistency level: must be one of: one, quorum, all")
	}
	return nil
}

// validateReadRequest validates read request parameters
func (h *KeyValueHandler) validateReadRequest(req *pb.ReadKeyValueRequest) error {
	if req.TenantId == "" {
		return errors.New("tenant_id is required")
	}
	if req.Key == "" {
		return errors.New("key is required")
	}
	if req.Consistency != "" && req.Consistency != "one" && req.Consistency != "quorum" && req.Consistency != "all" {
		return errors.New("invalid consistency level: must be one of: one, quorum, all")
	}
	return nil
}

// toProtoVectorClock converts internal vector clock to proto
func (h *KeyValueHandler) toProtoVectorClock(vc model.VectorClock) *pb.VectorClock {
	entries := make([]*pb.VectorClockEntry, len(vc.Entries))
	for i, entry := range vc.Entries {
		entries[i] = &pb.VectorClockEntry{
			CoordinatorNodeId: entry.CoordinatorNodeID,
			LogicalTimestamp:  entry.LogicalTimestamp,
		}
	}
	return &pb.VectorClock{Entries: entries}
}
