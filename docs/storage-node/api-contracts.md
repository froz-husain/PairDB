# Storage Node Service: API Contracts

This document defines the gRPC API contracts for the Storage Node service.

## 1. Service Definition

```protobuf
service StorageNodeService {
  // Core Operations
  rpc Write(WriteRequest) returns (WriteResponse);
  rpc Read(ReadRequest) returns (ReadResponse);
  rpc Delete(DeleteRequest) returns (DeleteResponse);
  rpc Repair(RepairRequest) returns (RepairResponse);

  // Migration Operations
  rpc ReplicateData(ReplicateRequest) returns (ReplicateResponse);
  rpc StreamKeys(StreamKeysRequest) returns (stream KeyValueEntry);
  rpc GetKeyRange(KeyRangeRequest) returns (KeyRangeResponse);
  rpc DrainNode(DrainRequest) returns (DrainResponse);

  // Streaming Operations (Phase 2)
  rpc StartStreaming(StartStreamingRequest) returns (StartStreamingResponse);
  rpc StopStreaming(StopStreamingRequest) returns (StopStreamingResponse);
  rpc GetStreamStatus(GetStreamStatusRequest) returns (GetStreamStatusResponse);
  rpc NotifyStreamingComplete(NotifyStreamingCompleteRequest) returns (NotifyStreamingCompleteResponse);

  // Health Check
  rpc HealthCheck(HealthCheckRequest) returns (HealthResponse);
}
```

## 2. Common Types

### 2.1 Vector Clock

```protobuf
message VectorClockEntry {
  string coordinator_node_id = 1;
  int64 logical_timestamp = 2;
}

message VectorClock {
  repeated VectorClockEntry entries = 1;
}
```

### 2.2 Key-Value Entry

```protobuf
message KeyValueEntry {
  string tenant_id = 1;
  string key = 2;
  bytes value = 3;
  VectorClock vector_clock = 4;
  int64 timestamp = 5;
  bool is_tombstone = 6;  // True if this is a delete marker
}
```

## 3. Core Operations

### 3.1 Write

**Method**: `Write(WriteRequest) returns (WriteResponse)`

**Request**:
```protobuf
message WriteRequest {
  string tenant_id = 1;
  string key = 2;
  bytes value = 3;
  VectorClock vector_clock = 4;
  string idempotency_key = 5;  // Optional, for logging
}
```

**Response**:
```protobuf
message WriteResponse {
  bool success = 1;
  VectorClock updated_vector_clock = 2;
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

### 3.2 Read

**Method**: `Read(ReadRequest) returns (ReadResponse)`

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
  VectorClock vector_clock = 3;
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

### 3.3 Repair

**Method**: `Repair(RepairRequest) returns (RepairResponse)`

**Request**:
```protobuf
message RepairRequest {
  string tenant_id = 1;
  string key = 2;
  bytes value = 3;
  VectorClock vector_clock = 4;
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

### 3.4 Delete

**Method**: `Delete(DeleteRequest) returns (DeleteResponse)`

**Request**:
```protobuf
message DeleteRequest {
  string tenant_id = 1;
  string key = 2;
  VectorClock vector_clock = 3;
  string idempotency_key = 4;  // Optional, for logging
}
```

**Response**:
```protobuf
message DeleteResponse {
  bool success = 1;
  VectorClock updated_vector_clock = 2;
  string error_message = 3;
}
```

**Delete Flow**:
1. Validate tenant_id and key
2. Create tombstone entry (IsTombstone=true, Value=nil)
3. Write tombstone to commit log with OperationType=delete
4. Insert tombstone in memtable
5. Update/invalidate cache
6. Update vector clock
7. Return success

**Tombstone Behavior**:
- Tombstones are special markers indicating key deletion
- Preserved in storage until compaction with TTL expiration
- Used for conflict resolution and anti-entropy
- Eventually garbage collected during major compaction

## 4. Migration Operations

### 4.1 Replicate Data

**Method**: `ReplicateData(ReplicateRequest) returns (ReplicateResponse)`

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

### 4.2 Stream Keys

**Method**: `StreamKeys(StreamKeysRequest) returns (stream KeyValueEntry)`

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
  VectorClock vector_clock = 4;
  int64 timestamp = 5;
}
```

**Streaming Flow**:
1. Coordinator initiates stream
2. Storage node reads keys in batches
3. Storage node streams batches to coordinator
4. Coordinator writes batches to target node
5. Process continues until all keys streamed

### 4.3 Get Key Range

**Method**: `GetKeyRange(KeyRangeRequest) returns (KeyRangeResponse)`

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

### 4.4 Drain Node

**Method**: `DrainNode(DrainRequest) returns (DrainResponse)`

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

### 4.5 Start Streaming (Phase 2)

**Method**: `StartStreaming(StartStreamingRequest) returns (StartStreamingResponse)`

**Purpose**: Initiates live data streaming to a target node for migration or replication.

**Request**:
```protobuf
message StartStreamingRequest {
  string target_node_id = 1;
  string target_host = 2;
  int32 target_port = 3;
  repeated KeyRange key_ranges = 4;  // Hash ranges to stream
  int32 batch_size = 5;              // Keys per batch during copy phase
}

message KeyRange {
  uint64 start_hash = 1;  // Inclusive
  uint64 end_hash = 2;    // Exclusive
}
```

**Response**:
```protobuf
message StartStreamingResponse {
  bool success = 1;
  string stream_id = 2;
  string error_message = 3;
}
```

**Streaming Flow**:
1. Create streaming context for target node
2. Start bulk copy phase in background worker
3. Enable write interception for live streaming
4. Return stream ID for status tracking

### 4.6 Stop Streaming (Phase 2)

**Method**: `StopStreaming(StopStreamingRequest) returns (StopStreamingResponse)`

**Request**:
```protobuf
message StopStreamingRequest {
  string target_node_id = 1;
  bool graceful = 2;  // Wait for pending operations
}
```

**Response**:
```protobuf
message StopStreamingResponse {
  bool success = 1;
  string error_message = 2;
}
```

### 4.7 Get Stream Status (Phase 2)

**Method**: `GetStreamStatus(GetStreamStatusRequest) returns (GetStreamStatusResponse)`

**Request**:
```protobuf
message GetStreamStatusRequest {
  string target_node_id = 1;
}
```

**Response**:
```protobuf
message GetStreamStatusResponse {
  bool active = 1;
  string state = 2;  // "copying", "streaming", "syncing", "completed", "failed"
  int64 keys_copied = 3;
  int64 keys_streamed = 4;
  int64 bytes_copied = 5;
  int64 bytes_streamed = 6;
  int64 duration_seconds = 7;
}
```

### 4.8 Notify Streaming Complete (Phase 2)

**Method**: `NotifyStreamingComplete(NotifyStreamingCompleteRequest) returns (NotifyStreamingCompleteResponse)`

**Purpose**: Coordinator notifies storage node that streaming is complete and routing can be updated.

**Request**:
```protobuf
message NotifyStreamingCompleteRequest {
  string source_node_id = 1;
  repeated KeyRange key_ranges = 2;
}
```

**Response**:
```protobuf
message NotifyStreamingCompleteResponse {
  bool success = 1;
  string error_message = 2;
}
```

## 5. Health Check

### 5.1 Health Check

**Method**: `HealthCheck(HealthCheckRequest) returns (HealthResponse)`

**Request**:
```protobuf
message HealthCheckRequest {
  // Empty
}
```

**Response**:
```protobuf
message HealthResponse {
  bool healthy = 1;
  string status = 2;  // "healthy", "degraded", "unhealthy"
  map<string, string> metrics = 3;  // disk_usage, memory_usage, etc.
}
```

## 6. Error Codes

All error responses use structured error codes with automatic gRPC mapping:

```protobuf
enum ErrorCode {
  // Success
  OK = 0;

  // Client Errors (1xxx - equivalent to HTTP 4xx)
  INVALID_ARGUMENT = 1000;      // Invalid input parameters
  KEY_NOT_FOUND = 1001;          // Key does not exist
  KEY_TOO_LARGE = 1002;          // Key exceeds 1KB limit
  VALUE_TOO_LARGE = 1003;        // Value exceeds 10MB limit
  INVALID_TENANT_ID = 1004;      // Tenant ID format invalid
  INVALID_KEY = 1005;            // Key contains forbidden characters
  CHECKSUM_FAILED = 1006;        // Checksum validation failed

  // Server Errors (2xxx - equivalent to HTTP 5xx)
  INTERNAL = 2000;               // Internal server error
  UNAVAILABLE = 2001;            // Service temporarily unavailable
  DISK_FULL = 2002;              // Disk usage >= 95% (circuit breaker)
  DISK_THROTTLED = 2003;         // Disk usage >= 90% (throttled)
  COMMIT_LOG_FAILED = 2004;      // Commit log write failure
  MEMTABLE_FAILED = 2005;        // MemTable operation failure
  SSTABLE_FAILED = 2006;         // SSTable operation failure
  CORRUPTED_DATA = 2007;         // Data corruption detected
  RESOURCE_EXHAUSTED = 2008;     // Generic resource exhaustion
}
```

### 6.1 Error Code to gRPC Code Mapping

| Internal Code | gRPC Status Code | Description |
|---------------|------------------|-------------|
| OK (0) | OK | Success |
| INVALID_ARGUMENT (1000) | InvalidArgument | Bad request parameters |
| KEY_NOT_FOUND (1001) | NotFound | Key does not exist |
| KEY_TOO_LARGE (1002) | InvalidArgument | Key exceeds size limit |
| VALUE_TOO_LARGE (1003) | InvalidArgument | Value exceeds size limit |
| INVALID_TENANT_ID (1004) | InvalidArgument | Invalid tenant ID format |
| INVALID_KEY (1005) | InvalidArgument | Invalid key format |
| CHECKSUM_FAILED (1006) | DataLoss | Checksum mismatch |
| INTERNAL (2000) | Internal | Server error |
| UNAVAILABLE (2001) | Unavailable | Service unavailable |
| DISK_FULL (2002) | ResourceExhausted | Disk full |
| DISK_THROTTLED (2003) | Unavailable | Disk throttled |
| COMMIT_LOG_FAILED (2004) | Internal | Commit log error |
| MEMTABLE_FAILED (2005) | Internal | MemTable error |
| SSTABLE_FAILED (2006) | Internal | SSTable error |
| CORRUPTED_DATA (2007) | DataLoss | Data corrupted |
| RESOURCE_EXHAUSTED (2008) | ResourceExhausted | Resource exhausted |

### 6.2 Error Response Structure

All operation responses include error details when failures occur:

```protobuf
message ErrorDetails {
  ErrorCode code = 1;
  string message = 2;
  map<string, string> details = 3;  // Additional context (tenant_id, key, size, etc.)
}
```

**Client Error Handling Guidelines**:
- **INVALID_ARGUMENT, KEY_TOO_LARGE, VALUE_TOO_LARGE**: Fix input and retry
- **KEY_NOT_FOUND**: Key legitimately doesn't exist, no retry needed
- **DISK_THROTTLED**: Implement exponential backoff, route to other replicas
- **DISK_FULL**: Route to other replicas, alert operators
- **UNAVAILABLE**: Retry with exponential backoff
- **CHECKSUM_FAILED, CORRUPTED_DATA**: Read from another replica
- **INTERNAL, COMMIT_LOG_FAILED, etc.**: Retry with backoff, alert on persistence

## 7. Notes

### 7.1 Input Validation

All write operations enforce strict validation:

**Size Limits**:
- Key: Maximum 1KB (1024 bytes)
- Value: Maximum 10MB (10,485,760 bytes)
- Tenant ID: Maximum 256 bytes
- Vector Clock: Maximum 1000 entries
- Coordinator Node ID: Maximum 128 bytes per entry

**Security Validation**:
- Tenant IDs cannot contain ':' character (reserved as separator)
- Keys and tenant IDs cannot contain null bytes (prevents injection)
- Control characters (except tab and newline in keys) are forbidden
- Vector clock logical timestamps must be non-negative

**Rejection Behavior**:
- Invalid inputs return `INVALID_ARGUMENT` error immediately
- Size limit violations return specific error codes (KEY_TOO_LARGE, VALUE_TOO_LARGE)
- No partial writes - validation occurs before any storage operation

### 7.2 Data Integrity

**Checksums**:
- CRC32 checksums computed for all commit log entries
- CRC32 checksums computed for all SSTable data blocks
- Validation performed on every read operation
- Checksum failures return `CHECKSUM_FAILED` error and increment metrics
- Corrupted data is skipped, and coordinator can read from replicas

**Durability Guarantees**:
- Commit log synced to disk before write acknowledgment (configurable)
- Sequence numbers ensure ordering during recovery
- Vector clocks preserved with every entry for causality tracking

### 7.3 Disk Space Management

**Proactive Checking**:
- Write size estimated before accepting operation
- Disk space checked against thresholds before write
- Estimation includes commit log, memtable, and amortized SSTable space

**Threshold Behavior**:
- < 80%: Normal operation
- 80-90%: Warning logged, operation continues
- 90-95%: Throttling (large writes rejected, small writes allowed)
- >= 95%: Circuit breaker (all writes rejected)

**Error Responses**:
- Throttled writes return `DISK_THROTTLED` (gRPC: Unavailable)
- Circuit breaker returns `DISK_FULL` (gRPC: ResourceExhausted)
- Clients should implement exponential backoff and route to other replicas

### 7.4 General Notes

- All timestamps are Unix timestamps (int64)
- Vector clocks use the format: list of `{coordinator_node_id, logical_timestamp}` pairs
- Tenant isolation is enforced at all layers
- Write operations go through: Validation → Commit Log → MemTable → Cache
- Read operations check: Cache → MemTable → SSTables
- Tombstones (IsTombstone=true) indicate deleted keys
- Migration operations (Phase 2) use streaming for efficient data transfer

