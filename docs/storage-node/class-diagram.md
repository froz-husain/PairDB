# Storage Node: Class Diagram

This document provides a class diagram showing the core entities and their relationships in the Storage Node service.

## Class Diagram

```mermaid
classDiagram
    class StorageService {
        -commitLogService: *CommitLogService
        -memTableService: *MemTableService
        -sstableService: *SSTableService
        -cacheService: *CacheService
        -vectorClockService: *VectorClockService
        -logger: *zap.Logger
        -nodeID: string
        +Write(ctx, tenantID, key, value, vectorClock) *WriteResponse
        +Read(ctx, tenantID, key) *ReadResponse
        +Repair(ctx, tenantID, key, value, vectorClock) error
        +triggerFlush()
        +validateWrite(tenantID, key, value) error
        +buildKey(tenantID, key) string
    }
    
    class CommitLogService {
        -config: *CommitLogConfig
        -currentFile: *os.File
        -writer: *CommitLogWriter
        -logger: *zap.Logger
        -mu: sync.Mutex
        -dataDir: string
        -segmentID: int64
        +Append(ctx, entry) error
        +Recover(ctx, memTableSvc) error
        +openNewSegment() error
        +checkRotation()
        +Close() error
    }
    
    class MemTableService {
        -config: *MemTableConfig
        -memTable: *MemTable
        -immutableMT: *MemTable
        -logger: *zap.Logger
        -mu: sync.RWMutex
        -flushMu: sync.Mutex
        +Put(ctx, entry) error
        +Get(ctx, key) *MemTableEntry, bool
        +ShouldFlush() bool
        +Flush(ctx, sstableSvc) error
    }
    
    class MemTable {
        -data: *SkipList
        -maxSize: int64
        -size: int64
        -mu: sync.RWMutex
        +Put(entry) error
        +Get(key) *MemTableEntry, bool
        +Size() int64
        +Count() int
        +Iterator() *MemTableIterator
    }
    
    class SkipList {
        -head: *SkipListNode
        -level: int
        -size: int
        +Insert(key, value)
        +Search(key) interface{}, bool
        +Len() int
        +randomLevel() int
    }
    
    class SSTableService {
        -config: *SSTableConfig
        -dataDir: string
        -logger: *zap.Logger
        -levels: map[SSTableLevel][]*SSTableMetadata
        -mu: sync.RWMutex
        +WriteFromMemTable(ctx, memTable) error
        +Get(ctx, tenantID, key) *KeyValueEntry
        +GetTablesForLevel(level) []*SSTableMetadata
        +AddTable(level, table)
        +RemoveTables(level, tableIDs)
        +keyInRange(key, keyRange) bool
    }
    
    class SSTableWriter {
        -dataFile: *os.File
        -indexFile: *os.File
        -bloomFile: *os.File
        -config: *SSTableConfig
        -offset: int64
        -index: []IndexEntry
        -bloomFilter: *BloomFilter
        +Write(entry) error
        +Finalize() error
        +Size() int64
        +Close() error
    }
    
    class SSTableReader {
        -dataFile: *os.File
        -indexFile: *os.File
        -index: []IndexEntry
        +Get(key) *KeyValueEntry
        +Close() error
    }
    
    class BloomFilter {
        -bits: []bool
        -size: uint64
        -hashCount: uint64
        +Add(key)
        +MayContain(key) bool
        +WriteTo(file) error
    }
    
    class CacheService {
        -config: *CacheConfig
        -cache: map[string]*CacheEntry
        -evictionList: *EvictionList
        -logger: *zap.Logger
        -mu: sync.RWMutex
        -currentSize: int64
        -frequencyWeight: float64
        -recencyWeight: float64
        +Get(key) *CacheEntry, bool
        +Put(key, value, vectorClock)
        +calculateScore(entry) float64
        +evictLowestScore()
        +AdjustWeights()
        +Stats() CacheStats
    }
    
    class CompactionService {
        -config: *CompactionConfig
        -sstableService: *SSTableService
        -logger: *zap.Logger
        -jobQueue: chan *CompactionJob
        -stopChan: chan struct{}
        -wg: sync.WaitGroup
        +compactionScheduler()
        +checkCompactionNeeded()
        +shouldCompactLevel(level) bool
        +triggerLevelCompaction(level)
        +compactionWorker(workerID)
        +executeCompaction(job)
        +mergeSSTables(ctx, inputTables, outputLevel) *SSTableMetadata
        +Stop()
    }
    
    class VectorClockService {
        +Increment(nodeID) VectorClock
        +Merge(clocks...) VectorClock
        +Compare(vc1, vc2) VectorClockComparison
    }
    
    class GossipService {
        -config: *GossipConfig
        -memberlist: *memberlist.Memberlist
        -nodeID: string
        -logger: *zap.Logger
        -healthData: *HealthStatus
        +NodeMeta(limit) []byte
        +NotifyMsg(data)
        +GetBroadcasts(overhead, limit) [][]byte
        +LocalState(join) []byte
        +MergeRemoteState(buf, join)
        +UpdateHealthStatus(metrics)
        +Shutdown() error
    }
    
    class KeyValueEntry {
        +TenantID: string
        +Key: string
        +Value: []byte
        +VectorClock: VectorClock
        +Timestamp: int64
    }
    
    class MemTableEntry {
        +Key: string
        +Value: []byte
        +VectorClock: VectorClock
        +Timestamp: int64
    }
    
    class CacheEntry {
        +Key: string
        +Value: []byte
        +VectorClock: VectorClock
        +AccessCount: int64
        +LastAccess: time.Time
        +Score: float64
    }
    
    class CommitLogEntry {
        +TenantID: string
        +Key: string
        +Value: []byte
        +VectorClock: VectorClock
        +Timestamp: int64
        +OperationType: OperationType
    }
    
    class SSTableMetadata {
        +SSTableID: string
        +TenantID: string
        +Level: int
        +Size: int64
        +KeyRange: KeyRange
        +CreatedAt: time.Time
        +FilePath: string
        +IndexPath: string
        +BloomPath: string
    }
    
    class CompactionJob {
        +JobID: string
        +Level: SSTableLevel
        +InputTables: []*SSTableMetadata
        +OutputLevel: SSTableLevel
        +StartedAt: time.Time
        +Status: CompactionStatus
    }
    
    class HealthStatus {
        +NodeID: string
        +Status: NodeStatus
        +Timestamp: int64
        +Metrics: HealthMetrics
    }
    
    class VectorClock {
        +Entries: []VectorClockEntry
    }
    
    StorageService --> CommitLogService : uses
    StorageService --> MemTableService : uses
    StorageService --> SSTableService : uses
    StorageService --> CacheService : uses
    StorageService --> VectorClockService : uses
    
    MemTableService --> MemTable : uses
    MemTable --> SkipList : uses
    
    MemTableService --> SSTableService : flushes to
    
    SSTableService --> SSTableWriter : uses
    SSTableService --> SSTableReader : uses
    SSTableService --> BloomFilter : uses
    
    SSTableWriter --> BloomFilter : creates
    
    CompactionService --> SSTableService : uses
    
    CommitLogService --> MemTableService : recovers to
```

## Class Descriptions

### StorageService
- Main service orchestrating all storage operations
- Coordinates writes through commit log, memtable, and cache
- Handles reads from cache, memtable, and SSTables
- Manages repair operations

### CommitLogService
- Manages write-ahead log (WAL) for durability
- Handles log rotation and segment management
- Recovers data from commit logs on startup
- Ensures data persistence

### MemTableService
- Manages in-memory sorted table for recent writes
- Handles memtable flushing to SSTables
- Supports immutable memtable during flush
- Provides fast write and read operations

### MemTable
- In-memory sorted table using skip list
- Tracks size and entry count
- Provides iterator for flushing
- Thread-safe operations

### SkipList
- Probabilistic data structure for O(log n) operations
- Supports insert, search, and iteration
- Used as underlying structure for memtable

### SSTableService
- Manages SSTable files organized by levels
- Handles writing from memtable to L0 SSTables
- Provides read operations with bloom filter optimization
- Manages SSTable metadata and indexing

### SSTableWriter
- Writes SSTable files with data, index, and bloom filter
- Maintains offset tracking for indexing
- Finalizes SSTable with metadata

### SSTableReader
- Reads from SSTable files using index
- Provides fast key lookups
- Handles file I/O operations

### BloomFilter
- Probabilistic data structure for membership testing
- Reduces unnecessary disk I/O
- Configurable false positive rate

### CacheService
- Adaptive cache combining LRU and LFU
- Dynamically adjusts weights based on workload
- Evicts entries based on adaptive score
- Tracks access patterns

### CompactionService
- Background service for SSTable compaction
- Manages compaction jobs and workers
- Triggers compaction based on level thresholds
- Merges SSTables to optimize storage

### VectorClockService
- Manages vector clocks for causality tracking
- Compares and merges vector clocks
- Supports conflict detection

### GossipService
- Implements gossip protocol for health monitoring
- Broadcasts node health status
- Receives health updates from other nodes
- Integrates with memberlist library

