# API Gateway: High-Level Design

## 1. Overview

The API Gateway is the entry point for all client requests to pairDB. It is a stateless HTTP server that exposes REST APIs to clients and forwards requests to the Coordinator service via gRPC. For simplicity, authentication and authorization are handled outside the gateway (e.g., by an external service mesh or load balancer).

## 2. Requirements Summary

### 2.1 Functional Requirements
- Expose REST APIs for key-value operations (POST, GET, DELETE)
- Expose REST APIs for tenant management (POST, PUT, GET)
- Convert HTTP/REST requests to gRPC requests for Coordinator service
- Handle HTTP response formatting from gRPC responses
- Load balance requests across multiple Coordinator instances
- Handle request/response transformation (HTTP ↔ gRPC)

### 2.2 Non-Functional Requirements
- Stateless and horizontally scalable
- Low latency request forwarding
- High availability
- Support decent QPS (thousands to tens of thousands per instance)
- Graceful error handling and response formatting

## 3. Service Architecture

### 3.1 Service Type
- **Protocol**: HTTP server exposing REST APIs
- **Deployment**: Stateless, horizontally scalable
- **Language**: Go (custom HTTP server)
- **Network Communication**:
  - **Receives from Clients**: HTTP/REST requests
  - **Sends to Coordinators**: gRPC requests
  - **Load Balancing**: Round-robin or least-connections across coordinator instances

### 3.2 Component Responsibilities

#### 3.2.1 HTTP Server
- Listen on HTTP port (e.g., 8080)
- Handle incoming REST API requests
- Parse HTTP request bodies and query parameters
- Format HTTP responses

#### 3.2.2 Request Router
- Route requests to appropriate handlers based on HTTP method and path
- Extract path parameters (e.g., tenant_id, key)
- Extract query parameters (e.g., consistency level)
- Extract headers (e.g., Idempotency-Key)

#### 3.2.3 gRPC Client
- Maintain gRPC connections to Coordinator service instances
- Connection pooling for efficient resource usage
- Load balancing across coordinator instances
- Handle gRPC request/response conversion

#### 3.2.4 Response Formatter
- Convert gRPC responses to HTTP JSON responses
- Format error responses with appropriate HTTP status codes
- Add standard HTTP headers

#### 3.2.5 Error Handler
- Handle gRPC errors and convert to HTTP status codes
- Format error responses consistently
- Log errors for observability

## 4. APIs Exposed

The API Gateway exposes the following REST endpoints:

### 4.1 Key-Value Operations

#### 4.1.1 Write Key-Value
- **Endpoint**: `POST /v1/key-value`
- **Request**: JSON body with `key`, `value`, optional `consistency`
- **Response**: JSON with `status`, `key`, `idempotency_key`, `vector_clock`, `replica_count`, `consistency`, `is_duplicate`
- **gRPC Call**: `WriteKeyValue(WriteKeyValueRequest) returns (WriteKeyValueResponse)`

#### 4.1.2 Read Key-Value
- **Endpoint**: `GET /v1/key-value?key={key}&consistency={consistency}`
- **Request**: Query parameters `key` (required), `consistency` (optional)
- **Response**: JSON with `status`, `key`, `value`, `vector_clock`
- **gRPC Call**: `ReadKeyValue(ReadKeyValueRequest) returns (ReadKeyValueResponse)`

#### 4.1.3 Delete Key-Value
- **Endpoint**: `DELETE /v1/key-value?key={key}&consistency={consistency}`
- **Request**: Query parameters `key` (required), `consistency` (optional), Header `X-Vector-Clock` (optional, for conditional deletes)
- **Response**: JSON with `status`, `key`, `vector_clock` (of the tombstone), `replica_count`
- **gRPC Call**: `DeleteKeyValue(DeleteKeyValueRequest) returns (DeleteKeyValueResponse)`
- **Behavior**:
  - Creates a tombstone marker across replicas (soft delete)
  - Respects consistency level (one, quorum, all)
  - Returns success after required replicas acknowledge the delete
  - Tombstones are cleaned up during compaction
- **Error Codes**:
  - `404 Not Found`: Key does not exist (optional, may return 200 for idempotent behavior)
  - `400 Bad Request`: Invalid key format or consistency level
  - `503 Service Unavailable`: Cannot reach quorum of replicas

### 4.2 Tenant Management Operations

#### 4.2.1 Create Tenant
- **Endpoint**: `POST /v1/tenants`
- **Request**: JSON body with `tenant_id`, optional `replication_factor`
- **Response**: JSON with `status`, `tenant_id`, `replication_factor`, `created_at`
- **gRPC Call**: `CreateTenant(CreateTenantRequest) returns (CreateTenantResponse)`

#### 4.2.2 Update Replication Factor
- **Endpoint**: `PUT /v1/tenants/{tenant_id}/replication-factor`
- **Request**: JSON body with `replication_factor`
- **Response**: JSON with `status`, `tenant_id`, `old_replication_factor`, `new_replication_factor`, `updated_at`
- **gRPC Call**: `UpdateReplicationFactor(UpdateReplicationFactorRequest) returns (UpdateReplicationFactorResponse)`

#### 4.2.3 Get Tenant
- **Endpoint**: `GET /v1/tenants/{tenant_id}`
- **Request**: Path parameter `tenant_id`
- **Response**: JSON with `status`, `tenant_id`, `replication_factor`, `created_at`, `updated_at`
- **gRPC Call**: `GetTenant(GetTenantRequest) returns (GetTenantResponse)`

### 4.3 Storage Node Management Operations (Admin)

#### 4.3.1 Add Storage Node
- **Endpoint**: `POST /v1/admin/storage-nodes`
- **Request**: JSON body with `node_id`, `host`, `port`, optional `virtual_nodes`
- **Response**: JSON with `status`, `node_id`, `migration_id`, `message`, `estimated_completion`
- **gRPC Call**: `AddStorageNode(AddStorageNodeRequest) returns (AddStorageNodeResponse)`

#### 4.3.2 Remove Storage Node
- **Endpoint**: `DELETE /v1/admin/storage-nodes/{node_id}`
- **Request**: Path parameter `node_id`
- **Response**: JSON with `status`, `node_id`, `migration_id`, `message`, `estimated_completion`
- **gRPC Call**: `RemoveStorageNode(RemoveStorageNodeRequest) returns (RemoveStorageNodeResponse)`

#### 4.3.3 Get Migration Status
- **Endpoint**: `GET /v1/admin/migrations/{migration_id}`
- **Request**: Path parameter `migration_id`
- **Response**: JSON with migration status details
- **gRPC Call**: `GetMigrationStatus(GetMigrationStatusRequest) returns (GetMigrationStatusResponse)`

#### 4.3.4 List Storage Nodes
- **Endpoint**: `GET /v1/admin/storage-nodes`
- **Request**: None
- **Response**: JSON with list of storage nodes
- **gRPC Call**: `ListStorageNodes(ListStorageNodesRequest) returns (ListStorageNodesResponse)`

## 5. Request Flow

### 5.1 Write Request Flow

```
Client → API Gateway (HTTP POST /v1/key-value)
    ↓
Parse HTTP request (body, headers)
    ↓
Extract: key, value, consistency, idempotency_key (from header)
    ↓
Convert to gRPC WriteKeyValueRequest
    ↓
Load balance and forward to Coordinator (gRPC)
    ↓
Receive gRPC WriteKeyValueResponse
    ↓
Convert to HTTP JSON response
    ↓
Return HTTP 200 OK with JSON body
```

### 5.2 Read Request Flow

```
Client → API Gateway (HTTP GET /v1/key-value?key=...&consistency=...)
    ↓
Parse HTTP request (query params)
    ↓
Extract: key, consistency
    ↓
Convert to gRPC ReadKeyValueRequest
    ↓
Load balance and forward to Coordinator (gRPC)
    ↓
Receive gRPC ReadKeyValueResponse
    ↓
Convert to HTTP JSON response
    ↓
Return HTTP 200 OK with JSON body
```

## 6. Error Handling

### 6.1 gRPC Error Mapping

The API Gateway maps gRPC errors to HTTP status codes:

- **gRPC OK (0)**: HTTP 200 OK
- **gRPC InvalidArgument**: HTTP 400 Bad Request
- **gRPC NotFound**: HTTP 404 Not Found
- **gRPC AlreadyExists**: HTTP 409 Conflict
- **gRPC Unauthenticated**: HTTP 401 Unauthorized (handled externally)
- **gRPC PermissionDenied**: HTTP 403 Forbidden (handled externally)
- **gRPC Unavailable**: HTTP 503 Service Unavailable
- **gRPC Internal**: HTTP 500 Internal Server Error
- **gRPC DeadlineExceeded**: HTTP 504 Gateway Timeout

### 6.2 Error Response Format

All error responses follow this format:

```json
{
  "status": "error",
  "error_code": "ERROR_CODE",
  "message": "Human-readable error message"
}
```

## 7. Load Balancing

### 7.1 Coordinator Instance Discovery

- **Service Discovery**: Kubernetes DNS or service discovery mechanism
- **Endpoint Format**: `coordinator-service:50051` (gRPC default port)
- **Multiple Instances**: Kubernetes Service provides load balancing

### 7.2 Load Balancing Strategy

- **Round-Robin**: Default strategy for distributing requests
- **Connection Pooling**: Reuse gRPC connections for efficiency
- **Health Checks**: Exclude unhealthy coordinator instances

## 8. Technology Stack

### 8.1 Language and Framework
- **Language**: Go
- **HTTP Server**: `net/http` (standard library) or `gorilla/mux` for routing
- **gRPC Client**: `google.golang.org/grpc`

### 8.2 Dependencies
- **gRPC**: For communication with Coordinator service
- **Protocol Buffers**: For request/response serialization
- **JSON**: For HTTP request/response formatting

## 9. Deployment

### 9.1 Containerization
- Docker container with API Gateway service
- Expose HTTP port (e.g., 8080)
- Health check endpoint for Kubernetes probes

### 9.2 Kubernetes Deployment
- **Deployment**: Stateless deployment (can scale horizontally)
- **Service**: ClusterIP or LoadBalancer for external access
- **Ingress**: Optional ingress controller for external routing
- **HPA**: Horizontal Pod Autoscaler based on CPU/memory/request rate

### 9.3 Configuration
- **ConfigMaps**: For coordinator service endpoints
- **Environment Variables**: For runtime configuration (port, timeouts, etc.)

## 10. Monitoring and Observability

### 10.1 Metrics
- Request rate (QPS) per endpoint
- Latency percentiles (p50, p95, p99)
- Error rates by error type
- gRPC connection pool metrics
- Request/response sizes

### 10.2 Logging
- Request/response logging (with request IDs)
- Error logging with stack traces
- gRPC call logging

### 10.3 Health Checks
- **Liveness Probe**: HTTP endpoint `/health` (always returns 200 if process is running)
- **Readiness Probe**: HTTP endpoint `/ready` (returns 200 if can connect to coordinators)

## 11. Scalability Considerations

### 11.1 Horizontal Scaling
- Stateless design allows horizontal scaling
- Multiple instances behind load balancer
- No shared state between instances

### 11.2 Performance Optimizations
- Connection pooling for gRPC clients
- Request/response buffering
- Efficient JSON serialization/deserialization
- Minimal request processing overhead

## 12. Design Decisions

1. **Stateless Design**: Enables horizontal scaling without shared state
2. **HTTP to gRPC Conversion**: Simplifies client integration while maintaining efficient backend communication
3. **No Authentication/Authorization in Gateway**: Handled externally for simplicity
4. **Round-Robin Load Balancing**: Simple and effective for stateless coordinators
5. **Standard HTTP Status Codes**: Familiar to clients and follows REST conventions
6. **JSON Request/Response Format**: Easy to use and widely supported

