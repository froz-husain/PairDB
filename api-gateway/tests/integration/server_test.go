package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/devrev/pairdb/api-gateway/internal/config"
	"github.com/devrev/pairdb/api-gateway/internal/converter"
	apierrors "github.com/devrev/pairdb/api-gateway/internal/errors"
	"github.com/devrev/pairdb/api-gateway/internal/handler"
	"github.com/devrev/pairdb/api-gateway/internal/middleware"
	pb "github.com/devrev/pairdb/api-gateway/pkg/proto"
	"github.com/devrev/pairdb/api-gateway/tests/mocks"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// TestHandlers wraps handlers for integration testing with mock gRPC client.
type TestHandlers struct {
	mockClient     *mocks.MockCoordinatorServiceClient
	handlers       *TestableHandlers
	router         *mux.Router
	logger         *zap.Logger
	errorHandler   *apierrors.Handler
}

// TestableHandlers is a handler wrapper for testing.
type TestableHandlers struct {
	mockClient   *mocks.MockCoordinatorServiceClient
	httpToGRPC   *converter.HTTPToGRPC
	grpcToHTTP   *converter.GRPCToHTTP
	errorHandler *apierrors.Handler
	logger       *zap.Logger
	timeout      time.Duration
}

func NewTestHandlers() *TestHandlers {
	logger, _ := zap.NewDevelopment()
	mockClient := mocks.NewMockCoordinatorServiceClient()
	errorHandler := apierrors.NewHandler(logger)

	handlers := &TestableHandlers{
		mockClient:   mockClient,
		httpToGRPC:   converter.NewHTTPToGRPC(),
		grpcToHTTP:   converter.NewGRPCToHTTP(),
		errorHandler: errorHandler,
		logger:       logger,
		timeout:      30 * time.Second,
	}

	router := mux.NewRouter()
	
	// Setup routes
	router.Use(middleware.RequestID)
	
	v1 := router.PathPrefix("/v1").Subrouter()
	v1.HandleFunc("/key-value", handlers.WriteKeyValue).Methods(http.MethodPost)
	v1.HandleFunc("/key-value", handlers.ReadKeyValue).Methods(http.MethodGet)
	v1.HandleFunc("/tenants", handlers.CreateTenant).Methods(http.MethodPost)
	v1.HandleFunc("/tenants/{tenant_id}", handlers.GetTenant).Methods(http.MethodGet)
	v1.HandleFunc("/tenants/{tenant_id}/replication-factor", handlers.UpdateReplicationFactor).Methods(http.MethodPut)
	v1.HandleFunc("/admin/storage-nodes", handlers.AddStorageNode).Methods(http.MethodPost)
	v1.HandleFunc("/admin/storage-nodes", handlers.ListStorageNodes).Methods(http.MethodGet)
	v1.HandleFunc("/admin/storage-nodes/{node_id}", handlers.RemoveStorageNode).Methods(http.MethodDelete)
	v1.HandleFunc("/admin/migrations/{migration_id}", handlers.GetMigrationStatus).Methods(http.MethodGet)

	return &TestHandlers{
		mockClient:   mockClient,
		handlers:     handlers,
		router:       router,
		logger:       logger,
		errorHandler: errorHandler,
	}
}

// Handler implementations for testing
func (h *TestableHandlers) WriteKeyValue(w http.ResponseWriter, r *http.Request) {
	requestID := r.Header.Get("X-Request-ID")
	grpcReq, err := h.httpToGRPC.WriteKeyValueRequest(r)
	if err != nil {
		h.errorHandler.WriteValidationError(w, err.Error(), requestID)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()

	grpcResp, err := h.mockClient.WriteKeyValue(ctx, grpcReq)
	if err != nil {
		h.errorHandler.HandleError(w, r, err)
		return
	}

	if !grpcResp.Success {
		h.errorHandler.WriteErrorResponse(w, http.StatusBadRequest, apierrors.ErrorCodeInvalidRequest, grpcResp.ErrorMessage, requestID)
		return
	}

	httpResp := h.grpcToHTTP.WriteKeyValueResponse(grpcResp)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(httpResp)
}

func (h *TestableHandlers) ReadKeyValue(w http.ResponseWriter, r *http.Request) {
	requestID := r.Header.Get("X-Request-ID")
	grpcReq, err := h.httpToGRPC.ReadKeyValueRequest(r)
	if err != nil {
		h.errorHandler.WriteValidationError(w, err.Error(), requestID)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()

	grpcResp, err := h.mockClient.ReadKeyValue(ctx, grpcReq)
	if err != nil {
		h.errorHandler.HandleError(w, r, err)
		return
	}

	if !grpcResp.Success {
		h.errorHandler.WriteErrorResponse(w, http.StatusNotFound, apierrors.ErrorCodeKeyNotFound, grpcResp.ErrorMessage, requestID)
		return
	}

	httpResp := h.grpcToHTTP.ReadKeyValueResponse(grpcResp)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(httpResp)
}

func (h *TestableHandlers) CreateTenant(w http.ResponseWriter, r *http.Request) {
	requestID := r.Header.Get("X-Request-ID")
	grpcReq, err := h.httpToGRPC.CreateTenantRequest(r)
	if err != nil {
		h.errorHandler.WriteValidationError(w, err.Error(), requestID)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()

	grpcResp, err := h.mockClient.CreateTenant(ctx, grpcReq)
	if err != nil {
		h.errorHandler.HandleError(w, r, err)
		return
	}

	if !grpcResp.Success {
		h.errorHandler.WriteErrorResponse(w, http.StatusConflict, apierrors.ErrorCodeTenantExists, grpcResp.ErrorMessage, requestID)
		return
	}

	httpResp := h.grpcToHTTP.CreateTenantResponse(grpcResp)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(httpResp)
}

func (h *TestableHandlers) GetTenant(w http.ResponseWriter, r *http.Request) {
	requestID := r.Header.Get("X-Request-ID")
	grpcReq, err := h.httpToGRPC.GetTenantRequest(r)
	if err != nil {
		h.errorHandler.WriteValidationError(w, err.Error(), requestID)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()

	grpcResp, err := h.mockClient.GetTenant(ctx, grpcReq)
	if err != nil {
		h.errorHandler.HandleError(w, r, err)
		return
	}

	if !grpcResp.Success {
		h.errorHandler.WriteErrorResponse(w, http.StatusNotFound, apierrors.ErrorCodeTenantNotFound, grpcResp.ErrorMessage, requestID)
		return
	}

	httpResp := h.grpcToHTTP.GetTenantResponse(grpcResp)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(httpResp)
}

func (h *TestableHandlers) UpdateReplicationFactor(w http.ResponseWriter, r *http.Request) {
	requestID := r.Header.Get("X-Request-ID")
	grpcReq, err := h.httpToGRPC.UpdateReplicationFactorRequest(r)
	if err != nil {
		h.errorHandler.WriteValidationError(w, err.Error(), requestID)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()

	grpcResp, err := h.mockClient.UpdateReplicationFactor(ctx, grpcReq)
	if err != nil {
		h.errorHandler.HandleError(w, r, err)
		return
	}

	if !grpcResp.Success {
		h.errorHandler.WriteErrorResponse(w, http.StatusBadRequest, apierrors.ErrorCodeInvalidReplicationFactor, grpcResp.ErrorMessage, requestID)
		return
	}

	httpResp := h.grpcToHTTP.UpdateReplicationFactorResponse(grpcResp)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(httpResp)
}

func (h *TestableHandlers) AddStorageNode(w http.ResponseWriter, r *http.Request) {
	requestID := r.Header.Get("X-Request-ID")
	grpcReq, err := h.httpToGRPC.AddStorageNodeRequest(r)
	if err != nil {
		h.errorHandler.WriteValidationError(w, err.Error(), requestID)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()

	grpcResp, err := h.mockClient.AddStorageNode(ctx, grpcReq)
	if err != nil {
		h.errorHandler.HandleError(w, r, err)
		return
	}

	if !grpcResp.Success {
		h.errorHandler.WriteErrorResponse(w, http.StatusBadRequest, apierrors.ErrorCodeInvalidRequest, grpcResp.ErrorMessage, requestID)
		return
	}

	httpResp := h.grpcToHTTP.AddStorageNodeResponse(grpcResp)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(httpResp)
}

func (h *TestableHandlers) RemoveStorageNode(w http.ResponseWriter, r *http.Request) {
	requestID := r.Header.Get("X-Request-ID")
	grpcReq, err := h.httpToGRPC.RemoveStorageNodeRequest(r)
	if err != nil {
		h.errorHandler.WriteValidationError(w, err.Error(), requestID)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()

	grpcResp, err := h.mockClient.RemoveStorageNode(ctx, grpcReq)
	if err != nil {
		h.errorHandler.HandleError(w, r, err)
		return
	}

	if !grpcResp.Success {
		h.errorHandler.WriteErrorResponse(w, http.StatusBadRequest, apierrors.ErrorCodeNodeInUse, grpcResp.ErrorMessage, requestID)
		return
	}

	httpResp := h.grpcToHTTP.RemoveStorageNodeResponse(grpcResp)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(httpResp)
}

func (h *TestableHandlers) GetMigrationStatus(w http.ResponseWriter, r *http.Request) {
	requestID := r.Header.Get("X-Request-ID")
	grpcReq, err := h.httpToGRPC.GetMigrationStatusRequest(r)
	if err != nil {
		h.errorHandler.WriteValidationError(w, err.Error(), requestID)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()

	grpcResp, err := h.mockClient.GetMigrationStatus(ctx, grpcReq)
	if err != nil {
		h.errorHandler.HandleError(w, r, err)
		return
	}

	if !grpcResp.Success {
		h.errorHandler.WriteErrorResponse(w, http.StatusNotFound, apierrors.ErrorCodeMigrationNotFound, grpcResp.ErrorMessage, requestID)
		return
	}

	httpResp := h.grpcToHTTP.GetMigrationStatusResponse(grpcResp)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(httpResp)
}

func (h *TestableHandlers) ListStorageNodes(w http.ResponseWriter, r *http.Request) {
	requestID := r.Header.Get("X-Request-ID")
	grpcReq, err := h.httpToGRPC.ListStorageNodesRequest(r)
	if err != nil {
		h.errorHandler.WriteValidationError(w, err.Error(), requestID)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()

	grpcResp, err := h.mockClient.ListStorageNodes(ctx, grpcReq)
	if err != nil {
		h.errorHandler.HandleError(w, r, err)
		return
	}

	if !grpcResp.Success {
		h.errorHandler.WriteErrorResponse(w, http.StatusInternalServerError, apierrors.ErrorCodeInternalError, grpcResp.ErrorMessage, requestID)
		return
	}

	httpResp := h.grpcToHTTP.ListStorageNodesResponse(grpcResp)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(httpResp)
}

// Integration Tests

func TestIntegration_WriteKeyValue(t *testing.T) {
	th := NewTestHandlers()

	t.Run("successful write", func(t *testing.T) {
		th.mockClient.On("WriteKeyValue", mock.Anything, mock.MatchedBy(func(req *pb.WriteKeyValueRequest) bool {
			return req.TenantId == "tenant1" && req.Key == "mykey"
		})).Return(&pb.WriteKeyValueResponse{
			Success:        true,
			Key:            "mykey",
			IdempotencyKey: "idem-123",
			ReplicaCount:   3,
			Consistency:    "quorum",
		}, nil).Once()

		body := `{"tenant_id": "tenant1", "key": "mykey", "value": "myvalue"}`
		req := httptest.NewRequest(http.MethodPost, "/v1/key-value", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", "idem-123")
		w := httptest.NewRecorder()

		th.router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		
		var resp converter.WriteKeyValueHTTPResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, "success", resp.Status)
		assert.Equal(t, "mykey", resp.Key)
	})

	t.Run("validation error - missing key", func(t *testing.T) {
		body := `{"tenant_id": "tenant1", "value": "myvalue"}`
		req := httptest.NewRequest(http.MethodPost, "/v1/key-value", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		th.router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "key is required")
	})
}

func TestIntegration_ReadKeyValue(t *testing.T) {
	th := NewTestHandlers()

	t.Run("successful read", func(t *testing.T) {
		th.mockClient.On("ReadKeyValue", mock.Anything, mock.MatchedBy(func(req *pb.ReadKeyValueRequest) bool {
			return req.TenantId == "tenant1" && req.Key == "mykey"
		})).Return(&pb.ReadKeyValueResponse{
			Success: true,
			Key:     "mykey",
			Value:   []byte("myvalue"),
		}, nil).Once()

		req := httptest.NewRequest(http.MethodGet, "/v1/key-value?tenant_id=tenant1&key=mykey", nil)
		w := httptest.NewRecorder()

		th.router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		
		var resp converter.ReadKeyValueHTTPResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, "success", resp.Status)
		assert.Equal(t, "mykey", resp.Key)
		assert.Equal(t, "myvalue", resp.Value)
	})

	t.Run("key not found", func(t *testing.T) {
		th.mockClient.On("ReadKeyValue", mock.Anything, mock.Anything).Return(&pb.ReadKeyValueResponse{
			Success:      false,
			ErrorMessage: "key not found",
		}, nil).Once()

		req := httptest.NewRequest(http.MethodGet, "/v1/key-value?tenant_id=tenant1&key=nonexistent", nil)
		w := httptest.NewRecorder()

		th.router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestIntegration_CreateTenant(t *testing.T) {
	th := NewTestHandlers()

	t.Run("successful creation", func(t *testing.T) {
		th.mockClient.On("CreateTenant", mock.Anything, mock.MatchedBy(func(req *pb.CreateTenantRequest) bool {
			return req.TenantId == "new-tenant"
		})).Return(&pb.CreateTenantResponse{
			Success:           true,
			TenantId:          "new-tenant",
			ReplicationFactor: 3,
			CreatedAt:         1609459200,
		}, nil).Once()

		body := `{"tenant_id": "new-tenant", "replication_factor": 3}`
		req := httptest.NewRequest(http.MethodPost, "/v1/tenants", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		th.router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusCreated, w.Code)
		
		var resp converter.CreateTenantHTTPResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, "success", resp.Status)
		assert.Equal(t, "new-tenant", resp.TenantID)
	})
}

func TestIntegration_GetTenant(t *testing.T) {
	th := NewTestHandlers()

	t.Run("successful get", func(t *testing.T) {
		th.mockClient.On("GetTenant", mock.Anything, mock.MatchedBy(func(req *pb.GetTenantRequest) bool {
			return req.TenantId == "tenant1"
		})).Return(&pb.GetTenantResponse{
			Success:           true,
			TenantId:          "tenant1",
			ReplicationFactor: 3,
			CreatedAt:         1609459200,
			UpdatedAt:         1609459300,
		}, nil).Once()

		req := httptest.NewRequest(http.MethodGet, "/v1/tenants/tenant1", nil)
		w := httptest.NewRecorder()

		th.router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})
}

func TestIntegration_ListStorageNodes(t *testing.T) {
	th := NewTestHandlers()

	t.Run("successful list", func(t *testing.T) {
		th.mockClient.On("ListStorageNodes", mock.Anything, mock.Anything).Return(&pb.ListStorageNodesResponse{
			Success: true,
			Nodes: []*pb.StorageNodeInfo{
				{
					NodeId: "node1",
					Host:   "192.168.1.1",
					Port:   50051,
					Status: "active",
				},
			},
		}, nil).Once()

		req := httptest.NewRequest(http.MethodGet, "/v1/admin/storage-nodes", nil)
		w := httptest.NewRecorder()

		th.router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		
		var resp converter.ListStorageNodesHTTPResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Len(t, resp.Nodes, 1)
	})
}

// Ignore unused variable warning for config
var _ = config.Config{}
var _ = handler.Handlers{}

