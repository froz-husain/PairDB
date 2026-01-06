# Storage Node: High-Level Design

## 1. Overview

The Storage Node is a stateful service that stores and serves key-value data. It implements a multi-layer storage architecture with commit log, in-memory cache, memtable, and SSTables for optimal read/write performance.

## 2. Requirements Summary

### 2.1 Functional Requirements
- Store tenant-specific key-value pairs with vector clocks
- Handle read/write operations with tenant isolation
- Maintain data in multiple storage layers (commit log, cache, memtable, SSTables)
- Write all mutations to commit log first (for durability)
- Participate in replication and consistency protocols
- Support data migration during replication factor changes
- Handle repair operations from coordinators

### 2.2 Non-Functional Requirements
- High durability (commit log ensures data persistence)
- Low latency reads (cache and memtable for hot data)
- Efficient writes (commit log + memtable)
- Support decent storage scale (terabytes per node)
- Tenant isolation at all layers

## 3. Service Architecture

### 3.1 Service Type
- **Protocol**: gRPC service (server)
- **Deployment**: Stateful, one instance per physical/virtual node
- **Language**: Go, C++, or Rust (recommended: Go for development speed, C++/Rust for performance)
- **Network Communication**:
  - **Receives from Coordinators**: gRPC requests
  - **Gossip Protocol**: Peer-to-peer communication with other storage nodes for health checks
  - **Local**: File system for commit logs and SSTables

### 3.2 Component Responsibilities

#### 3.2.1 Request Handler
- Receive read/write/repair requests from coordinators
- Validate tenant_id and key
- Route to appropriate storage layer handler

#### 3.2.2 Commit Log Manager
- Append-only log for all write operations
- Sequential writes for optimal disk performance
- Periodic rotation and truncation
- Recovery on node restart

#### 3.2.3 Cache Manager
- In-memory adaptive cache for frequently accessed keys
- **Adaptive Eviction Policy**: Combines LRU and LFU based on access patterns
  - Hot keys (frequently accessed): Use LFU component
  - Recent keys: Use LRU component
  - Adaptive scoring: `score = frequency_weight * access_count + recency_weight * time_since_last_access`
  - Automatically adjusts weights based on workload
- Tenant-aware caching
- Cache warming strategies

#### 3.2.4 MemTable Manager
- In-memory sorted table for recent writes
- Write-optimized data structure
- Flush to SSTable when size threshold reached
- Tenant-isolated memtables

#### 3.2.5 SSTable Manager
- Immutable sorted string tables on disk
- Read-optimized data structure
- Multiple SSTables per node
- Compaction process to merge SSTables
- Tenant-aware organization

#### 3.2.6 Vector Clock Store
- Store vector clocks with each key-value pair
- Update vector clocks on writes
- Compare vector clocks for conflict detection
- Efficient serialization/deserialization

#### 3.2.7 Tenant Isolation Manager
- Enforce tenant isolation at all layers
- Validate tenant_id on all operations
- Separate data structures per tenant (logical separation)

## 4. Core Entities

### 4.1 Key-Value Entry
```go
type VectorClockEntry struct {
    CoordinatorNodeID string
    LogicalTimestamp   int64
}

type VectorClock []VectorClockEntry

type KeyValueEntry struct {
    TenantID    string
    Key         string
    Value       []byte
    VectorClock VectorClock  // List of {coordinator_node_id, logical_timestamp}
    Timestamp   int64
}
```

### 4.2 Commit Log Entry
```go
type CommitLogEntry struct {
    TenantID     string
    Key          string
    Value        []byte
    VectorClock  VectorClock
    Timestamp    int64
    OperationType string  // "write", "repair", "delete"
}
```

### 4.3 MemTable Entry
```go
type MemTableEntry struct {
    Key         string  // Format: "{tenant_id}:{key}"
    Value       []byte
    VectorClock VectorClock
    Timestamp   int64
}
```

### 4.4 SSTable Metadata
```go
type SSTableMetadata struct {
    SSTableID   string
    TenantID    string
    Level       int      // L0, L1, L2, etc.
    Size        int64    // Size in bytes
    KeyRange    KeyRange
    CreatedAt   time.Time
    FilePath    string
}

type KeyRange struct {
    StartKey string
    EndKey   string
}
```

### 4.5 Cache Entry
```go
type CacheEntry struct {
    Key         string  // Format: "{tenant_id}:{key}"
    Value       []byte
    VectorClock VectorClock
    AccessCount int64      // For LFU component
    LastAccess  time.Time   // For LRU component
    Score       float64     // Adaptive score for eviction
}
```

### 4.6 Health Status (for Gossip Protocol)
```go
type HealthStatus struct {
    NodeID    string
    Status    string  // "healthy", "degraded", "unhealthy"
    Timestamp int64
    Metrics   map[string]float64  // cpu_usage, memory_usage, disk_usage, etc.
}
```

## 5. APIs Offered by Storage Node

**Note**: Detailed API contracts are documented in `api-contracts.md` in this directory.

### 5.1 gRPC Service APIs

The Storage Node exposes gRPC service APIs for communication with Coordinators. See `api-contracts.md` for complete protobuf definitions.

**Key gRPC Methods**:
- `Write(WriteRequest) returns (WriteResponse)` - Write key-value pair
- `Read(ReadRequest) returns (ReadResponse)` - Read key-value pair
- `Repair(RepairRequest) returns (RepairResponse)` - Repair conflicting data
- `ReplicateData(ReplicateRequest) returns (ReplicateResponse)` - Data replication for migration
- `StreamKeys(StreamKeysRequest) returns (stream KeyValueEntry)` - Stream keys for migration
- `GetKeyRange(KeyRangeRequest) returns (KeyRangeResponse)` - Get keys in range
- `DrainNode(DrainRequest) returns (DrainResponse)` - Prepare node for removal
- `HealthCheck() returns (HealthResponse)` - Health check endpoint


## 6. Storage Architecture

### 5.1 Multi-Layer Storage

```
Write Request
    ↓
Commit Log (Disk) ← Durability, Recovery
    ↓
MemTable (Memory) ← Recent Writes
    ↓
Cache (Memory) ← Frequently Accessed
    ↓
SSTables (Disk) ← Persistent Storage
```

### 5.2 Commit Log
- **Purpose**: Durability and crash recovery
- **Format**: Append-only log file
- **Location**: Disk (separate directory)
- **Entry Format**: `tenant_id|key|value|vector_clock|timestamp`
- **Rotation Policy**:
  - **Size-based**: Rotate when commit log reaches default size (e.g., 1GB)
  - **Time-based**: Rotate commit logs older than a few days (e.g., 3-7 days)
  - **Event-based**: Truncate after memtable flushes to SSTable (data persisted)
- **Recovery**: Replay commit log on node restart
- **Archival**: Old commit logs can be archived or deleted after verification

### 5.3 In-Memory Cache
- **Purpose**: Fast lookup for frequently accessed keys
- **Type**: Adaptive eviction policy (combines LRU and LFU)
- **Key Format**: `{tenant_id}:{key}`
- **Size**: Configurable (e.g., 1GB per node)
- **Adaptive Eviction Policy**:
  - **Hot Keys**: Frequently accessed keys use LFU (Least Frequently Used) - prioritize by access frequency
  - **Recent Keys**: Recently accessed keys use LRU (Least Recently Used) - prioritize by recency
  - **Adaptive Algorithm**: 
    - Monitor access patterns for each key
    - Calculate score: `score = frequency_weight * access_count + recency_weight * time_since_last_access`
    - Evict keys with lowest scores when cache is full
    - Automatically adjust weights based on workload patterns
  - **Benefits**: 
    - Better hit rates for mixed workloads
    - Adapts to changing access patterns
    - Balances between frequency and recency
- **Eviction**: When cache is full, evict keys with lowest adaptive scores

### 5.4 MemTable
- **Purpose**: Write-optimized in-memory storage
- **Data Structure**: Sorted map (e.g., skip list, B-tree)
- **Key Format**: `{tenant_id}:{key}`
- **Flush Trigger**: When size exceeds threshold (e.g., 64MB)
- **Flush Target**: New SSTable on disk

### 5.5 SSTables

- **Purpose**: Persistent, read-optimized storage
- **Format**: Immutable sorted string tables
- **Location**: Disk (separate directory)
- **Organization**: Multiple SSTables per node, per tenant
- **Compaction Strategy**: Hybrid approach combining size-tiered and level-based compaction

#### 5.5.1 Level-Based Compaction Structure

SSTables are organized into levels with increasing size thresholds:

- **L0 (Level 0)**: 
  - Size: 64MB per SSTable
  - Source: MemTable flushes
  - Multiple small SSTables allowed
  - Fast writes, may have overlapping key ranges
  
- **L1 (Level 1)**:
  - Size: 128MB per SSTable
  - Source: Compaction of L0 SSTables
  - No overlapping key ranges within level
  - Better read performance
  
- **L2 (Level 2)**:
  - Size: 256MB per SSTable
  - Source: Compaction of L1 SSTables
  
- **L3+ (Higher Levels)**:
  - Size: Doubles at each level (512MB, 1GB, 2GB, etc.)
  - Source: Compaction from previous level
  - Larger SSTables for older data

#### 5.5.2 Compaction Process

**Level 0 Compaction**:
- When L0 has too many SSTables (e.g., > 4), trigger compaction
- Merge L0 SSTables into L1 SSTables
- Remove duplicates, keep latest version

**Level N Compaction**:
- When level N exceeds size threshold, compact to level N+1
- Merge SSTables from level N into level N+1
- Maintain sorted order, remove duplicates

**Compaction Benefits**:
- Reduces read amplification (fewer SSTables to check)
- Reduces disk space usage (removes duplicates and old versions)
- Maintains sorted structure for efficient reads
- Background process, doesn't block reads/writes

## 7. Data Structures

### 7.1 Key-Value Entry
```go
type VectorClockEntry struct {
    CoordinatorNodeID string
    LogicalTimestamp  int64
}

type VectorClock []VectorClockEntry

type KeyValueEntry struct {
    TenantID    string
    Key         string
    Value       []byte
    VectorClock VectorClock  // List of {coordinator_node_id, logical_timestamp}
    Timestamp   int64
}
```

### 7.2 Vector Clock

- **Format**: List of `{coordinator_node_id, logical_timestamp}` pairs
- **Data Structure**:
```go
type VectorClockEntry struct {
    CoordinatorNodeID string
    LogicalTimestamp  int64
}

type VectorClock []VectorClockEntry
```

- **Example**:
```go
vectorClock := VectorClock{
    {CoordinatorNodeID: "coord-1", LogicalTimestamp: 5},
    {CoordinatorNodeID: "coord-2", LogicalTimestamp: 3},
    {CoordinatorNodeID: "coord-3", LogicalTimestamp: 2},
}
```

- **Storage**: Serialized as JSON or binary format with each key-value pair
- **Comparison**: Compare entries by coordinator_node_id, then by logical_timestamp

## 8. Key Operations

### 8.1 Write Operation
1. **Validate**: Check tenant_id and key format
2. **Commit Log**: Append write to commit log (sync to disk)
3. **MemTable**: Insert/update in memtable
4. **Cache**: Update cache if key exists
5. **Vector Clock**: Update vector clock
6. **Response**: Return success with updated vector clock

### 8.2 Read Operation
1. **Validate**: Check tenant_id and key format
2. **Cache**: Check cache (if hit, return)
3. **MemTable**: Check memtable (if found, return and cache)
4. **SSTables**: Search SSTables (check multiple, return latest)
5. **Version Comparison**: Compare versions, handle siblings
6. **Response**: Return value and vector clock

### 8.3 MemTable Flush
1. **Trigger**: When memtable size exceeds threshold
2. **Create SSTable**: Write memtable contents to new SSTable
3. **Sort**: SSTable is already sorted (memtable is sorted)
4. **Commit Log**: Truncate commit log (data now in SSTable)
5. **Clear MemTable**: Create new empty memtable

### 8.4 SSTable Compaction

**Level-Based Compaction Process**:

1. **L0 Compaction Trigger**: 
   - When L0 has > 4 SSTables (or total size > threshold)
   - Trigger compaction to L1

2. **Level N Compaction Trigger**:
   - When level N exceeds size threshold
   - Trigger compaction to level N+1

3. **Compaction Steps**:
   - Select SSTables from source level
   - Merge sorted SSTables (maintain sort order)
   - Remove duplicates (keep latest version based on vector clock)
   - Write merged data to target level SSTable
   - Update metadata and indexes
   - Cleanup: Delete old SSTables after verification

4. **Background Execution**:
   - Compaction runs in background threads
   - Doesn't block reads/writes
   - Throttled to avoid impacting normal operations
   - Prioritizes levels with most overlap or highest read amplification

### 8.5 Migration Operations

#### 8.5.1 Node Addition - Receiving Data

When a new node is added and data is being migrated to it:

1. **Receive Write Requests**: Node receives write requests from coordinator
2. **Normal Write Flow**: Process writes through commit log → memtable → cache
3. **Bulk Data Reception**: For bulk migration, coordinator streams data
4. **Verify Data**: After migration, node verifies data integrity

#### 8.5.2 Node Deletion - Sending Data

When a node is being removed:

1. **Drain Request**: Coordinator calls DrainNode API
2. **Stop New Writes**: Node stops accepting new write requests
3. **Complete Ongoing Operations**: Finish processing current requests
4. **Stream Data**: Stream keys to coordinator for replication
5. **Verify Replication**: Ensure all data has been copied to other nodes
6. **Mark as Drained**: Return "drained" status to coordinator

#### 8.5.3 Data Streaming for Migration

**Efficient Streaming**:
- Read from SSTables in sorted order
- Batch multiple keys per stream message
- Include vector clocks with each key-value pair
- Handle large datasets without memory issues

**Example Streaming Implementation**:
```go
func StreamKeys(tenantID string, keyRangeStart string, keyRangeEnd string) {
    // Open SSTables for tenant
    sstables := getSSTablesForTenant(tenantID)
    
    // Iterate through keys in range
    for _, sstable := range sstables {
        iterator := sstable.NewIterator(keyRangeStart, keyRangeEnd)
        for iterator.Next() {
            key := iterator.Key()
            value := iterator.Value()
            vectorClock := iterator.VectorClock()
            
            // Stream to coordinator
            stream.Send(&KeyValueEntry{
                TenantID: tenantID,
                Key: key,
                Value: value,
                VectorClock: vectorClock,
            })
        }
    }
}
```

## 9. Tenant Isolation

### 9.1 Logical Separation
- All data structures include tenant_id
- Keys are prefixed with tenant_id: `{tenant_id}:{key}`
- Separate memtables per tenant (logical, not physical)
- SSTables organized by tenant (directory structure)

### 9.2 Validation
- Validate tenant_id on all operations
- Reject operations with invalid tenant_id
- Log cross-tenant access attempts

## 10. Performance Optimizations

### 10.1 Write Optimizations
- **Batch Writes**: Batch multiple writes before flushing to commit log
- **Async Flush**: Don't block on commit log sync (optional, trade-off durability)
- **MemTable Size**: Tune memtable size for write performance

### 10.2 Read Optimizations
- **Cache**: Aggressive caching of frequently accessed keys
- **SSTable Indexing**: Index SSTables for faster lookups
- **Bloom Filters**: Use bloom filters to avoid unnecessary SSTable reads

### 10.3 Compaction Optimizations
- **Background Compaction**: Don't block reads/writes
- **Priority Compaction**: Prioritize compaction of frequently accessed SSTables
- **Parallel Compaction**: Compact multiple SSTables in parallel

## 11. Error Handling

### 11.1 Disk Failures
- Detect disk failures
- Mark node as unhealthy
- Stop accepting writes (reads may continue from cache/memtable)
- Alert monitoring system

### 11.2 Memory Pressure
- Monitor memory usage
- Aggressive cache eviction if memory pressure
- Trigger memtable flush early if needed
- Reject new writes if memory exhausted

### 11.3 Corrupted Data
- Detect corrupted commit log entries
- Skip corrupted entries during recovery
- Log errors for manual intervention
- Use checksums for data integrity

## 12. Monitoring and Observability

### 12.1 Health Checks with Gossip Protocol

**Gossip Protocol Implementation**:
- Storage nodes use gossip protocol for peer-to-peer health monitoring
- **Gossip Mechanism**:
  - Each node periodically (e.g., every 5 seconds) selects random peers
  - Exchanges health status with selected peers
  - Health status includes: node_id, status (healthy/degraded/unhealthy), timestamp, metrics
  - Health information propagates through gossip network
- **Failure Detection**:
  - Fast detection of node failures (typically within 15-30 seconds)
  - No single point of failure (decentralized)
  - Self-organizing network
- **Health Status Propagation**:
  - Coordinator nodes subscribe to gossip events
  - Coordinators update routing tables based on health status
  - Unhealthy nodes are removed from routing automatically

**Health Status Format**:
```go
type HealthStatus struct {
    NodeID    string
    Status    string  // "healthy", "degraded", "unhealthy"
    Timestamp int64
    Metrics   map[string]float64  // cpu_usage, memory_usage, disk_usage, etc.
}
```

### 12.2 Metrics
- Write/read QPS
- Latency percentiles (p50, p95, p99)
- Cache hit rate
- Memtable size
- SSTable count
- Disk usage
- Memory usage
- Commit log size
- Gossip protocol metrics (peer connections, health propagation time)

### 12.3 Logging
- Write/read operations (with tenant_id, key)
- Memtable flushes
- SSTable compactions
- Errors and exceptions
- Gossip protocol events (peer connections, health status updates)

### 12.4 Health Checks
- Disk space availability
- Memory usage
- Commit log health
- SSTable integrity
- Gossip protocol connectivity

## 13. Scalability Considerations

### 13.1 Storage Scale
- Support terabytes per node
- Efficient disk usage (compaction reduces space)
- Monitor disk usage and alert on thresholds

### 13.2 Performance Scale
- Support thousands of reads/writes per second per node
- Cache and memtable optimize for hot data
- SSTables handle cold data efficiently

### 13.3 Resource Requirements
- **CPU**: Moderate (compaction, SSTable searches)
- **Memory**: High (cache, memtable)
- **Disk**: High (commit logs, SSTables)
- **Network**: Moderate (communication with coordinators)

## 14. Deployment

### 14.1 Containerization
- Docker container with storage node service
- Persistent volumes for commit logs and SSTables
- Health check endpoints (for Kubernetes liveness/readiness probes)

### 14.2 Orchestration

**Kubernetes Deployment**:
- **StatefulSet**: Used for storage nodes (stateful, persistent storage)
  - Stable network identities (headless service)
  - Ordered deployment and scaling
  - Persistent volume claims for commit logs and SSTables
  - Pod disruption budgets for high availability
- **Node Affinity**: Optional node affinity for dedicated storage nodes
- **Resource Limits**: CPU and memory limits per pod
- **Health Probes**:
  - Liveness probe: Health check endpoint
  - Readiness probe: Check if node can accept requests

### 14.3 Configuration
- **ConfigMaps**: For non-sensitive configuration
  - Data directory paths
  - Cache size
  - Memtable size threshold
  - Compaction policies (level sizes: L0=64MB, L1=128MB, etc.)
  - Commit log rotation policies (default size, time-based: few days)
  - Gossip protocol settings (interval, peer count)
- **Secrets**: For sensitive data (if needed)
- **Environment Variables**: For runtime configuration overrides

