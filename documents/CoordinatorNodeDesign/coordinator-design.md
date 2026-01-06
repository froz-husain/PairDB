# Coordinator Service: High-Level Design

## 1. Overview

The Coordinator service is a stateless microservice that handles request routing, consistency coordination, and conflict resolution. It acts as an intermediary between the API Gateway and Storage Nodes, managing the complexity of distributed operations.

## 2. Requirements Summary

### 2.1 Functional Requirements
- Route requests to appropriate storage nodes based on consistent hashing
- Fetch and cache tenant configurations from metadata store
- Coordinate read/write operations based on consistency levels (one, quorum, all)
- Aggregate responses from multiple storage node replicas
- Detect and resolve conflicts using vector clocks
- Handle idempotency key checking and storage
- Generate and update vector clocks for write operations
- Trigger repair operations when conflicts are detected

### 2.2 Non-Functional Requirements
- Stateless design for horizontal scalability
- Low latency request processing
- High availability (99.9% uptime)
- Support decent QPS (thousands of requests per second per coordinator)
- Fast tenant configuration lookups (cached)

## 3. Service Architecture

### 3.1 Service Type
- **Protocol**: HTTP/gRPC microservice
- **Deployment**: Stateless, horizontally scalable
- **Language**: Go, Java, or Python (recommended: Go for performance)
- **Communication**: 
  - With API Gateway: HTTP/REST
  - With Storage Nodes: gRPC or HTTP (for better performance)
  - With Metadata Store: Database client (PostgreSQL)
  - With Idempotency Store: Redis client

### 3.2 Component Responsibilities

#### 3.2.1 Request Handler
- Receive requests from API Gateway
- Extract tenant ID from authentication token
- Validate request parameters
- Route to appropriate handler based on operation type

#### 3.2.2 Tenant Configuration Manager
- Fetch tenant configuration from metadata store
- Cache tenant configurations (with TTL and invalidation)
- Handle cache misses and updates
- Support default configurations for new tenants

#### 3.2.3 Consistent Hashing Router
- Implement consistent hashing algorithm
- Determine replica nodes for a given key (tenant-aware)
- Handle virtual nodes for even distribution
- Support dynamic node addition/removal

#### 3.2.4 Consistency Coordinator
- Determine which replicas to read/write based on consistency level
- For ONE: Single replica
- For QUORUM: Majority of replicas
- For ALL: All replicas
- Wait for required number of responses

#### 3.2.5 Vector Clock Manager
- Generate vector clocks for write operations
- Compare vector clocks from multiple replicas
- Detect conflicts (concurrent vector clocks)
- Update vector clocks based on replica responses

#### 3.2.6 Idempotency Handler
- Check idempotency store for duplicate requests
- Generate idempotency keys if not provided by client
- Store idempotency keys with responses
- Handle idempotency key conflicts

#### 3.2.7 Conflict Resolver
- Detect conflicts using vector clock comparison
- Trigger repair operations when conflicts detected
- Use last-write-wins strategy (simplified for decent scale)
- **Repair Mechanism**: Asynchronous - repairs are triggered in background without blocking the request
  - Conflict detected during read operation
  - Repair request added to repair queue
  - Background worker processes repair queue
  - Repair writes sent to all replicas asynchronously
  - Original read request returns immediately (may return slightly stale data)
- Coordinate repair writes to all replicas
- Repair queue for managing multiple repair operations
- Repair retry logic for failed repairs

#### 3.2.8 Response Aggregator
- Aggregate responses from multiple replicas
- Select best response based on vector clocks
- Handle partial failures (quorum not reached)
- Format response for client

## 4. APIs Offered by Coordinator

### 4.1 Internal APIs (gRPC/HTTP)

The Coordinator exposes internal APIs for communication with Storage Nodes:

#### 4.1.1 Write Request
```
rpc Write(WriteRequest) returns (WriteResponse)
```

**Request**:
```protobuf
message WriteRequest {
  string tenant_id = 1;
  string key = 2;
  bytes value = 3;
  map<string, int64> vector_clock = 4;
  string idempotency_key = 5;
}
```

**Response**:
```protobuf
message WriteResponse {
  bool success = 1;
  map<string, int64> vector_clock = 2;
  string error_message = 3;
}
```

#### 4.1.2 Read Request
```
rpc Read(ReadRequest) returns (ReadResponse)
```

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

#### 4.1.3 Repair Request
```
rpc Repair(RepairRequest) returns (RepairResponse)
```

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

### 4.2 External APIs (via API Gateway)

The Coordinator processes requests forwarded from the API Gateway:
- POST /v1/key-value
- GET /v1/key-value
- POST /v1/tenants
- PUT /v1/tenants/{tenant_id}/replication-factor
- GET /v1/tenants/{tenant_id}

## 5. Data Stores Used

### 5.1 Metadata Store (PostgreSQL)

- **Purpose**: Store tenant configurations
- **Implementation**: Direct database access by coordinator nodes (no separate service for now)
- **Future**: Can be moved to a separate service if needed for better scalability
- **Tables**:
  - `tenants`: tenant_id, replication_factor, created_at, updated_at
- **Access Pattern**: Read-heavy, occasional writes
- **Caching**: Redis cache at coordinator level (TTL: 5 minutes)
- **Connection**: Each coordinator maintains its own PostgreSQL connection pool

### 5.2 Idempotency Store (Redis)
- **Purpose**: Store idempotency keys and responses
- **Key Format**: `idempotency:{tenant_id}:{key}:{idempotency_key}`
- **Value Format**: JSON serialized response
- **TTL**: 24 hours
- **Access Pattern**: Read and write for every write request
- **Operations**: GET, SET, SETNX (for distributed locks)

### 5.3 Connection Pooling
- **Storage Nodes**: Connection pool per storage node (reuse connections)
- **PostgreSQL**: Connection pool for metadata store queries
- **Redis**: Connection pool for idempotency store operations

## 6. Key Algorithms

### 6.1 Consistent Hashing
- Hash function: SHA-256
- Virtual nodes: 100-200 per physical node
- Key format: `hash(tenant_id + key)`
- Replica selection: N consecutive nodes on ring (N = replication factor)

### 6.2 Vector Clock Implementation

#### 6.2.1 Vector Clock Storage Format

Vector clocks are stored as a list of `{coordinator_node_id, logical_timestamp}` pairs:

```go
type VectorClockEntry struct {
    CoordinatorNodeID string
    LogicalTimestamp  int64
}

type VectorClock []VectorClockEntry
```

**Example**:
```go
vectorClock := VectorClock{
    {CoordinatorNodeID: "coord-1", LogicalTimestamp: 5},
    {CoordinatorNodeID: "coord-2", LogicalTimestamp: 3},
    {CoordinatorNodeID: "coord-3", LogicalTimestamp: 2},
}
```

**Storage**:
- Serialized as JSON or binary format
- Stored with each key-value pair
- Transmitted in API requests/responses

#### 6.2.2 Vector Clock Comparison

- Compare vector clocks from multiple replicas
- Determine if clocks are:
  - **Identical**: All entries match
  - **Causal**: One happens before the other (all timestamps in one are <= corresponding timestamps in other)
  - **Concurrent**: Siblings (conflict) - neither happens before the other
- For concurrent clocks: Use last-write-wins (timestamp-based)

**Comparison Algorithm**:
```go
func CompareVectorClocks(vc1, vc2 VectorClock) ComparisonResult {
    // Check if identical
    if areIdentical(vc1, vc2) {
        return Identical
    }
    
    // Check if vc1 happens before vc2
    if happensBefore(vc1, vc2) {
        return Causal // vc1 -> vc2
    }
    
    // Check if vc2 happens before vc1
    if happensBefore(vc2, vc1) {
        return Causal // vc2 -> vc1
    }
    
    // Otherwise, concurrent
    return Concurrent
}
```

### 6.3 Quorum Calculation
- For replication factor N:
  - Quorum = (N / 2) + 1
  - Example: N=3, Quorum=2; N=5, Quorum=3

## 7. Error Handling

### 7.1 Storage Node Failures
- Retry failed requests to other replicas
- If quorum not reached, return error to client
- Log failures for monitoring

### 7.2 Metadata Store Failures
- Use cached tenant configurations (may be stale)
- Return error if cache miss and store unavailable
- Circuit breaker pattern for repeated failures

### 7.3 Idempotency Store Failures
- Graceful degradation: Continue without idempotency (log warning)
- Fallback to vector clocks for conflict detection
- Circuit breaker pattern for repeated failures

## 8. Performance Optimizations

### 8.1 Caching
- **Tenant Configurations**: Redis cache with 5-minute TTL
- **Connection Pools**: Reuse connections to storage nodes
- **Idempotency Lookups**: Fast Redis GET operations

### 8.2 Parallel Operations
- Parallel writes to multiple replicas
- Parallel reads from multiple replicas
- Async I/O for non-blocking operations

### 8.3 Request Batching
- Batch metadata store queries when possible
- Batch idempotency store operations (pipeline)

## 9. Monitoring and Observability

### 9.1 Metrics
- Request rate (QPS) per coordinator
- Latency percentiles (p50, p95, p99)
- Error rates by error type
- Cache hit rates (tenant config, idempotency)
- Vector clock conflict rates
- Repair operation counts

### 9.2 Logging
- Request/response logging (with tenant_id, key)
- Error logging with stack traces
- Performance logging (slow requests)
- Idempotency key collisions (should be rare)

### 9.3 Health Checks
- Liveness probe: Basic health check endpoint
- Readiness probe: Check connections to storage nodes, metadata store, idempotency store

## 10. Scalability Considerations

### 10.1 Horizontal Scaling
- Stateless design allows easy horizontal scaling
- Load balancer distributes requests across coordinators
- No shared state between coordinators

### 10.2 Resource Requirements
- **CPU**: Moderate (request processing, vector clock comparison)
- **Memory**: Low to moderate (caching, connection pools)
- **Network**: High (communication with storage nodes)

### 10.3 Scaling Triggers
- CPU utilization > 70%
- Request latency p95 > 100ms
- Error rate > 1%

## 11. Deployment

### 11.1 Containerization
- Docker container with coordinator service
- Health check endpoints (for Kubernetes liveness/readiness probes)
- Graceful shutdown handling

### 11.2 Orchestration

**Kubernetes Deployment**:
- **Deployment**: Used for coordinators (stateless, horizontally scalable)
  - Multiple replicas for high availability
  - Horizontal Pod Autoscaler (HPA) for automatic scaling
  - Service for load balancing (ClusterIP or LoadBalancer)
- **Service Discovery**: Kubernetes DNS and service discovery for storage nodes
- **Resource Limits**: CPU and memory limits per pod
- **Health Probes**:
  - Liveness probe: Health check endpoint
  - Readiness probe: Check connections to storage nodes, metadata store, idempotency store

### 11.3 Configuration
- **ConfigMaps**: For non-sensitive configuration
  - Metadata store connection string (PostgreSQL - direct DB access)
  - Redis connection string (idempotency store)
  - Storage node endpoints (or service discovery)
  - Cache TTLs (tenant config cache: 5 minutes)
  - Consistency level defaults
  - Gossip protocol subscription settings (for storage node health)
- **Secrets**: For sensitive data
  - Database passwords
  - Redis passwords
  - API keys
- **Environment Variables**: For runtime configuration overrides

## 12. Storage Node Lifecycle Management

### 12.1 Overview

Coordinators manage the consistent hash ring and handle storage node addition/deletion. When nodes are added or removed, data must be migrated to maintain proper replication and distribution.

### 12.2 Node Addition Process

#### 12.2.1 API Call Flow

**Admin calls**: `POST /v1/admin/storage-nodes`

**Process**:
1. **API Gateway** receives request, authenticates admin
2. **Coordinator** (designated leader or admin coordinator) receives request
3. **Coordinator** validates node configuration
4. **Coordinator** adds node to hash ring (with virtual nodes)
5. **Coordinator** initiates data migration process
6. **Coordinator** returns migration ID to admin

#### 12.2.2 Hash Ring Update

```go
// Pseudo-code for hash ring update
func AddNodeToRing(nodeID string, virtualNodes int) {
    // Add virtual nodes to hash ring
    for i := 0; i < virtualNodes; i++ {
        vnodeID := fmt.Sprintf("%s-vnode-%d", nodeID, i)
        hash := consistentHash(vnodeID)
        hashRing.AddNode(hash, nodeID)
    }
    
    // Update coordinator's view of ring
    updateRingState()
    
    // Notify all coordinators (via metadata store or event stream)
    broadcastRingUpdate()
}
```

#### 12.2.3 Data Migration Process

**Phase 1: Identify Keys to Migrate**
- Coordinator scans hash ring to identify keys that should move to new node
- For each key range that maps to new node:
  - Identify current replica nodes
  - Calculate which keys need to be copied

**Phase 2: Dual-Write Phase**
- Coordinator updates routing: writes go to both old and new nodes
- New writes are replicated to new node immediately
- Existing data is copied in background

**Phase 3: Background Data Copy**
- Coordinator reads keys from old nodes
- Coordinator writes keys to new node (using Write API)
- Progress tracked and checkpointed

**Phase 4: Cutover**
- After data copy completes, coordinator switches routing
- New node becomes primary for its key ranges
- Old nodes still receive reads (for consistency)

**Phase 5: Cleanup**
- After verification period, old nodes can be removed from routing
- Old data can be deleted from old nodes (optional)

#### 12.2.4 Request Handling During Node Addition

**Write Requests**:
- Coordinator routes writes to both old and new nodes (dual-write)
- Quorum calculated from both sets of replicas
- Ensures no data loss during migration

**Read Requests**:
- Coordinator can read from either old or new nodes
- Prefers new node if data is available
- Falls back to old node if new node doesn't have data yet

**Example Flow**:
```
Write Request for Key K:
1. Coordinator hashes key K → determines it should be on new node
2. Coordinator checks migration state → "dual-write" phase
3. Coordinator writes to:
   - Old replicas (A, B, C) - for backward compatibility
   - New replicas (D, E, F) - including new node
4. Wait for quorum from either set
5. Return success
```

### 12.3 Node Deletion Process

#### 12.3.1 API Call Flow

**Admin calls**: `DELETE /v1/admin/storage-nodes/{node_id}`

**Process**:
1. **API Gateway** receives request, authenticates admin
2. **Coordinator** validates node can be removed:
   - Check if removal would violate replication factor for any tenant
   - Verify sufficient replicas exist
3. **Coordinator** initiates data migration (copy data away from node)
4. **Coordinator** removes node from hash ring after migration
5. **Coordinator** returns migration ID

#### 12.3.2 Data Migration Process

**Phase 1: Stop Routing to Node**
- Coordinator stops routing new writes to node being removed
- Node marked as "draining" in coordinator state

**Phase 2: Verify Quorum**
- Coordinator verifies remaining nodes have all data
- For each key on node being removed:
  - Verify other replicas have the data
  - If not, trigger replication from node being removed

**Phase 3: Data Replication**
- Coordinator reads keys from node being removed
- Coordinator writes to other replica nodes
- Ensures replication factor maintained

**Phase 4: Remove from Ring**
- After data replicated, remove node from hash ring
- Update coordinator routing tables
- Node can be safely shut down

**Phase 5: Cleanup**
- Node data can be deleted (optional)
- Node removed from service discovery

#### 12.3.3 Request Handling During Node Deletion

**Write Requests**:
- Coordinator stops routing writes to node being removed
- Writes go to remaining replicas only
- Quorum calculated from remaining nodes

**Read Requests**:
- Coordinator stops reading from node being removed
- Reads from remaining replicas only
- If node has unique data, coordinator triggers replication first

### 12.4 Migration State Management

#### 12.4.1 Migration State Storage

Coordinators store migration state in metadata store:

```sql
CREATE TABLE migrations (
    migration_id VARCHAR(255) PRIMARY KEY,
    type VARCHAR(50),  -- 'node_addition' or 'node_deletion'
    node_id VARCHAR(255),
    status VARCHAR(50),  -- 'pending', 'in_progress', 'completed', 'failed'
    progress JSONB,  -- keys_migrated, total_keys, percentage
    started_at TIMESTAMP,
    completed_at TIMESTAMP,
    error_message TEXT
);

CREATE TABLE migration_checkpoints (
    migration_id VARCHAR(255),
    key_range_start VARCHAR(255),
    key_range_end VARCHAR(255),
    last_migrated_key VARCHAR(255),
    PRIMARY KEY (migration_id, key_range_start)
);
```

#### 12.4.2 Coordinator Coordination

**Leader Election**:
- One coordinator acts as migration leader
- Leader coordinates migration process
- Other coordinators follow leader's state

**State Synchronization**:
- Migration state stored in metadata store (PostgreSQL)
- All coordinators read migration state
- Coordinators update routing based on migration phase

**Example Migration State**:
```json
{
  "migration_id": "migration-12345",
  "type": "node_addition",
  "node_id": "storage-node-4",
  "status": "in_progress",
  "phase": "dual-write",
  "progress": {
    "keys_migrated": 50000,
    "total_keys": 100000,
    "percentage": 50
  }
}
```

### 12.5 Hash Ring Management

#### 12.5.1 Ring State Storage

Coordinators maintain hash ring state:

```go
type HashRingState struct {
    Nodes []NodeInfo
    VirtualNodes map[string][]VirtualNode
    LastUpdated time.Time
}

type NodeInfo struct {
    NodeID string
    Host string
    Port int
    Status string  // "active", "draining", "inactive"
    VirtualNodes int
}
```

#### 12.5.2 Ring Update Propagation

**Method 1: Metadata Store (Simple)**
- Ring state stored in PostgreSQL
- Coordinators poll metadata store for updates
- TTL-based cache with invalidation

**Method 2: Event Stream (Advanced)**
- Ring updates published to event stream
- Coordinators subscribe to updates
- Real-time propagation

### 12.6 Internal APIs for Migration

#### 12.6.1 Start Migration

**Endpoint**: Internal API (not exposed to clients)

```protobuf
rpc StartMigration(MigrationRequest) returns (MigrationResponse)

message MigrationRequest {
  string migration_id = 1;
  string type = 2;  // "node_addition" or "node_deletion"
  string node_id = 3;
  map<string, string> config = 4;
}

message MigrationResponse {
  bool success = 1;
  string migration_id = 2;
  string error_message = 3;
}
```

#### 12.6.2 Get Migration Status

**Endpoint**: Internal API

```protobuf
rpc GetMigrationStatus(MigrationStatusRequest) returns (MigrationStatusResponse)

message MigrationStatusRequest {
  string migration_id = 1;
}

message MigrationStatusResponse {
  string status = 1;
  int64 keys_migrated = 2;
  int64 total_keys = 3;
  float percentage = 4;
  string error_message = 5;
}
```

### 12.7 Error Handling

#### 12.7.1 Migration Failures

**Failure Scenarios**:
- Node fails during migration
- Network partition during migration
- Insufficient disk space on new node
- Data corruption detected

**Recovery**:
- Migration can be resumed from last checkpoint
- Failed migrations marked as "failed" in metadata store
- Manual intervention required for some failures
- Rollback capability for node additions

#### 12.7.2 Partial Migrations

**Handling**:
- Checkpoint progress regularly
- Resume from last checkpoint on restart
- Verify data integrity after migration
- Retry failed key migrations

### 12.8 Performance Considerations

#### 12.8.1 Migration Throttling

- Limit migration bandwidth to avoid impacting normal operations
- Prioritize frequently accessed keys
- Background migration during low traffic periods

#### 12.8.2 Request Impact

- Minimal impact during dual-write phase (slightly higher latency)
- No impact during background copy phase
- Brief impact during cutover (coordinator state update)

