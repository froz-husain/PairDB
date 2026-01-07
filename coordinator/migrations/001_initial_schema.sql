-- PairDB Coordinator Metadata Schema
-- Version: 001
-- Description: Initial schema for coordinator metadata

-- Tenants table
CREATE TABLE IF NOT EXISTS tenants (
    tenant_id VARCHAR(255) PRIMARY KEY,
    replication_factor INTEGER NOT NULL DEFAULT 3 CHECK (replication_factor >= 1 AND replication_factor <= 5),
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    version BIGINT NOT NULL DEFAULT 1
);

CREATE INDEX IF NOT EXISTS idx_tenants_updated_at ON tenants(updated_at);

-- Storage nodes table
CREATE TABLE IF NOT EXISTS storage_nodes (
    node_id VARCHAR(255) PRIMARY KEY,
    host VARCHAR(255) NOT NULL,
    port INTEGER NOT NULL CHECK (port > 0 AND port < 65536),
    status VARCHAR(50) NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'draining', 'inactive')),
    virtual_nodes INTEGER NOT NULL DEFAULT 150 CHECK (virtual_nodes > 0),
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_storage_nodes_status ON storage_nodes(status);
CREATE INDEX IF NOT EXISTS idx_storage_nodes_updated_at ON storage_nodes(updated_at);

-- Migrations table
CREATE TABLE IF NOT EXISTS migrations (
    migration_id VARCHAR(255) PRIMARY KEY,
    type VARCHAR(50) NOT NULL CHECK (type IN ('node_addition', 'node_deletion')),
    node_id VARCHAR(255) NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'in_progress', 'completed', 'failed', 'cancelled')),
    phase VARCHAR(50) CHECK (phase IN ('dual_write', 'data_copy', 'cutover', 'cleanup')),
    progress JSONB,
    started_at TIMESTAMP NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMP,
    error_message TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_migrations_status ON migrations(status);
CREATE INDEX IF NOT EXISTS idx_migrations_node_id ON migrations(node_id);
CREATE INDEX IF NOT EXISTS idx_migrations_type ON migrations(type);
CREATE INDEX IF NOT EXISTS idx_migrations_started_at ON migrations(started_at);

-- Migration checkpoints table
CREATE TABLE IF NOT EXISTS migration_checkpoints (
    migration_id VARCHAR(255) NOT NULL,
    key_range_start VARCHAR(255) NOT NULL,
    key_range_end VARCHAR(255) NOT NULL,
    last_migrated_key VARCHAR(255),
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    PRIMARY KEY (migration_id, key_range_start),
    FOREIGN KEY (migration_id) REFERENCES migrations(migration_id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_migration_checkpoints_migration_id ON migration_checkpoints(migration_id);

-- Insert default tenant for testing
INSERT INTO tenants (tenant_id, replication_factor, created_at, updated_at, version)
VALUES ('default', 3, NOW(), NOW(), 1)
ON CONFLICT (tenant_id) DO NOTHING;

-- Comments for documentation
COMMENT ON TABLE tenants IS 'Stores tenant configuration including replication factor';
COMMENT ON TABLE storage_nodes IS 'Stores storage node information and status';
COMMENT ON TABLE migrations IS 'Tracks node addition/deletion migrations';
COMMENT ON TABLE migration_checkpoints IS 'Stores checkpoints for migration progress tracking';

COMMENT ON COLUMN tenants.version IS 'Used for optimistic locking';
COMMENT ON COLUMN storage_nodes.virtual_nodes IS 'Number of virtual nodes in consistent hash ring';
COMMENT ON COLUMN migrations.progress IS 'JSON object with keys_migrated, total_keys, percentage';
