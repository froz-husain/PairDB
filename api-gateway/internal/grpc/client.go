// Package grpc provides gRPC client functionality for the API Gateway.
package grpc

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/devrev/pairdb/api-gateway/internal/config"
	pb "github.com/devrev/pairdb/api-gateway/pkg/proto"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/status"
)

// Client manages gRPC connections to the Coordinator service.
type Client struct {
	conn       *grpc.ClientConn
	client     pb.CoordinatorServiceClient
	cfg        config.CoordinatorConfig
	logger     *zap.Logger
	mu         sync.RWMutex
	isHealthy  bool
}

// NewClient creates a new gRPC client for the Coordinator service.
func NewClient(cfg config.CoordinatorConfig, logger *zap.Logger) (*Client, error) {
	if len(cfg.Endpoints) == 0 {
		return nil, fmt.Errorf("no coordinator endpoints provided")
	}

	// Build gRPC dial options
	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(cfg.MaxReceiveMessageSize),
			grpc.MaxCallSendMsgSize(cfg.MaxSendMessageSize),
		),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                cfg.KeepaliveTime,
			Timeout:             cfg.KeepaliveTimeout,
			PermitWithoutStream: true,
		}),
	}

	// For now, connect to the first endpoint (load balancing can be added later)
	target := cfg.Endpoints[0]
	
	conn, err := grpc.NewClient(target, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to coordinator: %w", err)
	}

	client := pb.NewCoordinatorServiceClient(conn)

	c := &Client{
		conn:      conn,
		client:    client,
		cfg:       cfg,
		logger:    logger,
		isHealthy: true,
	}

	return c, nil
}

// Close closes the gRPC connection.
func (c *Client) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// GetClient returns the underlying gRPC client.
func (c *Client) GetClient() pb.CoordinatorServiceClient {
	return c.client
}

// IsHealthy returns the health status of the client.
func (c *Client) IsHealthy() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.isHealthy
}

// setHealthy sets the health status of the client.
func (c *Client) setHealthy(healthy bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.isHealthy = healthy
}

// HealthCheck performs a health check on the gRPC connection.
func (c *Client) HealthCheck(ctx context.Context) error {
	// Use ListStorageNodes as a health check since it's a lightweight call
	_, err := c.client.ListStorageNodes(ctx, &pb.ListStorageNodesRequest{})
	if err != nil {
		c.setHealthy(false)
		return err
	}
	c.setHealthy(true)
	return nil
}

// withRetry wraps a gRPC call with retry logic.
func (c *Client) withRetry(ctx context.Context, operation func() error) error {
	var lastErr error
	for attempt := 0; attempt <= c.cfg.MaxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff
			backoff := c.cfg.RetryBackoff * time.Duration(1<<uint(attempt-1))
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
		}

		err := operation()
		if err == nil {
			return nil
		}

		lastErr = err

		// Check if error is retryable
		if !isRetryable(err) {
			return err
		}

		c.logger.Warn("gRPC call failed, retrying",
			zap.Int("attempt", attempt+1),
			zap.Error(err),
		)
	}

	return lastErr
}

// isRetryable determines if an error is retryable.
func isRetryable(err error) bool {
	if err == nil {
		return false
	}

	st, ok := status.FromError(err)
	if !ok {
		return false
	}

	switch st.Code() {
	case codes.Unavailable, codes.DeadlineExceeded, codes.Aborted, codes.ResourceExhausted:
		return true
	default:
		return false
	}
}

// WriteKeyValue calls the WriteKeyValue RPC with retry logic.
func (c *Client) WriteKeyValue(ctx context.Context, req *pb.WriteKeyValueRequest) (*pb.WriteKeyValueResponse, error) {
	var resp *pb.WriteKeyValueResponse
	err := c.withRetry(ctx, func() error {
		var err error
		resp, err = c.client.WriteKeyValue(ctx, req)
		return err
	})
	return resp, err
}

// ReadKeyValue calls the ReadKeyValue RPC with retry logic.
func (c *Client) ReadKeyValue(ctx context.Context, req *pb.ReadKeyValueRequest) (*pb.ReadKeyValueResponse, error) {
	var resp *pb.ReadKeyValueResponse
	err := c.withRetry(ctx, func() error {
		var err error
		resp, err = c.client.ReadKeyValue(ctx, req)
		return err
	})
	return resp, err
}

// CreateTenant calls the CreateTenant RPC with retry logic.
func (c *Client) CreateTenant(ctx context.Context, req *pb.CreateTenantRequest) (*pb.CreateTenantResponse, error) {
	var resp *pb.CreateTenantResponse
	err := c.withRetry(ctx, func() error {
		var err error
		resp, err = c.client.CreateTenant(ctx, req)
		return err
	})
	return resp, err
}

// UpdateReplicationFactor calls the UpdateReplicationFactor RPC with retry logic.
func (c *Client) UpdateReplicationFactor(ctx context.Context, req *pb.UpdateReplicationFactorRequest) (*pb.UpdateReplicationFactorResponse, error) {
	var resp *pb.UpdateReplicationFactorResponse
	err := c.withRetry(ctx, func() error {
		var err error
		resp, err = c.client.UpdateReplicationFactor(ctx, req)
		return err
	})
	return resp, err
}

// GetTenant calls the GetTenant RPC with retry logic.
func (c *Client) GetTenant(ctx context.Context, req *pb.GetTenantRequest) (*pb.GetTenantResponse, error) {
	var resp *pb.GetTenantResponse
	err := c.withRetry(ctx, func() error {
		var err error
		resp, err = c.client.GetTenant(ctx, req)
		return err
	})
	return resp, err
}

// AddStorageNode calls the AddStorageNode RPC with retry logic.
func (c *Client) AddStorageNode(ctx context.Context, req *pb.AddStorageNodeRequest) (*pb.AddStorageNodeResponse, error) {
	var resp *pb.AddStorageNodeResponse
	err := c.withRetry(ctx, func() error {
		var err error
		resp, err = c.client.AddStorageNode(ctx, req)
		return err
	})
	return resp, err
}

// RemoveStorageNode calls the RemoveStorageNode RPC with retry logic.
func (c *Client) RemoveStorageNode(ctx context.Context, req *pb.RemoveStorageNodeRequest) (*pb.RemoveStorageNodeResponse, error) {
	var resp *pb.RemoveStorageNodeResponse
	err := c.withRetry(ctx, func() error {
		var err error
		resp, err = c.client.RemoveStorageNode(ctx, req)
		return err
	})
	return resp, err
}

// GetMigrationStatus calls the GetMigrationStatus RPC with retry logic.
func (c *Client) GetMigrationStatus(ctx context.Context, req *pb.GetMigrationStatusRequest) (*pb.GetMigrationStatusResponse, error) {
	var resp *pb.GetMigrationStatusResponse
	err := c.withRetry(ctx, func() error {
		var err error
		resp, err = c.client.GetMigrationStatus(ctx, req)
		return err
	})
	return resp, err
}

// ListStorageNodes calls the ListStorageNodes RPC with retry logic.
func (c *Client) ListStorageNodes(ctx context.Context, req *pb.ListStorageNodesRequest) (*pb.ListStorageNodesResponse, error) {
	var resp *pb.ListStorageNodesResponse
	err := c.withRetry(ctx, func() error {
		var err error
		resp, err = c.client.ListStorageNodes(ctx, req)
		return err
	})
	return resp, err
}

