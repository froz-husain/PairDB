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
- **Protocol**: gRPC/HTTP service
- **Deployment**: Stateful, one instance per physical/virtual node
- **Language**: Go, C++, or Rust (recommended: Go for development speed, C++/Rust for performance)
- **Communication**: 
  - With Coordinators: gRPC or HTTP
  - Local: File system for commit logs and SSTables

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
- In-memory LRU/LFU cache for frequently accessed keys
- Tenant-aware caching
- Cache eviction policies
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

## 4. APIs Offered by Storage Node

### 4.1 Write API

**Endpoint**: `Write(WriteRequest) returns (WriteResponse)`

**Request**:
```protobuf
message WriteRequest {
  string tenant_id = 1;
  string key = 2;
  bytes value = 3;
  map<string, int64> vector_clock = 4;
  string idempotency_key = 5;  // Optional, for logging
}
```

**Response**:
```protobuf
message WriteResponse {
  bool success = 1;
  map<string, int64> updated_vector_clock = 2;
  string error_message = 3;
}
```

**Write Flow**:
1. Validate tenant_id and key
2. Write to commit log (append-only)
3. Update memtable
4. Update cache (if key exists in cache)
5. Update vector clock
6. Return success

### 4.2 Read API

**Endpoint**: `Read(ReadRequest) returns (ReadResponse)`

**Request**:
```protobuf
message ReadRequest {
  string tenant_id = 1;
  string key = 2;
}
```

**Response**:
```protobuf
message ReadResponse {
  bool success = 1;
  bytes value = 2;
  map<string, int64> vector_clock = 3;
  string error_message = 4;
}
```

**Read Flow**:
1. Validate tenant_id and key
2. Check cache (if hit, return)
3. Check memtable (if found, return and update cache)
4. Search SSTables (check multiple SSTables, return latest version)
5. Compare versions, check siblings
6. Return value and vector clock

### 4.3 Repair API

**Endpoint**: `Repair(RepairRequest) returns (RepairResponse)`

**Request**:
```protobuf
message RepairRequest {
  string tenant_id = 1;
  string key = 2;
  bytes value = 3;
  map<string, int64> vector_clock = 4;
}
```

**Response**:
```protobuf
message RepairResponse {
  bool success = 1;
  string error_message = 2;
}
```

**Repair Flow**:
1. Validate tenant_id and key
2. Write to commit log
3. Update memtable
4. Update cache
5. Update vector clock
6. Return success

### 4.4 Health Check API

**Endpoint**: `HealthCheck() returns (HealthResponse)`

**Response**:
```protobuf
message HealthResponse {
  bool healthy = 1;
  string status = 2;  // "healthy", "degraded", "unhealthy"
  map<string, string> metrics = 3;  // disk_usage, memory_usage, etc.
}
```

### 4.5 Migration APIs

Storage nodes participate in data migration when nodes are added or removed from the cluster.

#### 4.5.1 Replicate Data API

**Endpoint**: `ReplicateData(ReplicateRequest) returns (ReplicateResponse)`

**Purpose**: Used by coordinator to copy data from one node to another during migration.

**Request**:
```protobuf
message ReplicateRequest {
  string tenant_id = 1;
  string key_range_start = 2;  // Optional: for range-based replication
  string key_range_end = 3;     // Optional: for range-based replication
  repeated string keys = 4;     // Optional: specific keys to replicate
  string target_node_id = 5;    // Node to replicate to
  bool include_vector_clock = 6; // Include vector clocks in replication
}
```

**Response**:
```protobuf
message ReplicateResponse {
  bool success = 1;
  int64 keys_replicated = 2;
  string error_message = 3;
}
```

**Replication Flow**:
1. Coordinator calls ReplicateData on source node
2. Source node reads keys from SSTables (and memtable if needed)
3. Source node streams key-value pairs to coordinator
4. Coordinator writes to target node
5. Target node acknowledges writes

#### 4.5.2 Stream Keys API

**Endpoint**: `StreamKeys(StreamKeysRequest) returns (stream KeyValueEntry)`

**Purpose**: Efficiently stream keys for migration (better for large datasets).

**Request**:
```protobuf
message StreamKeysRequest {
  string tenant_id = 1;
  string key_range_start = 2;
  string key_range_end = 3;
  int32 batch_size = 4;  // Number of keys per batch
}
```

**Response Stream**:
```protobuf
message KeyValueEntry {
  string tenant_id = 1;
  string key = 2;
  bytes value = 3;
  map<string, int64> vector_clock = 4;
  int64 timestamp = 5;
}
```

**Streaming Flow**:
1. Coordinator initiates stream
2. Storage node reads keys in batches
3. Storage node streams batches to coordinator
4. Coordinator writes batches to target node
5. Process continues until all keys streamed

#### 4.5.3 Get Key Range API

**Endpoint**: `GetKeyRange(KeyRangeRequest) returns (KeyRangeResponse)`

**Purpose**: Get list of keys in a specific range (for migration planning).

**Request**:
```protobuf
message KeyRangeRequest {
  string tenant_id = 1;
  string key_range_start = 2;
  string key_range_end = 3;
  int32 max_keys = 4;  // Limit number of keys returned
}
```

**Response**:
```protobuf
message KeyRangeResponse {
  repeated string keys = 1;
  bool has_more = 2;
  string next_key = 3;  // For pagination
}
```

#### 4.5.4 Drain Node API

**Endpoint**: `DrainNode(DrainRequest) returns (DrainResponse)`

**Purpose**: Prepare node for removal by stopping new writes and ensuring data is replicated.

**Request**:
```protobuf
message DrainRequest {
  bool graceful = 1;  // Wait for ongoing operations to complete
  int32 timeout_seconds = 2;  // Timeout for graceful drain
}
```

**Response**:
```protobuf
message DrainResponse {
  bool success = 1;
  string status = 2;  // "drained", "draining", "failed"
  string error_message = 3;
}
```

**Drain Flow**:
1. Coordinator calls DrainNode
2. Storage node stops accepting new writes
3. Storage node completes ongoing operations
4. Storage node verifies all data has been replicated
5. Storage node returns "drained" status
6. Node can be safely removed

## 5. Storage Architecture

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
- **Rotation**: When memtable flushes to SSTable
- **Recovery**: Replay commit log on node restart

### 5.3 In-Memory Cache
- **Purpose**: Fast lookup for frequently accessed keys
- **Type**: LRU or LFU eviction policy
- **Key Format**: `{tenant_id}:{key}`
- **Size**: Configurable (e.g., 1GB per node)
- **Eviction**: When cache is full, evict least recently/frequently used

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
- **Compaction**: Background process to merge and clean up SSTables

## 6. Data Structures

### 6.1 Key-Value Entry
```go
type KeyValueEntry struct {
    TenantID    string
    Key         string
    Value       []byte
    VectorClock map[string]int64
    Timestamp   int64
}
```

### 6.2 Vector Clock
- **Format**: `map[replica_id]logical_timestamp`
- **Example**: `{"replica_1": 5, "replica_2": 3, "replica_3": 2}`
- **Storage**: Serialized with each key-value pair

## 7. Key Operations

### 7.1 Write Operation
1. **Validate**: Check tenant_id and key format
2. **Commit Log**: Append write to commit log (sync to disk)
3. **MemTable**: Insert/update in memtable
4. **Cache**: Update cache if key exists
5. **Vector Clock**: Update vector clock
6. **Response**: Return success with updated vector clock

### 7.2 Read Operation
1. **Validate**: Check tenant_id and key format
2. **Cache**: Check cache (if hit, return)
3. **MemTable**: Check memtable (if found, return and cache)
4. **SSTables**: Search SSTables (check multiple, return latest)
5. **Version Comparison**: Compare versions, handle siblings
6. **Response**: Return value and vector clock

### 7.3 MemTable Flush
1. **Trigger**: When memtable size exceeds threshold
2. **Create SSTable**: Write memtable contents to new SSTable
3. **Sort**: SSTable is already sorted (memtable is sorted)
4. **Commit Log**: Truncate commit log (data now in SSTable)
5. **Clear MemTable**: Create new empty memtable

### 7.4 SSTable Compaction
1. **Trigger**: Background process, periodic or on threshold
2. **Select SSTables**: Choose SSTables to merge
3. **Merge**: Merge sorted SSTables, remove duplicates
4. **Write**: Write merged data to new SSTable
5. **Cleanup**: Delete old SSTables

### 7.5 Migration Operations

#### 7.5.1 Node Addition - Receiving Data

When a new node is added and data is being migrated to it:

1. **Receive Write Requests**: Node receives write requests from coordinator
2. **Normal Write Flow**: Process writes through commit log → memtable → cache
3. **Bulk Data Reception**: For bulk migration, coordinator streams data
4. **Verify Data**: After migration, node verifies data integrity

#### 7.5.2 Node Deletion - Sending Data

When a node is being removed:

1. **Drain Request**: Coordinator calls DrainNode API
2. **Stop New Writes**: Node stops accepting new write requests
3. **Complete Ongoing Operations**: Finish processing current requests
4. **Stream Data**: Stream keys to coordinator for replication
5. **Verify Replication**: Ensure all data has been copied to other nodes
6. **Mark as Drained**: Return "drained" status to coordinator

#### 7.5.3 Data Streaming for Migration

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

## 8. Tenant Isolation

### 8.1 Logical Separation
- All data structures include tenant_id
- Keys are prefixed with tenant_id: `{tenant_id}:{key}`
- Separate memtables per tenant (logical, not physical)
- SSTables organized by tenant (directory structure)

### 8.2 Validation
- Validate tenant_id on all operations
- Reject operations with invalid tenant_id
- Log cross-tenant access attempts

## 9. Performance Optimizations

### 9.1 Write Optimizations
- **Batch Writes**: Batch multiple writes before flushing to commit log
- **Async Flush**: Don't block on commit log sync (optional, trade-off durability)
- **MemTable Size**: Tune memtable size for write performance

### 9.2 Read Optimizations
- **Cache**: Aggressive caching of frequently accessed keys
- **SSTable Indexing**: Index SSTables for faster lookups
- **Bloom Filters**: Use bloom filters to avoid unnecessary SSTable reads

### 9.3 Compaction Optimizations
- **Background Compaction**: Don't block reads/writes
- **Priority Compaction**: Prioritize compaction of frequently accessed SSTables
- **Parallel Compaction**: Compact multiple SSTables in parallel

## 10. Error Handling

### 10.1 Disk Failures
- Detect disk failures
- Mark node as unhealthy
- Stop accepting writes (reads may continue from cache/memtable)
- Alert monitoring system

### 10.2 Memory Pressure
- Monitor memory usage
- Aggressive cache eviction if memory pressure
- Trigger memtable flush early if needed
- Reject new writes if memory exhausted

### 10.3 Corrupted Data
- Detect corrupted commit log entries
- Skip corrupted entries during recovery
- Log errors for manual intervention
- Use checksums for data integrity

## 11. Monitoring and Observability

### 11.1 Metrics
- Write/read QPS
- Latency percentiles (p50, p95, p99)
- Cache hit rate
- Memtable size
- SSTable count
- Disk usage
- Memory usage
- Commit log size

### 11.2 Logging
- Write/read operations (with tenant_id, key)
- Memtable flushes
- SSTable compactions
- Errors and exceptions

### 11.3 Health Checks
- Disk space availability
- Memory usage
- Commit log health
- SSTable integrity

## 12. Scalability Considerations

### 12.1 Storage Scale
- Support terabytes per node
- Efficient disk usage (compaction reduces space)
- Monitor disk usage and alert on thresholds

### 12.2 Performance Scale
- Support thousands of reads/writes per second per node
- Cache and memtable optimize for hot data
- SSTables handle cold data efficiently

### 12.3 Resource Requirements
- **CPU**: Moderate (compaction, SSTable searches)
- **Memory**: High (cache, memtable)
- **Disk**: High (commit logs, SSTables)
- **Network**: Moderate (communication with coordinators)

## 13. Deployment

### 13.1 Containerization
- Docker container with storage node service
- Persistent volumes for commit logs and SSTables
- Health check endpoints

### 13.2 Orchestration
- Kubernetes StatefulSet (for persistent storage)
- Node affinity for dedicated storage nodes
- Persistent volume claims for data

### 13.3 Configuration
- Environment variables for:
  - Data directory paths
  - Cache size
  - Memtable size threshold
  - Compaction policies
  - Commit log rotation policies

