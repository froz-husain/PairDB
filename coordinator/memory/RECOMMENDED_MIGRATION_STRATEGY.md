# Recommended Migration Strategy: Streaming-Based Node Addition/Removal

**Date**: January 9, 2026
**Status**: Design Proposal

## Overview

This document describes a **streaming-based migration strategy** that eliminates the consistency issues found in the current coordinator-based dual-write implementation. The key insight: **let storage nodes handle streaming**, not the coordinator.

## Current Implementation Problems

### What We Have Now (Coordinator-Based Dual Write)

```
Write Request
    ↓
Coordinator checks: IsDualWriteActive()
    ↓
Coordinator writes to: [OldNodes] + [NewNode]
    ↓
Problem: Coordinator manages dual-write logic
```

**Issues**:
- ❌ Coordinator must track all migrations
- ❌ Every write checks migration state (overhead)
- ❌ Data copy doesn't coordinate with live writes
- ❌ Cutover race conditions
- ❌ No quorum overlap during transition

---

## Recommended Strategy: Storage-Node-Based Streaming

### Core Principle

> **"The hash ring dictates routing. Storage nodes handle data movement transparently."**

### The Strategy

```
Phase 1: PROVISIONING (Node Status = "provisioning")
  - New node added to metadata store with status="provisioning"
  - Hash ring NOT updated (writes still go to old nodes)
  - Old nodes detect they need to stream to new node
  - Background: Copy existing data + stream new writes

Phase 2: SYNCING (Node Status = "syncing")
  - Data copy completed
  - Old nodes still streaming new writes
  - New node catches up to real-time
  - Verify: Old and new nodes have identical data

Phase 3: CUTOVER (Node Status = "active")
  - Update hash ring atomically
  - New writes now routed to new node
  - Old nodes stop streaming
  - Old nodes can delete keys no longer in their range

Phase 4: CLEANUP (Optional)
  - Remove old keys from old nodes
  - Update metrics and monitoring
```

---

## Implementation Architecture

### 1. Storage Node Changes (Core Innovation)

**New Component**: `StreamingManager` on each storage node

```go
// On storage node
type StreamingManager struct {
    localNode      *StorageNode
    metadataStore  MetadataStore

    // Track which keys we're streaming to which nodes
    activeStreams  map[string]*StreamContext // nodeID -> StreamContext
    mu             sync.RWMutex
}

type StreamContext struct {
    TargetNodeID   string
    TargetHost     string
    TargetPort     int
    KeyRanges      []KeyRange        // Which keys to stream
    State          StreamState       // "copying" | "streaming" | "syncing"

    // Metrics
    KeysCopied     int64
    KeysStreamed   int64
    LastSyncTime   time.Time
}

type KeyRange struct {
    StartHash uint32
    EndHash   uint32
}
```

### 2. Node Addition Flow

#### Step 1: Node Added with "provisioning" Status

**Coordinator receives**: `AddStorageNode(nodeID="node-4", host="10.0.0.4", port=9090)`

```go
// coordinator/internal/handler/node_handler.go
func (h *NodeHandler) AddStorageNode(ctx context.Context, req *pb.AddStorageNodeRequest) (*pb.AddStorageNodeResponse, error) {
    // Create node with "provisioning" status
    node := &model.StorageNode{
        NodeID:       req.NodeId,
        Host:         req.Host,
        Port:         int(req.Port),
        Status:       model.NodeStatusProvisioning,  // NEW STATUS
        VirtualNodes: virtualNodes,
    }

    // Add to metadata store (NOT to hash ring yet!)
    if err := h.metadataStore.AddStorageNode(ctx, node); err != nil {
        return nil, status.Error(codes.Internal, err.Error())
    }

    // Calculate which existing nodes need to stream to this new node
    affectedNodes, keyRanges := h.routingService.CalculateKeyRanges(ctx, node)

    // Notify affected nodes to start streaming
    for _, oldNode := range affectedNodes {
        if err := h.notifyNodeToStartStreaming(ctx, oldNode, node, keyRanges[oldNode.NodeID]); err != nil {
            h.logger.Error("Failed to notify node to start streaming",
                zap.String("old_node", oldNode.NodeID),
                zap.String("new_node", node.NodeID),
                zap.Error(err))
        }
    }

    return &pb.AddStorageNodeResponse{
        Success: true,
        NodeId:  node.NodeID,
        Message: "Node provisioning started, data streaming initiated",
        Status:  string(model.NodeStatusProvisioning),
    }, nil
}
```

#### Step 2: Calculate Key Ranges

**Key Innovation**: Determine which keys will move from old nodes to new node

```go
// coordinator/internal/service/routing_service.go
func (s *RoutingService) CalculateKeyRanges(ctx context.Context, newNode *model.StorageNode) (map[string][]*KeyRange, error) {
    s.mu.RLock()
    defer s.mu.RUnlock()

    // Simulate adding the new node to a COPY of the hash ring
    tempRing := s.hashRing.Clone()
    tempRing.AddNode(newNode.NodeID, newNode.VirtualNodes)

    // For each existing node, find which key ranges will move to new node
    affectedRanges := make(map[string][]*KeyRange)

    // Get all virtual nodes in the ring
    allVNodes := tempRing.GetAllVirtualNodes()

    for _, vnode := range allVNodes {
        if vnode.NodeID == newNode.NodeID {
            // This virtual node belongs to the new node
            // Find which old node previously owned this range
            oldOwner := s.hashRing.GetNodeForHash(vnode.Hash)

            if oldOwner != "" && oldOwner != newNode.NodeID {
                // Keys in range [prevHash, vnode.Hash) will move from oldOwner to newNode
                prevHash := tempRing.GetPreviousHash(vnode.Hash)

                keyRange := &KeyRange{
                    StartHash: prevHash,
                    EndHash:   vnode.Hash,
                }

                affectedRanges[oldOwner] = append(affectedRanges[oldOwner], keyRange)
            }
        }
    }

    return affectedRanges, nil
}
```

#### Step 3: Storage Nodes Start Streaming

**On Old Storage Node**: Receives notification to stream

```go
// storage-node/internal/handler/streaming_handler.go
func (h *StreamingHandler) StartStreaming(ctx context.Context, req *pb.StartStreamingRequest) (*pb.StartStreamingResponse, error) {
    // req contains:
    // - TargetNodeID: "node-4"
    // - TargetHost: "10.0.0.4"
    // - TargetPort: 9090
    // - KeyRanges: [{StartHash: 1000, EndHash: 2000}, ...]

    h.logger.Info("Received streaming request",
        zap.String("target_node", req.TargetNodeId),
        zap.Int("key_ranges", len(req.KeyRanges)))

    // Create streaming context
    streamCtx := &StreamContext{
        TargetNodeID: req.TargetNodeId,
        TargetHost:   req.TargetHost,
        TargetPort:   int(req.TargetPort),
        KeyRanges:    convertKeyRanges(req.KeyRanges),
        State:        StreamStateCopying,
    }

    h.streamingManager.AddStream(req.TargetNodeId, streamCtx)

    // Start streaming in background
    go h.streamingManager.ExecuteStreaming(ctx, streamCtx)

    return &pb.StartStreamingResponse{
        Success: true,
        Message: "Streaming started",
    }, nil
}
```

#### Step 4: Two-Phase Streaming

**Phase A: Bulk Copy (Historical Data)**

```go
func (sm *StreamingManager) ExecuteStreaming(ctx context.Context, streamCtx *StreamContext) error {
    // Phase A: Copy existing data
    sm.logger.Info("Starting bulk copy phase",
        zap.String("target", streamCtx.TargetNodeID))

    for _, keyRange := range streamCtx.KeyRanges {
        // Scan local storage for keys in this range
        keys := sm.localNode.ScanKeysInRange(keyRange.StartHash, keyRange.EndHash)

        for _, key := range keys {
            value, vectorClock := sm.localNode.Get(key)

            // Send to new node
            err := sm.sendToTargetNode(streamCtx, key, value, vectorClock, false /* isLiveWrite */)
            if err != nil {
                sm.logger.Error("Failed to copy key",
                    zap.String("key", key),
                    zap.Error(err))
                continue
            }

            atomic.AddInt64(&streamCtx.KeysCopied, 1)
        }
    }

    // Phase A complete - switch to live streaming
    streamCtx.mu.Lock()
    streamCtx.State = StreamStateStreaming
    streamCtx.mu.Unlock()

    sm.logger.Info("Bulk copy complete, switching to live streaming",
        zap.String("target", streamCtx.TargetNodeID),
        zap.Int64("keys_copied", streamCtx.KeysCopied))

    return nil
}
```

**Phase B: Live Streaming (New Writes)**

```go
// Intercept writes on old storage node
func (sn *StorageNode) Write(ctx context.Context, key string, value []byte, vectorClock VectorClock) error {
    // Normal write to local storage
    if err := sn.storage.Put(key, value, vectorClock); err != nil {
        return err
    }

    // Check if we're streaming this key to any node
    keyHash := sn.hashKey(key)

    sn.streamingManager.mu.RLock()
    defer sn.streamingManager.mu.RUnlock()

    for _, streamCtx := range sn.streamingManager.activeStreams {
        // Check if this key falls in the ranges we're streaming
        if streamCtx.State == StreamStateStreaming && sn.keyInRanges(keyHash, streamCtx.KeyRanges) {
            // Stream this write to the target node (async, best effort)
            go func(ctx *StreamContext) {
                err := sn.streamingManager.sendToTargetNode(ctx, key, value, vectorClock, true /* isLiveWrite */)
                if err != nil {
                    sn.logger.Debug("Failed to stream live write",
                        zap.String("key", key),
                        zap.String("target", ctx.TargetNodeID),
                        zap.Error(err))
                }

                atomic.AddInt64(&ctx.KeysStreamed, 1)
            }(streamCtx)
        }
    }

    return nil
}
```

#### Step 5: Sync Verification

```go
func (sm *StreamingManager) VerifySync(ctx context.Context, streamCtx *StreamContext) error {
    sm.logger.Info("Starting sync verification",
        zap.String("target", streamCtx.TargetNodeID))

    streamCtx.mu.Lock()
    streamCtx.State = StreamStateSyncing
    streamCtx.mu.Unlock()

    // Compare hashes/checksums of key ranges
    for _, keyRange := range streamCtx.KeyRanges {
        localChecksum := sm.localNode.ComputeChecksum(keyRange)
        remoteChecksum := sm.getRemoteChecksum(streamCtx, keyRange)

        if localChecksum != remoteChecksum {
            sm.logger.Warn("Checksum mismatch, re-syncing",
                zap.String("key_range", fmt.Sprintf("%d-%d", keyRange.StartHash, keyRange.EndHash)))

            // Re-sync this range
            if err := sm.reSyncRange(ctx, streamCtx, keyRange); err != nil {
                return fmt.Errorf("re-sync failed: %w", err)
            }
        }
    }

    sm.logger.Info("Sync verification complete",
        zap.String("target", streamCtx.TargetNodeID))

    // Notify coordinator that we're ready for cutover
    return sm.notifyCoordinatorReadyForCutover(streamCtx)
}
```

#### Step 6: Cutover (Coordinator)

```go
// Coordinator receives notification that streaming is complete and synced
func (h *NodeHandler) PerformCutover(ctx context.Context, nodeID string) error {
    h.logger.Info("Performing cutover",
        zap.String("node_id", nodeID))

    // 1. Update node status to "active"
    if err := h.metadataStore.UpdateStorageNodeStatus(ctx, nodeID, string(model.NodeStatusActive)); err != nil {
        return fmt.Errorf("failed to update node status: %w", err)
    }

    // 2. Update hash ring atomically
    node, err := h.metadataStore.GetStorageNode(ctx, nodeID)
    if err != nil {
        return fmt.Errorf("failed to get node: %w", err)
    }

    if err := h.routingService.AddNode(ctx, node); err != nil {
        return fmt.Errorf("failed to add node to hash ring: %w", err)
    }

    // 3. Notify old nodes to stop streaming
    affectedNodes := h.getNodesStreamingTo(ctx, nodeID)
    for _, oldNode := range affectedNodes {
        if err := h.notifyNodeToStopStreaming(ctx, oldNode.NodeID, nodeID); err != nil {
            h.logger.Error("Failed to stop streaming",
                zap.String("old_node", oldNode.NodeID),
                zap.Error(err))
        }
    }

    h.logger.Info("Cutover complete, node is now active",
        zap.String("node_id", nodeID))

    return nil
}
```

---

## Node Removal Flow (Mirror Strategy)

### Step 1: Mark Node as "draining"

```go
func (h *NodeHandler) RemoveStorageNode(ctx context.Context, req *pb.RemoveStorageNodeRequest) (*pb.RemoveStorageNodeResponse, error) {
    // 1. Update node status to "draining"
    if err := h.metadataStore.UpdateStorageNodeStatus(ctx, req.NodeId, string(model.NodeStatusDraining)); err != nil {
        return nil, err
    }

    // 2. Calculate which nodes will inherit this node's data
    node, _ := h.metadataStore.GetStorageNode(ctx, req.NodeId)
    inheritingNodes, keyRanges := h.routingService.CalculateInheritingNodes(ctx, node)

    // 3. Notify draining node to start streaming to inheriting nodes
    for targetNode, ranges := range inheritingNodes {
        if err := h.notifyNodeToStartStreaming(ctx, node, targetNode, ranges); err != nil {
            h.logger.Error("Failed to notify streaming", zap.Error(err))
        }
    }

    return &pb.RemoveStorageNodeResponse{
        Success: true,
        NodeId:  req.NodeId,
        Message: "Node draining started, data streaming to inheriting nodes",
        Status:  string(model.NodeStatusDraining),
    }, nil
}
```

### Step 2: Draining Node Streams While Serving

**Critical**: Node continues serving reads/writes while streaming

```go
func (sn *StorageNode) Write(ctx context.Context, key string, value []byte, vectorClock VectorClock) error {
    // Check if we're draining
    if sn.status == NodeStatusDraining {
        // Still accept writes, but also stream to inheriting nodes
        if err := sn.storage.Put(key, value, vectorClock); err != nil {
            return err
        }

        // Stream to inheriting nodes
        keyHash := sn.hashKey(key)
        for _, streamCtx := range sn.streamingManager.activeStreams {
            if sn.keyInRanges(keyHash, streamCtx.KeyRanges) {
                go sn.streamingManager.sendToTargetNode(streamCtx, key, value, vectorClock, true)
            }
        }

        return nil
    }

    // Normal write
    return sn.storage.Put(key, value, vectorClock)
}
```

### Step 3: Cutover (Remove from Hash Ring)

```go
func (h *NodeHandler) PerformRemovalCutover(ctx context.Context, nodeID string) error {
    // 1. Remove from hash ring atomically
    if err := h.routingService.RemoveNode(ctx, nodeID); err != nil {
        return fmt.Errorf("failed to remove from hash ring: %w", err)
    }

    // 2. New writes now go to inheriting nodes (automatic via hash ring)

    // 3. Mark node as "inactive" (can be shut down)
    if err := h.metadataStore.UpdateStorageNodeStatus(ctx, nodeID, string(model.NodeStatusInactive)); err != nil {
        return fmt.Errorf("failed to mark inactive: %w", err)
    }

    return nil
}
```

---

## Comparison: Current vs Recommended

| Aspect | Current (Coordinator-Based) | Recommended (Storage-Based) |
|--------|----------------------------|----------------------------|
| **Dual-write logic** | Coordinator checks every write | Storage nodes handle transparently |
| **Hash ring update** | Updated during migration | Updated only at cutover (atomic) |
| **Read consistency** | May read stale (new node not queried) | Always consistent (ring dictates routing) |
| **Write coordination** | Manual quorum calculation | Automatic (stream follows write) |
| **Data copy** | Separate from live writes | Integrated (copy → stream) |
| **Race conditions** | Multiple (cutover, cleanup) | None (atomic cutover) |
| **Node removal** | Not implemented | Same pattern (symmetric) |
| **Complexity** | High (coordinator state) | Low (nodes self-manage) |

---

## Benefits of Recommended Approach

### 1. **Zero Coordinator Overhead**

**Current**:
```go
// Every write checks migration state
if s.migrationService.IsDualWriteActive() {
    additionalNodes := s.migrationService.GetDualWriteNodes(...)
    // More logic...
}
```

**Recommended**:
```go
// Coordinator just routes based on hash ring
replicas := s.routingService.GetReplicas(tenantID, key, replicationFactor)
// No migration checks!
```

### 2. **Automatic Consistency**

- Hash ring determines routing (single source of truth)
- No separate "dual-write" vs "regular replica" logic
- Reads always query correct nodes (no stale data)

### 3. **Symmetric Add/Remove**

- Same streaming pattern for both operations
- Draining node continues serving (no downtime)
- Clean abstraction

### 4. **Better Failure Handling**

**If streaming fails**:
- Old node still has data ✅
- Reads still work ✅
- Retry streaming ✅

**If cutover fails**:
- Hash ring not updated ✅
- Writes still go to old nodes ✅
- Rollback is easy ✅

---

## Implementation Checklist

### Phase 1: Storage Node Changes (Week 1-2)

- [ ] Add `StreamingManager` to storage nodes
- [ ] Implement bulk copy phase
- [ ] Implement live streaming (intercept writes)
- [ ] Add sync verification (checksums)
- [ ] Implement streaming APIs (gRPC)

### Phase 2: Coordinator Changes (Week 2-3)

- [ ] Add `CalculateKeyRanges()` to routing service
- [ ] Implement `notifyNodeToStartStreaming()`
- [ ] Add cutover API endpoint
- [ ] Update node status workflow (provisioning → syncing → active)
- [ ] Add monitoring and metrics

### Phase 3: Node Removal (Week 3-4)

- [ ] Implement draining status
- [ ] Calculate inheriting nodes
- [ ] Stream while serving (draining node)
- [ ] Cutover for removal

### Phase 4: Testing (Week 4-5)

- [ ] Unit tests for streaming logic
- [ ] Integration tests (full migration)
- [ ] Failure injection tests
- [ ] Performance benchmarks

---

## Migration Path from Current Implementation

### Option A: Run Both (Safe)

1. Keep current coordinator-based dual-write
2. Implement storage-based streaming
3. Run both in parallel (validation)
4. Switch over once validated
5. Remove coordinator dual-write logic

### Option B: Direct Migration (Fast)

1. Implement storage-based streaming
2. Feature flag to switch between modes
3. Test in staging
4. Deploy to production
5. Remove old code

**Recommendation**: Option A for production safety

---

## Conclusion

Your proposed strategy is **significantly better** than the current implementation:

✅ Eliminates coordinator overhead
✅ Solves all consistency issues
✅ Symmetric for add/remove
✅ Industry-proven pattern
✅ Simpler to reason about

**Recommendation**: Implement this approach as the primary migration strategy. The current coordinator-based dual-write should be deprecated.

**Estimated Effort**: 4-5 weeks for full implementation and testing

**Risk Level**: Medium (significant changes to storage nodes, but isolated)

**Value**: High (solves critical consistency issues, reduces complexity, enables node removal)
