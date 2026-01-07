package handler

import (
	"context"
	"errors"

	"github.com/devrev/pairdb/coordinator/internal/model"
	"github.com/devrev/pairdb/coordinator/internal/service"
	"github.com/devrev/pairdb/coordinator/internal/store"
	pb "github.com/devrev/pairdb/coordinator/pkg/proto"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// NodeHandler handles storage node management operations
type NodeHandler struct {
	pb.UnimplementedCoordinatorServiceServer
	metadataStore  store.MetadataStore
	routingService *service.RoutingService
	logger         *zap.Logger
}

// NewNodeHandler creates a new node handler
func NewNodeHandler(
	metadataStore store.MetadataStore,
	routingService *service.RoutingService,
	logger *zap.Logger,
) *NodeHandler {
	return &NodeHandler{
		metadataStore:  metadataStore,
		routingService: routingService,
		logger:         logger,
	}
}

// AddStorageNode handles storage node addition requests
func (h *NodeHandler) AddStorageNode(
	ctx context.Context,
	req *pb.AddStorageNodeRequest,
) (*pb.AddStorageNodeResponse, error) {
	// Validate request
	if err := h.validateAddStorageNodeRequest(req); err != nil {
		h.logger.Warn("Invalid add storage node request", zap.Error(err))
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	h.logger.Info("Received add storage node request",
		zap.String("node_id", req.NodeId),
		zap.String("host", req.Host),
		zap.Int32("port", req.Port))

	// Default virtual nodes
	virtualNodes := int(req.VirtualNodes)
	if virtualNodes == 0 {
		virtualNodes = 150
	}

	// Create storage node
	node := &model.StorageNode{
		NodeID:       req.NodeId,
		Host:         req.Host,
		Port:         int(req.Port),
		Status:       "active",
		VirtualNodes: virtualNodes,
	}

	// Add to metadata store
	if err := h.metadataStore.AddStorageNode(ctx, node); err != nil {
		h.logger.Error("Failed to add storage node",
			zap.String("node_id", req.NodeId),
			zap.Error(err))
		return nil, status.Error(codes.Internal, err.Error())
	}

	// Add to routing service
	if err := h.routingService.AddNode(ctx, node); err != nil {
		h.logger.Error("Failed to add node to routing service",
			zap.String("node_id", req.NodeId),
			zap.Error(err))
	}

	resp := &pb.AddStorageNodeResponse{
		Success: true,
		NodeId:  req.NodeId,
		Message: "Storage node added successfully",
	}

	h.logger.Info("Storage node added",
		zap.String("node_id", req.NodeId))

	return resp, nil
}

// RemoveStorageNode handles storage node removal requests
func (h *NodeHandler) RemoveStorageNode(
	ctx context.Context,
	req *pb.RemoveStorageNodeRequest,
) (*pb.RemoveStorageNodeResponse, error) {
	// Validate request
	if req.NodeId == "" {
		return nil, status.Error(codes.InvalidArgument, "node_id is required")
	}

	h.logger.Info("Received remove storage node request",
		zap.String("node_id", req.NodeId),
		zap.Bool("force", req.Force))

	// Remove from routing service first
	if err := h.routingService.RemoveNode(ctx, req.NodeId); err != nil {
		h.logger.Error("Failed to remove node from routing service",
			zap.String("node_id", req.NodeId),
			zap.Error(err))
	}

	// Remove from metadata store
	if err := h.metadataStore.RemoveStorageNode(ctx, req.NodeId); err != nil {
		h.logger.Error("Failed to remove storage node",
			zap.String("node_id", req.NodeId),
			zap.Error(err))
		return nil, status.Error(codes.Internal, err.Error())
	}

	resp := &pb.RemoveStorageNodeResponse{
		Success: true,
		NodeId:  req.NodeId,
		Message: "Storage node removed successfully",
	}

	h.logger.Info("Storage node removed",
		zap.String("node_id", req.NodeId))

	return resp, nil
}

// GetMigrationStatus handles migration status requests
func (h *NodeHandler) GetMigrationStatus(
	ctx context.Context,
	req *pb.GetMigrationStatusRequest,
) (*pb.GetMigrationStatusResponse, error) {
	// Validate request
	if req.MigrationId == "" {
		return nil, status.Error(codes.InvalidArgument, "migration_id is required")
	}

	h.logger.Info("Received get migration status request",
		zap.String("migration_id", req.MigrationId))

	// Get migration from metadata store
	migration, err := h.metadataStore.GetMigration(ctx, req.MigrationId)
	if err != nil {
		h.logger.Error("Failed to get migration",
			zap.String("migration_id", req.MigrationId),
			zap.Error(err))
		return nil, status.Error(codes.NotFound, "migration not found")
	}

	resp := &pb.GetMigrationStatusResponse{
		Success:     true,
		MigrationId: migration.MigrationID,
		Type:        migration.Type,
		NodeId:      migration.NodeID,
		Status:      migration.Status,
		StartedAt:   migration.StartedAt.Unix(),
	}

	return resp, nil
}

// ListStorageNodes handles list storage nodes requests
func (h *NodeHandler) ListStorageNodes(
	ctx context.Context,
	req *pb.ListStorageNodesRequest,
) (*pb.ListStorageNodesResponse, error) {
	h.logger.Info("Received list storage nodes request")

	// List nodes from metadata store
	nodes, err := h.metadataStore.ListStorageNodes(ctx)
	if err != nil {
		h.logger.Error("Failed to list storage nodes", zap.Error(err))
		return nil, status.Error(codes.Internal, err.Error())
	}

	// Convert to proto
	protoNodes := make([]*pb.StorageNodeInfo, len(nodes))
	for i, node := range nodes {
		protoNodes[i] = &pb.StorageNodeInfo{
			NodeId:       node.NodeID,
			Host:         node.Host,
			Port:         int32(node.Port),
			Status:       node.Status,
			VirtualNodes: int32(node.VirtualNodes),
		}
	}

	resp := &pb.ListStorageNodesResponse{
		Success: true,
		Nodes:   protoNodes,
	}

	h.logger.Info("Listed storage nodes",
		zap.Int("count", len(nodes)))

	return resp, nil
}

// validateAddStorageNodeRequest validates add storage node request
func (h *NodeHandler) validateAddStorageNodeRequest(req *pb.AddStorageNodeRequest) error {
	if req.NodeId == "" {
		return errors.New("node_id is required")
	}
	if req.Host == "" {
		return errors.New("host is required")
	}
	if req.Port <= 0 || req.Port > 65535 {
		return errors.New("port must be between 1 and 65535")
	}
	if req.VirtualNodes < 0 {
		return errors.New("virtual_nodes cannot be negative")
	}
	return nil
}
