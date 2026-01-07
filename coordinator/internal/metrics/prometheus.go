package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics holds all Prometheus metrics
type Metrics struct {
	// Request metrics
	RequestsTotal   *prometheus.CounterVec
	RequestDuration *prometheus.HistogramVec
	RequestErrors   *prometheus.CounterVec

	// Cache metrics
	CacheHits   *prometheus.CounterVec
	CacheMisses *prometheus.CounterVec

	// Consistency metrics
	QuorumFailures *prometheus.CounterVec
	ConflictsTotal *prometheus.CounterVec

	// Repair metrics
	RepairsTotal     *prometheus.CounterVec
	RepairQueueSize  prometheus.Gauge
	RepairDuration   *prometheus.HistogramVec

	// Storage node metrics
	StorageNodesActive prometheus.Gauge
	ReplicaWrites      *prometheus.CounterVec
	ReplicaReads       *prometheus.CounterVec
}

// NewMetrics creates and registers Prometheus metrics
func NewMetrics() *Metrics {
	return &Metrics{
		RequestsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "coordinator_requests_total",
				Help: "Total number of requests processed",
			},
			[]string{"operation", "tenant_id", "consistency"},
		),

		RequestDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "coordinator_request_duration_seconds",
				Help:    "Duration of request processing",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"operation", "consistency"},
		),

		RequestErrors: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "coordinator_request_errors_total",
				Help: "Total number of request errors",
			},
			[]string{"operation", "error_type"},
		),

		CacheHits: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "coordinator_cache_hits_total",
				Help: "Total number of cache hits",
			},
			[]string{"cache_type"},
		),

		CacheMisses: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "coordinator_cache_misses_total",
				Help: "Total number of cache misses",
			},
			[]string{"cache_type"},
		),

		QuorumFailures: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "coordinator_quorum_failures_total",
				Help: "Total number of quorum failures",
			},
			[]string{"operation", "consistency"},
		),

		ConflictsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "coordinator_conflicts_total",
				Help: "Total number of conflicts detected",
			},
			[]string{"tenant_id"},
		),

		RepairsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "coordinator_repairs_total",
				Help: "Total number of repair operations",
			},
			[]string{"status"},
		),

		RepairQueueSize: promauto.NewGauge(
			prometheus.GaugeOpts{
				Name: "coordinator_repair_queue_size",
				Help: "Current size of the repair queue",
			},
		),

		RepairDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "coordinator_repair_duration_seconds",
				Help:    "Duration of repair operations",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"status"},
		),

		StorageNodesActive: promauto.NewGauge(
			prometheus.GaugeOpts{
				Name: "coordinator_storage_nodes_active",
				Help: "Number of active storage nodes",
			},
		),

		ReplicaWrites: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "coordinator_replica_writes_total",
				Help: "Total number of replica write operations",
			},
			[]string{"node_id", "status"},
		),

		ReplicaReads: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "coordinator_replica_reads_total",
				Help: "Total number of replica read operations",
			},
			[]string{"node_id", "status"},
		),
	}
}

// RecordRequest records a request metric
func (m *Metrics) RecordRequest(operation, tenantID, consistency string, duration float64) {
	m.RequestsTotal.WithLabelValues(operation, tenantID, consistency).Inc()
	m.RequestDuration.WithLabelValues(operation, consistency).Observe(duration)
}

// RecordError records an error metric
func (m *Metrics) RecordError(operation, errorType string) {
	m.RequestErrors.WithLabelValues(operation, errorType).Inc()
}

// RecordCacheHit records a cache hit
func (m *Metrics) RecordCacheHit(cacheType string) {
	m.CacheHits.WithLabelValues(cacheType).Inc()
}

// RecordCacheMiss records a cache miss
func (m *Metrics) RecordCacheMiss(cacheType string) {
	m.CacheMisses.WithLabelValues(cacheType).Inc()
}

// RecordQuorumFailure records a quorum failure
func (m *Metrics) RecordQuorumFailure(operation, consistency string) {
	m.QuorumFailures.WithLabelValues(operation, consistency).Inc()
}

// RecordConflict records a conflict detection
func (m *Metrics) RecordConflict(tenantID string) {
	m.ConflictsTotal.WithLabelValues(tenantID).Inc()
}

// RecordRepair records a repair operation
func (m *Metrics) RecordRepair(status string, duration float64) {
	m.RepairsTotal.WithLabelValues(status).Inc()
	m.RepairDuration.WithLabelValues(status).Observe(duration)
}

// UpdateRepairQueueSize updates the repair queue size
func (m *Metrics) UpdateRepairQueueSize(size int) {
	m.RepairQueueSize.Set(float64(size))
}

// UpdateStorageNodesActive updates the active storage nodes count
func (m *Metrics) UpdateStorageNodesActive(count int) {
	m.StorageNodesActive.Set(float64(count))
}

// RecordReplicaWrite records a replica write operation
func (m *Metrics) RecordReplicaWrite(nodeID, status string) {
	m.ReplicaWrites.WithLabelValues(nodeID, status).Inc()
}

// RecordReplicaRead records a replica read operation
func (m *Metrics) RecordReplicaRead(nodeID, status string) {
	m.ReplicaReads.WithLabelValues(nodeID, status).Inc()
}
