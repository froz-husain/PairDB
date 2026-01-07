package store

import (
	"context"
	"fmt"

	"github.com/devrev/pairdb/coordinator/internal/model"
	"github.com/jackc/pgx/v5/pgxpool"
)

// MetadataStore handles PostgreSQL metadata operations
type MetadataStore struct {
	pool *pgxpool.Pool
}

// NewMetadataStore creates a new metadata store
func NewMetadataStore(connString string) (*MetadataStore, error) {
	pool, err := pgxpool.New(context.Background(), connString)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection pool: %w", err)
	}

	return &MetadataStore{pool: pool}, nil
}

// GetTenant retrieves tenant configuration
func (s *MetadataStore) GetTenant(ctx context.Context, tenantID string) (*model.Tenant, error) {
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
func (s *MetadataStore) CreateTenant(ctx context.Context, tenant *model.Tenant) error {
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
func (s *MetadataStore) UpdateTenant(ctx context.Context, tenant *model.Tenant) error {
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

// GetStorageNodes retrieves all storage nodes
func (s *MetadataStore) GetStorageNodes(ctx context.Context) ([]*model.StorageNode, error) {
	query := `
		SELECT node_id, host, port, status, virtual_nodes
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
		var status string
		if err := rows.Scan(&node.NodeID, &node.Host, &node.Port, &status, &node.VirtualNodes); err != nil {
			return nil, err
		}
		node.Status = model.NodeStatus(status)
		nodes = append(nodes, &node)
	}

	return nodes, rows.Err()
}

// CreateStorageNode creates a new storage node
func (s *MetadataStore) CreateStorageNode(ctx context.Context, node *model.StorageNode) error {
	query := `
		INSERT INTO storage_nodes (node_id, host, port, status, virtual_nodes, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, NOW(), NOW())
	`

	_, err := s.pool.Exec(ctx, query,
		node.NodeID,
		node.Host,
		node.Port,
		string(node.Status),
		node.VirtualNodes,
	)

	return err
}

// UpdateStorageNodeStatus updates the status of a storage node
func (s *MetadataStore) UpdateStorageNodeStatus(ctx context.Context, nodeID string, status model.NodeStatus) error {
	query := `
		UPDATE storage_nodes
		SET status = $2, updated_at = NOW()
		WHERE node_id = $1
	`

	result, err := s.pool.Exec(ctx, query, nodeID, string(status))
	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("storage node not found")
	}

	return nil
}

// DeleteStorageNode deletes a storage node
func (s *MetadataStore) DeleteStorageNode(ctx context.Context, nodeID string) error {
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

// CreateMigration creates a new migration record
func (s *MetadataStore) CreateMigration(ctx context.Context, migration *model.Migration) error {
	query := `
		INSERT INTO migrations (migration_id, type, node_id, status, phase, progress, started_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`

	progressJSON := fmt.Sprintf(`{"keys_migrated": %d, "total_keys": %d, "percentage": %f}`,
		migration.Progress.KeysMigrated,
		migration.Progress.TotalKeys,
		migration.Progress.Percentage)

	_, err := s.pool.Exec(ctx, query,
		migration.MigrationID,
		string(migration.Type),
		migration.NodeID,
		string(migration.Status),
		string(migration.Phase),
		progressJSON,
		migration.StartedAt,
	)

	return err
}

// GetMigration retrieves a migration by ID
func (s *MetadataStore) GetMigration(ctx context.Context, migrationID string) (*model.Migration, error) {
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

// UpdateMigrationStatus updates migration status
func (s *MetadataStore) UpdateMigrationStatus(ctx context.Context, migrationID string, status model.MigrationStatus, errorMessage string) error {
	query := `
		UPDATE migrations
		SET status = $2, error_message = $3, updated_at = NOW()
		WHERE migration_id = $1
	`

	_, err := s.pool.Exec(ctx, query, migrationID, string(status), errorMessage)
	return err
}

// Close closes the connection pool
func (s *MetadataStore) Close() {
	s.pool.Close()
}

// Ping checks the database connection
func (s *MetadataStore) Ping(ctx context.Context) error {
	return s.pool.Ping(ctx)
}
