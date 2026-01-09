# Issues Found and Recommended Fixes

## Summary of Analysis

After detailed review of the dual-write implementation for node addition, here's what we found:

| Issue | Status | Severity | Impact |
|-------|--------|----------|--------|
| Explicit dual-write phase | ✅ **IMPLEMENTED** | N/A | Working correctly |
| Migration state checking during routing | ⚠️ **PARTIAL** | 🟡 Medium | Reads may miss new node data |
| Separate quorum for old vs new sets | ❌ **MISSING** | 🔴 Critical | Potential data loss |
| Race condition prevention | ⚠️ **BASIC** | 🟠 Medium-High | Edge case inconsistencies |

---

## Issue #1: Reads Don't Check Migration State ⚠️

### Current Behavior

```
Migration Active:
  - New node D has data from dual-write
  - Hash ring: [A, B, C] (old)
  - GetReplicas() returns [A, B, C]
  - Read queries only A, B, C
  - ❌ Node D not queried even though it has the data
```

### The Code

**Current** ([coordinator_service.go:202-228](../internal/service/coordinator_service.go#L202-L228)):
```go
func (s *CoordinatorService) ReadKeyValue(ctx context.Context, tenantID, key string, consistency string) (*ReadResult, error) {
    // Get replicas from hash ring
    replicas, err := s.routingService.GetReplicas(ctx, tenantID, key, tenant.ReplicationFactor)

    // 🔴 MISSING: No check for active migrations
    // 🔴 MISSING: No inclusion of new nodes with dual-written data

    return s.readFromReplicas(ctx, replicas, tenantID, key, consistency)
}
```

### Recommended Fix

```go
func (s *CoordinatorService) ReadKeyValue(ctx context.Context, tenantID, key string, consistency string) (*ReadResult, error) {
    // Get regular replicas
    replicas, err := s.routingService.GetReplicas(ctx, tenantID, key, tenant.ReplicationFactor)
    if err != nil {
        return nil, err
    }

    // ✅ NEW: Include migration nodes in read path
    if s.migrationService != nil && s.migrationService.IsDualWriteActive() {
        migrationNodes, err := s.migrationService.GetDualWriteNodes(ctx, tenantID, key, replicas)
        if err != nil {
            s.logger.Warn("Failed to get migration nodes for read",
                zap.String("tenant_id", tenantID),
                zap.String("key", key),
                zap.Error(err))
        } else if len(migrationNodes) > 0 {
            s.logger.Debug("Including migration nodes in read",
                zap.String("tenant_id", tenantID),
                zap.String("key", key),
                zap.Int("migration_nodes", len(migrationNodes)))

            // Add migration nodes to replica set
            replicas = append(replicas, migrationNodes...)
        }
    }

    return s.readFromReplicas(ctx, replicas, tenantID, key, consistency)
}
```

### Impact
- **Before**: Reads during migration may return stale data
- **After**: Reads include all nodes with data (old + new)
- **Benefit**: Read-your-writes consistency during migration

---

## Issue #2: No Quorum Overlap During Cutover 🔴

### The Critical Problem

During cutover, the hash ring changes **instantly**, causing replica sets to change:

```
Before Cutover:
  Key "user:123" → Hash → Replicas [A, B, C]

Cutover Happens (hash ring updated):
  Key "user:123" → Hash → Replicas [B, C, D]

Problem:
  - Write succeeded on A, B (quorum 2/3 on old set)
  - Dual-write to D failed
  - After cutover, reads query B, C, D
  - If B or C fails, quorum cannot be reached (only 1/3 have data)
```

### Failure Scenario

```
Timeline:

t0: Write "user:123" = "value1"
    ├─ Regular replicas: A✅, B✅, C❌ (quorum 2/3 reached ✅)
    └─ Dual-write: D❌ (fails but ignored)
    Result: Write succeeds

t1: Cutover phase
    └─ Hash ring updated: key now maps to [B, C, D]

t2: Read "user:123"
    ├─ Query: B✅, C✅, D❌ (quorum 2/3)
    └─ Returns "value1" ✅

t3: Node B crashes

t4: Read "user:123"
    ├─ Query: B❌, C✅, D❌ (only 1/3 respond)
    └─ Quorum NOT reached 🔴 DATA UNAVAILABLE
```

### Current Code

**Current** ([coordinator_service.go:342-350](../internal/service/coordinator_service.go#L342-L350)):
```go
// Check if quorum is reached (ONLY on regular replicas)
if !s.consistencyService.IsQuorumReached(successCount, len(replicas), consistency) {
    return &WriteResult{
        Success:      false,
        ErrorMessage: fmt.Sprintf("quorum not reached: %d/%d", successCount, requiredReplicas),
    }, fmt.Errorf("quorum not reached: %d/%d", successCount, requiredReplicas)
}

// ❌ No check for quorum on new replica set during cutover
```

### Recommended Fix: Dual Quorum Strategy

```go
// Add to MigrationContext
type MigrationContext struct {
    Migration       *model.Migration
    NewNode         *model.StorageNode
    OldReplicas     []*model.StorageNode  // ✅ NEW: Track old replica set
    Phase           model.MigrationPhase
    DualWriteActive bool
    RequireDualQuorum bool                // ✅ NEW: Flag for cutover phase
    mu              sync.RWMutex
}

// Update writeToReplicas
func (s *CoordinatorService) writeToReplicas(
    ctx context.Context,
    replicas []*model.StorageNode,
    tenantID, key string,
    value []byte,
    vectorClock model.VectorClock,
    consistency string,
    additionalNodes []*model.StorageNode,
) (*WriteResult, error) {
    // ... existing write logic ...

    // ✅ NEW: Check if we need dual quorum (during cutover)
    requireDualQuorum := false
    var oldReplicaIDs map[string]bool

    if s.migrationService != nil {
        if mctx := s.migrationService.GetActiveMigrationForKey(ctx, tenantID, key); mctx != nil {
            mctx.mu.RLock()
            requireDualQuorum = mctx.RequireDualQuorum
            if requireDualQuorum && len(mctx.OldReplicas) > 0 {
                oldReplicaIDs = make(map[string]bool)
                for _, node := range mctx.OldReplicas {
                    oldReplicaIDs[node.NodeID] = true
                }
            }
            mctx.mu.RUnlock()
        }
    }

    // Count successes on old vs new replica sets
    oldSetSuccess := 0
    newSetSuccess := 0

    for _, resp := range responses {
        if resp.Success {
            if oldReplicaIDs != nil && oldReplicaIDs[resp.NodeID] {
                oldSetSuccess++
            } else {
                newSetSuccess++
            }
        }
    }

    // ✅ NEW: During cutover, require quorum on BOTH sets
    if requireDualQuorum {
        oldQuorum := s.consistencyService.IsQuorumReached(oldSetSuccess, len(oldReplicaIDs), consistency)
        newQuorum := s.consistencyService.IsQuorumReached(newSetSuccess, len(replicas), consistency)

        if !oldQuorum || !newQuorum {
            return &WriteResult{
                Success: false,
                ErrorMessage: fmt.Sprintf("dual quorum not reached: old=%d/%d, new=%d/%d",
                    oldSetSuccess, len(oldReplicaIDs), newSetSuccess, len(replicas)),
            }, fmt.Errorf("dual quorum not reached during cutover")
        }

        s.logger.Info("Dual quorum reached during cutover",
            zap.Int("old_set_success", oldSetSuccess),
            zap.Int("new_set_success", newSetSuccess))
    } else {
        // Normal quorum check
        if !s.consistencyService.IsQuorumReached(successCount, len(replicas), consistency) {
            return nil, fmt.Errorf("quorum not reached: %d/%d", successCount, len(replicas))
        }
    }

    return &WriteResult{
        Success:      true,
        ReplicaCount: int32(successCount),
    }, nil
}
```

### Update Migration Phases

```go
// Update executeCutoverPhase
func (s *MigrationService) executeCutoverPhase(ctx context.Context, mctx *MigrationContext) error {
    // ✅ NEW: Capture old replica set before cutover
    oldReplicas, err := s.routingService.GetReplicasForAllKeys(ctx)
    if err != nil {
        return fmt.Errorf("failed to capture old replicas: %w", err)
    }

    mctx.mu.Lock()
    mctx.OldReplicas = oldReplicas
    mctx.RequireDualQuorum = true  // Enable dual quorum requirement
    mctx.mu.Unlock()

    // Change node status and update hash ring
    if err := s.metadataStore.UpdateStorageNodeStatus(ctx, mctx.NewNode.NodeID, string(model.NodeStatusActive)); err != nil {
        return fmt.Errorf("failed to activate node: %w", err)
    }

    mctx.NewNode.Status = model.NodeStatusActive
    if err := s.routingService.AddNode(ctx, mctx.NewNode); err != nil {
        return fmt.Errorf("failed to add node to routing service: %w", err)
    }

    // ✅ NEW: Grace period for in-flight operations
    s.logger.Info("Cutover completed, entering grace period",
        zap.String("migration_id", mctx.Migration.MigrationID),
        zap.Duration("grace_period", 5*time.Second))

    time.Sleep(5 * time.Second)  // Allow in-flight writes to complete with dual quorum

    // ✅ NEW: Disable dual quorum after grace period
    mctx.mu.Lock()
    mctx.RequireDualQuorum = false
    mctx.mu.Unlock()

    s.logger.Info("Grace period completed, dual quorum disabled",
        zap.String("migration_id", mctx.Migration.MigrationID))

    return nil
}
```

### Impact
- **Before**: Write may succeed but become unavailable after cutover
- **After**: Write must succeed on both old AND new replica sets during transition
- **Tradeoff**: Higher write latency during cutover, but guarantees availability

---

## Issue #3: Data Copy Doesn't Coordinate with Live Writes ⚠️

### The Problem

```
Thread 1 (Data Copy):                  Thread 2 (Live Write):
1. Read key "user:123" from node A
   value = "v1"
                                       2. Write "user:123" = "v2" to nodes A,B,C,D
                                          (dual-write)
3. Write "v1" to new node D
   (overwrites "v2" with stale "v1")

Result: Node D has stale value "v1" instead of "v2"
```

### Recommended Fix: Version-Based Copy

```go
// Update executeDataCopyPhase
func (s *MigrationService) executeDataCopyPhase(ctx context.Context, mctx *MigrationContext) error {
    s.logger.Info("Starting data copy phase",
        zap.String("migration_id", mctx.Migration.MigrationID))

    mctx.mu.Lock()
    mctx.Phase = model.MigrationPhaseDataCopy
    mctx.mu.Unlock()

    // ✅ NEW: Get keys that should be on this node
    keyRanges := s.routingService.GetKeyRangesForNode(mctx.NewNode.NodeID)

    var copyCount, skipCount int64

    for _, keyRange := range keyRanges {
        // Get all keys in range from old replicas
        keys, err := s.scanKeysInRange(ctx, keyRange, mctx.OldNodes)
        if err != nil {
            return fmt.Errorf("failed to scan keys: %w", err)
        }

        for _, key := range keys {
            // ✅ NEW: Read with version/timestamp
            value, vectorClock, timestamp, err := s.readValueWithVersion(ctx, key, mctx.OldNodes)
            if err != nil {
                s.logger.Warn("Failed to read key for copy", zap.String("key", key), zap.Error(err))
                continue
            }

            // ✅ NEW: Write to new node with "copy" flag and version
            err = s.storageClient.WriteWithVersion(ctx, mctx.NewNode, key, value, vectorClock, timestamp, true /* isCopy */)
            if err != nil {
                s.logger.Warn("Failed to copy key", zap.String("key", key), zap.Error(err))
                continue
            }

            copyCount++
        }
    }

    s.logger.Info("Data copy phase completed",
        zap.String("migration_id", mctx.Migration.MigrationID),
        zap.Int64("keys_copied", copyCount),
        zap.Int64("keys_skipped", skipCount))

    return nil
}
```

### Storage Node Logic

```go
// On storage node: reject stale copies
func (s *StorageNode) Write(key, value string, vectorClock VectorClock, timestamp time.Time, isCopy bool) error {
    s.mu.Lock()
    defer s.mu.Unlock()

    existing := s.data[key]

    // ✅ NEW: If this is a copy, check if we already have newer data
    if isCopy && existing != nil {
        if existing.Timestamp.After(timestamp) {
            // We already have newer data from live write, reject stale copy
            s.logger.Debug("Rejecting stale copy",
                zap.String("key", key),
                zap.Time("copy_ts", timestamp),
                zap.Time("existing_ts", existing.Timestamp))
            return nil  // Not an error, just skip
        }
    }

    // Normal write logic
    s.data[key] = &Value{
        Data:        value,
        VectorClock: vectorClock,
        Timestamp:   timestamp,
    }

    return nil
}
```

### Impact
- **Before**: Copied data may overwrite newer live writes
- **After**: Storage nodes reject stale copies, keeping latest data
- **Benefit**: Consistency between copied and live-written data

---

## Issue #4: Race Condition During Phase Transitions ⚠️

### The Problem

```
Thread 1 (Migration):                Thread 2 (Write):
executeCutoverPhase()
  └─ routingService.AddNode()       GetReplicas()
     [updates hash ring]              └─ Returns NEW replicas [B,C,D]

executeCleanupPhase()                IsDualWriteActive()
  └─ DualWriteActive = false          └─ Returns FALSE

                                     GetDualWriteNodes()
                                       └─ Returns [] (empty)

                                     Writes only to [B,C,D]
                                     ❌ Node A missed the write!
```

### Recommended Fix: Two-Phase Cutover with Grace Period

```go
func (s *MigrationService) executeCutoverPhase(ctx context.Context, mctx *MigrationContext) error {
    s.logger.Info("Starting cutover phase with two-phase commit",
        zap.String("migration_id", mctx.Migration.MigrationID))

    // Phase 1: Add to hash ring but keep dual-write active
    mctx.mu.Lock()
    mctx.Phase = model.MigrationPhaseCutover
    // Note: DualWriteActive still TRUE
    mctx.mu.Unlock()

    // Activate node in metadata store
    if err := s.metadataStore.UpdateStorageNodeStatus(ctx, mctx.NewNode.NodeID, string(model.NodeStatusActive)); err != nil {
        return fmt.Errorf("failed to activate node: %w", err)
    }

    // Add to hash ring
    mctx.NewNode.Status = model.NodeStatusActive
    if err := s.routingService.AddNode(ctx, mctx.NewNode); err != nil {
        return fmt.Errorf("failed to add node to routing service: %w", err)
    }

    s.logger.Info("Node added to hash ring, entering grace period",
        zap.String("node_id", mctx.NewNode.NodeID),
        zap.Duration("duration", 5*time.Second))

    // ✅ CRITICAL: Grace period while dual-write is still active
    // This ensures all in-flight writes that started before cutover
    // will complete with dual-write semantics
    time.Sleep(5 * time.Second)

    s.logger.Info("Cutover phase completed",
        zap.String("migration_id", mctx.Migration.MigrationID))

    return nil
}

func (s *MigrationService) executeCleanupPhase(ctx context.Context, mctx *MigrationContext) error {
    s.logger.Info("Starting cleanup phase",
        zap.String("migration_id", mctx.Migration.MigrationID))

    mctx.mu.Lock()
    mctx.Phase = model.MigrationPhaseCleanup
    mctx.DualWriteActive = false  // NOW it's safe to disable
    mctx.mu.Unlock()

    s.logger.Info("Cleanup phase completed, dual-write disabled",
        zap.String("migration_id", mctx.Migration.MigrationID))

    return nil
}
```

### Impact
- **Before**: Write might miss old replicas if cleanup happens during write
- **After**: 5-second grace period ensures all in-flight writes complete
- **Tradeoff**: Migration takes 5 seconds longer, but guarantees consistency

---

## Priority Recommendations

### P0 (Critical - Must Fix Before Production)

1. ✅ **Implement Migration-Aware Reads** (Issue #1)
   - Add migration node check to ReadKeyValue()
   - Estimated effort: 30 minutes
   - Risk: Medium (read path change)

2. ✅ **Add Grace Period to Cutover** (Issue #4)
   - Add 5-second sleep between cutover and cleanup
   - Estimated effort: 5 minutes
   - Risk: Low (simple delay)

### P1 (High - Fix Soon)

3. ✅ **Implement Dual Quorum Strategy** (Issue #2)
   - Track old replica set during cutover
   - Require quorum on both old and new sets
   - Estimated effort: 2-3 hours
   - Risk: High (write path change, needs thorough testing)

### P2 (Medium - Fix in Next Sprint)

4. ✅ **Add Version-Based Data Copy** (Issue #3)
   - Include timestamps in write operations
   - Reject stale copies on storage nodes
   - Estimated effort: 4-6 hours
   - Risk: Medium (requires storage node changes)

---

## Testing Plan

### Unit Tests

```go
func TestMigrationAwareReads(t *testing.T) {
    // Setup: Migration active, new node has data
    // Test: Read should query both old and new nodes
    // Assert: New node data is included in read
}

func TestDualQuorumCutover(t *testing.T) {
    // Setup: Write during cutover with partial failure
    // Test: Write should fail if quorum not reached on both sets
    // Assert: Write rejected, data consistent
}

func TestGracePeriodRace(t *testing.T) {
    // Setup: Write starts just before cleanup
    // Test: Write should complete with dual-write semantics
    // Assert: All nodes (old + new) receive the write
}

func TestStaleDataCopyRejection(t *testing.T) {
    // Setup: Live write happens during data copy
    // Test: Copy should be rejected if stale
    // Assert: Storage node keeps newer live-written data
}
```

### Integration Tests

```go
func TestEndToEndMigration(t *testing.T) {
    // 1. Start cluster with 3 nodes
    // 2. Write 1000 keys
    // 3. Add 4th node (triggers migration)
    // 4. Continue writing during migration
    // 5. Verify all keys readable after migration
    // 6. Verify no data loss
}

func TestMigrationWithNodeFailure(t *testing.T) {
    // 1. Start migration
    // 2. Kill new node during dual-write phase
    // 3. Verify writes still succeed (quorum on old nodes)
    // 4. Recover node, complete migration
    // 5. Verify eventual consistency
}
```

---

## Summary

| Fix | Effort | Risk | Priority | Status |
|-----|--------|------|----------|--------|
| Migration-aware reads | 30 min | Medium | P0 | ⏳ Not started |
| Grace period in cutover | 5 min | Low | P0 | ⏳ Not started |
| Dual quorum strategy | 2-3 hrs | High | P1 | ⏳ Not started |
| Version-based data copy | 4-6 hrs | Medium | P2 | ⏳ Not started |

**Total Estimated Effort**: 7-10 hours for all fixes

**Recommended Approach**: Implement P0 fixes immediately, P1 within 1 week, P2 within 2 weeks.
