package handler

import (
	"context"
	"errors"

	"github.com/devrev/pairdb/coordinator/internal/service"
	pb "github.com/devrev/pairdb/coordinator/pkg/proto"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// TenantHandler handles tenant management operations
type TenantHandler struct {
	pb.UnimplementedCoordinatorServiceServer
	tenantService *service.TenantService
	logger        *zap.Logger
}

// NewTenantHandler creates a new tenant handler
func NewTenantHandler(
	tenantService *service.TenantService,
	logger *zap.Logger,
) *TenantHandler {
	return &TenantHandler{
		tenantService: tenantService,
		logger:        logger,
	}
}

// CreateTenant handles tenant creation requests
func (h *TenantHandler) CreateTenant(
	ctx context.Context,
	req *pb.CreateTenantRequest,
) (*pb.CreateTenantResponse, error) {
	// Validate request
	if err := h.validateCreateTenantRequest(req); err != nil {
		h.logger.Warn("Invalid create tenant request", zap.Error(err))
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	h.logger.Info("Received create tenant request",
		zap.String("tenant_id", req.TenantId),
		zap.Int32("replication_factor", req.ReplicationFactor))

	// Default replication factor
	replicationFactor := int(req.ReplicationFactor)
	if replicationFactor == 0 {
		replicationFactor = 3
	}

	// Create tenant
	tenant, err := h.tenantService.CreateTenant(ctx, req.TenantId, replicationFactor)
	if err != nil {
		h.logger.Error("Failed to create tenant",
			zap.String("tenant_id", req.TenantId),
			zap.Error(err))
		return nil, status.Error(codes.Internal, err.Error())
	}

	resp := &pb.CreateTenantResponse{
		Success:           true,
		TenantId:          tenant.TenantID,
		ReplicationFactor: int32(tenant.ReplicationFactor),
		CreatedAt:         tenant.CreatedAt.Unix(),
	}

	h.logger.Info("Tenant created",
		zap.String("tenant_id", tenant.TenantID),
		zap.Int("replication_factor", tenant.ReplicationFactor))

	return resp, nil
}

// UpdateReplicationFactor handles replication factor update requests
func (h *TenantHandler) UpdateReplicationFactor(
	ctx context.Context,
	req *pb.UpdateReplicationFactorRequest,
) (*pb.UpdateReplicationFactorResponse, error) {
	// Validate request
	if err := h.validateUpdateReplicationFactorRequest(req); err != nil {
		h.logger.Warn("Invalid update replication factor request", zap.Error(err))
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	h.logger.Info("Received update replication factor request",
		zap.String("tenant_id", req.TenantId),
		zap.Int32("new_replication_factor", req.NewReplicationFactor))

	// Update replication factor
	tenant, err := h.tenantService.UpdateReplicationFactor(
		ctx,
		req.TenantId,
		int(req.NewReplicationFactor),
	)
	if err != nil {
		h.logger.Error("Failed to update replication factor",
			zap.String("tenant_id", req.TenantId),
			zap.Error(err))
		return nil, status.Error(codes.Internal, err.Error())
	}

	resp := &pb.UpdateReplicationFactorResponse{
		Success:               true,
		TenantId:              tenant.TenantID,
		OldReplicationFactor:  int32(tenant.ReplicationFactor), // This should be old value
		NewReplicationFactor:  int32(tenant.ReplicationFactor),
		UpdatedAt:             tenant.UpdatedAt.Unix(),
	}

	h.logger.Info("Replication factor updated",
		zap.String("tenant_id", tenant.TenantID),
		zap.Int("new_replication_factor", tenant.ReplicationFactor))

	return resp, nil
}

// GetTenant handles tenant retrieval requests
func (h *TenantHandler) GetTenant(
	ctx context.Context,
	req *pb.GetTenantRequest,
) (*pb.GetTenantResponse, error) {
	// Validate request
	if req.TenantId == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id is required")
	}

	h.logger.Info("Received get tenant request", zap.String("tenant_id", req.TenantId))

	// Get tenant
	tenant, err := h.tenantService.GetTenant(ctx, req.TenantId)
	if err != nil {
		h.logger.Error("Failed to get tenant",
			zap.String("tenant_id", req.TenantId),
			zap.Error(err))
		return nil, status.Error(codes.NotFound, "tenant not found")
	}

	resp := &pb.GetTenantResponse{
		Success:           true,
		TenantId:          tenant.TenantID,
		ReplicationFactor: int32(tenant.ReplicationFactor),
		CreatedAt:         tenant.CreatedAt.Unix(),
		UpdatedAt:         tenant.UpdatedAt.Unix(),
	}

	return resp, nil
}

// validateCreateTenantRequest validates create tenant request
func (h *TenantHandler) validateCreateTenantRequest(req *pb.CreateTenantRequest) error {
	if req.TenantId == "" {
		return errors.New("tenant_id is required")
	}
	if req.ReplicationFactor < 0 || req.ReplicationFactor > 10 {
		return errors.New("replication_factor must be between 1 and 10")
	}
	return nil
}

// validateUpdateReplicationFactorRequest validates update replication factor request
func (h *TenantHandler) validateUpdateReplicationFactorRequest(req *pb.UpdateReplicationFactorRequest) error {
	if req.TenantId == "" {
		return errors.New("tenant_id is required")
	}
	if req.NewReplicationFactor < 1 || req.NewReplicationFactor > 10 {
		return errors.New("new_replication_factor must be between 1 and 10")
	}
	return nil
}
