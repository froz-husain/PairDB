# Coordinator Service: Low-Level Design (LLD)

## 1. Overview

This document provides the low-level implementation details for the Coordinator service. It complements the high-level design document with detailed class structures, algorithms, data flows, and implementation considerations.

## 2. Technology Stack

### 2.1 Language and Framework
- **Language**: Go 1.21+
- **gRPC Framework**: google.golang.org/grpc
- **Database Client**: pgx (PostgreSQL driver)
- **Cache Client**: go-redis/redis
- **Logging**: zap (structured logging)
- **Metrics**: Prometheus client
- **Configuration**: viper
- **Testing**: testify, gomock

### 2.2 Dependencies
```go
require (
    google.golang.org/grpc v1.58.0
    google.golang.org/protobuf v1.31.0
    github.com/jackc/pgx/v5 v5.4.3
    github.com/redis/go-redis/v9 v9.2.1
    go.uber.org/zap v1.26.0
    github.com/prometheus/client_golang v1.17.0
    github.com/spf13/viper v1.17.0
    github.com/stretchr/testify v1.8.4
    github.com/golang/mock v1.6.0
    github.com/google/uuid v1.4.0
    golang.org/x/sync v0.4.0
)
```

## 3. Package Structure

```
coordinator/
├── cmd/
│   └── coordinator/
│       └── main.go                 # Entry point
├── internal/
│   ├── config/
│   │   └── config.go              # Configuration management
│   ├── server/
│   │   └── grpc_server.go         # gRPC server setup
│   ├── handler/
│   │   ├── keyvalue_handler.go    # Key-value operation handlers
│   │   ├── tenant_handler.go      # Tenant management handlers
│   │   └── node_handler.go        # Storage node management handlers
│   ├── service/
│   │   ├── coordinator_service.go # Main coordination logic
│   │   ├── tenant_service.go      # Tenant configuration service
│   │   ├── routing_service.go     # Consistent hashing router
│   │   ├── consistency_service.go # Consistency coordination
│   │   ├── vectorclock_service.go # Vector clock management
│   │   ├── idempotency_service.go # Idempotency handling
│   │   ├── conflict_service.go    # Conflict resolution
│   │   └── migration_service.go   # Node migration management
│   ├── store/
│   │   ├── metadata_store.go      # PostgreSQL metadata access
│   │   ├── idempotency_store.go   # Redis idempotency access
│   │   └── cache.go               # In-memory caching layer
│   ├── client/
│   │   └── storage_client.go      # Storage node gRPC client
│   ├── model/
│   │   ├── tenant.go              # Tenant models
│   │   ├── vectorclock.go         # Vector clock models
│   │   ├── hashring.go            # Hash ring models
│   │   └── migration.go           # Migration models
│   ├── algorithm/
│   │   ├── consistent_hash.go     # Consistent hashing algorithm
│   │   ├── vectorclock_ops.go     # Vector clock operations
│   │   └── quorum.go              # Quorum calculation
│   ├── metrics/
│   │   └── prometheus.go          # Prometheus metrics
│   └── health/
│       └── health_check.go        # Health check endpoints
├── pkg/
│   └── proto/
│       ├── coordinator.proto      # gRPC definitions
│       └── storage.proto          # Storage node gRPC definitions
└── tests/
    ├── integration/
    └── unit/
```

## 4. Core Data Structures

### 4.1 Configuration

```go
// config/config.go
package config

import (
    "time"
)

type Config struct {
    Server      ServerConfig
    Database    DatabaseConfig
    Redis       RedisConfig
    HashRing    HashRingConfig
    Consistency ConsistencyConfig
    Cache       CacheConfig
    Metrics     MetricsConfig
    Logging     LoggingConfig
}

type ServerConfig struct {
    Host            string
    Port            int
    MaxConnections  int
    ReadTimeout     time.Duration
    WriteTimeout    time.Duration
    ShutdownTimeout time.Duration
}

type DatabaseConfig struct {
    Host            string
    Port            int
    Database        string
    User            string
    Password        string
    MaxConnections  int
    MinConnections  int
    ConnMaxLifetime time.Duration
}

type RedisConfig struct {
    Host            string
    Port            int
    Password        string
    DB              int
    MaxRetries      int
    PoolSize        int
    MinIdleConns    int
}

type HashRingConfig struct {
    VirtualNodes    int           // Default: 150
    UpdateInterval  time.Duration // How often to refresh ring from metadata
}

type ConsistencyConfig struct {
    DefaultLevel    string        // "quorum"
    WriteTimeout    time.Duration // 5s
    ReadTimeout     time.Duration // 3s
    RepairAsync     bool          // true
}

type CacheConfig struct {
    TenantConfigTTL time.Duration // 5 minutes
    MaxSize         int           // Max entries
}

type MetricsConfig struct {
    Enabled bool
    Port    int
    Path    string
}

type LoggingConfig struct {
    Level  string // "debug", "info", "warn", "error"
    Format string // "json", "console"
}
```

### 4.2 Domain Models

```go
// model/tenant.go
package model

import "time"

type Tenant struct {
    TenantID          string
    ReplicationFactor int
    CreatedAt         time.Time
    UpdatedAt         time.Time
    Version           int64 // For optimistic locking
}

// model/vectorclock.go
package model

type VectorClockEntry struct {
    CoordinatorNodeID string
    LogicalTimestamp  int64
}

type VectorClock struct {
    Entries []VectorClockEntry
}

// Comparison results
type VectorClockComparison int

const (
    Identical VectorClockComparison = iota
    Before    // First happens before second
    After     // First happens after second
    Concurrent // Conflict
)

// model/hashring.go
package model

import "time"

type HashRing struct {
    Nodes        map[string]*StorageNode
    VirtualNodes []*VirtualNode
    LastUpdated  time.Time
}

type StorageNode struct {
    NodeID       string
    Host         string
    Port         int
    Status       NodeStatus
    VirtualNodes int
}

type NodeStatus string

const (
    NodeStatusActive   NodeStatus = "active"
    NodeStatusDraining NodeStatus = "draining"
    NodeStatusInactive NodeStatus = "inactive"
)

type VirtualNode struct {
    VNodeID string
    Hash    uint64
    NodeID  string
}

// model/migration.go
package model

import "time"

type Migration struct {
    MigrationID  string
    Type         MigrationType
    NodeID       string
    Status       MigrationStatus
    Phase        MigrationPhase
    Progress     MigrationProgress
    StartedAt    time.Time
    CompletedAt  *time.Time
    ErrorMessage string
}

type MigrationType string

const (
    MigrationTypeNodeAddition MigrationType = "node_addition"
    MigrationTypeNodeDeletion MigrationType = "node_deletion"
)

type MigrationStatus string

const (
    MigrationStatusPending    MigrationStatus = "pending"
    MigrationStatusInProgress MigrationStatus = "in_progress"
    MigrationStatusCompleted  MigrationStatus = "completed"
    MigrationStatusFailed     MigrationStatus = "failed"
    MigrationStatusCancelled  MigrationStatus = "cancelled"
)

type MigrationPhase string

const (
    MigrationPhaseDualWrite     MigrationPhase = "dual_write"
    MigrationPhaseDataCopy      MigrationPhase = "data_copy"
    MigrationPhaseCutover       MigrationPhase = "cutover"
    MigrationPhaseCleanup       MigrationPhase = "cleanup"
)

type MigrationProgress struct {
    KeysMigrated int64
    TotalKeys    int64
    Percentage   float64
}

type RepairRequest struct {
    TenantID    string
    Key         string
    Value       []byte
    VectorClock VectorClock
    Timestamp   int64
}

type IdempotencyRecord struct {
    TenantID       string
    Key            string
    IdempotencyKey string
    Response       []byte
    Timestamp      time.Time
    TTL            time.Duration
}
```

## 5. Service Layer Implementation

### 5.1 Coordinator Service (Main)

```go
// service/coordinator_service.go
package service

import (
    "context"
    "fmt"
    "time"

    "go.uber.org/zap"
    "golang.org/x/sync/errgroup"
)

type CoordinatorService struct {
    tenantService      *TenantService
    routingService     *RoutingService
    consistencyService *ConsistencyService
    vectorClockService *VectorClockService
    idempotencyService *IdempotencyService
    conflictService    *ConflictService
    storageClient      *StorageClient
    logger             *zap.Logger
    nodeID             string // This coordinator's ID
}

func NewCoordinatorService(
    tenantSvc *TenantService,
    routingSvc *RoutingService,
    consistencySvc *ConsistencyService,
    vectorClockSvc *VectorClockService,
    idempotencySvc *IdempotencyService,
    conflictSvc *ConflictService,
    storageClient *StorageClient,
    logger *zap.Logger,
    nodeID string,
) *CoordinatorService {
    return &CoordinatorService{
        tenantService:      tenantSvc,
        routingService:     routingSvc,
        consistencyService: consistencySvc,
        vectorClockService: vectorClockSvc,
        idempotencyService: idempotencySvc,
        conflictService:    conflictSvc,
        storageClient:      storageClient,
        logger:             logger,
        nodeID:             nodeID,
    }
}

// WriteKeyValue handles write operations
func (s *CoordinatorService) WriteKeyValue(
    ctx context.Context,
    tenantID string,
    key string,
    value []byte,
    consistency string,
    idempotencyKey string,
) (*WriteResponse, error) {
    // 1. Check idempotency
    if idempotencyKey != "" {
        if cached, err := s.idempotencyService.Get(ctx, tenantID, key, idempotencyKey); err == nil {
            s.logger.Info("Request deduplicated", zap.String("idempotency_key", idempotencyKey))
            return cached, nil
        }
    } else {
        // Generate idempotency key
        idempotencyKey = s.idempotencyService.Generate(tenantID, key)
    }

    // 2. Get tenant configuration
    tenant, err := s.tenantService.GetTenant(ctx, tenantID)
    if err != nil {
        return nil, fmt.Errorf("failed to get tenant: %w", err)
    }

    // 3. Route to storage nodes (get replica list)
    replicas, err := s.routingService.GetReplicas(tenantID, key, tenant.ReplicationFactor)
    if err != nil {
        return nil, fmt.Errorf("failed to get replicas: %w", err)
    }

    // 4. Generate vector clock
    vectorClock := s.vectorClockService.Increment(s.nodeID)

    // 5. Determine required replica count based on consistency
    requiredReplicas := s.consistencyService.GetRequiredReplicas(consistency, len(replicas))

    // 6. Write to replicas in parallel
    successCount, responses := s.writeToReplicas(ctx, replicas, tenantID, key, value, vectorClock, requiredReplicas)

    // 7. Check if quorum reached
    if successCount < requiredReplicas {
        return nil, fmt.Errorf("quorum not reached: %d/%d", successCount, requiredReplicas)
    }

    // 8. Build response
    response := &WriteResponse{
        Success:         true,
        Key:             key,
        IdempotencyKey:  idempotencyKey,
        VectorClock:     vectorClock,
        ReplicaCount:    successCount,
        Consistency:     consistency,
    }

    // 9. Store idempotency record
    if err := s.idempotencyService.Store(ctx, tenantID, key, idempotencyKey, response); err != nil {
        s.logger.Warn("Failed to store idempotency record", zap.Error(err))
    }

    return response, nil
}

// writeToReplicas writes to multiple replicas in parallel
func (s *CoordinatorService) writeToReplicas(
    ctx context.Context,
    replicas []*model.StorageNode,
    tenantID string,
    key string,
    value []byte,
    vectorClock model.VectorClock,
    requiredReplicas int,
) (int, []*StorageResponse) {
    // Use errgroup for parallel writes with early termination
    g, ctx := errgroup.WithContext(ctx)
    responses := make([]*StorageResponse, len(replicas))
    successCount := 0

    for i, replica := range replicas {
        i, replica := i, replica // Capture loop variables
        g.Go(func() error {
            resp, err := s.storageClient.Write(ctx, replica, tenantID, key, value, vectorClock)
            if err != nil {
                s.logger.Warn("Write to replica failed",
                    zap.String("node_id", replica.NodeID),
                    zap.Error(err))
                return nil // Don't fail entire operation
            }
            responses[i] = resp
            successCount++
            return nil
        })
    }

    g.Wait()
    return successCount, responses
}

// ReadKeyValue handles read operations
func (s *CoordinatorService) ReadKeyValue(
    ctx context.Context,
    tenantID string,
    key string,
    consistency string,
) (*ReadResponse, error) {
    // 1. Get tenant configuration
    tenant, err := s.tenantService.GetTenant(ctx, tenantID)
    if err != nil {
        return nil, fmt.Errorf("failed to get tenant: %w", err)
    }

    // 2. Route to storage nodes
    replicas, err := s.routingService.GetReplicas(tenantID, key, tenant.ReplicationFactor)
    if err != nil {
        return nil, fmt.Errorf("failed to get replicas: %w", err)
    }

    // 3. Determine required replica count
    requiredReplicas := s.consistencyService.GetRequiredReplicas(consistency, len(replicas))

    // 4. Read from replicas in parallel
    responses := s.readFromReplicas(ctx, replicas, tenantID, key, requiredReplicas)

    // 5. Check if we have enough responses
    if len(responses) < requiredReplicas {
        return nil, fmt.Errorf("quorum not reached: %d/%d", len(responses), requiredReplicas)
    }

    // 6. Detect conflicts using vector clocks
    latest, conflicts := s.conflictService.DetectConflicts(responses)

    // 7. If conflicts detected, trigger async repair
    if len(conflicts) > 1 {
        s.logger.Info("Conflicts detected, triggering repair",
            zap.String("tenant_id", tenantID),
            zap.String("key", key),
            zap.Int("conflict_count", len(conflicts)))
        go s.conflictService.TriggerRepair(context.Background(), tenantID, key, latest, replicas)
    }

    // 8. Return latest value
    return &ReadResponse{
        Success:     true,
        Key:         key,
        Value:       latest.Value,
        VectorClock: latest.VectorClock,
    }, nil
}

// readFromReplicas reads from multiple replicas in parallel
func (s *CoordinatorService) readFromReplicas(
    ctx context.Context,
    replicas []*model.StorageNode,
    tenantID string,
    key string,
    requiredReplicas int,
) []*StorageResponse {
    g, ctx := errgroup.WithContext(ctx)
    responseChan := make(chan *StorageResponse, len(replicas))

    for _, replica := range replicas {
        replica := replica
        g.Go(func() error {
            resp, err := s.storageClient.Read(ctx, replica, tenantID, key)
            if err != nil {
                s.logger.Warn("Read from replica failed",
                    zap.String("node_id", replica.NodeID),
                    zap.Error(err))
                return nil
            }
            responseChan <- resp
            return nil
        })
    }

    g.Wait()
    close(responseChan)

    // Collect responses
    responses := make([]*StorageResponse, 0, len(replicas))
    for resp := range responseChan {
        responses = append(responses, resp)
    }

    return responses
}

type WriteResponse struct {
    Success         bool
    Key             string
    IdempotencyKey  string
    VectorClock     model.VectorClock
    ReplicaCount    int
    Consistency     string
    IsDuplicate     bool
}

type ReadResponse struct {
    Success     bool
    Key         string
    Value       []byte
    VectorClock model.VectorClock
}

type StorageResponse struct {
    NodeID      string
    Success     bool
    Value       []byte
    VectorClock model.VectorClock
    Error       error
}
```

### 5.2 Tenant Service

```go
// service/tenant_service.go
package service

import (
    "context"
    "fmt"
    "time"

    "go.uber.org/zap"
)

type TenantService struct {
    metadataStore *MetadataStore
    cache         *Cache
    logger        *zap.Logger
}

func NewTenantService(
    metadataStore *MetadataStore,
    cache *Cache,
    logger *zap.Logger,
) *TenantService {
    return &TenantService{
        metadataStore: metadataStore,
        cache:         cache,
        logger:        logger,
    }
}

// GetTenant retrieves tenant configuration (with caching)
func (s *TenantService) GetTenant(ctx context.Context, tenantID string) (*model.Tenant, error) {
    // Check cache first
    if tenant, found := s.cache.GetTenant(tenantID); found {
        return tenant, nil
    }

    // Cache miss - fetch from database
    tenant, err := s.metadataStore.GetTenant(ctx, tenantID)
    if err != nil {
        return nil, fmt.Errorf("failed to get tenant from store: %w", err)
    }

    // Store in cache
    s.cache.SetTenant(tenantID, tenant)

    return tenant, nil
}

// CreateTenant creates a new tenant
func (s *TenantService) CreateTenant(
    ctx context.Context,
    tenantID string,
    replicationFactor int,
) (*model.Tenant, error) {
    // Validate replication factor
    if replicationFactor < 1 || replicationFactor > 5 {
        return nil, fmt.Errorf("invalid replication factor: %d", replicationFactor)
    }

    tenant := &model.Tenant{
        TenantID:          tenantID,
        ReplicationFactor: replicationFactor,
        CreatedAt:         time.Now(),
        UpdatedAt:         time.Now(),
        Version:           1,
    }

    if err := s.metadataStore.CreateTenant(ctx, tenant); err != nil {
        return nil, fmt.Errorf("failed to create tenant: %w", err)
    }

    // Cache the new tenant
    s.cache.SetTenant(tenantID, tenant)

    return tenant, nil
}

// UpdateReplicationFactor updates the replication factor for a tenant
func (s *TenantService) UpdateReplicationFactor(
    ctx context.Context,
    tenantID string,
    newReplicationFactor int,
) (*model.Tenant, error) {
    // Get current tenant
    tenant, err := s.GetTenant(ctx, tenantID)
    if err != nil {
        return nil, err
    }

    // Validate
    if newReplicationFactor < 1 || newReplicationFactor > 5 {
        return nil, fmt.Errorf("invalid replication factor: %d", newReplicationFactor)
    }

    // Update
    tenant.ReplicationFactor = newReplicationFactor
    tenant.UpdatedAt = time.Now()
    tenant.Version++

    if err := s.metadataStore.UpdateTenant(ctx, tenant); err != nil {
        return nil, fmt.Errorf("failed to update tenant: %w", err)
    }

    // Invalidate cache
    s.cache.DeleteTenant(tenantID)

    return tenant, nil
}
```

### 5.3 Routing Service (Consistent Hashing)

```go
// service/routing_service.go
package service

import (
    "context"
    "fmt"
    "sync"
    "time"

    "go.uber.org/zap"
)

type RoutingService struct {
    hashRing      *HashRing
    metadataStore *MetadataStore
    hasher        *ConsistentHasher
    logger        *zap.Logger
    mu            sync.RWMutex
    updateTicker  *time.Ticker
}

func NewRoutingService(
    metadataStore *MetadataStore,
    hasher *ConsistentHasher,
    updateInterval time.Duration,
    logger *zap.Logger,
) *RoutingService {
    rs := &RoutingService{
        hashRing:      &HashRing{Nodes: make(map[string]*model.StorageNode)},
        metadataStore: metadataStore,
        hasher:        hasher,
        logger:        logger,
        updateTicker:  time.NewTicker(updateInterval),
    }

    // Start background refresh
    go rs.refreshHashRing()

    return rs
}

// GetReplicas returns the list of storage nodes for a given key
func (s *RoutingService) GetReplicas(tenantID, key string, replicationFactor int) ([]*model.StorageNode, error) {
    s.mu.RLock()
    defer s.mu.RUnlock()

    // Compute hash
    hashKey := fmt.Sprintf("%s:%s", tenantID, key)
    hash := s.hasher.Hash(hashKey)

    // Find replicas using consistent hashing
    replicas := s.hasher.GetNodes(hash, replicationFactor)

    if len(replicas) < replicationFactor {
        return nil, fmt.Errorf("insufficient replicas: got %d, need %d", len(replicas), replicationFactor)
    }

    // Convert to storage nodes
    nodes := make([]*model.StorageNode, 0, len(replicas))
    for _, vnode := range replicas {
        if node, exists := s.hashRing.Nodes[vnode.NodeID]; exists {
            nodes = append(nodes, node)
        }
    }

    return nodes, nil
}

// refreshHashRing periodically updates the hash ring from metadata store
func (s *RoutingService) refreshHashRing() {
    for range s.updateTicker.C {
        if err := s.updateHashRing(context.Background()); err != nil {
            s.logger.Error("Failed to update hash ring", zap.Error(err))
        }
    }
}

// updateHashRing fetches latest node list and rebuilds hash ring
func (s *RoutingService) updateHashRing(ctx context.Context) error {
    nodes, err := s.metadataStore.GetStorageNodes(ctx)
    if err != nil {
        return fmt.Errorf("failed to get storage nodes: %w", err)
    }

    s.mu.Lock()
    defer s.mu.Unlock()

    // Rebuild hash ring
    s.hashRing = &HashRing{
        Nodes:       make(map[string]*model.StorageNode),
        LastUpdated: time.Now(),
    }

    // Clear hasher
    s.hasher.Clear()

    // Add all active nodes
    for _, node := range nodes {
        if node.Status == model.NodeStatusActive {
            s.hashRing.Nodes[node.NodeID] = node
            s.hasher.AddNode(node.NodeID, node.VirtualNodes)
        }
    }

    s.logger.Info("Hash ring updated", zap.Int("node_count", len(s.hashRing.Nodes)))
    return nil
}
```

### 5.4 Vector Clock Service

```go
// service/vectorclock_service.go
package service

import (
    "sync"
)

type VectorClockService struct {
    nodeID          string
    localTimestamp  int64
    mu              sync.Mutex
}

func NewVectorClockService(nodeID string) *VectorClockService {
    return &VectorClockService{
        nodeID:         nodeID,
        localTimestamp: 0,
    }
}

// Increment increments the vector clock for this coordinator
func (s *VectorClockService) Increment(nodeID string) model.VectorClock {
    s.mu.Lock()
    defer s.mu.Unlock()

    s.localTimestamp++
    return model.VectorClock{
        Entries: []model.VectorClockEntry{
            {
                CoordinatorNodeID: s.nodeID,
                LogicalTimestamp:  s.localTimestamp,
            },
        },
    }
}

// Merge merges multiple vector clocks
func (s *VectorClockService) Merge(clocks ...model.VectorClock) model.VectorClock {
    merged := make(map[string]int64)

    for _, clock := range clocks {
        for _, entry := range clock.Entries {
            if existing, exists := merged[entry.CoordinatorNodeID]; !exists || entry.LogicalTimestamp > existing {
                merged[entry.CoordinatorNodeID] = entry.LogicalTimestamp
            }
        }
    }

    entries := make([]model.VectorClockEntry, 0, len(merged))
    for nodeID, timestamp := range merged {
        entries = append(entries, model.VectorClockEntry{
            CoordinatorNodeID: nodeID,
            LogicalTimestamp:  timestamp,
        })
    }

    return model.VectorClock{Entries: entries}
}

// Compare compares two vector clocks
func (s *VectorClockService) Compare(vc1, vc2 model.VectorClock) model.VectorClockComparison {
    // Build maps for easier comparison
    map1 := s.toMap(vc1)
    map2 := s.toMap(vc2)

    allBefore := true
    allAfter := true

    // Get all node IDs
    allNodes := make(map[string]bool)
    for nodeID := range map1 {
        allNodes[nodeID] = true
    }
    for nodeID := range map2 {
        allNodes[nodeID] = true
    }

    // Compare timestamps
    for nodeID := range allNodes {
        ts1 := map1[nodeID]
        ts2 := map2[nodeID]

        if ts1 < ts2 {
            allAfter = false
        } else if ts1 > ts2 {
            allBefore = false
        }
    }

    // Determine relationship
    if allBefore && allAfter {
        return model.Identical
    }
    if allBefore {
        return model.Before
    }
    if allAfter {
        return model.After
    }
    return model.Concurrent
}

func (s *VectorClockService) toMap(vc model.VectorClock) map[string]int64 {
    m := make(map[string]int64)
    for _, entry := range vc.Entries {
        m[entry.CoordinatorNodeID] = entry.LogicalTimestamp
    }
    return m
}
```

### 5.5 Consistency Service

```go
// service/consistency_service.go
package service

type ConsistencyService struct {
    defaultLevel string
}

func NewConsistencyService(defaultLevel string) *ConsistencyService {
    return &ConsistencyService{
        defaultLevel: defaultLevel,
    }
}

// GetRequiredReplicas calculates how many replicas must respond
func (s *ConsistencyService) GetRequiredReplicas(consistency string, totalReplicas int) int {
    switch consistency {
    case "one":
        return 1
    case "all":
        return totalReplicas
    case "quorum":
        fallthrough
    default:
        return (totalReplicas / 2) + 1
    }
}

// ValidateConsistencyLevel checks if consistency level is valid
func (s *ConsistencyService) ValidateConsistencyLevel(level string) bool {
    switch level {
    case "one", "quorum", "all":
        return true
    default:
        return false
    }
}
```

### 5.6 Idempotency Service

```go
// service/idempotency_service.go
package service

import (
    "context"
    "encoding/json"
    "fmt"
    "time"

    "github.com/google/uuid"
    "go.uber.org/zap"
)

type IdempotencyService struct {
    store  *IdempotencyStore
    ttl    time.Duration
    logger *zap.Logger
}

func NewIdempotencyService(
    store *IdempotencyStore,
    ttl time.Duration,
    logger *zap.Logger,
) *IdempotencyService {
    return &IdempotencyService{
        store:  store,
        ttl:    ttl,
        logger: logger,
    }
}

// Generate creates a new idempotency key
func (s *IdempotencyService) Generate(tenantID, key string) string {
    return fmt.Sprintf("%s-%s-%s", tenantID, key, uuid.New().String())
}

// Get retrieves a cached response
func (s *IdempotencyService) Get(
    ctx context.Context,
    tenantID, key, idempotencyKey string,
) (*WriteResponse, error) {
    data, err := s.store.Get(ctx, tenantID, key, idempotencyKey)
    if err != nil {
        return nil, err
    }

    var response WriteResponse
    if err := json.Unmarshal(data, &response); err != nil {
        return nil, fmt.Errorf("failed to unmarshal response: %w", err)
    }

    response.IsDuplicate = true
    return &response, nil
}

// Store saves a response for idempotency
func (s *IdempotencyService) Store(
    ctx context.Context,
    tenantID, key, idempotencyKey string,
    response *WriteResponse,
) error {
    data, err := json.Marshal(response)
    if err != nil {
        return fmt.Errorf("failed to marshal response: %w", err)
    }

    return s.store.Set(ctx, tenantID, key, idempotencyKey, data, s.ttl)
}
```

### 5.7 Conflict Resolution Service

```go
// service/conflict_service.go
package service

import (
    "context"
    "time"

    "go.uber.org/zap"
)

type ConflictService struct {
    vectorClockService *VectorClockService
    storageClient      *StorageClient
    logger             *zap.Logger
    repairQueue        chan *model.RepairRequest
}

func NewConflictService(
    vectorClockService *VectorClockService,
    storageClient *StorageClient,
    logger *zap.Logger,
) *ConflictService {
    cs := &ConflictService{
        vectorClockService: vectorClockService,
        storageClient:      storageClient,
        logger:             logger,
        repairQueue:        make(chan *model.RepairRequest, 1000),
    }

    // Start repair workers
    for i := 0; i < 5; i++ {
        go cs.repairWorker()
    }

    return cs
}

// DetectConflicts analyzes responses and finds conflicts
func (s *ConflictService) DetectConflicts(responses []*StorageResponse) (*StorageResponse, []*StorageResponse) {
    if len(responses) == 0 {
        return nil, nil
    }

    // Group responses by vector clock
    groups := make(map[string][]*StorageResponse)

    for _, resp := range responses {
        key := s.vectorClockKey(resp.VectorClock)
        groups[key] = append(groups[key], resp)
    }

    // Find latest version
    var latest *StorageResponse
    var latestTimestamp int64

    for _, groupResponses := range groups {
        resp := groupResponses[0]
        timestamp := s.getMaxTimestamp(resp.VectorClock)
        if timestamp > latestTimestamp {
            latestTimestamp = timestamp
            latest = resp
        }
    }

    // Collect all conflicting versions
    conflicts := make([]*StorageResponse, 0)
    for _, resp := range responses {
        comparison := s.vectorClockService.Compare(resp.VectorClock, latest.VectorClock)
        if comparison == model.Concurrent {
            conflicts = append(conflicts, resp)
        }
    }

    return latest, conflicts
}

// TriggerRepair adds a repair request to the queue
func (s *ConflictService) TriggerRepair(
    ctx context.Context,
    tenantID, key string,
    latest *StorageResponse,
    replicas []*model.StorageNode,
) {
    repairReq := &model.RepairRequest{
        TenantID:    tenantID,
        Key:         key,
        Value:       latest.Value,
        VectorClock: latest.VectorClock,
        Timestamp:   time.Now().Unix(),
    }

    select {
    case s.repairQueue <- repairReq:
        s.logger.Debug("Repair request queued", zap.String("key", key))
    default:
        s.logger.Warn("Repair queue full, dropping request", zap.String("key", key))
    }
}

// repairWorker processes repair requests
func (s *ConflictService) repairWorker() {
    for req := range s.repairQueue {
        ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)

        if err := s.executeRepair(ctx, req); err != nil {
            s.logger.Error("Repair failed",
                zap.String("tenant_id", req.TenantID),
                zap.String("key", req.Key),
                zap.Error(err))
        }

        cancel()
    }
}

// executeRepair performs the actual repair
func (s *ConflictService) executeRepair(ctx context.Context, req *model.RepairRequest) error {
    // Implementation would write the latest value to all replicas
    s.logger.Info("Executing repair",
        zap.String("tenant_id", req.TenantID),
        zap.String("key", req.Key))

    // TODO: Implement actual repair logic
    return nil
}

func (s *ConflictService) vectorClockKey(vc model.VectorClock) string {
    // Simple string representation for grouping
    key := ""
    for _, entry := range vc.Entries {
        key += fmt.Sprintf("%s:%d;", entry.CoordinatorNodeID, entry.LogicalTimestamp)
    }
    return key
}

func (s *ConflictService) getMaxTimestamp(vc model.VectorClock) int64 {
    var max int64
    for _, entry := range vc.Entries {
        if entry.LogicalTimestamp > max {
            max = entry.LogicalTimestamp
        }
    }
    return max
}
```

## 6. Algorithm Implementations

### 6.1 Consistent Hashing

```go
// algorithm/consistent_hash.go
package algorithm

import (
    "crypto/sha256"
    "encoding/binary"
    "fmt"
    "sort"
    "sync"
)

type ConsistentHasher struct {
    ring         []uint64              // Sorted hash values
    ringMap      map[uint64]string     // Hash -> VNodeID
    nodeVNodes   map[string][]uint64   // NodeID -> VNode hashes
    mu           sync.RWMutex
}

func NewConsistentHasher() *ConsistentHasher {
    return &ConsistentHasher{
        ring:       make([]uint64, 0),
        ringMap:    make(map[uint64]string),
        nodeVNodes: make(map[string][]uint64),
    }
}

// AddNode adds a physical node with virtual nodes
func (ch *ConsistentHasher) AddNode(nodeID string, virtualNodeCount int) {
    ch.mu.Lock()
    defer ch.mu.Unlock()

    vnodeHashes := make([]uint64, 0, virtualNodeCount)

    for i := 0; i < virtualNodeCount; i++ {
        vnodeID := fmt.Sprintf("%s-vnode-%d", nodeID, i)
        hash := ch.hash(vnodeID)

        ch.ring = append(ch.ring, hash)
        ch.ringMap[hash] = vnodeID
        vnodeHashes = append(vnodeHashes, hash)
    }

    ch.nodeVNodes[nodeID] = vnodeHashes
    sort.Slice(ch.ring, func(i, j int) bool { return ch.ring[i] < ch.ring[j] })
}

// RemoveNode removes a physical node and its virtual nodes
func (ch *ConsistentHasher) RemoveNode(nodeID string) {
    ch.mu.Lock()
    defer ch.mu.Unlock()

    vnodeHashes, exists := ch.nodeVNodes[nodeID]
    if !exists {
        return
    }

    // Remove from ring
    hashSet := make(map[uint64]bool)
    for _, hash := range vnodeHashes {
        hashSet[hash] = true
        delete(ch.ringMap, hash)
    }

    newRing := make([]uint64, 0, len(ch.ring)-len(vnodeHashes))
    for _, hash := range ch.ring {
        if !hashSet[hash] {
            newRing = append(newRing, hash)
        }
    }
    ch.ring = newRing

    delete(ch.nodeVNodes, nodeID)
}

// GetNodes returns N replica nodes for a given hash
func (ch *ConsistentHasher) GetNodes(keyHash uint64, count int) []*VirtualNode {
    ch.mu.RLock()
    defer ch.mu.RUnlock()

    if len(ch.ring) == 0 {
        return nil
    }

    // Find position in ring
    idx := sort.Search(len(ch.ring), func(i int) bool {
        return ch.ring[i] >= keyHash
    })

    // Wrap around if necessary
    if idx >= len(ch.ring) {
        idx = 0
    }

    // Collect unique physical nodes
    nodes := make([]*VirtualNode, 0, count)
    seen := make(map[string]bool)

    for i := 0; i < len(ch.ring) && len(nodes) < count; i++ {
        ringIdx := (idx + i) % len(ch.ring)
        hash := ch.ring[ringIdx]
        vnodeID := ch.ringMap[hash]

        // Extract physical node ID (format: nodeID-vnode-X)
        nodeID := ch.extractNodeID(vnodeID)

        if !seen[nodeID] {
            nodes = append(nodes, &VirtualNode{
                VNodeID: vnodeID,
                Hash:    hash,
                NodeID:  nodeID,
            })
            seen[nodeID] = true
        }
    }

    return nodes
}

// Hash computes hash for a key
func (ch *ConsistentHasher) Hash(key string) uint64 {
    return ch.hash(key)
}

// Clear removes all nodes
func (ch *ConsistentHasher) Clear() {
    ch.mu.Lock()
    defer ch.mu.Unlock()

    ch.ring = make([]uint64, 0)
    ch.ringMap = make(map[uint64]string)
    ch.nodeVNodes = make(map[string][]uint64)
}

// hash computes SHA-256 hash and converts to uint64
func (ch *ConsistentHasher) hash(key string) uint64 {
    h := sha256.New()
    h.Write([]byte(key))
    hashBytes := h.Sum(nil)
    return binary.BigEndian.Uint64(hashBytes[:8])
}

// extractNodeID extracts physical node ID from virtual node ID
func (ch *ConsistentHasher) extractNodeID(vnodeID string) string {
    // Format: nodeID-vnode-X
    // Simple implementation - in production, use better parsing
    for i := len(vnodeID) - 1; i >= 0; i-- {
        if vnodeID[i] == '-' {
            for j := i - 1; j >= 0; j-- {
                if vnodeID[j] == '-' {
                    return vnodeID[:j]
                }
            }
        }
    }
    return vnodeID
}

type VirtualNode struct {
    VNodeID string
    Hash    uint64
    NodeID  string
}
```

## 7. Storage Layer

### 7.1 Metadata Store (PostgreSQL)

```go
// store/metadata_store.go
package store

import (
    "context"
    "fmt"

    "github.com/jackc/pgx/v5/pgxpool"
)

type MetadataStore struct {
    pool *pgxpool.Pool
}

func NewMetadataStore(connString string) (*MetadataStore, error) {
    pool, err := pgxpool.New(context.Background(), connString)
    if err != nil {
        return nil, fmt.Errorf("failed to create connection pool: %w", err)
    }

    return &MetadataStore{pool: pool}, nil
}

// GetTenant retrieves tenant configuration
func (s *MetadataStore) GetTenant(ctx context.Context, tenantID string) (*model.Tenant, error) {
    query := `
        SELECT tenant_id, replication_factor, created_at, updated_at, version
        FROM tenants
        WHERE tenant_id = $1
    `

    var tenant model.Tenant
    err := s.pool.QueryRow(ctx, query, tenantID).Scan(
        &tenant.TenantID,
        &tenant.ReplicationFactor,
        &tenant.CreatedAt,
        &tenant.UpdatedAt,
        &tenant.Version,
    )

    if err != nil {
        return nil, fmt.Errorf("failed to get tenant: %w", err)
    }

    return &tenant, nil
}

// CreateTenant creates a new tenant
func (s *MetadataStore) CreateTenant(ctx context.Context, tenant *model.Tenant) error {
    query := `
        INSERT INTO tenants (tenant_id, replication_factor, created_at, updated_at, version)
        VALUES ($1, $2, $3, $4, $5)
    `

    _, err := s.pool.Exec(ctx, query,
        tenant.TenantID,
        tenant.ReplicationFactor,
        tenant.CreatedAt,
        tenant.UpdatedAt,
        tenant.Version,
    )

    return err
}

// UpdateTenant updates tenant configuration
func (s *MetadataStore) UpdateTenant(ctx context.Context, tenant *model.Tenant) error {
    query := `
        UPDATE tenants
        SET replication_factor = $2, updated_at = $3, version = $4
        WHERE tenant_id = $1 AND version = $5
    `

    result, err := s.pool.Exec(ctx, query,
        tenant.TenantID,
        tenant.ReplicationFactor,
        tenant.UpdatedAt,
        tenant.Version,
        tenant.Version-1, // Optimistic locking
    )

    if err != nil {
        return err
    }

    if result.RowsAffected() == 0 {
        return fmt.Errorf("tenant not found or version mismatch")
    }

    return nil
}

// GetStorageNodes retrieves all storage nodes
func (s *MetadataStore) GetStorageNodes(ctx context.Context) ([]*model.StorageNode, error) {
    query := `
        SELECT node_id, host, port, status, virtual_nodes
        FROM storage_nodes
        WHERE status != 'inactive'
    `

    rows, err := s.pool.Query(ctx, query)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    nodes := make([]*model.StorageNode, 0)
    for rows.Next() {
        var node model.StorageNode
        if err := rows.Scan(&node.NodeID, &node.Host, &node.Port, &node.Status, &node.VirtualNodes); err != nil {
            return nil, err
        }
        nodes = append(nodes, &node)
    }

    return nodes, rows.Err()
}

func (s *MetadataStore) Close() {
    s.pool.Close()
}
```

### 7.2 Idempotency Store (Redis)

```go
// store/idempotency_store.go
package store

import (
    "context"
    "fmt"
    "time"

    "github.com/redis/go-redis/v9"
)

type IdempotencyStore struct {
    client *redis.Client
}

func NewIdempotencyStore(addr, password string, db int) *IdempotencyStore {
    client := redis.NewClient(&redis.Options{
        Addr:     addr,
        Password: password,
        DB:       db,
    })

    return &IdempotencyStore{client: client}
}

// Get retrieves cached response
func (s *IdempotencyStore) Get(ctx context.Context, tenantID, key, idempotencyKey string) ([]byte, error) {
    redisKey := s.buildKey(tenantID, key, idempotencyKey)
    data, err := s.client.Get(ctx, redisKey).Bytes()
    if err != nil {
        return nil, err
    }
    return data, nil
}

// Set stores response with TTL
func (s *IdempotencyStore) Set(
    ctx context.Context,
    tenantID, key, idempotencyKey string,
    data []byte,
    ttl time.Duration,
) error {
    redisKey := s.buildKey(tenantID, key, idempotencyKey)
    return s.client.Set(ctx, redisKey, data, ttl).Err()
}

func (s *IdempotencyStore) buildKey(tenantID, key, idempotencyKey string) string {
    return fmt.Sprintf("idempotency:%s:%s:%s", tenantID, key, idempotencyKey)
}

func (s *IdempotencyStore) Close() error {
    return s.client.Close()
}
```

### 7.3 Cache Layer

```go
// store/cache.go
package store

import (
    "sync"
    "time"
)

type Cache struct {
    tenantCache map[string]*cacheEntry
    mu          sync.RWMutex
    ttl         time.Duration
}

type cacheEntry struct {
    tenant    *model.Tenant
    expiresAt time.Time
}

func NewCache(ttl time.Duration) *Cache {
    c := &Cache{
        tenantCache: make(map[string]*cacheEntry),
        ttl:         ttl,
    }

    // Start cleanup goroutine
    go c.cleanup()

    return c
}

func (c *Cache) GetTenant(tenantID string) (*model.Tenant, bool) {
    c.mu.RLock()
    defer c.mu.RUnlock()

    entry, exists := c.tenantCache[tenantID]
    if !exists || time.Now().After(entry.expiresAt) {
        return nil, false
    }

    return entry.tenant, true
}

func (c *Cache) SetTenant(tenantID string, tenant *model.Tenant) {
    c.mu.Lock()
    defer c.mu.Unlock()

    c.tenantCache[tenantID] = &cacheEntry{
        tenant:    tenant,
        expiresAt: time.Now().Add(c.ttl),
    }
}

func (c *Cache) DeleteTenant(tenantID string) {
    c.mu.Lock()
    defer c.mu.Unlock()

    delete(c.tenantCache, tenantID)
}

func (c *Cache) cleanup() {
    ticker := time.NewTicker(1 * time.Minute)
    defer ticker.Stop()

    for range ticker.C {
        c.mu.Lock()
        now := time.Now()
        for tenantID, entry := range c.tenantCache {
            if now.After(entry.expiresAt) {
                delete(c.tenantCache, tenantID)
            }
        }
        c.mu.Unlock()
    }
}
```

## 8. Client Layer

```go
// client/storage_client.go
package client

import (
    "context"
    "fmt"
    "sync"
    "time"

    "google.golang.org/grpc"
    "google.golang.org/grpc/credentials/insecure"
)

type StorageClient struct {
    connections map[string]*grpc.ClientConn
    mu          sync.RWMutex
}

func NewStorageClient() *StorageClient {
    return &StorageClient{
        connections: make(map[string]*grpc.ClientConn),
    }
}

// Write sends write request to storage node
func (c *StorageClient) Write(
    ctx context.Context,
    node *model.StorageNode,
    tenantID, key string,
    value []byte,
    vectorClock model.VectorClock,
) (*StorageResponse, error) {
    conn, err := c.getConnection(node)
    if err != nil {
        return nil, err
    }

    client := pb.NewStorageServiceClient(conn)

    resp, err := client.Write(ctx, &pb.WriteRequest{
        TenantId:    tenantID,
        Key:         key,
        Value:       value,
        VectorClock: c.toProtoVectorClock(vectorClock),
    })

    if err != nil {
        return nil, err
    }

    return &StorageResponse{
        NodeID:      node.NodeID,
        Success:     resp.Success,
        VectorClock: c.fromProtoVectorClock(resp.VectorClock),
    }, nil
}

// Read sends read request to storage node
func (c *StorageClient) Read(
    ctx context.Context,
    node *model.StorageNode,
    tenantID, key string,
) (*StorageResponse, error) {
    conn, err := c.getConnection(node)
    if err != nil {
        return nil, err
    }

    client := pb.NewStorageServiceClient(conn)

    resp, err := client.Read(ctx, &pb.ReadRequest{
        TenantId: tenantID,
        Key:      key,
    })

    if err != nil {
        return nil, err
    }

    return &StorageResponse{
        NodeID:      node.NodeID,
        Success:     resp.Success,
        Value:       resp.Value,
        VectorClock: c.fromProtoVectorClock(resp.VectorClock),
    }, nil
}

// getConnection returns or creates a gRPC connection
func (c *StorageClient) getConnection(node *model.StorageNode) (*grpc.ClientConn, error) {
    addr := fmt.Sprintf("%s:%d", node.Host, node.Port)

    c.mu.RLock()
    conn, exists := c.connections[addr]
    c.mu.RUnlock()

    if exists {
        return conn, nil
    }

    c.mu.Lock()
    defer c.mu.Unlock()

    // Double-check
    if conn, exists := c.connections[addr]; exists {
        return conn, nil
    }

    // Create new connection
    conn, err := grpc.Dial(addr,
        grpc.WithTransportCredentials(insecure.NewCredentials()),
        grpc.WithTimeout(5*time.Second),
    )

    if err != nil {
        return nil, fmt.Errorf("failed to connect to %s: %w", addr, err)
    }

    c.connections[addr] = conn
    return conn, nil
}

func (c *StorageClient) toProtoVectorClock(vc model.VectorClock) *pb.VectorClock {
    entries := make([]*pb.VectorClockEntry, len(vc.Entries))
    for i, entry := range vc.Entries {
        entries[i] = &pb.VectorClockEntry{
            CoordinatorNodeId: entry.CoordinatorNodeID,
            LogicalTimestamp:  entry.LogicalTimestamp,
        }
    }
    return &pb.VectorClock{Entries: entries}
}

func (c *StorageClient) fromProtoVectorClock(vc *pb.VectorClock) model.VectorClock {
    entries := make([]model.VectorClockEntry, len(vc.Entries))
    for i, entry := range vc.Entries {
        entries[i] = model.VectorClockEntry{
            CoordinatorNodeID: entry.CoordinatorNodeId,
            LogicalTimestamp:  entry.LogicalTimestamp,
        }
    }
    return model.VectorClock{Entries: entries}
}

func (c *StorageClient) Close() {
    c.mu.Lock()
    defer c.mu.Unlock()

    for _, conn := range c.connections {
        conn.Close()
    }
}
```

## 9. gRPC Handler Implementation

```go
// handler/keyvalue_handler.go
package handler

import (
    "context"

    "go.uber.org/zap"
    "google.golang.org/grpc/codes"
    "google.golang.org/grpc/status"
)

type KeyValueHandler struct {
    coordinatorService *service.CoordinatorService
    logger             *zap.Logger
    pb.UnimplementedCoordinatorServiceServer
}

func NewKeyValueHandler(
    coordinatorService *service.CoordinatorService,
    logger *zap.Logger,
) *KeyValueHandler {
    return &KeyValueHandler{
        coordinatorService: coordinatorService,
        logger:             logger,
    }
}

// WriteKeyValue handles write requests
func (h *KeyValueHandler) WriteKeyValue(
    ctx context.Context,
    req *pb.WriteKeyValueRequest,
) (*pb.WriteKeyValueResponse, error) {
    // Validate request
    if err := h.validateWriteRequest(req); err != nil {
        return nil, status.Error(codes.InvalidArgument, err.Error())
    }

    // Execute write
    resp, err := h.coordinatorService.WriteKeyValue(
        ctx,
        req.TenantId,
        req.Key,
        req.Value,
        req.Consistency,
        req.IdempotencyKey,
    )

    if err != nil {
        h.logger.Error("Write failed",
            zap.String("tenant_id", req.TenantId),
            zap.String("key", req.Key),
            zap.Error(err))
        return nil, status.Error(codes.Internal, "write failed")
    }

    return &pb.WriteKeyValueResponse{
        Success:        resp.Success,
        Key:            resp.Key,
        IdempotencyKey: resp.IdempotencyKey,
        VectorClock:    h.toProtoVectorClock(resp.VectorClock),
        ReplicaCount:   int32(resp.ReplicaCount),
        Consistency:    resp.Consistency,
        IsDuplicate:    resp.IsDuplicate,
    }, nil
}

// ReadKeyValue handles read requests
func (h *KeyValueHandler) ReadKeyValue(
    ctx context.Context,
    req *pb.ReadKeyValueRequest,
) (*pb.ReadKeyValueResponse, error) {
    // Validate request
    if err := h.validateReadRequest(req); err != nil {
        return nil, status.Error(codes.InvalidArgument, err.Error())
    }

    // Execute read
    resp, err := h.coordinatorService.ReadKeyValue(
        ctx,
        req.TenantId,
        req.Key,
        req.Consistency,
    )

    if err != nil {
        h.logger.Error("Read failed",
            zap.String("tenant_id", req.TenantId),
            zap.String("key", req.Key),
            zap.Error(err))
        return nil, status.Error(codes.Internal, "read failed")
    }

    return &pb.ReadKeyValueResponse{
        Success:     resp.Success,
        Key:         resp.Key,
        Value:       resp.Value,
        VectorClock: h.toProtoVectorClock(resp.VectorClock),
    }, nil
}

func (h *KeyValueHandler) validateWriteRequest(req *pb.WriteKeyValueRequest) error {
    if req.TenantId == "" {
        return fmt.Errorf("tenant_id is required")
    }
    if req.Key == "" {
        return fmt.Errorf("key is required")
    }
    if req.Value == nil {
        return fmt.Errorf("value is required")
    }
    return nil
}

func (h *KeyValueHandler) validateReadRequest(req *pb.ReadKeyValueRequest) error {
    if req.TenantId == "" {
        return fmt.Errorf("tenant_id is required")
    }
    if req.Key == "" {
        return fmt.Errorf("key is required")
    }
    return nil
}

func (h *KeyValueHandler) toProtoVectorClock(vc model.VectorClock) *pb.VectorClock {
    entries := make([]*pb.VectorClockEntry, len(vc.Entries))
    for i, entry := range vc.Entries {
        entries[i] = &pb.VectorClockEntry{
            CoordinatorNodeId: entry.CoordinatorNodeID,
            LogicalTimestamp:  entry.LogicalTimestamp,
        }
    }
    return &pb.VectorClock{Entries: entries}
}
```

## 10. Main Entry Point

```go
// cmd/coordinator/main.go
package main

import (
    "context"
    "fmt"
    "net"
    "os"
    "os/signal"
    "syscall"

    "go.uber.org/zap"
    "google.golang.org/grpc"
)

func main() {
    // Initialize logger
    logger, _ := zap.NewProduction()
    defer logger.Sync()

    // Load configuration
    cfg, err := config.Load()
    if err != nil {
        logger.Fatal("Failed to load config", zap.Error(err))
    }

    // Initialize metadata store
    metadataStore, err := store.NewMetadataStore(cfg.Database.ConnectionString())
    if err != nil {
        logger.Fatal("Failed to initialize metadata store", zap.Error(err))
    }
    defer metadataStore.Close()

    // Initialize idempotency store
    idempotencyStore := store.NewIdempotencyStore(
        cfg.Redis.Address(),
        cfg.Redis.Password,
        cfg.Redis.DB,
    )
    defer idempotencyStore.Close()

    // Initialize cache
    cache := store.NewCache(cfg.Cache.TenantConfigTTL)

    // Initialize services
    tenantService := service.NewTenantService(metadataStore, cache, logger)
    vectorClockService := service.NewVectorClockService(cfg.Server.NodeID)
    consistencyService := service.NewConsistencyService(cfg.Consistency.DefaultLevel)
    idempotencyService := service.NewIdempotencyService(idempotencyStore, 24*time.Hour, logger)

    storageClient := client.NewStorageClient()
    defer storageClient.Close()

    conflictService := service.NewConflictService(vectorClockService, storageClient, logger)

    hasher := algorithm.NewConsistentHasher()
    routingService := service.NewRoutingService(metadataStore, hasher, cfg.HashRing.UpdateInterval, logger)

    coordinatorService := service.NewCoordinatorService(
        tenantService,
        routingService,
        consistencyService,
        vectorClockService,
        idempotencyService,
        conflictService,
        storageClient,
        logger,
        cfg.Server.NodeID,
    )

    // Initialize handlers
    kvHandler := handler.NewKeyValueHandler(coordinatorService, logger)
    tenantHandler := handler.NewTenantHandler(tenantService, logger)

    // Create gRPC server
    grpcServer := grpc.NewServer(
        grpc.MaxConcurrentStreams(uint32(cfg.Server.MaxConnections)),
    )

    pb.RegisterCoordinatorServiceServer(grpcServer, kvHandler)

    // Start listening
    addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
    listener, err := net.Listen("tcp", addr)
    if err != nil {
        logger.Fatal("Failed to listen", zap.Error(err))
    }

    logger.Info("Coordinator service starting", zap.String("address", addr))

    // Handle graceful shutdown
    go func() {
        sigChan := make(chan os.Signal, 1)
        signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
        <-sigChan

        logger.Info("Shutting down gracefully...")
        grpcServer.GracefulStop()
    }()

    // Start server
    if err := grpcServer.Serve(listener); err != nil {
        logger.Fatal("Failed to serve", zap.Error(err))
    }
}
```

## 11. Database Schema

```sql
-- Tenants table
CREATE TABLE tenants (
    tenant_id VARCHAR(255) PRIMARY KEY,
    replication_factor INTEGER NOT NULL DEFAULT 3,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    version BIGINT NOT NULL DEFAULT 1
);

CREATE INDEX idx_tenants_updated_at ON tenants(updated_at);

-- Storage nodes table
CREATE TABLE storage_nodes (
    node_id VARCHAR(255) PRIMARY KEY,
    host VARCHAR(255) NOT NULL,
    port INTEGER NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'active',
    virtual_nodes INTEGER NOT NULL DEFAULT 150,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_storage_nodes_status ON storage_nodes(status);

-- Migrations table
CREATE TABLE migrations (
    migration_id VARCHAR(255) PRIMARY KEY,
    type VARCHAR(50) NOT NULL,
    node_id VARCHAR(255) NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'pending',
    phase VARCHAR(50),
    progress JSONB,
    started_at TIMESTAMP NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMP,
    error_message TEXT
);

CREATE INDEX idx_migrations_status ON migrations(status);
CREATE INDEX idx_migrations_node_id ON migrations(node_id);

-- Migration checkpoints table
CREATE TABLE migration_checkpoints (
    migration_id VARCHAR(255) NOT NULL,
    key_range_start VARCHAR(255) NOT NULL,
    key_range_end VARCHAR(255) NOT NULL,
    last_migrated_key VARCHAR(255),
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    PRIMARY KEY (migration_id, key_range_start),
    FOREIGN KEY (migration_id) REFERENCES migrations(migration_id) ON DELETE CASCADE
);
```

## 12. Configuration File Example

```yaml
# config.yaml
server:
  host: "0.0.0.0"
  port: 50051
  node_id: "coordinator-1"
  max_connections: 1000
  read_timeout: 10s
  write_timeout: 10s
  shutdown_timeout: 30s

database:
  host: "localhost"
  port: 5432
  database: "pairdb_metadata"
  user: "coordinator"
  password: "secret"
  max_connections: 50
  min_connections: 10
  conn_max_lifetime: 30m

redis:
  host: "localhost"
  port: 6379
  password: ""
  db: 0
  max_retries: 3
  pool_size: 100
  min_idle_conns: 10

hash_ring:
  virtual_nodes: 150
  update_interval: 30s

consistency:
  default_level: "quorum"
  write_timeout: 5s
  read_timeout: 3s
  repair_async: true

cache:
  tenant_config_ttl: 5m
  max_size: 10000

metrics:
  enabled: true
  port: 9090
  path: "/metrics"

logging:
  level: "info"
  format: "json"
```

## 13. Monitoring and Metrics

```go
// metrics/prometheus.go
package metrics

import (
    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promauto"
)

var (
    RequestsTotal = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "coordinator_requests_total",
            Help: "Total number of requests",
        },
        []string{"operation", "status"},
    )

    RequestDuration = promauto.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "coordinator_request_duration_seconds",
            Help:    "Request duration in seconds",
            Buckets: prometheus.DefBuckets,
        },
        []string{"operation"},
    )

    CacheHits = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "coordinator_cache_hits_total",
            Help: "Total cache hits",
        },
        []string{"cache_type"},
    )

    CacheMisses = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "coordinator_cache_misses_total",
            Help: "Total cache misses",
        },
        []string{"cache_type"},
    )

    ConflictsDetected = promauto.NewCounter(
        prometheus.CounterOpts{
            Name: "coordinator_conflicts_detected_total",
            Help: "Total number of conflicts detected",
        },
    )

    RepairsTriggered = promauto.NewCounter(
        prometheus.CounterOpts{
            Name: "coordinator_repairs_triggered_total",
            Help: "Total number of repairs triggered",
        },
    )

    QuorumFailures = promauto.NewCounter(
        prometheus.CounterOpts{
            Name: "coordinator_quorum_failures_total",
            Help: "Total number of quorum failures",
        },
    )
)
```

## 14. Testing Strategy

### 14.1 Unit Tests
- Test vector clock comparison logic
- Test consistent hashing algorithm
- Test quorum calculation
- Test idempotency key generation

### 14.2 Integration Tests
- Test write and read operations end-to-end
- Test conflict detection and resolution
- Test node addition/removal
- Test cache behavior

### 14.3 Load Tests
- Measure throughput under various loads
- Test concurrent request handling
- Test with multiple storage nodes

## 15. Deployment Checklist

- [ ] Build Docker image
- [ ] Create Kubernetes deployment manifests
- [ ] Configure resource limits (CPU, memory)
- [ ] Set up horizontal pod autoscaler
- [ ] Configure health check endpoints
- [ ] Set up Prometheus monitoring
- [ ] Configure logging (structured JSON logs)
- [ ] Set up alerting rules
- [ ] Test graceful shutdown
- [ ] Document operational runbooks

## 16. Performance Targets

- **Latency**: p95 < 50ms for writes, p95 < 30ms for reads
- **Throughput**: 10,000+ QPS per coordinator instance
- **Availability**: 99.9% uptime
- **Cache Hit Rate**: > 90% for tenant configurations
- **Repair Latency**: < 100ms to trigger repair (async execution)
