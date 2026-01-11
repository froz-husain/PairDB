# Storage Node: Operational Procedures

This document provides comprehensive operational procedures for managing and maintaining storage nodes in production.

## 1. Disk Space Management

### 1.1 Monitoring Procedures

**Key Metrics to Monitor**:
- `disk_usage_percent` - Current disk usage percentage
- `disk_available_bytes` - Available disk space in bytes
- `disk_throttle_events_total` - Count of throttling events
- `disk_circuit_breaker_events_total` - Count of circuit breaker engagements
- `disk_full_rejections_total` - Count of writes rejected due to full disk

**Alert Thresholds**:
- **Warning (80%)**: Low priority alert, plan capacity increase
- **Throttle (90%)**: Medium priority alert, initiate cleanup procedures
- **Circuit Breaker (95%)**: High priority alert, immediate action required

**Monitoring Dashboard**:
```
Disk Usage: [==================95%==================] CRITICAL
Available: 50 GB / 1 TB
Status: Circuit Breaker ENGAGED
Rejected Writes (last 5min): 1,234
```

### 1.2 Alert Response Procedures

#### Warning Level (80-90% Usage)

**Symptoms**:
- Disk usage warnings in logs
- No impact on write operations
- `disk_usage_percent` metric between 80-90

**Actions**:
1. **Assess Growth Rate**: Check disk usage trend over last 24h/7d
   ```bash
   # Query Prometheus for growth rate
   rate(disk_usage_percent[24h])
   ```

2. **Plan Capacity Expansion**: Estimate time until throttling
   ```
   Days until 90% = (90 - current_usage) / daily_growth_rate
   ```

3. **Optional Proactive Cleanup**:
   - Trigger manual compaction: `POST /api/v1/compaction/trigger`
   - Clean old commit log segments: Check and remove segments older than retention period
   - Verify backups before cleanup

4. **Schedule Capacity Increase**: Add disk space or scale horizontally

#### Throttle Level (90-95% Usage)

**Symptoms**:
- Large writes being rejected
- Clients receiving `DISK_THROTTLED` (gRPC: Unavailable) errors
- Small writes still succeeding
- Increased client retry rates

**Immediate Actions**:
1. **Verify Throttling State**:
   ```bash
   curl http://storage-node:8080/health | jq '.disk_status'
   # Expected: {"status": "throttled", "usage_percent": 92.5}
   ```

2. **Trigger Immediate Compaction**:
   ```bash
   curl -X POST http://storage-node:8080/api/v1/compaction/trigger-all-levels
   ```
   - This will merge SSTables and remove duplicates/tombstones
   - Monitor compaction progress: `compaction_running` metric

3. **Clean Commit Log Segments**:
   ```bash
   # Check which segments can be safely deleted
   curl http://storage-node:8080/api/v1/commitlog/cleanable-segments

   # Trigger cleanup
   curl -X POST http://storage-node:8080/api/v1/commitlog/cleanup
   ```

4. **Monitor Recovery**:
   - Watch `disk_usage_percent` metric
   - Expect 5-15% space recovery from compaction
   - Throttling automatically disengages when usage drops below 90%

5. **If No Improvement**:
   - Emergency capacity expansion required
   - Consider node replacement with larger disk
   - Initiate data migration to new nodes

#### Circuit Breaker Level (≥95% Usage)

**Symptoms**:
- ALL writes being rejected
- Clients receiving `DISK_FULL` (gRPC: ResourceExhausted) errors
- Coordinators routing writes to replica nodes
- Node marked as unhealthy for writes

**Critical Actions** (Execute in order):

1. **Immediate Triage**:
   ```bash
   # Check current status
   curl http://storage-node:8080/health

   # Check largest consumers
   du -sh /var/lib/pairdb/* | sort -h
   ```

2. **Emergency Compaction**:
   ```bash
   # Trigger aggressive compaction
   curl -X POST http://storage-node:8080/api/v1/compaction/emergency
   ```
   - Uses all available CPU for compaction
   - May impact read latency temporarily
   - Monitor: `compaction_bytes_reclaimed` metric

3. **Commit Log Emergency Cleanup**:
   ```bash
   # Force cleanup of all eligible segments
   curl -X POST http://storage-node:8080/api/v1/commitlog/force-cleanup
   ```
   - Only removes segments that have been flushed to SSTables
   - Verify via logs: "Commit log segment deleted"

4. **Temporary Cache Reduction**:
   ```bash
   # Reduce cache size to free memory (allows more disk operations)
   curl -X POST http://storage-node:8080/api/v1/cache/reduce \
     -d '{"target_size_mb": 512}'
   ```

5. **Monitor Recovery**:
   ```bash
   watch -n 5 'curl -s http://storage-node:8080/health | jq ".disk_status"'
   ```
   - Circuit breaker automatically disengages when usage drops below 95%
   - Log message: "Disk circuit breaker DISENGAGED"

6. **If Still Critical After 30 Minutes**:
   - **Option A**: Emergency node replacement
     - Add new storage node with larger disk
     - Stream data from current node to new node
     - Update coordinator routing tables
     - Decommission full node

   - **Option B**: Manual data cleanup (DANGEROUS)
     - Identify and delete oldest SSTable files (ONLY if replicated)
     - Verify data exists on replica nodes first
     ```bash
     # Find oldest SSTables
     find /var/lib/pairdb/sstables -name "*.sst" -mtime +30 | sort

     # Verify replication before deletion
     ./verify-replication.sh <tenant_id> <key_range>
     ```

### 1.3 Prevention Strategies

1. **Proactive Monitoring**:
   - Set up predictive alerts based on growth trends
   - Alert when projected to hit 90% within 7 days
   ```promql
   predict_linear(disk_usage_percent[7d], 7*24*3600) > 90
   ```

2. **Regular Compaction**:
   - Schedule compaction during off-peak hours
   - Monitor compaction effectiveness: space reclaimed per run

3. **Commit Log Rotation**:
   - Configure appropriate retention: `max_age: 7 days`
   - Verify flushing frequency is adequate
   - Monitor: `commitlog_segments_active` metric

4. **Capacity Planning**:
   - Maintain 20% free space buffer
   - Scale horizontally before reaching 80% on any node
   - Use consistent hashing for even data distribution

5. **Data Lifecycle Management**:
   - Implement TTL for ephemeral data
   - Archive old data to cold storage
   - Periodic tombstone cleanup during major compaction

### 1.4 Manual Cleanup Procedures

**Safe Cleanup Checklist**:
- [ ] Verify data is replicated (RF ≥ 2)
- [ ] Create backup of metadata
- [ ] Stop writes to node (drain mode)
- [ ] Perform cleanup operation
- [ ] Verify data integrity post-cleanup
- [ ] Re-enable writes
- [ ] Monitor for errors

**Cleanup Commands**:
```bash
# 1. Enter maintenance mode
curl -X POST http://storage-node:8080/api/v1/maintenance/enable

# 2. Compact all levels
curl -X POST http://storage-node:8080/api/v1/compaction/compact-all

# 3. Clean commit logs
curl -X POST http://storage-node:8080/api/v1/commitlog/cleanup

# 4. Verify disk space recovered
df -h /var/lib/pairdb

# 5. Exit maintenance mode
curl -X POST http://storage-node:8080/api/v1/maintenance/disable
```

---

## 2. Data Integrity Management

### 2.1 Monitoring Checksum Failures

**Key Metrics**:
- `checksum_failures_total` - Total checksum validation failures
- `checksum_validation_latency` - Time to validate checksums
- `corrupted_entries_total` - Count of corrupted entries detected
- `data_recovery_operations_total` - Recovery attempts

**Alert Thresholds**:
- **1-5 failures/day**: Low priority, monitor trend
- **>10 failures/hour**: Medium priority, investigate disk health
- **>100 failures/hour**: High priority, potential disk failure

### 2.2 Investigating Corruption

**Initial Triage**:
```bash
# Check recent checksum failures
curl http://storage-node:8080/api/v1/metrics | grep checksum_failures_total

# Get detailed error logs
kubectl logs storage-node-1 | grep "checksum validation failed"

# Output example:
# ERROR checksum validation failed file=/data/sstables/l0/sstable-12345.sst
#   offset=1048576 expected=0xABCD1234 actual=0xABCD9999
```

**Corruption Pattern Analysis**:

1. **Single File Corruption**:
   - Likely cause: Disk error, incomplete write
   - Action: Restore from replica
   ```bash
   # Identify corrupted SSTable
   SSTABLE_ID="sstable-12345"

   # Request recovery from replica
   curl -X POST http://storage-node:8080/api/v1/recovery/sstable \
     -d "{\"sstable_id\": \"$SSTABLE_ID\", \"source\": \"replica\"}"
   ```

2. **Multiple File Corruption**:
   - Likely cause: Disk hardware failure
   - Action: Replace node immediately
   ```bash
   # Mark node for replacement
   kubectl label node storage-node-1 status=failing

   # Initiate node replacement procedure (Section 3)
   ```

3. **Commit Log Corruption**:
   - Likely cause: Crash during write, power loss
   - Action: Recover from commit log and replicas
   ```bash
   # Recovery will skip corrupted entries
   # Data will be repaired from replicas via read repair or anti-entropy
   ```

### 2.3 Recovery Procedures

#### Automatic Recovery (Default)

The storage node automatically handles corruption:

1. **Detection**: CRC32 checksum mismatch on read
2. **Logging**: Error logged with full context
3. **Skip**: Corrupted entry skipped
4. **Metrics**: `checksum_failures_total` incremented
5. **Coordinator Fallback**: Coordinator reads from replica node

**No manual intervention needed** for isolated failures.

#### Manual Recovery (High Corruption Rate)

When checksum failures exceed threshold:

1. **Drain Node**:
   ```bash
   curl -X POST http://coordinator:8080/api/v1/nodes/storage-node-1/drain
   ```

2. **Run Integrity Check**:
   ```bash
   # Full scan of all SSTables
   curl -X POST http://storage-node:8080/api/v1/integrity/scan-all

   # Output: Report of corrupted files
   ```

3. **Restore from Replicas**:
   ```bash
   # For each corrupted file
   curl -X POST http://storage-node:8080/api/v1/recovery/restore-from-replica \
     -d '{"files": ["sstable-123.sst", "sstable-456.sst"]}'
   ```

4. **Verify Restoration**:
   ```bash
   # Re-run integrity check
   curl -X POST http://storage-node:8080/api/v1/integrity/scan-all

   # Expected: 0 corrupted files
   ```

5. **Re-enable Node**:
   ```bash
   curl -X POST http://coordinator:8080/api/v1/nodes/storage-node-1/undrain
   ```

### 2.4 Disk Health Monitoring

**S.M.A.R.T. Monitoring**:
```bash
# Check disk health
sudo smartctl -a /dev/sda

# Key metrics to monitor:
# - Reallocated_Sector_Ct (should be 0)
# - Current_Pending_Sector (should be 0)
# - Offline_Uncorrectable (should be 0)
# - UDMA_CRC_Error_Count (should be low)
```

**Preventive Actions**:
- Replace disks with reallocated sectors > 0
- Schedule proactive disk replacement every 3-5 years
- Use RAID for additional protection
- Monitor I/O errors: `iostat -x 1`

---

## 3. Node Bootstrapping (Phase 2)

### 3.1 Adding a New Storage Node

**Procedure** for adding a new storage node to the cluster:

#### Step 1: Provision New Node

```bash
# Deploy new storage node
kubectl apply -f storage-node-new.yaml

# Wait for node to be ready
kubectl wait --for=condition=Ready pod/storage-node-new --timeout=300s

# Verify health
curl http://storage-node-new:8080/health
# Expected: {"status": "healthy", "disk_usage_percent": 5}
```

#### Step 2: Register with Coordinator

```bash
# Register new node
curl -X POST http://coordinator:8080/api/v1/nodes/register \
  -d '{
    "node_id": "storage-node-new",
    "host": "storage-node-new.default.svc.cluster.local",
    "port": 8080,
    "capacity_gb": 1000
  }'

# Verify registration
curl http://coordinator:8080/api/v1/nodes | jq '.nodes[] | select(.node_id=="storage-node-new")'
```

#### Step 3: Initiate Data Streaming

```bash
# Coordinator computes key ranges to stream
curl -X POST http://coordinator:8080/api/v1/rebalance/start \
  -d '{
    "target_node": "storage-node-new",
    "source_nodes": ["storage-node-1", "storage-node-2"],
    "rebalance_strategy": "even_distribution"
  }'

# This triggers StartStreaming on source nodes
```

#### Step 4: Monitor Streaming Progress

```bash
# Check streaming status
watch -n 5 'curl -s http://coordinator:8080/api/v1/rebalance/status | jq .'

# Output:
# {
#   "target_node": "storage-node-new",
#   "state": "streaming",
#   "progress_percent": 45,
#   "keys_copied": 5000000,
#   "keys_streamed": 250000,
#   "bytes_transferred": 104857600,
#   "estimated_completion": "2025-01-09T14:30:00Z"
# }
```

**Streaming Phases**:
1. **Copying** (bulk data transfer): 60-80% of time
2. **Streaming** (live writes): 10-20% of time
3. **Syncing** (checksum verification): 10-20% of time

#### Step 5: Verify Data Integrity

```bash
# After streaming completes
curl -X POST http://storage-node-new:8080/api/v1/integrity/verify-ranges \
  -d '{"key_ranges": [...]}'  # Ranges received from streaming

# Expected output:
# {
#   "status": "success",
#   "ranges_verified": 10,
#   "checksum_mismatches": 0
# }
```

#### Step 6: Update Routing Tables

```bash
# Coordinator updates routing to include new node
curl -X POST http://coordinator:8080/api/v1/routing/update

# Verify new node receiving writes
curl http://storage-node-new:8080/api/v1/metrics | grep write_requests_total
```

### 3.2 Handling Streaming Failures

**Failure Scenarios**:

1. **Network Partition During Streaming**:
   - Streaming automatically pauses
   - Resumes when network recovers
   - No data loss (writes still go to existing replicas)

2. **Target Node Crash**:
   ```bash
   # Restart streaming after node recovery
   curl -X POST http://coordinator:8080/api/v1/rebalance/restart \
     -d '{"target_node": "storage-node-new"}'
   ```

3. **Checksum Mismatch During Sync Phase**:
   - Automatic re-sync of affected ranges
   - Check logs for: "Checksum mismatch, re-syncing range"
   - Monitor: `streaming_resync_operations_total` metric

4. **Source Node Failure**:
   ```bash
   # Switch to alternate source node
   curl -X POST http://coordinator:8080/api/v1/rebalance/change-source \
     -d '{
       "target_node": "storage-node-new",
       "old_source": "storage-node-1",
       "new_source": "storage-node-3"
     }'
   ```

### 3.3 Streaming Performance Tuning

**Configuration Parameters**:
```yaml
streaming:
  batch_size: 1000           # Keys per batch (tune based on key size)
  stream_buffer: 10000       # Buffered writes (tune based on write rate)
  checksum_workers: 4        # Parallel checksum verification
  max_concurrent_streams: 2  # Concurrent streams per node
```

**Optimization Guidelines**:
- Increase `batch_size` for small keys (< 1KB values)
- Decrease `batch_size` for large keys (> 1MB values)
- Increase `stream_buffer` during high write traffic
- Monitor: `streaming_throughput_mbps` metric
- Target: 100-500 MB/s per stream

### 3.4 Rollback Procedure

If streaming fails and needs to be aborted:

```bash
# 1. Stop streaming
curl -X POST http://coordinator:8080/api/v1/rebalance/abort \
  -d '{"target_node": "storage-node-new"}'

# 2. Clear partial data on new node
curl -X POST http://storage-node-new:8080/api/v1/data/clear-all

# 3. Decommission node
kubectl delete pod storage-node-new

# 4. Remove from coordinator
curl -X DELETE http://coordinator:8080/api/v1/nodes/storage-node-new
```

---

## 4. Error Response Handling

### 4.1 Client-Side Error Handling Strategies

#### Client Errors (1xxx)

**INVALID_ARGUMENT (1000)**: Bad Request
```python
try:
    response = storage_node.write(tenant_id, key, value, vector_clock)
except grpc.RpcError as e:
    if e.code() == grpc.StatusCode.INVALID_ARGUMENT:
        # Fix input and retry immediately
        error_details = parse_error_details(e)
        if error_details.code == 1002:  # KEY_TOO_LARGE
            key = key[:1024]  # Truncate key
            response = storage_node.write(tenant_id, key, value, vector_clock)
```

**KEY_NOT_FOUND (1001)**: Not Found
```python
try:
    response = storage_node.read(tenant_id, key)
except grpc.RpcError as e:
    if e.code() == grpc.StatusCode.NOT_FOUND:
        # Key doesn't exist - this is expected, handle accordingly
        return None  # or raise application-specific exception
```

#### Server Errors (2xxx)

**DISK_THROTTLED (2003)**: Temporarily Throttled
```python
MAX_RETRIES = 3
BASE_DELAY = 0.5  # seconds

for attempt in range(MAX_RETRIES):
    try:
        response = storage_node.write(tenant_id, key, value, vector_clock)
        break
    except grpc.RpcError as e:
        if e.code() == grpc.StatusCode.UNAVAILABLE:
            error_details = parse_error_details(e)
            if error_details.code == 2003:  # DISK_THROTTLED
                if attempt < MAX_RETRIES - 1:
                    # Exponential backoff
                    delay = BASE_DELAY * (2 ** attempt)
                    time.sleep(delay)
                    continue
                else:
                    # Route to replica
                    replica_node = coordinator.get_replica(tenant_id, key)
                    response = replica_node.write(tenant_id, key, value, vector_clock)
```

**DISK_FULL (2002)**: Disk Full
```python
try:
    response = storage_node.write(tenant_id, key, value, vector_clock)
except grpc.RpcError as e:
    if e.code() == grpc.StatusCode.RESOURCE_EXHAUSTED:
        error_details = parse_error_details(e)
        if error_details.code == 2002:  # DISK_FULL
            # Immediately route to replica, no retry
            replica_node = coordinator.get_replica(tenant_id, key)
            response = replica_node.write(tenant_id, key, value, vector_clock)

            # Alert operations team
            alert("Disk full on storage-node-1", severity="high")
```

**CHECKSUM_FAILED (1006)**: Data Corruption
```python
try:
    response = storage_node.read(tenant_id, key)
except grpc.RpcError as e:
    if e.code() == grpc.StatusCode.DATA_LOSS:
        error_details = parse_error_details(e)
        if error_details.code == 1006:  # CHECKSUM_FAILED
            # Read from replica
            replica_node = coordinator.get_replica(tenant_id, key)
            response = replica_node.read(tenant_id, key)

            # Optionally trigger repair
            storage_node.repair(tenant_id, key, response.value, response.vector_clock)

            # Alert for data integrity issue
            alert("Checksum failure on storage-node-1", severity="medium")
```

### 4.2 Retry Logic Patterns

**Exponential Backoff with Jitter**:
```python
import random

def exponential_backoff_with_jitter(attempt, base_delay=0.5, max_delay=30):
    """
    Calculate delay with exponential backoff and jitter
    """
    delay = min(base_delay * (2 ** attempt), max_delay)
    jitter = random.uniform(0, delay * 0.1)  # 10% jitter
    return delay + jitter

# Usage
for attempt in range(MAX_RETRIES):
    try:
        response = storage_node.write(...)
        break
    except grpc.RpcError as e:
        if is_retryable(e):
            delay = exponential_backoff_with_jitter(attempt)
            time.sleep(delay)
```

**Circuit Breaker Pattern**:
```python
class CircuitBreaker:
    def __init__(self, failure_threshold=5, timeout=60):
        self.failure_count = 0
        self.failure_threshold = failure_threshold
        self.timeout = timeout
        self.last_failure_time = None
        self.state = "CLOSED"  # CLOSED, OPEN, HALF_OPEN

    def call(self, func, *args, **kwargs):
        if self.state == "OPEN":
            if time.time() - self.last_failure_time > self.timeout:
                self.state = "HALF_OPEN"
            else:
                raise CircuitBreakerOpen("Circuit breaker is OPEN")

        try:
            result = func(*args, **kwargs)
            if self.state == "HALF_OPEN":
                self.state = "CLOSED"
                self.failure_count = 0
            return result
        except Exception as e:
            self.failure_count += 1
            self.last_failure_time = time.time()
            if self.failure_count >= self.failure_threshold:
                self.state = "OPEN"
            raise

# Usage
circuit_breaker = CircuitBreaker()
response = circuit_breaker.call(storage_node.write, tenant_id, key, value, vc)
```

### 4.3 Coordinator Circuit Breaker Coordination

**Coordinator-Level Circuit Breaking**:
```python
class CoordinatorCircuitBreaker:
    """
    Coordinator maintains circuit breakers for each storage node
    """
    def __init__(self):
        self.node_breakers = {}  # node_id -> CircuitBreaker

    def get_available_nodes(self, tenant_id, key):
        """
        Returns list of available storage nodes for a key
        Excludes nodes with open circuit breakers
        """
        all_nodes = self.routing_table.get_replicas(tenant_id, key)
        available = [
            node for node in all_nodes
            if self.node_breakers[node.id].state != "OPEN"
        ]
        return available

    def write(self, tenant_id, key, value, vector_clock):
        """
        Write with automatic failover using circuit breakers
        """
        nodes = self.get_available_nodes(tenant_id, key)

        for node in nodes:
            try:
                breaker = self.node_breakers[node.id]
                response = breaker.call(node.write, tenant_id, key, value, vector_clock)
                return response
            except CircuitBreakerOpen:
                continue  # Try next node
            except Exception as e:
                continue  # Try next node

        raise AllNodesUnavailable("All replica nodes unavailable")
```

### 4.4 Graceful Degradation

**Read Path Degradation**:
```python
def read_with_degradation(tenant_id, key):
    """
    Read with graceful degradation:
    1. Try quorum read (R=2)
    2. Fall back to single read (R=1)
    3. Return stale data from cache if available
    """
    try:
        # Attempt quorum read
        return coordinator.read_quorum(tenant_id, key, R=2)
    except QuorumNotMet:
        try:
            # Fall back to single read
            return coordinator.read(tenant_id, key, R=1)
        except AllNodesUnavailable:
            # Return stale cache data with warning
            cached = cache.get(tenant_id, key)
            if cached:
                return CachedResponse(
                    value=cached.value,
                    stale=True,
                    cached_at=cached.timestamp
                )
            raise KeyNotFound()
```

**Write Path Degradation**:
```python
def write_with_degradation(tenant_id, key, value, vector_clock):
    """
    Write with graceful degradation:
    1. Try quorum write (W=2)
    2. Fall back to single write (W=1) with async repair
    3. Queue for later if all nodes unavailable
    """
    try:
        # Attempt quorum write
        return coordinator.write_quorum(tenant_id, key, value, vector_clock, W=2)
    except QuorumNotMet:
        try:
            # Fall back to single write
            response = coordinator.write(tenant_id, key, value, vector_clock, W=1)

            # Schedule async repair to other replicas
            async_repair_queue.enqueue({
                'tenant_id': tenant_id,
                'key': key,
                'value': value,
                'vector_clock': response.vector_clock
            })

            return response
        except AllNodesUnavailable:
            # Queue write for later
            write_queue.enqueue({
                'tenant_id': tenant_id,
                'key': key,
                'value': value,
                'vector_clock': vector_clock,
                'timestamp': time.time()
            })
            raise WriteQueued("Write queued for later processing")
```

---

## 5. Performance Tuning

### 5.1 Disk Manager Threshold Tuning

**Default Thresholds**:
- Warning: 80%
- Throttle: 90%
- Circuit Breaker: 95%

**Tuning Considerations**:

**Conservative (High Durability)**:
```yaml
disk:
  warning_threshold: 70.0
  throttle_threshold: 80.0
  circuit_breaker_threshold: 85.0
```
- Use when: High write traffic, limited monitoring
- Effect: Earlier warnings, more buffer for cleanup
- Trade-off: May throttle unnecessarily

**Aggressive (High Utilization)**:
```yaml
disk:
  warning_threshold: 85.0
  throttle_threshold: 93.0
  circuit_breaker_threshold: 97.0
```
- Use when: Excellent monitoring, predictable traffic
- Effect: Better disk utilization
- Trade-off: Less buffer for unexpected spikes

**Monitoring-Based Tuning**:
```python
# Analyze historical data
df = load_metrics("disk_throttle_events_total", days=30)

throttle_events = df.sum()
if throttle_events > 100:
    # Too many throttle events - lower threshold
    new_throttle_threshold = 85.0
elif throttle_events == 0:
    # Never throttled - can increase threshold
    new_throttle_threshold = 93.0
```

### 5.2 Worker Pool Sizing

**Default Configuration**:
```yaml
worker_pools:
  flush_pool:
    max_workers: 4
    queue_size: 100
  compaction_pool:
    max_workers: 2
    queue_size: 50
  streaming_pool:
    max_workers: 4
    queue_size: 100
```

**CPU-Based Sizing**:
```python
import multiprocessing

cpu_count = multiprocessing.cpu_count()

# Flush pool: CPU-bound (compression, serialization)
flush_workers = max(2, cpu_count // 2)

# Compaction pool: I/O and CPU-bound
compaction_workers = max(2, cpu_count // 4)

# Streaming pool: Network and I/O-bound
streaming_workers = max(2, cpu_count // 2)
```

**Queue Size Tuning**:
```yaml
# Monitor queue utilization
worker_pool_queue_utilization_percent

# If frequently at 100%, increase queue size
# If rarely above 10%, decrease queue size

# Example adjustment:
worker_pools:
  flush_pool:
    max_workers: 4
    queue_size: 200  # Increased from 100
```

**Performance Impact**:
- Too few workers: Tasks queue up, increased latency
- Too many workers: Context switching overhead, memory pressure
- Too small queue: Task rejections, write failures
- Too large queue: Memory overhead, slow shutdown

### 5.3 Validation Optimization

**Selective Validation**:
```yaml
validation:
  enable_security_checks: true    # Null bytes, control characters
  enable_size_checks: true        # Size limits
  enable_format_checks: true      # Tenant ID format
  enable_vector_clock_checks: true # Vector clock limits
```

**Trusted Client Optimization**:
```yaml
# For trusted internal clients
validation:
  trusted_clients:
    - "internal-service-1"
    - "internal-service-2"
  skip_validation_for_trusted: true  # Skip some checks for trusted clients
```

**Validation Performance Metrics**:
```promql
# Validation latency
histogram_quantile(0.99, rate(validation_duration_seconds[5m]))

# Validation rejection rate
rate(validation_rejections_total[5m]) / rate(write_requests_total[5m])

# Target: < 1ms p99 latency, < 1% rejection rate
```

### 5.4 Streaming Batch Size Tuning

**Dynamic Batch Sizing**:
```python
def calculate_optimal_batch_size(avg_key_size, avg_value_size, network_bandwidth_mbps):
    """
    Calculate optimal batch size based on data characteristics

    Target: 1-10 MB per batch for efficient network utilization
    """
    avg_entry_size = avg_key_size + avg_value_size + 100  # 100 bytes overhead
    target_batch_size_bytes = 5 * 1024 * 1024  # 5 MB

    optimal_batch_size = target_batch_size_bytes // avg_entry_size

    # Clamp between 100 and 10000
    return max(100, min(10000, optimal_batch_size))

# Usage
batch_size = calculate_optimal_batch_size(
    avg_key_size=50,          # 50 bytes
    avg_value_size=5000,      # 5 KB
    network_bandwidth_mbps=1000  # 1 Gbps
)
# Result: ~1000 keys per batch
```

**Monitoring and Adjustment**:
```promql
# Streaming throughput
rate(streaming_bytes_transferred_total[5m]) / 1024 / 1024  # MB/s

# If throughput < 50 MB/s and network not saturated:
# - Increase batch_size
# If throughput high but latency increasing:
# - Decrease batch_size
```

---

## 6. Common Operational Procedures

### 6.1 Health Check Interpretation

**Health Check Response**:
```json
{
  "status": "healthy",
  "disk_status": {
    "usage_percent": 75.5,
    "available_bytes": 250000000000,
    "is_throttled": false,
    "is_circuit_broken": false
  },
  "cache_status": {
    "size_bytes": 1073741824,
    "utilization_percent": 85.0,
    "hit_rate": 0.92
  },
  "memtable_status": {
    "size_bytes": 52428800,
    "entry_count": 125000
  },
  "worker_pools": {
    "flush_pool": {
      "active_workers": 2,
      "queued_tasks": 5,
      "utilization_percent": 50.0
    },
    "compaction_pool": {
      "active_workers": 1,
      "queued_tasks": 2,
      "utilization_percent": 50.0
    }
  },
  "metrics": {
    "write_qps": 5000,
    "read_qps": 15000,
    "cache_hit_rate": 0.92,
    "p99_write_latency_ms": 8.5,
    "p99_read_latency_ms": 3.2
  }
}
```

**Interpretation Guide**:

| Metric | Healthy Range | Action If Outside Range |
|--------|---------------|-------------------------|
| disk_usage_percent | < 80% | Trigger cleanup/expansion |
| cache_hit_rate | > 80% | Adjust cache size/policy |
| write_qps | Steady | Investigate sudden changes |
| p99_write_latency_ms | < 20ms | Check disk I/O, compaction |
| p99_read_latency_ms | < 10ms | Check cache, SSTable count |
| worker_pool utilization | 30-70% | Adjust worker count |

### 6.2 Metric Analysis

**Key Metrics Dashboard**:
```promql
# Write throughput
sum(rate(write_requests_total[5m])) by (node)

# Write latency
histogram_quantile(0.99, rate(write_duration_seconds_bucket[5m]))

# Read throughput
sum(rate(read_requests_total[5m])) by (node)

# Read latency
histogram_quantile(0.99, rate(read_duration_seconds_bucket[5m]))

# Cache hit rate
sum(rate(cache_hits_total[5m])) / sum(rate(cache_requests_total[5m]))

# Disk usage
disk_usage_percent

# Error rate by code
sum(rate(errors_total[5m])) by (error_code)
```

### 6.3 Log Analysis

**Important Log Patterns**:

**Normal Operations**:
```
INFO  Write completed tenant_id=tenant-1 key=user-123 latency=5ms
INFO  Memtable flushed size=64MB entries=150000 duration=2.5s
INFO  Compaction completed level=L0->L1 input_tables=4 output_size=256MB duration=45s
```

**Warnings to Monitor**:
```
WARN  Disk usage warning usage=82.5% threshold=80% available=175GB
WARN  Cache eviction rate high evictions_per_sec=1000 cache_size=1GB
WARN  Worker pool queue filling pool=flush_pool queued=85 capacity=100
```

**Errors Requiring Action**:
```
ERROR Checksum validation failed file=sstable-123.sst expected=0xABCD actual=0x1234
ERROR Disk write failed error="no space left on device" file=commitlog-456.log
ERROR Worker pool task failed pool=compaction_pool task_id=compact-789 error="..."
```

### 6.4 Troubleshooting Workflows

**High Write Latency**:
```
1. Check disk I/O:
   iostat -x 1 10
   → If util% > 90%: Disk bottleneck

2. Check commit log sync:
   grep "commit log sync" /var/log/storage-node.log
   → If slow: Consider async sync or faster disk

3. Check memtable size:
   curl http://node:8080/health | jq '.memtable_status.size_bytes'
   → If near threshold: Trigger flush

4. Check compaction:
   curl http://node:8080/api/v1/compaction/status
   → If running: May impact write performance
```

**Low Cache Hit Rate**:
```
1. Check cache size:
   curl http://node:8080/health | jq '.cache_status'
   → If utilization high: Increase cache size

2. Check access pattern:
   curl http://node:8080/api/v1/cache/stats
   → If uniformly distributed: Adjust eviction policy

3. Check adaptive weights:
   grep "Adjusted cache weights" /var/log/storage-node.log
   → Verify weights adapting to workload
```

**SSTable Count Growing**:
```
1. Check compaction status:
   curl http://node:8080/api/v1/compaction/status
   → If not running: Trigger manual compaction

2. Check L0 count:
   curl http://node:8080/api/v1/sstables/count-by-level
   → If L0 > 10: Aggressive L0 compaction needed

3. Check compaction worker pool:
   curl http://node:8080/health | jq '.worker_pools.compaction_pool'
   → If saturated: Increase worker count
```

---

## 7. Appendices

### Appendix A: Alert Configuration Examples

**Prometheus Alert Rules**:
```yaml
groups:
  - name: storage_node_alerts
    rules:
      - alert: DiskUsageHigh
        expr: disk_usage_percent > 80
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Disk usage high on {{ $labels.node }}"
          description: "Disk usage is {{ $value }}%"

      - alert: DiskThrottled
        expr: disk_usage_percent > 90
        for: 1m
        labels:
          severity: critical
        annotations:
          summary: "Disk throttled on {{ $labels.node }}"
          description: "Disk usage is {{ $value }}%, writes throttled"

      - alert: DiskCircuitBreakerEngaged
        expr: disk_usage_percent >= 95
        for: 1m
        labels:
          severity: critical
        annotations:
          summary: "Disk circuit breaker engaged on {{ $labels.node }}"
          description: "Disk usage is {{ $value }}%, all writes rejected"

      - alert: HighChecksumFailureRate
        expr: rate(checksum_failures_total[5m]) > 10
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "High checksum failure rate on {{ $labels.node }}"
          description: "Checksum failures: {{ $value }}/sec"

      - alert: HighWriteLatency
        expr: histogram_quantile(0.99, rate(write_duration_seconds_bucket[5m])) > 0.1
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "High write latency on {{ $labels.node }}"
          description: "P99 write latency: {{ $value }}s"
```

### Appendix B: Maintenance Checklist

**Weekly Maintenance**:
- [ ] Review disk usage trends
- [ ] Check checksum failure rate
- [ ] Review error logs
- [ ] Verify backup completion
- [ ] Check compaction effectiveness
- [ ] Review cache hit rate

**Monthly Maintenance**:
- [ ] Analyze capacity growth trends
- [ ] Plan capacity expansion if needed
- [ ] Review and tune thresholds
- [ ] Test disaster recovery procedures
- [ ] Update documentation
- [ ] Review alert configurations

**Quarterly Maintenance**:
- [ ] Perform full integrity scan
- [ ] Review and optimize worker pool sizing
- [ ] Analyze performance trends
- [ ] Update runbooks based on incidents
- [ ] Test node replacement procedure
- [ ] Review SLAs and error budgets

## 5. Node Lifecycle Operations

### 5.1 Adding a Storage Node (Bootstrap)

#### Prerequisites
- New storage node is running and accessible
- Node has been allocated token positions (automatic via coordinator)
- Sufficient network bandwidth for data streaming

#### Bootstrap Procedure

**Step 1: Trigger Bootstrap**
```bash
grpcurl -plaintext \
  -d '{
    "node_id": "storage-node-4",
    "host": "10.0.1.4",
    "port": 8001,
    "virtual_nodes": 150
  }' \
  coordinator.internal:8080 \
  pairdb.CoordinatorService/AddStorageNode
```

**Step 2: Monitor Bootstrap Progress**
```bash
# Check node state (should be BOOTSTRAPPING)
grpcurl -plaintext \
  -d '{"node_id": "storage-node-4"}' \
  coordinator.internal:8080 \
  pairdb.CoordinatorService/GetStorageNodeStatus

# Monitor pending changes
psql -h postgres.internal -U pairdb_user -d pairdb \
  -c "SELECT change_id, type, status, progress FROM pending_changes WHERE type='bootstrap';"
```

**Step 3: Bootstrap Phases**

The node goes through these phases:
1. **State: BOOTSTRAPPING** - Node NOT in ring yet
2. **Data Streaming** - Receives historical data from source nodes
3. **Hint Replay** - Replays any missed writes (Phase 6)
4. **State: NORMAL** - Node added to ring, fully operational

**Step 4: Verify Success**
```bash
# Check node state (should be NORMAL)
grpcurl -plaintext \
  -d '{"node_id": "storage-node-4"}' \
  coordinator.internal:8080 \
  pairdb.CoordinatorService/GetStorageNodeStatus

# Verify node is in ring
grpcurl -plaintext \
  coordinator.internal:8080 \
  pairdb.CoordinatorService/ListStorageNodes | grep "storage-node-4"

# Check hint replay metrics
curl http://localhost:9090/metrics | grep hints_replayed_total
```

**Expected Duration**: 10-60 minutes depending on data size

### 5.2 Removing a Storage Node (Decommission)

#### Prerequisites
- Node to be removed is currently in NORMAL state
- Cluster has enough capacity to handle data redistribution
- No pending bootstrap operations

#### Decommission Procedure

**Step 1: Trigger Decommission**
```bash
grpcurl -plaintext \
  -d '{
    "node_id": "storage-node-2",
    "force": false
  }' \
  coordinator.internal:8080 \
  pairdb.CoordinatorService/RemoveStorageNode
```

**Step 2: Monitor Decommission Progress**
```bash
# Check node state (should be LEAVING)
grpcurl -plaintext \
  -d '{"node_id": "storage-node-2"}' \
  coordinator.internal:8080 \
  pairdb.CoordinatorService/GetStorageNodeStatus

# Monitor streaming progress
psql -h postgres.internal -U pairdb_user -d pairdb \
  -c "SELECT change_id, type, status, progress FROM pending_changes WHERE type='decommission';"
```

**Step 3: Decommission Phases**

The node goes through these phases:
1. **State: LEAVING** - Node STAYS in ring during streaming
2. **Data Streaming** - Streams data to inheritor nodes
3. **Removal from Ring** - After streaming completes
4. **Cleanup Schedule** - 24-hour grace period before data deletion (Phase 7)

**Step 4: Verify Success**
```bash
# Node should be removed from metadata
grpcurl -plaintext \
  coordinator.internal:8080 \
  pairdb.CoordinatorService/ListStorageNodes | grep "storage-node-2"
# (should return no results)

# Verify cleanup is scheduled
psql -h postgres.internal -U pairdb_user -d pairdb \
  -c "SELECT change_id, completed_at FROM pending_changes WHERE type='decommission' AND status='completed';"
```

**Expected Duration**: 10-60 minutes

### 5.3 Monitoring Node Operations

#### Key Metrics for Bootstrap/Decommission

**Hint Metrics** (Phase 6):
```
hints_stored_total - Total hints created for failed writes
hints_replayed_total - Total hints successfully replayed
hints_failed_total - Total hints that failed replay
hints_cleanup_deleted_total - Hints deleted by cleanup process
```

**Bootstrap Metrics**:
```
bootstrap_operations_total - Total bootstrap operations
bootstrap_with_hints_total - Bootstraps that replayed hints
bootstrap_hint_replay_duration_seconds - Time to replay hints
concurrent_bootstraps_gauge - Number of concurrent bootstraps (Phase 8)
```

**Cleanup Metrics** (Phase 7):
```
cleanup_scheduled_total - Total cleanups scheduled
cleanup_executed_total - Total cleanups executed
cleanup_failed_safety_check_total - Cleanups rejected by safety checks
cleanup_quorum_verification_duration_seconds - Time for quorum verification
```

#### Database Queries

**Check Active Topology Changes**:
```sql
SELECT
  change_id,
  type,
  node_id,
  status,
  start_time,
  EXTRACT(EPOCH FROM (NOW() - start_time)) AS duration_seconds
FROM pending_changes
WHERE status = 'in_progress';
```

**Check Hint Backlog**:
```sql
SELECT
  target_node_id,
  COUNT(*) AS hint_count,
  MIN(created_at) AS oldest_hint,
  MAX(created_at) AS newest_hint
FROM hints
GROUP BY target_node_id;
```

**Check Node States**:
```sql
SELECT
  node_id,
  state,
  status,
  created_at,
  updated_at
FROM storage_nodes
ORDER BY created_at;
```

### 5.4 Troubleshooting Node Operations

#### Bootstrap Stuck in BOOTSTRAPPING State

**Symptoms**: Node remains in BOOTSTRAPPING state for hours

**Diagnosis**:
```bash
# Check streaming progress
psql -h postgres.internal -U pairdb_user -d pairdb \
  -c "SELECT * FROM pending_changes WHERE node_id='<node-id>' AND type='bootstrap';"

# Check coordinator logs
tail -f /var/log/pairdb/coordinator.log | grep "<node-id>"
```

**Common Causes**:
1. Network issues - Source node unreachable
2. Streaming failure - Data transfer errors
3. Hint replay failure - Target node not responding

**Resolution**:
```bash
# Force rollback if needed (CAUTION)
psql -h postgres.internal -U pairdb_user -d pairdb \
  -c "UPDATE pending_changes SET status='failed' WHERE change_id='<change-id>';"

# Remove node from metadata
psql -h postgres.internal -U pairdb_user -d pairdb \
  -c "DELETE FROM storage_nodes WHERE node_id='<node-id>';"
```

#### Hints Not Being Replayed

**Symptoms**: Hints table growing, hints not deleted

**Diagnosis**:
```sql
SELECT
  hint_id,
  target_node_id,
  replay_count,
  created_at
FROM hints
WHERE target_node_id = '<node-id>'
ORDER BY created_at DESC
LIMIT 10;
```

**Common Causes**:
1. Target node down - Node not accepting writes
2. Max retries exceeded - Hints tried 3 times and failed
3. Hint replay service not running - Check coordinator logs

**Resolution**:
```bash
# Check target node health
curl http://<node-host>:8001/health

# Delete old failed hints (CAUTION)
psql -h postgres.internal -U pairdb_user -d pairdb \
  -c "DELETE FROM hints WHERE replay_count >= 3 AND created_at < NOW() - INTERVAL '7 days';"
```

#### Cleanup Safety Verification Failed

**Symptoms**: Pending changes remain after grace period

**Diagnosis**:
```sql
SELECT
  change_id,
  type,
  status,
  completed_at,
  EXTRACT(EPOCH FROM (NOW() - completed_at))/3600 AS hours_since_completion
FROM pending_changes
WHERE status = 'completed'
  AND completed_at < NOW() - INTERVAL '24 hours';
```

**Common Causes**:
1. Quorum verification failed - Not enough replicas have the data
2. Grace period tracking issue - CompletedAt not set correctly

**Resolution**:
```bash
# Check if quorum is available
psql -h postgres.internal -U pairdb_user -d pairdb \
  -c "SELECT node_id, state FROM storage_nodes WHERE state='NORMAL';"

# Force cleanup (EMERGENCY ONLY)
psql -h postgres.internal -U pairdb_user -d pairdb \
  -c "DELETE FROM pending_changes WHERE change_id='<change-id>';"
```

### 5.5 Best Practices

#### Bootstrap Best Practices
1. **Schedule during low traffic**: Minimize impact on production
2. **One at a time**: Unless testing concurrent capability (Phase 8)
3. **Monitor closely**: First 30 minutes are critical
4. **Verify hint replay**: Check metrics after completion

#### Decommission Best Practices
1. **Verify capacity**: Ensure cluster can handle redistribution
2. **Drain traffic first**: Reduce active writes to node
3. **Respect grace period**: Let 24-hour safety window protect data (Phase 7)
4. **Monitor inheritors**: Check they receive data correctly

#### Hint Management Best Practices
1. **Monitor backlog**: Keep under 1000 hints per node
2. **Investigate patterns**: Repeated failures indicate issues
3. **Tune TTL**: Adjust based on recovery patterns (default: 7 days)
4. **Clean up promptly**: After node recovery

### Appendix C: Emergency Contacts

**Escalation Path**:
1. On-call Engineer (Pagerduty)
2. Team Lead
3. Engineering Manager
4. VP Engineering

**Useful Links**:
- Monitoring Dashboard: https://grafana.internal/storage-nodes
- Runbook Repository: https://github.com/company/runbooks
- Incident Management: https://incident.io
- Slack Channel: #storage-team

---

*Document Version: 2.0*
*Last Updated: 2026-01-10*
*Maintained by: Storage Team*
