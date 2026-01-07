package store

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// IdempotencyStore handles Redis idempotency operations
type IdempotencyStore struct {
	client *redis.Client
}

// NewIdempotencyStore creates a new idempotency store
func NewIdempotencyStore(addr, password string, db int) *IdempotencyStore {
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})

	return &IdempotencyStore{client: client}
}

// Get retrieves cached response
func (s *IdempotencyStore) Get(ctx context.Context, tenantID, key, idempotencyKey string) ([]byte, error) {
	redisKey := s.buildKey(tenantID, key, idempotencyKey)
	data, err := s.client.Get(ctx, redisKey).Bytes()
	if err == redis.Nil {
		return nil, fmt.Errorf("idempotency key not found")
	}
	if err != nil {
		return nil, err
	}
	return data, nil
}

// Set stores response with TTL
func (s *IdempotencyStore) Set(
	ctx context.Context,
	tenantID, key, idempotencyKey string,
	data []byte,
	ttl time.Duration,
) error {
	redisKey := s.buildKey(tenantID, key, idempotencyKey)
	return s.client.Set(ctx, redisKey, data, ttl).Err()
}

// Delete removes an idempotency key
func (s *IdempotencyStore) Delete(ctx context.Context, tenantID, key, idempotencyKey string) error {
	redisKey := s.buildKey(tenantID, key, idempotencyKey)
	return s.client.Del(ctx, redisKey).Err()
}

// buildKey creates a Redis key from tenant, key, and idempotency key
func (s *IdempotencyStore) buildKey(tenantID, key, idempotencyKey string) string {
	return fmt.Sprintf("idempotency:%s:%s:%s", tenantID, key, idempotencyKey)
}

// Close closes the Redis client
func (s *IdempotencyStore) Close() error {
	return s.client.Close()
}

// Ping checks the Redis connection
func (s *IdempotencyStore) Ping(ctx context.Context) error {
	return s.client.Ping(ctx).Err()
}
