package store

import (
	"context"
	"errors"
	"time"

	"github.com/devrev/pairdb/coordinator/internal/model"
)

// ErrNotFound is returned when a key is not found
var ErrNotFound = errors.New("not found")

// MetadataStore interface for tenant and node metadata operations
type MetadataStore interface {
	// Tenant operations
	GetTenant(ctx context.Context, tenantID string) (*model.Tenant, error)
	CreateTenant(ctx context.Context, tenant *model.Tenant) error
	UpdateTenant(ctx context.Context, tenant *model.Tenant) error
	DeleteTenant(ctx context.Context, tenantID string) error

	// Storage node operations
	ListStorageNodes(ctx context.Context) ([]*model.StorageNode, error)
	AddStorageNode(ctx context.Context, node *model.StorageNode) error
	RemoveStorageNode(ctx context.Context, nodeID string) error
	UpdateStorageNodeStatus(ctx context.Context, nodeID string, status string) error

	// Migration operations
	GetMigration(ctx context.Context, migrationID string) (*model.Migration, error)
	CreateMigration(ctx context.Context, migration *model.Migration) error
	UpdateMigrationStatus(ctx context.Context, migrationID string, status model.MigrationStatus, errorMessage string) error

	// Health check
	Ping(ctx context.Context) error
	Close()
}

// IdempotencyStore interface for idempotency key operations
type IdempotencyStore interface {
	Get(ctx context.Context, key string) (interface{}, error)
	Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error
	Delete(ctx context.Context, key string) error
	Ping(ctx context.Context) error
	Close() error
}

// Cache interface for in-memory caching
type Cache interface {
	Get(ctx context.Context, key string) (interface{}, error)
	Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error
	Delete(ctx context.Context, key string) error
}
