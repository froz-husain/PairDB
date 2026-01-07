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
