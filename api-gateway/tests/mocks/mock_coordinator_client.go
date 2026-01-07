// Package mocks provides mock implementations for testing.
package mocks

import (
	"context"

	pb "github.com/devrev/pairdb/api-gateway/pkg/proto"
	"github.com/stretchr/testify/mock"
	"google.golang.org/grpc"
)

// MockCoordinatorServiceClient is a mock implementation of pb.CoordinatorServiceClient.
type MockCoordinatorServiceClient struct {
	mock.Mock
}

// WriteKeyValue mocks the WriteKeyValue RPC.
func (m *MockCoordinatorServiceClient) WriteKeyValue(ctx context.Context, req *pb.WriteKeyValueRequest, opts ...grpc.CallOption) (*pb.WriteKeyValueResponse, error) {
	args := m.Called(ctx, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*pb.WriteKeyValueResponse), args.Error(1)
}

// ReadKeyValue mocks the ReadKeyValue RPC.
func (m *MockCoordinatorServiceClient) ReadKeyValue(ctx context.Context, req *pb.ReadKeyValueRequest, opts ...grpc.CallOption) (*pb.ReadKeyValueResponse, error) {
	args := m.Called(ctx, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*pb.ReadKeyValueResponse), args.Error(1)
}

// CreateTenant mocks the CreateTenant RPC.
func (m *MockCoordinatorServiceClient) CreateTenant(ctx context.Context, req *pb.CreateTenantRequest, opts ...grpc.CallOption) (*pb.CreateTenantResponse, error) {
	args := m.Called(ctx, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*pb.CreateTenantResponse), args.Error(1)
}

// UpdateReplicationFactor mocks the UpdateReplicationFactor RPC.
func (m *MockCoordinatorServiceClient) UpdateReplicationFactor(ctx context.Context, req *pb.UpdateReplicationFactorRequest, opts ...grpc.CallOption) (*pb.UpdateReplicationFactorResponse, error) {
	args := m.Called(ctx, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*pb.UpdateReplicationFactorResponse), args.Error(1)
}

// GetTenant mocks the GetTenant RPC.
func (m *MockCoordinatorServiceClient) GetTenant(ctx context.Context, req *pb.GetTenantRequest, opts ...grpc.CallOption) (*pb.GetTenantResponse, error) {
	args := m.Called(ctx, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*pb.GetTenantResponse), args.Error(1)
}

// AddStorageNode mocks the AddStorageNode RPC.
func (m *MockCoordinatorServiceClient) AddStorageNode(ctx context.Context, req *pb.AddStorageNodeRequest, opts ...grpc.CallOption) (*pb.AddStorageNodeResponse, error) {
	args := m.Called(ctx, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*pb.AddStorageNodeResponse), args.Error(1)
}

// RemoveStorageNode mocks the RemoveStorageNode RPC.
func (m *MockCoordinatorServiceClient) RemoveStorageNode(ctx context.Context, req *pb.RemoveStorageNodeRequest, opts ...grpc.CallOption) (*pb.RemoveStorageNodeResponse, error) {
	args := m.Called(ctx, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*pb.RemoveStorageNodeResponse), args.Error(1)
}

// GetMigrationStatus mocks the GetMigrationStatus RPC.
func (m *MockCoordinatorServiceClient) GetMigrationStatus(ctx context.Context, req *pb.GetMigrationStatusRequest, opts ...grpc.CallOption) (*pb.GetMigrationStatusResponse, error) {
	args := m.Called(ctx, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*pb.GetMigrationStatusResponse), args.Error(1)
}

// ListStorageNodes mocks the ListStorageNodes RPC.
func (m *MockCoordinatorServiceClient) ListStorageNodes(ctx context.Context, req *pb.ListStorageNodesRequest, opts ...grpc.CallOption) (*pb.ListStorageNodesResponse, error) {
	args := m.Called(ctx, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*pb.ListStorageNodesResponse), args.Error(1)
}

// NewMockCoordinatorServiceClient creates a new mock client.
func NewMockCoordinatorServiceClient() *MockCoordinatorServiceClient {
	return &MockCoordinatorServiceClient{}
}

