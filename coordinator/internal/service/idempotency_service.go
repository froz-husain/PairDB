package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/devrev/pairdb/coordinator/internal/store"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// IdempotencyService manages idempotency keys for write operations
type IdempotencyService struct {
	idempotencyStore store.IdempotencyStore
	ttl              time.Duration
	logger           *zap.Logger
}

// IdempotencyResponse represents a cached response
type IdempotencyResponse struct {
	Success     bool
	Key         string
	VectorClock []byte // Serialized vector clock
	ReplicaCount int32
	Consistency string
	Timestamp   time.Time
}

// NewIdempotencyService creates a new idempotency service
func NewIdempotencyService(
	idempotencyStore store.IdempotencyStore,
	ttl time.Duration,
	logger *zap.Logger,
) *IdempotencyService {
	return &IdempotencyService{
		idempotencyStore: idempotencyStore,
		ttl:              ttl,
		logger:           logger,
	}
}

// Generate generates a server-side idempotency key
func (s *IdempotencyService) Generate(tenantID, key string) string {
	// Generate based on tenant, key, and timestamp for uniqueness
	data := fmt.Sprintf("%s:%s:%d:%s", tenantID, key, time.Now().UnixNano(), uuid.New().String())
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:])
}

// Get retrieves a cached idempotency response
func (s *IdempotencyService) Get(ctx context.Context, tenantID, key, idempotencyKey string) (*IdempotencyResponse, error) {
	storeKey := s.buildStoreKey(tenantID, key, idempotencyKey)

	data, err := s.idempotencyStore.Get(ctx, storeKey)
	if err != nil {
		if err == store.ErrNotFound {
			s.logger.Debug("Idempotency key not found",
				zap.String("tenant_id", tenantID),
				zap.String("key", key),
				zap.String("idempotency_key", idempotencyKey))
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get idempotency response: %w", err)
	}

	if data == nil {
		return nil, nil
	}

	// Parse the response
	response, ok := data.(*IdempotencyResponse)
	if !ok {
		s.logger.Error("Invalid idempotency response type",
			zap.String("tenant_id", tenantID),
			zap.String("key", key))
		return nil, fmt.Errorf("invalid idempotency response type")
	}

	s.logger.Debug("Idempotency response found",
		zap.String("tenant_id", tenantID),
		zap.String("key", key),
		zap.String("idempotency_key", idempotencyKey))

	return response, nil
}

// Store stores an idempotency response
func (s *IdempotencyService) Store(
	ctx context.Context,
	tenantID, key, idempotencyKey string,
	response *IdempotencyResponse,
) error {
	storeKey := s.buildStoreKey(tenantID, key, idempotencyKey)

	response.Timestamp = time.Now()

	if err := s.idempotencyStore.Set(ctx, storeKey, response, s.ttl); err != nil {
		return fmt.Errorf("failed to store idempotency response: %w", err)
	}

	s.logger.Debug("Stored idempotency response",
		zap.String("tenant_id", tenantID),
		zap.String("key", key),
		zap.String("idempotency_key", idempotencyKey),
		zap.Duration("ttl", s.ttl))

	return nil
}

// Delete deletes an idempotency key (used for cleanup)
func (s *IdempotencyService) Delete(ctx context.Context, tenantID, key, idempotencyKey string) error {
	storeKey := s.buildStoreKey(tenantID, key, idempotencyKey)

	if err := s.idempotencyStore.Delete(ctx, storeKey); err != nil {
		return fmt.Errorf("failed to delete idempotency key: %w", err)
	}

	s.logger.Debug("Deleted idempotency key",
		zap.String("tenant_id", tenantID),
		zap.String("key", key),
		zap.String("idempotency_key", idempotencyKey))

	return nil
}

// buildStoreKey builds the store key for idempotency
func (s *IdempotencyService) buildStoreKey(tenantID, key, idempotencyKey string) string {
	return fmt.Sprintf("idempotency:%s:%s:%s", tenantID, key, idempotencyKey)
}

// ValidateIdempotencyKey validates if an idempotency key format is valid
func (s *IdempotencyService) ValidateIdempotencyKey(idempotencyKey string) bool {
	// Idempotency keys should be 64 character hex strings (SHA256)
	if len(idempotencyKey) != 64 {
		return false
	}
	for _, c := range idempotencyKey {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}
