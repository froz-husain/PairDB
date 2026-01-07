# Coordinator: Sequence Diagrams

This document provides sequence diagrams for all major flows supported by the Coordinator service.

## 1. Write Key-Value Flow (Quorum Consistency)

```mermaid
sequenceDiagram
    participant API Gateway
    participant Coordinator
    participant IdempotencyService
    participant IdempotencyStore
    participant TenantService
    participant Cache
    participant MetadataStore
    participant RoutingService
    participant ConsistentHasher
    participant VectorClockService
    participant ConsistencyService
    participant StorageClient
    participant StorageNode1
    participant StorageNode2
    participant StorageNode3
    
    API Gateway->>Coordinator: WriteKeyValue Request
    Coordinator->>IdempotencyService: Check Idempotency Key
    alt Idempotency Key Provided
        IdempotencyService->>IdempotencyStore: Get(idempotencyKey)
        alt Key Found
            IdempotencyStore-->>IdempotencyService: Cached Response
            IdempotencyService-->>Coordinator: Cached Response (IsDuplicate=true)
            Coordinator-->>API Gateway: Response (Duplicate)
        else Key Not Found
            IdempotencyService-->>Coordinator: Not Found
        end
    end
    
    Coordinator->>TenantService: GetTenant(tenantID)
    TenantService->>Cache: GetTenant(tenantID)
    alt Cache Hit
        Cache-->>TenantService: Tenant
    else Cache Miss
        TenantService->>MetadataStore: GetTenant(tenantID)
        MetadataStore-->>TenantService: Tenant
        TenantService->>Cache: SetTenant(tenantID, tenant)
    end
    TenantService-->>Coordinator: Tenant
    
    Coordinator->>RoutingService: GetReplicas(tenantID, key, replicationFactor)
    RoutingService->>ConsistentHasher: Hash(tenantID:key)
    ConsistentHasher-->>RoutingService: Hash Value
    RoutingService->>ConsistentHasher: GetNodes(hash, replicationFactor)
    ConsistentHasher-->>RoutingService: Replica Nodes
    RoutingService-->>Coordinator: [StorageNode1, StorageNode2, StorageNode3]
    
    Coordinator->>VectorClockService: Increment(nodeID)
    VectorClockService-->>Coordinator: VectorClock
    
    Coordinator->>ConsistencyService: GetRequiredReplicas("quorum", 3)
    ConsistencyService-->>Coordinator: 2 (quorum)
    
    par Parallel Writes
        Coordinator->>StorageClient: Write(node1, tenantID, key, value, vectorClock)
        StorageClient->>StorageNode1: gRPC Write
        StorageNode1-->>StorageClient: Success
        StorageClient-->>Coordinator: Response1
        
        Coordinator->>StorageClient: Write(node2, tenantID, key, value, vectorClock)
        StorageClient->>StorageNode2: gRPC Write
        StorageNode2-->>StorageClient: Success
        StorageClient-->>Coordinator: Response2
        
        Coordinator->>StorageClient: Write(node3, tenantID, key, value, vectorClock)
        StorageClient->>StorageNode3: gRPC Write
        StorageNode3-->>StorageClient: Success
        StorageClient-->>Coordinator: Response3
    end
    
    Coordinator->>Coordinator: Check Quorum (2/3)
    Coordinator->>IdempotencyService: Store(idempotencyKey, response)
    IdempotencyService->>IdempotencyStore: Set(idempotencyKey, response, TTL)
    IdempotencyStore-->>IdempotencyService: OK
    IdempotencyService-->>Coordinator: OK
    
    Coordinator-->>API Gateway: WriteResponse (Success)
```

## 2. Read Key-Value Flow (Quorum Consistency)

```mermaid
sequenceDiagram
    participant API Gateway
    participant Coordinator
    participant TenantService
    participant Cache
    participant MetadataStore
    participant RoutingService
    participant ConsistentHasher
    participant ConsistencyService
    participant StorageClient
    participant StorageNode1
    participant StorageNode2
    participant StorageNode3
    participant ConflictService
    participant VectorClockService
    
    API Gateway->>Coordinator: ReadKeyValue Request
    Coordinator->>TenantService: GetTenant(tenantID)
    TenantService->>Cache: GetTenant(tenantID)
    Cache-->>TenantService: Tenant
    TenantService-->>Coordinator: Tenant
    
    Coordinator->>RoutingService: GetReplicas(tenantID, key, replicationFactor)
    RoutingService->>ConsistentHasher: GetNodes(hash, replicationFactor)
    ConsistentHasher-->>RoutingService: Replica Nodes
    RoutingService-->>Coordinator: [StorageNode1, StorageNode2, StorageNode3]
    
    Coordinator->>ConsistencyService: GetRequiredReplicas("quorum", 3)
    ConsistencyService-->>Coordinator: 2 (quorum)
    
    par Parallel Reads
        Coordinator->>StorageClient: Read(node1, tenantID, key)
        StorageClient->>StorageNode1: gRPC Read
        StorageNode1-->>StorageClient: Response1 (value1, vc1)
        StorageClient-->>Coordinator: Response1
        
        Coordinator->>StorageClient: Read(node2, tenantID, key)
        StorageClient->>StorageNode2: gRPC Read
        StorageNode2-->>StorageClient: Response2 (value2, vc2)
        StorageClient-->>Coordinator: Response2
        
        Coordinator->>StorageClient: Read(node3, tenantID, key)
        StorageClient->>StorageNode3: gRPC Read
        StorageNode3-->>StorageClient: Response3 (value3, vc3)
        StorageClient-->>Coordinator: Response3
    end
    
    Coordinator->>Coordinator: Check Quorum (2/3)
    Coordinator->>ConflictService: DetectConflicts(responses)
    ConflictService->>VectorClockService: Compare(vc1, vc2, vc3)
    VectorClockService-->>ConflictService: Comparison Results
    ConflictService->>ConflictService: Find Latest Version
    alt Conflicts Detected
        ConflictService->>ConflictService: TriggerRepair(latest, replicas)
        ConflictService->>ConflictService: Queue Repair Request
    end
    ConflictService-->>Coordinator: Latest Response, Conflicts
    
    Coordinator-->>API Gateway: ReadResponse (Latest Value)
```

## 3. Conflict Resolution Flow

```mermaid
sequenceDiagram
    participant Coordinator
    participant ConflictService
    participant VectorClockService
    participant StorageClient
    participant StorageNode1
    participant StorageNode2
    participant StorageNode3
    
    Coordinator->>ConflictService: DetectConflicts(responses)
    ConflictService->>VectorClockService: Compare(vc1, vc2)
    VectorClockService-->>ConflictService: Concurrent
    ConflictService->>VectorClockService: Compare(vc2, vc3)
    VectorClockService-->>ConflictService: Concurrent
    ConflictService->>ConflictService: Find Latest (by timestamp)
    ConflictService->>ConflictService: TriggerRepair(latest, replicas)
    ConflictService->>ConflictService: Queue Repair Request
    
    Note over ConflictService: Repair Worker Processes Queue
    
    ConflictService->>ConflictService: executeRepair(repairRequest)
    
    par Parallel Repairs
        ConflictService->>StorageClient: Repair(node1, tenantID, key, latestValue, latestVC)
        StorageClient->>StorageNode1: gRPC Repair
        StorageNode1-->>StorageClient: Success
        
        ConflictService->>StorageClient: Repair(node2, tenantID, key, latestValue, latestVC)
        StorageClient->>StorageNode2: gRPC Repair
        StorageNode2-->>StorageClient: Success
        
        ConflictService->>StorageClient: Repair(node3, tenantID, key, latestValue, latestVC)
        StorageClient->>StorageNode3: gRPC Repair
        StorageNode3-->>StorageClient: Success
    end
    
    ConflictService->>ConflictService: Repair Complete
```

## 4. Create Tenant Flow

```mermaid
sequenceDiagram
    participant API Gateway
    participant Coordinator
    participant TenantService
    participant MetadataStore
    participant Cache
    
    API Gateway->>Coordinator: CreateTenant Request
    Coordinator->>TenantService: CreateTenant(tenantID, replicationFactor)
    TenantService->>TenantService: Validate Replication Factor
    TenantService->>TenantService: Create Tenant Object
    TenantService->>MetadataStore: CreateTenant(tenant)
    MetadataStore->>MetadataStore: INSERT INTO tenants
    MetadataStore-->>TenantService: Success
    TenantService->>Cache: SetTenant(tenantID, tenant)
    Cache-->>TenantService: OK
    TenantService-->>Coordinator: Tenant
    Coordinator-->>API Gateway: CreateTenantResponse
```

## 5. Update Replication Factor Flow

```mermaid
sequenceDiagram
    participant API Gateway
    participant Coordinator
    participant TenantService
    participant Cache
    participant MetadataStore
    participant MigrationService
    
    API Gateway->>Coordinator: UpdateReplicationFactor Request
    Coordinator->>TenantService: UpdateReplicationFactor(tenantID, newRF)
    TenantService->>TenantService: GetTenant(tenantID)
    TenantService->>Cache: GetTenant(tenantID)
    Cache-->>TenantService: Tenant
    TenantService->>TenantService: Validate New Replication Factor
    TenantService->>TenantService: Update Tenant Object
    TenantService->>MetadataStore: UpdateTenant(tenant)
    MetadataStore->>MetadataStore: UPDATE tenants (with version check)
    MetadataStore-->>TenantService: Success
    TenantService->>Cache: DeleteTenant(tenantID)
    Cache-->>TenantService: OK
    TenantService-->>Coordinator: Updated Tenant
    
    alt Replication Factor Changed
        Coordinator->>MigrationService: Trigger Migration(tenantID, oldRF, newRF)
        MigrationService->>MigrationService: Initiate Data Migration
    end
    
    Coordinator-->>API Gateway: UpdateReplicationFactorResponse
```

## 6. Add Storage Node Flow

```mermaid
sequenceDiagram
    participant API Gateway
    participant Coordinator
    participant MetadataStore
    participant RoutingService
    participant ConsistentHasher
    participant MigrationService
    participant StorageNode
    
    API Gateway->>Coordinator: AddStorageNode Request
    Coordinator->>Coordinator: Validate Node Configuration
    Coordinator->>MetadataStore: CreateStorageNode(node)
    MetadataStore->>MetadataStore: INSERT INTO storage_nodes
    MetadataStore-->>Coordinator: Success
    
    Coordinator->>RoutingService: updateHashRing()
    RoutingService->>MetadataStore: GetStorageNodes()
    MetadataStore-->>RoutingService: All Nodes
    RoutingService->>ConsistentHasher: Clear()
    RoutingService->>ConsistentHasher: AddNode(newNode, virtualNodes)
    ConsistentHasher-->>RoutingService: OK
    RoutingService-->>Coordinator: Hash Ring Updated
    
    Coordinator->>MigrationService: Initiate Migration(node, type=addition)
    MigrationService->>MigrationService: Create Migration Record
    MigrationService->>MigrationService: Start Dual-Write Phase
    MigrationService->>StorageNode: Begin Data Migration
    StorageNode-->>MigrationService: Migration Started
    MigrationService-->>Coordinator: Migration ID
    
    Coordinator-->>API Gateway: AddStorageNodeResponse (Migration ID)
```

## 7. Hash Ring Update Flow

```mermaid
sequenceDiagram
    participant RoutingService
    participant MetadataStore
    participant ConsistentHasher
    participant HashRing
    
    Note over RoutingService: Periodic Update (every 30s)
    RoutingService->>RoutingService: refreshHashRing()
    RoutingService->>MetadataStore: GetStorageNodes()
    MetadataStore-->>RoutingService: All Active Nodes
    RoutingService->>RoutingService: Rebuild Hash Ring
    RoutingService->>ConsistentHasher: Clear()
    
    loop For Each Active Node
        RoutingService->>ConsistentHasher: AddNode(nodeID, virtualNodes)
        ConsistentHasher->>ConsistentHasher: Create Virtual Nodes
        ConsistentHasher->>HashRing: Add Virtual Nodes
    end
    
    ConsistentHasher-->>RoutingService: Hash Ring Updated
    RoutingService->>HashRing: Update LastUpdated
    RoutingService->>RoutingService: Hash Ring Refresh Complete
```

## 8. Idempotency Check Flow

```mermaid
sequenceDiagram
    participant Coordinator
    participant IdempotencyService
    participant IdempotencyStore
    
    Coordinator->>IdempotencyService: Get(tenantID, key, idempotencyKey)
    IdempotencyService->>IdempotencyStore: Get(redisKey)
    
    alt Key Found
        IdempotencyStore-->>IdempotencyService: Cached Response Data
        IdempotencyService->>IdempotencyService: Unmarshal Response
        IdempotencyService->>IdempotencyService: Set IsDuplicate=true
        IdempotencyService-->>Coordinator: Cached Response (IsDuplicate=true)
        Coordinator->>Coordinator: Skip Write Operation
    else Key Not Found
        IdempotencyStore-->>IdempotencyService: Not Found
        IdempotencyService-->>Coordinator: Not Found
        Coordinator->>Coordinator: Proceed with Write
        
        Note over Coordinator: Write Operation Completes
        
        Coordinator->>IdempotencyService: Store(tenantID, key, idempotencyKey, response)
        IdempotencyService->>IdempotencyService: Marshal Response
        IdempotencyService->>IdempotencyStore: Set(redisKey, data, TTL=24h)
        IdempotencyStore-->>IdempotencyService: OK
        IdempotencyService-->>Coordinator: Stored
    end
```

## Flow Descriptions

### Write Key-Value Flow
1. Check idempotency key (if provided)
2. Get tenant configuration (with caching)
3. Route to storage nodes using consistent hashing
4. Generate vector clock
5. Determine required replicas based on consistency
6. Write to replicas in parallel
7. Check quorum
8. Store idempotency key
9. Return response

### Read Key-Value Flow
1. Get tenant configuration
2. Route to storage nodes
3. Read from replicas in parallel
4. Check quorum
5. Detect conflicts using vector clocks
6. Trigger repair if conflicts found
7. Return latest value

### Conflict Resolution Flow
1. Compare vector clocks from all responses
2. Identify concurrent writes (conflicts)
3. Find latest version by timestamp
4. Queue repair requests
5. Repair workers update stale replicas
6. All replicas synchronized

### Create Tenant Flow
1. Validate replication factor
2. Create tenant in metadata store
3. Cache tenant configuration
4. Return created tenant

### Update Replication Factor Flow
1. Get current tenant
2. Validate new replication factor
3. Update in metadata store (with optimistic locking)
4. Invalidate cache
5. Trigger migration if needed

### Add Storage Node Flow
1. Validate node configuration
2. Create node in metadata store
3. Update hash ring
4. Initiate migration
5. Return migration ID

### Hash Ring Update Flow
1. Periodically refresh from metadata store
2. Rebuild hash ring with all active nodes
3. Update consistent hasher
4. Maintain virtual node distribution

### Idempotency Check Flow
1. Check Redis for idempotency key
2. If found, return cached response
3. If not found, proceed with write
4. Store response in Redis after successful write

