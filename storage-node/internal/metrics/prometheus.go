package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics holds all Prometheus metrics for the storage node
type Metrics struct {
	// Write/Read operation metrics
	WriteRequestsTotal     prometheus.Counter
	WriteRequestsDuration  prometheus.Histogram
	WriteRequestsBytes     prometheus.Histogram
	ReadRequestsTotal      prometheus.Counter
	ReadRequestsDuration   prometheus.Histogram
	ReadRequestsBytes      prometheus.Histogram
	RepairRequestsTotal    prometheus.Counter
	RepairRequestsDuration prometheus.Histogram

	// Cache metrics
	CacheHitsTotal      prometheus.Counter
	CacheMissesTotal    prometheus.Counter
	CacheEvictionsTotal prometheus.Counter
	CacheSizeBytes      prometheus.Gauge
	CacheEntriesTotal   prometheus.Gauge

	// Storage metrics
	MemTableSizeBytes   prometheus.Gauge
	MemTableEntriesTotal prometheus.Gauge
	MemTableFlushesTotal prometheus.Counter
	MemTableFlushDuration prometheus.Histogram

	SSTableCountByLevel prometheus.GaugeVec
	SSTableSizeByLevel  prometheus.GaugeVec
	SSTableReadsTotal   prometheus.Counter
	SSTableReadDuration prometheus.Histogram
	SSTableWritesTotal  prometheus.Counter
	SSTableWriteDuration prometheus.Histogram

	// Commit log metrics
	CommitLogSegmentsTotal prometheus.Gauge
	CommitLogSizeBytes     prometheus.Gauge
	CommitLogAppendsTotal  prometheus.Counter
	CommitLogAppendDuration prometheus.Histogram
	CommitLogSyncsTotal    prometheus.Counter
	CommitLogSyncDuration  prometheus.Histogram

	// Compaction metrics
	CompactionJobsTotal      prometheus.CounterVec
	CompactionJobDuration    prometheus.Histogram
	CompactionBytesProcessed prometheus.Counter
	CompactionBytesWritten   prometheus.Counter
	CompactionTablesInput    prometheus.Histogram
	CompactionTablesOutput   prometheus.Histogram

	// Gossip metrics
	GossipMembersTotal      prometheus.Gauge
	GossipMembersHealthy    prometheus.Gauge
	GossipMessagesTotal     prometheus.CounterVec
	GossipMessagesDuration  prometheus.Histogram

	// System metrics
	DiskUsageBytes      prometheus.Gauge
	DiskAvailableBytes  prometheus.Gauge
	DiskUsagePercent    prometheus.Gauge
	MemoryUsageBytes    prometheus.Gauge
	GoroutinesTotal     prometheus.Gauge
}

// NewMetrics creates and registers all Prometheus metrics
func NewMetrics(nodeID string) *Metrics {
	labels := prometheus.Labels{"node_id": nodeID}

	return &Metrics{
		// Write/Read operation metrics
		WriteRequestsTotal: promauto.NewCounter(prometheus.CounterOpts{
			Namespace:   "pairdb",
			Subsystem:   "storage",
			Name:        "write_requests_total",
			Help:        "Total number of write requests",
			ConstLabels: labels,
		}),
		WriteRequestsDuration: promauto.NewHistogram(prometheus.HistogramOpts{
			Namespace:   "pairdb",
			Subsystem:   "storage",
			Name:        "write_requests_duration_seconds",
			Help:        "Histogram of write request durations",
			ConstLabels: labels,
			Buckets:     prometheus.DefBuckets,
		}),
		WriteRequestsBytes: promauto.NewHistogram(prometheus.HistogramOpts{
			Namespace:   "pairdb",
			Subsystem:   "storage",
			Name:        "write_requests_bytes",
			Help:        "Histogram of write request sizes in bytes",
			ConstLabels: labels,
			Buckets:     prometheus.ExponentialBuckets(256, 2, 10), // 256B to 128KB
		}),
		ReadRequestsTotal: promauto.NewCounter(prometheus.CounterOpts{
			Namespace:   "pairdb",
			Subsystem:   "storage",
			Name:        "read_requests_total",
			Help:        "Total number of read requests",
			ConstLabels: labels,
		}),
		ReadRequestsDuration: promauto.NewHistogram(prometheus.HistogramOpts{
			Namespace:   "pairdb",
			Subsystem:   "storage",
			Name:        "read_requests_duration_seconds",
			Help:        "Histogram of read request durations",
			ConstLabels: labels,
			Buckets:     prometheus.DefBuckets,
		}),
		ReadRequestsBytes: promauto.NewHistogram(prometheus.HistogramOpts{
			Namespace:   "pairdb",
			Subsystem:   "storage",
			Name:        "read_requests_bytes",
			Help:        "Histogram of read response sizes in bytes",
			ConstLabels: labels,
			Buckets:     prometheus.ExponentialBuckets(256, 2, 10),
		}),
		RepairRequestsTotal: promauto.NewCounter(prometheus.CounterOpts{
			Namespace:   "pairdb",
			Subsystem:   "storage",
			Name:        "repair_requests_total",
			Help:        "Total number of repair requests",
			ConstLabels: labels,
		}),
		RepairRequestsDuration: promauto.NewHistogram(prometheus.HistogramOpts{
			Namespace:   "pairdb",
			Subsystem:   "storage",
			Name:        "repair_requests_duration_seconds",
			Help:        "Histogram of repair request durations",
			ConstLabels: labels,
			Buckets:     prometheus.DefBuckets,
		}),

		// Cache metrics
		CacheHitsTotal: promauto.NewCounter(prometheus.CounterOpts{
			Namespace:   "pairdb",
			Subsystem:   "cache",
			Name:        "hits_total",
			Help:        "Total number of cache hits",
			ConstLabels: labels,
		}),
		CacheMissesTotal: promauto.NewCounter(prometheus.CounterOpts{
			Namespace:   "pairdb",
			Subsystem:   "cache",
			Name:        "misses_total",
			Help:        "Total number of cache misses",
			ConstLabels: labels,
		}),
		CacheEvictionsTotal: promauto.NewCounter(prometheus.CounterOpts{
			Namespace:   "pairdb",
			Subsystem:   "cache",
			Name:        "evictions_total",
			Help:        "Total number of cache evictions",
			ConstLabels: labels,
		}),
		CacheSizeBytes: promauto.NewGauge(prometheus.GaugeOpts{
			Namespace:   "pairdb",
			Subsystem:   "cache",
			Name:        "size_bytes",
			Help:        "Current cache size in bytes",
			ConstLabels: labels,
		}),
		CacheEntriesTotal: promauto.NewGauge(prometheus.GaugeOpts{
			Namespace:   "pairdb",
			Subsystem:   "cache",
			Name:        "entries_total",
			Help:        "Current number of entries in cache",
			ConstLabels: labels,
		}),

		// Storage metrics
		MemTableSizeBytes: promauto.NewGauge(prometheus.GaugeOpts{
			Namespace:   "pairdb",
			Subsystem:   "memtable",
			Name:        "size_bytes",
			Help:        "Current memtable size in bytes",
			ConstLabels: labels,
		}),
		MemTableEntriesTotal: promauto.NewGauge(prometheus.GaugeOpts{
			Namespace:   "pairdb",
			Subsystem:   "memtable",
			Name:        "entries_total",
			Help:        "Current number of entries in memtable",
			ConstLabels: labels,
		}),
		MemTableFlushesTotal: promauto.NewCounter(prometheus.CounterOpts{
			Namespace:   "pairdb",
			Subsystem:   "memtable",
			Name:        "flushes_total",
			Help:        "Total number of memtable flushes",
			ConstLabels: labels,
		}),
		MemTableFlushDuration: promauto.NewHistogram(prometheus.HistogramOpts{
			Namespace:   "pairdb",
			Subsystem:   "memtable",
			Name:        "flush_duration_seconds",
			Help:        "Histogram of memtable flush durations",
			ConstLabels: labels,
			Buckets:     prometheus.DefBuckets,
		}),
		SSTableCountByLevel: *promauto.NewGaugeVec(prometheus.GaugeOpts{
			Namespace:   "pairdb",
			Subsystem:   "sstable",
			Name:        "count_by_level",
			Help:        "Number of SSTables by level",
			ConstLabels: labels,
		}, []string{"level"}),
		SSTableSizeByLevel: *promauto.NewGaugeVec(prometheus.GaugeOpts{
			Namespace:   "pairdb",
			Subsystem:   "sstable",
			Name:        "size_bytes_by_level",
			Help:        "Total size of SSTables by level in bytes",
			ConstLabels: labels,
		}, []string{"level"}),
		SSTableReadsTotal: promauto.NewCounter(prometheus.CounterOpts{
			Namespace:   "pairdb",
			Subsystem:   "sstable",
			Name:        "reads_total",
			Help:        "Total number of SSTable reads",
			ConstLabels: labels,
		}),
		SSTableReadDuration: promauto.NewHistogram(prometheus.HistogramOpts{
			Namespace:   "pairdb",
			Subsystem:   "sstable",
			Name:        "read_duration_seconds",
			Help:        "Histogram of SSTable read durations",
			ConstLabels: labels,
			Buckets:     prometheus.DefBuckets,
		}),
		SSTableWritesTotal: promauto.NewCounter(prometheus.CounterOpts{
			Namespace:   "pairdb",
			Subsystem:   "sstable",
			Name:        "writes_total",
			Help:        "Total number of SSTable writes",
			ConstLabels: labels,
		}),
		SSTableWriteDuration: promauto.NewHistogram(prometheus.HistogramOpts{
			Namespace:   "pairdb",
			Subsystem:   "sstable",
			Name:        "write_duration_seconds",
			Help:        "Histogram of SSTable write durations",
			ConstLabels: labels,
			Buckets:     prometheus.DefBuckets,
		}),

		// Commit log metrics
		CommitLogSegmentsTotal: promauto.NewGauge(prometheus.GaugeOpts{
			Namespace:   "pairdb",
			Subsystem:   "commitlog",
			Name:        "segments_total",
			Help:        "Current number of commit log segments",
			ConstLabels: labels,
		}),
		CommitLogSizeBytes: promauto.NewGauge(prometheus.GaugeOpts{
			Namespace:   "pairdb",
			Subsystem:   "commitlog",
			Name:        "size_bytes",
			Help:        "Current commit log size in bytes",
			ConstLabels: labels,
		}),
		CommitLogAppendsTotal: promauto.NewCounter(prometheus.CounterOpts{
			Namespace:   "pairdb",
			Subsystem:   "commitlog",
			Name:        "appends_total",
			Help:        "Total number of commit log appends",
			ConstLabels: labels,
		}),
		CommitLogAppendDuration: promauto.NewHistogram(prometheus.HistogramOpts{
			Namespace:   "pairdb",
			Subsystem:   "commitlog",
			Name:        "append_duration_seconds",
			Help:        "Histogram of commit log append durations",
			ConstLabels: labels,
			Buckets:     prometheus.DefBuckets,
		}),
		CommitLogSyncsTotal: promauto.NewCounter(prometheus.CounterOpts{
			Namespace:   "pairdb",
			Subsystem:   "commitlog",
			Name:        "syncs_total",
			Help:        "Total number of commit log syncs",
			ConstLabels: labels,
		}),
		CommitLogSyncDuration: promauto.NewHistogram(prometheus.HistogramOpts{
			Namespace:   "pairdb",
			Subsystem:   "commitlog",
			Name:        "sync_duration_seconds",
			Help:        "Histogram of commit log sync durations",
			ConstLabels: labels,
			Buckets:     prometheus.DefBuckets,
		}),

		// Compaction metrics
		CompactionJobsTotal: *promauto.NewCounterVec(prometheus.CounterOpts{
			Namespace:   "pairdb",
			Subsystem:   "compaction",
			Name:        "jobs_total",
			Help:        "Total number of compaction jobs by status",
			ConstLabels: labels,
		}, []string{"status"}),
		CompactionJobDuration: promauto.NewHistogram(prometheus.HistogramOpts{
			Namespace:   "pairdb",
			Subsystem:   "compaction",
			Name:        "job_duration_seconds",
			Help:        "Histogram of compaction job durations",
			ConstLabels: labels,
			Buckets:     prometheus.DefBuckets,
		}),
		CompactionBytesProcessed: promauto.NewCounter(prometheus.CounterOpts{
			Namespace:   "pairdb",
			Subsystem:   "compaction",
			Name:        "bytes_processed_total",
			Help:        "Total bytes processed during compaction",
			ConstLabels: labels,
		}),
		CompactionBytesWritten: promauto.NewCounter(prometheus.CounterOpts{
			Namespace:   "pairdb",
			Subsystem:   "compaction",
			Name:        "bytes_written_total",
			Help:        "Total bytes written during compaction",
			ConstLabels: labels,
		}),
		CompactionTablesInput: promauto.NewHistogram(prometheus.HistogramOpts{
			Namespace:   "pairdb",
			Subsystem:   "compaction",
			Name:        "tables_input",
			Help:        "Histogram of input tables per compaction",
			ConstLabels: labels,
			Buckets:     prometheus.LinearBuckets(1, 1, 10),
		}),
		CompactionTablesOutput: promauto.NewHistogram(prometheus.HistogramOpts{
			Namespace:   "pairdb",
			Subsystem:   "compaction",
			Name:        "tables_output",
			Help:        "Histogram of output tables per compaction",
			ConstLabels: labels,
			Buckets:     prometheus.LinearBuckets(1, 1, 10),
		}),

		// Gossip metrics
		GossipMembersTotal: promauto.NewGauge(prometheus.GaugeOpts{
			Namespace:   "pairdb",
			Subsystem:   "gossip",
			Name:        "members_total",
			Help:        "Total number of gossip members",
			ConstLabels: labels,
		}),
		GossipMembersHealthy: promauto.NewGauge(prometheus.GaugeOpts{
			Namespace:   "pairdb",
			Subsystem:   "gossip",
			Name:        "members_healthy",
			Help:        "Number of healthy gossip members",
			ConstLabels: labels,
		}),
		GossipMessagesTotal: *promauto.NewCounterVec(prometheus.CounterOpts{
			Namespace:   "pairdb",
			Subsystem:   "gossip",
			Name:        "messages_total",
			Help:        "Total number of gossip messages by type",
			ConstLabels: labels,
		}, []string{"type"}),
		GossipMessagesDuration: promauto.NewHistogram(prometheus.HistogramOpts{
			Namespace:   "pairdb",
			Subsystem:   "gossip",
			Name:        "messages_duration_seconds",
			Help:        "Histogram of gossip message processing durations",
			ConstLabels: labels,
			Buckets:     prometheus.DefBuckets,
		}),

		// System metrics
		DiskUsageBytes: promauto.NewGauge(prometheus.GaugeOpts{
			Namespace:   "pairdb",
			Subsystem:   "system",
			Name:        "disk_usage_bytes",
			Help:        "Current disk usage in bytes",
			ConstLabels: labels,
		}),
		DiskAvailableBytes: promauto.NewGauge(prometheus.GaugeOpts{
			Namespace:   "pairdb",
			Subsystem:   "system",
			Name:        "disk_available_bytes",
			Help:        "Available disk space in bytes",
			ConstLabels: labels,
		}),
		DiskUsagePercent: promauto.NewGauge(prometheus.GaugeOpts{
			Namespace:   "pairdb",
			Subsystem:   "system",
			Name:        "disk_usage_percent",
			Help:        "Disk usage percentage",
			ConstLabels: labels,
		}),
		MemoryUsageBytes: promauto.NewGauge(prometheus.GaugeOpts{
			Namespace:   "pairdb",
			Subsystem:   "system",
			Name:        "memory_usage_bytes",
			Help:        "Current memory usage in bytes",
			ConstLabels: labels,
		}),
		GoroutinesTotal: promauto.NewGauge(prometheus.GaugeOpts{
			Namespace:   "pairdb",
			Subsystem:   "system",
			Name:        "goroutines_total",
			Help:        "Current number of goroutines",
			ConstLabels: labels,
		}),
	}
}

// RecordWriteRequest records metrics for a write request
func (m *Metrics) RecordWriteRequest(duration float64, bytes int) {
	m.WriteRequestsTotal.Inc()
	m.WriteRequestsDuration.Observe(duration)
	m.WriteRequestsBytes.Observe(float64(bytes))
}

// RecordReadRequest records metrics for a read request
func (m *Metrics) RecordReadRequest(duration float64, bytes int) {
	m.ReadRequestsTotal.Inc()
	m.ReadRequestsDuration.Observe(duration)
	m.ReadRequestsBytes.Observe(float64(bytes))
}

// RecordRepairRequest records metrics for a repair request
func (m *Metrics) RecordRepairRequest(duration float64) {
	m.RepairRequestsTotal.Inc()
	m.RepairRequestsDuration.Observe(duration)
}

// RecordCacheHit records a cache hit
func (m *Metrics) RecordCacheHit() {
	m.CacheHitsTotal.Inc()
}

// RecordCacheMiss records a cache miss
func (m *Metrics) RecordCacheMiss() {
	m.CacheMissesTotal.Inc()
}

// RecordCacheEviction records a cache eviction
func (m *Metrics) RecordCacheEviction() {
	m.CacheEvictionsTotal.Inc()
}

// UpdateCacheSize updates cache size metrics
func (m *Metrics) UpdateCacheSize(bytes int64, entries int64) {
	m.CacheSizeBytes.Set(float64(bytes))
	m.CacheEntriesTotal.Set(float64(entries))
}

// UpdateMemTableSize updates memtable size metrics
func (m *Metrics) UpdateMemTableSize(bytes int64, entries int64) {
	m.MemTableSizeBytes.Set(float64(bytes))
	m.MemTableEntriesTotal.Set(float64(entries))
}

// RecordMemTableFlush records a memtable flush
func (m *Metrics) RecordMemTableFlush(duration float64) {
	m.MemTableFlushesTotal.Inc()
	m.MemTableFlushDuration.Observe(duration)
}

// UpdateSSTableStats updates SSTable statistics by level
func (m *Metrics) UpdateSSTableStats(level string, count int, sizeBytes int64) {
	m.SSTableCountByLevel.WithLabelValues(level).Set(float64(count))
	m.SSTableSizeByLevel.WithLabelValues(level).Set(float64(sizeBytes))
}

// RecordSSTableRead records an SSTable read
func (m *Metrics) RecordSSTableRead(duration float64) {
	m.SSTableReadsTotal.Inc()
	m.SSTableReadDuration.Observe(duration)
}

// RecordSSTableWrite records an SSTable write
func (m *Metrics) RecordSSTableWrite(duration float64) {
	m.SSTableWritesTotal.Inc()
	m.SSTableWriteDuration.Observe(duration)
}

// UpdateCommitLogStats updates commit log statistics
func (m *Metrics) UpdateCommitLogStats(segments int, sizeBytes int64) {
	m.CommitLogSegmentsTotal.Set(float64(segments))
	m.CommitLogSizeBytes.Set(float64(sizeBytes))
}

// RecordCommitLogAppend records a commit log append
func (m *Metrics) RecordCommitLogAppend(duration float64) {
	m.CommitLogAppendsTotal.Inc()
	m.CommitLogAppendDuration.Observe(duration)
}

// RecordCommitLogSync records a commit log sync
func (m *Metrics) RecordCommitLogSync(duration float64) {
	m.CommitLogSyncsTotal.Inc()
	m.CommitLogSyncDuration.Observe(duration)
}

// RecordCompactionJob records a compaction job
func (m *Metrics) RecordCompactionJob(status string, duration float64, inputTables, outputTables int, bytesProcessed, bytesWritten int64) {
	m.CompactionJobsTotal.WithLabelValues(status).Inc()
	m.CompactionJobDuration.Observe(duration)
	m.CompactionTablesInput.Observe(float64(inputTables))
	m.CompactionTablesOutput.Observe(float64(outputTables))
	m.CompactionBytesProcessed.Add(float64(bytesProcessed))
	m.CompactionBytesWritten.Add(float64(bytesWritten))
}

// UpdateGossipStats updates gossip statistics
func (m *Metrics) UpdateGossipStats(totalMembers, healthyMembers int) {
	m.GossipMembersTotal.Set(float64(totalMembers))
	m.GossipMembersHealthy.Set(float64(healthyMembers))
}

// RecordGossipMessage records a gossip message
func (m *Metrics) RecordGossipMessage(messageType string, duration float64) {
	m.GossipMessagesTotal.WithLabelValues(messageType).Inc()
	m.GossipMessagesDuration.Observe(duration)
}

// UpdateSystemStats updates system-level statistics
func (m *Metrics) UpdateSystemStats(diskUsage, diskAvailable, memoryUsage int64, goroutines int) {
	m.DiskUsageBytes.Set(float64(diskUsage))
	m.DiskAvailableBytes.Set(float64(diskAvailable))
	if diskUsage+diskAvailable > 0 {
		m.DiskUsagePercent.Set(float64(diskUsage) / float64(diskUsage+diskAvailable) * 100)
	}
	m.MemoryUsageBytes.Set(float64(memoryUsage))
	m.GoroutinesTotal.Set(float64(goroutines))
}
