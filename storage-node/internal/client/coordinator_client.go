package client

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "github.com/devrev/pairdb/coordinator/pkg/proto"
	"go.uber.org/zap"
)

// CoordinatorClient handles communication with the coordinator service
type CoordinatorClient struct {
	host   string
	port   int
	conn   *grpc.ClientConn
	client pb.CoordinatorServiceClient
	logger *zap.Logger
}

// NewCoordinatorClient creates a new coordinator client
func NewCoordinatorClient(host string, port int, logger *zap.Logger) (*CoordinatorClient, error) {
	addr := fmt.Sprintf("%s:%d", host, port)

	// Create gRPC connection
	conn, err := grpc.Dial(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to coordinator at %s: %w", addr, err)
	}

	client := pb.NewCoordinatorServiceClient(conn)

	return &CoordinatorClient{
		host:   host,
		port:   port,
		conn:   conn,
		client: client,
		logger: logger,
	}, nil
}

// RegisterNode registers the storage node with the coordinator
func (c *CoordinatorClient) RegisterNode(ctx context.Context, nodeID, host string, port, virtualNodes int) error {
	req := &pb.AddStorageNodeRequest{
		NodeId:       nodeID,
		Host:         host,
		Port:         int32(port),
		VirtualNodes: int32(virtualNodes),
	}

	c.logger.Info("Registering storage node with coordinator",
		zap.String("node_id", nodeID),
		zap.String("host", host),
		zap.Int("port", port),
		zap.Int("virtual_nodes", virtualNodes),
		zap.String("coordinator", fmt.Sprintf("%s:%d", c.host, c.port)))

	resp, err := c.client.AddStorageNode(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to register node: %w", err)
	}

	if !resp.Success {
		return fmt.Errorf("registration failed: %s", resp.ErrorMessage)
	}

	c.logger.Info("Successfully registered with coordinator",
		zap.String("node_id", resp.NodeId),
		zap.String("message", resp.Message),
		zap.String("migration_id", resp.MigrationId))

	return nil
}

// RegisterWithRetry attempts to register with the coordinator with retries
func (c *CoordinatorClient) RegisterWithRetry(ctx context.Context, nodeID, host string, port, virtualNodes int, maxRetries int, retryInterval time.Duration) error {
	var lastErr error

	for attempt := 1; attempt <= maxRetries; attempt++ {
		err := c.RegisterNode(ctx, nodeID, host, port, virtualNodes)
		if err == nil {
			return nil
		}

		lastErr = err
		c.logger.Warn("Failed to register with coordinator, retrying...",
			zap.Int("attempt", attempt),
			zap.Int("max_retries", maxRetries),
			zap.Error(err))

		if attempt < maxRetries {
			select {
			case <-ctx.Done():
				return fmt.Errorf("context cancelled during registration: %w", ctx.Err())
			case <-time.After(retryInterval):
				// Continue to next attempt
			}
		}
	}

	return fmt.Errorf("failed to register after %d attempts: %w", maxRetries, lastErr)
}

// Close closes the coordinator client connection
func (c *CoordinatorClient) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}
