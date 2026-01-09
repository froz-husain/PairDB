package client

import (
	"context"
	"fmt"
	"time"

	"github.com/devrev/pairdb/coordinator/internal/model"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "github.com/devrev/pairdb/storage-node/pkg/proto"
)

// StorageNodeClient provides APIs for coordinator to communicate with storage nodes
// Specifically for streaming operations during node addition/removal
type StorageNodeClient struct {
	connections map[string]*grpc.ClientConn // nodeID -> connection
	clients     map[string]pb.StorageNodeServiceClient
	timeout     time.Duration
	logger      *zap.Logger
}

// NewStorageNodeClient creates a new storage node client
func NewStorageNodeClient(timeout time.Duration, logger *zap.Logger) *StorageNodeClient {
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	return &StorageNodeClient{
		connections: make(map[string]*grpc.ClientConn),
		clients:     make(map[string]pb.StorageNodeServiceClient),
		timeout:     timeout,
		logger:      logger,
	}
}

// KeyRange represents a hash range for streaming
type KeyRange struct {
	StartHash uint64 `json:"start_hash"`
	EndHash   uint64 `json:"end_hash"`
}

// StartStreamingRequest contains parameters for initiating streaming
type StartStreamingRequest struct {
	SourceNodeID string
	TargetNodeID string
	TargetHost   string
	TargetPort   int
	KeyRanges    []KeyRange
}

// StartStreamingResponse contains the result of starting streaming
type StartStreamingResponse struct {
	Success bool
	Message string
}

// StopStreamingRequest contains parameters for stopping streaming
type StopStreamingRequest struct {
	SourceNodeID string
	TargetNodeID string
}

// StopStreamingResponse contains the result of stopping streaming
type StopStreamingResponse struct {
	Success bool
	Message string
}

// StreamStatusRequest contains parameters for checking streaming status
type StreamStatusRequest struct {
	SourceNodeID string
	TargetNodeID string
}

// StreamStatusResponse contains streaming status information
type StreamStatusResponse struct {
	Active        bool
	State         string
	KeysCopied    int64
	KeysStreamed  int64
	BytesCopied   int64
	BytesStreamed int64
}

// StartStreaming instructs a source node to start streaming to a target node
func (c *StorageNodeClient) StartStreaming(
	ctx context.Context,
	sourceNode *model.StorageNode,
	req *StartStreamingRequest,
) (*StartStreamingResponse, error) {
	client, err := c.getClient(sourceNode)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for node %s: %w", sourceNode.NodeID, err)
	}

	// Convert key ranges to proto format
	protoRanges := make([]*pb.StreamKeyRange, len(req.KeyRanges))
	for i, kr := range req.KeyRanges {
		protoRanges[i] = &pb.StreamKeyRange{
			StartHash: kr.StartHash,
			EndHash:   kr.EndHash,
		}
	}

	// Call StartStreaming RPC
	grpcReq := &pb.StartStreamingRequest{
		SourceNodeId: req.SourceNodeID,
		TargetNodeId: req.TargetNodeID,
		TargetHost:   req.TargetHost,
		TargetPort:   int32(req.TargetPort),
		KeyRanges:    protoRanges,
	}

	grpcCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	resp, err := client.StartStreaming(grpcCtx, grpcReq)
	if err != nil {
		c.logger.Error("StartStreaming RPC failed",
			zap.String("source_node", sourceNode.NodeID),
			zap.String("target_node", req.TargetNodeID),
			zap.Error(err))
		return nil, fmt.Errorf("StartStreaming RPC failed: %w", err)
	}

	c.logger.Info("Streaming started successfully",
		zap.String("source_node", sourceNode.NodeID),
		zap.String("target_node", req.TargetNodeID),
		zap.Int("key_ranges", len(req.KeyRanges)))

	return &StartStreamingResponse{
		Success: resp.Success,
		Message: resp.Message,
	}, nil
}

// StopStreaming instructs a source node to stop streaming to a target node
func (c *StorageNodeClient) StopStreaming(
	ctx context.Context,
	sourceNode *model.StorageNode,
	req *StopStreamingRequest,
) (*StopStreamingResponse, error) {
	client, err := c.getClient(sourceNode)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for node %s: %w", sourceNode.NodeID, err)
	}

	// Call StopStreaming RPC
	grpcReq := &pb.StopStreamingRequest{
		SourceNodeId: req.SourceNodeID,
		TargetNodeId: req.TargetNodeID,
	}

	grpcCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	resp, err := client.StopStreaming(grpcCtx, grpcReq)
	if err != nil {
		c.logger.Error("StopStreaming RPC failed",
			zap.String("source_node", sourceNode.NodeID),
			zap.String("target_node", req.TargetNodeID),
			zap.Error(err))
		return nil, fmt.Errorf("StopStreaming RPC failed: %w", err)
	}

	c.logger.Info("Streaming stopped successfully",
		zap.String("source_node", sourceNode.NodeID),
		zap.String("target_node", req.TargetNodeID))

	return &StopStreamingResponse{
		Success: resp.Success,
		Message: resp.Message,
	}, nil
}

// GetStreamStatus retrieves the status of streaming from a source node
func (c *StorageNodeClient) GetStreamStatus(
	ctx context.Context,
	sourceNode *model.StorageNode,
	req *StreamStatusRequest,
) (*StreamStatusResponse, error) {
	client, err := c.getClient(sourceNode)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for node %s: %w", sourceNode.NodeID, err)
	}

	// Call GetStreamStatus RPC
	grpcReq := &pb.StreamStatusRequest{
		SourceNodeId: req.SourceNodeID,
		TargetNodeId: req.TargetNodeID,
	}

	grpcCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	resp, err := client.GetStreamStatus(grpcCtx, grpcReq)
	if err != nil {
		c.logger.Error("GetStreamStatus RPC failed",
			zap.String("source_node", sourceNode.NodeID),
			zap.String("target_node", req.TargetNodeID),
			zap.Error(err))
		return nil, fmt.Errorf("GetStreamStatus RPC failed: %w", err)
	}

	return &StreamStatusResponse{
		Active:        resp.Active,
		State:         resp.State,
		KeysCopied:    resp.KeysCopied,
		KeysStreamed:  resp.KeysStreamed,
		BytesCopied:   resp.BytesCopied,
		BytesStreamed: resp.BytesStreamed,
	}, nil
}

// NotifyStreamingComplete tells the coordinator that streaming is complete
// This is typically called by the storage node when it finishes bootstrapping
func (c *StorageNodeClient) NotifyStreamingComplete(
	ctx context.Context,
	node *model.StorageNode,
	targetNodeID string,
) error {
	client, err := c.getClient(node)
	if err != nil {
		return fmt.Errorf("failed to get client for node %s: %w", node.NodeID, err)
	}

	// Call NotifyStreamingComplete RPC
	grpcReq := &pb.StreamingCompleteRequest{
		SourceNodeId: node.NodeID,
		TargetNodeId: targetNodeID,
	}

	grpcCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	resp, err := client.NotifyStreamingComplete(grpcCtx, grpcReq)
	if err != nil {
		c.logger.Error("NotifyStreamingComplete RPC failed",
			zap.String("source_node", node.NodeID),
			zap.String("target_node", targetNodeID),
			zap.Error(err))
		return fmt.Errorf("NotifyStreamingComplete RPC failed: %w", err)
	}

	if !resp.Success {
		return fmt.Errorf("coordinator rejected streaming completion: %s", resp.Message)
	}

	c.logger.Info("Streaming completion notified to coordinator",
		zap.String("source_node", node.NodeID),
		zap.String("target_node", targetNodeID))

	return nil
}

// getClient retrieves or creates a gRPC client for a storage node
func (c *StorageNodeClient) getClient(node *model.StorageNode) (pb.StorageNodeServiceClient, error) {
	// Check if client already exists
	if client, exists := c.clients[node.NodeID]; exists {
		return client, nil
	}

	// Create new connection
	target := fmt.Sprintf("%s:%d", node.Host, node.Port)
	conn, err := grpc.Dial(
		target,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
		grpc.WithTimeout(5*time.Second),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to %s: %w", target, err)
	}

	client := pb.NewStorageNodeServiceClient(conn)

	// Cache connection and client
	c.connections[node.NodeID] = conn
	c.clients[node.NodeID] = client

	c.logger.Info("Created gRPC client for storage node",
		zap.String("node_id", node.NodeID),
		zap.String("target", target))

	return client, nil
}

// Close closes all gRPC connections
func (c *StorageNodeClient) Close() error {
	c.logger.Info("Closing all storage node connections")

	for nodeID, conn := range c.connections {
		if err := conn.Close(); err != nil {
			c.logger.Warn("Failed to close connection",
				zap.String("node_id", nodeID),
				zap.Error(err))
		}
	}

	c.connections = make(map[string]*grpc.ClientConn)
	c.clients = make(map[string]pb.StorageNodeServiceClient)

	return nil
}
