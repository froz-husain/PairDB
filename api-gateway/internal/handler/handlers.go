// Package handler provides HTTP request handlers for the API Gateway.
package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/devrev/pairdb/api-gateway/internal/config"
	"github.com/devrev/pairdb/api-gateway/internal/converter"
	apierrors "github.com/devrev/pairdb/api-gateway/internal/errors"
	"github.com/devrev/pairdb/api-gateway/internal/grpc"
	"go.uber.org/zap"
)

// Handlers contains all HTTP handlers and their dependencies.
type Handlers struct {
	grpcClient     *grpc.Client
	httpToGRPC     *converter.HTTPToGRPC
	grpcToHTTP     *converter.GRPCToHTTP
	errorHandler   *apierrors.Handler
	logger         *zap.Logger
	timeout        time.Duration
}

// NewHandlers creates a new Handlers instance.
func NewHandlers(
	grpcClient *grpc.Client,
	errorHandler *apierrors.Handler,
	logger *zap.Logger,
	cfg config.CoordinatorConfig,
) *Handlers {
	return &Handlers{
		grpcClient:   grpcClient,
		httpToGRPC:   converter.NewHTTPToGRPC(),
		grpcToHTTP:   converter.NewGRPCToHTTP(),
		errorHandler: errorHandler,
		logger:       logger,
		timeout:      cfg.Timeout,
	}
}

// WriteKeyValue handles POST /v1/key-value requests.
func (h *Handlers) WriteKeyValue(w http.ResponseWriter, r *http.Request) {
	requestID := r.Header.Get("X-Request-ID")

	// Convert HTTP request to gRPC request
	grpcReq, err := h.httpToGRPC.WriteKeyValueRequest(r)
	if err != nil {
		h.errorHandler.WriteValidationError(w, err.Error(), requestID)
		return
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()

	// Call gRPC client
	grpcResp, err := h.grpcClient.WriteKeyValue(ctx, grpcReq)
	if err != nil {
		h.errorHandler.HandleError(w, r, err)
		return
	}

	// Check for application-level error
	if !grpcResp.Success {
		h.errorHandler.WriteErrorResponse(w, http.StatusBadRequest, apierrors.ErrorCodeInvalidRequest, grpcResp.ErrorMessage, requestID)
		return
	}

	// Convert gRPC response to HTTP response
	httpResp := h.grpcToHTTP.WriteKeyValueResponse(grpcResp)

	// Write HTTP response
	h.writeJSONResponse(w, http.StatusOK, httpResp)
}

// ReadKeyValue handles GET /v1/key-value requests.
func (h *Handlers) ReadKeyValue(w http.ResponseWriter, r *http.Request) {
	requestID := r.Header.Get("X-Request-ID")

	// Convert HTTP request to gRPC request
	grpcReq, err := h.httpToGRPC.ReadKeyValueRequest(r)
	if err != nil {
		h.errorHandler.WriteValidationError(w, err.Error(), requestID)
		return
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()

	// Call gRPC client
	grpcResp, err := h.grpcClient.ReadKeyValue(ctx, grpcReq)
	if err != nil {
		h.errorHandler.HandleError(w, r, err)
		return
	}

	// Check for application-level error
	if !grpcResp.Success {
		h.errorHandler.WriteErrorResponse(w, http.StatusNotFound, apierrors.ErrorCodeKeyNotFound, grpcResp.ErrorMessage, requestID)
		return
	}

	// Convert gRPC response to HTTP response
	httpResp := h.grpcToHTTP.ReadKeyValueResponse(grpcResp)

	// Write HTTP response
	h.writeJSONResponse(w, http.StatusOK, httpResp)
}

// CreateTenant handles POST /v1/tenants requests.
func (h *Handlers) CreateTenant(w http.ResponseWriter, r *http.Request) {
	requestID := r.Header.Get("X-Request-ID")

	// Convert HTTP request to gRPC request
	grpcReq, err := h.httpToGRPC.CreateTenantRequest(r)
	if err != nil {
		h.errorHandler.WriteValidationError(w, err.Error(), requestID)
		return
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()

	// Call gRPC client
	grpcResp, err := h.grpcClient.CreateTenant(ctx, grpcReq)
	if err != nil {
		h.errorHandler.HandleError(w, r, err)
		return
	}

	// Check for application-level error
	if !grpcResp.Success {
		h.errorHandler.WriteErrorResponse(w, http.StatusConflict, apierrors.ErrorCodeTenantExists, grpcResp.ErrorMessage, requestID)
		return
	}

	// Convert gRPC response to HTTP response
	httpResp := h.grpcToHTTP.CreateTenantResponse(grpcResp)

	// Write HTTP response
	h.writeJSONResponse(w, http.StatusCreated, httpResp)
}

// UpdateReplicationFactor handles PUT /v1/tenants/{tenant_id}/replication-factor requests.
func (h *Handlers) UpdateReplicationFactor(w http.ResponseWriter, r *http.Request) {
	requestID := r.Header.Get("X-Request-ID")

	// Convert HTTP request to gRPC request
	grpcReq, err := h.httpToGRPC.UpdateReplicationFactorRequest(r)
	if err != nil {
		h.errorHandler.WriteValidationError(w, err.Error(), requestID)
		return
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()

	// Call gRPC client
	grpcResp, err := h.grpcClient.UpdateReplicationFactor(ctx, grpcReq)
	if err != nil {
		h.errorHandler.HandleError(w, r, err)
		return
	}

	// Check for application-level error
	if !grpcResp.Success {
		h.errorHandler.WriteErrorResponse(w, http.StatusBadRequest, apierrors.ErrorCodeInvalidReplicationFactor, grpcResp.ErrorMessage, requestID)
		return
	}

	// Convert gRPC response to HTTP response
	httpResp := h.grpcToHTTP.UpdateReplicationFactorResponse(grpcResp)

	// Write HTTP response
	h.writeJSONResponse(w, http.StatusOK, httpResp)
}

// GetTenant handles GET /v1/tenants/{tenant_id} requests.
func (h *Handlers) GetTenant(w http.ResponseWriter, r *http.Request) {
	requestID := r.Header.Get("X-Request-ID")

	// Convert HTTP request to gRPC request
	grpcReq, err := h.httpToGRPC.GetTenantRequest(r)
	if err != nil {
		h.errorHandler.WriteValidationError(w, err.Error(), requestID)
		return
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()

	// Call gRPC client
	grpcResp, err := h.grpcClient.GetTenant(ctx, grpcReq)
	if err != nil {
		h.errorHandler.HandleError(w, r, err)
		return
	}

	// Check for application-level error
	if !grpcResp.Success {
		h.errorHandler.WriteErrorResponse(w, http.StatusNotFound, apierrors.ErrorCodeTenantNotFound, grpcResp.ErrorMessage, requestID)
		return
	}

	// Convert gRPC response to HTTP response
	httpResp := h.grpcToHTTP.GetTenantResponse(grpcResp)

	// Write HTTP response
	h.writeJSONResponse(w, http.StatusOK, httpResp)
}

// AddStorageNode handles POST /v1/admin/storage-nodes requests.
func (h *Handlers) AddStorageNode(w http.ResponseWriter, r *http.Request) {
	requestID := r.Header.Get("X-Request-ID")

	// Convert HTTP request to gRPC request
	grpcReq, err := h.httpToGRPC.AddStorageNodeRequest(r)
	if err != nil {
		h.errorHandler.WriteValidationError(w, err.Error(), requestID)
		return
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()

	// Call gRPC client
	grpcResp, err := h.grpcClient.AddStorageNode(ctx, grpcReq)
	if err != nil {
		h.errorHandler.HandleError(w, r, err)
		return
	}

	// Check for application-level error
	if !grpcResp.Success {
		h.errorHandler.WriteErrorResponse(w, http.StatusBadRequest, apierrors.ErrorCodeInvalidRequest, grpcResp.ErrorMessage, requestID)
		return
	}

	// Convert gRPC response to HTTP response
	httpResp := h.grpcToHTTP.AddStorageNodeResponse(grpcResp)

	// Write HTTP response
	h.writeJSONResponse(w, http.StatusAccepted, httpResp)
}

// RemoveStorageNode handles DELETE /v1/admin/storage-nodes/{node_id} requests.
func (h *Handlers) RemoveStorageNode(w http.ResponseWriter, r *http.Request) {
	requestID := r.Header.Get("X-Request-ID")

	// Convert HTTP request to gRPC request
	grpcReq, err := h.httpToGRPC.RemoveStorageNodeRequest(r)
	if err != nil {
		h.errorHandler.WriteValidationError(w, err.Error(), requestID)
		return
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()

	// Call gRPC client
	grpcResp, err := h.grpcClient.RemoveStorageNode(ctx, grpcReq)
	if err != nil {
		h.errorHandler.HandleError(w, r, err)
		return
	}

	// Check for application-level error
	if !grpcResp.Success {
		h.errorHandler.WriteErrorResponse(w, http.StatusBadRequest, apierrors.ErrorCodeNodeInUse, grpcResp.ErrorMessage, requestID)
		return
	}

	// Convert gRPC response to HTTP response
	httpResp := h.grpcToHTTP.RemoveStorageNodeResponse(grpcResp)

	// Write HTTP response
	h.writeJSONResponse(w, http.StatusAccepted, httpResp)
}

// GetMigrationStatus handles GET /v1/admin/migrations/{migration_id} requests.
func (h *Handlers) GetMigrationStatus(w http.ResponseWriter, r *http.Request) {
	requestID := r.Header.Get("X-Request-ID")

	// Convert HTTP request to gRPC request
	grpcReq, err := h.httpToGRPC.GetMigrationStatusRequest(r)
	if err != nil {
		h.errorHandler.WriteValidationError(w, err.Error(), requestID)
		return
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()

	// Call gRPC client
	grpcResp, err := h.grpcClient.GetMigrationStatus(ctx, grpcReq)
	if err != nil {
		h.errorHandler.HandleError(w, r, err)
		return
	}

	// Check for application-level error
	if !grpcResp.Success {
		h.errorHandler.WriteErrorResponse(w, http.StatusNotFound, apierrors.ErrorCodeMigrationNotFound, grpcResp.ErrorMessage, requestID)
		return
	}

	// Convert gRPC response to HTTP response
	httpResp := h.grpcToHTTP.GetMigrationStatusResponse(grpcResp)

	// Write HTTP response
	h.writeJSONResponse(w, http.StatusOK, httpResp)
}

// ListStorageNodes handles GET /v1/admin/storage-nodes requests.
func (h *Handlers) ListStorageNodes(w http.ResponseWriter, r *http.Request) {
	requestID := r.Header.Get("X-Request-ID")

	// Convert HTTP request to gRPC request
	grpcReq, err := h.httpToGRPC.ListStorageNodesRequest(r)
	if err != nil {
		h.errorHandler.WriteValidationError(w, err.Error(), requestID)
		return
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()

	// Call gRPC client
	grpcResp, err := h.grpcClient.ListStorageNodes(ctx, grpcReq)
	if err != nil {
		h.errorHandler.HandleError(w, r, err)
		return
	}

	// Check for application-level error
	if !grpcResp.Success {
		h.errorHandler.WriteErrorResponse(w, http.StatusInternalServerError, apierrors.ErrorCodeInternalError, grpcResp.ErrorMessage, requestID)
		return
	}

	// Convert gRPC response to HTTP response
	httpResp := h.grpcToHTTP.ListStorageNodesResponse(grpcResp)

	// Write HTTP response
	h.writeJSONResponse(w, http.StatusOK, httpResp)
}

// writeJSONResponse writes a JSON response to the HTTP response writer.
func (h *Handlers) writeJSONResponse(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		h.logger.Error("failed to encode response", zap.Error(err))
	}
}

