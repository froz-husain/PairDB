package model

import "time"

// NodeState represents the lifecycle state of a storage node
type NodeState string

const (
	// NodeStateNormal indicates a fully operational node
	NodeStateNormal NodeState = "NORMAL"
	// NodeStateBootstrapping indicates a node receiving historical data
	NodeStateBootstrapping NodeState = "BOOTSTRAPPING"
	// NodeStateLeaving indicates a node transferring data before removal
	NodeStateLeaving NodeState = "LEAVING"
	// NodeStateDown indicates a failed or unreachable node
	NodeStateDown NodeState = "DOWN"
)

// TokenRange represents a hash range [Start, End)
type TokenRange struct {
	Start uint64 `json:"start"`
	End   uint64 `json:"end"`
}

// PendingRangeInfo tracks ranges being received during bootstrap or decommission
// This is stored on nodes that are RECEIVING ranges (new owners)
type PendingRangeInfo struct {
	Range          TokenRange `json:"range"`
	OldOwnerID     string     `json:"old_owner_id"`     // Source node (streaming FROM)
	NewOwnerID     string     `json:"new_owner_id"`     // This node (streaming TO)
	StreamingState string     `json:"streaming_state"`  // "pending" | "streaming" | "completed"
}

// LeavingRangeInfo tracks ranges being transferred during decommission
// This is stored on nodes that are LEAVING (old owners)
type LeavingRangeInfo struct {
	Range          TokenRange `json:"range"`
	OldOwnerID     string     `json:"old_owner_id"`     // This node (leaving)
	NewOwnerID     string     `json:"new_owner_id"`     // Inheriting node (streaming TO)
	StreamingState string     `json:"streaming_state"`  // "pending" | "streaming" | "completed"
}

// PendingChangeStatus tracks the lifecycle of topology changes
type PendingChangeStatus string

const (
	// PendingChangeInProgress indicates ongoing topology change
	PendingChangeInProgress PendingChangeStatus = "in_progress"
	// PendingChangeCompleted indicates successfully completed change
	PendingChangeCompleted PendingChangeStatus = "completed"
	// PendingChangeFailed indicates failed topology change
	PendingChangeFailed PendingChangeStatus = "failed"
	// PendingChangeRolledBack indicates reverted topology change
	PendingChangeRolledBack PendingChangeStatus = "rolled_back"
)

// StreamingProgress tracks per-stream progress during topology changes
type StreamingProgress struct {
	SourceNodeID     string    `json:"source_node_id"`
	TargetNodeID     string    `json:"target_node_id"`
	KeysCopied       int64     `json:"keys_copied"`
	KeysStreamed     int64     `json:"keys_streamed"`
	BytesTransferred int64     `json:"bytes_transferred"`
	State            string    `json:"state"` // "streaming" | "completed" | "failed"
	LastUpdate       time.Time `json:"last_update"`
}

// PendingChange tracks ongoing topology changes (bootstrap or decommission)
// Stored in metadata store for durability and coordinator failover support
type PendingChange struct {
	ChangeID      string                       `json:"change_id"`
	Type          string                       `json:"type"` // "bootstrap" | "decommission"
	NodeID        string                       `json:"node_id"`
	AffectedNodes []string                     `json:"affected_nodes"`
	Ranges        []TokenRange                 `json:"ranges"`
	StartTime     time.Time                    `json:"start_time"`
	Status        PendingChangeStatus          `json:"status"`
	ErrorMessage  string                       `json:"error_message,omitempty"`
	LastUpdated   time.Time                    `json:"last_updated"`
	CompletedAt   time.Time                    `json:"completed_at,omitempty"` // Phase 7: For grace period tracking
	Progress      map[string]StreamingProgress `json:"progress"`
}

// IsCompleted checks if a pending change has completed
func (pc *PendingChange) IsCompleted() bool {
	return pc.Status == PendingChangeCompleted
}

// IsFailed checks if a pending change has failed
func (pc *PendingChange) IsFailed() bool {
	return pc.Status == PendingChangeFailed
}

// IsInProgress checks if a pending change is still ongoing
func (pc *PendingChange) IsInProgress() bool {
	return pc.Status == PendingChangeInProgress
}

// Hint represents a missed write that needs to be replayed
// When a write fails to a node, a hint is stored to replay it later
type Hint struct {
	HintID       string    `json:"hint_id"`
	TargetNodeID string    `json:"target_node_id"` // Node that missed the write
	TenantID     string    `json:"tenant_id"`
	Key          string    `json:"key"`
	Value        []byte    `json:"value"`
	VectorClock  string    `json:"vector_clock"` // For conflict resolution
	Timestamp    time.Time `json:"timestamp"`
	CreatedAt    time.Time `json:"created_at"`
	ReplayCount  int       `json:"replay_count"` // Number of replay attempts
}
