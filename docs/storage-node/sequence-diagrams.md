# Storage Node: Sequence Diagrams

This document provides sequence diagrams for all major flows supported by the Storage Node service.

## 1. Write Operation Flow

```mermaid
sequenceDiagram
    participant Coordinator
    participant StorageService
    participant CommitLogService
    participant MemTableService
    participant MemTable
    participant SkipList
    participant CacheService
    
    Coordinator->>StorageService: Write(tenantID, key, value, vectorClock)
    StorageService->>StorageService: validateWrite(tenantID, key, value)
    StorageService->>StorageService: buildKey(tenantID, key)
    
    StorageService->>StorageService: Create CommitLogEntry
    StorageService->>CommitLogService: Append(entry)
    CommitLogService->>CommitLogService: Serialize Entry
    CommitLogService->>CommitLogService: Write to File
    CommitLogService->>CommitLogService: Sync to Disk (if enabled)
    CommitLogService-->>StorageService: Success
    
    StorageService->>StorageService: Create MemTableEntry
    StorageService->>MemTableService: Put(entry)
    MemTableService->>MemTable: Put(entry)
    MemTable->>SkipList: Insert(key, entry)
    SkipList->>SkipList: Find Position
    SkipList->>SkipList: Insert Node
    SkipList-->>MemTable: Success
    MemTable->>MemTable: Update Size
    MemTable-->>MemTableService: Success
    MemTableService-->>StorageService: Success
    
    StorageService->>CacheService: Put(key, value, vectorClock)
    CacheService->>CacheService: Update or Add Entry
    CacheService-->>StorageService: Success
    
    StorageService->>MemTableService: ShouldFlush()
    MemTableService->>MemTable: Size()
    MemTable-->>MemTableService: Current Size
    alt Size >= Flush Threshold
        MemTableService-->>StorageService: Should Flush
        StorageService->>StorageService: triggerFlush() (async)
    end
    
    StorageService-->>Coordinator: WriteResponse (Success)
```

## 2. Read Operation Flow

```mermaid
sequenceDiagram
    participant Coordinator
    participant StorageService
    participant CacheService
    participant MemTableService
    participant MemTable
    participant SkipList
    participant SSTableService
    participant SSTableReader
    participant BloomFilter
    
    Coordinator->>StorageService: Read(tenantID, key)
    StorageService->>StorageService: buildKey(tenantID, key)
    
    StorageService->>CacheService: Get(key)
    alt Cache Hit
        CacheService->>CacheService: Update Access Stats
        CacheService-->>StorageService: CacheEntry
        StorageService-->>Coordinator: ReadResponse (from Cache)
    else Cache Miss
        StorageService->>MemTableService: Get(key)
        MemTableService->>MemTable: Get(key)
        MemTable->>SkipList: Search(key)
        alt MemTable Hit
            SkipList-->>MemTable: Entry
            MemTable-->>MemTableService: MemTableEntry
            MemTableService-->>StorageService: Entry
            StorageService->>CacheService: Put(key, value, vectorClock)
            StorageService-->>Coordinator: ReadResponse (from MemTable)
        else MemTable Miss
            MemTable-->>MemTableService: Not Found
            MemTableService-->>StorageService: Not Found
            StorageService->>SSTableService: Get(tenantID, key)
            
            loop For Each Level (L0 to L4)
                SSTableService->>SSTableService: GetTablesForLevel(level)
                loop For Each SSTable
                    SSTableService->>SSTableService: keyInRange(key, keyRange)
                    alt Key in Range
                        SSTableService->>BloomFilter: LoadBloomFilter(bloomPath)
                        BloomFilter-->>SSTableService: BloomFilter
                        SSTableService->>BloomFilter: MayContain(key)
                        alt Bloom Filter Positive
                            BloomFilter-->>SSTableService: May Contain
                            SSTableService->>SSTableReader: NewSSTableReader(filePath, indexPath)
                            SSTableReader-->>SSTableService: Reader
                            SSTableService->>SSTableReader: Get(key)
                            SSTableReader->>SSTableReader: Binary Search Index
                            SSTableReader->>SSTableReader: Read from Data File
                            alt Key Found
                                SSTableReader-->>SSTableService: KeyValueEntry
                                SSTableService->>SSTableService: Check if Latest (by timestamp)
                                SSTableService->>CacheService: Put(key, value, vectorClock)
                                SSTableService-->>StorageService: Entry
                                StorageService-->>Coordinator: ReadResponse (from SSTable)
                            else Key Not Found
                                SSTableReader-->>SSTableService: Not Found
                            end
                            SSTableService->>SSTableReader: Close()
                        else Bloom Filter Negative
                            BloomFilter-->>SSTableService: Not in SSTable
                        end
                    end
                end
            end
            
            alt Key Not Found in Any SSTable
                SSTableService-->>StorageService: Not Found
                StorageService-->>Coordinator: Error (Key Not Found)
            end
        end
    end
```

## 3. MemTable Flush Flow

```mermaid
sequenceDiagram
    participant StorageService
    participant MemTableService
    participant MemTable
    participant SkipList
    participant SSTableService
    participant SSTableWriter
    participant BloomFilter
    
    StorageService->>MemTableService: triggerFlush()
    MemTableService->>MemTableService: Flush(ctx, sstableService)
    MemTableService->>MemTable: Size()
    MemTable-->>MemTableService: Size
    
    alt Size > 0
        MemTableService->>MemTableService: Make MemTable Immutable
        MemTableService->>MemTable: Create New MemTable
        MemTableService->>MemTable: Get Immutable MemTable
        
        MemTableService->>SSTableService: WriteFromMemTable(ctx, immutableMemTable)
        SSTableService->>SSTableService: Generate SSTable ID
        SSTableService->>SSTableWriter: NewSSTableWriter(filePath, config)
        SSTableWriter-->>SSTableService: Writer
        
        MemTable->>MemTable: Iterator()
        MemTable-->>MemTableService: Iterator
        
        loop For Each Entry
            MemTableService->>MemTable: Next()
            MemTable-->>MemTableService: Entry
            SSTableService->>SSTableWriter: Write(entry)
            SSTableWriter->>SSTableWriter: Serialize Entry
            SSTableWriter->>SSTableWriter: Write to Data File
            SSTableWriter->>SSTableWriter: Add to Index
            SSTableWriter->>BloomFilter: Add(key)
            SSTableWriter-->>SSTableService: Success
        end
        
        SSTableService->>SSTableWriter: Finalize()
        SSTableWriter->>SSTableWriter: Write Index File
        SSTableWriter->>BloomFilter: WriteTo(bloomFile)
        BloomFilter-->>SSTableWriter: Success
        SSTableWriter->>SSTableWriter: Close Files
        SSTableWriter-->>SSTableService: Success
        
        SSTableService->>SSTableService: Create SSTableMetadata
        SSTableService->>SSTableService: AddTable(L0, metadata)
        SSTableService-->>MemTableService: Success
        
        MemTableService->>MemTableService: Clear Immutable MemTable
    end
    
    MemTableService-->>StorageService: Flush Complete
```

## 4. Compaction Flow

```mermaid
sequenceDiagram
    participant CompactionService
    participant SSTableService
    participant SSTableReader
    participant SSTableWriter
    participant BloomFilter
    
    Note over CompactionService: Compaction Scheduler (every 30s)
    CompactionService->>CompactionService: checkCompactionNeeded()
    CompactionService->>SSTableService: GetTablesForLevel(L0)
    SSTableService-->>CompactionService: L0 Tables
    
    alt L0 Tables >= Trigger (4)
        CompactionService->>CompactionService: Create CompactionJob
        CompactionService->>CompactionService: Queue Job
        
        Note over CompactionService: Compaction Worker Processes Job
        CompactionService->>CompactionService: executeCompaction(job)
        
        CompactionService->>SSTableService: GetTablesForLevel(L0)
        SSTableService-->>CompactionService: Input Tables
        
        CompactionService->>CompactionService: mergeSSTables(ctx, inputTables, L1)
        
        loop For Each Input SSTable
            CompactionService->>SSTableReader: NewSSTableReader(table)
            SSTableReader-->>CompactionService: Reader
        end
        
        CompactionService->>SSTableWriter: NewSSTableWriter(outputPath, config)
        SSTableWriter-->>CompactionService: Writer
        
        Note over CompactionService: Merge Sort All Entries
        loop For Each Merged Entry
            CompactionService->>SSTableWriter: Write(entry)
            SSTableWriter->>SSTableWriter: Write Entry
            SSTableWriter->>BloomFilter: Add(key)
        end
        
        CompactionService->>SSTableWriter: Finalize()
        SSTableWriter->>SSTableWriter: Write Index & Bloom Filter
        SSTableWriter-->>CompactionService: Success
        
        CompactionService->>SSTableService: AddTable(L1, outputTable)
        CompactionService->>SSTableService: RemoveTables(L0, inputTableIDs)
        SSTableService->>SSTableService: Delete Old Files
        
        CompactionService->>CompactionService: Mark Job Complete
    end
```

## 5. Repair Operation Flow

```mermaid
sequenceDiagram
    participant Coordinator
    participant StorageService
    participant CommitLogService
    participant MemTableService
    participant CacheService
    
    Coordinator->>StorageService: Repair(tenantID, key, value, vectorClock)
    StorageService->>StorageService: buildKey(tenantID, key)
    
    StorageService->>StorageService: Create CommitLogEntry (OperationType=Repair)
    StorageService->>CommitLogService: Append(entry)
    CommitLogService->>CommitLogService: Write to Commit Log
    CommitLogService-->>StorageService: Success
    
    StorageService->>StorageService: Create MemTableEntry
    StorageService->>MemTableService: Put(entry)
    MemTableService->>MemTable: Put(entry)
    MemTable-->>MemTableService: Success
    MemTableService-->>StorageService: Success
    
    StorageService->>CacheService: Put(key, value, vectorClock)
    CacheService->>CacheService: Update Cache Entry
    CacheService-->>StorageService: Success
    
    StorageService-->>Coordinator: Success
```

## 6. Commit Log Recovery Flow

```mermaid
sequenceDiagram
    participant CommitLogService
    participant MemTableService
    participant MemTable
    participant SkipList
    
    Note over CommitLogService: On Startup
    CommitLogService->>CommitLogService: Recover(ctx, memTableService)
    CommitLogService->>CommitLogService: Find All Commit Log Files
    
    loop For Each Commit Log File
        CommitLogService->>CommitLogService: recoverFromFile(filePath, memTableService)
        CommitLogService->>CommitLogService: Open File
        
        loop For Each Entry in File
            CommitLogService->>CommitLogService: Read Entry
            CommitLogService->>CommitLogService: Deserialize Entry
            CommitLogService->>MemTableService: Put(entry)
            MemTableService->>MemTable: Put(entry)
            MemTable->>SkipList: Insert(key, entry)
            SkipList-->>MemTable: Success
            MemTable-->>MemTableService: Success
            MemTableService-->>CommitLogService: Success
        end
        
        CommitLogService->>CommitLogService: Close File
    end
    
    CommitLogService->>CommitLogService: Recovery Complete
    CommitLogService-->>MemTableService: Recovery Done
```

## 7. Cache Eviction Flow

```mermaid
sequenceDiagram
    participant CacheService
    participant CacheEntry
    
    Note over CacheService: On Put Operation
    CacheService->>CacheService: Check Current Size
    
    alt Current Size + Entry Size > Max Size
        loop Until Space Available
            CacheService->>CacheService: evictLowestScore()
            CacheService->>CacheService: Calculate Scores for All Entries
            
            loop For Each Entry
                CacheService->>CacheService: calculateScore(entry)
                CacheService->>CacheService: Score = frequencyWeight * accessCount - recencyWeight * timeSinceAccess
            end
            
            CacheService->>CacheService: Find Entry with Lowest Score
            CacheService->>CacheService: Remove Entry
            CacheService->>CacheService: Update Current Size
        end
    end
    
    CacheService->>CacheService: Add New Entry
    CacheService->>CacheService: Update Current Size
```

## 8. Gossip Health Monitoring Flow

```mermaid
sequenceDiagram
    participant GossipService
    participant Memberlist
    participant StorageNode1
    participant StorageNode2
    participant StorageNode3
    
    Note over GossipService: Periodic Health Update
    GossipService->>GossipService: UpdateHealthStatus(metrics)
    GossipService->>GossipService: Determine Status (Healthy/Degraded/Unhealthy)
    GossipService->>GossipService: Update Health Data
    
    GossipService->>Memberlist: SendReliable(member, healthData)
    Memberlist->>StorageNode1: Gossip Message
    StorageNode1->>StorageNode1: Update Peer Health Status
    
    GossipService->>Memberlist: SendReliable(member, healthData)
    Memberlist->>StorageNode2: Gossip Message
    StorageNode2->>StorageNode2: Update Peer Health Status
    
    Note over StorageNode1,StorageNode3: Health Status Propagates
    
    StorageNode1->>Memberlist: SendReliable(healthData)
    Memberlist->>StorageNode3: Gossip Message
    StorageNode3->>StorageNode3: Update Peer Health Status
    
    Note over GossipService,StorageNode3: Eventually Consistent Health Status
```

## Flow Descriptions

### Write Operation Flow
1. Validate write parameters
2. Write to commit log (durability)
3. Write to memtable (in-memory)
4. Update cache
5. Check if memtable needs flushing
6. Return success

### Read Operation Flow
1. Check cache first (fastest)
2. If cache miss, check memtable
3. If memtable miss, search SSTables (L0 to L4)
4. Use bloom filter to skip non-existent keys
5. Binary search index for fast lookup
6. Return latest version based on timestamp
7. Update cache with retrieved value

### MemTable Flush Flow
1. Check if memtable size exceeds threshold
2. Make current memtable immutable
3. Create new memtable for writes
4. Iterate through immutable memtable
5. Write entries to new L0 SSTable
6. Create index and bloom filter
7. Add SSTable metadata to L0
8. Clear immutable memtable

### Compaction Flow
1. Scheduler checks compaction triggers
2. If L0 has enough SSTables, create compaction job
3. Worker merges multiple SSTables
4. Sort and deduplicate entries
5. Write merged SSTable to next level
6. Remove old SSTables
7. Update SSTable metadata

### Repair Operation Flow
1. Receive repair request with latest value
2. Write to commit log
3. Update memtable
4. Update cache
5. Ensure all layers have latest version

### Commit Log Recovery Flow
1. On startup, scan commit log directory
2. Read all commit log files
3. Replay entries to memtable
4. Restore in-memory state
5. Ready for normal operations

### Cache Eviction Flow
1. Check cache size on put
2. If full, calculate scores for all entries
3. Evict entry with lowest adaptive score
4. Repeat until space available
5. Add new entry

### Gossip Health Monitoring Flow
1. Periodically update local health status
2. Broadcast to random peers
3. Receive health updates from peers
4. Propagate through cluster
5. Eventually consistent health view

