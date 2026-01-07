package unit

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/devrev/pairdb/api-gateway/internal/metrics"
	"github.com/stretchr/testify/assert"
)

func TestMetrics_RecordHTTPRequest(t *testing.T) {
	m := metrics.NewMetrics()

	// Just verify it doesn't panic
	m.RecordHTTPRequest("GET", "/v1/key-value", 200, 100*time.Millisecond)
	m.RecordHTTPRequest("POST", "/v1/key-value", 400, 50*time.Millisecond)
	m.RecordHTTPRequest("GET", "/v1/tenants/123", 404, 30*time.Millisecond)
}

func TestMetrics_RecordResponseSize(t *testing.T) {
	m := metrics.NewMetrics()

	// Just verify it doesn't panic
	m.RecordResponseSize("GET", "/v1/key-value", 1024)
	m.RecordResponseSize("POST", "/v1/tenants", 256)
}

func TestMetrics_RequestsInFlight(t *testing.T) {
	m := metrics.NewMetrics()

	// Just verify it doesn't panic
	m.IncRequestsInFlight()
	m.IncRequestsInFlight()
	m.DecRequestsInFlight()
	m.DecRequestsInFlight()
}

func TestMetrics_RecordGRPCRequest(t *testing.T) {
	m := metrics.NewMetrics()

	// Just verify it doesn't panic
	m.RecordGRPCRequest("WriteKeyValue", "OK", 50*time.Millisecond)
	m.RecordGRPCRequest("ReadKeyValue", "OK", 30*time.Millisecond)
	m.RecordGRPCRequest("CreateTenant", "AlreadyExists", 10*time.Millisecond)
}

func TestMetrics_RecordGRPCError(t *testing.T) {
	m := metrics.NewMetrics()

	// Just verify it doesn't panic
	m.RecordGRPCError("WriteKeyValue", "Unavailable")
	m.RecordGRPCError("ReadKeyValue", "NotFound")
}

func TestMetrics_SetHealthStatus(t *testing.T) {
	m := metrics.NewMetrics()

	// Just verify it doesn't panic
	m.SetHealthStatus(true)
	m.SetHealthStatus(false)
}

func TestMetricsMiddleware(t *testing.T) {
	m := metrics.NewMetrics()

	handler := metrics.MetricsMiddleware(m)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestMetricsMiddleware_CapturesStatusCode(t *testing.T) {
	m := metrics.NewMetrics()

	handler := metrics.MetricsMiddleware(m)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestMetricsMiddleware_CapturesResponseSize(t *testing.T) {
	m := metrics.NewMetrics()

	handler := metrics.MetricsMiddleware(m)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Hello, World!"))
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "Hello, World!", w.Body.String())
}

