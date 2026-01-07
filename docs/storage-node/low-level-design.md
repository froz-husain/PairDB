# Storage Node Service: Low-Level Design (LLD)

## 1. Overview

This document provides the low-level implementation details for the Storage Node service. It complements the high-level design document with detailed class structures, algorithms, data flows, and implementation considerations.

## 2. Technology Stack

### 2.1 Language and Framework
- **Language**: Go 1.21+
- **gRPC Framework**: google.golang.org/grpc
- **Storage Engine**: Badger (for SSTables) or custom implementation
- **Logging**: zap (structured logging)
- **Metrics**: Prometheus client
- **Configuration**: viper
- **Testing**: testify, gomock

### 2.2 Dependencies
```go
require (
    google.golang.org/grpc v1.58.0
    google.golang.org/protobuf v1.31.0
    github.com/dgraph-io/badger/v4 v4.2.0
    go.uber.org/zap v1.26.0
    github.com/prometheus/client_golang v1.17.0
    github.com/spf13/viper v1.17.0
    github.com/stretchr/testify v1.8.4
    github.com/golang/mock v1.6.0
    github.com/google/uuid v1.4.0
    golang.org/x/sync v0.4.0
    github.com/hashicorp/memberlist v0.5.0  // For gossip protocol
)
```

## 3. Package Structure

```
storage-node/
├── cmd/
│   └── storage/
│       └── main.go                    # Entry point
├── internal/
│   ├── config/
│   │   └── config.go                 # Configuration management
│   ├── server/
│   │   └── grpc_server.go            # gRPC server setup
│   ├── handler/
│   │   ├── storage_handler.go        # Storage operation handlers
│   │   ├── migration_handler.go      # Migration operation handlers
│   │   └── health_handler.go         # Health check handlers
│   ├── service/
│   │   ├── storage_service.go        # Main storage logic
│   │   ├── commitlog_service.go      # Commit log management
│   │   ├── memtable_service.go       # MemTable management
│   │   ├── sstable_service.go        # SSTable management
│   │   ├── cache_service.go          # Cache management
│   │   ├── compaction_service.go     # Compaction logic
│   │   ├── vectorclock_service.go    # Vector clock operations
│   │   ├── gossip_service.go         # Gossip protocol for health
│   │   └── migration_service.go      # Migration operations
│   ├── storage/
│   │   ├── commitlog/
│   │   │   ├── writer.go             # Commit log writer
│   │   │   ├── reader.go             # Commit log reader
│   │   │   └── rotation.go           # Log rotation logic
│   │   ├── memtable/
│   │   │   ├── memtable.go           # In-memory table
│   │   │   └── skiplist.go           # Skip list implementation
│   │   ├── sstable/
│   │   │   ├── writer.go             # SSTable writer
│   │   │   ├── reader.go             # SSTable reader
│   │   │   ├── index.go              # SSTable indexing
│   │   │   └── bloom_filter.go       # Bloom filter
│   │   └── cache/
│   │       ├── adaptive_cache.go     # Adaptive LRU/LFU cache
│   │       └── eviction.go           # Eviction policies
│   ├── model/
│   │   ├── entry.go                  # Key-value entry models
│   │   ├── vectorclock.go            # Vector clock models
│   │   ├── sstable.go                # SSTable models
│   │   └── health.go                 # Health status models
│   ├── algorithm/
│   │   ├── vectorclock_ops.go        # Vector clock operations
│   │   ├── compaction.go             # Compaction algorithms
│   │   └── adaptive_eviction.go      # Adaptive cache eviction
│   ├── metrics/
│   │   └── prometheus.go             # Prometheus metrics
│   └── health/
│       └── health_check.go           # Health check logic
├── pkg/
│   └── proto/
│       └── storage.proto             # gRPC definitions
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
    Server       ServerConfig
    Storage      StorageConfig
    CommitLog    CommitLogConfig
    MemTable     MemTableConfig
    SSTable      SSTableConfig
    Cache        CacheConfig
    Compaction   CompactionConfig
    Gossip       GossipConfig
    Metrics      MetricsConfig
    Logging      LoggingConfig
}

type ServerConfig struct {
    NodeID          string
    Host            string
    Port            int
    MaxConnections  int
    ReadTimeout     time.Duration
    WriteTimeout    time.Duration
    ShutdownTimeout time.Duration
}

type StorageConfig struct {
    DataDir         string        // Base directory for all data
    CommitLogDir    string        // Directory for commit logs
    SSTableDir      string        // Directory for SSTables
    MaxDiskUsage    float64       // Max disk usage percentage (0.9 = 90%)
}

type CommitLogConfig struct {
    SegmentSize     int64         // 1GB default
    MaxAge          time.Duration // 7 days default
    SyncWrites      bool          // Sync to disk on every write
    BufferSize      int           // Write buffer size
}

type MemTableConfig struct {
    MaxSize         int64         // 64MB default
    FlushThreshold  int64         // When to trigger flush
    NumMemTables    int           // Number of concurrent memtables
}

type SSTableConfig struct {
    L0Size          int64         // 64MB
    L1Size          int64         // 128MB
    L2Size          int64         // 256MB
    LevelMultiplier int           // 2x size increase per level
    BloomFilterFP   float64       // False positive rate (0.01 = 1%)
    BlockSize       int           // 4KB default
    IndexInterval   int           // Key interval for index
}

type CacheConfig struct {
    MaxSize         int64         // Max cache size in bytes
    FrequencyWeight float64       // Weight for LFU component (0.5)
    RecencyWeight   float64       // Weight for LRU component (0.5)
    AdaptiveWindow  time.Duration // Window for adaptive adjustments
}

type CompactionConfig struct {
    L0Trigger       int           // Number of L0 SSTables to trigger compaction
    Workers         int           // Number of compaction workers
    Throttle        int           // Throttle rate (MB/s)
}

type GossipConfig struct {
    Enabled         bool
    BindPort        int
    SeedNodes       []string      // Initial seed nodes
    GossipInterval  time.Duration // 5 seconds
    ProbeTimeout    time.Duration // 500ms
    ProbeInterval   time.Duration // 1 second
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
// model/entry.go
package model

import "time"

type VectorClockEntry struct {
    CoordinatorNodeID string
    LogicalTimestamp  int64
}

type VectorClock struct {
    Entries []VectorClockEntry
}

type KeyValueEntry struct {
    TenantID    string
    Key         string
    Value       []byte
    VectorClock VectorClock
    Timestamp   int64
}

type CommitLogEntry struct {
    TenantID      string
    Key           string
    Value         []byte
    VectorClock   VectorClock
    Timestamp     int64
    OperationType OperationType
}

type OperationType string

const (
    OperationTypeWrite  OperationType = "write"
    OperationTypeRepair OperationType = "repair"
    OperationTypeDelete OperationType = "delete"
)

type MemTableEntry struct {
    Key         string  // Format: "{tenant_id}:{key}"
    Value       []byte
    VectorClock VectorClock
    Timestamp   int64
}

type CacheEntry struct {
    Key         string  // Format: "{tenant_id}:{key}"
    Value       []byte
    VectorClock VectorClock
    AccessCount int64      // For LFU
    LastAccess  time.Time  // For LRU
    Score       float64    // Adaptive score
}

// model/sstable.go
package model

import "time"

type SSTableMetadata struct {
    SSTableID   string
    TenantID    string
    Level       int
    Size        int64
    KeyRange    KeyRange
    CreatedAt   time.Time
    FilePath    string
    IndexPath   string
    BloomPath   string
}

type KeyRange struct {
    StartKey string
    EndKey   string
}

type SSTableLevel int

const (
    L0 SSTableLevel = 0
    L1 SSTableLevel = 1
    L2 SSTableLevel = 2
    L3 SSTableLevel = 3
    L4 SSTableLevel = 4
)

type CompactionJob struct {
    JobID       string
    Level       SSTableLevel
    InputTables []*SSTableMetadata
    OutputLevel SSTableLevel
    StartedAt   time.Time
    Status      CompactionStatus
}

type CompactionStatus string

const (
    CompactionStatusPending    CompactionStatus = "pending"
    CompactionStatusRunning    CompactionStatus = "running"
    CompactionStatusCompleted  CompactionStatus = "completed"
    CompactionStatusFailed     CompactionStatus = "failed"
)

// model/health.go
package model

import "time"

type HealthStatus struct {
    NodeID      string
    Status      NodeStatus
    Timestamp   int64
    Metrics     HealthMetrics
}

type NodeStatus string

const (
    NodeStatusHealthy   NodeStatus = "healthy"
    NodeStatusDegraded  NodeStatus = "degraded"
    NodeStatusUnhealthy NodeStatus = "unhealthy"
)

type HealthMetrics struct {
    CPUUsage       float64
    MemoryUsage    float64
    DiskUsage      float64
    RequestRate    float64
    ErrorRate      float64
    CacheHitRate   float64
}
```

## 5. Service Layer Implementation

### 5.1 Storage Service (Main)

```go
// service/storage_service.go
package service

import (
    "context"
    "fmt"
    "time"

    "go.uber.org/zap"
)

type StorageService struct {
    commitLogService  *CommitLogService
    memTableService   *MemTableService
    sstableService    *SSTableService
    cacheService      *CacheService
    vectorClockService *VectorClockService
    logger            *zap.Logger
    nodeID            string
}

func NewStorageService(
    commitLogSvc *CommitLogService,
    memTableSvc *MemTableService,
    sstableSvc *SSTableService,
    cacheSvc *CacheService,
    vectorClockSvc *VectorClockService,
    logger *zap.Logger,
    nodeID string,
) *StorageService {
    return &StorageService{
        commitLogService:   commitLogSvc,
        memTableService:    memTableSvc,
        sstableService:     sstableSvc,
        cacheService:       cacheSvc,
        vectorClockService: vectorClockSvc,
        logger:             logger,
        nodeID:             nodeID,
    }
}

// Write handles write operations
func (s *StorageService) Write(
    ctx context.Context,
    tenantID string,
    key string,
    value []byte,
    vectorClock model.VectorClock,
) (*WriteResponse, error) {
    startTime := time.Now()

    // 1. Validate inputs
    if err := s.validateWrite(tenantID, key, value); err != nil {
        return nil, fmt.Errorf("validation failed: %w", err)
    }

    // 2. Create commit log entry
    entry := &model.CommitLogEntry{
        TenantID:      tenantID,
        Key:           key,
        Value:         value,
        VectorClock:   vectorClock,
        Timestamp:     time.Now().Unix(),
        OperationType: model.OperationTypeWrite,
    }

    // 3. Write to commit log (durability)
    if err := s.commitLogService.Append(ctx, entry); err != nil {
        s.logger.Error("Failed to write to commit log",
            zap.String("tenant_id", tenantID),
            zap.String("key", key),
            zap.Error(err))
        return nil, fmt.Errorf("commit log write failed: %w", err)
    }

    // 4. Write to memtable
    memTableKey := s.buildKey(tenantID, key)
    memEntry := &model.MemTableEntry{
        Key:         memTableKey,
        Value:       value,
        VectorClock: vectorClock,
        Timestamp:   entry.Timestamp,
    }

    if err := s.memTableService.Put(ctx, memEntry); err != nil {
        s.logger.Error("Failed to write to memtable",
            zap.String("key", memTableKey),
            zap.Error(err))
        return nil, fmt.Errorf("memtable write failed: %w", err)
    }

    // 5. Update cache
    s.cacheService.Put(memTableKey, value, vectorClock)

    // 6. Check if memtable needs flushing
    if s.memTableService.ShouldFlush() {
        go s.triggerFlush()
    }

    latency := time.Since(startTime)
    s.logger.Debug("Write completed",
        zap.String("tenant_id", tenantID),
        zap.String("key", key),
        zap.Duration("latency", latency))

    return &WriteResponse{
        Success:     true,
        VectorClock: vectorClock,
    }, nil
}

// Read handles read operations
func (s *StorageService) Read(
    ctx context.Context,
    tenantID string,
    key string,
) (*ReadResponse, error) {
    startTime := time.Now()
    memTableKey := s.buildKey(tenantID, key)

    // 1. Check cache first
    if entry, found := s.cacheService.Get(memTableKey); found {
        s.logger.Debug("Cache hit",
            zap.String("tenant_id", tenantID),
            zap.String("key", key))

        return &ReadResponse{
            Success:     true,
            Value:       entry.Value,
            VectorClock: entry.VectorClock,
            Source:      "cache",
        }, nil
    }

    // 2. Check memtable
    if entry, found := s.memTableService.Get(ctx, memTableKey); found {
        s.logger.Debug("MemTable hit",
            zap.String("tenant_id", tenantID),
            zap.String("key", key))

        // Update cache
        s.cacheService.Put(memTableKey, entry.Value, entry.VectorClock)

        return &ReadResponse{
            Success:     true,
            Value:       entry.Value,
            VectorClock: entry.VectorClock,
            Source:      "memtable",
        }, nil
    }

    // 3. Search SSTables
    entry, err := s.sstableService.Get(ctx, tenantID, key)
    if err != nil {
        s.logger.Error("SSTable read failed",
            zap.String("tenant_id", tenantID),
            zap.String("key", key),
            zap.Error(err))
        return nil, fmt.Errorf("sstable read failed: %w", err)
    }

    if entry == nil {
        return nil, fmt.Errorf("key not found")
    }

    // Update cache
    s.cacheService.Put(memTableKey, entry.Value, entry.VectorClock)

    latency := time.Since(startTime)
    s.logger.Debug("Read completed",
        zap.String("tenant_id", tenantID),
        zap.String("key", key),
        zap.String("source", "sstable"),
        zap.Duration("latency", latency))

    return &ReadResponse{
        Success:     true,
        Value:       entry.Value,
        VectorClock: entry.VectorClock,
        Source:      "sstable",
    }, nil
}

// Repair handles repair operations
func (s *StorageService) Repair(
    ctx context.Context,
    tenantID string,
    key string,
    value []byte,
    vectorClock model.VectorClock,
) error {
    // Create repair entry
    entry := &model.CommitLogEntry{
        TenantID:      tenantID,
        Key:           key,
        Value:         value,
        VectorClock:   vectorClock,
        Timestamp:     time.Now().Unix(),
        OperationType: model.OperationTypeRepair,
    }

    // Write to commit log
    if err := s.commitLogService.Append(ctx, entry); err != nil {
        return fmt.Errorf("repair commit log failed: %w", err)
    }

    // Update memtable
    memTableKey := s.buildKey(tenantID, key)
    memEntry := &model.MemTableEntry{
        Key:         memTableKey,
        Value:       value,
        VectorClock: vectorClock,
        Timestamp:   entry.Timestamp,
    }

    if err := s.memTableService.Put(ctx, memEntry); err != nil {
        return fmt.Errorf("repair memtable failed: %w", err)
    }

    // Update cache
    s.cacheService.Put(memTableKey, value, vectorClock)

    s.logger.Info("Repair completed",
        zap.String("tenant_id", tenantID),
        zap.String("key", key))

    return nil
}

// triggerFlush triggers memtable flush to SSTable
func (s *StorageService) triggerFlush() {
    ctx := context.Background()

    s.logger.Info("Triggering memtable flush")

    if err := s.memTableService.Flush(ctx, s.sstableService); err != nil {
        s.logger.Error("Memtable flush failed", zap.Error(err))
    }
}

// validateWrite validates write parameters
func (s *StorageService) validateWrite(tenantID, key string, value []byte) error {
    if tenantID == "" {
        return fmt.Errorf("tenant_id is required")
    }
    if key == "" {
        return fmt.Errorf("key is required")
    }
    if value == nil {
        return fmt.Errorf("value is required")
    }
    return nil
}

// buildKey creates composite key
func (s *StorageService) buildKey(tenantID, key string) string {
    return fmt.Sprintf("%s:%s", tenantID, key)
}

type WriteResponse struct {
    Success     bool
    VectorClock model.VectorClock
}

type ReadResponse struct {
    Success     bool
    Value       []byte
    VectorClock model.VectorClock
    Source      string
}
```

### 5.2 Commit Log Service

```go
// service/commitlog_service.go
package service

import (
    "context"
    "encoding/json"
    "fmt"
    "os"
    "path/filepath"
    "sync"
    "time"

    "go.uber.org/zap"
)

type CommitLogService struct {
    config      *config.CommitLogConfig
    currentFile *os.File
    writer      *CommitLogWriter
    logger      *zap.Logger
    mu          sync.Mutex
    dataDir     string
    segmentID   int64
}

func NewCommitLogService(
    cfg *config.CommitLogConfig,
    dataDir string,
    logger *zap.Logger,
) (*CommitLogService, error) {
    cls := &CommitLogService{
        config:    cfg,
        logger:    logger,
        dataDir:   dataDir,
        segmentID: time.Now().Unix(),
    }

    if err := cls.openNewSegment(); err != nil {
        return nil, fmt.Errorf("failed to open commit log segment: %w", err)
    }

    // Start rotation checker
    go cls.rotationChecker()

    return cls, nil
}

// Append appends an entry to the commit log
func (s *CommitLogService) Append(ctx context.Context, entry *model.CommitLogEntry) error {
    s.mu.Lock()
    defer s.mu.Unlock()

    // Serialize entry
    data, err := json.Marshal(entry)
    if err != nil {
        return fmt.Errorf("failed to marshal entry: %w", err)
    }

    // Append to file
    if _, err := s.currentFile.Write(append(data, '\n')); err != nil {
        return fmt.Errorf("failed to write to commit log: %w", err)
    }

    // Sync if configured
    if s.config.SyncWrites {
        if err := s.currentFile.Sync(); err != nil {
            return fmt.Errorf("failed to sync commit log: %w", err)
        }
    }

    return nil
}

// openNewSegment creates a new commit log segment
func (s *CommitLogService) openNewSegment() error {
    // Close current file if exists
    if s.currentFile != nil {
        s.currentFile.Close()
    }

    // Create new segment file
    segmentPath := filepath.Join(s.dataDir, fmt.Sprintf("commitlog-%d.log", s.segmentID))
    file, err := os.OpenFile(segmentPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
    if err != nil {
        return fmt.Errorf("failed to open commit log file: %w", err)
    }

    s.currentFile = file
    s.segmentID = time.Now().Unix()

    s.logger.Info("Opened new commit log segment", zap.String("path", segmentPath))

    return nil
}

// rotationChecker periodically checks if rotation is needed
func (s *CommitLogService) rotationChecker() {
    ticker := time.NewTicker(1 * time.Minute)
    defer ticker.Stop()

    for range ticker.C {
        s.checkRotation()
    }
}

// checkRotation checks if commit log needs rotation
func (s *CommitLogService) checkRotation() {
    s.mu.Lock()
    defer s.mu.Unlock()

    if s.currentFile == nil {
        return
    }

    // Check file size
    fileInfo, err := s.currentFile.Stat()
    if err != nil {
        s.logger.Error("Failed to stat commit log", zap.Error(err))
        return
    }

    if fileInfo.Size() >= s.config.SegmentSize {
        s.logger.Info("Rotating commit log due to size",
            zap.Int64("size", fileInfo.Size()),
            zap.Int64("threshold", s.config.SegmentSize))

        if err := s.openNewSegment(); err != nil {
            s.logger.Error("Failed to rotate commit log", zap.Error(err))
        }
    }
}

// Recover replays commit log entries on startup
func (s *CommitLogService) Recover(ctx context.Context, memTableSvc *MemTableService) error {
    s.logger.Info("Starting commit log recovery")

    // Find all commit log files
    files, err := filepath.Glob(filepath.Join(s.dataDir, "commitlog-*.log"))
    if err != nil {
        return fmt.Errorf("failed to list commit log files: %w", err)
    }

    recovered := 0
    for _, filePath := range files {
        count, err := s.recoverFromFile(ctx, filePath, memTableSvc)
        if err != nil {
            s.logger.Error("Failed to recover from file",
                zap.String("file", filePath),
                zap.Error(err))
            continue
        }
        recovered += count
    }

    s.logger.Info("Commit log recovery completed", zap.Int("entries", recovered))
    return nil
}

// recoverFromFile recovers entries from a single commit log file
func (s *CommitLogService) recoverFromFile(
    ctx context.Context,
    filePath string,
    memTableSvc *MemTableService,
) (int, error) {
    file, err := os.Open(filePath)
    if err != nil {
        return 0, err
    }
    defer file.Close()

    // Read and replay entries
    // Implementation details omitted for brevity

    return 0, nil
}

func (s *CommitLogService) Close() error {
    s.mu.Lock()
    defer s.mu.Unlock()

    if s.currentFile != nil {
        return s.currentFile.Close()
    }
    return nil
}
```

### 5.3 MemTable Service

```go
// service/memtable_service.go
package service

import (
    "context"
    "fmt"
    "sync"

    "go.uber.org/zap"
)

type MemTableService struct {
    config       *config.MemTableConfig
    memTable     *MemTable
    immutableMT  *MemTable  // Immutable memtable being flushed
    logger       *zap.Logger
    mu           sync.RWMutex
    flushMu      sync.Mutex
}

func NewMemTableService(
    cfg *config.MemTableConfig,
    logger *zap.Logger,
) *MemTableService {
    return &MemTableService{
        config:   cfg,
        memTable: NewMemTable(cfg.MaxSize),
        logger:   logger,
    }
}

// Put inserts or updates an entry in memtable
func (s *MemTableService) Put(ctx context.Context, entry *model.MemTableEntry) error {
    s.mu.Lock()
    defer s.mu.Unlock()

    return s.memTable.Put(entry)
}

// Get retrieves an entry from memtable
func (s *MemTableService) Get(ctx context.Context, key string) (*model.MemTableEntry, bool) {
    s.mu.RLock()
    defer s.mu.RUnlock()

    // Check current memtable
    if entry, found := s.memTable.Get(key); found {
        return entry, true
    }

    // Check immutable memtable if exists
    if s.immutableMT != nil {
        if entry, found := s.immutableMT.Get(key); found {
            return entry, true
        }
    }

    return nil, false
}

// ShouldFlush checks if memtable should be flushed
func (s *MemTableService) ShouldFlush() bool {
    s.mu.RLock()
    defer s.mu.RUnlock()

    return s.memTable.Size() >= s.config.FlushThreshold
}

// Flush flushes memtable to SSTable
func (s *MemTableService) Flush(ctx context.Context, sstableSvc *SSTableService) error {
    s.flushMu.Lock()
    defer s.flushMu.Unlock()

    // Make current memtable immutable
    s.mu.Lock()
    if s.memTable.Size() == 0 {
        s.mu.Unlock()
        return nil // Nothing to flush
    }

    s.immutableMT = s.memTable
    s.memTable = NewMemTable(s.config.MaxSize)
    immutable := s.immutableMT
    s.mu.Unlock()

    s.logger.Info("Starting memtable flush",
        zap.Int64("size", immutable.Size()),
        zap.Int("entries", immutable.Count()))

    // Write to SSTable
    if err := sstableSvc.WriteFromMemTable(ctx, immutable); err != nil {
        s.logger.Error("Failed to flush memtable", zap.Error(err))
        return err
    }

    // Clear immutable memtable
    s.mu.Lock()
    s.immutableMT = nil
    s.mu.Unlock()

    s.logger.Info("Memtable flush completed")
    return nil
}

// MemTable is an in-memory sorted table
type MemTable struct {
    data    *SkipList
    maxSize int64
    size    int64
    mu      sync.RWMutex
}

func NewMemTable(maxSize int64) *MemTable {
    return &MemTable{
        data:    NewSkipList(),
        maxSize: maxSize,
    }
}

func (mt *MemTable) Put(entry *model.MemTableEntry) error {
    mt.mu.Lock()
    defer mt.mu.Unlock()

    // Estimate size
    entrySize := int64(len(entry.Key) + len(entry.Value) + 64) // Approximate

    mt.data.Insert(entry.Key, entry)
    mt.size += entrySize

    return nil
}

func (mt *MemTable) Get(key string) (*model.MemTableEntry, bool) {
    mt.mu.RLock()
    defer mt.mu.RUnlock()

    value, found := mt.data.Search(key)
    if !found {
        return nil, false
    }

    return value.(*model.MemTableEntry), true
}

func (mt *MemTable) Size() int64 {
    mt.mu.RLock()
    defer mt.mu.RUnlock()
    return mt.size
}

func (mt *MemTable) Count() int {
    mt.mu.RLock()
    defer mt.mu.RUnlock()
    return mt.data.Len()
}

func (mt *MemTable) Iterator() *MemTableIterator {
    mt.mu.RLock()
    defer mt.mu.RUnlock()
    return &MemTableIterator{
        skipList: mt.data,
        current:  mt.data.head,
    }
}

// MemTableIterator iterates over memtable entries
type MemTableIterator struct {
    skipList *SkipList
    current  *SkipListNode
}

func (it *MemTableIterator) Next() bool {
    if it.current == nil {
        return false
    }
    it.current = it.current.forward[0]
    return it.current != nil
}

func (it *MemTableIterator) Entry() *model.MemTableEntry {
    if it.current == nil {
        return nil
    }
    return it.current.value.(*model.MemTableEntry)
}
```

### 5.4 SSTable Service

```go
// service/sstable_service.go
package service

import (
    "context"
    "fmt"
    "path/filepath"
    "sync"
    "time"

    "go.uber.org/zap"
)

type SSTableService struct {
    config      *config.SSTableConfig
    dataDir     string
    logger      *zap.Logger
    levels      map[model.SSTableLevel][]*model.SSTableMetadata
    mu          sync.RWMutex
}

func NewSSTableService(
    cfg *config.SSTableConfig,
    dataDir string,
    logger *zap.Logger,
) *SSTableService {
    return &SSTableService{
        config:  cfg,
        dataDir: dataDir,
        logger:  logger,
        levels:  make(map[model.SSTableLevel][]*model.SSTableMetadata),
    }
}

// WriteFromMemTable writes memtable contents to new L0 SSTable
func (s *SSTableService) WriteFromMemTable(
    ctx context.Context,
    memTable *MemTable,
) error {
    s.mu.Lock()
    defer s.mu.Unlock()

    // Generate SSTable ID
    sstableID := fmt.Sprintf("sstable-%d", time.Now().UnixNano())
    filePath := filepath.Join(s.dataDir, "l0", sstableID+".sst")

    // Create SSTable writer
    writer, err := NewSSTableWriter(filePath, s.config)
    if err != nil {
        return fmt.Errorf("failed to create sstable writer: %w", err)
    }
    defer writer.Close()

    // Write entries from memtable
    iterator := memTable.Iterator()
    var keyRange model.KeyRange
    entryCount := 0

    for iterator.Next() {
        entry := iterator.Entry()

        if entryCount == 0 {
            keyRange.StartKey = entry.Key
        }
        keyRange.EndKey = entry.Key

        if err := writer.Write(entry); err != nil {
            return fmt.Errorf("failed to write entry: %w", err)
        }
        entryCount++
    }

    // Finalize SSTable
    if err := writer.Finalize(); err != nil {
        return fmt.Errorf("failed to finalize sstable: %w", err)
    }

    // Create metadata
    metadata := &model.SSTableMetadata{
        SSTableID: sstableID,
        Level:     int(model.L0),
        Size:      writer.Size(),
        KeyRange:  keyRange,
        CreatedAt: time.Now(),
        FilePath:  filePath,
        IndexPath: filePath + ".idx",
        BloomPath: filePath + ".bloom",
    }

    // Add to level 0
    s.levels[model.L0] = append(s.levels[model.L0], metadata)

    s.logger.Info("Created new SSTable",
        zap.String("sstable_id", sstableID),
        zap.Int("level", 0),
        zap.Int("entries", entryCount),
        zap.Int64("size", metadata.Size))

    return nil
}

// Get retrieves a value from SSTables
func (s *SSTableService) Get(
    ctx context.Context,
    tenantID string,
    key string,
) (*model.KeyValueEntry, error) {
    s.mu.RLock()
    defer s.mu.RUnlock()

    compositeKey := fmt.Sprintf("%s:%s", tenantID, key)
    var latestEntry *model.KeyValueEntry
    var latestTimestamp int64

    // Search from L0 to higher levels
    for level := model.L0; level <= model.L4; level++ {
        tables := s.levels[level]

        for _, table := range tables {
            // Check if key is in range
            if !s.keyInRange(compositeKey, table.KeyRange) {
                continue
            }

            // Check bloom filter
            bloomFilter, err := LoadBloomFilter(table.BloomPath)
            if err != nil {
                s.logger.Warn("Failed to load bloom filter", zap.Error(err))
                continue
            }

            if !bloomFilter.MayContain(compositeKey) {
                continue // Definitely not in this SSTable
            }

            // Search in SSTable
            reader, err := NewSSTableReader(table.FilePath, table.IndexPath)
            if err != nil {
                s.logger.Error("Failed to open sstable", zap.Error(err))
                continue
            }

            entry, err := reader.Get(compositeKey)
            reader.Close()

            if err != nil {
                continue
            }

            if entry != nil && entry.Timestamp > latestTimestamp {
                latestEntry = entry
                latestTimestamp = entry.Timestamp
            }
        }
    }

    return latestEntry, nil
}

// keyInRange checks if key falls within range
func (s *SSTableService) keyInRange(key string, keyRange model.KeyRange) bool {
    return key >= keyRange.StartKey && key <= keyRange.EndKey
}

// GetTablesForLevel returns SSTables for a specific level
func (s *SSTableService) GetTablesForLevel(level model.SSTableLevel) []*model.SSTableMetadata {
    s.mu.RLock()
    defer s.mu.RUnlock()

    return s.levels[level]
}

// AddTable adds an SSTable to a level
func (s *SSTableService) AddTable(level model.SSTableLevel, table *model.SSTableMetadata) {
    s.mu.Lock()
    defer s.mu.Unlock()

    s.levels[level] = append(s.levels[level], table)
}

// RemoveTables removes SSTables from a level
func (s *SSTableService) RemoveTables(level model.SSTableLevel, tableIDs []string) {
    s.mu.Lock()
    defer s.mu.Unlock()

    // Create set of IDs to remove
    removeSet := make(map[string]bool)
    for _, id := range tableIDs {
        removeSet[id] = true
    }

    // Filter tables
    filtered := make([]*model.SSTableMetadata, 0)
    for _, table := range s.levels[level] {
        if !removeSet[table.SSTableID] {
            filtered = append(filtered, table)
        }
    }

    s.levels[level] = filtered
}
```

### 5.5 Cache Service (Adaptive LRU/LFU)

```go
// service/cache_service.go
package service

import (
    "sync"
    "time"

    "go.uber.org/zap"
)

type CacheService struct {
    config          *config.CacheConfig
    cache           map[string]*model.CacheEntry
    evictionList    *EvictionList
    logger          *zap.Logger
    mu              sync.RWMutex
    currentSize     int64
    frequencyWeight float64
    recencyWeight   float64
}

func NewCacheService(
    cfg *config.CacheConfig,
    logger *zap.Logger,
) *CacheService {
    return &CacheService{
        config:          cfg,
        cache:           make(map[string]*model.CacheEntry),
        evictionList:    NewEvictionList(),
        logger:          logger,
        frequencyWeight: cfg.FrequencyWeight,
        recencyWeight:   cfg.RecencyWeight,
    }
}

// Get retrieves a value from cache
func (s *CacheService) Get(key string) (*model.CacheEntry, bool) {
    s.mu.Lock()
    defer s.mu.Unlock()

    entry, found := s.cache[key]
    if !found {
        return nil, false
    }

    // Update access statistics
    entry.AccessCount++
    entry.LastAccess = time.Now()
    entry.Score = s.calculateScore(entry)

    return entry, true
}

// Put adds or updates a value in cache
func (s *CacheService) Put(key string, value []byte, vectorClock model.VectorClock) {
    s.mu.Lock()
    defer s.mu.Unlock()

    entrySize := int64(len(key) + len(value) + 64)

    // Check if key already exists
    if existing, found := s.cache[key]; found {
        // Update existing entry
        existing.Value = value
        existing.VectorClock = vectorClock
        existing.AccessCount++
        existing.LastAccess = time.Now()
        existing.Score = s.calculateScore(existing)
        return
    }

    // Check if cache is full
    for s.currentSize+entrySize > s.config.MaxSize {
        s.evictLowestScore()
    }

    // Add new entry
    entry := &model.CacheEntry{
        Key:         key,
        Value:       value,
        VectorClock: vectorClock,
        AccessCount: 1,
        LastAccess:  time.Now(),
    }
    entry.Score = s.calculateScore(entry)

    s.cache[key] = entry
    s.currentSize += entrySize
}

// calculateScore computes adaptive score for eviction
func (s *CacheService) calculateScore(entry *model.CacheEntry) float64 {
    // Frequency component (normalized)
    frequencyScore := float64(entry.AccessCount)

    // Recency component (time since last access in seconds)
    recencyScore := time.Since(entry.LastAccess).Seconds()

    // Combined adaptive score
    score := s.frequencyWeight*frequencyScore - s.recencyWeight*recencyScore

    return score
}

// evictLowestScore evicts the entry with lowest score
func (s *CacheService) evictLowestScore() {
    var lowestKey string
    var lowestScore float64 = 1e9

    for key, entry := range s.cache {
        if entry.Score < lowestScore {
            lowestScore = entry.Score
            lowestKey = key
        }
    }

    if lowestKey != "" {
        entry := s.cache[lowestKey]
        entrySize := int64(len(entry.Key) + len(entry.Value) + 64)

        delete(s.cache, lowestKey)
        s.currentSize -= entrySize

        s.logger.Debug("Evicted cache entry",
            zap.String("key", lowestKey),
            zap.Float64("score", lowestScore))
    }
}

// AdjustWeights adjusts frequency and recency weights based on workload
func (s *CacheService) AdjustWeights() {
    s.mu.Lock()
    defer s.mu.Unlock()

    // Analyze access patterns
    var totalAccesses int64
    var recentAccesses int64
    recentThreshold := time.Now().Add(-s.config.AdaptiveWindow)

    for _, entry := range s.cache {
        totalAccesses += entry.AccessCount
        if entry.LastAccess.After(recentThreshold) {
            recentAccesses++
        }
    }

    if totalAccesses == 0 {
        return
    }

    // Calculate hotness ratio
    hotnessRatio := float64(recentAccesses) / float64(len(s.cache))

    // Adjust weights based on hotness
    if hotnessRatio > 0.7 {
        // High recency workload - favor LRU
        s.recencyWeight = 0.7
        s.frequencyWeight = 0.3
    } else if hotnessRatio < 0.3 {
        // High frequency workload - favor LFU
        s.recencyWeight = 0.3
        s.frequencyWeight = 0.7
    } else {
        // Balanced workload
        s.recencyWeight = 0.5
        s.frequencyWeight = 0.5
    }

    s.logger.Debug("Adjusted cache weights",
        zap.Float64("recency_weight", s.recencyWeight),
        zap.Float64("frequency_weight", s.frequencyWeight),
        zap.Float64("hotness_ratio", hotnessRatio))
}

// Stats returns cache statistics
func (s *CacheService) Stats() CacheStats {
    s.mu.RLock()
    defer s.mu.RUnlock()

    return CacheStats{
        Size:        s.currentSize,
        MaxSize:     s.config.MaxSize,
        EntryCount:  len(s.cache),
        UsagePercent: float64(s.currentSize) / float64(s.config.MaxSize) * 100,
    }
}

type CacheStats struct {
    Size         int64
    MaxSize      int64
    EntryCount   int
    UsagePercent float64
}

type EvictionList struct {
    // Simple placeholder for eviction list
}

func NewEvictionList() *EvictionList {
    return &EvictionList{}
}
```

### 5.6 Compaction Service

```go
// service/compaction_service.go
package service

import (
    "context"
    "fmt"
    "sync"
    "time"

    "go.uber.org/zap"
)

type CompactionService struct {
    config         *config.CompactionConfig
    sstableService *SSTableService
    logger         *zap.Logger
    jobQueue       chan *model.CompactionJob
    stopChan       chan struct{}
    wg             sync.WaitGroup
}

func NewCompactionService(
    cfg *config.CompactionConfig,
    sstableSvc *SSTableService,
    logger *zap.Logger,
) *CompactionService {
    cs := &CompactionService{
        config:         cfg,
        sstableService: sstableSvc,
        logger:         logger,
        jobQueue:       make(chan *model.CompactionJob, 100),
        stopChan:       make(chan struct{}),
    }

    // Start compaction workers
    for i := 0; i < cfg.Workers; i++ {
        cs.wg.Add(1)
        go cs.compactionWorker(i)
    }

    // Start compaction scheduler
    go cs.compactionScheduler()

    return cs
}

// compactionScheduler periodically checks for compaction opportunities
func (s *CompactionService) compactionScheduler() {
    ticker := time.NewTicker(30 * time.Second)
    defer ticker.Stop()

    for {
        select {
        case <-ticker.C:
            s.checkCompactionNeeded()
        case <-s.stopChan:
            return
        }
    }
}

// checkCompactionNeeded checks if compaction is needed
func (s *CompactionService) checkCompactionNeeded() {
    // Check L0 compaction
    l0Tables := s.sstableService.GetTablesForLevel(model.L0)
    if len(l0Tables) >= s.config.L0Trigger {
        s.logger.Info("L0 compaction triggered",
            zap.Int("table_count", len(l0Tables)))

        job := &model.CompactionJob{
            JobID:       fmt.Sprintf("compact-l0-%d", time.Now().Unix()),
            Level:       model.L0,
            InputTables: l0Tables,
            OutputLevel: model.L1,
            StartedAt:   time.Now(),
            Status:      model.CompactionStatusPending,
        }

        select {
        case s.jobQueue <- job:
        default:
            s.logger.Warn("Compaction queue full")
        }
    }

    // Check other levels
    for level := model.L1; level < model.L4; level++ {
        if s.shouldCompactLevel(level) {
            s.triggerLevelCompaction(level)
        }
    }
}

// shouldCompactLevel checks if a level needs compaction
func (s *CompactionService) shouldCompactLevel(level model.SSTableLevel) bool {
    tables := s.sstableService.GetTablesForLevel(level)

    var totalSize int64
    for _, table := range tables {
        totalSize += table.Size
    }

    // Get level size threshold
    threshold := s.getLevelThreshold(level)

    return totalSize > threshold
}

// getLevelThreshold returns size threshold for a level
func (s *CompactionService) getLevelThreshold(level model.SSTableLevel) int64 {
    switch level {
    case model.L0:
        return s.config.L0Size
    case model.L1:
        return s.config.L1Size
    case model.L2:
        return s.config.L2Size
    default:
        // Calculate based on multiplier
        baseSize := s.config.L2Size
        multiplier := int64(1)
        for i := model.L2; i < level; i++ {
            multiplier *= int64(s.config.LevelMultiplier)
        }
        return baseSize * multiplier
    }
}

// triggerLevelCompaction triggers compaction for a level
func (s *CompactionService) triggerLevelCompaction(level model.SSTableLevel) {
    tables := s.sstableService.GetTablesForLevel(level)
    if len(tables) == 0 {
        return
    }

    job := &model.CompactionJob{
        JobID:       fmt.Sprintf("compact-l%d-%d", level, time.Now().Unix()),
        Level:       level,
        InputTables: tables,
        OutputLevel: level + 1,
        StartedAt:   time.Now(),
        Status:      model.CompactionStatusPending,
    }

    select {
    case s.jobQueue <- job:
    default:
        s.logger.Warn("Compaction queue full")
    }
}

// compactionWorker processes compaction jobs
func (s *CompactionService) compactionWorker(workerID int) {
    defer s.wg.Done()

    s.logger.Info("Compaction worker started", zap.Int("worker_id", workerID))

    for {
        select {
        case job := <-s.jobQueue:
            s.executeCompaction(job)
        case <-s.stopChan:
            return
        }
    }
}

// executeCompaction performs the actual compaction
func (s *CompactionService) executeCompaction(job *model.CompactionJob) {
    ctx := context.Background()

    s.logger.Info("Starting compaction",
        zap.String("job_id", job.JobID),
        zap.Int("level", int(job.Level)),
        zap.Int("input_tables", len(job.InputTables)))

    job.Status = model.CompactionStatusRunning

    // Merge SSTables
    outputTable, err := s.mergeSSTables(ctx, job.InputTables, job.OutputLevel)
    if err != nil {
        s.logger.Error("Compaction failed",
            zap.String("job_id", job.JobID),
            zap.Error(err))
        job.Status = model.CompactionStatusFailed
        return
    }

    // Update SSTable service
    s.sstableService.AddTable(job.OutputLevel, outputTable)

    // Remove input tables
    tableIDs := make([]string, len(job.InputTables))
    for i, table := range job.InputTables {
        tableIDs[i] = table.SSTableID
    }
    s.sstableService.RemoveTables(job.Level, tableIDs)

    // Delete old SSTable files
    // TODO: Implement file deletion

    job.Status = model.CompactionStatusCompleted

    s.logger.Info("Compaction completed",
        zap.String("job_id", job.JobID),
        zap.String("output_table", outputTable.SSTableID),
        zap.Int64("output_size", outputTable.Size))
}

// mergeSSTables merges multiple SSTables into one
func (s *CompactionService) mergeSSTables(
    ctx context.Context,
    inputTables []*model.SSTableMetadata,
    outputLevel model.SSTableLevel,
) (*model.SSTableMetadata, error) {
    // Create output SSTable
    sstableID := fmt.Sprintf("sstable-l%d-%d", outputLevel, time.Now().UnixNano())
    // Implementation details omitted for brevity

    return &model.SSTableMetadata{
        SSTableID: sstableID,
        Level:     int(outputLevel),
    }, nil
}

func (s *CompactionService) Stop() {
    close(s.stopChan)
    s.wg.Wait()
}
```

### 5.7 Gossip Service (Health Monitoring)

```go
// service/gossip_service.go
package service

import (
    "encoding/json"
    "fmt"
    "time"

    "github.com/hashicorp/memberlist"
    "go.uber.org/zap"
)

type GossipService struct {
    config     *config.GossipConfig
    memberlist *memberlist.Memberlist
    nodeID     string
    logger     *zap.Logger
    healthData *model.HealthStatus
}

func NewGossipService(
    cfg *config.GossipConfig,
    nodeID string,
    logger *zap.Logger,
) (*GossipService, error) {
    gs := &GossipService{
        config: cfg,
        nodeID: nodeID,
        logger: logger,
        healthData: &model.HealthStatus{
            NodeID:    nodeID,
            Status:    model.NodeStatusHealthy,
            Timestamp: time.Now().Unix(),
        },
    }

    // Configure memberlist
    mlConfig := memberlist.DefaultLocalConfig()
    mlConfig.Name = nodeID
    mlConfig.BindPort = cfg.BindPort
    mlConfig.GossipInterval = cfg.GossipInterval
    mlConfig.ProbeTimeout = cfg.ProbeTimeout
    mlConfig.ProbeInterval = cfg.ProbeInterval
    mlConfig.Delegate = gs
    mlConfig.Events = &GossipEventDelegate{service: gs}

    // Create memberlist
    ml, err := memberlist.Create(mlConfig)
    if err != nil {
        return nil, fmt.Errorf("failed to create memberlist: %w", err)
    }

    gs.memberlist = ml

    // Join seed nodes
    if len(cfg.SeedNodes) > 0 {
        _, err := ml.Join(cfg.SeedNodes)
        if err != nil {
            logger.Warn("Failed to join some seed nodes", zap.Error(err))
        }
    }

    return gs, nil
}

// NodeMeta implements memberlist.Delegate
func (s *GossipService) NodeMeta(limit int) []byte {
    data, _ := json.Marshal(s.healthData)
    if len(data) > limit {
        return data[:limit]
    }
    return data
}

// NotifyMsg implements memberlist.Delegate
func (s *GossipService) NotifyMsg(data []byte) {
    var healthStatus model.HealthStatus
    if err := json.Unmarshal(data, &healthStatus); err != nil {
        s.logger.Warn("Failed to unmarshal gossip message", zap.Error(err))
        return
    }

    s.logger.Debug("Received health status",
        zap.String("node_id", healthStatus.NodeID),
        zap.String("status", string(healthStatus.Status)))
}

// GetBroadcasts implements memberlist.Delegate
func (s *GossipService) GetBroadcasts(overhead, limit int) [][]byte {
    return nil
}

// LocalState implements memberlist.Delegate
func (s *GossipService) LocalState(join bool) []byte {
    data, _ := json.Marshal(s.healthData)
    return data
}

// MergeRemoteState implements memberlist.Delegate
func (s *GossipService) MergeRemoteState(buf []byte, join bool) {
    // No-op for now
}

// UpdateHealthStatus updates the local health status
func (s *GossipService) UpdateHealthStatus(metrics model.HealthMetrics) {
    s.healthData.Timestamp = time.Now().Unix()
    s.healthData.Metrics = metrics

    // Determine status based on metrics
    if metrics.CPUUsage > 90 || metrics.MemoryUsage > 90 || metrics.DiskUsage > 90 {
        s.healthData.Status = model.NodeStatusDegraded
    } else if metrics.ErrorRate > 0.1 {
        s.healthData.Status = model.NodeStatusUnhealthy
    } else {
        s.healthData.Status = model.NodeStatusHealthy
    }

    // Broadcast to cluster
    data, _ := json.Marshal(s.healthData)
    s.memberlist.SendReliable(s.memberlist.Members()[0], data)
}

func (s *GossipService) Shutdown() error {
    return s.memberlist.Shutdown()
}

// GossipEventDelegate handles memberlist events
type GossipEventDelegate struct {
    service *GossipService
}

func (d *GossipEventDelegate) NotifyJoin(node *memberlist.Node) {
    d.service.logger.Info("Node joined",
        zap.String("node_id", node.Name),
        zap.String("addr", node.Addr.String()))
}

func (d *GossipEventDelegate) NotifyLeave(node *memberlist.Node) {
    d.service.logger.Info("Node left",
        zap.String("node_id", node.Name))
}

func (d *GossipEventDelegate) NotifyUpdate(node *memberlist.Node) {
    d.service.logger.Debug("Node updated",
        zap.String("node_id", node.Name))
}
```

## 6. Storage Layer Implementation

### 6.1 Skip List (for MemTable)

```go
// storage/memtable/skiplist.go
package memtable

import (
    "math/rand"
)

const (
    MaxLevel    = 16
    Probability = 0.5
)

type SkipListNode struct {
    key     string
    value   interface{}
    forward []*SkipListNode
}

type SkipList struct {
    head  *SkipListNode
    level int
    size  int
}

func NewSkipList() *SkipList {
    head := &SkipListNode{
        forward: make([]*SkipListNode, MaxLevel),
    }
    return &SkipList{
        head:  head,
        level: 0,
    }
}

func (sl *SkipList) randomLevel() int {
    level := 0
    for rand.Float64() < Probability && level < MaxLevel-1 {
        level++
    }
    return level
}

func (sl *SkipList) Insert(key string, value interface{}) {
    update := make([]*SkipListNode, MaxLevel)
    current := sl.head

    // Find position
    for i := sl.level; i >= 0; i-- {
        for current.forward[i] != nil && current.forward[i].key < key {
            current = current.forward[i]
        }
        update[i] = current
    }

    // Check if key exists
    current = current.forward[0]
    if current != nil && current.key == key {
        current.value = value
        return
    }

    // Insert new node
    newLevel := sl.randomLevel()
    if newLevel > sl.level {
        for i := sl.level + 1; i <= newLevel; i++ {
            update[i] = sl.head
        }
        sl.level = newLevel
    }

    newNode := &SkipListNode{
        key:     key,
        value:   value,
        forward: make([]*SkipListNode, newLevel+1),
    }

    for i := 0; i <= newLevel; i++ {
        newNode.forward[i] = update[i].forward[i]
        update[i].forward[i] = newNode
    }

    sl.size++
}

func (sl *SkipList) Search(key string) (interface{}, bool) {
    current := sl.head

    for i := sl.level; i >= 0; i-- {
        for current.forward[i] != nil && current.forward[i].key < key {
            current = current.forward[i]
        }
    }

    current = current.forward[0]
    if current != nil && current.key == key {
        return current.value, true
    }

    return nil, false
}

func (sl *SkipList) Len() int {
    return sl.size
}
```

### 6.2 SSTable Writer

```go
// storage/sstable/writer.go
package sstable

import (
    "encoding/binary"
    "fmt"
    "os"
)

type SSTableWriter struct {
    dataFile   *os.File
    indexFile  *os.File
    bloomFile  *os.File
    config     *config.SSTableConfig
    offset     int64
    index      []IndexEntry
    bloomFilter *BloomFilter
}

type IndexEntry struct {
    Key    string
    Offset int64
    Size   int32
}

func NewSSTableWriter(filePath string, config *config.SSTableConfig) (*SSTableWriter, error) {
    dataFile, err := os.Create(filePath)
    if err != nil {
        return nil, err
    }

    indexFile, err := os.Create(filePath + ".idx")
    if err != nil {
        dataFile.Close()
        return nil, err
    }

    bloomFile, err := os.Create(filePath + ".bloom")
    if err != nil {
        dataFile.Close()
        indexFile.Close()
        return nil, err
    }

    return &SSTableWriter{
        dataFile:    dataFile,
        indexFile:   indexFile,
        bloomFile:   bloomFile,
        config:      config,
        index:       make([]IndexEntry, 0),
        bloomFilter: NewBloomFilter(10000, config.BloomFilterFP),
    }, nil
}

func (w *SSTableWriter) Write(entry *model.MemTableEntry) error {
    // Serialize entry
    data, err := serializeEntry(entry)
    if err != nil {
        return err
    }

    // Write to data file
    n, err := w.dataFile.Write(data)
    if err != nil {
        return err
    }

    // Add to index
    w.index = append(w.index, IndexEntry{
        Key:    entry.Key,
        Offset: w.offset,
        Size:   int32(n),
    })

    // Add to bloom filter
    w.bloomFilter.Add(entry.Key)

    w.offset += int64(n)
    return nil
}

func (w *SSTableWriter) Finalize() error {
    // Write index
    for _, entry := range w.index {
        if err := w.writeIndexEntry(entry); err != nil {
            return err
        }
    }

    // Write bloom filter
    if err := w.bloomFilter.WriteTo(w.bloomFile); err != nil {
        return err
    }

    return nil
}

func (w *SSTableWriter) writeIndexEntry(entry IndexEntry) error {
    // Write key length
    keyLen := int32(len(entry.Key))
    if err := binary.Write(w.indexFile, binary.LittleEndian, keyLen); err != nil {
        return err
    }

    // Write key
    if _, err := w.indexFile.Write([]byte(entry.Key)); err != nil {
        return err
    }

    // Write offset and size
    if err := binary.Write(w.indexFile, binary.LittleEndian, entry.Offset); err != nil {
        return err
    }
    if err := binary.Write(w.indexFile, binary.LittleEndian, entry.Size); err != nil {
        return err
    }

    return nil
}

func (w *SSTableWriter) Size() int64 {
    return w.offset
}

func (w *SSTableWriter) Close() error {
    w.dataFile.Close()
    w.indexFile.Close()
    w.bloomFile.Close()
    return nil
}

func serializeEntry(entry *model.MemTableEntry) ([]byte, error) {
    // Simple serialization - in production use protobuf
    // Format: keyLen|key|valueLen|value|vectorClock|timestamp
    // Implementation omitted for brevity
    return nil, nil
}
```

### 6.3 Bloom Filter

```go
// storage/sstable/bloom_filter.go
package sstable

import (
    "hash/fnv"
    "math"
    "os"
)

type BloomFilter struct {
    bits      []bool
    size      uint64
    hashCount uint64
}

func NewBloomFilter(expectedElements int, falsePositiveRate float64) *BloomFilter {
    // Calculate optimal size
    size := uint64(-float64(expectedElements) * math.Log(falsePositiveRate) / (math.Ln2 * math.Ln2))

    // Calculate optimal hash count
    hashCount := uint64(float64(size) / float64(expectedElements) * math.Ln2)

    return &BloomFilter{
        bits:      make([]bool, size),
        size:      size,
        hashCount: hashCount,
    }
}

func (bf *BloomFilter) Add(key string) {
    hashes := bf.getHashes(key)
    for _, hash := range hashes {
        bf.bits[hash%bf.size] = true
    }
}

func (bf *BloomFilter) MayContain(key string) bool {
    hashes := bf.getHashes(key)
    for _, hash := range hashes {
        if !bf.bits[hash%bf.size] {
            return false
        }
    }
    return true
}

func (bf *BloomFilter) getHashes(key string) []uint64 {
    hashes := make([]uint64, bf.hashCount)

    h := fnv.New64()
    h.Write([]byte(key))
    hash1 := h.Sum64()

    h.Reset()
    h.Write([]byte(key + "salt"))
    hash2 := h.Sum64()

    for i := uint64(0); i < bf.hashCount; i++ {
        hashes[i] = hash1 + i*hash2
    }

    return hashes
}

func (bf *BloomFilter) WriteTo(file *os.File) error {
    // Serialize and write bloom filter
    // Implementation omitted for brevity
    return nil
}

func LoadBloomFilter(filePath string) (*BloomFilter, error) {
    // Load bloom filter from file
    // Implementation omitted for brevity
    return nil, nil
}
```

## 7. gRPC Handler Implementation

```go
// handler/storage_handler.go
package handler

import (
    "context"

    "go.uber.org/zap"
    "google.golang.org/grpc/codes"
    "google.golang.org/grpc/status"
)

type StorageHandler struct {
    storageService *service.StorageService
    logger         *zap.Logger
    pb.UnimplementedStorageServiceServer
}

func NewStorageHandler(
    storageSvc *service.StorageService,
    logger *zap.Logger,
) *StorageHandler {
    return &StorageHandler{
        storageService: storageSvc,
        logger:         logger,
    }
}

// Write handles write requests
func (h *StorageHandler) Write(
    ctx context.Context,
    req *pb.WriteRequest,
) (*pb.WriteResponse, error) {
    // Validate request
    if err := h.validateWriteRequest(req); err != nil {
        return nil, status.Error(codes.InvalidArgument, err.Error())
    }

    // Convert protobuf vector clock
    vectorClock := h.fromProtoVectorClock(req.VectorClock)

    // Execute write
    resp, err := h.storageService.Write(
        ctx,
        req.TenantId,
        req.Key,
        req.Value,
        vectorClock,
    )

    if err != nil {
        h.logger.Error("Write failed",
            zap.String("tenant_id", req.TenantId),
            zap.String("key", req.Key),
            zap.Error(err))
        return nil, status.Error(codes.Internal, "write failed")
    }

    return &pb.WriteResponse{
        Success:     resp.Success,
        VectorClock: h.toProtoVectorClock(resp.VectorClock),
    }, nil
}

// Read handles read requests
func (h *StorageHandler) Read(
    ctx context.Context,
    req *pb.ReadRequest,
) (*pb.ReadResponse, error) {
    // Validate request
    if err := h.validateReadRequest(req); err != nil {
        return nil, status.Error(codes.InvalidArgument, err.Error())
    }

    // Execute read
    resp, err := h.storageService.Read(
        ctx,
        req.TenantId,
        req.Key,
    )

    if err != nil {
        h.logger.Error("Read failed",
            zap.String("tenant_id", req.TenantId),
            zap.String("key", req.Key),
            zap.Error(err))
        return nil, status.Error(codes.NotFound, "key not found")
    }

    return &pb.ReadResponse{
        Success:     resp.Success,
        Value:       resp.Value,
        VectorClock: h.toProtoVectorClock(resp.VectorClock),
    }, nil
}

func (h *StorageHandler) validateWriteRequest(req *pb.WriteRequest) error {
    // Implementation details
    return nil
}

func (h *StorageHandler) validateReadRequest(req *pb.ReadRequest) error {
    // Implementation details
    return nil
}

func (h *StorageHandler) toProtoVectorClock(vc model.VectorClock) *pb.VectorClock {
    // Implementation details
    return nil
}

func (h *StorageHandler) fromProtoVectorClock(vc *pb.VectorClock) model.VectorClock {
    // Implementation details
    return model.VectorClock{}
}
```

## 8. Main Entry Point

```go
// cmd/storage/main.go
package main

import (
    "context"
    "fmt"
    "net"
    "os"
    "os/signal"
    "syscall"
    "time"

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

    // Create data directories
    os.MkdirAll(cfg.Storage.CommitLogDir, 0755)
    os.MkdirAll(cfg.Storage.SSTableDir, 0755)

    // Initialize services
    commitLogService, err := service.NewCommitLogService(
        &cfg.CommitLog,
        cfg.Storage.CommitLogDir,
        logger,
    )
    if err != nil {
        logger.Fatal("Failed to initialize commit log service", zap.Error(err))
    }
    defer commitLogService.Close()

    memTableService := service.NewMemTableService(&cfg.MemTable, logger)

    sstableService := service.NewSSTableService(
        &cfg.SSTable,
        cfg.Storage.SSTableDir,
        logger,
    )

    cacheService := service.NewCacheService(&cfg.Cache, logger)

    vectorClockService := service.NewVectorClockService()

    compactionService := service.NewCompactionService(
        &cfg.Compaction,
        sstableService,
        logger,
    )
    defer compactionService.Stop()

    storageService := service.NewStorageService(
        commitLogService,
        memTableService,
        sstableService,
        cacheService,
        vectorClockService,
        logger,
        cfg.Server.NodeID,
    )

    // Recover from commit log
    if err := commitLogService.Recover(context.Background(), memTableService); err != nil {
        logger.Error("Failed to recover from commit log", zap.Error(err))
    }

    // Initialize gossip service
    if cfg.Gossip.Enabled {
        gossipService, err := service.NewGossipService(&cfg.Gossip, cfg.Server.NodeID, logger)
        if err != nil {
            logger.Error("Failed to initialize gossip service", zap.Error(err))
        } else {
            defer gossipService.Shutdown()
        }
    }

    // Initialize handlers
    storageHandler := handler.NewStorageHandler(storageService, logger)

    // Create gRPC server
    grpcServer := grpc.NewServer(
        grpc.MaxConcurrentStreams(uint32(cfg.Server.MaxConnections)),
    )

    pb.RegisterStorageServiceServer(grpcServer, storageHandler)

    // Start listening
    addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
    listener, err := net.Listen("tcp", addr)
    if err != nil {
        logger.Fatal("Failed to listen", zap.Error(err))
    }

    logger.Info("Storage node service starting",
        zap.String("node_id", cfg.Server.NodeID),
        zap.String("address", addr))

    // Handle graceful shutdown
    go func() {
        sigChan := make(chan os.Signal, 1)
        signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
        <-sigChan

        logger.Info("Shutting down gracefully...")

        // Flush memtable
        memTableService.Flush(context.Background(), sstableService)

        grpcServer.GracefulStop()
    }()

    // Start server
    if err := grpcServer.Serve(listener); err != nil {
        logger.Fatal("Failed to serve", zap.Error(err))
    }
}
```

## 9. Configuration File Example

```yaml
# config.yaml
server:
  node_id: "storage-node-1"
  host: "0.0.0.0"
  port: 50052
  max_connections: 1000
  read_timeout: 10s
  write_timeout: 10s
  shutdown_timeout: 30s

storage:
  data_dir: "/var/lib/pairdb"
  commit_log_dir: "/var/lib/pairdb/commitlog"
  sstable_dir: "/var/lib/pairdb/sstables"
  max_disk_usage: 0.9

commit_log:
  segment_size: 1073741824  # 1GB
  max_age: 168h             # 7 days
  sync_writes: true
  buffer_size: 65536

mem_table:
  max_size: 67108864        # 64MB
  flush_threshold: 60000000 # 60MB
  num_mem_tables: 2

sstable:
  l0_size: 67108864         # 64MB
  l1_size: 134217728        # 128MB
  l2_size: 268435456        # 256MB
  level_multiplier: 2
  bloom_filter_fp: 0.01     # 1% false positive
  block_size: 4096
  index_interval: 100

cache:
  max_size: 1073741824      # 1GB
  frequency_weight: 0.5
  recency_weight: 0.5
  adaptive_window: 5m

compaction:
  l0_trigger: 4
  workers: 2
  throttle: 50              # 50 MB/s

gossip:
  enabled: true
  bind_port: 7946
  seed_nodes:
    - "storage-node-2:7946"
    - "storage-node-3:7946"
  gossip_interval: 5s
  probe_timeout: 500ms
  probe_interval: 1s

metrics:
  enabled: true
  port: 9091
  path: "/metrics"

logging:
  level: "info"
  format: "json"
```

## 10. Performance Targets

- **Write Latency**: p95 < 10ms (with commit log sync)
- **Read Latency**: p95 < 5ms (cache hit), p95 < 20ms (SSTable read)
- **Throughput**: 50,000+ writes/sec, 100,000+ reads/sec per node
- **Cache Hit Rate**: > 85%
- **Compaction**: Non-blocking, < 10% CPU overhead
- **Recovery Time**: < 1 minute for 10GB of commit logs
- **Gossip Latency**: < 30 seconds for health propagation

## 11. Testing Strategy

### 11.1 Unit Tests
- MemTable operations (insert, get, flush)
- Skip list implementation
- SSTable write and read
- Bloom filter accuracy
- Vector clock operations
- Cache eviction policies

### 11.2 Integration Tests
- Write-read cycle end-to-end
- Memtable flush to SSTable
- Compaction workflow
- Commit log recovery
- Gossip protocol integration

### 11.3 Load Tests
- Sustained write throughput
- Sustained read throughput
- Mixed read/write workload
- Cache performance under load
- Compaction overhead measurement

## 12. Deployment Checklist

- [ ] Build Docker image
- [ ] Create Kubernetes StatefulSet manifests
- [ ] Configure persistent volumes for data
- [ ] Set up resource limits (CPU, memory, disk)
- [ ] Configure health check endpoints
- [ ] Set up Prometheus monitoring
- [ ] Configure structured logging
- [ ] Set up alerting rules
- [ ] Test crash recovery
- [ ] Document operational runbooks
- [ ] Test gossip protocol connectivity
