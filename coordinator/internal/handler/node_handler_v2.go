package handler

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/devrev/pairdb/coordinator/internal/client"
	"github.com/devrev/pairdb/coordinator/internal/model"
	"github.com/devrev/pairdb/coordinator/internal/service"
	"github.com/devrev/pairdb/coordinator/internal/store"
	pb "github.com/devrev/pairdb/coordinator/pkg/proto"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// NodeHandlerV2 handles storage node management with Phase 2 streaming
type NodeHandlerV2 struct {
	pb.UnimplementedCoordinatorServiceServer
	metadataStore      store.MetadataStore
	routingService     *service.RoutingService
	keyRangeService    *service.KeyRangeService
	storageNodeClient  *client.StorageNodeClient
	logger             *zap.Logger
	streamPollInterval time.Duration
	topologyMu         sync.Mutex // Prevents concurrent topology changes
}

// NewNodeHandlerV2 creates a new Phase 2 node handler
func NewNodeHandlerV2(
	metadataStore store.MetadataStore,
	routingService *service.RoutingService,
	keyRangeService *service.KeyRangeService,
	storageNodeClient *client.StorageNodeClient,
	logger *zap.Logger,
) *NodeHandlerV2 {
	return &NodeHandlerV2{
		metadataStore:      metadataStore,
		routingService:     routingService,
		keyRangeService:    keyRangeService,
		storageNodeClient:  storageNodeClient,
		logger:             logger,
		streamPollInterval: 10 * time.Second,
	}
}

// AddStorageNode is the proto interface method that calls AddStorageNodeV2
func (h *NodeHandlerV2) AddStorageNode(
	ctx context.Context,
	req *pb.AddStorageNodeRequest,
) (*pb.AddStorageNodeResponse, error) {
	return h.AddStorageNodeV2(ctx, req)
}

// RemoveStorageNode is the proto interface method that calls RemoveStorageNodeV2
func (h *NodeHandlerV2) RemoveStorageNode(
	ctx context.Context,
	req *pb.RemoveStorageNodeRequest,
) (*pb.RemoveStorageNodeResponse, error) {
	return h.RemoveStorageNodeV2(ctx, req)
}

// AddStorageNodeV2 handles storage node addition with Phase 2 streaming
func (h *NodeHandlerV2) AddStorageNodeV2(
	ctx context.Context,
	req *pb.AddStorageNodeRequest,
) (*pb.AddStorageNodeResponse, error) {
	// Prevent concurrent topology changes (node addition/removal)
	h.topologyMu.Lock()
	defer h.topologyMu.Unlock()

	// Validate request
	if req.NodeId == "" {
		return nil, status.Error(codes.InvalidArgument, "node_id is required")
	}
	if req.Host == "" {
		return nil, status.Error(codes.InvalidArgument, "host is required")
	}
	if req.Port == 0 {
		return nil, status.Error(codes.InvalidArgument, "port is required")
	}

	h.logger.Info("Received add storage node request (V2 with streaming)",
		zap.String("node_id", req.NodeId),
		zap.String("host", req.Host),
		zap.Int32("port", req.Port))

	// Default virtual nodes
	virtualNodes := int(req.VirtualNodes)
	if virtualNodes == 0 {
		virtualNodes = 150
	}

	// Create storage node with bootstrapping status
	// Phase 2: Node is immediately added to ring and receives writes
	node := &model.StorageNode{
		NodeID:       req.NodeId,
		Host:         req.Host,
		Port:         int(req.Port),
		Status:       model.NodeStatusBootstrapping, // NEW: Bootstrapping status
		VirtualNodes: virtualNodes,
	}

	// Add to metadata store
	if err := h.metadataStore.AddStorageNode(ctx, node); err != nil {
		h.logger.Error("Failed to add storage node to metadata",
			zap.String("node_id", req.NodeId),
			zap.Error(err))
		return nil, status.Error(codes.Internal, err.Error())
	}

	// Add to hash ring IMMEDIATELY (Cassandra pattern)
	// New node will start receiving writes via Phase 1 hinted handoff
	if err := h.routingService.AddNode(ctx, node); err != nil {
		h.logger.Error("Failed to add node to routing service",
			zap.String("node_id", req.NodeId),
			zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to add to hash ring: "+err.Error())
	}

	h.logger.Info("Node added to hash ring, calculating key ranges",
		zap.String("node_id", req.NodeId))

	// Calculate which key ranges will move to new node
	hashRing := h.routingService.GetHashRing()
	rangeAssignments, err := h.keyRangeService.CalculateKeyRangesForNewNode(ctx, hashRing, node)
	if err != nil {
		h.logger.Error("Failed to calculate key ranges",
			zap.String("node_id", req.NodeId),
			zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to calculate key ranges: "+err.Error())
	}

	h.logger.Info("Key range calculation complete",
		zap.String("node_id", req.NodeId),
		zap.Int("affected_nodes", len(rangeAssignments)))

	// Initiate streaming from old nodes to new node
	streamingStarted := 0
	for oldNodeID, ranges := range rangeAssignments {
		// Get old node details
		oldNode, err := h.getStorageNode(ctx, oldNodeID)
		if err != nil {
			h.logger.Warn("Failed to get old node details",
				zap.String("old_node_id", oldNodeID),
				zap.Error(err))
			continue
		}

		// Convert ranges to client format
		clientRanges := make([]client.KeyRange, len(ranges))
		for i, r := range ranges {
			clientRanges[i] = client.KeyRange{
				StartHash: r.StartHash,
				EndHash:   r.EndHash,
			}
		}

		// Start streaming
		streamReq := &client.StartStreamingRequest{
			SourceNodeID: oldNodeID,
			TargetNodeID: node.NodeID,
			TargetHost:   node.Host,
			TargetPort:   node.Port,
			KeyRanges:    clientRanges,
		}

		resp, err := h.storageNodeClient.StartStreaming(ctx, oldNode, streamReq)
		if err != nil {
			h.logger.Error("Failed to start streaming",
				zap.String("old_node", oldNodeID),
				zap.String("new_node", node.NodeID),
				zap.Error(err))
			continue
		}

		if !resp.Success {
			h.logger.Warn("Streaming start unsuccessful",
				zap.String("old_node", oldNodeID),
				zap.String("new_node", node.NodeID),
				zap.String("message", resp.Message))
			continue
		}

		streamingStarted++
		h.logger.Info("Streaming started",
			zap.String("old_node", oldNodeID),
			zap.String("new_node", node.NodeID),
			zap.Int("key_ranges", len(ranges)))
	}

	// Start background task to monitor streaming progress
	go h.monitorStreamingProgress(context.Background(), node, rangeAssignments)

	return &pb.AddStorageNodeResponse{
		Success: true,
		NodeId:  node.NodeID,
		Message: fmt.Sprintf("Node added successfully with bootstrapping status. Streaming initiated from %d nodes.", streamingStarted),
	}, nil
}

// RemoveStorageNodeV2 handles storage node removal with Phase 2 streaming
func (h *NodeHandlerV2) RemoveStorageNodeV2(
	ctx context.Context,
	req *pb.RemoveStorageNodeRequest,
) (*pb.RemoveStorageNodeResponse, error) {
	// Prevent concurrent topology changes (node addition/removal)
	h.topologyMu.Lock()
	defer h.topologyMu.Unlock()

	// Validate request
	if req.NodeId == ""  {
		return nil, status.Error(codes.InvalidArgument, "node_id is required")
	}

	h.logger.Info("Received remove storage node request (V2 with streaming)",
		zap.String("node_id", req.NodeId),
		zap.Bool("force", req.Force))

	// Get the node to be removed
	removedNode, err := h.getStorageNode(ctx, req.NodeId)
	if err != nil {
		h.logger.Error("Failed to get storage node",
			zap.String("node_id", req.NodeId),
			zap.Error(err))
		return nil, status.Error(codes.NotFound, "node not found")
	}

	// Validate that there will be sufficient nodes after removal
	// Must have at least replication_factor nodes remaining
	allNodes, err := h.metadataStore.ListStorageNodes(ctx)
	if err != nil {
		h.logger.Error("Failed to list storage nodes",
			zap.String("node_id", req.NodeId),
			zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to validate node count")
	}

	activeNodeCount := 0
	for _, node := range allNodes {
		if node.Status == model.NodeStatusActive || node.Status == model.NodeStatusBootstrapping {
			activeNodeCount++
		}
	}

	// After removal, we need at least 3 nodes for typical replication (or 1 for testing)
	minNodesRequired := 3
	if !req.Force && activeNodeCount-1 < minNodesRequired {
		h.logger.Warn("Insufficient nodes after removal",
			zap.String("node_id", req.NodeId),
			zap.Int("current_nodes", activeNodeCount),
			zap.Int("min_required", minNodesRequired))
		return nil, status.Error(codes.FailedPrecondition,
			fmt.Sprintf("Cannot remove node: would leave only %d nodes, minimum %d required. Use force=true to override.",
				activeNodeCount-1, minNodesRequired))
	}

	// Update node status to draining
	if err := h.metadataStore.UpdateStorageNodeStatus(ctx, req.NodeId, string(model.NodeStatusDraining)); err != nil {
		h.logger.Error("Failed to update node status",
			zap.String("node_id", req.NodeId),
			zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to update node status")
	}

	h.logger.Info("Node marked as draining",
		zap.String("node_id", req.NodeId))

	// Calculate which nodes will inherit the key ranges
	hashRing := h.routingService.GetHashRing()
	rangeAssignments, err := h.keyRangeService.CalculateKeyRangesForRemovedNode(ctx, hashRing, removedNode)
	if err != nil {
		h.logger.Error("Failed to calculate key ranges for removal",
			zap.String("node_id", req.NodeId),
			zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to calculate key ranges")
	}

	h.logger.Info("Key range calculation complete for removal",
		zap.String("node_id", req.NodeId),
		zap.Int("inheriting_nodes", len(rangeAssignments)))

	// Initiate streaming from removed node to inheriting nodes
	streamingStarted := 0
	for inheritingNodeID, ranges := range rangeAssignments {
		// Get inheriting node details
		inheritingNode, err := h.getStorageNode(ctx, inheritingNodeID)
		if err != nil {
			h.logger.Warn("Failed to get inheriting node details",
				zap.String("inheriting_node_id", inheritingNodeID),
				zap.Error(err))
			continue
		}

		// Convert ranges to client format
		clientRanges := make([]client.KeyRange, len(ranges))
		for i, r := range ranges {
			clientRanges[i] = client.KeyRange{
				StartHash: r.StartHash,
				EndHash:   r.EndHash,
			}
		}

		// Start streaming from removed node to inheriting node
		streamReq := &client.StartStreamingRequest{
			SourceNodeID: req.NodeId,
			TargetNodeID: inheritingNodeID,
			TargetHost:   inheritingNode.Host,
			TargetPort:   inheritingNode.Port,
			KeyRanges:    clientRanges,
		}

		resp, err := h.storageNodeClient.StartStreaming(ctx, removedNode, streamReq)
		if err != nil {
			h.logger.Error("Failed to start streaming for removal",
				zap.String("removed_node", req.NodeId),
				zap.String("inheriting_node", inheritingNodeID),
				zap.Error(err))
			continue
		}

		if !resp.Success {
			h.logger.Warn("Streaming start unsuccessful for removal",
				zap.String("removed_node", req.NodeId),
				zap.String("inheriting_node", inheritingNodeID),
				zap.String("message", resp.Message))
			continue
		}

		streamingStarted++
		h.logger.Info("Streaming started for removal",
			zap.String("removed_node", req.NodeId),
			zap.String("inheriting_node", inheritingNodeID),
			zap.Int("key_ranges", len(ranges)))
	}

	// Start background task to monitor streaming and complete removal
	go h.monitorRemovalProgress(context.Background(), removedNode, rangeAssignments)

	return &pb.RemoveStorageNodeResponse{
		Success: true,
		NodeId:  req.NodeId,
		Message: fmt.Sprintf("Node marked as draining. Streaming initiated to %d inheriting nodes.", streamingStarted),
	}, nil
}

// monitorRemovalProgress monitors streaming and removes node when complete
func (h *NodeHandlerV2) monitorRemovalProgress(
	ctx context.Context,
	removedNode *model.StorageNode,
	rangeAssignments map[string][]service.KeyRange,
) {
	h.logger.Info("Starting removal progress monitor",
		zap.String("removed_node_id", removedNode.NodeID),
		zap.Int("inheriting_nodes", len(rangeAssignments)))

	ticker := time.NewTicker(h.streamPollInterval)
	defer ticker.Stop()

	inheritingNodes := make([]string, 0, len(rangeAssignments))
	for nodeID := range rangeAssignments {
		inheritingNodes = append(inheritingNodes, nodeID)
	}

	completedNodes := make(map[string]bool)
	maxAttempts := 360 // 1 hour with 10s intervals

	for attempt := 0; attempt < maxAttempts; attempt++ {
		<-ticker.C

		allCompleted := true

		for _, inheritingNodeID := range inheritingNodes {
			if completedNodes[inheritingNodeID] {
				continue // Already completed
			}

			// Check streaming status from removed node to inheriting node
			statusReq := &client.StreamStatusRequest{
				SourceNodeID: removedNode.NodeID,
				TargetNodeID: inheritingNodeID,
			}

			statusResp, err := h.storageNodeClient.GetStreamStatus(ctx, removedNode, statusReq)
			if err != nil {
				h.logger.Warn("Failed to get streaming status for removal",
					zap.String("removed_node", removedNode.NodeID),
					zap.String("inheriting_node", inheritingNodeID),
					zap.Error(err))
				allCompleted = false
				continue
			}

			if statusResp.State == "completed" {
				completedNodes[inheritingNodeID] = true
				h.logger.Info("Streaming completed to inheriting node",
					zap.String("removed_node", removedNode.NodeID),
					zap.String("inheriting_node", inheritingNodeID),
					zap.Int64("keys_copied", statusResp.KeysCopied),
					zap.Int64("keys_streamed", statusResp.KeysStreamed))
			} else {
				allCompleted = false
				h.logger.Debug("Streaming in progress for removal",
					zap.String("removed_node", removedNode.NodeID),
					zap.String("inheriting_node", inheritingNodeID),
					zap.String("state", statusResp.State),
					zap.Int64("keys_copied", statusResp.KeysCopied),
					zap.Int64("keys_streamed", statusResp.KeysStreamed))
			}
		}

		if allCompleted {
			h.logger.Info("All streaming completed for removal, removing node from cluster",
				zap.String("removed_node_id", removedNode.NodeID),
				zap.Int("completed_streams", len(completedNodes)))

			// Remove from hash ring
			if err := h.routingService.RemoveNode(ctx, removedNode.NodeID); err != nil {
				h.logger.Error("Failed to remove node from hash ring",
					zap.String("node_id", removedNode.NodeID),
					zap.Error(err))
				return
			}

			// Remove from metadata store
			if err := h.metadataStore.RemoveStorageNode(ctx, removedNode.NodeID); err != nil {
				h.logger.Error("Failed to remove node from metadata store",
					zap.String("node_id", removedNode.NodeID),
					zap.Error(err))
				return
			}

			// Cleanup streaming contexts
			for _, inheritingNodeID := range inheritingNodes {
				stopReq := &client.StopStreamingRequest{
					SourceNodeID: removedNode.NodeID,
					TargetNodeID: inheritingNodeID,
				}

				if _, err := h.storageNodeClient.StopStreaming(ctx, removedNode, stopReq); err != nil {
					h.logger.Warn("Failed to stop streaming for removal",
						zap.String("removed_node", removedNode.NodeID),
						zap.String("inheriting_node", inheritingNodeID),
						zap.Error(err))
				}
			}

			h.logger.Info("Node successfully removed from cluster",
				zap.String("node_id", removedNode.NodeID))

			return
		}
	}

	// Timeout reached - rollback the node removal
	h.logger.Error("Removal monitor timeout reached, initiating rollback",
		zap.String("node_id", removedNode.NodeID),
		zap.Int("completed_nodes", len(completedNodes)),
		zap.Int("total_nodes", len(inheritingNodes)))

	// Rollback: Reactivate the node since removal failed
	h.rollbackNodeRemoval(ctx, removedNode, inheritingNodes)
}

// rollbackNodeRemoval reactivates a node after failed removal
func (h *NodeHandlerV2) rollbackNodeRemoval(
	ctx context.Context,
	node *model.StorageNode,
	inheritingNodes []string,
) {
	h.logger.Warn("Rolling back failed node removal",
		zap.String("node_id", node.NodeID))

	// Stop all streaming operations
	for _, inheritingNodeID := range inheritingNodes {
		stopReq := &client.StopStreamingRequest{
			SourceNodeID: node.NodeID,
			TargetNodeID: inheritingNodeID,
		}

		if _, err := h.storageNodeClient.StopStreaming(ctx, node, stopReq); err != nil {
			h.logger.Warn("Failed to stop streaming during removal rollback",
				zap.String("removed_node", node.NodeID),
				zap.String("inheriting_node", inheritingNodeID),
				zap.Error(err))
		}
	}

	// Reactivate node (change status back to active)
	if err := h.metadataStore.UpdateStorageNodeStatus(ctx, node.NodeID, string(model.NodeStatusActive)); err != nil {
		h.logger.Error("Failed to reactivate node during rollback",
			zap.String("node_id", node.NodeID),
			zap.Error(err))
	}

	h.logger.Info("Node removal rollback completed - node reactivated",
		zap.String("node_id", node.NodeID))
}

// monitorStreamingProgress monitors streaming and transitions node to active when complete
func (h *NodeHandlerV2) monitorStreamingProgress(
	ctx context.Context,
	newNode *model.StorageNode,
	rangeAssignments map[string][]service.KeyRange,
) {
	h.logger.Info("Starting streaming progress monitor",
		zap.String("node_id", newNode.NodeID),
		zap.Int("source_nodes", len(rangeAssignments)))

	ticker := time.NewTicker(h.streamPollInterval)
	defer ticker.Stop()

	sourceNodes := make([]string, 0, len(rangeAssignments))
	for nodeID := range rangeAssignments {
		sourceNodes = append(sourceNodes, nodeID)
	}

	completedNodes := make(map[string]bool)
	maxAttempts := 360 // 1 hour with 10s intervals

	for attempt := 0; attempt < maxAttempts; attempt++ {
		<-ticker.C

		allCompleted := true

		for _, sourceNodeID := range sourceNodes {
			if completedNodes[sourceNodeID] {
				continue // Already completed
			}

			// Get source node
			sourceNode, err := h.getStorageNode(ctx, sourceNodeID)
			if err != nil {
				h.logger.Warn("Failed to get source node",
					zap.String("source_node", sourceNodeID),
					zap.Error(err))
				allCompleted = false
				continue
			}

			// Check streaming status
			statusReq := &client.StreamStatusRequest{
				SourceNodeID: sourceNodeID,
				TargetNodeID: newNode.NodeID,
			}

			statusResp, err := h.storageNodeClient.GetStreamStatus(ctx, sourceNode, statusReq)
			if err != nil {
				h.logger.Warn("Failed to get streaming status",
					zap.String("source_node", sourceNodeID),
					zap.String("target_node", newNode.NodeID),
					zap.Error(err))
				allCompleted = false
				continue
			}

			if statusResp.State == "completed" {
				completedNodes[sourceNodeID] = true
				h.logger.Info("Streaming completed from source node",
					zap.String("source_node", sourceNodeID),
					zap.String("target_node", newNode.NodeID),
					zap.Int64("keys_copied", statusResp.KeysCopied),
					zap.Int64("keys_streamed", statusResp.KeysStreamed))
			} else {
				allCompleted = false
				h.logger.Debug("Streaming in progress",
					zap.String("source_node", sourceNodeID),
					zap.String("target_node", newNode.NodeID),
					zap.String("state", statusResp.State),
					zap.Int64("keys_copied", statusResp.KeysCopied),
					zap.Int64("keys_streamed", statusResp.KeysStreamed))
			}
		}

		if allCompleted {
			h.logger.Info("All streaming completed, transitioning node to active",
				zap.String("node_id", newNode.NodeID),
				zap.Int("completed_streams", len(completedNodes)))

			// Update node status to active
			if err := h.metadataStore.UpdateStorageNodeStatus(ctx, newNode.NodeID, string(model.NodeStatusActive)); err != nil {
				h.logger.Error("Failed to update node status to active",
					zap.String("node_id", newNode.NodeID),
					zap.Error(err))
				return
			}

			// Notify all source nodes that streaming is complete (cleanup)
			for _, sourceNodeID := range sourceNodes {
				sourceNode, err := h.getStorageNode(ctx, sourceNodeID)
				if err != nil {
					continue
				}

				stopReq := &client.StopStreamingRequest{
					SourceNodeID: sourceNodeID,
					TargetNodeID: newNode.NodeID,
				}

				if _, err := h.storageNodeClient.StopStreaming(ctx, sourceNode, stopReq); err != nil {
					h.logger.Warn("Failed to stop streaming",
						zap.String("source_node", sourceNodeID),
						zap.Error(err))
				}
			}

			h.logger.Info("Node successfully bootstrapped and activated",
				zap.String("node_id", newNode.NodeID))

			return
		}
	}

	// Timeout reached - rollback the node addition
	h.logger.Error("Streaming monitor timeout reached, initiating rollback",
		zap.String("node_id", newNode.NodeID),
		zap.Int("completed_nodes", len(completedNodes)),
		zap.Int("total_nodes", len(sourceNodes)))

	// Rollback: Remove node from hash ring and metadata
	h.rollbackNodeAddition(ctx, newNode, sourceNodes)
}

// rollbackNodeAddition removes a partially bootstrapped node after streaming failure
func (h *NodeHandlerV2) rollbackNodeAddition(
	ctx context.Context,
	node *model.StorageNode,
	sourceNodes []string,
) {
	h.logger.Warn("Rolling back failed node addition",
		zap.String("node_id", node.NodeID))

	// Stop all streaming operations
	for _, sourceNodeID := range sourceNodes {
		sourceNode, err := h.getStorageNode(ctx, sourceNodeID)
		if err != nil {
			continue
		}

		stopReq := &client.StopStreamingRequest{
			SourceNodeID: sourceNodeID,
			TargetNodeID: node.NodeID,
		}

		if _, err := h.storageNodeClient.StopStreaming(ctx, sourceNode, stopReq); err != nil {
			h.logger.Warn("Failed to stop streaming during rollback",
				zap.String("source_node", sourceNodeID),
				zap.String("target_node", node.NodeID),
				zap.Error(err))
		}
	}

	// Remove from hash ring
	if err := h.routingService.RemoveNode(ctx, node.NodeID); err != nil {
		h.logger.Error("Failed to remove node from hash ring during rollback",
			zap.String("node_id", node.NodeID),
			zap.Error(err))
	}

	// Mark node as failed in metadata store (don't delete, keep for audit trail)
	if err := h.metadataStore.UpdateStorageNodeStatus(ctx, node.NodeID, "failed"); err != nil {
		h.logger.Error("Failed to update node status during rollback",
			zap.String("node_id", node.NodeID),
			zap.Error(err))
	}

	h.logger.Info("Node addition rollback completed",
		zap.String("node_id", node.NodeID))
}

// getStorageNode retrieves a storage node by ID from metadata store
// Helper method since MetadataStore only has ListStorageNodes
func (h *NodeHandlerV2) getStorageNode(ctx context.Context, nodeID string) (*model.StorageNode, error) {
	nodes, err := h.metadataStore.ListStorageNodes(ctx)
	if err != nil {
		return nil, err
	}

	for _, node := range nodes {
		if node.NodeID == nodeID {
			return node, nil
		}
	}

	return nil, fmt.Errorf("node not found: %s", nodeID)
}

// GetMigrationStatus handles migration status requests
// Note: Phase 2 uses streaming status instead of traditional migration tracking
func (h *NodeHandlerV2) GetMigrationStatus(
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
		Type:        string(migration.Type),
		NodeId:      migration.NodeID,
		Status:      string(migration.Status),
		StartedAt:   migration.StartedAt.Unix(),
	}

	return resp, nil
}

// ListStorageNodes handles list storage nodes requests
func (h *NodeHandlerV2) ListStorageNodes(
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
			Status:       string(node.Status),
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
