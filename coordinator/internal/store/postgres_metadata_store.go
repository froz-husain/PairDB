package store

import (
	"context"
	"fmt"

	"github.com/devrev/pairdb/coordinator/internal/model"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

// PostgresMetadataStore implements MetadataStore for PostgreSQL
type PostgresMetadataStore struct {
	pool   *pgxpool.Pool
	logger *zap.Logger
}

// GetPool returns the underlying pgxpool.Pool
// This is used by other stores (e.g., HintStore) that share the same database
func (s *PostgresMetadataStore) GetPool() *pgxpool.Pool {
	return s.pool
}

// NewPostgresMetadataStore creates a new PostgreSQL metadata store
func NewPostgresMetadataStore(
	host string,
	port int,
	database, user, password string,
	maxConns, minConns int,
	logger *zap.Logger,
) (MetadataStore, error) {
	connString := fmt.Sprintf(
		"host=%s port=%d dbname=%s user=%s password=%s pool_max_conns=%d pool_min_conns=%d",
		host, port, database, user, password, maxConns, minConns,
	)

	config, err := pgxpool.ParseConfig(connString)
	if err != nil {
		return nil, fmt.Errorf("failed to parse connection string: %w", err)
	}

	pool, err := pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection pool: %w", err)
	}

	// Test connection
	if err := pool.Ping(context.Background()); err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return &PostgresMetadataStore{
		pool:   pool,
		logger: logger,
	}, nil
}

// GetTenant retrieves tenant configuration
func (s *PostgresMetadataStore) GetTenant(ctx context.Context, tenantID string) (*model.Tenant, error) {
	query := `
		SELECT tenant_id, replication_factor, created_at, updated_at, version
		FROM tenants
		WHERE tenant_id = $1
	`

	var tenant model.Tenant
	err := s.pool.QueryRow(ctx, query, tenantID).Scan(
		&tenant.TenantID,
		&tenant.ReplicationFactor,
		&tenant.CreatedAt,
		&tenant.UpdatedAt,
		&tenant.Version,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to get tenant: %w", err)
	}

	return &tenant, nil
}

// CreateTenant creates a new tenant
func (s *PostgresMetadataStore) CreateTenant(ctx context.Context, tenant *model.Tenant) error {
	query := `
		INSERT INTO tenants (tenant_id, replication_factor, created_at, updated_at, version)
		VALUES ($1, $2, $3, $4, $5)
	`

	_, err := s.pool.Exec(ctx, query,
		tenant.TenantID,
		tenant.ReplicationFactor,
		tenant.CreatedAt,
		tenant.UpdatedAt,
		tenant.Version,
	)

	return err
}

// UpdateTenant updates tenant configuration
func (s *PostgresMetadataStore) UpdateTenant(ctx context.Context, tenant *model.Tenant) error {
	query := `
		UPDATE tenants
		SET replication_factor = $2, updated_at = $3, version = $4
		WHERE tenant_id = $1 AND version = $5
	`

	result, err := s.pool.Exec(ctx, query,
		tenant.TenantID,
		tenant.ReplicationFactor,
		tenant.UpdatedAt,
		tenant.Version,
		tenant.Version-1, // Optimistic locking
	)

	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("tenant not found or version mismatch")
	}

	return nil
}

// DeleteTenant deletes a tenant
func (s *PostgresMetadataStore) DeleteTenant(ctx context.Context, tenantID string) error {
	query := `DELETE FROM tenants WHERE tenant_id = $1`
	result, err := s.pool.Exec(ctx, query, tenantID)
	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("tenant not found")
	}

	return nil
}

// ListStorageNodes retrieves all storage nodes
func (s *PostgresMetadataStore) ListStorageNodes(ctx context.Context) ([]*model.StorageNode, error) {
	query := `
		SELECT node_id, host, port, status, state, virtual_nodes
		FROM storage_nodes
		WHERE status != 'inactive'
		ORDER BY node_id
	`

	rows, err := s.pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	nodes := make([]*model.StorageNode, 0)
	for rows.Next() {
		var node model.StorageNode
		var stateStr string
		if err := rows.Scan(&node.NodeID, &node.Host, &node.Port, &node.Status, &stateStr, &node.VirtualNodes); err != nil {
			return nil, err
		}
		node.State = model.NodeState(stateStr)
		nodes = append(nodes, &node)
	}

	return nodes, rows.Err()
}

// AddStorageNode adds a new storage node
func (s *PostgresMetadataStore) AddStorageNode(ctx context.Context, node *model.StorageNode) error {
	query := `
		INSERT INTO storage_nodes (node_id, host, port, status, state, virtual_nodes, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, NOW(), NOW())
	`

	_, err := s.pool.Exec(ctx, query,
		node.NodeID,
		node.Host,
		node.Port,
		node.Status,
		node.State,
		node.VirtualNodes,
	)

	return err
}

// RemoveStorageNode removes a storage node
func (s *PostgresMetadataStore) RemoveStorageNode(ctx context.Context, nodeID string) error {
	query := `DELETE FROM storage_nodes WHERE node_id = $1`
	result, err := s.pool.Exec(ctx, query, nodeID)
	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("storage node not found")
	}

	return nil
}

// UpdateStorageNodeStatus updates the status of a storage node
func (s *PostgresMetadataStore) UpdateStorageNodeStatus(ctx context.Context, nodeID string, status string) error {
	query := `
		UPDATE storage_nodes
		SET status = $2, updated_at = NOW()
		WHERE node_id = $1
	`

	result, err := s.pool.Exec(ctx, query, nodeID, status)
	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("storage node not found")
	}

	return nil
}

// GetMigration retrieves a migration by ID
func (s *PostgresMetadataStore) GetMigration(ctx context.Context, migrationID string) (*model.Migration, error) {
	query := `
		SELECT migration_id, type, node_id, status, phase, started_at, completed_at, error_message
		FROM migrations
		WHERE migration_id = $1
	`

	var migration model.Migration
	var migrationType, status, phase string
	err := s.pool.QueryRow(ctx, query, migrationID).Scan(
		&migration.MigrationID,
		&migrationType,
		&migration.NodeID,
		&status,
		&phase,
		&migration.StartedAt,
		&migration.CompletedAt,
		&migration.ErrorMessage,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to get migration: %w", err)
	}

	migration.Type = model.MigrationType(migrationType)
	migration.Status = model.MigrationStatus(status)
	migration.Phase = model.MigrationPhase(phase)

	return &migration, nil
}

// CreateMigration creates a new migration record
func (s *PostgresMetadataStore) CreateMigration(ctx context.Context, migration *model.Migration) error {
	query := `
		INSERT INTO migrations (migration_id, type, node_id, status, phase, started_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`

	_, err := s.pool.Exec(ctx, query,
		migration.MigrationID,
		migration.Type,
		migration.NodeID,
		migration.Status,
		"",
		migration.StartedAt,
	)

	return err
}

// UpdateMigrationStatus updates migration status
func (s *PostgresMetadataStore) UpdateMigrationStatus(ctx context.Context, migrationID string, status model.MigrationStatus, errorMessage string) error {
	query := `
		UPDATE migrations
		SET status = $2, error_message = $3, updated_at = NOW()
		WHERE migration_id = $1
	`

	_, err := s.pool.Exec(ctx, query, migrationID, string(status), errorMessage)
	return err
}

// UpdateNodeState updates the state of a storage node
func (s *PostgresMetadataStore) UpdateNodeState(ctx context.Context, nodeID string, state model.NodeState) error {
	query := `
		UPDATE storage_nodes
		SET state = $2, updated_at = NOW()
		WHERE node_id = $1
	`

	result, err := s.pool.Exec(ctx, query, nodeID, string(state))
	if err != nil {
		return fmt.Errorf("failed to update node state: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("storage node not found")
	}

	return nil
}

// UpdateNodeRanges updates the pending and leaving ranges for a storage node
func (s *PostgresMetadataStore) UpdateNodeRanges(ctx context.Context, nodeID string, pendingRanges []model.PendingRangeInfo, leavingRanges []model.LeavingRangeInfo) error {
	query := `
		UPDATE storage_nodes
		SET pending_ranges = $2, leaving_ranges = $3, updated_at = NOW()
		WHERE node_id = $1
	`

	result, err := s.pool.Exec(ctx, query, nodeID, pendingRanges, leavingRanges)
	if err != nil {
		return fmt.Errorf("failed to update node ranges: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("storage node not found")
	}

	return nil
}

// GetStorageNode retrieves a single storage node by ID
func (s *PostgresMetadataStore) GetStorageNode(ctx context.Context, nodeID string) (*model.StorageNode, error) {
	query := `
		SELECT node_id, host, port, status, state, virtual_nodes, tokens,
		       pending_ranges, leaving_ranges, created_at, updated_at
		FROM storage_nodes
		WHERE node_id = $1
	`

	var node model.StorageNode
	var stateStr string
	err := s.pool.QueryRow(ctx, query, nodeID).Scan(
		&node.NodeID,
		&node.Host,
		&node.Port,
		&node.Status,
		&stateStr,
		&node.VirtualNodes,
		&node.Tokens,
		&node.PendingRanges,
		&node.LeavingRanges,
		&node.CreatedAt,
		&node.UpdatedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to get storage node: %w", err)
	}

	node.State = model.NodeState(stateStr)
	return &node, nil
}

// AddPendingChange creates a new pending topology change record
func (s *PostgresMetadataStore) AddPendingChange(ctx context.Context, change *model.PendingChange) error {
	query := `
		INSERT INTO pending_changes (
			change_id, type, node_id, affected_nodes, ranges,
			start_time, status, last_updated, progress
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`

	_, err := s.pool.Exec(ctx, query,
		change.ChangeID,
		change.Type,
		change.NodeID,
		change.AffectedNodes,
		change.Ranges,
		change.StartTime,
		string(change.Status),
		change.LastUpdated,
		change.Progress,
	)

	if err != nil {
		return fmt.Errorf("failed to add pending change: %w", err)
	}

	return nil
}

// GetPendingChange retrieves a pending change by ID
func (s *PostgresMetadataStore) GetPendingChange(ctx context.Context, changeID string) (*model.PendingChange, error) {
	query := `
		SELECT change_id, type, node_id, affected_nodes, ranges,
		       start_time, status, error_message, last_updated, progress
		FROM pending_changes
		WHERE change_id = $1
	`

	var change model.PendingChange
	var statusStr string
	err := s.pool.QueryRow(ctx, query, changeID).Scan(
		&change.ChangeID,
		&change.Type,
		&change.NodeID,
		&change.AffectedNodes,
		&change.Ranges,
		&change.StartTime,
		&statusStr,
		&change.ErrorMessage,
		&change.LastUpdated,
		&change.Progress,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to get pending change: %w", err)
	}

	change.Status = model.PendingChangeStatus(statusStr)
	return &change, nil
}

// UpdatePendingChange updates an existing pending change
func (s *PostgresMetadataStore) UpdatePendingChange(ctx context.Context, change *model.PendingChange) error {
	query := `
		UPDATE pending_changes
		SET status = $2, error_message = $3, last_updated = $4, progress = $5
		WHERE change_id = $1
	`

	result, err := s.pool.Exec(ctx, query,
		change.ChangeID,
		string(change.Status),
		change.ErrorMessage,
		change.LastUpdated,
		change.Progress,
	)

	if err != nil {
		return fmt.Errorf("failed to update pending change: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("pending change not found")
	}

	return nil
}

// ListPendingChanges retrieves all pending changes with optional status filter
func (s *PostgresMetadataStore) ListPendingChanges(ctx context.Context, status model.PendingChangeStatus) ([]*model.PendingChange, error) {
	var query string
	var args []interface{}

	if status != "" {
		query = `
			SELECT change_id, type, node_id, affected_nodes, ranges,
			       start_time, status, error_message, last_updated, progress
			FROM pending_changes
			WHERE status = $1
			ORDER BY start_time DESC
		`
		args = append(args, string(status))
	} else {
		query = `
			SELECT change_id, type, node_id, affected_nodes, ranges,
			       start_time, status, error_message, last_updated, progress
			FROM pending_changes
			ORDER BY start_time DESC
		`
	}

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list pending changes: %w", err)
	}
	defer rows.Close()

	changes := make([]*model.PendingChange, 0)
	for rows.Next() {
		var change model.PendingChange
		var statusStr string
		if err := rows.Scan(
			&change.ChangeID,
			&change.Type,
			&change.NodeID,
			&change.AffectedNodes,
			&change.Ranges,
			&change.StartTime,
			&statusStr,
			&change.ErrorMessage,
			&change.LastUpdated,
			&change.Progress,
		); err != nil {
			return nil, fmt.Errorf("failed to scan pending change: %w", err)
		}
		change.Status = model.PendingChangeStatus(statusStr)
		changes = append(changes, &change)
	}

	return changes, rows.Err()
}

// DeletePendingChange deletes a pending change record
func (s *PostgresMetadataStore) DeletePendingChange(ctx context.Context, changeID string) error {
	query := `DELETE FROM pending_changes WHERE change_id = $1`
	result, err := s.pool.Exec(ctx, query, changeID)
	if err != nil {
		return fmt.Errorf("failed to delete pending change: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("pending change not found")
	}

	return nil
}

// Ping checks the database connection
func (s *PostgresMetadataStore) Ping(ctx context.Context) error {
	return s.pool.Ping(ctx)
}

// Close closes the connection pool
func (s *PostgresMetadataStore) Close() {
	s.pool.Close()
}
