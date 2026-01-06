# API Contracts: pairDB Key-Value Store

## 1. Base URL

All APIs are served under the base path: `/v1`

## 2. Authentication

All APIs require authentication via Bearer token in the Authorization header:

```
Authorization: Bearer <token>
```

The token contains tenant information which is extracted by the API Gateway.

## 3. Tenant Management APIs

### 3.1 Create Tenant

**Endpoint**: `POST /v1/tenants`

**Request Headers**:
```
Content-Type: application/json
Authorization: Bearer <admin_token>
```

**Request Body**:
```json
{
  "tenant_id": "string",
  "replication_factor": 3  // optional, default: 3
}
```

**Response** (200 OK):
```json
{
  "status": "success",
  "tenant_id": "string",
  "replication_factor": 3,
  "created_at": "2024-01-01T12:00:00Z"
}
```

**Error Response** (400 Bad Request):
```json
{
  "status": "error",
  "error_code": "INVALID_REQUEST",
  "message": "Invalid tenant_id or replication_factor"
}
```

**Error Response** (409 Conflict):
```json
{
  "status": "error",
  "error_code": "TENANT_EXISTS",
  "message": "Tenant already exists"
}
```

### 3.2 Update Replication Factor

**Endpoint**: `PUT /v1/tenants/{tenant_id}/replication-factor`

**Request Headers**:
```
Content-Type: application/json
Authorization: Bearer <admin_token>
```

**Request Body**:
```json
{
  "replication_factor": 5
}
```

**Response** (200 OK):
```json
{
  "status": "success",
  "tenant_id": "string",
  "old_replication_factor": 3,
  "new_replication_factor": 5,
  "updated_at": "2024-01-01T12:00:00Z"
}
```

**Error Response** (404 Not Found):
```json
{
  "status": "error",
  "error_code": "TENANT_NOT_FOUND",
  "message": "Tenant not found"
}
```

**Error Response** (400 Bad Request):
```json
{
  "status": "error",
  "error_code": "INVALID_REPLICATION_FACTOR",
  "message": "Replication factor must be between 1 and 10"
}
```

### 3.3 Get Tenant Configuration

**Endpoint**: `GET /v1/tenants/{tenant_id}`

**Request Headers**:
```
Authorization: Bearer <admin_token>
```

**Response** (200 OK):
```json
{
  "status": "success",
  "tenant_id": "string",
  "replication_factor": 3,
  "created_at": "2024-01-01T12:00:00Z",
  "updated_at": "2024-01-01T12:00:00Z"
}
```

**Error Response** (404 Not Found):
```json
{
  "status": "error",
  "error_code": "TENANT_NOT_FOUND",
  "message": "Tenant not found"
}
```

## 4. Key-Value APIs

### 4.1 Store Key-Value Pair

**Endpoint**: `POST /v1/key-value`

**Request Headers**:
```
Content-Type: application/json
Authorization: Bearer <tenant_token>
Idempotency-Key: <optional_client_provided_key>  // Optional header
```

**Request Body**:
```json
{
  "key": "string",
  "value": <opaque object>,  // Can be string, number, object, array, etc.
  "consistency": "quorum"  // optional: "one", "quorum", "all". Default: "quorum"
}
```

**Request Parameters**:
- `key` (required, string): The key to store
- `value` (required, any): The value to store (opaque object)
- `consistency` (optional, string): Consistency level. One of: "one", "quorum", "all". Default: "quorum"
- `Idempotency-Key` (optional header, string): Client-provided idempotency key. If not provided, server generates one.

**Response** (200 OK):
```json
{
  "status": "success",
  "key": "string",
  "idempotency_key": "string",  // Server-generated if not provided by client
  "vector_clock": {
    "replica_1": 1,
    "replica_2": 0,
    "replica_3": 0
  },
  "replica_count": 2,  // number of replicas that acknowledged
  "consistency": "quorum",
  "is_duplicate": false  // true if request was deduplicated using idempotency key
}
```

**Response** (200 OK - Duplicate Request):
```json
{
  "status": "success",
  "key": "string",
  "idempotency_key": "abc-123-def-456",
  "vector_clock": {
    "replica_1": 1,
    "replica_2": 0,
    "replica_3": 0
  },
  "replica_count": 2,
  "consistency": "quorum",
  "is_duplicate": true  // Indicates this was a duplicate request
}
```

**Error Response** (400 Bad Request):
```json
{
  "status": "error",
  "error_code": "INVALID_REQUEST",
  "message": "Missing required field: key"
}
```

**Error Response** (400 Bad Request - Idempotency Key Conflict):
```json
{
  "status": "error",
  "error_code": "IDEMPOTENCY_KEY_CONFLICT",
  "message": "Idempotency key exists with different value"
}
```

**Error Response** (503 Service Unavailable - Quorum Not Reached):
```json
{
  "status": "error",
  "error_code": "QUORUM_NOT_REACHED",
  "message": "Failed to reach quorum for write operation"
}
```

**Idempotency Behavior**:
- If `Idempotency-Key` is provided and matches a previous request, return the original response (no write performed)
- If `Idempotency-Key` is not provided, server generates a unique idempotency key and returns it in response
- Idempotency keys are scoped per tenant and key combination: `(tenant_id, key, idempotency_key)`
- Idempotency keys have a TTL of 24 hours after which they expire

### 4.2 Retrieve Key-Value Pair

**Endpoint**: `GET /v1/key-value`

**Request Headers**:
```
Authorization: Bearer <tenant_token>
```

**Query Parameters**:
- `key` (required, string): The key to retrieve
- `consistency` (optional, string): Consistency level. One of: "one", "quorum", "all". Default: "quorum"

**Example Request**:
```
GET /v1/key-value?key=user:123&consistency=quorum
```

**Response** (200 OK):
```json
{
  "status": "success",
  "key": "string",
  "value": <opaque object>,
  "vector_clock": {
    "replica_1": 1,
    "replica_2": 1,
    "replica_3": 0
  }
}
```

**Error Response** (404 Not Found):
```json
{
  "status": "error",
  "error_code": "KEY_NOT_FOUND",
  "message": "Key not found"
}
```

**Error Response** (503 Service Unavailable - Quorum Not Reached):
```json
{
  "status": "error",
  "error_code": "QUORUM_NOT_REACHED",
  "message": "Failed to reach quorum for read operation"
}
```

**Note**: GET operations are naturally idempotent. No idempotency key is needed for read operations.

## 5. Consistency Levels

### 5.1 ONE
- Read/write from/to one replica
- Lowest latency
- Eventual consistency
- Use case: High read throughput, eventual consistency acceptable

### 5.2 QUORUM
- Read/write from/to majority of replicas
- Balanced latency and consistency
- Default consistency level
- Use case: General purpose, balanced requirements

### 5.3 ALL
- Read/write from/to all replicas
- Strongest consistency
- Higher latency
- Use case: Critical data requiring strong consistency

## 6. Vector Clock Format

Vector clocks are represented as JSON objects where:
- Keys are replica identifiers (strings)
- Values are logical timestamps (integers)

**Example**:
```json
{
  "replica_1": 5,
  "replica_2": 3,
  "replica_3": 2
}
```

This indicates:
- Replica 1 has seen 5 operations
- Replica 2 has seen 3 operations
- Replica 3 has seen 2 operations

## 7. Error Codes

| Error Code | HTTP Status | Description |
|------------|-------------|-------------|
| INVALID_REQUEST | 400 | Invalid request parameters |
| TENANT_NOT_FOUND | 404 | Tenant does not exist |
| TENANT_EXISTS | 409 | Tenant already exists |
| KEY_NOT_FOUND | 404 | Key does not exist |
| QUORUM_NOT_REACHED | 503 | Failed to reach quorum for operation |
| IDEMPOTENCY_KEY_CONFLICT | 400 | Idempotency key exists with different value |
| INVALID_REPLICATION_FACTOR | 400 | Invalid replication factor value |
| UNAUTHORIZED | 401 | Authentication failed |
| FORBIDDEN | 403 | Authorization failed |
| RATE_LIMIT_EXCEEDED | 429 | Rate limit exceeded |
| INTERNAL_ERROR | 500 | Internal server error |

## 8. Rate Limiting

Rate limiting is applied per tenant. Default limits:
- Write operations: 1000 requests per minute per tenant
- Read operations: 10000 requests per minute per tenant

Rate limit headers are included in responses:
```
X-RateLimit-Limit: 1000
X-RateLimit-Remaining: 999
X-RateLimit-Reset: 1609459200
```

## 9. Request/Response Examples

### 9.1 Store String Value
```bash
curl -X POST https://api.pairdb.com/v1/key-value \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: abc-123-def-456" \
  -d '{
    "key": "user:123:name",
    "value": "John Doe",
    "consistency": "quorum"
  }'
```

### 9.2 Store Object Value
```bash
curl -X POST https://api.pairdb.com/v1/key-value \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{
    "key": "user:123:profile",
    "value": {
      "name": "John Doe",
      "email": "john@example.com",
      "age": 30
    },
    "consistency": "quorum"
  }'
```

### 9.3 Retrieve Value
```bash
curl -X GET "https://api.pairdb.com/v1/key-value?key=user:123:name&consistency=quorum" \
  -H "Authorization: Bearer <token>"
```

### 9.4 Create Tenant
```bash
curl -X POST https://api.pairdb.com/v1/tenants \
  -H "Authorization: Bearer <admin_token>" \
  -H "Content-Type: application/json" \
  -d '{
    "tenant_id": "acme-corp",
    "replication_factor": 3
  }'
```

## 10. Storage Node Management APIs

These APIs are used by system administrators to add or remove storage nodes from the cluster.

### 10.1 Add Storage Node

**Endpoint**: `POST /v1/admin/storage-nodes`

**Request Headers**:
```
Content-Type: application/json
Authorization: Bearer <admin_token>
```

**Request Body**:
```json
{
  "node_id": "storage-node-4",
  "host": "storage-node-4.example.com",
  "port": 8080,
  "virtual_nodes": 150
}
```

**Response** (202 Accepted):
```json
{
  "status": "accepted",
  "node_id": "storage-node-4",
  "migration_id": "migration-12345",
  "message": "Node addition initiated. Migration in progress.",
  "estimated_completion": "2024-01-01T13:00:00Z"
}
```

**Response** (400 Bad Request):
```json
{
  "status": "error",
  "error_code": "INVALID_NODE_CONFIG",
  "message": "Invalid node configuration"
}
```

### 10.2 Remove Storage Node

**Endpoint**: `DELETE /v1/admin/storage-nodes/{node_id}`

**Request Headers**:
```
Authorization: Bearer <admin_token>
```

**Query Parameters**:
- `force` (optional, boolean): Force removal without data migration (default: false)

**Response** (202 Accepted):
```json
{
  "status": "accepted",
  "node_id": "storage-node-3",
  "migration_id": "migration-12346",
  "message": "Node removal initiated. Data migration in progress.",
  "estimated_completion": "2024-01-01T14:00:00Z"
}
```

**Response** (400 Bad Request):
```json
{
  "status": "error",
  "error_code": "NODE_IN_USE",
  "message": "Cannot remove node: insufficient replicas for some tenants"
}
```

### 10.3 Get Migration Status

**Endpoint**: `GET /v1/admin/migrations/{migration_id}`

**Request Headers**:
```
Authorization: Bearer <admin_token>
```

**Response** (200 OK):
```json
{
  "status": "success",
  "migration_id": "migration-12345",
  "type": "node_addition",
  "node_id": "storage-node-4",
  "status": "in_progress",
  "progress": {
    "keys_migrated": 50000,
    "total_keys": 100000,
    "percentage": 50
  },
  "started_at": "2024-01-01T12:00:00Z",
  "estimated_completion": "2024-01-01T13:00:00Z"
}
```

**Migration Status Values**:
- `pending`: Migration queued but not started
- `in_progress`: Migration currently running
- `completed`: Migration successfully completed
- `failed`: Migration failed (requires manual intervention)
- `cancelled`: Migration was cancelled

### 10.4 List Storage Nodes

**Endpoint**: `GET /v1/admin/storage-nodes`

**Request Headers**:
```
Authorization: Bearer <admin_token>
```

**Response** (200 OK):
```json
{
  "status": "success",
  "nodes": [
    {
      "node_id": "storage-node-1",
      "host": "storage-node-1.example.com",
      "port": 8080,
      "status": "active",
      "virtual_nodes": 150,
      "keys_count": 50000,
      "disk_usage_percent": 45
    },
    {
      "node_id": "storage-node-2",
      "host": "storage-node-2.example.com",
      "port": 8080,
      "status": "active",
      "virtual_nodes": 150,
      "keys_count": 52000,
      "disk_usage_percent": 48
    }
  ]
}
```

