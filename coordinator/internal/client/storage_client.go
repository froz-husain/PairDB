package client

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/devrev/pairdb/coordinator/internal/model"
	pb "github.com/devrev/pairdb/coordinator/pkg/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// StorageClient handles communication with storage nodes
type StorageClient struct {
	connections map[string]*grpc.ClientConn
	mu          sync.RWMutex
	timeout     time.Duration
}

// StorageResponse represents a response from a storage node
type StorageResponse struct {
	NodeID      string
	Success     bool
	Value       []byte
	VectorClock model.VectorClock
	Timestamp   int64
	Found       bool
	Error       error
}

// NewStorageClient creates a new storage client
func NewStorageClient(timeout time.Duration) *StorageClient {
	return &StorageClient{
		connections: make(map[string]*grpc.ClientConn),
		timeout:     timeout,
	}
}

// Write sends write request to storage node
func (c *StorageClient) Write(
	ctx context.Context,
	node *model.StorageNode,
	tenantID, key string,
	value []byte,
	vectorClock model.VectorClock,
) (*StorageResponse, error) {
	conn, err := c.getConnection(node)
	if err != nil {
		return nil, err
	}

	client := pb.NewStorageServiceClient(conn)

	// Set timeout
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	resp, err := client.Write(ctx, &pb.WriteRequest{
		TenantId:    tenantID,
		Key:         key,
		Value:       value,
		VectorClock: c.toProtoVectorClock(vectorClock),
		Timestamp:   time.Now().UnixMilli(),
	})

	if err != nil {
		return &StorageResponse{
			NodeID:  node.NodeID,
			Success: false,
			Error:   err,
		}, err
	}

	return &StorageResponse{
		NodeID:      node.NodeID,
		Success:     resp.Success,
		VectorClock: c.fromProtoVectorClock(resp.VectorClock),
	}, nil
}

// Read sends read request to storage node
func (c *StorageClient) Read(
	ctx context.Context,
	node *model.StorageNode,
	tenantID, key string,
) (*StorageResponse, error) {
	conn, err := c.getConnection(node)
	if err != nil {
		return nil, err
	}

	client := pb.NewStorageServiceClient(conn)

	// Set timeout
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	resp, err := client.Read(ctx, &pb.ReadRequest{
		TenantId: tenantID,
		Key:      key,
	})

	if err != nil {
		return &StorageResponse{
			NodeID:  node.NodeID,
			Success: false,
			Error:   err,
		}, err
	}

	return &StorageResponse{
		NodeID:      node.NodeID,
		Success:     resp.Success,
		Value:       resp.Value,
		VectorClock: c.fromProtoVectorClock(resp.VectorClock),
		Timestamp:   resp.Timestamp,
		Found:       resp.Found,
	}, nil
}

// Delete sends delete request to storage node
func (c *StorageClient) Delete(
	ctx context.Context,
	node *model.StorageNode,
	tenantID, key string,
	vectorClock model.VectorClock,
) (*StorageResponse, error) {
	conn, err := c.getConnection(node)
	if err != nil {
		return nil, err
	}

	client := pb.NewStorageServiceClient(conn)

	// Set timeout
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	resp, err := client.Delete(ctx, &pb.DeleteRequest{
		TenantId:    tenantID,
		Key:         key,
		VectorClock: c.toProtoVectorClock(vectorClock),
	})

	if err != nil {
		return &StorageResponse{
			NodeID:  node.NodeID,
			Success: false,
			Error:   err,
		}, err
	}

	return &StorageResponse{
		NodeID:  node.NodeID,
		Success: resp.Success,
	}, nil
}

// Repair sends repair request to storage node
func (c *StorageClient) Repair(
	ctx context.Context,
	node *model.StorageNode,
	tenantID, key string,
	value []byte,
	vectorClock model.VectorClock,
	timestamp int64,
) error {
	conn, err := c.getConnection(node)
	if err != nil {
		return err
	}

	client := pb.NewStorageServiceClient(conn)

	// Set timeout
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	_, err = client.Repair(ctx, &pb.RepairRequest{
		TenantId:    tenantID,
		Key:         key,
		Value:       value,
		VectorClock: c.toProtoVectorClock(vectorClock),
		Timestamp:   timestamp,
	})

	return err
}

// getConnection returns or creates a gRPC connection
func (c *StorageClient) getConnection(node *model.StorageNode) (*grpc.ClientConn, error) {
	addr := fmt.Sprintf("%s:%d", node.Host, node.Port)

	c.mu.RLock()
	conn, exists := c.connections[addr]
	c.mu.RUnlock()

	if exists {
		return conn, nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Double-check
	if conn, exists := c.connections[addr]; exists {
		return conn, nil
	}

	// Create new connection
	conn, err := grpc.Dial(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
		grpc.WithTimeout(5*time.Second),
	)

	if err != nil {
		return nil, fmt.Errorf("failed to connect to %s: %w", addr, err)
	}

	c.connections[addr] = conn
	return conn, nil
}

// toProtoVectorClock converts model vector clock to proto
func (c *StorageClient) toProtoVectorClock(vc model.VectorClock) *pb.VectorClock {
	entries := make([]*pb.VectorClockEntry, len(vc.Entries))
	for i, entry := range vc.Entries {
		entries[i] = &pb.VectorClockEntry{
			CoordinatorNodeId: entry.CoordinatorNodeID,
			LogicalTimestamp:  entry.LogicalTimestamp,
		}
	}
	return &pb.VectorClock{Entries: entries}
}

// fromProtoVectorClock converts proto vector clock to model
func (c *StorageClient) fromProtoVectorClock(vc *pb.VectorClock) model.VectorClock {
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

// Close closes all connections
func (c *StorageClient) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, conn := range c.connections {
		conn.Close()
	}
	c.connections = make(map[string]*grpc.ClientConn)
}

// CloseConnection closes a specific connection
func (c *StorageClient) CloseConnection(node *model.StorageNode) error {
	addr := fmt.Sprintf("%s:%d", node.Host, node.Port)

	c.mu.Lock()
	defer c.mu.Unlock()

	if conn, exists := c.connections[addr]; exists {
		delete(c.connections, addr)
		return conn.Close()
	}

	return nil
}
