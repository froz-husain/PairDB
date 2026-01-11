package service

import (
	"errors"

	"github.com/devrev/pairdb/coordinator/internal/algorithm"
	"github.com/devrev/pairdb/coordinator/internal/model"
)

// ConsistencyService manages consistency levels and quorum calculations
type ConsistencyService struct {
	defaultLevel string
	quorum       *algorithm.QuorumCalculator
}

// NewConsistencyService creates a new consistency service
func NewConsistencyService(defaultLevel string) *ConsistencyService {
	return &ConsistencyService{
		defaultLevel: defaultLevel,
		quorum:       algorithm.NewQuorumCalculator(),
	}
}

// GetRequiredReplicas calculates the required number of replicas for a consistency level
// Uses simple calculation (backward compatibility - treats all replicas as active)
func (s *ConsistencyService) GetRequiredReplicas(consistency string, totalReplicas int) int {
	return s.quorum.GetRequiredReplicasSimple(consistency, totalReplicas)
}

// GetRequiredReplicasWithStatus calculates required replicas accounting for node status
// This is used during topology changes to exclude bootstrapping/draining nodes from quorum
func (s *ConsistencyService) GetRequiredReplicasWithStatus(consistency string, totalReplicas, activeReplicas int) int {
	return s.quorum.GetRequiredReplicas(consistency, totalReplicas, activeReplicas)
}

// ValidateConsistencyLevel validates if a consistency level is valid
func (s *ConsistencyService) ValidateConsistencyLevel(level string) bool {
	switch level {
	case "one", "quorum", "all":
		return true
	default:
		return false
	}
}

// GetDefaultLevel returns the default consistency level
func (s *ConsistencyService) GetDefaultLevel() string {
	return s.defaultLevel
}

// NormalizeConsistencyLevel returns the consistency level to use, defaulting if empty
func (s *ConsistencyService) NormalizeConsistencyLevel(level string) (string, error) {
	if level == "" {
		return s.defaultLevel, nil
	}
	if !s.ValidateConsistencyLevel(level) {
		return "", errors.New("invalid consistency level: must be one of: one, quorum, all")
	}
	return level, nil
}

// IsQuorumReached checks if quorum is reached for the given responses
func (s *ConsistencyService) IsQuorumReached(successCount, totalReplicas int, consistency string) bool {
	required := s.GetRequiredReplicas(consistency, totalReplicas)
	return successCount >= required
}

// GetRequiredReplicasForWriteSet calculates required replicas for write operations
// This is Cassandra-correct: quorum is calculated from AUTHORITATIVE replicas only
// - Excludes BOOTSTRAPPING nodes (they receive writes but don't count toward quorum)
// - Excludes LEAVING nodes (they receive writes but don't count toward quorum)
// - Only NORMAL nodes count as authoritative
func (s *ConsistencyService) GetRequiredReplicasForWriteSet(consistency string, writeReplicas []*model.StorageNode) int {
	// Count authoritative replicas (only NORMAL state)
	authoritativeCount := 0
	for _, node := range writeReplicas {
		if node.State == model.NodeStateNormal {
			authoritativeCount++
		}
	}

	// Calculate required replicas based on consistency level
	switch consistency {
	case "one":
		return 1
	case "all":
		// For ALL consistency during topology changes:
		// ALL means all AUTHORITATIVE replicas, not including bootstrapping/leaving
		return authoritativeCount
	case "quorum":
		fallthrough
	default:
		// Quorum calculated from authoritative replicas only
		if authoritativeCount > 0 {
			return s.quorum.CalculateQuorum(authoritativeCount)
		}
		// If no authoritative replicas, require at least one write to succeed
		return 1
	}
}
