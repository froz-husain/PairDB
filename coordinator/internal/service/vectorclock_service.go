package service

import (
	"sync"

	"github.com/devrev/pairdb/coordinator/internal/algorithm"
	"github.com/devrev/pairdb/coordinator/internal/model"
)

// VectorClockService manages vector clocks for causality tracking
type VectorClockService struct {
	nodeID         string
	localTimestamp int64
	mu             sync.Mutex
	vcOps          *algorithm.VectorClockOps
}

// NewVectorClockService creates a new vector clock service
func NewVectorClockService(nodeID string) *VectorClockService {
	return &VectorClockService{
		nodeID:         nodeID,
		localTimestamp: 0,
		vcOps:          algorithm.NewVectorClockOps(),
	}
}

// Increment increments the vector clock for this coordinator
func (s *VectorClockService) Increment(nodeID string) model.VectorClock {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.localTimestamp++
	return model.VectorClock{
		Entries: []model.VectorClockEntry{
			{
				CoordinatorNodeID: s.nodeID,
				LogicalTimestamp:  s.localTimestamp,
			},
		},
	}
}

// IncrementFrom increments the vector clock based on an existing vector clock
// This is used for read-modify-write operations to maintain causality
func (s *VectorClockService) IncrementFrom(existing model.VectorClock) model.VectorClock {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.localTimestamp++

	// Build a map of existing entries
	vcMap := make(map[string]int64)
	for _, entry := range existing.Entries {
		vcMap[entry.CoordinatorNodeID] = entry.LogicalTimestamp
	}

	// Increment the local timestamp for this coordinator
	// Take max of existing timestamp and incremented local timestamp
	if existingTS, exists := vcMap[s.nodeID]; exists {
		if existingTS >= s.localTimestamp {
			s.localTimestamp = existingTS + 1
		}
	}
	vcMap[s.nodeID] = s.localTimestamp

	// Convert map back to entries
	entries := make([]model.VectorClockEntry, 0, len(vcMap))
	for nodeID, ts := range vcMap {
		entries = append(entries, model.VectorClockEntry{
			CoordinatorNodeID: nodeID,
			LogicalTimestamp:  ts,
		})
	}

	return model.VectorClock{Entries: entries}
}

// Merge merges multiple vector clocks
func (s *VectorClockService) Merge(clocks ...model.VectorClock) model.VectorClock {
	return s.vcOps.Merge(clocks...)
}

// Compare compares two vector clocks
func (s *VectorClockService) Compare(vc1, vc2 model.VectorClock) model.VectorClockComparison {
	return s.vcOps.Compare(vc1, vc2)
}

// GetMaxTimestamp returns the maximum timestamp in a vector clock
func (s *VectorClockService) GetMaxTimestamp(vc model.VectorClock) int64 {
	return s.vcOps.GetMaxTimestamp(vc)
}
