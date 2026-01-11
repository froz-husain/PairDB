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
- **Protocol**: gRPC service (server)
- **Deployment**: Stateless, horizontally scalable
- **Language**: Go, Java, or Python (recommended: Go for performance)
- **Network Communication**:
  - **Receives from API Gateway**: gRPC requests (API Gateway converts HTTP to gRPC)
  - **Sends to Storage Nodes**: gRPC requests
  - **Metadata Store**: Direct PostgreSQL database client connection
  - **Idempotency Store**: Direct Redis client connection

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

## 4. Core Entities

### 4.1 Tenant Configuration
```go
type TenantConfig struct {
    TenantID         string
    ReplicationFactor int
    CreatedAt        time.Time
    UpdatedAt        time.Time
    Version          int64  // For optimistic locking
}
```

### 4.2 Vector Clock
```go
type VectorClockEntry struct {
    CoordinatorNodeID string
    LogicalTimestamp   int64
}

type VectorClock []VectorClockEntry
```

### 4.3 Hash Ring State
```go
type HashRingState struct {
    Nodes        []NodeInfo
    VirtualNodes map[string][]VirtualNode
    LastUpdated  time.Time
}

type NodeInfo struct {
    NodeID       string
    Host         string
    Port         int
    Status       string  // "active", "draining", "inactive"
    VirtualNodes int
}

type VirtualNode struct {
    VNodeID string
    Hash    uint64
    NodeID  string
}
```

### 4.4 Migration State
```go
type MigrationState struct {
    MigrationID  string
    Type         string  // "node_addition" or "node_deletion"
    NodeID       string
    Status       string  // "pending", "in_progress", "completed", "failed"
    Phase        string  // "dual-write", "data-copy", "cutover", "cleanup"
    Progress     MigrationProgress
    StartedAt    time.Time
    CompletedAt  *time.Time
}

type MigrationProgress struct {
    KeysMigrated int64
    TotalKeys    int64
    Percentage   float64
}
```

### 4.5 Repair Request
```go
type RepairRequest struct {
    TenantID    string
    Key         string
    Value       []byte
    VectorClock VectorClock
    Timestamp   int64
}
```

### 4.6 Idempotency Key
```go
type IdempotencyKey struct {
    TenantID       string
    Key            string
    IdempotencyKey string
    Response       []byte  // Serialized response
    Timestamp      time.Time
    TTL            time.Duration
}
```

## 5. APIs Offered by Coordinator

**Note**: Detailed API contracts are documented in `api-contracts.md` in this directory.

### 5.1 gRPC Service APIs (Internal)

The Coordinator exposes gRPC service APIs for communication with Storage Nodes. See `api-contracts.md` for complete protobuf definitions.

**Key gRPC Methods**:
- `Write(WriteRequest) returns (WriteResponse)` - Write key-value pair to storage node
- `Read(ReadRequest) returns (ReadResponse)` - Read key-value pair from storage node
- `Repair(RepairRequest) returns (RepairResponse)` - Repair conflicting data on storage node

### 5.2 External APIs (via API Gateway)

The Coordinator processes gRPC requests forwarded from the API Gateway (which converts HTTP to gRPC):
- POST /v1/key-value → gRPC Write
- GET /v1/key-value → gRPC Read
- POST /v1/tenants → Tenant creation
- PUT /v1/tenants/{tenant_id}/replication-factor → Replication factor update
- GET /v1/tenants/{tenant_id} → Tenant configuration retrieval

## 6. Data Stores Used

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

## 7. Key Algorithms

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

## 8. Error Handling

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

## 9. Performance Optimizations

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

## 10. Monitoring and Observability

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

## 11. Scalability Considerations

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

## 12. Deployment

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

## 13. Hinted Handoff System (Phase 6)

### 13.1 Overview

Hinted handoff ensures write durability during node failures by storing missed writes as "hints" and replaying them when the node recovers. This prevents data loss during temporary node unavailability.

### 13.2 Core Components

#### 13.2.1 HintStore Interface

```go
type HintStore interface {
    StoreHint(ctx context.Context, hint *model.Hint) error
    GetHintsForNode(ctx context.Context, targetNodeID string, limit int) ([]*model.Hint, error)
    DeleteHint(ctx context.Context, hintID string) error
    CleanupOldHints(ctx context.Context, ttl time.Duration) (int64, error)
}
```

**Implementation**: PostgreSQL-backed storage (`PostgreSQLHintStore`)

#### 13.2.2 Hint Model

```go
type Hint struct {
    HintID       string
    TargetNodeID string    // Node that missed the write
    TenantID     string
    Key          string
    Value        []byte
    VectorClock  string
    Timestamp    time.Time
    CreatedAt    time.Time
    ReplayCount  int       // Number of replay attempts (max 3)
}
```

### 13.3 Hint Creation Flow

Hints are created automatically during write operations when a replica fails:

1. Coordinator attempts write to all replicas
2. If write fails to a replica (network error, node down, timeout):
   - Create hint with write details
   - Store hint in PostgreSQL hints table
   - Continue with write if quorum is still reached
3. Hint is queued for replay when node recovers

**Key Point**: Hints do NOT block the write operation. If quorum is reached from other replicas, the write succeeds and the hint is replayed asynchronously.

### 13.4 Hint Replay Service

#### 13.4.1 Replay Triggers

Hints are replayed in two scenarios:

1. **During Bootstrap**: Before a node transitions to NORMAL state
   - Node completes data streaming
   - HintReplayService replays all hints for the node
   - Only after successful replay does node become NORMAL
   - If replay fails, bootstrap fails safely

2. **Periodic Replay**: Background process checks for hints periodically
   - Useful for nodes that go down and recover without bootstrap
   - Not yet implemented (future enhancement)

#### 13.4.2 Replay Configuration

```go
type HintReplayService struct {
    batchSize      int           // 100 hints per batch
    replayInterval time.Duration // 1 second between batches
    maxRetries     int           // 3 retry attempts per hint
}
```

#### 13.4.3 Replay Algorithm

```
For each hint in batch:
1. Attempt to write hint to target node
2. If successful:
   - Delete hint from store
   - Increment replayed counter
3. If failed:
   - Increment replay_count
   - If replay_count >= maxRetries (3):
     - Delete hint (give up after 3 attempts)
   - Otherwise:
     - Leave hint for next batch
4. Wait 1 second between batches (rate limiting)
```

### 13.5 Hint Cleanup

#### 13.5.1 Background Cleanup

A background goroutine runs every hour to clean up old hints:

```go
func (s *HintReplayService) CleanupOldHints(ctx context.Context, ttl time.Duration)
```

**TTL**: 7 days (configurable)

**Cleanup Criteria**:
- Hints older than TTL
- Hints with replay_count >= 3 (max retries exceeded)

#### 13.5.2 Manual Cleanup

Administrators can manually clean up hints for dead nodes:

```sql
DELETE FROM hints WHERE target_node_id = '<dead-node-id>';
```

### 13.6 Database Schema

```sql
CREATE TABLE hints (
    hint_id VARCHAR(255) PRIMARY KEY,
    target_node_id VARCHAR(255) NOT NULL,
    tenant_id VARCHAR(255) NOT NULL,
    key VARCHAR(255) NOT NULL,
    value BYTEA NOT NULL,
    vector_clock TEXT,
    timestamp TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    replay_count INT DEFAULT 0,
    INDEX idx_hints_target_node (target_node_id),
    INDEX idx_hints_created_at (created_at)
);
```

## 14. Cleanup Safety System (Phase 7)

### 14.1 Overview

Cleanup safety prevents premature data deletion after topology changes by enforcing grace periods and quorum verification before any data cleanup.

### 14.2 Core Components

#### 14.2.1 CleanupService

```go
type CleanupService struct {
    gracePeriod time.Duration // 24 hours default
}
```

**Key Methods**:
- `VerifyCleanupSafe()` - Checks if cleanup is safe
- `ExecuteCleanup()` - Executes cleanup after verification
- `ForceCleanup()` - Emergency override (bypasses safety checks)

### 14.3 Safety Checks

Before any data cleanup, the following checks must pass:

#### Check 1: Change Completed
- PendingChange status must be "completed"
- Ensures topology change finished successfully

#### Check 2: No Active Streaming
- All streaming operations must be in "completed" state
- Prevents cleanup while data is still being transferred

#### Check 3: Grace Period Elapsed
- Default: 24 hours (configurable)
- Calculated from `CompletedAt` timestamp (not `LastUpdated`)
- Provides safety window for detecting issues

#### Check 4: Quorum Verification
- For each affected token range:
  - Get current replicas for the range
  - Verify quorum of replicas have the data
  - Quorum = (RF / 2) + 1
- Ensures data is safely replicated before cleanup

### 14.4 CompletedAt Tracking

The `PendingChange` model includes a `CompletedAt` field for accurate grace period tracking:

```go
type PendingChange struct {
    ChangeID      string
    Type          string
    Status        PendingChangeStatus
    LastUpdated   time.Time
    CompletedAt   time.Time  // Set when status → "completed"
    // ...
}
```

**Why CompletedAt is Important**:
- `LastUpdated` changes during progress updates
- `CompletedAt` is set once when change completes
- Grace period calculated from `CompletedAt` for precision

### 14.5 Cleanup Flow

```
1. Topology change completes
2. NodeHandlerV2 sets CompletedAt timestamp
3. NodeHandlerV2 calls CleanupService.ScheduleCleanup()
4. CleanupService waits for grace period (24 hours)
5. After grace period:
   - VerifyCleanupSafe() runs all safety checks
   - If all checks pass: ExecuteCleanup()
   - If any check fails: Log error, skip cleanup
6. After cleanup: Delete PendingChange record
```

### 14.6 Force Cleanup (Emergency)

For emergency situations, administrators can force cleanup:

```go
func (s *CleanupService) ForceCleanup(ctx context.Context, changeID string, reason string) error
```

**WARNING**: Bypasses all safety checks. Use only when:
- Node is permanently dead
- Data loss is acceptable
- Manual verification confirms safety

All force cleanup operations are logged with reason.

## 15. Concurrent Node Operations (Phase 8)

### 15.1 Overview

Phase 8 enables multiple nodes to bootstrap or decommission simultaneously by replacing coarse-grained global locking with per-node locks.

### 15.2 Per-Node Locking

#### 15.2.1 Previous Design (Phases 1-7)

```go
// OLD: Coarse-grained global lock
type NodeHandlerV2 struct {
    topologyMu sync.Mutex  // Blocks ALL operations
}

func (h *NodeHandlerV2) AddStorageNodeV2() {
    h.topologyMu.Lock()    // Blocks all other adds/removes
    defer h.topologyMu.Unlock()
    // ... bootstrap logic
}
```

**Problem**: Only ONE node operation at a time, even for different nodes.

#### 15.2.2 New Design (Phase 8)

```go
// NEW: Per-node locking
type NodeHandlerV2 struct {
    nodeLocks sync.Map  // map[string]*sync.Mutex
}

func (h *NodeHandlerV2) acquireNodeLock(nodeID string) *sync.Mutex {
    mu, _ := h.nodeLocks.LoadOrStore(nodeID, &sync.Mutex{})
    return mu.(*sync.Mutex)
}

func (h *NodeHandlerV2) AddStorageNodeV2(req *pb.AddStorageNodeRequest) {
    mu := h.acquireNodeLock(req.NodeId)  // Lock ONLY this node
    mu.Lock()
    defer mu.Unlock()
    // ... bootstrap logic
}
```

**Benefits**:
- Bootstrap node-1 and node-2 simultaneously
- Bootstrap node-1 while decommissioning node-3
- No contention for different nodes
- Better resource utilization

### 15.3 Lock Acquisition

The `acquireNodeLock()` method uses `sync.Map.LoadOrStore()` for thread-safe lock creation:

```go
func (h *NodeHandlerV2) acquireNodeLock(nodeID string) *sync.Mutex {
    // LoadOrStore is atomic:
    // - If nodeID exists: return existing mutex
    // - If nodeID doesn't exist: store new mutex, return it
    mu, _ := h.nodeLocks.LoadOrStore(nodeID, &sync.Mutex{})
    return mu.(*sync.Mutex)
}
```

### 15.4 Concurrency Scenarios

#### Scenario 1: Concurrent Bootstraps (Different Nodes)
```
Time    Node-1          Node-2          Result
----    ------          ------          ------
T0      AddNode start   -               Lock node-1
T1      Streaming...    AddNode start   Lock node-2
T2      Streaming...    Streaming...    Both proceed concurrently
T3      Complete        Streaming...    Unlock node-1
T4      -               Complete        Unlock node-2
```

#### Scenario 2: Bootstrap + Decommission (Different Nodes)
```
Time    Node-1          Node-2          Result
----    ------          ------          ------
T0      AddNode start   -               Lock node-1
T1      Streaming...    RemoveNode      Lock node-2
T2      Streaming...    Streaming...    Both proceed concurrently
```

#### Scenario 3: Same Node (Serialized)
```
Time    Operation-1     Operation-2     Result
----    -----------     -----------     ------
T0      AddNode-X       -               Lock node-X
T1      Streaming...    AddNode-X       Blocks on node-X lock
T2      Complete        Still blocked   Unlock node-X
T3      -               Now proceeds    Lock node-X
```

### 15.5 Implementation Changes

**File**: `coordinator/internal/handler/node_handler_v2.go`

**Changes**:
- Added `nodeLocks sync.Map` field
- Added `acquireNodeLock(nodeID string)` method
- Updated `AddStorageNodeV2()` to use per-node lock
- Updated `RemoveStorageNodeV2()` to use per-node lock

## 16. Replica Selection and Quorum (Phases 3-5)

### 16.1 Split Replica Selection

The `RoutingService` provides separate methods for write and read replica selection:

#### 16.1.1 GetWriteReplicas

```go
func (s *RoutingService) GetWriteReplicas(ctx context.Context, tenantID, key string, rf int) ([]*model.StorageNode, error)
```

**Returns nodes that should receive writes**:
- NORMAL nodes: Authoritative owners
- BOOTSTRAPPING nodes: Receive writes for PendingRanges
- LEAVING nodes: Still receive writes until streaming completes

**Why include BOOTSTRAPPING nodes?**
- Cassandra-correct pattern
- Ensures bootstrapping nodes get current writes
- Prevents data gaps during bootstrap

#### 16.1.2 GetReadReplicas

```go
func (s *RoutingService) GetReadReplicas(ctx context.Context, tenantID, key string, rf int) ([]*model.StorageNode, error)
```

**Returns nodes that should be queried for reads**:
- NORMAL nodes: Authoritative owners
- BOOTSTRAPPING nodes: May have incomplete data, but queryable
- LEAVING nodes: Still has data until streaming completes

**Why include BOOTSTRAPPING nodes?**
- Provides consistency during bootstrap
- Read repair will fix any missing data

### 16.2 Authoritative Replica Concept

For quorum calculation, only "authoritative" replicas count:

```go
func (s *RoutingService) GetAuthoritativeReplicas(nodes []*model.StorageNode) []*model.StorageNode {
    authoritative := make([]*model.StorageNode, 0)
    for _, node := range nodes {
        if node.State == model.NodeStateNormal {
            authoritative = append(authoritative, node)
        }
    }
    return authoritative
}
```

**Authoritative = NORMAL state only**

**Non-authoritative**:
- BOOTSTRAPPING nodes (receiving data, not yet authoritative)
- LEAVING nodes (transferring data, no longer authoritative)

### 16.3 Quorum Calculation

The `ConsistencyService` calculates quorum from authoritative replicas only:

```go
func (s *ConsistencyService) GetRequiredReplicasForWriteSet(
    consistency string,
    writeReplicas []*model.StorageNode,
) int {
    // Count ONLY NORMAL state nodes
    authoritativeCount := 0
    for _, node := range writeReplicas {
        if node.State == model.NodeStateNormal {
            authoritativeCount++
        }
    }

    // Calculate quorum from authoritative count
    switch consistency {
    case "one":
        return 1
    case "all":
        return authoritativeCount  // ALL authoritative replicas
    case "quorum":
        return (authoritativeCount / 2) + 1  // Majority of authoritative
    }
}
```

**Example during bootstrap**:
```
Write Replicas: [Node-1 (NORMAL), Node-2 (NORMAL), Node-3 (BOOTSTRAPPING)]
Authoritative Replicas: [Node-1, Node-2]
Quorum: (2 / 2) + 1 = 2

For QUORUM consistency:
- Write to all 3 nodes (including BOOTSTRAPPING)
- Require 2 successful writes (from NORMAL nodes)
- If Node-3 write fails, hint is created
- Write succeeds if Node-1 and Node-2 succeed
```

## 17. Storage Node Lifecycle Management

### 17.1 Overview

Coordinators manage the consistent hash ring and handle storage node addition/deletion. When nodes are added or removed, data must be migrated to maintain proper replication and distribution.

### 17.2 Node Addition Process

#### 17.2.1 API Call Flow

**Admin calls**: `POST /v1/admin/storage-nodes`

**Process**:
1. **API Gateway** receives request, authenticates admin
2. **Coordinator** (designated leader or admin coordinator) receives request
3. **Coordinator** validates node configuration
4. **Coordinator** adds node to hash ring (with virtual nodes)
5. **Coordinator** initiates data migration process
6. **Coordinator** returns migration ID to admin

#### 17.2.2 Hash Ring Update

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

#### 17.2.3 Data Migration Process

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

#### 17.2.4 Request Handling During Node Addition

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

### 17.3 Node Deletion Process

#### 17.3.1 API Call Flow

**Admin calls**: `DELETE /v1/admin/storage-nodes/{node_id}`

**Process**:
1. **API Gateway** receives request, authenticates admin
2. **Coordinator** validates node can be removed:
   - Check if removal would violate replication factor for any tenant
   - Verify sufficient replicas exist
3. **Coordinator** initiates data migration (copy data away from node)
4. **Coordinator** removes node from hash ring after migration
5. **Coordinator** returns migration ID

#### 17.3.2 Data Migration Process

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

#### 17.3.3 Request Handling During Node Deletion

**Write Requests**:
- Coordinator stops routing writes to node being removed
- Writes go to remaining replicas only
- Quorum calculated from remaining nodes

**Read Requests**:
- Coordinator stops reading from node being removed
- Reads from remaining replicas only
- If node has unique data, coordinator triggers replication first

### 17.4 Migration State Management

#### 17.4.1 Migration State Storage

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

#### 17.4.2 Coordinator Coordination

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

### 17.5 Hash Ring Management

#### 17.5.1 Ring State Storage

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

#### 17.5.2 Ring Update Propagation

**Method 1: Metadata Store (Simple)**
- Ring state stored in PostgreSQL
- Coordinators poll metadata store for updates
- TTL-based cache with invalidation

**Method 2: Event Stream (Advanced)**
- Ring updates published to event stream
- Coordinators subscribe to updates
- Real-time propagation

### 17.6 Internal APIs for Migration

#### 17.6.1 Start Migration

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

#### 17.6.2 Get Migration Status

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

### 17.7 Error Handling

#### 17.7.1 Migration Failures

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

#### 17.7.2 Partial Migrations

**Handling**:
- Checkpoint progress regularly
- Resume from last checkpoint on restart
- Verify data integrity after migration
- Retry failed key migrations

### 17.8 Performance Considerations

#### 17.8.1 Migration Throttling

- Limit migration bandwidth to avoid impacting normal operations
- Prioritize frequently accessed keys
- Background migration during low traffic periods

#### 17.8.2 Request Impact

- Minimal impact during dual-write phase (slightly higher latency)
- No impact during background copy phase
- Brief impact during cutover (coordinator state update)

