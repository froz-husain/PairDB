package store

import (
	"context"
	"time"

	"github.com/devrev/pairdb/coordinator/internal/model"
)

// HintStore manages storage and retrieval of hints for failed writes
// Hints are used to replay writes that failed to reach a node during its downtime
type HintStore interface {
	// StoreHint stores a hint for a failed write to a node
	StoreHint(ctx context.Context, hint *model.Hint) error

	// GetHintsForNode retrieves all hints for a specific node
	// Returns hints ordered by timestamp (oldest first)
	GetHintsForNode(ctx context.Context, targetNodeID string, limit int) ([]*model.Hint, error)

	// DeleteHint deletes a specific hint after successful replay
	DeleteHint(ctx context.Context, hintID string) error

	// DeleteHintsForNode deletes all hints for a specific node (after successful replay)
	DeleteHintsForNode(ctx context.Context, targetNodeID string) error

	// ListHints lists all hints with optional filters
	ListHints(ctx context.Context, filter HintFilter) ([]*model.Hint, error)

	// CleanupOldHints deletes hints older than the specified TTL
	CleanupOldHints(ctx context.Context, ttl time.Duration) (int64, error)

	// GetHintCount returns the number of hints for a specific node
	GetHintCount(ctx context.Context, targetNodeID string) (int64, error)
}

// HintFilter represents filters for listing hints
type HintFilter struct {
	TargetNodeID string
	TenantID     string
	CreatedAfter time.Time
	Limit        int
	Offset       int
}
