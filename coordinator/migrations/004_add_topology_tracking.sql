-- Migration 004: Add Cassandra-correct topology tracking
-- Adds bidirectional range tracking for bootstrap/decommission operations

-- Add new columns to storage_nodes table
ALTER TABLE storage_nodes
ADD COLUMN IF NOT EXISTS state VARCHAR(20) DEFAULT 'NORMAL',
ADD COLUMN IF NOT EXISTS tokens BIGINT[],
ADD COLUMN IF NOT EXISTS pending_ranges JSONB,
ADD COLUMN IF NOT EXISTS leaving_ranges JSONB,
ADD COLUMN IF NOT EXISTS created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
ADD COLUMN IF NOT EXISTS updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP;

-- Create index on state for efficient filtering
CREATE INDEX IF NOT EXISTS idx_storage_nodes_state ON storage_nodes(state);

-- Create pending_changes table for tracking topology operations
CREATE TABLE IF NOT EXISTS pending_changes (
    change_id VARCHAR(64) PRIMARY KEY,
    type VARCHAR(20) NOT NULL CHECK (type IN ('bootstrap', 'decommission')),
    node_id VARCHAR(255) NOT NULL,
    affected_nodes TEXT[],
    ranges JSONB,
    start_time TIMESTAMP NOT NULL,
    status VARCHAR(20) NOT NULL CHECK (status IN ('in_progress', 'completed', 'failed', 'rolled_back')),
    error_message TEXT,
    last_updated TIMESTAMP NOT NULL,
    progress JSONB,

    CONSTRAINT fk_pending_changes_node FOREIGN KEY (node_id)
        REFERENCES storage_nodes(node_id) ON DELETE CASCADE
);

-- Create indexes for efficient queries
CREATE INDEX IF NOT EXISTS idx_pending_changes_status ON pending_changes(status);
CREATE INDEX IF NOT EXISTS idx_pending_changes_node ON pending_changes(node_id);
CREATE INDEX IF NOT EXISTS idx_pending_changes_start_time ON pending_changes(start_time);

-- Create trigger to update updated_at timestamp
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER update_storage_nodes_updated_at
    BEFORE UPDATE ON storage_nodes
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- Comments for documentation
COMMENT ON COLUMN storage_nodes.state IS 'Cassandra-style node state: NORMAL, BOOTSTRAPPING, LEAVING, DOWN';
COMMENT ON COLUMN storage_nodes.tokens IS 'Actual token positions (hash values) in the consistent hash ring';
COMMENT ON COLUMN storage_nodes.pending_ranges IS 'JSON array of PendingRangeInfo for ranges being received';
COMMENT ON COLUMN storage_nodes.leaving_ranges IS 'JSON array of LeavingRangeInfo for ranges being transferred out';
COMMENT ON TABLE pending_changes IS 'Tracks ongoing topology changes for coordinator failover and audit trail';
