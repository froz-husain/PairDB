package service

import (
	"context"
)

// CoordinatorServiceInterface defines the interface for coordinator services
type CoordinatorServiceInterface interface {
	WriteKeyValue(
		ctx context.Context,
		tenantID, key string,
		value []byte,
		consistency, idempotencyKey string,
	) (*WriteResult, error)

	ReadKeyValue(
		ctx context.Context,
		tenantID, key string,
		consistency string,
	) (*ReadResult, error)
}

// Ensure V2 implementation satisfies the interface
var _ CoordinatorServiceInterface = (*CoordinatorServiceV2)(nil)
