package algorithm

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"sort"
	"sync"

	"github.com/devrev/pairdb/coordinator/internal/model"
)

// ConsistentHasher implements consistent hashing with virtual nodes
type ConsistentHasher struct {
	ring       []uint64              // Sorted hash values
	ringMap    map[uint64]string     // Hash -> VNodeID
	nodeVNodes map[string][]uint64   // NodeID -> VNode hashes
	mu         sync.RWMutex
}

// NewConsistentHasher creates a new consistent hasher
func NewConsistentHasher() *ConsistentHasher {
	return &ConsistentHasher{
		ring:       make([]uint64, 0),
		ringMap:    make(map[uint64]string),
		nodeVNodes: make(map[string][]uint64),
	}
}

// AddNode adds a physical node with virtual nodes
func (ch *ConsistentHasher) AddNode(nodeID string, virtualNodeCount int) {
	ch.mu.Lock()
	defer ch.mu.Unlock()

	vnodeHashes := make([]uint64, 0, virtualNodeCount)

	for i := 0; i < virtualNodeCount; i++ {
		vnodeID := fmt.Sprintf("%s-vnode-%d", nodeID, i)
		hash := ch.hash(vnodeID)

		ch.ring = append(ch.ring, hash)
		ch.ringMap[hash] = vnodeID
		vnodeHashes = append(vnodeHashes, hash)
	}

	ch.nodeVNodes[nodeID] = vnodeHashes
	sort.Slice(ch.ring, func(i, j int) bool { return ch.ring[i] < ch.ring[j] })
}

// RemoveNode removes a physical node and its virtual nodes
func (ch *ConsistentHasher) RemoveNode(nodeID string) {
	ch.mu.Lock()
	defer ch.mu.Unlock()

	vnodeHashes, exists := ch.nodeVNodes[nodeID]
	if !exists {
		return
	}

	// Remove from ring
	hashSet := make(map[uint64]bool)
	for _, hash := range vnodeHashes {
		hashSet[hash] = true
		delete(ch.ringMap, hash)
	}

	newRing := make([]uint64, 0, len(ch.ring)-len(vnodeHashes))
	for _, hash := range ch.ring {
		if !hashSet[hash] {
			newRing = append(newRing, hash)
		}
	}
	ch.ring = newRing

	delete(ch.nodeVNodes, nodeID)
}

// GetNodes returns N replica nodes for a given hash
func (ch *ConsistentHasher) GetNodes(keyHash uint64, count int) []*model.VirtualNode {
	ch.mu.RLock()
	defer ch.mu.RUnlock()

	if len(ch.ring) == 0 {
		return nil
	}

	// Find position in ring
	idx := sort.Search(len(ch.ring), func(i int) bool {
		return ch.ring[i] >= keyHash
	})

	// Wrap around if necessary
	if idx >= len(ch.ring) {
		idx = 0
	}

	// Collect unique physical nodes
	nodes := make([]*model.VirtualNode, 0, count)
	seen := make(map[string]bool)

	for i := 0; i < len(ch.ring) && len(nodes) < count; i++ {
		ringIdx := (idx + i) % len(ch.ring)
		hash := ch.ring[ringIdx]
		vnodeID := ch.ringMap[hash]

		// Extract physical node ID (format: nodeID-vnode-X)
		nodeID := ch.extractNodeID(vnodeID)

		if !seen[nodeID] {
			nodes = append(nodes, &model.VirtualNode{
				VNodeID: vnodeID,
				Hash:    hash,
				NodeID:  nodeID,
			})
			seen[nodeID] = true
		}
	}

	return nodes
}

// Hash computes hash for a key
func (ch *ConsistentHasher) Hash(key string) uint64 {
	return ch.hash(key)
}

// Clear removes all nodes
func (ch *ConsistentHasher) Clear() {
	ch.mu.Lock()
	defer ch.mu.Unlock()

	ch.ring = make([]uint64, 0)
	ch.ringMap = make(map[uint64]string)
	ch.nodeVNodes = make(map[string][]uint64)
}

// hash computes SHA-256 hash and converts to uint64
func (ch *ConsistentHasher) hash(key string) uint64 {
	h := sha256.New()
	h.Write([]byte(key))
	hashBytes := h.Sum(nil)
	return binary.BigEndian.Uint64(hashBytes[:8])
}

// extractNodeID extracts physical node ID from virtual node ID
// Format: nodeID-vnode-X
func (ch *ConsistentHasher) extractNodeID(vnodeID string) string {
	// Find the last occurrence of "-vnode-"
	vnodeSuffix := "-vnode-"
	idx := len(vnodeID) - len(vnodeSuffix)

	for ; idx >= 0; idx-- {
		if idx+len(vnodeSuffix) <= len(vnodeID) {
			if vnodeID[idx:idx+len(vnodeSuffix)] == vnodeSuffix {
				return vnodeID[:idx]
			}
		}
	}

	return vnodeID
}

// NodeCount returns the number of physical nodes
func (ch *ConsistentHasher) NodeCount() int {
	ch.mu.RLock()
	defer ch.mu.RUnlock()
	return len(ch.nodeVNodes)
}
