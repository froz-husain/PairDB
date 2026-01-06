# Coordinator Service: API Contracts

This document defines the gRPC API contracts for the Coordinator service.

## 1. Service Definition

```protobuf
service CoordinatorService {
  // Key-Value Operations
  rpc WriteKeyValue(WriteKeyValueRequest) returns (WriteKeyValueResponse);
  rpc ReadKeyValue(ReadKeyValueRequest) returns (ReadKeyValueResponse);
  
  // Tenant Management
  rpc CreateTenant(CreateTenantRequest) returns (CreateTenantResponse);
  rpc UpdateReplicationFactor(UpdateReplicationFactorRequest) returns (UpdateReplicationFactorResponse);
  rpc GetTenant(GetTenantRequest) returns (GetTenantResponse);
  
  // Storage Node Management
  rpc AddStorageNode(AddStorageNodeRequest) returns (AddStorageNodeResponse);
  rpc RemoveStorageNode(RemoveStorageNodeRequest) returns (RemoveStorageNodeResponse);
  rpc GetMigrationStatus(GetMigrationStatusRequest) returns (GetMigrationStatusResponse);
  rpc ListStorageNodes(ListStorageNodesRequest) returns (ListStorageNodesResponse);
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

## 3. Key-Value Operations

### 3.1 Write Key-Value

**Method**: `WriteKeyValue(WriteKeyValueRequest) returns (WriteKeyValueResponse)`

**Request**:
```protobuf
message WriteKeyValueRequest {
  string tenant_id = 1;
  string key = 2;
  bytes value = 3;
  string consistency = 4;  // "one", "quorum", "all"
  string idempotency_key = 5;  // Optional: client-provided idempotency key
}
```

**Response**:
```protobuf
message WriteKeyValueResponse {
  bool success = 1;
  string key = 2;
  string idempotency_key = 3;  // Server-generated if not provided
  VectorClock vector_clock = 4;
  int32 replica_count = 5;  // Number of replicas that acknowledged
  string consistency = 6;
  bool is_duplicate = 7;  // true if request was deduplicated
  string error_message = 8;
}
```

### 3.2 Read Key-Value

**Method**: `ReadKeyValue(ReadKeyValueRequest) returns (ReadKeyValueResponse)`

**Request**:
```protobuf
message ReadKeyValueRequest {
  string tenant_id = 1;
  string key = 2;
  string consistency = 3;  // "one", "quorum", "all"
}
```

**Response**:
```protobuf
message ReadKeyValueResponse {
  bool success = 1;
  string key = 2;
  bytes value = 3;
  VectorClock vector_clock = 4;
  string error_message = 5;
}
```

## 4. Tenant Management Operations

### 4.1 Create Tenant

**Method**: `CreateTenant(CreateTenantRequest) returns (CreateTenantResponse)`

**Request**:
```protobuf
message CreateTenantRequest {
  string tenant_id = 1;
  int32 replication_factor = 2;  // Optional, default: 3
}
```

**Response**:
```protobuf
message CreateTenantResponse {
  bool success = 1;
  string tenant_id = 2;
  int32 replication_factor = 3;
  int64 created_at = 4;  // Unix timestamp
  string error_message = 5;
}
```

### 4.2 Update Replication Factor

**Method**: `UpdateReplicationFactor(UpdateReplicationFactorRequest) returns (UpdateReplicationFactorResponse)`

**Request**:
```protobuf
message UpdateReplicationFactorRequest {
  string tenant_id = 1;
  int32 new_replication_factor = 2;
}
```

**Response**:
```protobuf
message UpdateReplicationFactorResponse {
  bool success = 1;
  string tenant_id = 2;
  int32 old_replication_factor = 3;
  int32 new_replication_factor = 4;
  string migration_id = 5;  // If migration is triggered
  int64 updated_at = 6;  // Unix timestamp
  string error_message = 7;
}
```

### 4.3 Get Tenant

**Method**: `GetTenant(GetTenantRequest) returns (GetTenantResponse)`

**Request**:
```protobuf
message GetTenantRequest {
  string tenant_id = 1;
}
```

**Response**:
```protobuf
message GetTenantResponse {
  bool success = 1;
  string tenant_id = 2;
  int32 replication_factor = 3;
  int64 created_at = 4;
  int64 updated_at = 5;
  string error_message = 6;
}
```

## 5. Storage Node Management Operations

### 5.1 Add Storage Node

**Method**: `AddStorageNode(AddStorageNodeRequest) returns (AddStorageNodeResponse)`

**Request**:
```protobuf
message AddStorageNodeRequest {
  string node_id = 1;
  string host = 2;
  int32 port = 3;
  int32 virtual_nodes = 4;  // Number of virtual nodes (default: 150)
}
```

**Response**:
```protobuf
message AddStorageNodeResponse {
  bool success = 1;
  string node_id = 2;
  string migration_id = 3;
  string message = 4;
  int64 estimated_completion = 5;  // Unix timestamp
  string error_message = 6;
}
```

### 5.2 Remove Storage Node

**Method**: `RemoveStorageNode(RemoveStorageNodeRequest) returns (RemoveStorageNodeResponse)`

**Request**:
```protobuf
message RemoveStorageNodeRequest {
  string node_id = 1;
  bool force = 2;  // Force removal without data migration
}
```

**Response**:
```protobuf
message RemoveStorageNodeResponse {
  bool success = 1;
  string node_id = 2;
  string migration_id = 3;
  string message = 4;
  int64 estimated_completion = 5;  // Unix timestamp
  string error_message = 6;
}
```

### 5.3 Get Migration Status

**Method**: `GetMigrationStatus(GetMigrationStatusRequest) returns (GetMigrationStatusResponse)`

**Request**:
```protobuf
message GetMigrationStatusRequest {
  string migration_id = 1;
}
```

**Response**:
```protobuf
message GetMigrationStatusResponse {
  bool success = 1;
  string migration_id = 2;
  string type = 3;  // "node_addition" or "node_deletion"
  string node_id = 4;
  string status = 5;  // "pending", "in_progress", "completed", "failed", "cancelled"
  MigrationProgress progress = 6;
  int64 started_at = 7;
  int64 estimated_completion = 8;
  string error_message = 9;
}

message MigrationProgress {
  int64 keys_migrated = 1;
  int64 total_keys = 2;
  float percentage = 3;
}
```

### 5.4 List Storage Nodes

**Method**: `ListStorageNodes(ListStorageNodesRequest) returns (ListStorageNodesResponse)`

**Request**:
```protobuf
message ListStorageNodesRequest {
  // Empty - lists all nodes
}
```

**Response**:
```protobuf
message ListStorageNodesResponse {
  bool success = 1;
  repeated StorageNodeInfo nodes = 2;
  string error_message = 3;
}

message StorageNodeInfo {
  string node_id = 1;
  string host = 2;
  int32 port = 3;
  string status = 4;  // "active", "draining", "inactive"
  int32 virtual_nodes = 5;
  int64 keys_count = 6;
  float disk_usage_percent = 7;
}
```

## 6. Error Codes

All error responses include an error code:

```protobuf
enum ErrorCode {
  UNKNOWN = 0;
  INVALID_REQUEST = 1;
  TENANT_NOT_FOUND = 2;
  TENANT_EXISTS = 3;
  KEY_NOT_FOUND = 4;
  QUORUM_NOT_REACHED = 5;
  IDEMPOTENCY_KEY_CONFLICT = 6;
  INVALID_REPLICATION_FACTOR = 7;
  NODE_IN_USE = 8;
  MIGRATION_NOT_FOUND = 9;
  UNAUTHORIZED = 10;
  FORBIDDEN = 11;
  RATE_LIMIT_EXCEEDED = 12;
  INTERNAL_ERROR = 13;
}
```

## 7. Notes

- All timestamps are Unix timestamps (int64)
- Vector clocks use the format: list of `{coordinator_node_id, logical_timestamp}` pairs
- Consistency levels: "one", "quorum", "all"
- Idempotency keys are optional for write operations
- If idempotency key is not provided, server generates one and returns it in response

