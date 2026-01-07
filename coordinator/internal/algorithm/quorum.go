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
func (q *QuorumCalculator) GetRequiredReplicas(consistency string, totalReplicas int) int {
	switch consistency {
	case "one":
		return 1
	case "all":
		return totalReplicas
	case "quorum":
		fallthrough
	default:
		return q.CalculateQuorum(totalReplicas)
	}
}

// IsQuorumReached checks if quorum is reached
func (q *QuorumCalculator) IsQuorumReached(successCount, totalReplicas int) bool {
	quorum := q.CalculateQuorum(totalReplicas)
	return successCount >= quorum
}
