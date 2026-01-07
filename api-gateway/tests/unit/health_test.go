package unit

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/devrev/pairdb/api-gateway/internal/health"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// MockGRPCClient is a mock for the gRPC client for health check testing.
type MockGRPCClient struct {
	healthErr error
}

func (m *MockGRPCClient) HealthCheck() error {
	return m.healthErr
}

func TestHealthCheck_LivenessHandler(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	hc := health.NewHealthCheck(nil, logger)

	t.Run("returns healthy", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		w := httptest.NewRecorder()

		hc.LivenessHandler(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp health.LivenessResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, "healthy", resp.Status)
	})
}

func TestHealthCheck_SetReady(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	hc := health.NewHealthCheck(nil, logger)

	t.Run("initially not ready", func(t *testing.T) {
		assert.False(t, hc.IsReady())
	})

	t.Run("can set ready", func(t *testing.T) {
		hc.SetReady(true)
		assert.True(t, hc.IsReady())
	})

	t.Run("can set not ready", func(t *testing.T) {
		hc.SetReady(false)
		assert.False(t, hc.IsReady())
	})
}

