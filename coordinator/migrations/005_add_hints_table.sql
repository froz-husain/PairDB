-- Migration: Add hints table for hinted handoff
-- Purpose: Store missed writes that need to be replayed when nodes recover

CREATE TABLE IF NOT EXISTS hints (
    hint_id VARCHAR(255) PRIMARY KEY,
    target_node_id VARCHAR(255) NOT NULL,
    tenant_id VARCHAR(255) NOT NULL,
    key VARCHAR(1024) NOT NULL,
    value BYTEA NOT NULL,
    vector_clock TEXT,
    timestamp TIMESTAMP NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    replay_count INTEGER NOT NULL DEFAULT 0
);

-- Index for efficient retrieval by target node
CREATE INDEX idx_hints_target_node ON hints(target_node_id, created_at);

-- Index for efficient cleanup of old hints
CREATE INDEX idx_hints_created_at ON hints(created_at);

-- Index for tenant-based queries
CREATE INDEX idx_hints_tenant ON hints(tenant_id);

-- Comments
COMMENT ON TABLE hints IS 'Stores missed writes (hints) that need to be replayed when nodes recover';
COMMENT ON COLUMN hints.target_node_id IS 'Node that missed the write';
COMMENT ON COLUMN hints.vector_clock IS 'Vector clock for conflict resolution';
COMMENT ON COLUMN hints.replay_count IS 'Number of times this hint has been replayed';
