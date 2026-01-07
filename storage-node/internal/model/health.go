package model

// HealthStatus represents the health state of a storage node
type HealthStatus struct {
	NodeID    string
	Status    NodeStatus
	Timestamp int64
	Metrics   HealthMetrics
}

// NodeStatus defines the operational status of a node
type NodeStatus string

const (
	NodeStatusHealthy   NodeStatus = "healthy"
	NodeStatusDegraded  NodeStatus = "degraded"
	NodeStatusUnhealthy NodeStatus = "unhealthy"
)

// HealthMetrics contains various health metrics
type HealthMetrics struct {
	CPUUsage     float64
	MemoryUsage  float64
	DiskUsage    float64
	RequestRate  float64
	ErrorRate    float64
	CacheHitRate float64
}
