package service

import (
	"github.com/devrev/pairdb/storage-node/internal/model"
)

// VectorClockService manages vector clock operations
type VectorClockService struct {
}

// NewVectorClockService creates a new vector clock service
func NewVectorClockService() *VectorClockService {
	return &VectorClockService{}
}

// Compare compares two vector clocks
// Returns: -1 (a < b), 0 (concurrent), 1 (a > b)
func (s *VectorClockService) Compare(a, b model.VectorClock) int {
	aMap := s.toMap(a)
	bMap := s.toMap(b)

	aGreater := false
	bGreater := false

	// Check all nodes in a
	for node, aTS := range aMap {
		bTS, exists := bMap[node]
		if !exists || aTS > bTS {
			aGreater = true
		} else if aTS < bTS {
			bGreater = true
		}
	}

	// Check nodes only in b
	for node, bTS := range bMap {
		if _, exists := aMap[node]; !exists && bTS > 0 {
			bGreater = true
		}
	}

	if aGreater && !bGreater {
		return 1
	} else if !aGreater && bGreater {
		return -1
	}
	return 0
}

// Merge merges two vector clocks
func (s *VectorClockService) Merge(a, b model.VectorClock) model.VectorClock {
	aMap := s.toMap(a)
	bMap := s.toMap(b)

	merged := make(map[string]int64)

	// Take maximum timestamp for each node
	for node, ts := range aMap {
		merged[node] = ts
	}

	for node, ts := range bMap {
		if existing, exists := merged[node]; !exists || ts > existing {
			merged[node] = ts
		}
	}

	return s.fromMap(merged)
}

// Increment increments the timestamp for a specific node
func (s *VectorClockService) Increment(vc model.VectorClock, nodeID string) model.VectorClock {
	vcMap := s.toMap(vc)
	vcMap[nodeID]++
	return s.fromMap(vcMap)
}

// toMap converts vector clock to map
func (s *VectorClockService) toMap(vc model.VectorClock) map[string]int64 {
	result := make(map[string]int64)
	for _, entry := range vc.Entries {
		result[entry.CoordinatorNodeID] = entry.LogicalTimestamp
	}
	return result
}

// fromMap converts map to vector clock
func (s *VectorClockService) fromMap(m map[string]int64) model.VectorClock {
	entries := make([]model.VectorClockEntry, 0, len(m))
	for node, ts := range m {
		entries = append(entries, model.VectorClockEntry{
			CoordinatorNodeID: node,
			LogicalTimestamp:  ts,
		})
	}
	return model.VectorClock{Entries: entries}
}
