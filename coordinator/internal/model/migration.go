package model

import "time"

// Migration represents a data migration operation
type Migration struct {
	MigrationID  string
	Type         MigrationType
	NodeID       string
	Status       MigrationStatus
	Phase        MigrationPhase
	Progress     MigrationProgress
	StartedAt    time.Time
	CompletedAt  *time.Time
	ErrorMessage string
}

// MigrationType represents the type of migration
type MigrationType string

const (
	// MigrationTypeNodeAddition represents adding a new storage node
	MigrationTypeNodeAddition MigrationType = "node_addition"
	// MigrationTypeNodeDeletion represents removing a storage node
	MigrationTypeNodeDeletion MigrationType = "node_deletion"
)

// MigrationStatus represents the status of a migration
type MigrationStatus string

const (
	// MigrationStatusPending indicates migration is pending
	MigrationStatusPending MigrationStatus = "pending"
	// MigrationStatusInProgress indicates migration is in progress
	MigrationStatusInProgress MigrationStatus = "in_progress"
	// MigrationStatusCompleted indicates migration completed successfully
	MigrationStatusCompleted MigrationStatus = "completed"
	// MigrationStatusFailed indicates migration failed
	MigrationStatusFailed MigrationStatus = "failed"
	// MigrationStatusCancelled indicates migration was cancelled
	MigrationStatusCancelled MigrationStatus = "cancelled"
)

// MigrationPhase represents the phase of a migration
type MigrationPhase string

const (
	// MigrationPhaseDualWrite indicates dual-write phase
	MigrationPhaseDualWrite MigrationPhase = "dual_write"
	// MigrationPhaseDataCopy indicates data copy phase
	MigrationPhaseDataCopy MigrationPhase = "data_copy"
	// MigrationPhaseCutover indicates cutover phase
	MigrationPhaseCutover MigrationPhase = "cutover"
	// MigrationPhaseCleanup indicates cleanup phase
	MigrationPhaseCleanup MigrationPhase = "cleanup"
)

// MigrationProgress represents migration progress
type MigrationProgress struct {
	KeysMigrated int64
	TotalKeys    int64
	Percentage   float64
}

// RepairRequest represents a repair request for conflicting data
type RepairRequest struct {
	TenantID    string
	Key         string
	Value       []byte
	VectorClock VectorClock
	Timestamp   int64
}

// IdempotencyRecord represents an idempotency record
type IdempotencyRecord struct {
	TenantID       string
	Key            string
	IdempotencyKey string
	Response       []byte
	Timestamp      time.Time
	TTL            time.Duration
}
