package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/devrev/pairdb/coordinator/internal/algorithm"
	"github.com/devrev/pairdb/coordinator/internal/model"
	"github.com/devrev/pairdb/coordinator/internal/store"
	"go.uber.org/zap"
)

// RoutingService manages consistent hashing and replica routing
type RoutingService struct {
	metadataStore store.MetadataStore
	hashRing      *algorithm.ConsistentHash
	mu            sync.RWMutex
	updateTicker  *time.Ticker
	stopCh        chan struct{}
	logger        *zap.Logger
}

// NewRoutingService creates a new routing service
func NewRoutingService(
	metadataStore store.MetadataStore,
	virtualNodes int,
	updateInterval time.Duration,
	logger *zap.Logger,
) *RoutingService {
	rs := &RoutingService{
		metadataStore: metadataStore,
		hashRing:      algorithm.NewConsistentHash(virtualNodes),
		updateTicker:  time.NewTicker(updateInterval),
		stopCh:        make(chan struct{}),
		logger:        logger,
	}

	// Start background hash ring updater
	go rs.refreshHashRing()

	return rs
}

// GetReplicas returns the storage nodes for the given tenant and key
func (s *RoutingService) GetReplicas(tenantID, key string, replicationFactor int) ([]*model.StorageNode, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Create composite key for consistent hashing
	compositeKey := fmt.Sprintf("%s:%s", tenantID, key)

	// Get nodes from hash ring
	nodeIDs := s.hashRing.GetNodes(compositeKey, replicationFactor)
	if len(nodeIDs) < replicationFactor {
		return nil, fmt.Errorf("insufficient storage nodes: need %d, got %d", replicationFactor, len(nodeIDs))
	}

	// Get node details from hash ring
	nodes := make([]*model.StorageNode, 0, len(nodeIDs))
	for _, nodeID := range nodeIDs {
		node := s.hashRing.GetNodeByID(nodeID)
		if node != nil {
			nodes = append(nodes, node)
		}
	}

	if len(nodes) < replicationFactor {
		return nil, fmt.Errorf("failed to resolve all storage nodes: need %d, got %d", replicationFactor, len(nodes))
	}

	s.logger.Debug("Resolved replicas",
		zap.String("tenant_id", tenantID),
		zap.String("key", key),
		zap.Int("replication_factor", replicationFactor),
		zap.Int("resolved_nodes", len(nodes)))

	return nodes, nil
}

// refreshHashRing periodically refreshes the hash ring from metadata store
func (s *RoutingService) refreshHashRing() {
	// Initial load
	ctx := context.Background()
	if err := s.updateHashRing(ctx); err != nil {
		s.logger.Error("Failed initial hash ring load", zap.Error(err))
	}

	// Periodic refresh
	for {
		select {
		case <-s.updateTicker.C:
			if err := s.updateHashRing(ctx); err != nil {
				s.logger.Error("Failed to update hash ring", zap.Error(err))
			}
		case <-s.stopCh:
			s.updateTicker.Stop()
			return
		}
	}
}

// updateHashRing fetches active storage nodes and updates the hash ring
func (s *RoutingService) updateHashRing(ctx context.Context) error {
	// Fetch active storage nodes from metadata store
	nodes, err := s.metadataStore.ListStorageNodes(ctx)
	if err != nil {
		return fmt.Errorf("failed to list storage nodes: %w", err)
	}

	// Filter active nodes only
	activeNodes := make([]*model.StorageNode, 0)
	for _, node := range nodes {
		if node.Status == "active" {
			activeNodes = append(activeNodes, node)
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Clear and rebuild hash ring
	s.hashRing.Clear()
	for _, node := range activeNodes {
		s.hashRing.AddNode(node)
	}

	s.logger.Info("Hash ring updated",
		zap.Int("total_nodes", len(nodes)),
		zap.Int("active_nodes", len(activeNodes)))

	return nil
}

// AddNode adds a storage node to the hash ring
func (s *RoutingService) AddNode(ctx context.Context, node *model.StorageNode) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.hashRing.AddNode(node)

	s.logger.Info("Added node to hash ring",
		zap.String("node_id", node.NodeID),
		zap.String("host", node.Host),
		zap.Int("port", node.Port))

	return nil
}

// RemoveNode removes a storage node from the hash ring
func (s *RoutingService) RemoveNode(ctx context.Context, nodeID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.hashRing.RemoveNode(nodeID)

	s.logger.Info("Removed node from hash ring",
		zap.String("node_id", nodeID))

	return nil
}

// GetNodeCount returns the current number of nodes in the hash ring
func (s *RoutingService) GetNodeCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.hashRing.GetNodeCount()
}

// Stop stops the routing service
func (s *RoutingService) Stop() {
	close(s.stopCh)
	s.logger.Info("Routing service stopped")
}
