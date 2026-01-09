package algorithm

// QuorumCalculator calculates quorum requirements
type QuorumCalculator struct{}

// NewQuorumCalculator creates a new quorum calculator
func NewQuorumCalculator() *QuorumCalculator {
	return &QuorumCalculator{}
}

// CalculateQuorum returns the number of replicas required for quorum
func (q *QuorumCalculator) CalculateQuorum(totalReplicas int) int {
	return (totalReplicas / 2) + 1
}

// GetRequiredReplicas returns the number of replicas required based on consistency level
// totalReplicas: total number of replicas including bootstrapping nodes
// activeReplicas: number of fully active replicas (excludes bootstrapping/draining)
func (q *QuorumCalculator) GetRequiredReplicas(consistency string, totalReplicas, activeReplicas int) int {
	switch consistency {
	case "one":
		return 1
	case "all":
		return totalReplicas
	case "quorum":
		fallthrough
	default:
		// Quorum should be calculated based on active replicas only
		// This prevents counting bootstrapping nodes that may not have all data yet
		if activeReplicas > 0 {
			return q.CalculateQuorum(activeReplicas)
		}
		// Fallback to total replicas if no active replicas specified
		return q.CalculateQuorum(totalReplicas)
	}
}

// GetRequiredReplicasSimple returns required replicas using total count (backward compatibility)
func (q *QuorumCalculator) GetRequiredReplicasSimple(consistency string, totalReplicas int) int {
	return q.GetRequiredReplicas(consistency, totalReplicas, totalReplicas)
}

// IsQuorumReached checks if quorum is reached
func (q *QuorumCalculator) IsQuorumReached(successCount, totalReplicas int) bool {
	quorum := q.CalculateQuorum(totalReplicas)
	return successCount >= quorum
}
