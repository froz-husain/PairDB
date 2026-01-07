# Monitoring Guide

## Overview

This guide covers monitoring, metrics, and alerting for PairDB Storage Node.

## Metrics Endpoint

Prometheus metrics are exposed on port 9091:
```
http://<pod-ip>:9091/metrics
```

## Key Metrics

### Request Metrics

#### Write Operations
```promql
# Request rate
rate(pairdb_storage_write_requests_total[5m])

# P99 latency
histogram_quantile(0.99, rate(pairdb_storage_write_requests_duration_seconds_bucket[5m]))

# Write throughput (bytes/sec)
rate(pairdb_storage_write_requests_bytes_sum[5m])
```

#### Read Operations
```promql
# Request rate
rate(pairdb_storage_read_requests_total[5m])

# P99 latency
histogram_quantile(0.99, rate(pairdb_storage_read_requests_duration_seconds_bucket[5m]))

# Read throughput (bytes/sec)
rate(pairdb_storage_read_requests_bytes_sum[5m])
```

### Cache Metrics

#### Cache Hit Rate
```promql
# Hit rate percentage
rate(pairdb_cache_hits_total[5m]) /
(rate(pairdb_cache_hits_total[5m]) + rate(pairdb_cache_misses_total[5m])) * 100
```

#### Cache Size
```promql
# Current cache size in bytes
pairdb_cache_size_bytes

# Number of entries
pairdb_cache_entries_total
```

#### Cache Evictions
```promql
# Eviction rate
rate(pairdb_cache_evictions_total[5m])
```

### Storage Metrics

#### MemTable
```promql
# MemTable size
pairdb_memtable_size_bytes

# MemTable entries
pairdb_memtable_entries_total

# Flush rate
rate(pairdb_memtable_flushes_total[5m])
```

#### SSTable
```promql
# SSTable count by level
pairdb_sstable_count_by_level

# SSTable size by level
pairdb_sstable_size_bytes_by_level

# SSTable read rate
rate(pairdb_sstable_reads_total[5m])
```

### Compaction Metrics

```promql
# Compaction job rate by status
rate(pairdb_compaction_jobs_total[5m])

# Average compaction duration
rate(pairdb_compaction_job_duration_seconds_sum[5m]) /
rate(pairdb_compaction_job_duration_seconds_count[5m])

# Bytes processed per second
rate(pairdb_compaction_bytes_processed_total[5m])
```

### System Metrics

#### Disk
```promql
# Disk usage percentage
pairdb_system_disk_usage_percent

# Available disk space
pairdb_system_disk_available_bytes
```

#### Memory
```promql
# Memory usage
pairdb_system_memory_usage_bytes
```

#### Goroutines
```promql
# Goroutine count (leak detection)
pairdb_system_goroutines_total
```

### Gossip Metrics

```promql
# Cluster members
pairdb_gossip_members_total

# Healthy members
pairdb_gossip_members_healthy

# Message rate
rate(pairdb_gossip_messages_total[5m])
```

## Alerts

Alerts are defined in `deployments/monitoring/alerts.yaml`.

### Critical Alerts

#### Pod Down
```yaml
alert: PairDBPodDown
expr: up{job="pairdb-storage-node"} == 0
for: 2m
severity: critical
```

**What it means**: A storage node pod is not responding to health checks.

**Action**:
1. Check pod status: `kubectl get pods -n pairdb`
2. Check pod logs: `kubectl logs -n pairdb <pod-name>`
3. Check events: `kubectl describe pod -n pairdb <pod-name>`

#### Critical Disk Usage
```yaml
alert: PairDBCriticalDiskUsage
expr: pairdb_system_disk_usage_percent > 95
for: 2m
severity: critical
```

**What it means**: Disk is >95% full. Node will become readonly soon.

**Action**:
1. Identify large files: `kubectl exec <pod> -- du -sh /data/*`
2. Trigger compaction if safe
3. Consider scaling up storage or cleaning old data
4. Check commit log retention settings

### Warning Alerts

#### High Error Rate
```yaml
alert: PairDBHighErrorRate
expr: error_rate > 5%
for: 5m
severity: warning
```

**What it means**: More than 5% of requests are failing.

**Action**:
1. Check logs for error patterns
2. Verify coordinator health
3. Check network connectivity
4. Review recent changes/deployments

#### Slow Write Latency
```yaml
alert: PairDBSlowWriteLatency
expr: P99 write latency > 100ms
for: 5m
severity: warning
```

**What it means**: Write performance is degraded.

**Action**:
1. Check disk I/O: `kubectl exec <pod> -- iostat`
2. Check memory pressure
3. Review compaction status
4. Check for resource contention

#### Too Many L0 SSTables
```yaml
alert: PairDBTooManyL0SSTables
expr: pairdb_sstable_count_by_level{level="0"} > 8
for: 5m
severity: warning
```

**What it means**: Compaction is falling behind.

**Action**:
1. Check compaction workers are running
2. Review compaction failure logs
3. Check disk I/O capacity
4. Consider increasing compaction workers

### Info Alerts

#### Low Cache Hit Rate
```yaml
alert: PairDBLowCacheHitRate
expr: cache_hit_rate < 50%
for: 10m
severity: info
```

**What it means**: Cache is not effective. Read performance may be impacted.

**Action**:
1. Review access patterns
2. Consider increasing cache size
3. Check for cache eviction rate
4. Analyze query distribution

## Dashboards

### Recommended Grafana Panels

#### Overview Dashboard

1. **Request Rate**
   - Write requests/sec
   - Read requests/sec
   - Error rate

2. **Latency**
   - P50, P95, P99 write latency
   - P50, P95, P99 read latency

3. **Cache Performance**
   - Hit rate percentage
   - Cache size
   - Eviction rate

4. **Storage Health**
   - MemTable size
   - SSTable count by level
   - Compaction rate

5. **System Resources**
   - Disk usage %
   - Memory usage
   - Goroutine count

#### Detailed Dashboard

1. **Per-Node Metrics**
   - All above metrics per pod

2. **Commit Log**
   - Segment count
   - Size
   - Append/sync latency

3. **Compaction**
   - Jobs by status
   - Duration
   - Bytes processed

4. **Gossip**
   - Member count
   - Health status
   - Message rate

## Logging

### Log Levels

Configure via config.yaml:
```yaml
logging:
  level: info  # debug, info, warn, error
  format: json # json or console
```

### Structured Logging

All logs include:
- Timestamp
- Log level
- Component
- Message
- Contextual fields (tenant_id, key, etc.)

### Log Aggregation

Use a log aggregation system (ELK, Loki, etc.):

```bash
# Example: Forward logs to Loki
kubectl logs -f -n pairdb pairdb-storage-node-0 | promtail
```

### Important Log Patterns

#### Errors
```
{"level":"error","component":"storage","msg":"Write failed"}
```

#### Warnings
```
{"level":"warn","component":"compaction","msg":"Compaction queue full"}
```

#### Performance
```
{"level":"info","component":"storage","msg":"Write completed","latency":"15ms"}
```

## Health Checks

### Liveness Probe

**Endpoint**: `GET /health/live`

**Success criteria**:
- HTTP 200
- Process responsive

**Failure action**:
- Kubernetes restarts the pod

### Readiness Probe

**Endpoint**: `GET /health/ready`

**Success criteria**:
- HTTP 200
- Disk usage < 90%
- Data directory accessible
- No critical errors

**Failure action**:
- Pod removed from service endpoints
- No new traffic routed to pod

## Troubleshooting

### High Latency

1. Check metrics:
```promql
histogram_quantile(0.99,
  rate(pairdb_storage_write_requests_duration_seconds_bucket[5m])
)
```

2. Check resource usage:
```bash
kubectl top pod -n pairdb
```

3. Check disk I/O:
```bash
kubectl exec -n pairdb <pod> -- iostat -x 1 5
```

### Memory Issues

1. Check memory usage:
```promql
pairdb_system_memory_usage_bytes
```

2. Check for goroutine leaks:
```promql
pairdb_system_goroutines_total
```

3. Get Go heap profile:
```bash
kubectl exec -n pairdb <pod> -- curl http://localhost:6060/debug/pprof/heap > heap.prof
go tool pprof heap.prof
```

### Disk Issues

1. Check disk usage:
```bash
kubectl exec -n pairdb <pod> -- df -h
```

2. Find large files:
```bash
kubectl exec -n pairdb <pod> -- du -sh /data/* | sort -h
```

3. Check commit log segments:
```bash
kubectl exec -n pairdb <pod> -- ls -lh /data/commitlog/
```

## Performance Tuning

### Cache Size

Increase cache for better read performance:
```yaml
cache:
  max_size: 536870912  # 512MB (increased from 256MB)
```

### Compaction Workers

Increase workers for faster compaction:
```yaml
compaction:
  workers: 8  # Increased from 4
```

### MemTable Size

Larger memtable reduces flush frequency:
```yaml
memtable:
  flush_threshold: 104857600  # 100MB (increased from 50MB)
```

## Best Practices

1. **Set up alerts before going to production**
   - Configure PagerDuty/OpsGenie
   - Test alert delivery
   - Document runbooks

2. **Monitor trends over time**
   - Track latency trends
   - Watch disk usage growth
   - Monitor cache effectiveness

3. **Use dashboards for visibility**
   - Create team dashboards
   - Display on TVs
   - Review weekly

4. **Automate responses**
   - Auto-scale based on metrics
   - Auto-restart unhealthy pods
   - Auto-clean old data

5. **Regular reviews**
   - Weekly metric reviews
   - Monthly capacity planning
   - Quarterly optimization

## Metrics Collection

### Prometheus Configuration

```yaml
scrape_configs:
- job_name: 'pairdb-storage-node'
  kubernetes_sd_configs:
  - role: pod
    namespaces:
      names:
      - pairdb
  relabel_configs:
  - source_labels: [__meta_kubernetes_pod_label_app]
    regex: pairdb-storage-node
    action: keep
  - source_labels: [__meta_kubernetes_pod_name]
    target_label: pod
  - source_labels: [__meta_kubernetes_pod_ip]
    target_label: __address__
    replacement: $1:9091
```

## Resources

- [Prometheus Query Examples](https://prometheus.io/docs/prometheus/latest/querying/examples/)
- [Grafana Dashboard Best Practices](https://grafana.com/docs/grafana/latest/best-practices/)
- [Alert Rule Best Practices](https://prometheus.io/docs/practices/alerting/)
