# Storage Node Service: API Contracts

This document defines the gRPC API contracts for the Storage Node service.

## 1. Service Definition

```protobuf
service StorageNodeService {
  // Core Operations
  rpc Write(WriteRequest) returns (WriteResponse);
  rpc Read(ReadRequest) returns (ReadResponse);
  rpc Repair(RepairRequest) returns (RepairResponse);
  
  // Migration Operations
  rpc ReplicateData(ReplicateRequest) returns (ReplicateResponse);
  rpc StreamKeys(StreamKeysRequest) returns (stream KeyValueEntry);
  rpc GetKeyRange(KeyRangeRequest) returns (KeyRangeResponse);
  rpc DrainNode(DrainRequest) returns (DrainResponse);
  
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

All error responses include an error code:

```protobuf
enum ErrorCode {
  UNKNOWN = 0;
  INVALID_REQUEST = 1;
  TENANT_NOT_FOUND = 2;
  KEY_NOT_FOUND = 3;
  INTERNAL_ERROR = 4;
  DISK_FULL = 5;
  MEMORY_PRESSURE = 6;
  NODE_DRAINING = 7;
  CORRUPTED_DATA = 8;
}
```

## 7. Notes

- All timestamps are Unix timestamps (int64)
- Vector clocks use the format: list of `{coordinator_node_id, logical_timestamp}` pairs
- Tenant isolation is enforced at all layers
- Write operations go through: Commit Log → MemTable → Cache
- Read operations check: Cache → MemTable → SSTables
- Migration operations are used during node addition/deletion

