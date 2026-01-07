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
		if err := rows.Scan(&node.NodeID, &node.Host, &node.Port, &node.Status, &node.VirtualNodes); err != nil {
			return nil, err
		}
		nodes = append(nodes, &node)
	}

	return nodes, rows.Err()
}

// AddStorageNode adds a new storage node
func (s *PostgresMetadataStore) AddStorageNode(ctx context.Context, node *model.StorageNode) error {
	query := `
		INSERT INTO storage_nodes (node_id, host, port, status, virtual_nodes, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, NOW(), NOW())
	`

	_, err := s.pool.Exec(ctx, query,
		node.NodeID,
		node.Host,
		node.Port,
		node.Status,
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

// Ping checks the database connection
func (s *PostgresMetadataStore) Ping(ctx context.Context) error {
	return s.pool.Ping(ctx)
}

// Close closes the connection pool
func (s *PostgresMetadataStore) Close() {
	s.pool.Close()
}
