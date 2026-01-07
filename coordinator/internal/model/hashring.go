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
	Status       NodeStatus
	VirtualNodes int
}

// NodeStatus represents the status of a storage node
type NodeStatus string

const (
	// NodeStatusActive indicates node is active and accepting requests
	NodeStatusActive NodeStatus = "active"
	// NodeStatusDraining indicates node is being drained (migration in progress)
	NodeStatusDraining NodeStatus = "draining"
	// NodeStatusInactive indicates node is inactive
	NodeStatusInactive NodeStatus = "inactive"
}

// VirtualNode represents a virtual node in the hash ring
type VirtualNode struct {
	VNodeID string
	Hash    uint64
	NodeID  string
}
