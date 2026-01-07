package service

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/devrev/pairdb/storage-node/internal/model"
	"github.com/hashicorp/memberlist"
	"go.uber.org/zap"
)

// GossipService manages cluster membership and health propagation
type GossipService struct {
	config     *GossipConfig
	memberlist *memberlist.Memberlist
	nodeID     string
	logger     *zap.Logger
	healthData *model.HealthStatus
}

// GossipConfig holds gossip protocol configuration
type GossipConfig struct {
	Enabled        bool
	BindPort       int
	SeedNodes      []string
	GossipInterval time.Duration
	ProbeTimeout   time.Duration
	ProbeInterval  time.Duration
}

// NewGossipService creates a new gossip service
func NewGossipService(cfg *GossipConfig, nodeID string, logger *zap.Logger) (*GossipService, error) {
	gs := &GossipService{
		config: cfg,
		nodeID: nodeID,
		logger: logger,
		healthData: &model.HealthStatus{
			NodeID:    nodeID,
			Status:    model.NodeStatusHealthy,
			Timestamp: time.Now().Unix(),
		},
	}

	// Configure memberlist
	mlConfig := memberlist.DefaultLocalConfig()
	mlConfig.Name = nodeID
	mlConfig.BindPort = cfg.BindPort
	mlConfig.GossipInterval = cfg.GossipInterval
	mlConfig.ProbeTimeout = cfg.ProbeTimeout
	mlConfig.ProbeInterval = cfg.ProbeInterval
	mlConfig.Delegate = gs
	mlConfig.Events = &GossipEventDelegate{service: gs}

	// Create memberlist
	ml, err := memberlist.Create(mlConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create memberlist: %w", err)
	}

	gs.memberlist = ml

	// Join seed nodes
	if len(cfg.SeedNodes) > 0 {
		_, err := ml.Join(cfg.SeedNodes)
		if err != nil {
			logger.Warn("Failed to join some seed nodes", zap.Error(err))
		}
	}

	return gs, nil
}

// NodeMeta implements memberlist.Delegate
func (s *GossipService) NodeMeta(limit int) []byte {
	data, _ := json.Marshal(s.healthData)
	if len(data) > limit {
		return data[:limit]
	}
	return data
}

// NotifyMsg implements memberlist.Delegate
func (s *GossipService) NotifyMsg(data []byte) {
	var healthStatus model.HealthStatus
	if err := json.Unmarshal(data, &healthStatus); err != nil {
		s.logger.Warn("Failed to unmarshal gossip message", zap.Error(err))
		return
	}

	s.logger.Debug("Received health status",
		zap.String("node_id", healthStatus.NodeID),
		zap.String("status", string(healthStatus.Status)))
}

// GetBroadcasts implements memberlist.Delegate
func (s *GossipService) GetBroadcasts(overhead, limit int) [][]byte {
	return nil
}

// LocalState implements memberlist.Delegate
func (s *GossipService) LocalState(join bool) []byte {
	data, _ := json.Marshal(s.healthData)
	return data
}

// MergeRemoteState implements memberlist.Delegate
func (s *GossipService) MergeRemoteState(buf []byte, join bool) {
	// No-op for now
}

// UpdateHealthStatus updates the local health status
func (s *GossipService) UpdateHealthStatus(metrics model.HealthMetrics) {
	s.healthData.Timestamp = time.Now().Unix()
	s.healthData.Metrics = metrics

	// Determine status based on metrics
	if metrics.CPUUsage > 90 || metrics.MemoryUsage > 90 || metrics.DiskUsage > 90 {
		s.healthData.Status = model.NodeStatusDegraded
	} else if metrics.ErrorRate > 0.1 {
		s.healthData.Status = model.NodeStatusUnhealthy
	} else {
		s.healthData.Status = model.NodeStatusHealthy
	}
}

// Shutdown shuts down the gossip service
func (s *GossipService) Shutdown() error {
	return s.memberlist.Shutdown()
}

// GossipEventDelegate handles memberlist events
type GossipEventDelegate struct {
	service *GossipService
}

// NotifyJoin is called when a node joins
func (d *GossipEventDelegate) NotifyJoin(node *memberlist.Node) {
	d.service.logger.Info("Node joined",
		zap.String("node_id", node.Name),
		zap.String("addr", node.Addr.String()))
}

// NotifyLeave is called when a node leaves
func (d *GossipEventDelegate) NotifyLeave(node *memberlist.Node) {
	d.service.logger.Info("Node left",
		zap.String("node_id", node.Name))
}

// NotifyUpdate is called when a node is updated
func (d *GossipEventDelegate) NotifyUpdate(node *memberlist.Node) {
	d.service.logger.Debug("Node updated",
		zap.String("node_id", node.Name))
}
