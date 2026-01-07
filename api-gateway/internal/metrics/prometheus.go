// Package metrics provides Prometheus metrics for the API Gateway.
package metrics

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
)

// Metrics holds all Prometheus metrics.
type Metrics struct {
	requestsTotal     *prometheus.CounterVec
	requestDuration   *prometheus.HistogramVec
	requestsInFlight  prometheus.Gauge
	responseSize      *prometheus.HistogramVec
	grpcRequestsTotal *prometheus.CounterVec
	grpcRequestDuration *prometheus.HistogramVec
	grpcErrors        *prometheus.CounterVec
	healthStatus      prometheus.Gauge
}

var globalMetrics *Metrics

// NewMetrics creates and registers Prometheus metrics.
func NewMetrics() *Metrics {
	if globalMetrics != nil {
		return globalMetrics
	}

	globalMetrics = &Metrics{
		requestsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "api_gateway_http_requests_total",
				Help: "Total number of HTTP requests",
			},
			[]string{"method", "path", "status"},
		),
		requestDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "api_gateway_http_request_duration_seconds",
				Help:    "HTTP request duration in seconds",
				Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
			},
			[]string{"method", "path", "status"},
		),
		requestsInFlight: promauto.NewGauge(
			prometheus.GaugeOpts{
				Name: "api_gateway_http_requests_in_flight",
				Help: "Number of HTTP requests currently being processed",
			},
		),
		responseSize: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "api_gateway_http_response_size_bytes",
				Help:    "HTTP response size in bytes",
				Buckets: []float64{100, 500, 1000, 5000, 10000, 50000, 100000},
			},
			[]string{"method", "path"},
		),
		grpcRequestsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "api_gateway_grpc_requests_total",
				Help: "Total number of gRPC requests to coordinator",
			},
			[]string{"method", "status"},
		),
		grpcRequestDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "api_gateway_grpc_request_duration_seconds",
				Help:    "gRPC request duration in seconds",
				Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
			},
			[]string{"method"},
		),
		grpcErrors: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "api_gateway_grpc_errors_total",
				Help: "Total number of gRPC errors",
			},
			[]string{"method", "code"},
		),
		healthStatus: promauto.NewGauge(
			prometheus.GaugeOpts{
				Name: "api_gateway_health_status",
				Help: "Health status of the API Gateway (1 = healthy, 0 = unhealthy)",
			},
		),
	}

	return globalMetrics
}

// RecordHTTPRequest records metrics for an HTTP request.
func (m *Metrics) RecordHTTPRequest(method, path string, statusCode int, duration time.Duration) {
	status := strconv.Itoa(statusCode)
	m.requestsTotal.WithLabelValues(method, path, status).Inc()
	m.requestDuration.WithLabelValues(method, path, status).Observe(duration.Seconds())
}

// RecordResponseSize records the response size.
func (m *Metrics) RecordResponseSize(method, path string, size int) {
	m.responseSize.WithLabelValues(method, path).Observe(float64(size))
}

// IncRequestsInFlight increments the in-flight requests counter.
func (m *Metrics) IncRequestsInFlight() {
	m.requestsInFlight.Inc()
}

// DecRequestsInFlight decrements the in-flight requests counter.
func (m *Metrics) DecRequestsInFlight() {
	m.requestsInFlight.Dec()
}

// RecordGRPCRequest records metrics for a gRPC request.
func (m *Metrics) RecordGRPCRequest(method, status string, duration time.Duration) {
	m.grpcRequestsTotal.WithLabelValues(method, status).Inc()
	m.grpcRequestDuration.WithLabelValues(method).Observe(duration.Seconds())
}

// RecordGRPCError records a gRPC error.
func (m *Metrics) RecordGRPCError(method, code string) {
	m.grpcErrors.WithLabelValues(method, code).Inc()
}

// SetHealthStatus sets the health status.
func (m *Metrics) SetHealthStatus(healthy bool) {
	if healthy {
		m.healthStatus.Set(1)
	} else {
		m.healthStatus.Set(0)
	}
}

// MetricsServer provides a separate HTTP server for Prometheus metrics.
type MetricsServer struct {
	server *http.Server
	logger *zap.Logger
}

// NewMetricsServer creates a new metrics server.
func NewMetricsServer(port int, path string, logger *zap.Logger) *MetricsServer {
	mux := http.NewServeMux()
	mux.Handle(path, promhttp.Handler())

	return &MetricsServer{
		server: &http.Server{
			Addr:    fmt.Sprintf(":%d", port),
			Handler: mux,
		},
		logger: logger,
	}
}

// Start starts the metrics server.
func (ms *MetricsServer) Start() error {
	ms.logger.Info("starting metrics server", zap.String("addr", ms.server.Addr))
	return ms.server.ListenAndServe()
}

// Shutdown gracefully shuts down the metrics server.
func (ms *MetricsServer) Shutdown(ctx context.Context) error {
	return ms.server.Shutdown(ctx)
}

// MetricsMiddleware creates middleware that records HTTP metrics.
func MetricsMiddleware(m *Metrics) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			m.IncRequestsInFlight()
			defer m.DecRequestsInFlight()

			start := time.Now()
			rw := &metricsResponseWriter{ResponseWriter: w, statusCode: http.StatusOK}

			next.ServeHTTP(rw, r)

			duration := time.Since(start)
			m.RecordHTTPRequest(r.Method, r.URL.Path, rw.statusCode, duration)
			m.RecordResponseSize(r.Method, r.URL.Path, rw.size)
		})
	}
}

// metricsResponseWriter wraps http.ResponseWriter to capture metrics.
type metricsResponseWriter struct {
	http.ResponseWriter
	statusCode int
	size       int
}

// WriteHeader captures the status code.
func (rw *metricsResponseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// Write captures the response size.
func (rw *metricsResponseWriter) Write(b []byte) (int, error) {
	size, err := rw.ResponseWriter.Write(b)
	rw.size += size
	return size, err
}

