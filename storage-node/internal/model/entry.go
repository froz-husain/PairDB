package model

import "time"

// VectorClockEntry represents a single entry in the vector clock
type VectorClockEntry struct {
	CoordinatorNodeID string
	LogicalTimestamp  int64
}

// VectorClock tracks causality across coordinators
type VectorClock struct {
	Entries []VectorClockEntry
}

// KeyValueEntry represents a complete key-value pair with metadata
type KeyValueEntry struct {
	TenantID    string
	Key         string
	Value       []byte
	VectorClock VectorClock
	Timestamp   int64
}

// CommitLogEntry represents an entry in the commit log
type CommitLogEntry struct {
	TenantID      string
	Key           string
	Value         []byte
	VectorClock   VectorClock
	Timestamp     int64
	OperationType OperationType
}

// OperationType defines the type of operation
type OperationType string

const (
	OperationTypeWrite  OperationType = "write"
	OperationTypeRepair OperationType = "repair"
	OperationTypeDelete OperationType = "delete"
}

// MemTableEntry represents an entry in the memtable
type MemTableEntry struct {
	Key         string // Format: "{tenant_id}:{key}"
	Value       []byte
	VectorClock VectorClock
	Timestamp   int64
}

// CacheEntry represents an entry in the cache
type CacheEntry struct {
	Key         string // Format: "{tenant_id}:{key}"
	Value       []byte
	VectorClock VectorClock
	AccessCount int64     // For LFU
	LastAccess  time.Time // For LRU
	Score       float64   // Adaptive score
}
