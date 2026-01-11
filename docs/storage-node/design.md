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

#### 3.2.8 Disk Manager
- **Real-time disk space monitoring** with configurable check intervals
- **Three-tier threshold system**:
  - Warning threshold (default: 80%) - log warnings, no action
  - Throttle threshold (default: 90%) - reject large writes, allow small writes
  - Circuit breaker threshold (default: 95%) - reject all writes
- **Write size estimation** before accepting operations
- **Automatic recovery** when disk space becomes available
- **Metrics** for monitoring disk usage, throttling events, and rejections

#### 3.2.9 Validation Manager
- **Pre-write validation pipeline** for all operations
- **Size limit enforcement**:
  - Key: max 1KB
  - Value: max 10MB
  - Tenant ID: max 256 bytes
- **Security validation**:
  - Prevent null byte injection attacks
  - Block control characters in keys and tenant IDs
  - Forbid ':' character in tenant IDs (reserved as separator)
- **Vector clock validation**:
  - Max 1000 entries per vector clock
  - Max 128 bytes per coordinator node ID
  - Non-negative logical timestamps
- **Composite key validation** for tenant isolation

#### 3.2.10 Worker Pool Manager
- **Bounded goroutine pool** for background tasks with configurable size
- **Task queueing** with configurable buffer size
- **Panic recovery** to prevent worker crashes
- **Error tracking** and logging for all tasks
- **Graceful shutdown** with timeout for clean termination
- **Used for**: memtable flushes, compaction, async streaming operations
- **Metrics**: active workers, queue utilization, task success rate

#### 3.2.11 Streaming Manager (Phase 2)
- **Live data streaming** to new or existing nodes during migration
- **Three-phase streaming process**:
  1. Copy phase: bulk copy of historical data in batches
  2. Stream phase: live streaming of new writes (write interception)
  3. Sync phase: checksum verification and re-sync if needed
- **Key hash-based range streaming** for partitioned data distribution
- **Write interception** during streaming phase to capture live updates
- **Stream state tracking**: copying, streaming, syncing, completed, failed
- **Metrics**: keys copied, keys streamed, bytes transferred, duration

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
    IsTombstone bool         // True if this is a delete marker
}
```

### 4.2 Commit Log Entry
```go
type CommitLogEntry struct {
    SequenceNumber uint64        // Monotonically increasing sequence number
    TenantID       string
    Key            string
    Value          []byte
    VectorClock    VectorClock
    Timestamp      int64
    OperationType  string        // "write", "repair", "delete"
    Checksum       uint32        // CRC32 checksum for data integrity
}
```

### 4.3 MemTable Entry
```go
type MemTableEntry struct {
    Key         string  // Format: "{tenant_id}:{key}"
    Value       []byte
    VectorClock VectorClock
    Timestamp   int64
    IsTombstone bool    // True if this is a delete marker
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

### 8.5 Delete Operation
1. **Validate**: Check tenant_id and key format
2. **Create Tombstone**: Create entry with `IsTombstone=true`, `Value=nil`
3. **Commit Log**: Write to commit log with `OperationType=delete`
4. **MemTable**: Insert tombstone in memtable
5. **Cache**: Update cache with tombstone (or invalidate entry)
6. **Compaction**: Tombstones are removed during compaction (after propagation)
7. **Response**: Return success with updated vector clock

**Tombstone Lifecycle**:
- Tombstones are treated as special entries with nil value
- Preserved during compaction until TTL expires or manually cleaned
- Used to signal deletions during read repair and anti-entropy
- Eventually garbage collected during major compaction

### 8.6 Disk Space Check Flow
1. **Pre-write Estimation**: Estimate total bytes needed for write operation
   - Commit log entry size
   - MemTable overhead
   - Eventual SSTable size (amortized)
   - Safety margin (20% buffer)
2. **Check Thresholds**: Query disk manager for current state
3. **Handle States**:
   - **Normal** (< 80%): Allow write, no action
   - **Warning** (80-90%): Allow write, log warning
   - **Throttled** (90-95%): Reject large writes, allow small writes
   - **Circuit Breaker** (>= 95%): Reject all writes
4. **Error Response**: Return appropriate error code if rejected
5. **Automatic Recovery**: When disk space freed, automatically transition back to normal state

### 8.7 Migration Operations

#### 8.7.1 Node Addition - Receiving Data

When a new node is added and data is being migrated to it:

1. **Receive Write Requests**: Node receives write requests from coordinator
2. **Normal Write Flow**: Process writes through commit log → memtable → cache
3. **Bulk Data Reception**: For bulk migration, coordinator streams data
4. **Verify Data**: After migration, node verifies data integrity

#### 8.7.2 Node Deletion - Sending Data

When a node is being removed:

1. **Drain Request**: Coordinator calls DrainNode API
2. **Stop New Writes**: Node stops accepting new write requests
3. **Complete Ongoing Operations**: Finish processing current requests
4. **Stream Data**: Stream keys to coordinator for replication
5. **Verify Replication**: Ensure all data has been copied to other nodes
6. **Mark as Drained**: Return "drained" status to coordinator

#### 8.7.3 Data Streaming for Migration

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

### 11.1 Structured Error System

The storage node uses a **comprehensive structured error system** with error codes, context preservation, and automatic gRPC mapping.

**StorageError Structure**:
```go
type StorageError struct {
    Code    ErrorCode                  // Numeric error code
    Message string                     // Human-readable message
    Details map[string]interface{}     // Additional context
    Cause   error                      // Wrapped underlying error
}
```

**Key Features**:
- **Automatic gRPC mapping**: Internal error codes map to appropriate gRPC status codes
- **Context preservation**: Errors carry relevant details (tenant_id, key, sizes, usage, etc.)
- **Error chaining**: Underlying errors are wrapped and preserved
- **Type-safe helpers**: Convenience constructors for common error types

### 11.2 Error Code Categories

All error codes follow a structured numbering system:

#### Client Errors (1xxx series - equivalent to HTTP 4xx)

| Code | Name | Description | gRPC Code |
|------|------|-------------|-----------|
| 1000 | `ErrCodeInvalidArgument` | Invalid input parameters | InvalidArgument |
| 1001 | `ErrCodeKeyNotFound` | Key does not exist | NotFound |
| 1002 | `ErrCodeKeyTooLarge` | Key exceeds 1KB limit | InvalidArgument |
| 1003 | `ErrCodeValueTooLarge` | Value exceeds 10MB limit | InvalidArgument |
| 1004 | `ErrCodeInvalidTenantID` | Tenant ID format invalid or contains forbidden characters | InvalidArgument |
| 1005 | `ErrCodeInvalidKey` | Key contains null bytes or control characters | InvalidArgument |
| 1006 | `ErrCodeChecksumFailed` | Checksum validation failed on read | DataLoss |

#### Server Errors (2xxx series - equivalent to HTTP 5xx)

| Code | Name | Description | gRPC Code |
|------|------|-------------|-----------|
| 2000 | `ErrCodeInternal` | Internal server error | Internal |
| 2001 | `ErrCodeUnavailable` | Service temporarily unavailable | Unavailable |
| 2002 | `ErrCodeDiskFull` | Disk usage >= 95% (circuit breaker) | ResourceExhausted |
| 2003 | `ErrCodeDiskThrottled` | Disk usage >= 90% (throttled) | Unavailable |
| 2004 | `ErrCodeCommitLogFailed` | Commit log write failure | Internal |
| 2005 | `ErrCodeMemTableFailed` | MemTable operation failure | Internal |
| 2006 | `ErrCodeSSTableFailed` | SSTable operation failure | Internal |
| 2007 | `ErrCodeCorruptedData` | Data corruption detected | DataLoss |
| 2008 | `ErrCodeResourceExhausted` | Generic resource exhaustion | ResourceExhausted |

### 11.3 Data Corruption Handling

**Detection**:
- **CRC32 checksums** computed for all data in commit log and SSTables
- **Validation on read**: Checksum verified when reading from disk
- **Automatic skip**: Corrupted entries are skipped during recovery
- **Alerting**: Corruption events logged and metrics incremented

**Recovery**:
1. **Log corruption event** with full context (file, offset, checksum values)
2. **Skip corrupted entry** and continue processing
3. **Increment metrics**: `checksum_failures_total` counter
4. **Alert monitoring system**: Trigger alerts for manual investigation
5. **Use replication**: Coordinator can fetch correct value from replica nodes
6. **Manual intervention**: Operators can restore from backups or replicas

**Corruption Sources**:
- Disk hardware failures
- Bit rot over time
- File system bugs
- Incomplete writes (mitigated by sync operations)

### 11.4 Disk Full Handling

**Proactive Prevention**:
1. **Check before write**: Estimate write size and check available space
2. **Three-tier thresholds**:
   - 80%: Warning logged, no action
   - 90%: Throttling enabled (reject large writes)
   - 95%: Circuit breaker (reject all writes)
3. **Automatic recovery**: When space freed, system automatically recovers

**Mitigation Actions**:
- **Compact SSTables**: Trigger immediate compaction to reclaim space
- **Truncate commit logs**: Remove old segments after verification
- **Alert operators**: Send alerts for manual cleanup
- **Reject writes**: Return `ErrCodeDiskFull` or `ErrCodeDiskThrottled`

**Client Response**:
- Clients should implement **exponential backoff** for disk throttle errors
- Coordinators can **route to other replicas** when disk full detected
- Monitor `disk_usage_percent` metric to predict issues

### 11.5 Memory Pressure Handling

**Detection**:
- Monitor heap usage via Go runtime metrics
- Track cache size and memtable size
- Detect allocation pressure and GC frequency

**Mitigation**:
1. **Aggressive cache eviction**: Reduce cache size by evicting lowest-score entries
2. **Early memtable flush**: Trigger flush before reaching size threshold
3. **Reject new writes**: Return `ErrCodeResourceExhausted` if memory critical
4. **Throttle compaction**: Pause compaction workers to reduce memory usage

**Recovery**:
- Automatic recovery as memory pressure decreases
- Gradual ramp-up of cache size and compaction activity

### 11.6 Disk Hardware Failures

**Detection**:
- File system I/O errors
- Repeated write/read failures
- Syscall errors from disk operations

**Actions**:
1. **Mark node unhealthy**: Update health status to "unhealthy"
2. **Stop accepting writes**: Reject all write operations
3. **Continue reads from memory**: Cache and memtable still accessible
4. **Alert monitoring**: Send critical alerts
5. **Gossip failure**: Broadcast unhealthy status to coordinators
6. **Graceful degradation**: Allow reads to continue if possible

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

**Core Performance Metrics**:
- Write/read QPS (queries per second)
- Latency percentiles (p50, p95, p99, p999)
- Cache hit rate and miss rate
- Memtable size and flush frequency
- SSTable count per level
- Compaction duration and frequency

**Resource Utilization Metrics**:
- **Disk usage percentage** (critical for throttling/circuit breaker)
- **Disk available bytes**
- **Disk throttle events** (count of throttled writes)
- **Disk circuit breaker events** (count of rejected writes due to full disk)
- Memory usage (heap, cache, memtable)
- CPU usage
- Commit log size and rotation frequency

**Data Integrity Metrics**:
- **Checksum validation failures** (total count)
- **Checksum validation latency** (time to validate)
- **Corrupted entries detected** (commit log, SSTables)
- Data recovery operations

**Validation Metrics**:
- **Validation rejection rate** by reason (key size, value size, tenant ID format, etc.)
- **Write size estimation accuracy** (estimated vs actual)
- Input sanitization operations

**Worker Pool Metrics**:
- **Worker pool utilization** (active workers / max workers)
- **Queue utilization** (queued tasks / queue size)
- **Task success rate** (completed / total)
- **Task failure rate** (failed / total)
- **Task rejection rate** (rejected / submitted)
- Worker pool latency (time from submit to completion)

**Error Metrics**:
- **Error code distribution** (count by error code)
- Total errors by category (client vs server)
- Error rate per operation (write, read, delete)

**Streaming Metrics** (Phase 2):
- Active streams count
- Keys copied per stream
- Keys streamed per stream
- Bytes transferred (copied + streamed)
- Stream duration and throughput
- Checksum verification time

**Gossip Protocol Metrics**:
- Peer connections (active peers)
- Health propagation time (time to propagate status change)
- Gossip message rate

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

