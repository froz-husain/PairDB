package model

// VectorClockEntry represents a single entry in the vector clock
type VectorClockEntry struct {
	CoordinatorNodeID string
	LogicalTimestamp  int64
}

// VectorClock represents a vector clock for causality tracking
type VectorClock struct {
	Entries []VectorClockEntry
}

// VectorClockComparison represents the result of comparing two vector clocks
type VectorClockComparison int

const (
	// Identical means both vector clocks are identical
	Identical VectorClockComparison = iota
	// Before means first happens before second
	Before
	// After means first happens after second
	After
	// Concurrent means conflict (siblings)
	Concurrent
)

// Equals checks if two vector clocks are identical
func (vc VectorClock) Equals(other VectorClock) bool {
	if len(vc.Entries) != len(other.Entries) {
		return false
	}

	// Create map for quick lookup
	vcMap := make(map[string]int64)
	for _, entry := range vc.Entries {
		vcMap[entry.CoordinatorNodeID] = entry.LogicalTimestamp
	}

	// Check if all entries in other match
	for _, entry := range other.Entries {
		timestamp, exists := vcMap[entry.CoordinatorNodeID]
		if !exists || timestamp != entry.LogicalTimestamp {
			return false
		}
	}

	return true
}
