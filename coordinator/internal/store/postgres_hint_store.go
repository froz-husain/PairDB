package store

import (
	"context"
	"fmt"
	"time"

	"github.com/devrev/pairdb/coordinator/internal/model"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresHintStore implements HintStore using PostgreSQL
type PostgresHintStore struct {
	pool *pgxpool.Pool
}

// NewPostgresHintStore creates a new PostgreSQL hint store
func NewPostgresHintStore(pool *pgxpool.Pool) *PostgresHintStore {
	return &PostgresHintStore{
		pool: pool,
	}
}

// StoreHint stores a hint for a failed write
func (s *PostgresHintStore) StoreHint(ctx context.Context, hint *model.Hint) error {
	query := `
		INSERT INTO hints (
			hint_id, target_node_id, tenant_id, key, value,
			vector_clock, timestamp, created_at, replay_count
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`

	_, err := s.pool.Exec(ctx, query,
		hint.HintID,
		hint.TargetNodeID,
		hint.TenantID,
		hint.Key,
		hint.Value,
		hint.VectorClock,
		hint.Timestamp,
		hint.CreatedAt,
		hint.ReplayCount,
	)

	if err != nil {
		return fmt.Errorf("failed to store hint: %w", err)
	}

	return nil
}

// GetHintsForNode retrieves all hints for a specific node
func (s *PostgresHintStore) GetHintsForNode(ctx context.Context, targetNodeID string, limit int) ([]*model.Hint, error) {
	query := `
		SELECT hint_id, target_node_id, tenant_id, key, value,
		       vector_clock, timestamp, created_at, replay_count
		FROM hints
		WHERE target_node_id = $1
		ORDER BY created_at ASC
		LIMIT $2
	`

	rows, err := s.pool.Query(ctx, query, targetNodeID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get hints: %w", err)
	}
	defer rows.Close()

	hints := make([]*model.Hint, 0)
	for rows.Next() {
		var hint model.Hint
		if err := rows.Scan(
			&hint.HintID,
			&hint.TargetNodeID,
			&hint.TenantID,
			&hint.Key,
			&hint.Value,
			&hint.VectorClock,
			&hint.Timestamp,
			&hint.CreatedAt,
			&hint.ReplayCount,
		); err != nil {
			return nil, fmt.Errorf("failed to scan hint: %w", err)
		}
		hints = append(hints, &hint)
	}

	return hints, rows.Err()
}

// DeleteHint deletes a specific hint
func (s *PostgresHintStore) DeleteHint(ctx context.Context, hintID string) error {
	query := `DELETE FROM hints WHERE hint_id = $1`

	result, err := s.pool.Exec(ctx, query, hintID)
	if err != nil {
		return fmt.Errorf("failed to delete hint: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("hint not found")
	}

	return nil
}

// DeleteHintsForNode deletes all hints for a specific node
func (s *PostgresHintStore) DeleteHintsForNode(ctx context.Context, targetNodeID string) error {
	query := `DELETE FROM hints WHERE target_node_id = $1`

	_, err := s.pool.Exec(ctx, query, targetNodeID)
	if err != nil {
		return fmt.Errorf("failed to delete hints for node: %w", err)
	}

	return nil
}

// ListHints lists all hints with optional filters
func (s *PostgresHintStore) ListHints(ctx context.Context, filter HintFilter) ([]*model.Hint, error) {
	query := `
		SELECT hint_id, target_node_id, tenant_id, key, value,
		       vector_clock, timestamp, created_at, replay_count
		FROM hints
		WHERE 1=1
	`
	args := make([]interface{}, 0)
	argPos := 1

	if filter.TargetNodeID != "" {
		query += fmt.Sprintf(" AND target_node_id = $%d", argPos)
		args = append(args, filter.TargetNodeID)
		argPos++
	}

	if filter.TenantID != "" {
		query += fmt.Sprintf(" AND tenant_id = $%d", argPos)
		args = append(args, filter.TenantID)
		argPos++
	}

	if !filter.CreatedAfter.IsZero() {
		query += fmt.Sprintf(" AND created_at > $%d", argPos)
		args = append(args, filter.CreatedAfter)
		argPos++
	}

	query += " ORDER BY created_at ASC"

	if filter.Limit > 0 {
		query += fmt.Sprintf(" LIMIT $%d", argPos)
		args = append(args, filter.Limit)
		argPos++
	}

	if filter.Offset > 0 {
		query += fmt.Sprintf(" OFFSET $%d", argPos)
		args = append(args, filter.Offset)
		argPos++
	}

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list hints: %w", err)
	}
	defer rows.Close()

	hints := make([]*model.Hint, 0)
	for rows.Next() {
		var hint model.Hint
		if err := rows.Scan(
			&hint.HintID,
			&hint.TargetNodeID,
			&hint.TenantID,
			&hint.Key,
			&hint.Value,
			&hint.VectorClock,
			&hint.Timestamp,
			&hint.CreatedAt,
			&hint.ReplayCount,
		); err != nil {
			return nil, fmt.Errorf("failed to scan hint: %w", err)
		}
		hints = append(hints, &hint)
	}

	return hints, rows.Err()
}

// CleanupOldHints deletes hints older than the specified TTL
func (s *PostgresHintStore) CleanupOldHints(ctx context.Context, ttl time.Duration) (int64, error) {
	query := `DELETE FROM hints WHERE created_at < $1`

	cutoffTime := time.Now().Add(-ttl)
	result, err := s.pool.Exec(ctx, query, cutoffTime)
	if err != nil {
		return 0, fmt.Errorf("failed to cleanup old hints: %w", err)
	}

	return result.RowsAffected(), nil
}

// GetHintCount returns the number of hints for a specific node
func (s *PostgresHintStore) GetHintCount(ctx context.Context, targetNodeID string) (int64, error) {
	query := `SELECT COUNT(*) FROM hints WHERE target_node_id = $1`

	var count int64
	err := s.pool.QueryRow(ctx, query, targetNodeID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to get hint count: %w", err)
	}

	return count, nil
}
