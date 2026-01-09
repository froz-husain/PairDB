package service

import (
	"errors"

	"github.com/devrev/pairdb/coordinator/internal/algorithm"
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
