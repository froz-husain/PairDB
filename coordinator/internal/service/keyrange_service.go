package service

import (
	"context"

	"github.com/devrev/pairdb/coordinator/internal/algorithm"
	"github.com/devrev/pairdb/coordinator/internal/model"
	"go.uber.org/zap"
)

// KeyRangeService calculates key ranges for node addition/removal
type KeyRangeService struct {
	logger *zap.Logger
}

// NewKeyRangeService creates a new key range service
func NewKeyRangeService(logger *zap.Logger) *KeyRangeService {
	return &KeyRangeService{
		logger: logger,
	}
}

// KeyRangeAssignment represents which key ranges should move from old node to new node
type KeyRangeAssignment struct {
	OldNodeID string
	NewNodeID string
	Ranges    []KeyRange
}

// KeyRange represents a hash range
type KeyRange struct {
	StartHash uint64
	EndHash   uint64
}

// CalculateKeyRangesForNewNode calculates which key ranges will move to a new node
// Returns a map of oldNodeID -> list of ranges that should be streamed to new node
func (s *KeyRangeService) CalculateKeyRangesForNewNode(
	ctx context.Context,
	currentHashRing *algorithm.ConsistentHasher,
	newNode *model.StorageNode,
) (map[string][]KeyRange, error) {
	s.logger.Info("Calculating key ranges for new node",
		zap.String("new_node_id", newNode.NodeID))

	// Create a temporary hash ring with the new node added
	tempRing := s.cloneHashRing(currentHashRing)
	tempRing.AddNode(newNode.NodeID, newNode.VirtualNodes)

	// Get all virtual nodes in the temporary ring
	allVNodes := tempRing.GetAllVirtualNodes()

	// Map: oldNodeID -> list of ranges
	rangeAssignments := make(map[string][]KeyRange)

	// For each virtual node owned by the new node, find the old owner
	for _, vnode := range allVNodes {
		if vnode.NodeID != newNode.NodeID {
			continue
		}

		// This virtual node belongs to the new node
		// Find which old node previously owned this range
		oldOwner := currentHashRing.GetNodeForHash(vnode.Hash)

		if oldOwner == "" || oldOwner == newNode.NodeID {
			// No old owner (shouldn't happen) or new node already owned it
			continue
		}

		// Calculate the key range for this virtual node
		// Keys with hash in [prevHash, vnode.Hash) will move from oldOwner to newNode
		prevHash := tempRing.GetPreviousHash(vnode.Hash)

		keyRange := KeyRange{
			StartHash: prevHash,
			EndHash:   vnode.Hash,
		}

		// Add to assignments for this old node
		rangeAssignments[oldOwner] = append(rangeAssignments[oldOwner], keyRange)

		s.logger.Debug("Key range assignment",
			zap.String("old_node", oldOwner),
			zap.String("new_node", newNode.NodeID),
			zap.Uint64("start_hash", prevHash),
			zap.Uint64("end_hash", vnode.Hash))
	}

	s.logger.Info("Key range calculation complete",
		zap.String("new_node_id", newNode.NodeID),
		zap.Int("affected_nodes", len(rangeAssignments)))

	return rangeAssignments, nil
}

// CalculateKeyRangesForRemovedNode calculates which nodes will inherit ranges from a removed node
// Returns a map of inheritingNodeID -> list of ranges that should be streamed from removed node
func (s *KeyRangeService) CalculateKeyRangesForRemovedNode(
	ctx context.Context,
	currentHashRing *algorithm.ConsistentHasher,
	removedNode *model.StorageNode,
) (map[string][]KeyRange, error) {
	s.logger.Info("Calculating key ranges for removed node",
		zap.String("removed_node_id", removedNode.NodeID))

	// Get all virtual nodes for the removed node
	allVNodes := currentHashRing.GetAllVirtualNodes()

	// Create a temporary hash ring without the removed node
	tempRing := s.cloneHashRing(currentHashRing)
	tempRing.RemoveNode(removedNode.NodeID)

	// Map: inheritingNodeID -> list of ranges
	rangeAssignments := make(map[string][]KeyRange)

	// For each virtual node owned by the removed node, find the new owner
	for _, vnode := range allVNodes {
		if vnode.NodeID != removedNode.NodeID {
			continue
		}

		// This virtual node belongs to the removed node
		// Find which node will inherit this range
		newOwner := tempRing.GetNodeForHash(vnode.Hash)

		if newOwner == "" || newOwner == removedNode.NodeID {
			// No new owner found (shouldn't happen)
			continue
		}

		// Calculate the key range for this virtual node
		prevHash := currentHashRing.GetPreviousHash(vnode.Hash)

		keyRange := KeyRange{
			StartHash: prevHash,
			EndHash:   vnode.Hash,
		}

		// Add to assignments for the inheriting node
		rangeAssignments[newOwner] = append(rangeAssignments[newOwner], keyRange)

		s.logger.Debug("Key range inheritance",
			zap.String("removed_node", removedNode.NodeID),
			zap.String("inheriting_node", newOwner),
			zap.Uint64("start_hash", prevHash),
			zap.Uint64("end_hash", vnode.Hash))
	}

	s.logger.Info("Key range calculation complete for removal",
		zap.String("removed_node_id", removedNode.NodeID),
		zap.Int("inheriting_nodes", len(rangeAssignments)))

	return rangeAssignments, nil
}

// cloneHashRing creates a copy of the hash ring for simulation
func (s *KeyRangeService) cloneHashRing(original *algorithm.ConsistentHasher) *algorithm.ConsistentHasher {
	// Create new hash ring
	newRing := algorithm.NewConsistentHasher()

	// Copy all virtual nodes from original
	allVNodes := original.GetAllVirtualNodes()

	// Group by node ID
	nodeVirtualCounts := make(map[string]int)
	for _, vnode := range allVNodes {
		nodeVirtualCounts[vnode.NodeID]++
	}

	// Add each node with its virtual node count
	for nodeID, count := range nodeVirtualCounts {
		newRing.AddNode(nodeID, count)
	}

	return newRing
}

// MergeRanges merges adjacent or overlapping key ranges
// This is useful for reducing the number of ranges to stream
func (s *KeyRangeService) MergeRanges(ranges []KeyRange) []KeyRange {
	if len(ranges) == 0 {
		return ranges
	}

	// Sort ranges by start hash
	sortedRanges := make([]KeyRange, len(ranges))
	copy(sortedRanges, ranges)

	// Simple bubble sort (sufficient for small lists)
	for i := 0; i < len(sortedRanges); i++ {
		for j := i + 1; j < len(sortedRanges); j++ {
			if sortedRanges[j].StartHash < sortedRanges[i].StartHash {
				sortedRanges[i], sortedRanges[j] = sortedRanges[j], sortedRanges[i]
			}
		}
	}

	// Merge adjacent ranges
	merged := []KeyRange{sortedRanges[0]}

	for i := 1; i < len(sortedRanges); i++ {
		current := sortedRanges[i]
		last := &merged[len(merged)-1]

		// Check if current range is adjacent to or overlaps with last range
		if current.StartHash <= last.EndHash {
			// Merge: extend the end of last range
			if current.EndHash > last.EndHash {
				last.EndHash = current.EndHash
			}
		} else {
			// Not adjacent, add as new range
			merged = append(merged, current)
		}
	}

	s.logger.Debug("Merged key ranges",
		zap.Int("original_count", len(ranges)),
		zap.Int("merged_count", len(merged)))

	return merged
}

// EstimateKeyCount estimates the number of keys in a range
// This is a rough estimate based on hash distribution
func (s *KeyRangeService) EstimateKeyCount(totalKeys int64, rangeStart, rangeEnd uint64) int64 {
	// Calculate the proportion of the hash space covered by this range
	hashSpaceSize := uint64(0xFFFFFFFFFFFFFFFF) // Max uint64

	var rangeSize uint64
	if rangeEnd >= rangeStart {
		rangeSize = rangeEnd - rangeStart
	} else {
		// Wrap-around case
		rangeSize = (hashSpaceSize - rangeStart) + rangeEnd
	}

	// Estimate keys in this range
	proportion := float64(rangeSize) / float64(hashSpaceSize)
	estimatedKeys := int64(float64(totalKeys) * proportion)

	return estimatedKeys
}
