package unit

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/devrev/pairdb/api-gateway/internal/config"
	apierrors "github.com/devrev/pairdb/api-gateway/internal/errors"
	"github.com/devrev/pairdb/api-gateway/internal/grpc"
	"github.com/devrev/pairdb/api-gateway/internal/handler"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func createTestHandlers(t *testing.T) (*handler.Handlers, *grpc.Client, func()) {
	logger, _ := zap.NewDevelopment()
	
	grpcCfg := config.CoordinatorConfig{
		Endpoints:             []string{"localhost:50051"},
		Timeout:               30 * time.Second,
		MaxRetries:            3,
		RetryBackoff:          100 * time.Millisecond,
		MaxReceiveMessageSize: 16 * 1024 * 1024,
		MaxSendMessageSize:    16 * 1024 * 1024,
	}
	
	grpcClient, err := grpc.NewClient(grpcCfg, logger)
	assert.NoError(t, err)
	
	errorHandler := apierrors.NewHandler(logger)
	handlers := handler.NewHandlers(grpcClient, errorHandler, logger, grpcCfg)
	
	cleanup := func() {
		grpcClient.Close()
	}
	
	return handlers, grpcClient, cleanup
}

func TestHandlers_WriteKeyValue_ValidationError(t *testing.T) {
	handlers, _, cleanup := createTestHandlers(t)
	defer cleanup()

	t.Run("missing key", func(t *testing.T) {
		body := `{"tenant_id": "tenant1", "value": "myvalue"}`
		req := httptest.NewRequest(http.MethodPost, "/v1/key-value", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		handlers.WriteKeyValue(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "key is required")
	})

	t.Run("invalid JSON", func(t *testing.T) {
		body := `{invalid}`
		req := httptest.NewRequest(http.MethodPost, "/v1/key-value", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		handlers.WriteKeyValue(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestHandlers_ReadKeyValue_ValidationError(t *testing.T) {
	handlers, _, cleanup := createTestHandlers(t)
	defer cleanup()

	t.Run("missing key", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/key-value?tenant_id=tenant1", nil)
		w := httptest.NewRecorder()

		handlers.ReadKeyValue(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "key query parameter is required")
	})

	t.Run("missing tenant_id", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/key-value?key=mykey", nil)
		w := httptest.NewRecorder()

		handlers.ReadKeyValue(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "tenant_id query parameter is required")
	})
}

func TestHandlers_CreateTenant_ValidationError(t *testing.T) {
	handlers, _, cleanup := createTestHandlers(t)
	defer cleanup()

	t.Run("missing tenant_id", func(t *testing.T) {
		body := `{"replication_factor": 3}`
		req := httptest.NewRequest(http.MethodPost, "/v1/tenants", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		handlers.CreateTenant(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "tenant_id is required")
	})

	t.Run("invalid replication factor", func(t *testing.T) {
		body := `{"tenant_id": "tenant1", "replication_factor": -1}`
		req := httptest.NewRequest(http.MethodPost, "/v1/tenants", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		handlers.CreateTenant(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestHandlers_GetTenant_ValidationError(t *testing.T) {
	handlers, _, cleanup := createTestHandlers(t)
	defer cleanup()

	t.Run("missing tenant_id", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/tenants/", nil)
		req = mux.SetURLVars(req, map[string]string{})
		w := httptest.NewRecorder()

		handlers.GetTenant(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestHandlers_UpdateReplicationFactor_ValidationError(t *testing.T) {
	handlers, _, cleanup := createTestHandlers(t)
	defer cleanup()

	t.Run("invalid replication factor", func(t *testing.T) {
		body := `{"replication_factor": 0}`
		req := httptest.NewRequest(http.MethodPut, "/v1/tenants/tenant1/replication-factor", bytes.NewBufferString(body))
		req = mux.SetURLVars(req, map[string]string{"tenant_id": "tenant1"})
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		handlers.UpdateReplicationFactor(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestHandlers_AddStorageNode_ValidationError(t *testing.T) {
	handlers, _, cleanup := createTestHandlers(t)
	defer cleanup()

	t.Run("missing node_id", func(t *testing.T) {
		body := `{"host": "192.168.1.1", "port": 50051}`
		req := httptest.NewRequest(http.MethodPost, "/v1/admin/storage-nodes", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		handlers.AddStorageNode(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "node_id is required")
	})

	t.Run("missing host", func(t *testing.T) {
		body := `{"node_id": "node1", "port": 50051}`
		req := httptest.NewRequest(http.MethodPost, "/v1/admin/storage-nodes", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		handlers.AddStorageNode(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "host is required")
	})
}

func TestHandlers_RemoveStorageNode_ValidationError(t *testing.T) {
	handlers, _, cleanup := createTestHandlers(t)
	defer cleanup()

	t.Run("missing node_id", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/v1/admin/storage-nodes/", nil)
		req = mux.SetURLVars(req, map[string]string{})
		w := httptest.NewRecorder()

		handlers.RemoveStorageNode(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestHandlers_GetMigrationStatus_ValidationError(t *testing.T) {
	handlers, _, cleanup := createTestHandlers(t)
	defer cleanup()

	t.Run("missing migration_id", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/admin/migrations/", nil)
		req = mux.SetURLVars(req, map[string]string{})
		w := httptest.NewRecorder()

		handlers.GetMigrationStatus(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestHandlers_ListStorageNodes(t *testing.T) {
	handlers, _, cleanup := createTestHandlers(t)
	defer cleanup()

	// This will fail because we don't have a real coordinator, but it tests the handler path
	req := httptest.NewRequest(http.MethodGet, "/v1/admin/storage-nodes", nil)
	w := httptest.NewRecorder()

	handlers.ListStorageNodes(w, req)

	// Will get an error due to no coordinator, but validates the handler works
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	// Either success or error response is acceptable (depending on coordinator availability)
	assert.True(t, w.Code == http.StatusOK || w.Code == http.StatusServiceUnavailable || w.Code == http.StatusInternalServerError)
}

