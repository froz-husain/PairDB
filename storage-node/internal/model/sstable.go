package model

import "time"

// SSTableMetadata contains metadata about an SSTable
type SSTableMetadata struct {
	SSTableID string
	TenantID  string
	Level     int
	Size      int64
	KeyRange  KeyRange
	CreatedAt time.Time
	FilePath  string
	IndexPath string
	BloomPath string
}

// KeyRange defines the range of keys in an SSTable
type KeyRange struct {
	StartKey string
	EndKey   string
}

// SSTableLevel represents the compaction level
type SSTableLevel int

const (
	L0 SSTableLevel = 0
	L1 SSTableLevel = 1
	L2 SSTableLevel = 2
	L3 SSTableLevel = 3
	L4 SSTableLevel = 4
)

// CompactionJob represents a compaction task
type CompactionJob struct {
	JobID       string
	Level       SSTableLevel
	InputTables []*SSTableMetadata
	OutputLevel SSTableLevel
	StartedAt   time.Time
	Status      CompactionStatus
}

// CompactionStatus indicates the state of a compaction job
type CompactionStatus string

const (
	CompactionStatusPending   CompactionStatus = "pending"
	CompactionStatusRunning   CompactionStatus = "running"
	CompactionStatusCompleted CompactionStatus = "completed"
	CompactionStatusFailed    CompactionStatus = "failed"
)
