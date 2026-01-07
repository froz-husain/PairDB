package unit

import (
	"net/http"
	"net/http/httptest"
	"testing"

	apierrors "github.com/devrev/pairdb/api-gateway/internal/errors"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestErrorHandler_GRPCToHTTPStatus(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	handler := apierrors.NewHandler(logger)

	tests := []struct {
		name         string
		grpcCode     codes.Code
		expectedHTTP int
	}{
		{"OK", codes.OK, http.StatusOK},
		{"InvalidArgument", codes.InvalidArgument, http.StatusBadRequest},
		{"NotFound", codes.NotFound, http.StatusNotFound},
		{"AlreadyExists", codes.AlreadyExists, http.StatusConflict},
		{"PermissionDenied", codes.PermissionDenied, http.StatusForbidden},
		{"Unauthenticated", codes.Unauthenticated, http.StatusUnauthorized},
		{"ResourceExhausted", codes.ResourceExhausted, http.StatusTooManyRequests},
		{"FailedPrecondition", codes.FailedPrecondition, http.StatusPreconditionFailed},
		{"Aborted", codes.Aborted, http.StatusConflict},
		{"OutOfRange", codes.OutOfRange, http.StatusBadRequest},
		{"Unimplemented", codes.Unimplemented, http.StatusNotImplemented},
		{"Internal", codes.Internal, http.StatusInternalServerError},
		{"Unavailable", codes.Unavailable, http.StatusServiceUnavailable},
		{"DeadlineExceeded", codes.DeadlineExceeded, http.StatusGatewayTimeout},
		{"Unknown", codes.Unknown, http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := status.Error(tt.grpcCode, "test error")
			httpStatus := handler.GRPCToHTTPStatus(err)
			assert.Equal(t, tt.expectedHTTP, httpStatus)
		})
	}

	t.Run("nil error", func(t *testing.T) {
		httpStatus := handler.GRPCToHTTPStatus(nil)
		assert.Equal(t, http.StatusOK, httpStatus)
	})
}

func TestErrorHandler_GRPCToErrorCode(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	handler := apierrors.NewHandler(logger)

	tests := []struct {
		name         string
		grpcCode     codes.Code
		message      string
		expectedCode apierrors.ErrorCode
	}{
		{"InvalidArgument", codes.InvalidArgument, "invalid request", apierrors.ErrorCodeInvalidRequest},
		{"NotFound tenant", codes.NotFound, "tenant not found", apierrors.ErrorCodeTenantNotFound},
		{"NotFound key", codes.NotFound, "key not found", apierrors.ErrorCodeKeyNotFound},
		{"NotFound migration", codes.NotFound, "migration not found", apierrors.ErrorCodeMigrationNotFound},
		{"AlreadyExists tenant", codes.AlreadyExists, "tenant already exists", apierrors.ErrorCodeTenantExists},
		{"AlreadyExists idempotency", codes.AlreadyExists, "idempotency conflict", apierrors.ErrorCodeIdempotencyConflict},
		{"PermissionDenied", codes.PermissionDenied, "access denied", apierrors.ErrorCodeForbidden},
		{"Unauthenticated", codes.Unauthenticated, "not authenticated", apierrors.ErrorCodeUnauthorized},
		{"ResourceExhausted", codes.ResourceExhausted, "rate limit", apierrors.ErrorCodeRateLimited},
		{"Unavailable", codes.Unavailable, "service down", apierrors.ErrorCodeServiceDown},
		{"DeadlineExceeded", codes.DeadlineExceeded, "timeout", apierrors.ErrorCodeTimeout},
		{"Internal", codes.Internal, "internal error", apierrors.ErrorCodeInternalError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := status.Error(tt.grpcCode, tt.message)
			errorCode := handler.GRPCToErrorCode(err)
			assert.Equal(t, tt.expectedCode, errorCode)
		})
	}

	t.Run("nil error", func(t *testing.T) {
		errorCode := handler.GRPCToErrorCode(nil)
		assert.Equal(t, apierrors.ErrorCodeUnknown, errorCode)
	})
}

func TestErrorHandler_WriteErrorResponse(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	handler := apierrors.NewHandler(logger)

	t.Run("writes error response", func(t *testing.T) {
		w := httptest.NewRecorder()
		handler.WriteErrorResponse(w, http.StatusBadRequest, apierrors.ErrorCodeInvalidRequest, "invalid key", "req-123")

		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Equal(t, "application/json", w.Header().Get("Content-Type"))
		assert.Contains(t, w.Body.String(), `"status":"error"`)
		assert.Contains(t, w.Body.String(), `"error_code":"INVALID_REQUEST"`)
		assert.Contains(t, w.Body.String(), `"message":"invalid key"`)
		assert.Contains(t, w.Body.String(), `"request_id":"req-123"`)
	})
}

func TestErrorHandler_WriteValidationError(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	handler := apierrors.NewHandler(logger)

	w := httptest.NewRecorder()
	handler.WriteValidationError(w, "key is required", "req-456")

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), `"error_code":"INVALID_REQUEST"`)
	assert.Contains(t, w.Body.String(), `"message":"key is required"`)
}

func TestErrorHandler_WriteInternalError(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	handler := apierrors.NewHandler(logger)

	w := httptest.NewRecorder()
	handler.WriteInternalError(w, "unexpected error", "req-789")

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), `"error_code":"INTERNAL_ERROR"`)
}

func TestErrorHandler_WriteServiceUnavailable(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	handler := apierrors.NewHandler(logger)

	w := httptest.NewRecorder()
	handler.WriteServiceUnavailable(w, "coordinator unavailable", "req-abc")

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	assert.Contains(t, w.Body.String(), `"error_code":"SERVICE_UNAVAILABLE"`)
}

func TestErrorHandler_WriteRateLimitedError(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	handler := apierrors.NewHandler(logger)

	w := httptest.NewRecorder()
	handler.WriteRateLimitedError(w, "req-def")

	assert.Equal(t, http.StatusTooManyRequests, w.Code)
	assert.Contains(t, w.Body.String(), `"error_code":"RATE_LIMITED"`)
	assert.Contains(t, w.Body.String(), `"message":"rate limit exceeded"`)
}

func TestErrorHandler_HandleError(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	handler := apierrors.NewHandler(logger)

	t.Run("handles gRPC error", func(t *testing.T) {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/test", nil)
		r.Header.Set("X-Request-ID", "req-123")

		err := status.Error(codes.NotFound, "key not found")
		handler.HandleError(w, r, err)

		assert.Equal(t, http.StatusNotFound, w.Code)
		assert.Contains(t, w.Body.String(), `"message":"key not found"`)
	})
}

