package algorithm

import (
	"github.com/devrev/pairdb/coordinator/internal/model"
)

// VectorClockOps provides operations on vector clocks
type VectorClockOps struct{}

// NewVectorClockOps creates a new VectorClockOps
func NewVectorClockOps() *VectorClockOps {
	return &VectorClockOps{}
}

// Compare compares two vector clocks
func (v *VectorClockOps) Compare(vc1, vc2 model.VectorClock) model.VectorClockComparison {
	// Build maps for easier comparison
	map1 := v.toMap(vc1)
	map2 := v.toMap(vc2)

	allBefore := true
	allAfter := true

	// Get all node IDs
	allNodes := make(map[string]bool)
	for nodeID := range map1 {
		allNodes[nodeID] = true
	}
	for nodeID := range map2 {
		allNodes[nodeID] = true
	}

	// Compare timestamps
	for nodeID := range allNodes {
		ts1 := map1[nodeID]
		ts2 := map2[nodeID]

		if ts1 < ts2 {
			allAfter = false
		} else if ts1 > ts2 {
			allBefore = false
		}
	}

	// Determine relationship
	if allBefore && allAfter {
		return model.Identical
	}
	if allBefore {
		return model.Before
	}
	if allAfter {
		return model.After
	}
	return model.Concurrent
}

// Merge merges multiple vector clocks
func (v *VectorClockOps) Merge(clocks ...model.VectorClock) model.VectorClock {
	merged := make(map[string]int64)

	for _, clock := range clocks {
		for _, entry := range clock.Entries {
			if existing, exists := merged[entry.CoordinatorNodeID]; !exists || entry.LogicalTimestamp > existing {
				merged[entry.CoordinatorNodeID] = entry.LogicalTimestamp
			}
		}
	}

	entries := make([]model.VectorClockEntry, 0, len(merged))
	for nodeID, timestamp := range merged {
		entries = append(entries, model.VectorClockEntry{
			CoordinatorNodeID: nodeID,
			LogicalTimestamp:  timestamp,
		})
	}

	return model.VectorClock{Entries: entries}
}

// Increment increments the vector clock for a given node
func (v *VectorClockOps) Increment(vc model.VectorClock, nodeID string) model.VectorClock {
	vcMap := v.toMap(vc)
	vcMap[nodeID]++

	entries := make([]model.VectorClockEntry, 0, len(vcMap))
	for nid, ts := range vcMap {
		entries = append(entries, model.VectorClockEntry{
			CoordinatorNodeID: nid,
			LogicalTimestamp:  ts,
		})
	}

	return model.VectorClock{Entries: entries}
}

// GetMaxTimestamp returns the maximum timestamp in the vector clock
func (v *VectorClockOps) GetMaxTimestamp(vc model.VectorClock) int64 {
	var max int64
	for _, entry := range vc.Entries {
		if entry.LogicalTimestamp > max {
			max = entry.LogicalTimestamp
		}
	}
	return max
}

// toMap converts vector clock to map for easier operations
func (v *VectorClockOps) toMap(vc model.VectorClock) map[string]int64 {
	m := make(map[string]int64)
	for _, entry := range vc.Entries {
		m[entry.CoordinatorNodeID] = entry.LogicalTimestamp
	}
	return m
}
