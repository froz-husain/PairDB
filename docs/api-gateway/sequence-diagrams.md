# API Gateway: Sequence Diagrams

This document provides sequence diagrams for all major flows supported by the API Gateway service.

## 1. Write Key-Value Flow

```mermaid
sequenceDiagram
    participant Client
    participant API Gateway
    participant Middleware
    participant Handler
    participant Converter
    participant CoordinatorClient
    participant Coordinator
    
    Client->>API Gateway: POST /v1/key-value
    API Gateway->>Middleware: Request
    Middleware->>Middleware: Authenticate & Extract Tenant
    Middleware->>Middleware: Rate Limit Check
    Middleware->>Handler: Forward Request
    Handler->>Converter: Convert HTTP to gRPC
    Converter->>Converter: Parse JSON Body
    Converter->>Converter: Extract Headers
    Converter-->>Handler: WriteKeyValueRequest
    Handler->>CoordinatorClient: WriteKeyValue(ctx, req)
    CoordinatorClient->>Coordinator: gRPC Call
    Coordinator-->>CoordinatorClient: WriteKeyValueResponse
    CoordinatorClient-->>Handler: Response
    Handler->>Converter: Convert gRPC to HTTP
    Converter->>Converter: Format JSON Response
    Converter-->>Handler: HTTP Response
    Handler->>API Gateway: Write Response
    API Gateway-->>Client: 200 OK with JSON
```

## 2. Read Key-Value Flow

```mermaid
sequenceDiagram
    participant Client
    participant API Gateway
    participant Middleware
    participant Handler
    participant Converter
    participant CoordinatorClient
    participant Coordinator
    
    Client->>API Gateway: GET /v1/key-value?key=...
    API Gateway->>Middleware: Request
    Middleware->>Middleware: Authenticate & Extract Tenant
    Middleware->>Middleware: Rate Limit Check
    Middleware->>Handler: Forward Request
    Handler->>Converter: Convert HTTP to gRPC
    Converter->>Converter: Parse Query Params
    Converter-->>Handler: ReadKeyValueRequest
    Handler->>CoordinatorClient: ReadKeyValue(ctx, req)
    CoordinatorClient->>Coordinator: gRPC Call
    Coordinator-->>CoordinatorClient: ReadKeyValueResponse
    CoordinatorClient-->>Handler: Response
    Handler->>Converter: Convert gRPC to HTTP
    Converter->>Converter: Format JSON Response
    Converter-->>Handler: HTTP Response
    Handler->>API Gateway: Write Response
    API Gateway-->>Client: 200 OK with JSON
```

## 3. Delete Key-Value Flow

```mermaid
sequenceDiagram
    participant Client
    participant API Gateway
    participant Middleware
    participant Handler
    participant Converter
    participant CoordinatorClient
    participant Coordinator

    Client->>API Gateway: DELETE /v1/key-value?key=...
    API Gateway->>Middleware: Request
    Middleware->>Middleware: Authenticate & Extract Tenant
    Middleware->>Middleware: Rate Limit Check
    Middleware->>Handler: Forward Request
    Handler->>Converter: Convert HTTP to gRPC
    Converter->>Converter: Parse Query Params
    Converter->>Converter: Extract X-Vector-Clock Header (optional)
    Converter-->>Handler: DeleteKeyValueRequest
    Handler->>CoordinatorClient: DeleteKeyValue(ctx, req)
    CoordinatorClient->>Coordinator: gRPC Call
    Note over Coordinator: 1. Fetch tenant config<br/>2. Get current vector clock<br/>3. Write tombstone to replicas<br/>4. Wait for consistency level
    Coordinator-->>CoordinatorClient: DeleteKeyValueResponse
    CoordinatorClient-->>Handler: Response
    Handler->>Converter: Convert gRPC to HTTP
    Converter->>Converter: Format JSON Response
    Converter-->>Handler: HTTP Response
    Handler->>API Gateway: Write Response
    API Gateway-->>Client: 200 OK with JSON
```

## 4. Create Tenant Flow

```mermaid
sequenceDiagram
    participant Client
    participant API Gateway
    participant Middleware
    participant Handler
    participant Converter
    participant CoordinatorClient
    participant Coordinator
    
    Client->>API Gateway: POST /v1/tenants
    API Gateway->>Middleware: Request
    Middleware->>Middleware: Authenticate (Admin)
    Middleware->>Middleware: Rate Limit Check
    Middleware->>Handler: Forward Request
    Handler->>Converter: Convert HTTP to gRPC
    Converter->>Converter: Parse JSON Body
    Converter-->>Handler: CreateTenantRequest
    Handler->>CoordinatorClient: CreateTenant(ctx, req)
    CoordinatorClient->>Coordinator: gRPC Call
    Coordinator-->>CoordinatorClient: CreateTenantResponse
    CoordinatorClient-->>Handler: Response
    Handler->>Converter: Convert gRPC to HTTP
    Converter-->>Handler: HTTP Response
    Handler->>API Gateway: Write Response
    API Gateway-->>Client: 200 OK with JSON
```

## 4. Update Replication Factor Flow

```mermaid
sequenceDiagram
    participant Client
    participant API Gateway
    participant Middleware
    participant Handler
    participant Converter
    participant CoordinatorClient
    participant Coordinator
    
    Client->>API Gateway: PUT /v1/tenants/{id}/replication-factor
    API Gateway->>Middleware: Request
    Middleware->>Middleware: Authenticate
    Middleware->>Handler: Forward Request
    Handler->>Converter: Convert HTTP to gRPC
    Converter->>Converter: Parse Path & Body
    Converter-->>Handler: UpdateReplicationFactorRequest
    Handler->>CoordinatorClient: UpdateReplicationFactor(ctx, req)
    CoordinatorClient->>Coordinator: gRPC Call
    Coordinator-->>CoordinatorClient: UpdateReplicationFactorResponse
    CoordinatorClient-->>Handler: Response
    Handler->>Converter: Convert gRPC to HTTP
    Converter-->>Handler: HTTP Response
    Handler->>API Gateway: Write Response
    API Gateway-->>Client: 200 OK with JSON
```

## 5. Add Storage Node Flow

```mermaid
sequenceDiagram
    participant Admin
    participant API Gateway
    participant Middleware
    participant Handler
    participant Converter
    participant CoordinatorClient
    participant Coordinator
    
    Admin->>API Gateway: POST /v1/admin/storage-nodes
    API Gateway->>Middleware: Request
    Middleware->>Middleware: Authenticate (Admin)
    Middleware->>Handler: Forward Request
    Handler->>Converter: Convert HTTP to gRPC
    Converter->>Converter: Parse JSON Body
    Converter-->>Handler: AddStorageNodeRequest
    Handler->>CoordinatorClient: AddStorageNode(ctx, req)
    CoordinatorClient->>Coordinator: gRPC Call
    Coordinator-->>CoordinatorClient: AddStorageNodeResponse
    CoordinatorClient-->>Handler: Response
    Handler->>Converter: Convert gRPC to HTTP
    Converter-->>Handler: HTTP Response
    Handler->>API Gateway: Write Response
    API Gateway-->>Admin: 202 Accepted with Migration ID
```

## 6. Error Handling Flow

```mermaid
sequenceDiagram
    participant Client
    participant API Gateway
    participant Handler
    participant ErrorHandler
    participant CoordinatorClient
    participant Coordinator
    
    Client->>API Gateway: Request
    API Gateway->>Handler: Forward
    Handler->>CoordinatorClient: gRPC Call
    CoordinatorClient->>Coordinator: Request
    Coordinator-->>CoordinatorClient: gRPC Error
    CoordinatorClient-->>Handler: Error
    Handler->>ErrorHandler: HandleError(err)
    ErrorHandler->>ErrorHandler: Map gRPC to HTTP Status
    ErrorHandler->>ErrorHandler: Format Error Response
    ErrorHandler-->>Handler: Error Response
    Handler->>API Gateway: Write Error Response
    API Gateway-->>Client: HTTP Error (4xx/5xx)
```

## 7. Health Check Flow

```mermaid
sequenceDiagram
    participant K8s
    participant API Gateway
    participant HealthHandler
    participant CoordinatorClient
    participant Coordinator
    
    K8s->>API Gateway: GET /health
    API Gateway->>HealthHandler: HealthHandler()
    HealthHandler->>API Gateway: 200 OK
    API Gateway-->>K8s: Healthy
    
    K8s->>API Gateway: GET /ready
    API Gateway->>HealthHandler: ReadinessHandler()
    HealthHandler->>CoordinatorClient: HealthCheck()
    CoordinatorClient->>Coordinator: gRPC Health Check
    Coordinator-->>CoordinatorClient: OK
    CoordinatorClient-->>HealthHandler: OK
    HealthHandler->>API Gateway: 200 OK
    API Gateway-->>K8s: Ready
```

## 8. Idempotency Key Handling Flow

```mermaid
sequenceDiagram
    participant Client
    participant API Gateway
    participant Handler
    participant Converter
    participant CoordinatorClient
    participant Coordinator
    
    Client->>API Gateway: POST /v1/key-value<br/>Idempotency-Key: abc-123
    API Gateway->>Handler: Request
    Handler->>Converter: Convert HTTP to gRPC
    Converter->>Converter: Extract Idempotency-Key Header
    Converter-->>Handler: Request with IdempotencyKey
    Handler->>CoordinatorClient: WriteKeyValue(ctx, req)
    CoordinatorClient->>Coordinator: gRPC Call with IdempotencyKey
    Coordinator->>Coordinator: Check Idempotency Store
    alt Idempotency Key Found
        Coordinator-->>CoordinatorClient: Cached Response (IsDuplicate=true)
    else Idempotency Key Not Found
        Coordinator->>Coordinator: Execute Write
        Coordinator-->>CoordinatorClient: New Response
    end
    CoordinatorClient-->>Handler: Response
    Handler->>Converter: Convert gRPC to HTTP
    Converter-->>Handler: HTTP Response
    Handler->>API Gateway: Write Response
    API Gateway-->>Client: 200 OK (with is_duplicate flag)
```

## Flow Descriptions

### Write Key-Value Flow
1. Client sends HTTP POST request with JSON body
2. Middleware authenticates and extracts tenant ID
3. Rate limiter checks if request is within limits
4. Handler converts HTTP request to gRPC request
5. Coordinator client makes gRPC call to Coordinator
6. Response is converted back to HTTP JSON
7. Response sent to client

### Read Key-Value Flow
1. Client sends HTTP GET request with query parameters
2. Similar flow to write, but reads from coordinator
3. Returns key-value pair with vector clock

### Delete Key-Value Flow
1. Client sends HTTP DELETE request with query parameters
2. Middleware authenticates and extracts tenant ID
3. Handler extracts optional vector clock from X-Vector-Clock header
4. Coordinator fetches tenant configuration from metadata store
5. Coordinator reads current value to get latest vector clock (if not provided)
6. Coordinator increments vector clock for delete operation
7. Coordinator writes tombstone markers to replica nodes based on consistency level
8. Waits for acknowledgments per consistency level (one/quorum/all)
9. Returns success response with tombstone vector clock
10. Tombstones eventually removed during compaction

### Create Tenant Flow
1. Admin client creates new tenant
2. Requires admin authentication
3. Coordinator creates tenant in metadata store

### Update Replication Factor Flow
1. Updates tenant's replication factor
2. May trigger data migration
3. Returns updated tenant configuration

### Add Storage Node Flow
1. Admin adds new storage node
2. Coordinator initiates migration
3. Returns migration ID for tracking

### Error Handling Flow
1. Errors from coordinator are caught
2. Error handler maps gRPC errors to HTTP status codes
3. Consistent error response format returned

### Health Check Flow
1. Kubernetes probes health endpoint
2. Readiness check verifies coordinator connectivity
3. Returns appropriate status codes

### Idempotency Key Handling Flow
1. Client provides idempotency key in header
2. Coordinator checks idempotency store
3. Returns cached response if duplicate, otherwise processes request

