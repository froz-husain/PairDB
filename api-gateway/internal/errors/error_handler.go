// Package errors provides error handling and HTTP status code mapping for the API Gateway.
package errors

import (
	"encoding/json"
	"net/http"

	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ErrorCode represents application-specific error codes.
type ErrorCode string

const (
	// General errors
	ErrorCodeUnknown         ErrorCode = "UNKNOWN"
	ErrorCodeInvalidRequest  ErrorCode = "INVALID_REQUEST"
	ErrorCodeInternalError   ErrorCode = "INTERNAL_ERROR"
	ErrorCodeServiceDown     ErrorCode = "SERVICE_UNAVAILABLE"
	ErrorCodeTimeout         ErrorCode = "TIMEOUT"
	ErrorCodeRateLimited     ErrorCode = "RATE_LIMITED"

	// Tenant errors
	ErrorCodeTenantNotFound ErrorCode = "TENANT_NOT_FOUND"
	ErrorCodeTenantExists   ErrorCode = "TENANT_EXISTS"

	// Key-Value errors
	ErrorCodeKeyNotFound     ErrorCode = "KEY_NOT_FOUND"
	ErrorCodeQuorumNotReached ErrorCode = "QUORUM_NOT_REACHED"

	// Idempotency errors
	ErrorCodeIdempotencyConflict ErrorCode = "IDEMPOTENCY_KEY_CONFLICT"

	// Replication errors
	ErrorCodeInvalidReplicationFactor ErrorCode = "INVALID_REPLICATION_FACTOR"

	// Node errors
	ErrorCodeNodeInUse          ErrorCode = "NODE_IN_USE"
	ErrorCodeMigrationNotFound  ErrorCode = "MIGRATION_NOT_FOUND"

	// Auth errors
	ErrorCodeUnauthorized ErrorCode = "UNAUTHORIZED"
	ErrorCodeForbidden    ErrorCode = "FORBIDDEN"
)

// ErrorResponse represents the standard error response format.
type ErrorResponse struct {
	Status    string    `json:"status"`
	ErrorCode ErrorCode `json:"error_code"`
	Message   string    `json:"message"`
	RequestID string    `json:"request_id,omitempty"`
}

// Handler provides error handling functionality.
type Handler struct {
	logger *zap.Logger
}

// NewHandler creates a new error handler.
func NewHandler(logger *zap.Logger) *Handler {
	return &Handler{
		logger: logger,
	}
}

// HandleError processes an error and writes an appropriate HTTP response.
func (h *Handler) HandleError(w http.ResponseWriter, r *http.Request, err error) {
	statusCode := h.GRPCToHTTPStatus(err)
	errorCode := h.GRPCToErrorCode(err)
	message := err.Error()

	// Extract gRPC status message if available
	if st, ok := status.FromError(err); ok {
		message = st.Message()
	}

	requestID := r.Header.Get("X-Request-ID")

	h.WriteErrorResponse(w, statusCode, errorCode, message, requestID)
}

// GRPCToHTTPStatus converts a gRPC error to an HTTP status code.
func (h *Handler) GRPCToHTTPStatus(err error) int {
	if err == nil {
		return http.StatusOK
	}

	st, ok := status.FromError(err)
	if !ok {
		return http.StatusInternalServerError
	}

	switch st.Code() {
	case codes.OK:
		return http.StatusOK
	case codes.InvalidArgument:
		return http.StatusBadRequest
	case codes.NotFound:
		return http.StatusNotFound
	case codes.AlreadyExists:
		return http.StatusConflict
	case codes.PermissionDenied:
		return http.StatusForbidden
	case codes.Unauthenticated:
		return http.StatusUnauthorized
	case codes.ResourceExhausted:
		return http.StatusTooManyRequests
	case codes.FailedPrecondition:
		return http.StatusPreconditionFailed
	case codes.Aborted:
		return http.StatusConflict
	case codes.OutOfRange:
		return http.StatusBadRequest
	case codes.Unimplemented:
		return http.StatusNotImplemented
	case codes.Internal:
		return http.StatusInternalServerError
	case codes.Unavailable:
		return http.StatusServiceUnavailable
	case codes.DeadlineExceeded:
		return http.StatusGatewayTimeout
	default:
		return http.StatusInternalServerError
	}
}

// GRPCToErrorCode converts a gRPC error to an application error code.
func (h *Handler) GRPCToErrorCode(err error) ErrorCode {
	if err == nil {
		return ErrorCodeUnknown
	}

	st, ok := status.FromError(err)
	if !ok {
		return ErrorCodeInternalError
	}

	// Check for specific error messages to determine the error code
	msg := st.Message()

	switch st.Code() {
	case codes.InvalidArgument:
		return ErrorCodeInvalidRequest
	case codes.NotFound:
		// Determine specific not found error based on message
		if containsAny(msg, "tenant") {
			return ErrorCodeTenantNotFound
		}
		if containsAny(msg, "key") {
			return ErrorCodeKeyNotFound
		}
		if containsAny(msg, "migration") {
			return ErrorCodeMigrationNotFound
		}
		return ErrorCodeInvalidRequest
	case codes.AlreadyExists:
		if containsAny(msg, "tenant") {
			return ErrorCodeTenantExists
		}
		if containsAny(msg, "idempotency") {
			return ErrorCodeIdempotencyConflict
		}
		return ErrorCodeInvalidRequest
	case codes.PermissionDenied:
		return ErrorCodeForbidden
	case codes.Unauthenticated:
		return ErrorCodeUnauthorized
	case codes.ResourceExhausted:
		return ErrorCodeRateLimited
	case codes.Unavailable:
		return ErrorCodeServiceDown
	case codes.DeadlineExceeded:
		return ErrorCodeTimeout
	default:
		return ErrorCodeInternalError
	}
}

// WriteErrorResponse writes a formatted error response to the HTTP response writer.
func (h *Handler) WriteErrorResponse(w http.ResponseWriter, statusCode int, errorCode ErrorCode, message string, requestID string) {
	h.logger.Warn("HTTP error response",
		zap.Int("status_code", statusCode),
		zap.String("error_code", string(errorCode)),
		zap.String("message", message),
		zap.String("request_id", requestID),
	)

	resp := ErrorResponse{
		Status:    "error",
		ErrorCode: errorCode,
		Message:   message,
		RequestID: requestID,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(resp)
}

// WriteValidationError writes a validation error response.
func (h *Handler) WriteValidationError(w http.ResponseWriter, message string, requestID string) {
	h.WriteErrorResponse(w, http.StatusBadRequest, ErrorCodeInvalidRequest, message, requestID)
}

// WriteInternalError writes an internal error response.
func (h *Handler) WriteInternalError(w http.ResponseWriter, message string, requestID string) {
	h.WriteErrorResponse(w, http.StatusInternalServerError, ErrorCodeInternalError, message, requestID)
}

// WriteServiceUnavailable writes a service unavailable response.
func (h *Handler) WriteServiceUnavailable(w http.ResponseWriter, message string, requestID string) {
	h.WriteErrorResponse(w, http.StatusServiceUnavailable, ErrorCodeServiceDown, message, requestID)
}

// WriteRateLimitedError writes a rate limit exceeded response.
func (h *Handler) WriteRateLimitedError(w http.ResponseWriter, requestID string) {
	h.WriteErrorResponse(w, http.StatusTooManyRequests, ErrorCodeRateLimited, "rate limit exceeded", requestID)
}

// containsAny checks if the string contains any of the substrings (case-insensitive).
func containsAny(s string, substrs ...string) bool {
	sLower := toLower(s)
	for _, substr := range substrs {
		if contains(sLower, toLower(substr)) {
			return true
		}
	}
	return false
}

// toLower converts string to lowercase.
func toLower(s string) string {
	result := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		result[i] = c
	}
	return string(result)
}

// contains checks if s contains substr.
func contains(s, substr string) bool {
	if len(substr) > len(s) {
		return false
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

