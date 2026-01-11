package model

import "time"

// HashRing represents the consistent hash ring state
type HashRing struct {
	Nodes        map[string]*StorageNode
	VirtualNodes []*VirtualNode
	LastUpdated  time.Time
}

// StorageNode represents a physical storage node
type StorageNode struct {
	NodeID       string
	Host         string
	Port         int
	Status       NodeStatus // Kept for backward compatibility
	State        NodeState  // NEW: Use this for Cassandra-correct lifecycle
	VirtualNodes int
	Tokens       []uint64 // NEW: Actual token positions in the ring

	// NEW: Bidirectional range tracking for Cassandra-correct topology changes
	PendingRanges []PendingRangeInfo // Ranges being received (bootstrap or inheriting)
	LeavingRanges []LeavingRangeInfo // Ranges being transferred (decommission)

	CreatedAt time.Time
	UpdatedAt time.Time
}

// NodeStatus represents the status of a storage node
type NodeStatus string

const (
	NodeStatusActive        NodeStatus = "active"
	NodeStatusBootstrapping NodeStatus = "bootstrapping" // Node is receiving new writes but still copying old data
	NodeStatusDraining      NodeStatus = "draining"
	NodeStatusInactive      NodeStatus = "inactive"
)

// VirtualNode represents a virtual node in the hash ring
type VirtualNode struct {
	VNodeID string
	Hash    uint64
	NodeID  string
}
