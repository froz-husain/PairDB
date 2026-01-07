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
