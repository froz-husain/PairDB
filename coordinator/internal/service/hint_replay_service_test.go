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

// MockHintStore is a mock implementation of HintStore
type MockHintStore struct {
	mock.Mock
}

func (m *MockHintStore) StoreHint(ctx context.Context, hint *model.Hint) error {
	args := m.Called(ctx, hint)
	return args.Error(0)
}

func (m *MockHintStore) GetHintsForNode(ctx context.Context, targetNodeID string, limit int) ([]*model.Hint, error) {
	args := m.Called(ctx, targetNodeID, limit)
	return args.Get(0).([]*model.Hint), args.Error(1)
}

func (m *MockHintStore) DeleteHint(ctx context.Context, hintID string) error {
	args := m.Called(ctx, hintID)
	return args.Error(0)
}

func (m *MockHintStore) DeleteHintsForNode(ctx context.Context, targetNodeID string) error {
	args := m.Called(ctx, targetNodeID)
	return args.Error(0)
}

func (m *MockHintStore) ListHints(ctx context.Context, filter store.HintFilter) ([]*model.Hint, error) {
	args := m.Called(ctx, filter)
	return args.Get(0).([]*model.Hint), args.Error(1)
}

func (m *MockHintStore) CleanupOldHints(ctx context.Context, ttl time.Duration) (int64, error) {
	args := m.Called(ctx, ttl)
	return args.Get(0).(int64), args.Error(1)
}

func (m *MockHintStore) GetHintCount(ctx context.Context, targetNodeID string) (int64, error) {
	args := m.Called(ctx, targetNodeID)
	return args.Get(0).(int64), args.Error(1)
}

// MockStorageNodeClient is a mock implementation of StorageNodeClient
type MockStorageNodeClient struct {
	mock.Mock
}

func (m *MockStorageNodeClient) Write(ctx context.Context, node *model.StorageNode, req *client.WriteRequest) (*client.WriteResponse, error) {
	args := m.Called(ctx, node, req)
	return args.Get(0).(*client.WriteResponse), args.Error(1)
}

func TestHintReplayService_ReplayHintsForNode_NoHints(t *testing.T) {
	mockHintStore := new(MockHintStore)
	mockClient := new(MockStorageNodeClient)
	logger := zap.NewNop()

	service := NewHintReplayService(mockHintStore, mockClient, logger)

	node := &model.StorageNode{
		NodeID: "node-1",
		Host:   "localhost",
		Port:   8001,
	}

	ctx := context.Background()

	// Mock: No hints for this node
	mockHintStore.On("GetHintCount", ctx, "node-1").Return(int64(0), nil)

	err := service.ReplayHintsForNode(ctx, node)

	assert.NoError(t, err)
	mockHintStore.AssertExpectations(t)
}

func TestHintReplayService_ReplayHintsForNode_Success(t *testing.T) {
	mockHintStore := new(MockHintStore)
	mockClient := new(MockStorageNodeClient)
	logger := zap.NewNop()

	service := NewHintReplayService(mockHintStore, mockClient, logger)

	node := &model.StorageNode{
		NodeID: "node-1",
		Host:   "localhost",
		Port:   8001,
	}

	ctx := context.Background()

	// Create test hints
	hints := []*model.Hint{
		{
			HintID:       "hint-1",
			TargetNodeID: "node-1",
			TenantID:     "tenant-1",
			Key:          "key-1",
			Value:        []byte("value-1"),
			VectorClock:  "{}",
			Timestamp:    time.Now(),
			CreatedAt:    time.Now(),
			ReplayCount:  0,
		},
		{
			HintID:       "hint-2",
			TargetNodeID: "node-1",
			TenantID:     "tenant-1",
			Key:          "key-2",
			Value:        []byte("value-2"),
			VectorClock:  "{}",
			Timestamp:    time.Now(),
			CreatedAt:    time.Now(),
			ReplayCount:  0,
		},
	}

	// Mock: 2 hints for this node
	mockHintStore.On("GetHintCount", ctx, "node-1").Return(int64(2), nil)
	mockHintStore.On("GetHintsForNode", ctx, "node-1", 100).Return(hints, nil).Once()
	mockHintStore.On("GetHintsForNode", ctx, "node-1", 100).Return([]*model.Hint{}, nil).Once()

	// Mock: Successful writes
	mockClient.On("Write", ctx, node, mock.AnythingOfType("*client.WriteRequest")).
		Return(&client.WriteResponse{Success: true}, nil).Times(2)

	// Mock: Successful hint deletions
	mockHintStore.On("DeleteHint", ctx, "hint-1").Return(nil)
	mockHintStore.On("DeleteHint", ctx, "hint-2").Return(nil)

	err := service.ReplayHintsForNode(ctx, node)

	assert.NoError(t, err)
	mockHintStore.AssertExpectations(t)
	mockClient.AssertExpectations(t)
}

func TestHintReplayService_ReplayHintsForNode_WriteFailure(t *testing.T) {
	mockHintStore := new(MockHintStore)
	mockClient := new(MockStorageNodeClient)
	logger := zap.NewNop()

	service := NewHintReplayService(mockHintStore, mockClient, logger)

	node := &model.StorageNode{
		NodeID: "node-1",
		Host:   "localhost",
		Port:   8001,
	}

	ctx := context.Background()

	// Create test hint that will fail replay
	hints := []*model.Hint{
		{
			HintID:       "hint-1",
			TargetNodeID: "node-1",
			TenantID:     "tenant-1",
			Key:          "key-1",
			Value:        []byte("value-1"),
			VectorClock:  "{}",
			Timestamp:    time.Now(),
			CreatedAt:    time.Now(),
			ReplayCount:  3, // Already at max retries
		},
	}

	// Mock: 1 hint for this node
	mockHintStore.On("GetHintCount", ctx, "node-1").Return(int64(1), nil)
	mockHintStore.On("GetHintsForNode", ctx, "node-1", 100).Return(hints, nil).Once()
	mockHintStore.On("GetHintsForNode", ctx, "node-1", 100).Return([]*model.Hint{}, nil).Once()

	// Mock: Failed write
	mockClient.On("Write", ctx, node, mock.AnythingOfType("*client.WriteRequest")).
		Return(&client.WriteResponse{Success: false, Message: "node unavailable"}, nil)

	// Mock: Hint deletion after max retries exceeded
	mockHintStore.On("DeleteHint", ctx, "hint-1").Return(nil)

	err := service.ReplayHintsForNode(ctx, node)

	assert.NoError(t, err) // Service continues despite failures
	mockHintStore.AssertExpectations(t)
	mockClient.AssertExpectations(t)
}

func TestHintReplayService_CleanupOldHints(t *testing.T) {
	mockHintStore := new(MockHintStore)
	mockClient := new(MockStorageNodeClient)
	logger := zap.NewNop()

	service := NewHintReplayService(mockHintStore, mockClient, logger)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	ttl := 7 * 24 * time.Hour

	// Mock: Cleanup returns 10 deleted hints
	mockHintStore.On("CleanupOldHints", mock.Anything, ttl).Return(int64(10), nil).Maybe()

	// Start cleanup in background
	go service.CleanupOldHints(ctx, ttl)

	// Wait for context to expire
	<-ctx.Done()

	// Verify cleanup was called at least once
	mockHintStore.AssertCalled(t, "CleanupOldHints", mock.Anything, ttl)
}
