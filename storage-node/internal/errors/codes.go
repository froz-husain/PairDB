package errors

import (
	"fmt"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ErrorCode represents internal error codes for storage operations
type ErrorCode int

const (
	// Success
	ErrCodeOK ErrorCode = 0

	// Client errors (4xx equivalent)
	ErrCodeInvalidArgument ErrorCode = 1000
	ErrCodeKeyNotFound     ErrorCode = 1001
	ErrCodeKeyTooLarge     ErrorCode = 1002
	ErrCodeValueTooLarge   ErrorCode = 1003
	ErrCodeInvalidTenantID ErrorCode = 1004
	ErrCodeInvalidKey      ErrorCode = 1005
	ErrCodeChecksumFailed  ErrorCode = 1006

	// Server errors (5xx equivalent)
	ErrCodeInternal        ErrorCode = 2000
	ErrCodeUnavailable     ErrorCode = 2001
	ErrCodeDiskFull        ErrorCode = 2002
	ErrCodeDiskThrottled   ErrorCode = 2003
	ErrCodeCommitLogFailed ErrorCode = 2004
	ErrCodeMemTableFailed  ErrorCode = 2005
	ErrCodeSSTableFailed   ErrorCode = 2006
	ErrCodeCorruptedData   ErrorCode = 2007
	ErrCodeResourceExhausted ErrorCode = 2008
)

// StorageError represents a structured error with code and context
type StorageError struct {
	Code    ErrorCode
	Message string
	Details map[string]interface{}
	Cause   error
}

// Error implements the error interface
func (e *StorageError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Cause)
	}
	return e.Message
}

// Unwrap returns the underlying error
func (e *StorageError) Unwrap() error {
	return e.Cause
}

// ToGRPCStatus converts StorageError to gRPC status
func (e *StorageError) ToGRPCStatus() *status.Status {
	grpcCode := e.toGRPCCode()
	return status.New(grpcCode, e.Error())
}

// toGRPCCode maps internal error codes to gRPC codes
func (e *StorageError) toGRPCCode() codes.Code {
	switch e.Code {
	case ErrCodeOK:
		return codes.OK
	case ErrCodeInvalidArgument, ErrCodeKeyTooLarge, ErrCodeValueTooLarge,
		ErrCodeInvalidTenantID, ErrCodeInvalidKey:
		return codes.InvalidArgument
	case ErrCodeKeyNotFound:
		return codes.NotFound
	case ErrCodeDiskFull, ErrCodeResourceExhausted:
		return codes.ResourceExhausted
	case ErrCodeDiskThrottled:
		return codes.Unavailable
	case ErrCodeChecksumFailed, ErrCodeCorruptedData:
		return codes.DataLoss
	case ErrCodeUnavailable:
		return codes.Unavailable
	default:
		return codes.Internal
	}
}

// NewStorageError creates a new StorageError
func NewStorageError(code ErrorCode, message string, cause error) *StorageError {
	return &StorageError{
		Code:    code,
		Message: message,
		Details: make(map[string]interface{}),
		Cause:   cause,
	}
}

// WithDetail adds a detail to the error
func (e *StorageError) WithDetail(key string, value interface{}) *StorageError {
	e.Details[key] = value
	return e
}

// Convenience constructors for common errors

func InvalidArgument(message string, cause error) *StorageError {
	return NewStorageError(ErrCodeInvalidArgument, message, cause)
}

func KeyNotFound(tenantID, key string) *StorageError {
	return NewStorageError(ErrCodeKeyNotFound, fmt.Sprintf("key not found: %s:%s", tenantID, key), nil).
		WithDetail("tenant_id", tenantID).
		WithDetail("key", key)
}

func KeyTooLarge(size, maxSize int) *StorageError {
	return NewStorageError(ErrCodeKeyTooLarge, fmt.Sprintf("key size %d exceeds maximum %d", size, maxSize), nil).
		WithDetail("size", size).
		WithDetail("max_size", maxSize)
}

func ValueTooLarge(size, maxSize int) *StorageError {
	return NewStorageError(ErrCodeValueTooLarge, fmt.Sprintf("value size %d exceeds maximum %d", size, maxSize), nil).
		WithDetail("size", size).
		WithDetail("max_size", maxSize)
}

func InvalidTenantID(tenantID, reason string) *StorageError {
	return NewStorageError(ErrCodeInvalidTenantID, fmt.Sprintf("invalid tenant ID '%s': %s", tenantID, reason), nil).
		WithDetail("tenant_id", tenantID).
		WithDetail("reason", reason)
}

func InvalidKey(key, reason string) *StorageError {
	return NewStorageError(ErrCodeInvalidKey, fmt.Sprintf("invalid key '%s': %s", key, reason), nil).
		WithDetail("key", key).
		WithDetail("reason", reason)
}

func ChecksumFailed(expected, actual uint32) *StorageError {
	return NewStorageError(ErrCodeChecksumFailed, fmt.Sprintf("checksum validation failed: expected %d, got %d", expected, actual), nil).
		WithDetail("expected", expected).
		WithDetail("actual", actual)
}

func InternalError(message string, cause error) *StorageError {
	return NewStorageError(ErrCodeInternal, message, cause)
}

func Unavailable(message string, cause error) *StorageError {
	return NewStorageError(ErrCodeUnavailable, message, cause)
}

func DiskFull(usagePercent float64, availableBytes uint64) *StorageError {
	return NewStorageError(ErrCodeDiskFull, fmt.Sprintf("disk full: %.2f%% used, %d bytes available", usagePercent, availableBytes), nil).
		WithDetail("usage_percent", usagePercent).
		WithDetail("available_bytes", availableBytes)
}

func DiskThrottled(usagePercent float64) *StorageError {
	return NewStorageError(ErrCodeDiskThrottled, fmt.Sprintf("disk write throttled: %.2f%% used", usagePercent), nil).
		WithDetail("usage_percent", usagePercent)
}

func CommitLogFailed(message string, cause error) *StorageError {
	return NewStorageError(ErrCodeCommitLogFailed, message, cause)
}

func MemTableFailed(message string, cause error) *StorageError {
	return NewStorageError(ErrCodeMemTableFailed, message, cause)
}

func SSTableFailed(message string, cause error) *StorageError {
	return NewStorageError(ErrCodeSSTableFailed, message, cause)
}

func CorruptedData(message string, cause error) *StorageError {
	return NewStorageError(ErrCodeCorruptedData, message, cause)
}

func ResourceExhausted(resource string, current, limit int) *StorageError {
	return NewStorageError(ErrCodeResourceExhausted, fmt.Sprintf("%s exhausted: %d/%d", resource, current, limit), nil).
		WithDetail("resource", resource).
		WithDetail("current", current).
		WithDetail("limit", limit)
}

// IsStorageError checks if an error is a StorageError
func IsStorageError(err error) bool {
	_, ok := err.(*StorageError)
	return ok
}

// GetCode extracts the error code from an error
func GetCode(err error) ErrorCode {
	if se, ok := err.(*StorageError); ok {
		return se.Code
	}
	return ErrCodeInternal
}
