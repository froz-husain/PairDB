package service

import (
	"context"
	"testing"
	"time"

	"github.com/devrev/pairdb/coordinator/internal/client"
	"github.com/devrev/pairdb/coordinator/internal/model"
	"github.com/devrev/pairdb/coordinator/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
)

// MockMetadataStore is a mock implementation of MetadataStore
type MockMetadataStore struct {
	mock.Mock
}

func (m *MockMetadataStore) GetTenant(ctx context.Context, tenantID string) (*model.Tenant, error) {
	args := m.Called(ctx, tenantID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.Tenant), args.Error(1)
}

func (m *MockMetadataStore) CreateTenant(ctx context.Context, tenant *model.Tenant) error {
	args := m.Called(ctx, tenant)
	return args.Error(0)
}

func (m *MockMetadataStore) UpdateTenant(ctx context.Context, tenant *model.Tenant) error {
	args := m.Called(ctx, tenant)
	return args.Error(0)
}

func (m *MockMetadataStore) ListTenants(ctx context.Context) ([]*model.Tenant, error) {
	args := m.Called(ctx)
	return args.Get(0).([]*model.Tenant), args.Error(1)
}

func (m *MockMetadataStore) AddStorageNode(ctx context.Context, node *model.StorageNode) error {
	args := m.Called(ctx, node)
	return args.Error(0)
}

func (m *MockMetadataStore) RemoveStorageNode(ctx context.Context, nodeID string) error {
	args := m.Called(ctx, nodeID)
	return args.Error(0)
}

func (m *MockMetadataStore) ListStorageNodes(ctx context.Context) ([]*model.StorageNode, error) {
	args := m.Called(ctx)
	return args.Get(0).([]*model.StorageNode), args.Error(1)
}

func (m *MockMetadataStore) GetStorageNode(ctx context.Context, nodeID string) (*model.StorageNode, error) {
	args := m.Called(ctx, nodeID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.StorageNode), args.Error(1)
}

func (m *MockMetadataStore) UpdateStorageNodeStatus(ctx context.Context, nodeID, status string) error {
	args := m.Called(ctx, nodeID, status)
	return args.Error(0)
}

func (m *MockMetadataStore) UpdateNodeState(ctx context.Context, nodeID string, state model.NodeState) error {
	args := m.Called(ctx, nodeID, state)
	return args.Error(0)
}

func (m *MockMetadataStore) UpdateNodeRanges(ctx context.Context, nodeID string, pendingRanges []model.PendingRangeInfo, leavingRanges []model.LeavingRangeInfo) error {
	args := m.Called(ctx, nodeID, pendingRanges, leavingRanges)
	return args.Error(0)
}

func (m *MockMetadataStore) AddPendingChange(ctx context.Context, change *model.PendingChange) error {
	args := m.Called(ctx, change)
	return args.Error(0)
}

func (m *MockMetadataStore) GetPendingChange(ctx context.Context, changeID string) (*model.PendingChange, error) {
	args := m.Called(ctx, changeID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.PendingChange), args.Error(1)
}

func (m *MockMetadataStore) UpdatePendingChange(ctx context.Context, change *model.PendingChange) error {
	args := m.Called(ctx, change)
	return args.Error(0)
}

func (m *MockMetadataStore) ListPendingChanges(ctx context.Context, status model.PendingChangeStatus) ([]*model.PendingChange, error) {
	args := m.Called(ctx, status)
	return args.Get(0).([]*model.PendingChange), args.Error(1)
}

func (m *MockMetadataStore) DeletePendingChange(ctx context.Context, changeID string) error {
	args := m.Called(ctx, changeID)
	return args.Error(0)
}

func TestCleanupService_VerifyCleanupSafe_NotCompleted(t *testing.T) {
	mockMetadata := new(MockMetadataStore)
	mockClient := new(MockStorageNodeClient)
	logger := zap.NewNop()

	// Create routing service mock
	routingService := NewRoutingService(mockMetadata, 150, 30*time.Second, logger)

	service := NewCleanupService(mockMetadata, mockClient, routingService, 24*time.Hour, logger)

	ctx := context.Background()

	change := &model.PendingChange{
		ChangeID: "change-1",
		Type:     "bootstrap",
		Status:   model.PendingChangeInProgress, // Not completed
	}

	safe, err := service.VerifyCleanupSafe(ctx, change)

	assert.False(t, safe)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not completed")
}

func TestCleanupService_VerifyCleanupSafe_StreamingActive(t *testing.T) {
	mockMetadata := new(MockMetadataStore)
	mockClient := new(MockStorageNodeClient)
	logger := zap.NewNop()

	routingService := NewRoutingService(mockMetadata, 150, 30*time.Second, logger)

	service := NewCleanupService(mockMetadata, mockClient, routingService, 24*time.Hour, logger)

	ctx := context.Background()

	change := &model.PendingChange{
		ChangeID: "change-1",
		Type:     "bootstrap",
		Status:   model.PendingChangeCompleted,
		Progress: map[string]model.StreamingProgress{
			"node-1": {
				State: "streaming", // Still active
			},
		},
		CompletedAt: time.Now(),
	}

	safe, err := service.VerifyCleanupSafe(ctx, change)

	assert.False(t, safe)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "streaming still active")
}

func TestCleanupService_VerifyCleanupSafe_GracePeriodNotElapsed(t *testing.T) {
	mockMetadata := new(MockMetadataStore)
	mockClient := new(MockStorageNodeClient)
	logger := zap.NewNop()

	routingService := NewRoutingService(mockMetadata, 150, 30*time.Second, logger)

	service := NewCleanupService(mockMetadata, mockClient, routingService, 24*time.Hour, logger)

	ctx := context.Background()

	change := &model.PendingChange{
		ChangeID: "change-1",
		Type:     "bootstrap",
		Status:   model.PendingChangeCompleted,
		Progress: map[string]model.StreamingProgress{
			"node-1": {
				State: "completed",
			},
		},
		CompletedAt: time.Now().Add(-1 * time.Hour), // Only 1 hour ago (need 24)
	}

	safe, err := service.VerifyCleanupSafe(ctx, change)

	assert.False(t, safe)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "grace period not elapsed")
}

func TestCleanupService_VerifyCleanupSafe_Success(t *testing.T) {
	mockMetadata := new(MockMetadataStore)
	mockClient := new(MockStorageNodeClient)
	logger := zap.NewNop()

	// Mock storage nodes for quorum verification
	nodes := []*model.StorageNode{
		{NodeID: "node-1", State: model.NodeStateNormal},
		{NodeID: "node-2", State: model.NodeStateNormal},
		{NodeID: "node-3", State: model.NodeStateNormal},
	}
	mockMetadata.On("ListStorageNodes", mock.Anything).Return(nodes, nil)

	routingService := NewRoutingService(mockMetadata, 150, 30*time.Second, logger)

	service := NewCleanupService(mockMetadata, mockClient, routingService, 24*time.Hour, logger)

	ctx := context.Background()

	change := &model.PendingChange{
		ChangeID: "change-1",
		Type:     "bootstrap",
		Status:   model.PendingChangeCompleted,
		Progress: map[string]model.StreamingProgress{
			"node-1": {
				State: "completed",
			},
		},
		Ranges: []model.TokenRange{
			{Start: 0, End: 100},
		},
		CompletedAt: time.Now().Add(-25 * time.Hour), // 25 hours ago (grace period passed)
	}

	safe, err := service.VerifyCleanupSafe(ctx, change)

	assert.True(t, safe)
	assert.NoError(t, err)
	mockMetadata.AssertExpectations(t)
}

func TestCleanupService_ExecuteCleanup_Success(t *testing.T) {
	mockMetadata := new(MockMetadataStore)
	mockClient := new(MockStorageNodeClient)
	logger := zap.NewNop()

	// Mock storage nodes
	nodes := []*model.StorageNode{
		{NodeID: "node-1", State: model.NodeStateNormal},
		{NodeID: "node-2", State: model.NodeStateNormal},
		{NodeID: "node-3", State: model.NodeStateNormal},
	}
	mockMetadata.On("ListStorageNodes", mock.Anything).Return(nodes, nil)

	routingService := NewRoutingService(mockMetadata, 150, 30*time.Second, logger)

	service := NewCleanupService(mockMetadata, mockClient, routingService, 24*time.Hour, logger)

	ctx := context.Background()

	change := &model.PendingChange{
		ChangeID: "change-1",
		Type:     "decommission",
		Status:   model.PendingChangeCompleted,
		Progress: map[string]model.StreamingProgress{
			"node-1": {
				State: "completed",
			},
		},
		Ranges: []model.TokenRange{
			{Start: 0, End: 100},
		},
		CompletedAt: time.Now().Add(-25 * time.Hour),
	}

	// Mock: Delete pending change
	mockMetadata.On("DeletePendingChange", ctx, "change-1").Return(nil)

	err := service.ExecuteCleanup(ctx, change)

	assert.NoError(t, err)
	mockMetadata.AssertExpectations(t)
}

func TestCleanupService_ForceCleanup(t *testing.T) {
	mockMetadata := new(MockMetadataStore)
	mockClient := new(MockStorageNodeClient)
	logger := zap.NewNop()

	routingService := NewRoutingService(mockMetadata, 150, 30*time.Second, logger)

	service := NewCleanupService(mockMetadata, mockClient, routingService, 24*time.Hour, logger)

	ctx := context.Background()

	change := &model.PendingChange{
		ChangeID: "change-1",
		Type:     "bootstrap",
		Status:   model.PendingChangeInProgress, // Not completed, but force cleanup
	}

	// Mock: Get pending change
	mockMetadata.On("GetPendingChange", ctx, "change-1").Return(change, nil)

	// Mock: Force delete pending change
	mockMetadata.On("DeletePendingChange", ctx, "change-1").Return(nil)

	err := service.ForceCleanup(ctx, "change-1", "emergency recovery")

	assert.NoError(t, err)
	mockMetadata.AssertExpectations(t)
}
