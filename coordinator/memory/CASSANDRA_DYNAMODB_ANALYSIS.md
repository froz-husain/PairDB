# How Cassandra and DynamoDB Handle Node Addition at Scale

**Date**: January 9, 2026
**Research Focus**: Production-grade migration strategies

## Executive Summary

After analyzing Cassandra and DynamoDB implementations, the key insight is:

> **"Update the ring immediately, accept temporary inconsistency, repair asynchronously"**

Both systems prioritize **availability over immediate consistency** during migrations, using:
- Immediate hash ring updates (no cutover phase)
- Multi-version data (old + new nodes both have data)
- Hinted handoff for missed writes
- Background repair/anti-entropy
- Read repair for consistency

---

## Apache Cassandra: Bootstrap Process

### Architecture Overview

Cassandra uses a **token ring** with virtual nodes (vnodes). Each node owns ranges of the token space.

### Node Addition Flow

```
PHASE 1: JOINING
├─ New node announces itself to cluster
├─ Gossip protocol spreads the news (seconds)
├─ Ring is updated IMMEDIATELY across cluster
└─ Node state: "joining" (visible to all)

PHASE 2: STREAMING (Background)
├─ New node calculates token ranges it owns
├─ Existing nodes stream data to new node
├─ Writes during streaming go to BOTH old and new owners
│  └─ This is automatic (ring-based routing)
└─ Node state: still "joining"

PHASE 3: NORMAL
├─ Streaming completes
├─ Node state: "normal" (fully operational)
└─ Old nodes can delete transferred data
```

### Key Insight: No Cutover Phase!

**Cassandra's Strategy:**
```
1. Update ring immediately
2. Accept that new node has incomplete data
3. Route writes to new node immediately
4. Stream historical data in background
5. Use "consistency level" to handle partial data
```

### How Reads/Writes Work During Bootstrap

#### Write Path

```go
// Cassandra write during bootstrap
func WriteKey(key, value) {
    // 1. Hash key to find token
    token := hash(key)

    // 2. Get replicas based on CURRENT ring (includes new node!)
    replicas := ring.GetReplicas(token, replicationFactor=3)
    // If new node owns this token, it's in the list

    // 3. Write to ALL replicas (includes new node even if bootstrapping)
    for _, replica := range replicas {
        // Async write (no waiting)
        go replica.Write(key, value, timestamp)

        // If write fails, store "hint" locally
        if err != nil {
            storeHint(replica, key, value)
        }
    }

    // 4. Wait for QUORUM (2/3) to acknowledge
    // New node might not have old data, but has NEW writes ✓
}
```

**Critical Point**: New node immediately receives NEW writes, even though it doesn't have old data yet.

#### Read Path

```go
// Cassandra read during bootstrap
func ReadKey(key, consistency=QUORUM) {
    token := hash(key)
    replicas := ring.GetReplicas(token, replicationFactor=3)

    // Read from multiple replicas in parallel
    responses := []Response{}
    for _, replica := range replicas {
        resp := replica.Read(key)
        responses = append(responses, resp)
    }

    // Compare timestamps/versions
    if consistency == QUORUM {
        // Need 2/3 responses
        if len(responses) >= 2 {
            // Return most recent version (highest timestamp)
            latest := findLatestVersion(responses)

            // READ REPAIR: If replicas have different versions,
            // send latest to stale replicas (async)
            if hasConflicts(responses) {
                go readRepair(replicas, latest)
            }

            return latest
        }
    }
}
```

**Key Mechanism: Read Repair**
- Reads may return stale data from new node (it's bootstrapping)
- But QUORUM ensures at least one old node responds
- Read repair updates stale replicas in background

### Streaming Protocol (Background)

```
New Node (Joining):
  1. Calculates token ranges it will own
  2. Identifies which nodes currently own those ranges
  3. Opens streaming sessions with those nodes

Old Node (Streaming Source):
  FOR EACH token range being transferred:
    - Scan local data in that range
    - Stream to new node
    - Mark data as "pending delete" (not deleted immediately)
    - After streaming completes, delete transferred data

During Streaming:
  - NEW WRITES go to both old and new owners
  - Old node still serves reads (until streaming done)
  - New node accepts writes but may have incomplete data
```

### Hinted Handoff

**Problem**: What if new node is down when write arrives?

**Cassandra's Solution**:
```
Coordinator receives write for key owned by Node-4 (bootstrapping)

1. Try to write to Node-4
   └─ Fails (node temporarily down)

2. Store "hint" locally
   hints[node-4].append(key, value, timestamp)

3. Background process retries hints
   └─ When Node-4 comes back, replay all hints

Result: Eventual consistency guaranteed
```

### Gossip Protocol for Ring Updates

```
Node-1: "New node Node-4 is joining with tokens [100-200, 300-400]"
  └─ Gossips to random nodes every 1 second
      └─ Node-2 receives gossip
          └─ Updates local ring view
          └─ Gossips to more nodes
              └─ Within 10 seconds, entire cluster knows

Result: Ring updates propagate in SECONDS (not minutes)
```

---

## Amazon DynamoDB: Partition Rebalancing

### Architecture Overview

DynamoDB uses **consistent hashing** with virtual nodes (similar to Cassandra). Data is partitioned across storage nodes.

### Key Differences from Cassandra

1. **Managed Service**: AWS handles node addition transparently
2. **No Client-Side Coordination**: Clients don't see individual nodes
3. **Automatic Rebalancing**: Triggered by load, not manual addition

### Node Addition Flow (Simplified)

```
PHASE 1: DECISION
├─ System detects need for more capacity
│  └─ Hot partition, high throughput, etc.
├─ Allocate new storage node
└─ Update ring metadata (in control plane)

PHASE 2: DUAL-OWNERSHIP
├─ Partition is owned by BOTH old and new nodes
├─ Writes go to BOTH nodes (synchronous)
├─ Reads prefer new node, fallback to old
└─ State: "migrating"

PHASE 3: BACKGROUND COPY
├─ New node pulls data from old node
├─ Uses "snapshot + stream" approach:
│  └─ Snapshot: Bulk copy at time T
│  └─ Stream: Replay writes since T
└─ Verify checksum

PHASE 4: OWNERSHIP TRANSFER
├─ New node confirms it has all data
├─ Update metadata: partition now owned by new node only
├─ Old node deletes data
└─ State: "normal"
```

### Write Path During Migration

```
Client → API Gateway → Request Router → Storage Nodes

// DynamoDB write during partition migration
func PutItem(key, value) {
    partition := hash(key) % numPartitions

    // Check partition state
    state := getPartitionState(partition)

    if state == "migrating" {
        // DUAL-WRITE: Write to BOTH old and new nodes
        oldNode := getOldOwner(partition)
        newNode := getNewOwner(partition)

        // SYNCHRONOUS writes to both (not async!)
        err1 := oldNode.Write(key, value)
        err2 := newNode.Write(key, value)

        // Success if BOTH succeed
        if err1 != nil || err2 != nil {
            return error("write failed during migration")
        }

        return success
    } else {
        // Normal write (single owner)
        node := getCurrentOwner(partition)
        return node.Write(key, value)
    }
}
```

**Critical Difference**: DynamoDB's dual-write is **synchronous** (both must succeed), while Cassandra's is **asynchronous** (best effort + hints).

### Read Path During Migration

```go
func GetItem(key) {
    partition := hash(key) % numPartitions
    state := getPartitionState(partition)

    if state == "migrating" {
        // Read from NEW node first (it has latest data)
        newNode := getNewOwner(partition)
        value, err := newNode.Read(key)

        if err == nil && value != nil {
            return value
        }

        // Fallback to OLD node (might have data new node doesn't have yet)
        oldNode := getOldOwner(partition)
        value, err = oldNode.Read(key)

        if err == nil && value != nil {
            // Read repair: Copy to new node
            go newNode.Write(key, value)
            return value
        }

        return notFound
    } else {
        // Normal read (single owner)
        node := getCurrentOwner(partition)
        return node.Read(key)
    }
}
```

### Multi-Master Writes (Eventual Consistency)

DynamoDB (when using eventual consistency):
```
Write arrives at partition-123

Partition-123 is owned by replicas: [Node-A, Node-B, Node-C]

Write Path:
1. Write to Node-A (primary) - SYNCHRONOUS
   └─ Returns success to client immediately

2. Replicate to Node-B, Node-C - ASYNCHRONOUS
   └─ Background replication queue
   └─ Retries with exponential backoff

3. If replication fails:
   └─ Anti-entropy repair finds inconsistencies
   └─ Merkle tree comparison (periodic)
```

---

## Comparison: Cassandra vs DynamoDB vs Our Current Approach

| Aspect | Cassandra | DynamoDB | Current PairDB | Your Proposal |
|--------|-----------|----------|----------------|---------------|
| **Ring Update Timing** | Immediate (gossip) | Immediate (metadata) | After migration | After migration |
| **Write During Migration** | Async to all replicas | Sync to old+new | Coordinator dual-write | Storage node streaming |
| **Read During Migration** | Quorum + read repair | New node first, fallback old | Old nodes only ❌ | Old nodes only ❌ |
| **Consistency Model** | Eventual (tunable) | Eventual (strong available) | Strong | Strong |
| **Failure Handling** | Hinted handoff | Retry queue | Best effort ❌ | Retry needed |
| **Data Copy** | Streaming protocol | Snapshot + stream | Separate task | Streaming protocol |
| **Cutover** | None (gradual) | Atomic ownership transfer | Atomic hash ring update | Atomic hash ring update |

---

## Key Insights from Production Systems

### 1. **Ring Updates IMMEDIATELY** ✅

Both Cassandra and DynamoDB update the ring/metadata **as soon as** the new node is added:

```
Cassandra: Gossip spreads ring update in SECONDS
DynamoDB: Metadata update in control plane (sub-second)

Our Current Approach: Wait until migration completes ❌
Better: Update ring immediately, handle partial data
```

### 2. **Writes Go to New Node Immediately** ✅

**Cassandra**:
```go
// New node immediately in replica set
replicas := GetReplicas(key) // Includes new node
for replica in replicas {
    go replica.Write(key, value) // Async
}
```

**DynamoDB**:
```go
// Dual-write during migration
if migrating {
    oldNode.Write(key, value) // Sync
    newNode.Write(key, value) // Sync
}
```

**Our Current**: Coordinator checks migration state ❌
**Better**: Ring determines replicas automatically

### 3. **Read Repair for Consistency** ✅

Both systems rely on **read repair** instead of perfect replication:

```go
// On read, if replicas disagree:
func ReadRepair(key, replicas, latestValue) {
    for replica in replicas {
        if replica.version < latestValue.version {
            // Async update stale replica
            go replica.Write(key, latestValue)
        }
    }
}
```

This is **cheaper** than synchronous replication during migration.

### 4. **Hinted Handoff for Failures** ✅

**Cassandra's Approach**:
```
Write arrives for key on Node-4 (bootstrapping, temporarily down)

Coordinator:
1. Try to write to Node-4 → Fails
2. Store hint locally: hints[node-4].append(key, value)
3. Return success to client (other replicas succeeded)
4. Background: Replay hints when Node-4 recovers

Result: Zero data loss, eventual consistency
```

**Our Current**: Dual-write failures ignored (potential data loss) ❌

### 5. **Streaming is Background, Not Blocking** ✅

**Cassandra**:
```
Streaming happens AFTER ring update:
1. New node receives NEW writes immediately
2. Old data streamed in background (may take hours)
3. Node is "operational" even with incomplete data
4. Consistency level (QUORUM) ensures correctness
```

**Our Current**: Streaming must complete before cutover ❌
**Your Proposal**: Streaming before cutover ❌
**Better**: Stream after ring update, rely on consistency level

### 6. **No Global Cutover Phase** ✅

Neither system has a "cutover" moment:

**Cassandra**: Gradual transition
```
t0: Ring updated, new node state="joining"
t1: New node receives writes, streams old data
t2: Streaming completes, state="normal"
    └─ No atomic switch, just status change
```

**DynamoDB**: Ownership transfer
```
t0: Partition in "migrating" state
t1: Writes to both old+new, reads prefer new
t2: Data copy complete, ownership=new node only
    └─ Single metadata update (not global)
```

---

## Recommended Strategy for PairDB (Based on Production Systems)

### Strategy: Immediate Ring Update + Consistency Levels

```
PHASE 1: ADD NODE TO RING IMMEDIATELY
├─ Node added with status="bootstrapping"
├─ Hash ring updated INSTANTLY
├─ Gossip/broadcast ring update to all coordinators
└─ New node IMMEDIATELY receives writes

PHASE 2: BACKGROUND STREAMING
├─ New node identifies ranges it owns
├─ Streams data from old owners (background)
├─ Old owners still serve reads
└─ Both old and new have overlapping data

PHASE 3: MARK AS NORMAL
├─ Streaming completes
├─ Node status="normal"
├─ Old owners can delete transferred keys
└─ No cutover needed (gradual transition)
```

### Write Path (Simplified)

```go
// NO migration checks needed!
func (s *CoordinatorService) WriteKeyValue(ctx context.Context, tenantID, key string, value []byte, consistency string) (*WriteResult, error) {
    // 1. Get replicas from current ring (includes bootstrapping nodes)
    replicas := s.routingService.GetReplicas(ctx, tenantID, key, replicationFactor)

    // 2. Write to all replicas (async)
    responses := make(chan *WriteResponse, len(replicas))
    for _, replica := range replicas {
        go func(node *StorageNode) {
            resp, err := s.storageClient.Write(ctx, node, tenantID, key, value, vectorClock)
            if err != nil {
                // Store hint for later replay
                s.hintedHandoff.StoreHint(node.NodeID, key, value)
                responses <- &WriteResponse{NodeID: node.NodeID, Success: false}
            } else {
                responses <- resp
            }
        }(replica)
    }

    // 3. Wait for quorum (no distinction between old/new nodes)
    successCount := 0
    for i := 0; i < len(replicas); i++ {
        resp := <-responses
        if resp.Success {
            successCount++
        }

        // Return as soon as quorum reached
        if s.consistencyService.IsQuorumReached(successCount, len(replicas), consistency) {
            return &WriteResult{Success: true, ReplicaCount: int32(successCount)}, nil
        }
    }

    return nil, fmt.Errorf("quorum not reached: %d/%d", successCount, len(replicas))
}
```

### Read Path with Read Repair

```go
func (s *CoordinatorService) ReadKeyValue(ctx context.Context, tenantID, key string, consistency string) (*ReadResult, error) {
    replicas := s.routingService.GetReplicas(ctx, tenantID, key, replicationFactor)

    // Read from all replicas (parallel)
    responses := make([]*StorageResponse, 0, len(replicas))
    for _, replica := range replicas {
        resp, err := s.storageClient.Read(ctx, replica, tenantID, key)
        if err == nil {
            responses = append(responses, resp)
        }
    }

    // Check quorum
    if len(responses) < s.consistencyService.GetRequiredReplicas(consistency, len(replicas)) {
        return nil, fmt.Errorf("quorum not reached")
    }

    // Find latest version (by vector clock)
    latest := s.conflictService.ResolveConflicts(responses)

    // READ REPAIR: Update stale replicas (async)
    for _, resp := range responses {
        if !resp.VectorClock.Equals(latest.VectorClock) {
            go func(node *StorageNode) {
                s.storageClient.Write(ctx, node, tenantID, key, latest.Value, latest.VectorClock)
            }(resp.Node)
        }
    }

    return &ReadResult{
        Success: true,
        Value:   latest.Value,
        VectorClock: latest.VectorClock,
    }, nil
}
```

### Hinted Handoff Service

```go
type HintedHandoffService struct {
    hints map[string][]*Hint // nodeID -> hints
    mu    sync.RWMutex
}

type Hint struct {
    Key         string
    Value       []byte
    VectorClock VectorClock
    Timestamp   time.Time
}

func (h *HintedHandoffService) StoreHint(nodeID string, key string, value []byte) {
    h.mu.Lock()
    defer h.mu.Unlock()

    h.hints[nodeID] = append(h.hints[nodeID], &Hint{
        Key:       key,
        Value:     value,
        Timestamp: time.Now(),
    })
}

func (h *HintedHandoffService) ReplayHints(ctx context.Context, nodeID string) error {
    h.mu.RLock()
    nodeHints := h.hints[nodeID]
    h.mu.RUnlock()

    for _, hint := range nodeHints {
        // Replay hint to node
        err := h.storageClient.Write(ctx, node, hint.Key, hint.Value, hint.VectorClock)
        if err == nil {
            // Remove hint after successful replay
            h.removeHint(nodeID, hint)
        }
    }

    return nil
}

// Background process
func (h *HintedHandoffService) Start() {
    ticker := time.NewTicker(10 * time.Second)
    for range ticker.C {
        h.mu.RLock()
        for nodeID := range h.hints {
            go h.ReplayHints(context.Background(), nodeID)
        }
        h.mu.RUnlock()
    }
}
```

---

## Comparison: Three Approaches

### Approach 1: Current Implementation (Coordinator Dual-Write)

```
❌ Coordinator checks migration state on every write
❌ Dual-write logic in coordinator
❌ Reads don't include new nodes
❌ Cutover has race conditions
❌ No hinted handoff
```

### Approach 2: Your Proposal (Storage Node Streaming)

```
✅ Storage nodes handle streaming
✅ Cleaner separation of concerns
❌ Hash ring not updated until cutover
❌ Reads don't include new nodes during streaming
❌ Global cutover phase (doesn't scale)
⚠️ Better than current, but not optimal
```

### Approach 3: Cassandra/DynamoDB Style (Immediate Ring Update)

```
✅ Hash ring updated immediately
✅ New nodes receive writes instantly
✅ Reads automatically include new nodes
✅ No coordinator migration checks
✅ Hinted handoff for failures
✅ Read repair for consistency
✅ No global cutover phase
✅ Scales to hundreds of nodes
```

---

## Recommendation: Implement Approach 3

**Why**:
1. **Proven at scale** (Cassandra: 1000+ node clusters, DynamoDB: millions of partitions)
2. **Simpler coordinator logic** (no migration checks)
3. **Better availability** (gradual transition, no cutover)
4. **Automatic consistency** (read repair, hinted handoff)
5. **Symmetric for removal** (same pattern)

**Implementation Plan**:

### Week 1: Core Changes
- [ ] Add `HintedHandoffService`
- [ ] Implement read repair in `ReadKeyValue`
- [ ] Remove migration checks from write path
- [ ] Update `GetReplicas()` to include bootstrapping nodes

### Week 2: Streaming
- [ ] Add node status: "bootstrapping"
- [ ] Implement background streaming on storage nodes
- [ ] Add progress tracking

### Week 3: Testing
- [ ] Test bootstrap with writes during streaming
- [ ] Test read repair correctness
- [ ] Test hinted handoff replay
- [ ] Failure injection tests

### Week 4: Migration
- [ ] Feature flag: old vs new approach
- [ ] Gradual rollout
- [ ] Remove old code

---

## Conclusion

**Key Insight**: Don't fight eventual consistency, embrace it!

Cassandra and DynamoDB show that:
1. ✅ Update ring immediately (don't wait)
2. ✅ Accept temporary inconsistency
3. ✅ Use consistency levels (QUORUM) to hide partial data
4. ✅ Repair asynchronously (read repair, anti-entropy)
5. ✅ No global cutover (gradual transition)

This approach **scales to massive clusters** because it avoids coordination bottlenecks.

**Recommendation**: Implement Approach 3 (Cassandra/DynamoDB style) for production-grade scalability.
